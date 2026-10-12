package k8srelease_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/k8srelease"
)

// recordingSource is a stand-in for dl.k8s.io. It serves the stable lists and,
// as the trap, a candidate list that answers with a release candidate. It
// writes down every path it was asked.
func recordingSource(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var (
		mu    sync.Mutex
		paths []string
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/release/stable-1.33.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "v1.33.7")
	})
	mux.HandleFunc("/release/stable-1.34.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "v1.34.2\n")
	})
	mux.HandleFunc("/release/latest-1.33.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "v1.33.0-rc.1")
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := append([]string(nil), paths...)
		sort.Strings(out)
		return out
	}
}

func TestNewestAsksTheStableListAndReturnsThePatchOfTheMinor(t *testing.T) {
	t.Parallel()

	srv, asked := recordingSource(t)
	client, err := k8srelease.New(srv.URL + "/release")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	got, err := client.Newest(t.Context(), []int{33, 34})
	if err != nil {
		t.Fatalf("Newest: %v", err)
	}

	want := map[int]string{33: "v1.33.7", 34: "v1.34.2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Newest = %v, want %v", got, want)
	}
	wantPaths := []string{"/release/stable-1.33.txt", "/release/stable-1.34.txt"}
	if !reflect.DeepEqual(asked(), wantPaths) {
		t.Errorf("the source was asked %v, want exactly %v (never the list of newest candidates)", asked(), wantPaths)
	}
}

// answer serves one fixed status and body on every path and counts the requests.
func answer(t *testing.T, status int, body string) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var n atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

func newClient(t *testing.T, base string, opts ...k8srelease.Option) *k8srelease.Client {
	t.Helper()
	c, err := k8srelease.New(base, opts...)
	if err != nil {
		t.Fatalf("New(%q): %v", base, err)
	}
	return c
}

func TestNewestRefusesWhatIsNotAStableReleaseOfTheMinor(t *testing.T) {
	t.Parallel()

	const xml = `<?xml version='1.0' encoding='UTF-8'?><Error><Code>NoSuchKey</Code><Message>The specified key does not exist.</Message></Error>`

	tests := []struct {
		name     string
		status   int
		body     string
		want     string // the version, or empty when an error is expected
		errMatch string // a word the error must carry besides the file
	}{
		{"one line", 200, "v1.35.9", "v1.35.9", ""},
		{"one line with a line ending", 200, "v1.35.9\n", "v1.35.9", ""},
		{"not found with an XML body", 404, xml, "", "404"},
		{"not found with a plausible body", 404, "v1.35.9", "", "404"},
		{"server error", 500, "v1.35.9", "", "500"},
		{"another minor", 200, "v1.36.0", "", "not a stable"},
		{"a pre-release", 200, "v1.35.0-rc.1", "", "not a stable"},
		{"two lines", 200, "v1.35.9\nv1.35.8\n", "", "not a stable"},
		{"empty", 200, "", "", "not a stable"},
		{"no patch number", 200, "v1.35", "", "not a stable"},
		{"too long", 200, "v1.35." + strings.Repeat("1", 80), "", "too long"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, _ := answer(t, tc.status, tc.body)
			got, err := newClient(t, srv.URL+"/release").Newest(t.Context(), []int{35})

			if tc.want != "" {
				if err != nil {
					t.Fatalf("Newest: %v", err)
				}
				if got[35] != tc.want {
					t.Errorf("Newest = %v, want 35 -> %s", got, tc.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("Newest accepted status %d with body %q: %v", tc.status, tc.body, got)
			}
			if got != nil {
				t.Errorf("a failed call returned %v beside its error", got)
			}
			for _, want := range []string{"stable-1.35.txt", tc.errMatch} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the error %q does not mention %q", err, want)
				}
			}
		})
	}
}

func TestNewestRefusesAServerThatDropsTheConnection(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		_ = conn.Close()
	}))
	t.Cleanup(srv.Close)

	if _, err := newClient(t, srv.URL+"/release").Newest(t.Context(), []int{35}); err == nil {
		t.Fatal("Newest took a dropped connection for an answer")
	}
}

func TestNoErrorRepeatsWhatTheSourceAnswered(t *testing.T) {
	t.Parallel()

	const marker = "BODY-MARKER-7f3a"
	for name, tc := range map[string]struct {
		status int
		body   string
	}{
		"200 and no version":      {200, marker},
		"404":                     {404, marker},
		"500":                     {500, marker},
		"a version that is a lie": {200, "v1.35.9 " + marker},
		"too long":                {200, marker + strings.Repeat("x", 100)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			srv, _ := answer(t, tc.status, tc.body)
			_, err := newClient(t, srv.URL+"/release").Newest(t.Context(), []int{35})
			if err == nil {
				t.Fatal("Newest accepted the answer")
			}
			if strings.Contains(err.Error(), marker) {
				t.Errorf("the error repeats the answer: %v", err)
			}
		})
	}
}

