package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
)

// errUsage asks main to print the usage and exit 2, which is what a shell
// expects from a command invoked wrongly.
var errUsage = errors.New("usage")

const usage = `holzkubectl — a command-line client for a holzkube-manager instance

  holzkubectl nodes                 every machine, across every cluster
  holzkubectl clusters              the clusters this instance manages
  holzkubectl jobs                  long-running operations and where they are
  holzkubectl classes               the machine classes and what they name now
  holzkubectl scale <cluster>       which of a cluster's nodes may be removed
  holzkubectl renew-certificate <cluster>
                                    issue this installation a fresh admin
                                    certificate for the cluster
  holzkubectl rotate-authority <cluster>
                                    print what rotating the cluster's Talos
                                    certificate authority would do; add
                                    --confirm <cluster-name> to do it
  holzkubectl label <id> k=v ...    replace a machine's labels (none clears them)
  holzkubectl template plan <file>  what a cluster template would mean
  holzkubectl template export <id>  write a cluster down as a template
  holzkubectl kubeconfig <cluster>  admin credentials for the cluster's Kubernetes
  holzkubectl talosconfig <cluster> an admin talosconfig
  holzkubectl version               this tool's version

Configuration is environment variables only:

  HOLZKUBE_URL          https://holzkube.example:8443
  HOLZKUBE_TOKEN        a service-account token (Settings, Accounts — shown once)
  HOLZKUBE_FINGERPRINT  the server's TLS certificate, SHA-256 hex. Optional, and
                        the way to reach an instance using its own generated
                        certificate. There is deliberately no flag that skips
                        verification: this product pins node certificates by
                        fingerprint rather than skipping the check, and a tool
                        that took the shortcut it denies its own transport would
                        be saying two things about one risk.
  HOLZKUBE_TIMEOUT      how long one request may take. Default 30s.

Add --json to any read to get the server's answer unchanged.
`

// run dispatches, and it is the whole of the control flow.
func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errUsage
	}

	// version needs no server, so it is answered before the client is built.
	if args[0] == "version" {
		fmt.Println(version)
		return nil
	}

	asJSON := false
	kept := args[:0:0]
	for _, arg := range args {
		if arg == "--json" {
			asJSON = true
			continue
		}
		kept = append(kept, arg)
	}
	args = kept
	if len(args) == 0 {
		return errUsage
	}

	client, err := NewClient(FromEnvironment())
	if err != nil {
		return err
	}

	switch args[0] {
	case "nodes":
		return listNodes(ctx, client, asJSON)
	case "clusters":
		return listClusters(ctx, client, asJSON)
	case "jobs":
		return listJobs(ctx, client, asJSON)
	case "classes":
		return listClasses(ctx, client, asJSON)
	case "scale":
		return clusterScale(ctx, client, args[1:], asJSON)
	case "renew-certificate":
		return renewCertificate(ctx, client, args[1:], asJSON)
	case "rotate-authority":
		return rotateAuthority(ctx, client, args[1:], asJSON)
	case "label":
		return setLabels(ctx, client, args[1:], asJSON)
	case "template":
		return template(ctx, client, args[1:], asJSON)
	case "kubeconfig":
		return configFile(ctx, client, args[1:], "kubeconfig")
	case "talosconfig":
		return configFile(ctx, client, args[1:], "talosconfig")
	default:
		return errUsage
	}
}

// printJSON writes the server's answer through unchanged.
//
// Unchanged is the point: --json exists so that a script never depends on this
// tool's formatting, and re-encoding would put this tool's opinions about key
// order and number formatting in front of the server's.
func printJSON(raw []byte) error {
	_, err := os.Stdout.Write(raw)
	if err == nil && len(raw) > 0 && raw[len(raw)-1] != '\n' {
		fmt.Println()
	}
	return err
}

func table() *tabwriter.Writer {
	return tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
}

