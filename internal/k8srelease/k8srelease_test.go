package k8srelease_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"sync"
	"testing"

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
