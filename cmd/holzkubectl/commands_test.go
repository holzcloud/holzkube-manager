package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// instance points the environment at a server that answers h.
//
// Environment and not a constructor, because that is the only way an operator
// configures this tool and a test that reached past it would be exercising a
// path nobody has.
func instance(t *testing.T, h http.Handler) {
	t.Helper()
	srv := serverWithItsOwnCertificate(t, h)
	t.Setenv("HOLZKUBE_URL", srv.URL)
	t.Setenv("HOLZKUBE_TOKEN", "hkm_test")
	t.Setenv("HOLZKUBE_FINGERPRINT", fingerprintOfServer(t, srv))
	t.Setenv("HOLZKUBE_TIMEOUT", "5s")
}

// capture runs the tool and returns what an operator would have seen.
func capture(t *testing.T, args ...string) (string, error) {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	saved := os.Stdout
	os.Stdout = w

	runErr := run(context.Background(), args)

	os.Stdout = saved
	_ = w.Close()
	out, readErr := io.ReadAll(r)
	_ = r.Close()
	if readErr != nil {
		t.Fatalf("reading what the tool printed: %v", readErr)
	}
	return string(out), runErr
}

func answer(body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})
}

// TestTheNodeTableShowsWhatTheServerSaid, including the two things a plain
// value cannot carry: a reading that is not available is a reason rather than a
// blank, and a locked node says so next to its stage because that is the answer
// to "why did the upgrade walk past this one".
func TestTheNodeTableShowsWhatTheServerSaid(t *testing.T) {
	instance(t, answer(`{"machines":[
		{"id":"11111111-2222-3333-4444-555555555555","cluster":"prod","role":"controlplane",
		 "stage":"running","locked":true,"labels":{"rack":"b","tier":"gold"},
		 "hostname":{"value":"cp-1","available":true},
		 "addr":{"value":"","available":false,"unavailable_reason":"the node has not answered since 09:14"},
		 "talos_version":{"value":"v1.13.9","available":true}}
	]}`))

	out, err := capture(t, "nodes")
	if err != nil {
		t.Fatalf("nodes: %v", err)
	}

	for _, want := range []string{
		"cp-1",
		"running (locked)",
		"controlplane",
		"the node has not answered since 09:14",
		"v1.13.9",
		"prod",
		"rack=b,tier=gold",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the node table does not carry %q:\n%s", want, out)
		}
	}
}

// TestTheClusterTableCountsTheNodesTheServerCounted.
//
// The count is the server's, under the name the server gives it. A CLI decoding
// a key the API does not have prints a zero and says nothing about it, which is
// the quietest way for a client to be wrong -- the column is there, it is
// filled in, and it is a lie.
func TestTheClusterTableCountsTheNodesTheServerCounted(t *testing.T) {
	instance(t, answer(`{"clusters":[
		{"id":"prod","name":"production","origin":"adopted","endpoint":"https://10.0.0.1:6443",
		 "locked":true,"nodes":5,"control_plane":3,"workers":2}
	]}`))

	out, err := capture(t, "clusters")
	if err != nil {
		t.Fatalf("clusters: %v", err)
	}

	for _, want := range []string{"prod", "production", "adopted", "https://10.0.0.1:6443", "5", "yes"} {
		if !strings.Contains(out, want) {
			t.Errorf("the cluster table does not carry %q:\n%s", want, out)
		}
	}
}

// TestTheJobTableCountsTheStepsTheServerCallsDone, and the state it calls them
// is "done". The same quiet wrongness as the count above: a tool matching a
// state string the model does not use reports 0/3 for a finished job.
func TestTheJobTableCountsTheStepsTheServerCallsDone(t *testing.T) {
	instance(t, answer(`{"jobs":[
		{"id":"job-1","kind":"upgrade","state":"running","steps":[
			{"name":"drain","state":"done"},
			{"name":"upgrade","state":"done"},
			{"name":"uncordon","state":"running"}
		]}
	]}`))

	out, err := capture(t, "jobs")
	if err != nil {
		t.Fatalf("jobs: %v", err)
	}
	if !strings.Contains(out, "2/3") {
		t.Errorf("two of three steps are done and the table does not say so:\n%s", out)
	}
}

// TestATemplateIsSentAsTheYAMLItIs.
//
// The plan route takes a YAML document as the body, which is why the browser
// posts application/yaml. A client that labelled the same bytes as JSON would
// be relying on the server not looking -- and the day something in front of it
// does look, the failure lands on the operator rather than here.
func TestATemplateIsSentAsTheYAMLItIs(t *testing.T) {
	var gotType, gotBody string
	instance(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotType = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		_, _ = w.Write([]byte(`{"plan":{"sentence":"ok","problems":[],"notes":[]},"notice":""}`))
	}))

	doc := "kind: ClusterTemplate\nname: homelab\n"
	path := filepath.Join(t.TempDir(), "cluster.yaml")
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := capture(t, "template", "plan", path); err != nil {
		t.Fatalf("template plan: %v", err)
	}

	if gotType != "application/yaml" {
		t.Errorf("the template was posted as %q", gotType)
	}
	if gotBody != doc {
		t.Errorf("the document was not sent as written: %q", gotBody)
	}
}