// stepDone is the state the API gives a job step that finished successfully.
//
// It is written down here rather than imported so that this binary does not
// carry the server's packages, and there is a test in this package that checks
// it against model.StepDone -- because the first version of this file guessed
// "succeeded", which is not a state the server has, and the steps column read
// 0/3 for every finished job. A string that is merely never matched is the
// quietest way for a client to be wrong: the column is there, it is filled in,
// and it is a lie.
const stepDone = "done"

// The shapes this tool reads out of the API's answers.
//
// Named types rather than structs declared inside the functions that decode
// them, so that cmd/holzkubectl/wire_test.go can hold every json tag here
// against the server type it claims to be reading. That test exists because
// two of these were wrong on the first attempt -- a cluster's node count was
// read from a key called "machine_count", which the API does not have -- and
// nothing said so: encoding/json leaves a field it cannot find at its zero
// value, so the column printed 0 and looked like an answer.
type machineRow struct {
	ID      string            `json:"id"`
	Cluster string            `json:"cluster"`
	Role    string            `json:"role"`
	Stage   string            `json:"stage"`
	Locked  bool              `json:"locked"`
	Labels  map[string]string `json:"labels"`

	Hostname     field `json:"hostname"`
	Addr         field `json:"addr"`
	TalosVersion field `json:"talos_version"`
}

type clusterRow struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Origin   string `json:"origin"`
	Endpoint string `json:"endpoint"`
	Locked   bool   `json:"locked"`
	Nodes    int    `json:"nodes"`

	// CertNotAfter is when the certificate this installation dials the cluster
	// with expires. It is a string because this tool prints it and never does
	// arithmetic on it -- the days-left figure is the server's, computed
	// against the server's clock, which is the clock the warning uses.
	CertNotAfter string `json:"client_cert_not_after"`
}

type jobRow struct {
	ID    string       `json:"id"`
	Kind  string       `json:"kind"`
	State string       `json:"state"`
	Steps []jobStepRow `json:"steps"`
}

type jobStepRow struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

type classRow struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Sentence string `json:"sentence"`
	Count    int    `json:"count"`
}

type scaleRow struct {
	Name         string `json:"name"`
	ControlPlane int    `json:"control_plane"`
	Workers      int    `json:"workers"`

	MembersKnown bool `json:"members_known"`
	Voting       int  `json:"voting"`
	Tolerates    int  `json:"tolerates"`

	Removals  []scaleRemovalRow   `json:"removals"`
	Additions []scaleCandidateRow `json:"additions"`
	Advice    []string            `json:"advice"`
	Sentence  string              `json:"sentence"`
}

type scaleRemovalRow struct {
	Name    string `json:"name"`
	Role    string `json:"role"`
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
}

type scaleCandidateRow struct {
	Name   string `json:"name"`
	Ready  bool   `json:"ready"`
	Reason string `json:"reason"`
}

type planRow struct {
	Sentence     string     `json:"sentence"`
	Problems     []string   `json:"problems"`
	Notes        []string   `json:"notes"`
	ControlPlane nodeSetRow `json:"control_plane"`
	Workers      nodeSetRow `json:"workers"`
}

type nodeSetRow struct {
	Machines []string `json:"machines"`
}

// field is one of the API's Field[T] read models, of which this tool needs only
// the two parts a terminal can show.
type field struct {
	Value     any    `json:"value"`
	Available bool   `json:"available"`
	Reason    string `json:"unavailable_reason"`
}

// String renders a field the way the read model means it: a value that is
// not available is not blank, it is a reason.
func (f field) String() string {
	if !f.Available {
		if f.Reason != "" {
			return "(" + f.Reason + ")"
		}
		return "(unavailable)"
	}
	if f.Value == nil {
		return ""
	}
	return fmt.Sprint(f.Value)
}

