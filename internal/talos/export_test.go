package talos

// Test-only access to the shared call path.
//
// It lives in an _test.go file, so it is compiled into the package's test
// binary and never into holzkube-managerd: this is a window for the test package next
// door, not an escape hatch on the seam. A production caller reaches a node
// through a method on one of the two client types, and that is the property
// D-06 and the maintenance client's closed method set exist to hold.
//
// It exists because the dry-run enumeration has to call *every* RPC the class
// table marks ClassMutation -- twenty-one of them -- and only three of those
// have a wrapper method. Hand-writing the other eighteen wrappers purely so a
// test could reach them would put methods on ClusterClient that nothing calls,
// which is the opposite of what the closed method set is for. Iterating the
// table instead is also what makes a mutating RPC added in a later phase
// covered by the enumeration on the day it is classified.

import (
	"context"

	"google.golang.org/grpc"
)

// RawInvoke issues one unary RPC by full method name through the shared call
// path, interceptors and all.
func (c *ClusterClient) RawInvoke(ctx context.Context, method string, req, reply any) error {
	return c.conn.c.Conn().Invoke(ctx, method, req, reply)
}

// RawStream opens one streaming RPC by full method name through the shared call
// path, interceptors and all.
func (c *ClusterClient) RawStream(ctx context.Context, desc *grpc.StreamDesc, method string) (grpc.ClientStream, error) {
	return c.conn.c.Conn().NewStream(ctx, desc, method)
}

// MaintenanceClient deliberately gets no such window.
//
// TestMaintenanceClientMethodSetIsClosed asserts its exported method set whole,
// by reflection, in this same test binary -- so a test-only method on it is
// still a method on it, and adding one here would have meant editing the guard
// that D-06 exists to hold. It also is not needed: the one mutation that path
// serves is ApplyConfiguration, which has a real wrapper, and the gate it hits
// is installed in dial, which is the same function the cluster path goes
// through. Proving it there proves it for both.
