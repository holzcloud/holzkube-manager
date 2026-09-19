package kube

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Pod logs (2026-09-19).
//
// # Why this is the first thing the Kubernetes half was missing
//
// Everything built before this says WHAT is wrong -- a pod is in
// CrashLoopBackOff, a deployment has one of two ready. None of it says WHY, and
// the why is almost always in the log of the container that died. Without it an
// operator reads the screen, learns something is broken, and reaches for kubectl
// anyway.
//
// # The previous container is the point, not a nicety
//
// A pod in CrashLoopBackOff is, at the moment you look at it, either starting or
// waiting to start. Its CURRENT container has produced nothing yet. The output
// that explains the crash belongs to the container that already exited, and
// Kubernetes keeps exactly one of those. `Previous` is therefore the flag that
// makes this useful for the case people actually open it for, and a log view
// without it would answer every crash loop with an empty box.
//
// # Nothing here is archived
//
// A log line is whatever the workload printed, which regularly includes tokens,
// connection strings and personal data. D-16 keeps the audit archive forever, so
// a log in it is a secret in it, forever. The archive records that somebody read
// the log of a named pod; it never records a byte of what they read.

// ErrNoSuchContainer reports a container name the pod does not have.
var ErrNoSuchContainer = errors.New("kube: that pod has no such container")

// MaxLogBytes bounds one log read.
//
// A container can produce a gigabyte and this process would hold it. The cut is
// reported rather than made quietly, for the reason a truncated proxy response
// is: half a log that looked whole would be read as the whole log, and the line
// that explains the crash is usually the last one.
const MaxLogBytes = 1 << 20

// MaxLogLines is the default number of lines asked for.
//
// Tail rather than head, because the end is where the failure is. A caller may
// ask for fewer; it may not ask for the whole log, which is what MaxLogBytes is
// for.
const MaxLogLines int64 = 1000

// LogOptions is what to read.
type LogOptions struct {
	// Container picks one of a pod's containers. Empty means the pod's only
	// container, and is refused when the pod has several -- guessing there
	// would show somebody the log of a sidecar and let them conclude the
	// application printed nothing.
	Container string

	// Previous reads the container that exited rather than the one running.
	Previous bool

	// TailLines defaults to MaxLogLines.
	TailLines int64

	// Since limits how far back to read. Zero means no limit.
	Since time.Duration
}

// PodLog is what a container said.
type PodLog struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Container string `json:"container"`

	// Previous echoes which of the two logs this is, so a screen cannot label
	// the dead container's output as the running one's.
	Previous bool `json:"previous"`

	Lines []string `json:"lines"`

	// Truncated is true when the container said more than MaxLogBytes. The cut
	// is at the START: the end of a log is the part that explains the failure.
	Truncated bool `json:"truncated"`
}

// ContainersOf lists a pod's containers, so a caller can pick one.
//
// Init containers are included and marked, because a pod stuck in Init is a pod
// whose interesting log belongs to a container the ordinary list does not show.
func (c *Client) ContainersOf(ctx context.Context, namespace, name string) ([]Container, error) {
	pod, err := c.cs.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("%w: %s/%s", ErrNoSuchWorkload, namespace, name)
		}
		return nil, fmt.Errorf("kube: reading %s/%s: %w", namespace, name, err)
	}
	return containersOf(pod), nil
}

