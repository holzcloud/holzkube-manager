package provision

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/machineconfig"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// Provisioning as a job, which is what makes a closed browser tab harmless
// (PROV-11).
//
// The state lives in the job record, in the store, and the browser is a
// viewer. That is not a convenience: a provisioning run takes minutes, the
// operator will navigate away, and a run whose state lived in a page would be
// a run that is lost when they do -- with a machine half-installed and nothing
// anywhere saying so.
//
// The steps, and what each one costs if it is interrupted:
//
//  1. **verify the machine** -- a read. Repeating it is free, and it is the
//     PROV-05 check: the UUID is confirmed against the machine *now*, not
//     against what the wizard read two minutes ago.
//  2. **apply the configuration** -- the irreversible one. It is verifiable:
//     a machine that took its configuration is no longer in maintenance mode,
//     and asking is a read.
//  3. **wait for it to come back** -- a read, repeated. It is the three-way
//     probe rather than a spinner.
//  4. **bootstrap etcd**, for the first control-plane node only -- guarded by
//     four independent mechanisms, and unverifiable in the step sense, so an
//     interruption here parks the job rather than retrying.

// JobKindProvision is the job kind.
const JobKindProvision = model.JobKind("node.provision")

// Deps is what the provisioning steps need.
type Deps struct {
	Dialer talos.Dialer

	// MaintenanceCreds builds the credentials for an unconfigured machine.
	// They are a function of the pinned fingerprint, which is why this is a
	// function rather than a value.
	MaintenanceCreds func(fingerprint string) talos.Creds

	// ClusterCreds loads a cluster's credentials from its stored bundle.
	ClusterCreds func(ctx context.Context, id model.ClusterID) (talos.Creds, error)

	// Secrets loads a cluster's stored bundle, which is what the generated
	// configuration is built from. Phase 3 derived it; this phase consumes it
	// and never creates one.
	Secrets func(ctx context.Context, id model.ClusterID) (model.ClusterSecrets, error)

	// Cluster loads the cluster record, for its name and endpoint.
	Cluster func(ctx context.Context, id model.ClusterID) (model.Cluster, error)

	// PatchBodies resolves stored patch ids to bodies.
	PatchBodies func(ctx context.Context, ids []string) ([]string, error)

	// Record files the new machine in the inventory once it answers.
	Record func(ctx context.Context, cluster model.ClusterID, addr string, controlPlane bool) error

	// Bootstrapper owns the etcd lease.
	Bootstrapper *Bootstrapper

	// KubernetesVersion is what a generated configuration installs, for the
	// cluster this machine is joining.
	//
	// It is a function of the cluster rather than a constant because a node
	// that joins at a different Kubernetes version than the rest of the
	// cluster is a node that joins and then does not work, in a way that looks
	// like a networking problem. The cluster's own nodes are what it is read
	// from; the composition root decides what to do about a cluster that has
	// none yet.
	KubernetesVersion func(ctx context.Context, id model.ClusterID) (string, error)
}

// Register teaches a job engine how to provision.
func Register(e *jobs.Engine, d Deps) {
	e.Register(JobKindProvision, func(j model.Job) ([]jobs.Step, error) {
		req, err := RequestFromParams(j.Params)
		if err != nil {
			return nil, err
		}
		return steps(d, req), nil
	})
}

