package kube

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

// Running a command in a container (2026-09-19).
//
// # The most dangerous thing in this product, and what makes it acceptable
//
// `kubectl exec` is a shell inside somebody's workload. Offered carelessly from
// a management product it is a remote shell with the product's own credentials,
// reachable by anybody with a session, and recorded as "exec" in an archive that
// cannot say what was run.
//
// Four things make it something this product can have:
//
//  1. IT IS NOT A SHELL. A command and its arguments are sent, it runs, and the
//     output comes back. There is no terminal, no stdin, no session that stays
//     open. `sh -lc "..."` is refused by name -- passing a shell a string is
//     exactly the shape that makes the archive useless, because the archive
//     would hold "sh -lc" and the interesting half would be in an argument
//     nobody reads.
//
//  2. THE COMMAND IS THE EVENT. Every argument is archived in clear. That is
//     only possible because of (1): a shell string would be one opaque argument.
//
//  3. IT RUNS AS THE OPERATOR when impersonation is on. Without that this is a
//     shell in somebody's cluster attributed to a product, which is precisely
//     what the identity work was for. The route refuses when the cluster has no
//     identity set, rather than quietly using system:masters.
//
//  4. IT IS BOUNDED. One command, a deadline, an output cap. No interactive
//     stream to leave open and nothing that outlives the request.
//
// What that costs: it cannot be used to poke around interactively, which is the
// point. Somebody who needs a shell has kubectl and their own credentials, and
// their cluster's audit log will say so.

// ErrExecRefused reports a command this product will not run.
var ErrExecRefused = errors.New("kube: that is not a command this will run")

// ErrNoIdentityForExec reports a cluster where nobody is being acted as.
var ErrNoIdentityForExec = errors.New(
	"kube: running a command needs this cluster to act as a person")

// MaxExecBytes bounds what one command may return.
const MaxExecBytes = 256 << 10

// ExecBudget bounds how long one command may take.
//
// Short on purpose: this runs a command and returns its output, and anything
// that takes longer than this is a thing to run as a Job rather than from a
// management screen.
const ExecBudget = 30 * time.Second

// shellsAndTheirFlags are the programs that turn a string into a command.
//
// Refused by name, because passing one a string defeats the archive: it would
// record "sh -lc" and the interesting half would sit in an argument nobody
// reads. The list is short and deliberate rather than a pattern -- a pattern
// would eventually refuse somebody's legitimate binary called "bash-exporter".
var shellsAndTheirFlags = map[string][]string{
	"sh":   {"-c", "-lc", "-ec"},
	"bash": {"-c", "-lc", "-ec"},
	"zsh":  {"-c", "-lc", "-ec"},
	"ash":  {"-c", "-lc", "-ec"},
	"dash": {"-c", "-lc", "-ec"},
}

// ExecResult is what a command said.
type ExecResult struct {
	Namespace string   `json:"namespace"`
	Pod       string   `json:"pod"`
	Container string   `json:"container"`
	Command   []string `json:"command"`

	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`

	// Truncated is true when the command said more than MaxExecBytes.
	Truncated bool `json:"truncated"`

	// Identity is who the cluster saw run this. It is carried into the answer
	// so the screen can say it rather than implying the product ran it.
	Identity string `json:"identity"`
}

// Exec runs one command in a container and returns what it said.
func (c *Client) Exec(
	ctx context.Context, namespace, pod, container string, command []string,
) (ExecResult, error) {
	if c.identity.Empty() {
		return ExecResult{}, fmt.Errorf("%w. Set one under \"who this acts as\", so your "+
			"cluster's audit log records the person who ran it rather than this product",
			ErrNoIdentityForExec)
	}
	if err := checkCommand(command); err != nil {
		return ExecResult{}, err
	}

	target, err := c.cs.CoreV1().Pods(namespace).Get(ctx, pod, metav1.GetOptions{})
	if err != nil {
		return ExecResult{}, err
	}
	name, err := pickContainer(target, container)
	if err != nil {
		return ExecResult{}, err
	}

	request := c.cs.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: name,
			Command:   command,
			Stdout:    true,
			Stderr:    true,
			// No stdin and no TTY. Both exist to keep a session open, and this
			// runs one command and returns.
			Stdin: false,
			TTY:   false,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(c.cfg, "POST", request.URL())
	if err != nil {
		return ExecResult{}, fmt.Errorf("kube: preparing to run in %s/%s: %w", namespace, pod, err)
	}

	var stdout, stderr bytes.Buffer
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &capped{to: &stdout},
		Stderr: &capped{to: &stderr},
	})

	out := ExecResult{
		Namespace: namespace, Pod: pod, Container: name, Command: command,
		Stdout:    stdout.String(),
		Stderr:    stderr.String(),
		Truncated: stdout.Len() >= MaxExecBytes || stderr.Len() >= MaxExecBytes,
		Identity:  c.identity.User,
	}
	if err != nil {
		// A non-zero exit is the command's answer rather than a failure of this
		// product -- `test -f /etc/passwd` returning 1 is information. It comes
		// back with whatever was written, and the error text alongside.
		out.Stderr = strings.TrimSpace(out.Stderr + "\n" + err.Error())
	}
	return out, nil
}

// checkCommand refuses the shapes that make the archive useless.
func checkCommand(command []string) error {
	if len(command) == 0 {
		return fmt.Errorf("%w: no command was given", ErrExecRefused)
	}
	for _, part := range command {
		if strings.ContainsAny(part, "\n\r") {
			return fmt.Errorf("%w: an argument cannot contain a line break", ErrExecRefused)
		}
	}

	program := command[0]
	if slash := strings.LastIndex(program, "/"); slash >= 0 {
		program = program[slash+1:]
	}
	flags, isShell := shellsAndTheirFlags[program]
	if !isShell {
		return nil
	}
	// A shell with no string to interpret is a shell somebody meant to start
	// interactively, which this cannot do anyway -- there is no stdin.
	if len(command) == 1 {
		return fmt.Errorf("%w: there is no terminal here, so a bare %s would do nothing",
			ErrExecRefused, program)
	}
	for _, flag := range flags {
		if command[1] == flag {
			return fmt.Errorf("%w: %s %s hides the real command inside a string, and this "+
				"product archives what was run. Give the program and its arguments instead",
				ErrExecRefused, program, flag)
		}
	}
	return nil
}

// capped is a writer that stops at MaxExecBytes.
//
// The cut is at the END here, unlike a log: a command's output is read from the
// top, and the beginning is what says whether it did anything.
type capped struct{ to *bytes.Buffer }

func (c *capped) Write(p []byte) (int, error) {
	room := MaxExecBytes - c.to.Len()
	if room <= 0 {
		// Reported as written so the far end does not error; the truncation is
		// carried in the result instead.
		return len(p), nil
	}
	if len(p) > room {
		c.to.Write(p[:room])
		return len(p), nil
	}
	c.to.Write(p)
	return len(p), nil
}
