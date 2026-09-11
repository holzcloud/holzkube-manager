// Command walking-skeleton drives the whole provisioning path once, end to
// end, against a QEMU machine — hardcoded, ugly and real.
//
// # This is throwaway code and it is marked as such deliberately
//
// It exists to turn four unknowns into known problems before phase 8 builds
// the real thing on top of them (ROADMAP phase 4). It satisfies **no**
// requirement. Nothing in `internal/` imports it, nothing in the product calls
// it, and a green run here is explicitly **not** a reason to shorten phase 8.
// The module boundary makes that structural rather than a promise: this is the
// sandbox module, and `internal/depguard_test.go` asserts none of it reaches
// the product binary.
//
// The four unknowns, in the order this program meets them:
//
//  1. Does a QEMU Talos node come up at all on the operator's host, and what
//     does it cost in setup? (`sudo`, `vmnet-shared` on darwin.)
//  2. Does a machine in maintenance mode accept a MachineConfig generated from
//     a hardcoded schematic, install, reboot and come back as a cluster node —
//     with nobody opening a terminal?
//  3. **How long is the silence after the apply?** This is the number phase 8
//     needs and the reason this program measures rather than narrates. An
//     unmeasured silence becomes "it seems to hang" in a progress bar somebody
//     has to design.
//  4. What do the two dangerous mistakes actually look like — a second etcd
//     bootstrap, and an apply sent to the wrong address?
//
// Everything it observes is written to a markdown record, because a
// measurement nobody wrote down is an anecdote.
//
// Usage:
//
//	walking-skeleton --node 10.5.0.2 --out .planning/phases/04-walking-skeleton/04-MEASUREMENTS.md
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
	clientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"
	"github.com/siderolabs/talos/pkg/machinery/config/generate"
	machinetype "github.com/siderolabs/talos/pkg/machinery/config/machine"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The hardcoded everything. This is the "ugly" half of the phase's own
// description, and it is on purpose: every value here is a decision phase 7 or
// phase 8 makes properly, and hardcoding it is what keeps this program about
// the four unknowns instead of about configuration.
const (
	clusterName       = "skeleton"
	kubernetesVersion = "1.34.1"

	// The schematic id of a stock Talos image with no extensions. Hardcoded,
	// because choosing one is phase 8's job and building the Factory call into
	// a throwaway program would be building phase 8 twice.
	schematicID = "376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba"

	// How long the program is willing to wait for a node to come back after
	// the apply. Generous, because the point is to *measure* the silence: a
	// short ceiling would report a timeout where the answer is a number.
	rebootBudget = 20 * time.Minute

	// How often the node is probed while it is silent. Two seconds is fine
	// here and would not be in the product: this is one node, and the
	// resolution of the measurement is the point.
	probeInterval = 2 * time.Second
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "walking-skeleton:", err)
		os.Exit(1)
	}
}