// PodLogs reads one container's output.
func (c *Client) PodLogs(
	ctx context.Context, namespace, name string, opts LogOptions,
) (PodLog, error) {
	pod, err := c.cs.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return PodLog{}, fmt.Errorf("%w: %s/%s", ErrNoSuchWorkload, namespace, name)
		}
		return PodLog{}, fmt.Errorf("kube: reading %s/%s: %w", namespace, name, err)
	}

	container, err := pickContainer(pod, opts.Container)
	if err != nil {
		return PodLog{}, err
	}

	tail := opts.TailLines
	if tail <= 0 || tail > MaxLogLines {
		tail = MaxLogLines
	}
	request := &corev1.PodLogOptions{
		Container: container,
		Previous:  opts.Previous,
		TailLines: &tail,
	}
	if opts.Since > 0 {
		seconds := int64(opts.Since.Seconds())
		request.SinceSeconds = &seconds
	}

	stream, err := c.cs.CoreV1().Pods(namespace).GetLogs(name, request).Stream(ctx)
	if err != nil {
		// A pod that has not started has no log yet, and a previous container
		// that never existed has none either. Both are ordinary states of a
		// broken workload rather than failures of this product, and the API
		// server says which in its message.
		if apierrors.IsBadRequest(err) {
			return PodLog{
				Namespace: namespace, Name: name, Container: container, Previous: opts.Previous,
				Lines: []string{fmt.Sprintf("(the cluster has no such log: %s)", apiMessage(err))},
			}, nil
		}
		return PodLog{}, fmt.Errorf("kube: reading the log of %s/%s: %w", namespace, name, err)
	}
	defer func() { _ = stream.Close() }()

	lines, truncated, err := readLines(stream)
	if err != nil {
		return PodLog{}, fmt.Errorf("kube: reading the log of %s/%s: %w", namespace, name, err)
	}

	return PodLog{
		Namespace: namespace,
		Name:      name,
		Container: container,
		Previous:  opts.Previous,
		Lines:     lines,
		Truncated: truncated,
	}, nil
}

// pickContainer resolves which container was meant, and refuses to guess.
func pickContainer(pod *corev1.Pod, wanted string) (string, error) {
	all := containersOf(pod)
	if wanted != "" {
		for _, c := range all {
			if c.Name == wanted {
				return wanted, nil
			}
		}
		names := make([]string, 0, len(all))
		for _, c := range all {
			names = append(names, c.Name)
		}
		return "", fmt.Errorf("%w: %s/%s has %s", ErrNoSuchContainer,
			pod.Namespace, pod.Name, strings.Join(names, ", "))
	}

	// Only the ordinary containers count for the default. An init container is
	// never what somebody means by "the log of this pod" unless they name it.
	ordinary := make([]string, 0, len(pod.Spec.Containers))
	for _, c := range pod.Spec.Containers {
		ordinary = append(ordinary, c.Name)
	}
	switch len(ordinary) {
	case 0:
		return "", fmt.Errorf("%w: %s/%s has no containers", ErrNoSuchContainer,
			pod.Namespace, pod.Name)
	case 1:
		return ordinary[0], nil
	default:
		// Refused rather than guessed: picking the first would show a sidecar's
		// log and let somebody conclude the application printed nothing.
		return "", fmt.Errorf("%w: %s/%s has %d containers (%s), so one has to be named",
			ErrNoSuchContainer, pod.Namespace, pod.Name, len(ordinary),
			strings.Join(ordinary, ", "))
	}
}

// readLines reads the stream, keeping the END when it is too long.
//
// The cut is at the FRONT and that is the whole point: the last line is the one
// that explains the crash. The first attempt wrapped the stream in an
// io.LimitReader, which cuts the other end -- it bounded memory correctly and
// threw away the only line anybody opens a log for. The test caught it.
//
// So the window rolls: lines are appended and dropped from the front as the
// total passes MaxLogBytes, which holds the same amount of memory while keeping
// the other half of the log. hardCap bounds the READING as well, because a
// rolling window over an endless stream is still an endless read.
func readLines(stream io.Reader) ([]string, bool, error) {
	const hardCap = 8 * MaxLogBytes

	reader := bufio.NewReader(io.LimitReader(stream, hardCap+1))
	var (
		lines     []string
		size      int
		read      int
		truncated bool
	)
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			read += len(line)
			size += len(line)
			lines = append(lines, strings.TrimRight(line, "\r\n"))
			// Drop from the front while it is over the cap, which is what keeps
			// the end.
			for size > MaxLogBytes && len(lines) > 1 {
				size -= len(lines[0]) + 1
				lines = lines[1:]
				truncated = true
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, false, err
		}
	}
	if read > hardCap {
		truncated = true
	}
	return lines, truncated, nil
}

// apiMessage is the API server's own sentence, without the wrapping.
func apiMessage(err error) string {
	var status apierrors.APIStatus
	if errors.As(err, &status) && status.Status().Message != "" {
		return status.Status().Message
	}
	return err.Error()
}