func steps(d Deps, req Request) []jobs.Step {
	return []jobs.Step{
		{
			Name: "confirm this is the right machine",
			Do: func(ctx context.Context, job *model.Job) error {
				// PROV-05. It reconnects and reads the UUID rather than
				// trusting what the wizard saw: between the operator reading
				// the screen and this running, a DHCP lease can move.
				if err := VerifyMachine(ctx, d.Dialer, req.Addr, req.UUID,
					d.MaintenanceCreds(req.Fingerprint)); err != nil {
					return err
				}
				job.Steps[job.Current].Detail = fmt.Sprintf("%s is %s", req.Addr, req.UUID)
				return nil
			},
			// A read that changes nothing, so repeating it after an
			// interruption costs one round trip and risks nothing.
			Happened: func(context.Context, *model.Job) (bool, error) { return false, nil },
		},
		{
			Name: "apply the configuration",
			Do: func(ctx context.Context, job *model.Job) error {
				raw, err := buildConfig(ctx, d, req)
				if err != nil {
					return err
				}

				// Verified once more, immediately before the write. The step
				// above ran at the start of this job; this runs in the same
				// breath as the apply, which is what PROV-05 is actually
				// asking for.
				if err := VerifyMachine(ctx, d.Dialer, req.Addr, req.UUID,
					d.MaintenanceCreds(req.Fingerprint)); err != nil {
					return err
				}

				target := talos.Target{Machine: req.UUID, Addr: req.Addr}
				mc, err := talos.NewMaintenanceClient(ctx, d.Dialer, target,
					d.MaintenanceCreds(req.Fingerprint), talos.Mode{})
				if err != nil {
					return err
				}
				defer mc.Close() //nolint:errcheck // the apply's verdict is its own

				result, err := mc.ApplyConfiguration(ctx, raw)
				if err != nil {
					return err
				}
				job.Steps[job.Current].Detail = "accepted in " + result.Mode + " mode; " +
					"the machine installs to " + req.InstallDisk + " and reboots"
				return nil
			},
			// A machine that took its configuration is no longer in
			// maintenance mode, and asking is a read. That is what makes the
			// one irreversible step here resumable rather than parked.
			Happened: func(ctx context.Context, _ *model.Job) (bool, error) {
				return !inMaintenance(ctx, d, req), nil
			},
		},
		{
			Name: "wait for the machine to come back",
			Do: func(ctx context.Context, job *model.Job) error {
				started := time.Now()
				clusterCreds, err := d.ClusterCreds(ctx, req.Cluster)
				if err != nil {
					return err
				}

				for {
					r := Probe(ctx, d.Dialer, req.Addr, req.UUID, clusterCreds,
						d.MaintenanceCreds(req.Fingerprint), started)
					job.Steps[job.Current].Detail = r.Sentence

					switch r.State {
					case ReappearBack:
						return d.Record(ctx, req.Cluster, req.Addr, req.ControlPlane)
					case ReappearStillMaintenance:
						// This never resolves itself. Failing now is better
						// than a progress bar that runs until somebody gives
						// up on it.
						return fmt.Errorf("%s", r.Sentence)
					}

					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(10 * time.Second):
					}
				}
			},
			// A read, repeated. The machine is back or it is not.
			Happened: func(context.Context, *model.Job) (bool, error) { return false, nil },
		},
		{
			Name: "bootstrap etcd",
			Do: func(ctx context.Context, job *model.Job) error {
				if !req.ControlPlane {
					job.Steps[job.Current].Detail = "not a control-plane node; nothing to bootstrap"
					return nil
				}

				creds, err := d.ClusterCreds(ctx, req.Cluster)
				if err != nil {
					return err
				}
				target := talos.Target{Cluster: req.Cluster, Machine: req.UUID, Addr: req.Addr}

				cc, err := talos.NewClusterClient(ctx, d.Dialer, target, creds, talos.Mode{})
				if err != nil {
					return err
				}
				defer cc.Close() //nolint:errcheck // the bootstrap's verdict is its own

				err = d.Bootstrapper.Bootstrap(ctx, cc, req.Cluster, req.UUID, req.Addr)
				switch {
				case err == nil:
					job.Steps[job.Current].Detail = "etcd initialised. " + CNINotice
					return nil
				case isAlready(err):
					// Not a failure: the cluster is in the state that was
					// wanted, and reporting it as a failure would send
					// somebody to fix something that works.
					job.Steps[job.Current].Detail = "etcd was already running. " + CNINotice
					return nil
				default:
					return err
				}
			},
			// Deliberately nil. A bootstrap that was interrupted mid-call
			// cannot be checked -- a node whose etcd has not started yet looks
			// exactly like one that was never bootstrapped -- and the four
			// mechanisms in bootstrap.go exist because guessing here destroys
			// a cluster. An interruption parks the job.
			Happened: nil,
		},
	}
}