func listNodes(ctx context.Context, c *Client, asJSON bool) error {
	var raw []byte
	if err := c.Do(ctx, request{Method: http.MethodGet, Path: "/api/v1/machines"}, &raw); err != nil {
		return err
	}
	if asJSON {
		return printJSON(raw)
	}

	var body struct {
		Machines []machineRow `json:"machines"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}

	w := table()
	fmt.Fprintln(w, "HOST\tSTATE\tROLE\tADDRESS\tTALOS\tCLUSTER\tLABELS")
	for _, m := range body.Machines {
		host := m.Hostname.String()
		if host == "" {
			host = m.ID
		}
		state := m.Stage
		if m.Locked {
			// Not a stage, and shown next to one: a locked node is skipped by
			// rolling operations whatever its health is, and an operator
			// wondering why an upgrade walked past it is looking at this line.
			state += " (locked)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			host, state, m.Role, m.Addr, m.TalosVersion, m.Cluster, labelsOf(m.Labels))
	}
	return w.Flush()
}

func labelsOf(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	parts := make([]string, 0, len(labels))
	for k, v := range labels {
		parts = append(parts, k+"="+v)
	}
	slices.Sort(parts)
	return strings.Join(parts, ",")
}

func listClusters(ctx context.Context, c *Client, asJSON bool) error {
	var raw []byte
	if err := c.Do(ctx, request{Method: http.MethodGet, Path: "/api/v1/clusters"}, &raw); err != nil {
		return err
	}
	if asJSON {
		return printJSON(raw)
	}

	var body struct {
		Clusters []clusterRow `json:"clusters"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}

	w := table()
	fmt.Fprintln(w, "ID\tNAME\tORIGIN\tENDPOINT\tNODES\tLOCKED")
	for _, cl := range body.Clusters {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\n",
			cl.ID, cl.Name, cl.Origin, cl.Endpoint, cl.Nodes, yesNo(cl.Locked))
	}
	return w.Flush()
}

func listJobs(ctx context.Context, c *Client, asJSON bool) error {
	var raw []byte
	if err := c.Do(ctx, request{Method: http.MethodGet, Path: "/api/v1/jobs"}, &raw); err != nil {
		return err
	}
	if asJSON {
		return printJSON(raw)
	}

	var body struct {
		Jobs []jobRow `json:"jobs"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}

	w := table()
	fmt.Fprintln(w, "ID\tKIND\tSTATE\tSTEPS")
	for _, j := range body.Jobs {
		done := 0
		for _, s := range j.Steps {
			if s.State == stepDone {
				done++
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%d/%d\n", j.ID, j.Kind, j.State, done, len(j.Steps))
	}
	return w.Flush()
}

func listClasses(ctx context.Context, c *Client, asJSON bool) error {
	var raw []byte
	if err := c.Do(ctx, request{Method: http.MethodGet, Path: "/api/v1/machine-classes"}, &raw); err != nil {
		return err
	}
	if asJSON {
		return printJSON(raw)
	}

	var body struct {
		Classes []classRow `json:"classes"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}

	w := table()
	fmt.Fprintln(w, "ID\tNAME\tNAMES NOW\tSELECTS")
	for _, cl := range body.Classes {
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", cl.ID, cl.Name, cl.Count, cl.Sentence)
	}
	return w.Flush()
}