// TestAPlanWithProblemsExitsNonZeroAndStillPrintsThem. Both halves matter: the
// exit code so a pipeline can branch without parsing, and the printing because
// an exit code that swallowed the reasons would make this worse than silence.
func TestAPlanWithProblemsExitsNonZeroAndStillPrintsThem(t *testing.T) {
	instance(t, answer(`{"plan":{
		"sentence":"homelab: 1 control plane node and 2 workers.",
		"problems":["the class rack-b names 1 machine and the control plane asks for 3"],
		"notes":["2 machines already belong to another cluster"],
		"control_plane":{"machines":["cp-1"]},
		"workers":{"machines":["w-1","w-2"]}
	},"notice":"Nothing here applies a template."}`))

	path := filepath.Join(t.TempDir(), "cluster.yaml")
	if err := os.WriteFile(path, []byte("kind: ClusterTemplate\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, "template", "plan", path)
	if err == nil {
		t.Error("a plan with a problem in it exited zero")
	}

	for _, want := range []string{
		"homelab: 1 control plane node and 2 workers.",
		"the class rack-b names 1 machine and the control plane asks for 3",
		"2 machines already belong to another cluster",
		"cp-1",
		"w-1 w-2",
		"Nothing here applies a template.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the plan was not printed: missing %q\n%s", want, out)
		}
	}
}

// TestJSONIsTheServersBytesUnchanged. --json exists so a script never depends
// on this tool's formatting; re-encoding would put this tool's opinions about
// key order in front of the server's.
func TestJSONIsTheServersBytesUnchanged(t *testing.T) {
	const body = `{"clusters":[{"id":"prod","nodes":5,"name":"production"}]}`
	instance(t, answer(body))

	out, err := capture(t, "clusters", "--json")
	if err != nil {
		t.Fatalf("clusters --json: %v", err)
	}
	if strings.TrimRight(out, "\n") != body {
		t.Errorf("the bytes were rewritten on the way through:\n got %s\nwant %s", out, body)
	}
}

// TestLabelRefusesAWordThatIsNotAPair, before sending anything. A bare word is
// a typo, and the alternative -- sending it as a key with an empty value --
// would put a label on a machine that the operator did not write.
func TestLabelRefusesAWordThatIsNotAPair(t *testing.T) {
	instance(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("a request was sent for an argument that is not a label")
	}))

	_, err := capture(t, "label", "machine-1", "rack=b", "tier")
	if err == nil {
		t.Fatal(`"tier" was accepted as a label`)
	}
	if !strings.Contains(err.Error(), "key=value") {
		t.Errorf("the refusal does not say what the shape is: %v", err)
	}
}

// TestLabelWithNoPairsClearsThem, and sends an empty object rather than null.
// The route replaces, so the empty case is the only way to remove a label, and
// a null there is a different request: "I am not saying".
func TestLabelWithNoPairsClearsThem(t *testing.T) {
	var got string
	instance(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		got = string(raw)
		_, _ = w.Write([]byte(`{"id":"machine-1"}`))
	}))

	out, err := capture(t, "label", "machine-1")
	if err != nil {
		t.Fatalf("label: %v", err)
	}
	if got != `{"labels":{}}` {
		t.Errorf("clearing the labels sent %s", got)
	}
	if !strings.Contains(out, "no labels") {
		t.Errorf("the tool did not say what it did:\n%s", out)
	}
}

// TestAConfigFileIsWrittenByteForByte. A kubeconfig this tool reformatted is a
// kubeconfig with a different hash than the one the server signed.
func TestAConfigFileIsWrittenByteForByte(t *testing.T) {
	const kubeconfig = "apiVersion: v1\nkind: Config\nclusters: []\n"
	instance(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write([]byte(kubeconfig))
	}))

	out, err := capture(t, "kubeconfig", "prod")
	if err != nil {
		t.Fatalf("kubeconfig: %v", err)
	}
	if out != kubeconfig {
		t.Errorf("the credentials were rewritten on the way through: %q", out)
	}
}