// buildConfig generates this machine's configuration.
func buildConfig(ctx context.Context, d Deps, req Request) ([]byte, error) {
	secrets, err := d.Secrets(ctx, req.Cluster)
	if err != nil {
		return nil, err
	}
	cluster, err := d.Cluster(ctx, req.Cluster)
	if err != nil {
		return nil, err
	}

	patches, err := d.PatchBodies(ctx, req.PatchIDs)
	if err != nil {
		return nil, err
	}

	kubernetes, err := d.KubernetesVersion(ctx, req.Cluster)
	if err != nil {
		return nil, err
	}

	// The install patch is built here rather than being something the operator
	// writes, and the image comes from the *same* schematic id as the ISO
	// (PROV-08). A machine that boots an ISO with extensions and installs a
	// stock installer comes up without them, and nothing reports it.
	install := fmt.Sprintf("machine:\n  install:\n    disk: %s\n    image: %s\n",
		req.InstallDisk, InstallImage(req.SchematicID, req.TalosVersion))
	patches = append(patches, install)

	if req.Hostname != "" {
		patches = append(patches, fmt.Sprintf("machine:\n  network:\n    hostname: %s\n", req.Hostname))
	}

	endpoint := cluster.Endpoint
	if endpoint != "" && len(endpoint) > 4 && endpoint[:4] != "http" {
		endpoint = "https://" + endpoint + ":6443"
	}

	return machineconfig.Generate(machineconfig.GenerateInput{
		ClusterName:  cluster.Name,
		Endpoint:     endpoint,
		Secrets:      secrets,
		ControlPlane: req.ControlPlane,
		// The cluster's pinned contract, not this build's idea of one: a
		// configuration generated under a newer contract can use fields an
		// older node rejects.
		TalosVersion:      contractOf(req.TalosVersion),
		KubernetesVersion: kubernetes,
		Patches:           patches,
	})
}

// contractOf reduces a full version to the major.minor contract machinery
// wants, because "v1.13.9" is a release and "v1.13" is a contract.
func contractOf(version string) string {
	v := version
	if len(v) > 0 && v[0] != 'v' {
		v = "v" + v
	}
	dots := 0
	for i, c := range v {
		if c == '.' {
			dots++
			if dots == 2 {
				return v[:i]
			}
		}
	}
	return v
}

func inMaintenance(ctx context.Context, d Deps, req Request) bool {
	probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	target := talos.Target{Machine: req.UUID, Addr: req.Addr}
	mc, err := talos.NewMaintenanceClient(probeCtx, d.Dialer, target,
		d.MaintenanceCreds(req.Fingerprint), talos.Mode{})
	if err != nil {
		return false
	}
	defer mc.Close() //nolint:errcheck // a probe's verdict is its own

	_, err = mc.Version(probeCtx)
	return err == nil
}

// isAlready recognises the one bootstrap outcome that is not a failure: the
// cluster is already in the state that was wanted.
func isAlready(err error) bool {
	return errors.Is(err, ErrAlreadyBootstrapped)
}

// Params turns a request into the job's stored parameters.
//
// They are stored so that a resumed job provisions the machine that was asked
// for rather than a default -- the same reason a reset stores its flags.
func (r Request) Params() map[string]string {
	out := map[string]string{
		"addr":          r.Addr,
		"uuid":          string(r.UUID),
		"cluster":       string(r.Cluster),
		"control_plane": strconv.FormatBool(r.ControlPlane),
		"install_disk":  r.InstallDisk,
		"talos_version": r.TalosVersion,
	}
	if r.SchematicID != "" {
		out["schematic_id"] = r.SchematicID
	}
	if r.Fingerprint != "" {
		out["fingerprint"] = r.Fingerprint
	}
	if r.Hostname != "" {
		out["hostname"] = r.Hostname
	}
	if len(r.PatchIDs) > 0 {
		raw, err := json.Marshal(r.PatchIDs)
		if err == nil {
			out["patch_ids"] = string(raw)
		}
	}
	return out
}

// RequestFromParams reads a request back.
func RequestFromParams(params map[string]string) (Request, error) {
	r := Request{
		Addr:         params["addr"],
		UUID:         model.MachineID(params["uuid"]),
		Cluster:      model.ClusterID(params["cluster"]),
		InstallDisk:  params["install_disk"],
		SchematicID:  params["schematic_id"],
		TalosVersion: params["talos_version"],
		Fingerprint:  params["fingerprint"],
		Hostname:     params["hostname"],
	}

	if raw, ok := params["control_plane"]; ok && raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return Request{}, fmt.Errorf("provision: %q is not a boolean for control_plane", raw)
		}
		r.ControlPlane = v
	}
	if raw := params["patch_ids"]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &r.PatchIDs); err != nil {
			return Request{}, fmt.Errorf("provision: the stored patch ids do not parse: %w", err)
		}
	}

	if _, err := r.Validate(0); err != nil {
		return Request{}, err
	}
	return r, nil
}
