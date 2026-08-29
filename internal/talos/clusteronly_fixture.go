//go:build talos_compile_fail

// This file exists in order to fail to compile, and it is excluded from every
// ordinary build by a tag nothing else sets.
//
// D-06 says cluster and maintenance clients are two distinct wrapper types with
// non-overlapping method sets, and that a cluster-only call against a
// maintenance node must be a compile error rather than a runtime rejection.
// That claim cannot be asserted from inside the test binary: a file that does
// not compile does not compile for the tests either. So the offending call
// lives here, and TestMaintenanceClientRejectsClusterOnlyCall runs
//
//	go build -tags talos_compile_fail .
//
// and takes the non-zero exit -- plus the compiler naming MaintenanceClient and
// Bootstrap -- as the evidence.
//
// The second function is the negative control. It makes the same call on a
// *ClusterClient, where the method does exist, so a broken fixture (a typo, a
// renamed method) fails visibly instead of producing a build error that would
// look like the assertion succeeding.

package talos

import "context"

// clusterOnlyCallOnMaintenanceClient must not compile.
func clusterOnlyCallOnMaintenanceClient(ctx context.Context, m *MaintenanceClient) error {
	return m.Bootstrap(ctx)
}

// clusterOnlyCallOnClusterClient must compile. It is the control.
func clusterOnlyCallOnClusterClient(ctx context.Context, c *ClusterClient) error {
	return c.Bootstrap(ctx)
}
