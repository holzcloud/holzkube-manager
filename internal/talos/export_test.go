package talos

// Test-only access to the shared call path.
//
// It lives in an _test.go file, so it is compiled into the package's test
// binary and never into holzkubed: this is a window for the test package next
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

// RawInvoke is the maintenance client's half, so the enumeration can prove the
// gate applies on both paths rather than on the one it happened to test.
func (m *MaintenanceClient) RawInvoke(ctx context.Context, method string, req, reply any) error {
	return m.conn.c.Conn().Invoke(ctx, method, req, reply)
}

// RawStream is the maintenance client's half of RawStream.
func (m *MaintenanceClient) RawStream(ctx context.Context, desc *grpc.StreamDesc, method string) (grpc.ClientStream, error) {
	return m.conn.c.Conn().NewStream(ctx, desc, method)
}
