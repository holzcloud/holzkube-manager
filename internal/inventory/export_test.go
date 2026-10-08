package inventory

import (
	"context"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// SupervisorDone returns a channel that closes when the machine's observer
// loops have both returned, or nil if the machine is not supervised.
func (s *Service) SupervisorDone(id model.MachineID) <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sup, ok := s.supervised[id]; ok {
		return sup.done
	}
	return nil
}

// RecordMachine exposes the identity-filing rule to the package's own tests.
//
// D-10 is a rule about what happens between two records, and driving it
// through the import path would need two simulated nodes taking turns at one
// address -- a test about scaffolding rather than about the rule.
func (s *Service) RecordMachine(
	ctx context.Context,
	cluster model.ClusterID,
	facts talos.NodeFacts,
	addr string,
	role model.MachineRole,
) (model.Machine, error) {
	return s.recordMachine(ctx, cluster, facts, addr, role)
}