func TestNewestAsksEveryMinorAtOnce(t *testing.T) {
	t.Parallel()

	var (
		arrived atomic.Int64
		all     = make(chan struct{})
		once    sync.Once
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if arrived.Add(1) == 3 {
			once.Do(func() { close(all) })
		}
		// A client that asks one after the other never gets the third request
		// out, and ends here, after the wait, as an error and not as a hang.
		select {
		case <-all:
		case <-time.After(3 * time.Second):
			http.Error(w, "only one request at a time arrived", http.StatusServiceUnavailable)
			return
		}
		name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/release/stable-"), ".txt")
		_, _ = fmt.Fprintf(w, "v%s.1", name)
	}))
	t.Cleanup(srv.Close)

	got, err := newClient(t, srv.URL+"/release").Newest(t.Context(), []int{33, 34, 35})
	if err != nil {
		t.Fatalf("Newest: %v", err)
	}
	want := map[int]string{33: "v1.33.1", 34: "v1.34.1", 35: "v1.35.1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Newest = %v, want %v", got, want)
	}
}

func TestNewestFailsWhenOneOfTheMinorsFailsAndSaysWhich(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/release/stable-1.34.txt" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprint(w, "v1.33.7")
	}))
	t.Cleanup(srv.Close)

	got, err := newClient(t, srv.URL+"/release").Newest(t.Context(), []int{33, 34})
	if err == nil {
		t.Fatalf("Newest answered %v although 1.34 has no list", got)
	}
	if !strings.Contains(err.Error(), "stable-1.34.txt") {
		t.Errorf("the error %q does not name the minor that failed", err)
	}
	if strings.Contains(err.Error(), "stable-1.33.txt") {
		t.Errorf("the error %q names a minor that did not fail", err)
	}
}

// deadlineTransport writes down the deadline of every request and answers at
// once with an error, so a missing deadline is a failed assertion and never a
// wait.
type deadlineTransport struct {
	mu        sync.Mutex
	deadlines []time.Time
	missing   int
}

func (d *deadlineTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if dl, ok := req.Context().Deadline(); ok {
		d.deadlines = append(d.deadlines, dl)
	} else {
		d.missing++
	}
	return nil, errors.New("no network in this test")
}

func TestNewestAppliesTheBudgetAsADeadline(t *testing.T) {
	t.Parallel()

	t.Run("a caller without a deadline gets the budget", func(t *testing.T) {
		t.Parallel()
		tr := &deadlineTransport{}
		c := newClient(t, "https://dl.example.test/release", k8srelease.WithHTTPClient(&http.Client{Transport: tr}))

		_, _ = c.Newest(t.Context(), []int{33, 34})

		if tr.missing != 0 || len(tr.deadlines) != 2 {
			t.Fatalf("%d requests carried no deadline, %d did (want 0 and 2)", tr.missing, len(tr.deadlines))
		}
		for _, dl := range tr.deadlines {
			left := time.Until(dl)
			if left > k8srelease.Budget || left < k8srelease.Budget-5*time.Second {
				t.Errorf("a request has %v left, want about the budget of %v", left, k8srelease.Budget)
			}
		}
	})

	t.Run("a caller with a shorter deadline keeps it", func(t *testing.T) {
		t.Parallel()
		tr := &deadlineTransport{}
		c := newClient(t, "https://dl.example.test/release", k8srelease.WithHTTPClient(&http.Client{Transport: tr}))
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()

		_, _ = c.Newest(ctx, []int{33})

		if len(tr.deadlines) != 1 {
			t.Fatalf("%d requests carried a deadline, want 1", len(tr.deadlines))
		}
		if left := time.Until(tr.deadlines[0]); left > time.Second {
			t.Errorf("a request has %v left, want at most the caller's second", left)
		}
	})
}

func TestNewestFollowsARedirectInsideTheHost(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/release/stable-1.35.txt" {
			http.Redirect(w, r, "/release/final-1.35.txt", http.StatusFound)
			return
		}
		_, _ = fmt.Fprint(w, "v1.35.9")
	}))
	t.Cleanup(srv.Close)

	got, err := newClient(t, srv.URL+"/release").Newest(t.Context(), []int{35})
	if err != nil {
		t.Fatalf("Newest: %v", err)
	}
	if got[35] != "v1.35.9" {
		t.Errorf("Newest = %v, want 35 -> v1.35.9", got)
	}
}

func TestNewestRefusesARedirectToAnotherHost(t *testing.T) {
	t.Parallel()

	elsewhere, hits := answer(t, 200, "v1.35.9")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+"/release/stable-1.35.txt", http.StatusFound)
	}))
	t.Cleanup(srv.Close)

	got, err := newClient(t, srv.URL+"/release").Newest(t.Context(), []int{35})
	if err == nil {
		t.Fatalf("Newest followed a redirect to another host and answered %v", got)
	}
	if hits.Load() != 0 {
		t.Errorf("the other host was asked %d times", hits.Load())
	}
	if strings.Contains(err.Error(), elsewhere.URL) || strings.Contains(err.Error(), url.PathEscape(elsewhere.URL)) {
		t.Errorf("the error repeats the address the source sent us to: %v", err)
	}
}

