package machineconfig

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/siderolabs/talos/pkg/machinery/config/configloader"
	"github.com/siderolabs/talos/pkg/machinery/config/encoder"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

var (
	// ErrStagedAlreadyPending reports a second staged apply while one is
	// already waiting for the next boot (CFG-08).
	//
	// Talos accepts it and silently replaces the first, which is the worst
	// available outcome: the operator staged two changes and exactly one of
	// them will happen, with nothing anywhere saying which.
	ErrStagedAlreadyPending = errors.New("machineconfig: a staged configuration is already waiting for the next boot")

	// ErrDryRun reports that this instance was started with --dry-run and
	// cannot apply anything. It is not a failure of the request.
	ErrDryRun = errors.New("machineconfig: this instance runs in dry-run mode and applies nothing")
)

// Connector opens a client to a machine, exactly as the jobs package takes
// one: this package has no business knowing about the inventory.
type Connector func(ctx context.Context, id model.MachineID) (*talos.ClusterClient, error)

// Deps is what the service needs.
type Deps struct {
	Connect Connector

	// Mode carries whether this process may mutate. It is the value itself
	// rather than a boolean, for the reason httpapi.Deps states: it is what a
	// caller hands talos.NewClusterClient, so one field answers both "may we"
	// and "what do I pass".
	Mode talos.Mode

	Now func() time.Time
}

// Service is the config domain.
type Service struct {
	deps Deps

	mu sync.Mutex
	// staged records which machines have a configuration waiting for their
	// next boot, and what it was.
	//
	// In memory, and therefore lost on restart -- which is a real limitation
	// and is stated in the doc of StagedPending rather than hidden. The
	// alternative, a stored flag, would be a claim about a node that only the
	// node can answer, and it would go stale the moment somebody rebooted the
	// node outside holzkube-manager.
	staged map[model.MachineID]stagedRecord
}

type stagedRecord struct {
	At     time.Time
	Digest string
}

// New builds a service.
func New(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Service{deps: d, staged: map[model.MachineID]stagedRecord{}}
}

// View is a node's configuration as the screen shows it.
//
// Both fields are redacted. There is no unredacted field on this type and no
// flag that produces one: the type is the contract, and a caller cannot ask
// for the secrets by mistake.
type View struct {
	Machine model.MachineID `json:"machine"`

	// Raw is the configuration as YAML, redacted.
	Raw string `json:"raw"`

	// Rendered is the same document with comments and defaults filled in,
	// redacted. It is what an operator reads; Raw is what they diff.
	Rendered string `json:"rendered"`

	// ReadAt is when the node answered.
	ReadAt time.Time `json:"read_at"`

	// StagedPending reports that a configuration is waiting for this node's
	// next boot *as far as this process knows*.
	//
	// The qualifier is the honest part and it is why the field is named on the
	// wire rather than folded into a boolean somewhere: the record is in
	// memory, so a restart of holzkube-manager forgets it, and a reboot performed
	// outside holzkube-manager clears it on the node without clearing it here. It is
	// a guard against the common mistake -- staging twice in one sitting --
	// and not a claim about the node.
	StagedPending bool      `json:"staged_pending"`
	StagedAt      time.Time `json:"staged_at,omitzero"`
}

// Get reads a node's configuration and redacts it.
func (s *Service) Get(ctx context.Context, id model.MachineID) (View, error) {
	raw, err := s.read(ctx, id)
	if err != nil {
		return View{}, err
	}

	redacted, err := Redact(raw)
	if err != nil {
		return View{}, err
	}

	rendered, err := renderFull(raw)
	if err != nil {
		return View{}, err
	}
	renderedRedacted, err := Redact(rendered)
	if err != nil {
		return View{}, err
	}

	v := View{
		Machine:  id,
		Raw:      string(redacted),
		Rendered: string(renderedRedacted),
		ReadAt:   s.deps.Now().UTC(),
	}

	s.mu.Lock()
	if rec, ok := s.staged[id]; ok {
		v.StagedPending = true
		v.StagedAt = rec.At
	}
	s.mu.Unlock()

	return v, nil
}

// Preview is what a set of patches would do to a node.
type Preview struct {
	Machine model.MachineID `json:"machine"`

	Diff    Diff    `json:"diff"`
	Verdict Verdict `json:"verdict"`

	// Result is the patched configuration, redacted. It is here so the
	// operator can read the whole thing rather than only the changes; it is
	// the fourth of Redact's five exits.
	Result string `json:"result"`

	// Idempotent reports whether applying these patches twice is the same as
	// applying them once (CFG-05). A false here is nearly always a patch that
	// appends to a list.
	Idempotent bool `json:"idempotent"`

	// Valid reports whether Talos itself accepts the result, and Validation is
	// what it said.
	Valid      bool     `json:"valid"`
	Validation []string `json:"validation,omitempty"`
}

// Plan computes the diff, the mode and the validation for a set of patches
// without touching the node.
func (s *Service) Plan(ctx context.Context, id model.MachineID, patches []string) (Preview, error) {
	base, err := s.read(ctx, id)
	if err != nil {
		return Preview{}, err
	}

	patched, err := ApplyPatches(base, patches)
	if err != nil {
		return Preview{}, err
	}

	diff, err := Compare(base, patched)
	if err != nil {
		return Preview{}, err
	}

	redacted, err := Redact(patched)
	if err != nil {
		return Preview{}, err
	}

	p := Preview{
		Machine: id,
		Diff:    diff,
		Verdict: ModeFor(diff.Paths),
		Result:  string(redacted),
	}

	// Idempotence is checked per plan rather than per patch, because what the
	// operator is about to do is the whole set: two patches that are each
	// idempotent can still be non-idempotent together.
	idempotent, err := idempotentSet(base, patches)
	if err != nil {
		return Preview{}, err
	}
	p.Idempotent = idempotent

	p.Valid, p.Validation = validate(patched)
	return p, nil
}

