// Package k8srelease names the newest stable Kubernetes patch of a minor.
//
// The Kubernetes upgrade chain passes through every minor between the cluster
// and its target, and each step on the way is the newest stable patch of its
// minor: the first release of a minor, its .0, is the oldest and the least
// fixed one. This package asks where that patch is written down.
//
// What is asked is https://dl.k8s.io/release/stable-1.N.txt, one small file per
// minor, and never latest-1.N.txt. The second is the list of the newest
// candidates, and it was measured to hand out a release candidate: latest-1.37.txt
// answered v1.37.0-rc.1 on 2026-10-12, while stable-1.37.txt answered v1.37.1.
// A chain step that is a release candidate is a step the operator would type
// into a cluster.
//
// The contract, measured on 2026-10-12 with curl from a workstation:
//
//	GET /release/stable-1.35.txt -> 200, content-type text/plain, the body
//	  "v1.35.9" with no line ending (7 bytes), cache-control public, max-age=86400
//	GET /release/stable-1.38.txt -> 404 with an XML body (NoSuchKey): a minor
//	  that has only alphas has no stable file
//	GET /release/stable-1.99.txt -> 404 with an XML body (NoSuchKey)
//	five answers for 1.33 to 1.37 took 0.06 to 0.24 seconds
//
// There is no document that promises this format. That is why the client is
// strict: it accepts a 200 with exactly one line v1.N.<patch> of the minor it
// asked for, and everything else is an error. A format that changes therefore
// shows up as a plan that cannot be made, never as a wrong step.
//
// This package does not import internal/upgrade. The signature of Newest fits
// upgrade.KubernetesReleaseSource, and the host stays here.
package k8srelease

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	// DefaultBaseURL is where the stable lists are. It is a constant and no
	// setting: there is no flag, no environment variable and no configuration
	// field for it.
	DefaultBaseURL = "https://dl.k8s.io/release"

	// Budget is the one deadline over all the requests of one Newest call. They
	// run at once, so it bounds the call and not each request.
	//
	// Ten seconds against five measured answers of 0.06 to 0.24 seconds. It is
	// the bound of a fast read of a node (talos.FastReadDeadline), a multiple of
	// the slowest observation, and a small CDN file that takes longer than this
	// is a source that is not answering.
	Budget = 10 * time.Second

	// maxBodyBytes caps what is read of an answer. The measured answer is 7
	// bytes; the longest legitimate one is a few more.
	maxBodyBytes = 64

	// maxRedirects is the most redirects one request follows, all within the
	// host and the scheme it started with.
	maxRedirects = 3
)

// Client reads the stable lists of one base address.
type Client struct {
	base *url.URL
	http *http.Client
}

// Option changes a Client at construction.
type Option func(*Client)

// WithHTTPClient has the client use the transport of h. The client keeps its
// own copy: its timeout is cleared, because the deadline comes from the
// context, and its redirect rule is replaced by the one of this package.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		cp := *h
		cp.Timeout = 0
		cp.CheckRedirect = refuseForeignRedirect
		c.http = &cp
	}
}

// New builds a client for a base address such as DefaultBaseURL.
func New(base string, opts ...Option) (*Client, error) {
	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("k8srelease: parse base URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("k8srelease: base URL %q must be http or https, got scheme %q", base, u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("k8srelease: base URL %q has no host", base)
	}
	c := &Client{
		base: u,
		// No timeout of its own: the deadline is the context's, set by Newest.
		http: &http.Client{CheckRedirect: refuseForeignRedirect},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// refuseForeignRedirect follows a redirect only inside the host and the scheme
// the request started with, and for at most maxRedirects hops. A source that can
// bounce this client to another host decides where the next request goes, and
// one that can bounce it to plain http takes the certificate check away.
//
// It is the rule of internal/imagefactory, written again here: there it is not
// exported, and moving it would mean touching the guards of that package.
// The error names no address: the address comes from the answer.
func refuseForeignRedirect(req *http.Request, via []*http.Request) error {
	if len(via) == 0 {
		return nil
	}
	origin := via[0].URL
	if req.URL.Host != origin.Host {
		return errors.New("refusing a redirect to another host")
	}
	if req.URL.Scheme != origin.Scheme {
		return errors.New("refusing a redirect that changes the scheme")
	}
	if len(via) > maxRedirects {
		return fmt.Errorf("refusing to follow more than %d redirects", maxRedirects)
	}
	return nil
}

// Newest names, for each of the given minors, the newest stable patch of it as
// a version such as "v1.35.9". All minors are asked at once, under one deadline
// of Budget (or the caller's, when that is sooner). A list without minors is
// answered without a request.
//
// If one minor fails, the call fails, and the error names the file of that
// minor. No error repeats a byte of what the source answered.
func (c *Client) Newest(ctx context.Context, minors []int) (map[int]string, error) {
	distinct := make([]int, 0, len(minors))
	seen := make(map[int]bool, len(minors))
	for _, m := range minors {
		if m < 0 {
			return nil, fmt.Errorf("k8srelease: %d is not a Kubernetes minor", m)
		}
		if !seen[m] {
			seen[m] = true
			distinct = append(distinct, m)
		}
	}
	sort.Ints(distinct)

	out := make(map[int]string, len(distinct))
	if len(distinct) == 0 {
		return out, nil
	}

	ctx, cancel := context.WithTimeout(ctx, Budget)
	defer cancel()

	var (
		mu sync.Mutex
		g  errgroup.Group
	)
	for _, minor := range distinct {
		g.Go(func() error {
			version, err := c.one(ctx, minor)
			if err != nil {
				return err
			}
			mu.Lock()
			out[minor] = version
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return out, nil
}

// one reads the stable list of one minor and holds the answer to the form
// v1.<minor>.<patch>.
func (c *Client) one(ctx context.Context, minor int) (string, error) {
	file := "stable-1." + strconv.Itoa(minor) + ".txt"

	u := *c.base
	u.Path = strings.TrimRight(u.Path, "/") + "/" + file
	u.RawPath, u.RawQuery, u.Fragment = "", "", ""

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("k8srelease: %s: %w", file, err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return "", fmt.Errorf("k8srelease: %s: %w", file, err)
	}
	defer func() { _ = resp.Body.Close() }()

	// The status counts on its own: the body of a 404 is not even read, and a
	// 404 that happens to carry something plausible is still a 404.
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("k8srelease: %s: the source answered %d", file, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return "", fmt.Errorf("k8srelease: %s: reading the answer: %w", file, err)
	}
	if len(body) > maxBodyBytes {
		return "", fmt.Errorf("k8srelease: %s: answer too long (more than %d bytes)", file, maxBodyBytes)
	}

	line := strings.TrimSpace(string(body))
	if !stableOf(minor).MatchString(line) {
		return "", fmt.Errorf("k8srelease: %s: the answer is not a stable v1.%d release", file, minor)
	}
	return line, nil
}

// stableOf is the one shape an answer may have: v1.<minor>.<patch>, digits only,
// with the minor that was asked. A pre-release (a dash and a suffix), another
// minor, a second line and everything else do not match.
func stableOf(minor int) *regexp.Regexp {
	return regexp.MustCompile(`^v1\.` + strconv.Itoa(minor) + `\.[0-9]+$`)
}
