package inventory

import (
	"context"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

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