func run() error {
	node := flag.String("node", "", "address of a machine in maintenance mode (required)")
	wrong := flag.String("wrong-node", "", "an address that is NOT a Talos node, for unknown 4")
	out := flag.String("out", "", "write the measurement record here (required)")
	talosconfigOut := flag.String("talosconfig", "talosconfig", "where to write the generated talosconfig")
	flag.Parse()

	if *node == "" || *out == "" {
		flag.Usage()
		return errors.New("--node and --out are both required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), rebootBudget+10*time.Minute)
	defer cancel()

	rec := &record{Node: *node, StartedAt: time.Now()}
	defer func() {
		if err := rec.write(*out); err != nil {
			fmt.Fprintln(os.Stderr, "could not write the measurement record:", err)
		}
	}()

	// --- Unknown 2, first half: generate a configuration for this machine.
	addr, err := netip.ParseAddr(*node)
	if err != nil {
		return fmt.Errorf("--node %q: %w", *node, err)
	}

	in, err := generate.NewInput(clusterName, "https://"+addr.String()+":6443", kubernetesVersion)
	if err != nil {
		return fmt.Errorf("generate config input: %w", err)
	}
	cfg, err := in.Config(machinetype.TypeControlPlane)
	if err != nil {
		return fmt.Errorf("generate control-plane config: %w", err)
	}
	raw, err := cfg.Bytes()
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	rec.ConfigBytes = len(raw)
	rec.SchematicID = schematicID

	tc, err := in.Talosconfig()
	if err != nil {
		return fmt.Errorf("generate talosconfig: %w", err)
	}
	tc.Contexts[tc.Context].Endpoints = []string{addr.String()}
	tc.Contexts[tc.Context].Nodes = []string{addr.String()}
	tcRaw, err := tc.Bytes()
	if err != nil {
		return fmt.Errorf("encode talosconfig: %w", err)
	}
	if err := os.WriteFile(*talosconfigOut, tcRaw, 0o600); err != nil {
		return fmt.Errorf("write talosconfig: %w", err)
	}

	// --- Unknown 2, second half: apply it to the maintenance-mode node.
	fmt.Printf("applying a %d-byte configuration to %s in maintenance mode…\n", len(raw), addr)

	appliedAt := time.Now()
	if err := applyInMaintenance(ctx, addr.String(), raw); err != nil {
		rec.ApplyError = err.Error()
		return fmt.Errorf("apply: %w", err)
	}
	rec.AppliedAt = appliedAt

	// --- Unknown 3: the silence. This is the number phase 8 needs.
	fmt.Println("measuring the installation silence…")
	silence, firstAnswer, err := measureSilence(ctx, addr.String(), tcRaw, appliedAt)
	rec.SilenceSeconds = silence.Seconds()
	rec.FirstAnswer = firstAnswer
	if err != nil {
		rec.SilenceError = err.Error()
		fmt.Fprintln(os.Stderr, "the node did not come back inside the budget:", err)
	} else {
		fmt.Printf("the node answered again after %s\n", silence.Round(time.Second))
	}

	// --- Unknown 4a: bootstrap twice.
	fmt.Println("bootstrapping, then bootstrapping again…")
	rec.FirstBootstrap, rec.SecondBootstrap = bootstrapTwice(ctx, addr.String(), tcRaw)

	// --- Unknown 4b: apply to an address that is not a Talos node.
	if *wrong != "" {
		fmt.Printf("applying to %s, which is not a Talos node…\n", *wrong)
		start := time.Now()
		err := applyInMaintenance(ctx, *wrong, raw)
		rec.WrongAddressSeconds = time.Since(start).Seconds()
		rec.WrongAddressResult = describe(err)
	} else {
		rec.WrongAddressResult = "not attempted: --wrong-node was not given"
	}

	rec.FinishedAt = time.Now()
	fmt.Printf("\nmeasurement record written to %s\n", *out)
	return nil
}

// applyInMaintenance sends a configuration to a node that has no cluster PKI.
//
// Insecure, because maintenance mode has no certificate authority to verify
// against and this is a throwaway program on a throwaway network. The product
// pins a fingerprint instead; that is phase 8's work and one of the reasons
// this program exists is to find out what it has to pin.
func applyInMaintenance(ctx context.Context, addr string, cfg []byte) error {
	callCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	c, err := client.New(callCtx,
		client.WithTLSConfig(insecureTLS()),
		client.WithEndpoints(addr),
	)
	if err != nil {
		return err
	}
	defer c.Close() //nolint:errcheck // throwaway

	_, err = c.ApplyConfiguration(callCtx, &machine.ApplyConfigurationRequest{
		Data: cfg,
		Mode: machine.ApplyConfigurationRequest_AUTO,
	})
	return err
}

// measureSilence is unknown 3.
//
// It probes with the *cluster* credentials, not the maintenance ones: what is
// being measured is not "is something listening" but "is the node this
// configuration made, answering as a member of this cluster". A probe that
// accepted the maintenance-mode API would stop the clock while the machine was
// still the machine it used to be.
func measureSilence(ctx context.Context, addr string, talosconfig []byte, from time.Time) (time.Duration, string, error) {
	cfg, err := clientconfig.FromBytes(talosconfig)
	if err != nil {
		return 0, "", err
	}

	deadline := from.Add(rebootBudget)
	var lastErr error

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return time.Since(from), "", ctx.Err()
		case <-time.After(probeInterval):
		}

		probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		c, err := client.New(probeCtx, client.WithConfig(cfg), client.WithEndpoints(addr))
		if err != nil {
			cancel()
			lastErr = err
			continue
		}
		resp, err := c.Version(probeCtx)
		_ = c.Close()
		cancel()

		if err != nil {
			lastErr = err
			continue
		}
		if len(resp.GetMessages()) == 0 {
			lastErr = errors.New("empty version response")
			continue
		}
		return time.Since(from), resp.GetMessages()[0].GetVersion().GetTag(), nil
	}
	return time.Since(from), "", fmt.Errorf("still silent after %s; last error: %w", rebootBudget, lastErr)
}

// bootstrapTwice is unknown 4a: what a second etcd bootstrap actually does.
//
// The expected answer is AlreadyExists, and the simulator asserts exactly that
// (talossim's second_bootstrap_returns_AlreadyExists). This is where that
// expectation is checked against a machine nobody is simulating — which is the
// whole reason the phase exists.
func bootstrapTwice(ctx context.Context, addr string, talosconfig []byte) (first, second string) {
	cfg, err := clientconfig.FromBytes(talosconfig)
	if err != nil {
		return "config: " + err.Error(), "not attempted"
	}

	call := func() string {
		callCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()

		c, err := client.New(callCtx, client.WithConfig(cfg), client.WithEndpoints(addr))
		if err != nil {
			return describe(err)
		}
		defer c.Close() //nolint:errcheck // throwaway

		return describe(c.Bootstrap(callCtx, &machine.BootstrapRequest{}))
	}

	return call(), call()
}