func TestNewestRefusesALongChainOfRedirects(t *testing.T) {
	t.Parallel()

	// A chain of n redirects ends in the answer after n hops.
	chain := func(n int) *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var hop int
			if _, err := fmt.Sscanf(r.URL.Path, "/release/hop-%d", &hop); err != nil {
				hop = 0
			}
			if hop < n {
				http.Redirect(w, r, fmt.Sprintf("/release/hop-%d", hop+1), http.StatusFound)
				return
			}
			_, _ = fmt.Fprint(w, "v1.35.9")
		}))
		t.Cleanup(srv.Close)
		return srv
	}

	if _, err := newClient(t, chain(3).URL+"/release").Newest(t.Context(), []int{35}); err != nil {
		t.Errorf("three redirects were refused: %v", err)
	}
	if got, err := newClient(t, chain(4).URL+"/release").Newest(t.Context(), []int{35}); err == nil {
		t.Errorf("four redirects were followed and answered %v", got)
	}
}

// redirectingTransport answers every request with a redirect to the location
// it was given and counts the requests.
type redirectingTransport struct {
	location string
	n        atomic.Int64
}

func (r *redirectingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.n.Add(1)
	return &http.Response{
		StatusCode: http.StatusFound,
		Header:     http.Header{"Location": {r.location}},
		Body:       http.NoBody,
		Request:    req,
	}, nil
}

func TestNewestRefusesARedirectFromHttpsToHttp(t *testing.T) {
	t.Parallel()

	tr := &redirectingTransport{location: "http://dl.example.test/release/stable-1.35.txt"}
	c := newClient(t, "https://dl.example.test/release", k8srelease.WithHTTPClient(&http.Client{Transport: tr}))

	got, err := c.Newest(t.Context(), []int{35})
	if err == nil {
		t.Fatalf("Newest followed a redirect from https to http and answered %v", got)
	}
	if tr.n.Load() != 1 {
		t.Errorf("the transport saw %d requests, want 1 (the redirect must not be followed)", tr.n.Load())
	}
}

func TestNewestWithNoMinorsAsksNothing(t *testing.T) {
	t.Parallel()

	srv, hits := answer(t, 200, "v1.35.9")
	for _, minors := range [][]int{nil, {}} {
		got, err := newClient(t, srv.URL+"/release").Newest(t.Context(), minors)
		if err != nil {
			t.Fatalf("Newest(%v): %v", minors, err)
		}
		if got == nil || len(got) != 0 {
			t.Errorf("Newest(%v) = %#v, want an empty map", minors, got)
		}
	}
	if hits.Load() != 0 {
		t.Errorf("the source was asked %d times for no minor", hits.Load())
	}
}

func TestNewestAsksOnceForAMinorNamedTwice(t *testing.T) {
	t.Parallel()

	srv, hits := answer(t, 200, "v1.35.9")
	got, err := newClient(t, srv.URL+"/release").Newest(t.Context(), []int{35, 35})
	if err != nil {
		t.Fatalf("Newest: %v", err)
	}
	if got[35] != "v1.35.9" || len(got) != 1 {
		t.Errorf("Newest = %v, want only 35 -> v1.35.9", got)
	}
	if hits.Load() != 1 {
		t.Errorf("the source was asked %d times for one minor", hits.Load())
	}
}

func TestNewestRefusesAMinorThatCannotBeOne(t *testing.T) {
	t.Parallel()

	srv, hits := answer(t, 200, "v1.35.9")
	if got, err := newClient(t, srv.URL+"/release").Newest(t.Context(), []int{35, -1}); err == nil {
		t.Errorf("Newest answered %v for a negative minor", got)
	}
	if hits.Load() != 0 {
		t.Errorf("the source was asked %d times before the minors were checked", hits.Load())
	}
}

func TestTheDefaultSourceIsTheFixedHttpsHost(t *testing.T) {
	t.Parallel()

	u, err := url.Parse(k8srelease.DefaultBaseURL)
	if err != nil {
		t.Fatalf("DefaultBaseURL does not parse: %v", err)
	}
	if u.Scheme != "https" || u.Host != "dl.k8s.io" {
		t.Errorf("DefaultBaseURL is %q, want https on dl.k8s.io", k8srelease.DefaultBaseURL)
	}
	if _, err := k8srelease.New(k8srelease.DefaultBaseURL); err != nil {
		t.Errorf("New(DefaultBaseURL): %v", err)
	}
	for _, bad := range []string{"", "ftp://x", "http://", "dl.k8s.io/release"} {
		if _, err := k8srelease.New(bad); err == nil {
			t.Errorf("New(%q) was accepted", bad)
		}
	}
}