// Apply sends a configuration to a node.
//
// It refuses a second staged apply (CFG-08) rather than letting Talos replace
// the first silently, and it refuses everything while the process is in
// dry-run.
func (s *Service) Apply(ctx context.Context, id model.MachineID, patches []string, mode Mode) (talos.ApplyResult, error) {
	if s.deps.Mode.DryRun {
		return talos.ApplyResult{}, ErrDryRun
	}

	if mode == ModeStaged {
		s.mu.Lock()
		rec, pending := s.staged[id]
		s.mu.Unlock()

		if pending {
			return talos.ApplyResult{}, fmt.Errorf(
				"%w: one was staged at %s. Talos would replace it without saying so, and exactly "+
					"one of the two changes would happen. Reboot the node to apply the first, or "+
					"apply this one immediately instead",
				ErrStagedAlreadyPending, rec.At.Format(time.RFC3339))
		}
	}

	base, err := s.read(ctx, id)
	if err != nil {
		return talos.ApplyResult{}, err
	}
	patched, err := ApplyPatches(base, patches)
	if err != nil {
		return talos.ApplyResult{}, err
	}

	cc, err := s.deps.Connect(ctx, id)
	if err != nil {
		return talos.ApplyResult{}, err
	}
	defer cc.Close() //nolint:errcheck // the apply's verdict is not a close error's to change

	result, err := cc.ApplyConfigurationWithMode(ctx, patched, string(mode))
	if err != nil {
		return talos.ApplyResult{}, err
	}

	if mode == ModeStaged {
		s.mu.Lock()
		s.staged[id] = stagedRecord{At: s.deps.Now().UTC(), Digest: result.Details}
		s.mu.Unlock()
	}

	// A reboot or a try applies now and leaves nothing staged, so anything
	// this process was remembering about the node is no longer true.
	if mode == ModeReboot || mode == ModeTry {
		s.mu.Lock()
		delete(s.staged, id)
		s.mu.Unlock()
	}

	return result, nil
}

// ClearStaged forgets that a node has a staged configuration.
//
// It is what a reboot job calls, and what an operator can call when they know
// the node booted outside holzkube-manager. It exists because the alternative --
// a guard nobody can clear -- is a guard people work around.
func (s *Service) ClearStaged(id model.MachineID) {
	s.mu.Lock()
	delete(s.staged, id)
	s.mu.Unlock()
}

// read fetches a node's current configuration.
func (s *Service) read(ctx context.Context, id model.MachineID) ([]byte, error) {
	cc, err := s.deps.Connect(ctx, id)
	if err != nil {
		return nil, err
	}
	defer cc.Close() //nolint:errcheck // the read's verdict is its own

	return cc.MachineConfigYAML(ctx)
}

// renderFull re-encodes a configuration with its comments and defaults, which
// is the "rendered" half of CFG-01.
func renderFull(raw []byte) ([]byte, error) {
	provider, err := configloader.NewFromBytes(raw)
	if err != nil {
		return nil, fmt.Errorf("machineconfig: the configuration did not parse: %w", err)
	}
	out, err := provider.EncodeBytes(encoder.WithComments(encoder.CommentsAll))
	if err != nil {
		return nil, fmt.Errorf("machineconfig: render the configuration: %w", err)
	}
	return out, nil
}

// validate asks Talos's own validator whether a configuration is acceptable.
func validate(raw []byte) (bool, []string) {
	provider, err := configloader.NewFromBytes(raw)
	if err != nil {
		return false, []string{err.Error()}
	}

	// The container mode, because that is the weakest of the three: it is the
	// one whose requirements a metal node also meets, so a configuration that
	// passes here passes everywhere, and one that fails is wrong regardless of
	// where it would run.
	// ValidateAsClient and not Validate, and machinery v1.14 deprecating the
	// latter is what made the distinction visible rather than what created it.
	// Validate is the in-Talos variant: it is allowed to check things that are
	// only knowable on the node itself. This process runs beside the cluster
	// and never on it, so asking the node's question here can only produce a
	// verdict about a machine that is not this one.
	warnings, err := provider.ValidateAsClient(validationMode{})
	messages := append([]string(nil), warnings...)
	if err != nil {
		messages = append(messages, err.Error())
		return false, messages
	}
	return true, messages
}

// validationMode is the RuntimeMode Talos's validator asks for.
//
// It reports itself as not requiring an installed disk, which is what makes
// the validation a statement about the document rather than about the machine
// it happens to be run on.
type validationMode struct{}

func (validationMode) String() string        { return "holzkube-manager" }
func (validationMode) RequiresInstall() bool { return false }
func (validationMode) InContainer() bool     { return false }

// idempotentSet reports whether applying the whole set twice is the same as
// once.
func idempotentSet(base []byte, patches []string) (bool, error) {
	once, err := ApplyPatches(base, patches)
	if err != nil {
		return false, err
	}
	twice, err := ApplyPatches(once, patches)
	if err != nil {
		return false, err
	}
	return string(once) == string(twice), nil
}