// clusterScale prints what changing a cluster's size would mean.
//
// The refusals are printed whole, on their own lines, rather than squeezed into
// a column. They are the only thing here that says what to do instead, and a
// table that truncated one would be a tool refusing without a reason.
func clusterScale(ctx context.Context, c *Client, args []string, asJSON bool) error {
	if len(args) != 1 {
		return errUsage
	}

	var raw []byte
	if err := c.Do(ctx, request{
		Method: http.MethodGet,
		Path:   "/api/v1/clusters/" + args[0] + "/scale",
	}, &raw); err != nil {
		return err
	}
	if asJSON {
		return printJSON(raw)
	}

	var body struct {
		Plan   scaleRow `json:"plan"`
		Notice string   `json:"notice"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	p := body.Plan

	fmt.Println(p.Sentence)
	for _, line := range p.Advice {
		fmt.Printf("  %s\n", line)
	}

	fmt.Println()
	w := table()
	fmt.Fprintln(w, "NODE\tROLE\tMAY BE REMOVED")
	for _, r := range p.Removals {
		verdict := "yes"
		if !r.Allowed {
			verdict = "no"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", r.Name, r.Role, verdict)
	}
	if err := w.Flush(); err != nil {
		return err
	}

	for _, r := range p.Removals {
		if !r.Allowed {
			fmt.Printf("\n%s: %s\n", r.Name, r.Reason)
		}
	}

	for _, a := range p.Additions {
		if a.Ready {
			fmt.Printf("\ncould join: %s\n", a.Name)
		} else {
			fmt.Printf("\ncould not join: %s — %s\n", a.Name, a.Reason)
		}
	}

	if body.Notice != "" {
		fmt.Printf("\n%s\n", body.Notice)
	}
	return nil
}

// renewCertificate issues this installation a fresh admin certificate for one
// cluster.
//
// It is here because this is the operation somebody wants on a timer: the
// certificate expires once a year, and a cron entry that renews it in the
// quarter before is a better answer than a banner somebody has to be logged in
// to see. The server proves the new certificate against a node before keeping
// it, so a failed run means the old one is still in place -- which is what
// makes putting this in a cron entry defensible at all.
func renewCertificate(ctx context.Context, c *Client, args []string, asJSON bool) error {
	if len(args) != 1 {
		return errUsage
	}

	var raw []byte
	if err := c.Do(ctx, request{
		Method: http.MethodPost,
		Path:   "/api/v1/clusters/" + args[0] + "/client-certificate",

		// A POST with no body. The cluster is in the path and there is nothing
		// to choose: a renewal takes no options, because every option it could
		// take would be a way to mint a certificate that is not the one this
		// installation needs.
		ContentType: "application/json",
		Body:        []byte("{}"),
	}, &raw); err != nil {
		return err
	}
	if asJSON {
		return printJSON(raw)
	}

	var cluster clusterRow
	if err := json.Unmarshal(raw, &cluster); err != nil {
		return err
	}
	fmt.Printf("%s: renewed, expiring %s. No node was touched.\n",
		cluster.Name, cluster.CertNotAfter)
	return nil
}

// rotateAuthority rotates a cluster's Talos certificate authority.
//
// Two forms, and the split is the point. Without --confirm it PRINTS the plan:
// the four passes, the nodes that will be written, and the warnings -- the same
// text the screen shows, because they come from the same route. With --confirm
// <name> it types the cluster's name the way the dialog does and submits.
//
// There is no single-flag form that skips the reading. The one operation in
// this product that can leave a cluster trusting nobody is not one a shell
// history should be able to repeat by accident.
func rotateAuthority(ctx context.Context, c *Client, args []string, asJSON bool) error {
	if len(args) == 0 {
		return errUsage
	}
	id := args[0]

	typed := ""
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		if rest[i] != "--confirm" {
			return errUsage
		}
		if i+1 >= len(rest) {
			return errUsage
		}
		typed = rest[i+1]
		i++
	}

	var raw []byte
	if err := c.Do(ctx, request{
		Method: http.MethodGet,
		Path:   "/api/v1/clusters/" + id + "/authority",
	}, &raw); err != nil {
		return err
	}

	var plan authorityPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return err
	}

	if typed == "" {
		if asJSON {
			return printJSON(raw)
		}
		return printAuthorityPlan(plan)
	}

	// The phrase is checked by the server too. It is compared here first so
	// that a typo costs a line of output instead of a request that reads as a
	// refusal from the cluster.
	if typed != plan.ConfirmPhrase {
		return fmt.Errorf("%q is not this cluster's name, which is %q: rotating its certificate "+
			"authority needs the name typed exactly", typed, plan.ConfirmPhrase)
	}
	if plan.Locked {
		return fmt.Errorf("%s is adopted read-only, so nothing may change it. Unlock it first",
			plan.Name)
	}

	body, err := json.Marshal(map[string]string{"typed": typed})
	if err != nil {
		return err
	}
	var confirmRaw []byte
	if err := c.Do(ctx, request{
		Method:      http.MethodPost,
		Path:        "/api/v1/clusters/" + id + "/authority/confirm",
		ContentType: "application/json",
		Body:        body,
	}, &confirmRaw); err != nil {
		return err
	}
	var confirmation struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(confirmRaw, &confirmation); err != nil {
		return err
	}

	submit, err := json.Marshal(map[string]string{"confirmation": confirmation.Token})
	if err != nil {
		return err
	}
	var jobRaw []byte
	if err := c.Do(ctx, request{
		Method:      http.MethodPost,
		Path:        "/api/v1/clusters/" + id + "/authority",
		ContentType: "application/json",
		Body:        submit,
	}, &jobRaw); err != nil {
		return err
	}
	if asJSON {
		return printJSON(jobRaw)
	}

	var accepted struct {
		Job struct {
			ID    string `json:"id"`
			Steps []struct {
				Name string `json:"name"`
			} `json:"steps"`
		} `json:"job"`
	}
	if err := json.Unmarshal(jobRaw, &accepted); err != nil {
		return err
	}
	fmt.Printf("rotation started: job %s, %d steps. Watch it with: holzkubectl jobs\n",
		accepted.Job.ID, len(accepted.Job.Steps))
	return nil
}

// authorityPlan is the preview route's answer.
type authorityPlan struct {
	Name          string              `json:"name"`
	ConfirmPhrase string              `json:"confirm_phrase"`
	InProgress    bool                `json:"in_progress"`
	Locked        bool                `json:"locked"`
	Passes        []string            `json:"passes"`
	Warnings      []string            `json:"warnings"`
	Nodes         []authorityPlanNode `json:"nodes"`
}

// authorityPlanNode is one node in the plan. A named type rather than an
// anonymous struct because wire_test.go holds it against the server's.
type authorityPlanNode struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname"`
	Role     string `json:"role"`
}

func printAuthorityPlan(plan authorityPlan) error {
	fmt.Printf("%s: rotating the Talos certificate authority\n\n", plan.Name)
	if plan.InProgress {
		fmt.Println("A rotation is already in progress on this cluster. Submitting continues it.")
		fmt.Println()
	}
	if plan.Locked {
		fmt.Println("This cluster is adopted read-only. Unlock it before rotating.")
		fmt.Println()
	}

	fmt.Println("What happens, in order:")
	for i, pass := range plan.Passes {
		fmt.Printf("  %d. %s\n", i+1, pass)
	}

	fmt.Printf("\nNodes that will be written (%d), and every one of them has to answer:\n",
		len(plan.Nodes))
	for _, n := range plan.Nodes {
		name := n.Hostname
		if name == "" {
			name = "(no hostname recorded)"
		}
		fmt.Printf("  %-38s %-14s %s\n", n.ID, n.Role, name)
	}

	fmt.Println()
	for _, w := range plan.Warnings {
		fmt.Printf("! %s\n", w)
	}
	fmt.Printf("\nTo do it: holzkubectl rotate-authority <cluster> --confirm %q\n", plan.ConfirmPhrase)
	return nil
}

// setLabels replaces a machine's labels, which is what the route does.
//
// Replaces, and the usage says so: `holzkubectl label <id>` with no pairs
// clears them. A CLI that merged would need a second verb to remove one, and
// this tool would then be describing an operation the API does not have.
func setLabels(ctx context.Context, c *Client, args []string, asJSON bool) error {
	if len(args) == 0 {
		return errUsage
	}

	id, pairs := args[0], args[1:]
	labels := map[string]string{}
	for _, pair := range pairs {
		key, value, found := strings.Cut(pair, "=")
		if !found {
			return fmt.Errorf("%q is not a label: write them as key=value", pair)
		}
		labels[key] = value
	}

	body, err := json.Marshal(map[string]any{"labels": labels})
	if err != nil {
		return err
	}

	var raw []byte
	if err := c.Do(ctx, request{
		Method:      http.MethodPut,
		Path:        "/api/v1/machines/" + id + "/labels",
		ContentType: "application/json",
		Body:        body,
	}, &raw); err != nil {
		return err
	}
	if asJSON {
		return printJSON(raw)
	}

	if len(labels) == 0 {
		fmt.Printf("%s now has no labels.\n", id)
		return nil
	}
	fmt.Printf("%s: %s\n", id, labelsOf(labels))
	return nil
}

func template(ctx context.Context, c *Client, args []string, asJSON bool) error {
	if len(args) < 2 {
		return errUsage
	}

	switch args[0] {
	case "plan":
		doc, err := os.ReadFile(args[1]) //nolint:gosec // a path the operator typed, in a tool that runs as them
		if err != nil {
			return err
		}

		var raw []byte
		if err := c.Do(ctx, request{
			Method: http.MethodPost,
			Path:   "/api/v1/cluster-templates/plan",

			// YAML, because that is what the document is. The route takes the
			// file the operator wrote rather than a JSON envelope around it:
			// escaping every newline to post it would put a file nobody can
			// read into the request log.
			ContentType: "application/yaml",
			Body:        doc,
		}, &raw); err != nil {
			return err
		}
		if asJSON {
			return printJSON(raw)
		}
		return printPlan(raw)

	case "export":
		var raw []byte
		if err := c.Do(ctx, request{Method: http.MethodGet, Path: "/api/v1/clusters/" + args[1] + "/template"}, &raw); err != nil {
			return err
		}
		_, err := os.Stdout.Write(raw)
		return err

	default:
		return errUsage
	}
}

// printPlan renders the server's plan, and adds nothing to it.
//
// Every sentence here came from the response. This tool does not decide whether
// a template is usable, does not count the problems itself, and does not
// reword them -- the server's plan is the answer, and a CLI that formed a
// second opinion of it is the thing this whole tool is built to avoid.
func printPlan(raw []byte) error {
	var body struct {
		Plan   planRow `json:"plan"`
		Notice string  `json:"notice"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}

	fmt.Println(body.Plan.Sentence)
	for _, p := range body.Plan.Problems {
		fmt.Printf("  problem: %s\n", p)
	}
	for _, n := range body.Plan.Notes {
		fmt.Printf("  note: %s\n", n)
	}
	if len(body.Plan.ControlPlane.Machines) > 0 {
		fmt.Printf("  control plane: %s\n", strings.Join(body.Plan.ControlPlane.Machines, " "))
	}
	if len(body.Plan.Workers.Machines) > 0 {
		fmt.Printf("  workers: %s\n", strings.Join(body.Plan.Workers.Machines, " "))
	}
	if body.Notice != "" {
		fmt.Printf("\n%s\n", body.Notice)
	}

	// A plan with problems is a non-zero exit, so a pipeline can branch on it
	// without parsing anything. The plan is still printed: an exit code that
	// suppressed the reasons would make this worse than useless in a script.
	if len(body.Plan.Problems) > 0 {
		return fmt.Errorf("the template cannot be applied as written")
	}
	return nil
}

// configFile writes a talosconfig or a kubeconfig to stdout.
//
// To stdout and never to a file this tool chose: `> ~/.kube/config` is the
// operator saying where it goes, and a tool that wrote there itself would
// overwrite a file holding credentials for every other cluster they have.
func configFile(ctx context.Context, c *Client, args []string, which string) error {
	if len(args) != 1 {
		return errUsage
	}

	var raw []byte
	if err := c.Do(ctx, request{Method: http.MethodGet, Path: "/api/v1/clusters/" + args[0] + "/" + which}, &raw); err != nil {
		return err
	}
	_, err := os.Stdout.Write(raw)
	return err
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