// describe renders an outcome the way the record wants it: the gRPC code when
// there is one, because that is the thing phase 6 will branch on, and the
// message either way.
func describe(err error) string {
	if err == nil {
		return "OK"
	}
	if st, ok := status.FromError(err); ok && st.Code() != codes.Unknown {
		return fmt.Sprintf("%s: %s", st.Code(), st.Message())
	}
	return err.Error()
}

// record is what this program writes down.
type record struct {
	Node        string
	SchematicID string
	ConfigBytes int

	StartedAt  time.Time
	AppliedAt  time.Time
	FinishedAt time.Time

	ApplyError string

	SilenceSeconds float64
	SilenceError   string
	FirstAnswer    string

	FirstBootstrap  string
	SecondBootstrap string

	WrongAddressSeconds float64
	WrongAddressResult  string
}

func (r *record) write(path string) error {
	var b strings.Builder

	fmt.Fprintf(&b, "# Phase 4 — Walking Skeleton: measurements\n\n")
	fmt.Fprintf(&b, "Written by `sandbox/cmd/walking-skeleton` on %s.\n\n", r.StartedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "This is a record of what one machine actually did, not of what it was expected to do.\n\n")

	fmt.Fprintf(&b, "## What was driven\n\n")
	fmt.Fprintf(&b, "| | |\n|---|---|\n")
	fmt.Fprintf(&b, "| Node | `%s` |\n", r.Node)
	fmt.Fprintf(&b, "| Schematic | `%s` |\n", r.SchematicID)
	fmt.Fprintf(&b, "| Config size | %d bytes |\n", r.ConfigBytes)
	fmt.Fprintf(&b, "| Applied at | %s |\n\n", stamp(r.AppliedAt))

	fmt.Fprintf(&b, "## Unknown 2 — does a maintenance-mode machine take the config\n\n")
	if r.ApplyError == "" {
		fmt.Fprintf(&b, "Accepted.\n\n")
	} else {
		fmt.Fprintf(&b, "**Refused**: `%s`\n\n", r.ApplyError)
	}

	fmt.Fprintf(&b, "## Unknown 3 — the installation silence\n\n")
	fmt.Fprintf(&b, "**%.0f seconds** (%s) between the apply returning and the node answering\n", r.SilenceSeconds, time.Duration(r.SilenceSeconds*float64(time.Second)).Round(time.Second))
	fmt.Fprintf(&b, "`Version` under its *cluster* credentials.\n\n")
	if r.FirstAnswer != "" {
		fmt.Fprintf(&b, "It came back as `%s`.\n\n", r.FirstAnswer)
	}
	if r.SilenceError != "" {
		fmt.Fprintf(&b, "**It did not come back**: `%s`\n\n", r.SilenceError)
	}
	fmt.Fprintf(&b, "This is the number phase 8 has to design a progress indicator around. "+
		"Anything that leaves the operator without a signal for this long reads as a hang.\n\n")

	fmt.Fprintf(&b, "## Unknown 4a — bootstrapping twice\n\n")
	fmt.Fprintf(&b, "| attempt | outcome |\n|---|---|\n")
	fmt.Fprintf(&b, "| first | `%s` |\n", r.FirstBootstrap)
	fmt.Fprintf(&b, "| second | `%s` |\n\n", r.SecondBootstrap)
	fmt.Fprintf(&b, "`internal/talossim`'s `second_bootstrap_returns_AlreadyExists` claims the second is "+
		"`AlreadyExists`. If the row above says something else, the simulator is wrong and every test "+
		"built on it is measuring the wrong thing.\n\n")

	fmt.Fprintf(&b, "## Unknown 4b — applying to the wrong address\n\n")
	fmt.Fprintf(&b, "`%s` after %.1f seconds.\n\n", r.WrongAddressResult, r.WrongAddressSeconds)
	fmt.Fprintf(&b, "What matters here is the *shape* and the *timing*: phase 8 has to tell an operator "+
		"who typed one digit wrong what happened, quickly, and without implying the machine was harmed.\n\n")

	fmt.Fprintf(&b, "---\n\n*Finished %s.*\n", stamp(r.FinishedAt))

	return os.WriteFile(path, []byte(b.String()), 0o644) //nolint:gosec // a record meant to be read and committed
}

func stamp(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format(time.RFC3339)
}

// insecureTLS is the maintenance-mode connection.
//
// A node that has never been configured presents a self-signed certificate and
// there is no cluster authority to check it against, so there is nothing to
// verify. The product does not do this: it pins a fingerprint the operator
// confirmed. That pinning is phase 8's work, and one of the reasons this
// program exists is to find out what there is to pin.
func insecureTLS() *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // maintenance mode has no CA; see the comment above
		MinVersion:         tls.VersionTLS12,
	}
}