// TestTheRouteEachVerbAsksFor. The paths are the API's, and a verb pointing at
// a path the server does not serve is a 404 the operator has to decode.
func TestTheRouteEachVerbAsksFor(t *testing.T) {
	for _, tc := range []struct {
		args   []string
		method string
		path   string
	}{
		{[]string{"nodes"}, http.MethodGet, "/api/v1/machines"},
		{[]string{"clusters"}, http.MethodGet, "/api/v1/clusters"},
		{[]string{"jobs"}, http.MethodGet, "/api/v1/jobs"},
		{[]string{"classes"}, http.MethodGet, "/api/v1/machine-classes"},
		{[]string{"scale", "prod"}, http.MethodGet, "/api/v1/clusters/prod/scale"},
		{[]string{"label", "m1", "a=b"}, http.MethodPut, "/api/v1/machines/m1/labels"},
		{[]string{"template", "export", "prod"}, http.MethodGet, "/api/v1/clusters/prod/template"},
		{[]string{"kubeconfig", "prod"}, http.MethodGet, "/api/v1/clusters/prod/kubeconfig"},
		{[]string{"talosconfig", "prod"}, http.MethodGet, "/api/v1/clusters/prod/talosconfig"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			var method, path string
			instance(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				method, path = r.Method, r.URL.Path
				_, _ = w.Write([]byte(`{}`))
			}))

			if _, err := capture(t, tc.args...); err != nil {
				t.Fatalf("%v: %v", tc.args, err)
			}
			if method != tc.method || path != tc.path {
				t.Errorf("asked for %s %s, want %s %s", method, path, tc.method, tc.path)
			}
		})
	}
}

// TestAScaleRefusalIsPrintedWholeAndNotAsAColumn.
//
// The refusal is the only thing on this screen that says what to do instead,
// and it is a sentence rather than a word. A table cell that showed "no" and
// dropped it would be a tool refusing without a reason.
func TestAScaleRefusalIsPrintedWholeAndNotAsAColumn(t *testing.T) {
	const refusal = "cp-1 is the only etcd member. Removing it does not make the cluster " +
		"smaller, it ends it. To take this node out of service, reset it"

	instance(t, answer(`{"plan":{
		"name":"homelab","control_plane":1,"workers":1,
		"members_known":true,"voting":1,"tolerates":0,
		"removals":[
			{"name":"cp-1","role":"controlplane","allowed":false,"reason":"`+refusal+`"},
			{"name":"w-1","role":"worker","allowed":true}
		],
		"additions":[{"name":"new-1","ready":false,"reason":"This machine is down."}],
		"advice":["One voting member. Add two."],
		"sentence":"homelab: 1 control-plane node(s) and 1 worker(s)."
	},"notice":"Nothing here changes the cluster."}`))

	out, err := capture(t, "scale", "prod")
	if err != nil {
		t.Fatalf("scale: %v", err)
	}

	for _, want := range []string{
		"homelab: 1 control-plane node(s) and 1 worker(s).",
		"One voting member. Add two.",
		refusal,
		"could not join: new-1",
		"This machine is down.",
		"Nothing here changes the cluster.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the scale output does not carry %q:\n%s", want, out)
		}
	}
}

// TestVersionNeedsNoServer. `holzkubectl version` is what somebody runs to find
// out whether the tool is installed, and requiring a configured instance to
// answer it would make the first thing anybody types fail.
func TestVersionNeedsNoServer(t *testing.T) {
	t.Setenv("HOLZKUBE_URL", "")
	t.Setenv("HOLZKUBE_TOKEN", "")

	out, err := capture(t, "version")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if strings.TrimSpace(out) != version {
		t.Errorf("version printed %q", out)
	}
}

// TestAVerbThisToolDoesNotHaveAsksForTheUsage, which main turns into exit 2 --
// what a shell expects from a command invoked wrongly, and distinct from the 1
// that means the server refused something.
func TestAVerbThisToolDoesNotHaveAsksForTheUsage(t *testing.T) {
	instance(t, answer(`{}`))

	for _, args := range [][]string{
		{},
		{"nodez"},
		{"label"},
		{"template"},
		{"template", "aplly", "x"},
		{"scale"},
		{"scale", "a", "b"},
		{"kubeconfig"},
		{"kubeconfig", "a", "b"},
	} {
		if _, err := capture(t, args...); !errors.Is(err, errUsage) {
			t.Errorf("%v: %v, want the usage", args, err)
		}
	}
}

// TestAnUnavailableFieldReadsAsItsReason. The Field[T] read model exists so a
// missing reading is never confused with a reading of nothing, and a table
// printing a blank would throw that away at the last step.
func TestAnUnavailableFieldReadsAsItsReason(t *testing.T) {
	for _, tc := range []struct {
		name string
		f    field
		want string
	}{
		{"a value", field{Value: "cp-1", Available: true}, "cp-1"},
		{"a reason", field{Available: false, Reason: "the node has not answered"}, "(the node has not answered)"},
		{"no reason given", field{Available: false}, "(unavailable)"},
		{"available and empty", field{Available: true}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.f.String(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
