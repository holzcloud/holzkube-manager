package provision

import (
	"context"
	"errors"
	"fmt"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// Service is what the HTTP layer holds.
//
// It exists so that the handlers have one thing to call and no credentials to
// assemble. The wizard's steps are four reads and one submission, and every
// one of the reads needs the same two things -- a dialer and the credentials a
// machine with no PKI is reached with -- which is exactly the pair a handler
// should not be building for itself.
type Service struct {
	scanner     *Scanner
	dialer      talos.Dialer
	maintenance func(fingerprint string) talos.Creds

	bootstrapper *Bootstrapper

	// controlPlanes counts the control-plane nodes a cluster already has, so
	// the even-count warning can be about the cluster as it will be.
	controlPlanes func(ctx context.Context, id model.ClusterID) (int, error)
}

// NewService wires one up.
func NewService(
	d talos.Dialer,
	maintenance func(fingerprint string) talos.Creds,
	known func(ctx context.Context, addr string) bool,
	b *Bootstrapper,
	controlPlanes func(ctx context.Context, id model.ClusterID) (int, error),
) *Service {
	return &Service{
		scanner:       NewScanner(d, known),
		dialer:        d,
		maintenance:   maintenance,
		bootstrapper:  b,
		controlPlanes: controlPlanes,
	}
}

// Scan finds machines (PROV-01).
func (s *Service) Scan(ctx context.Context, req ScanRequest) ([]Found, error) {
	return s.scanner.Scan(ctx, req)
}

// Inspect reads one machine (PROV-03, PROV-06).
//
// fingerprint, when given, is the value the operator read off the machine's
// console and is what turns an unauthenticated connection into one that at
// least cannot be silently taken over by a second machine at the same address
// (PROV-04).
func (s *Service) Inspect(ctx context.Context, addr, fingerprint string) (Candidate, error) {
	return Inspect(ctx, s.dialer, addr, s.maintenance(fingerprint))
}

// Preview is what the last screen before the apply shows.
//
// Every field on it is something the operator would otherwise have to work out
// themselves from three other screens, and the two that matter most -- the
// install image and whether this run initialises etcd -- are the two whose
// consequences arrive minutes later and look like something else.
type Preview struct {
	Request Request `json:"request"`

	// Warnings are the things that are true of this plan and will otherwise
	// produce a failure that looks like something else.
	Warnings []string `json:"warnings,omitempty"`

	// InstallImage is the exact `.machine.install.image` this plan writes. It
	// is shown because PROV-08's failure is silent: the install succeeds, the
	// node joins, and the extensions are simply gone.
	InstallImage string `json:"install_image"`

	// ControlPlaneAfter is how many control-plane nodes the cluster will have.
	ControlPlaneAfter int `json:"control_plane_after"`

	// Bootstrap says whether this run will initialise etcd, which is the one
	// irreversible thing in it that is not about this machine.
	Bootstrap bool `json:"bootstrap"`

	// CNINotice is PROV-13, carried here so that the screen the operator
	// watches afterwards has it before the state it explains appears.
	CNINotice string `json:"cni_notice"`

	// MaintenanceWarning is PROV-04, for the same reason.
	MaintenanceWarning string `json:"maintenance_warning"`
}

// Plan validates a request and describes what applying it would do.
//
// It writes nothing. The PROV-05 identity check is deliberately *not* here:
// verifying two screens before the apply would be verifying a memory, and the
// job does it again in the same breath as the write.
func (s *Service) Plan(ctx context.Context, req Request) (Preview, error) {
	existing := 0
	if s.controlPlanes != nil {
		n, err := s.controlPlanes(ctx, req.Cluster)
		if err != nil {
			return Preview{}, err
		}
		existing = n
	}

	warnings, err := req.Validate(existing)
	if err != nil {
		return Preview{}, err
	}

	p := Preview{
		Request:            req,
		Warnings:           warnings,
		InstallImage:       req.InstallerImage,
		ControlPlaneAfter:  existing,
		Bootstrap:          req.ControlPlane && existing == 0,
		CNINotice:          CNINotice,
		MaintenanceWarning: MaintenanceWarning,
	}
	if req.ControlPlane {
		p.ControlPlaneAfter = existing + 1
	}

	// An etcd bootstrap that this plan would perform, over a cluster that has
	// an unresolved attempt on record, is refused here rather than at the job
	// step -- so that the operator learns it on the screen where they can still
	// go and look, instead of watching a job get most of the way through a
	// machine install and then stop.
	if p.Bootstrap && s.bootstrapper != nil {
		if intent, ok := s.bootstrapper.PendingIntent(req.Cluster); ok {
			return Preview{}, fmt.Errorf("%w: an attempt against %s started at %s and has no "+
				"recorded outcome. Resolve it before provisioning another control-plane node",
				ErrBootstrapUnclear, intent.Addr, intent.StartedAt.Format("2006-01-02 15:04:05 MST"))
		}
	}

	return p, nil
}

// PendingBootstraps is the recovery screen's list (PROV-10).
func (s *Service) PendingBootstraps() ([]BootstrapIntent, error) {
	if s.bootstrapper == nil {
		return nil, errors.New("provision: this instance has no bootstrap lease directory")
	}
	return s.bootstrapper.Pending()
}

// ResolveBootstrap records what a person found.
func (s *Service) ResolveBootstrap(cluster model.ClusterID, bootstrapped bool, note string) error {
	if s.bootstrapper == nil {
		return errors.New("provision: this instance has no bootstrap lease directory")
	}
	if note == "" {
		return errors.New("provision: say what you found. This record is the only account of a " +
			"bootstrap nothing else can reconstruct, and an empty note makes it an account of nothing")
	}
	return s.bootstrapper.Resolve(cluster, bootstrapped, note)
}

// Notices are the sentences the wizard opens with (PROV-12, PROV-04).
//
// They are served rather than written into the frontend because each describes
// how a machine boots rather than anything holzkube-manager does, and a copy in
// the browser bundle is a copy that drifts from the behaviour it describes.
func (s *Service) Notices() []string {
	return []string{ShadowedISOWarning, NoDHCPWarning, MaintenanceWarning}
}
