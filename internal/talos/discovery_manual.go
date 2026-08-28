package talos

import "context"

// NewManualSource returns a DiscoverySource for machines the operator named
// directly.
//
// It is the floor under discovery: whatever automatic source is configured, an
// operator must be able to say "this machine, at this address" and have it
// appear. It is also the source that makes the fan-in call site testable
// without a network.
func NewManualSource(targets ...Target) DiscoverySource {
	copied := make([]Target, len(targets))
	copy(copied, targets)
	return &manualSource{targets: copied}
}

type manualSource struct{ targets []Target }

func (m *manualSource) Kind() string { return "manual" }

// Run emits one Candidate per configured target and returns when the list is
// exhausted or the context is cancelled.
//
// It does not close out. A call site fans several sources into one channel, so
// closing belongs to the fan-in; a source that closed the channel would take
// the others down with it.
func (m *manualSource) Run(ctx context.Context, out chan<- Candidate) error {
	for _, t := range m.targets {
		candidate := Candidate{
			Target:   t,
			Identity: Identity{Machine: t.Machine},
			Source:   m.Kind(),
		}
		select {
		case out <- candidate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
