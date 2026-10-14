package talos

import (
	"context"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

// ApplyAttempt is one configuration a ClusterClient is about to send to its
// node.
type ApplyAttempt struct {
	Cluster model.ClusterID
	Machine model.MachineID

	// Config is the configuration as it is sent. It holds the cluster's
	// secrets and stays in memory: an observer reads it, keeps no copy of it
	// and writes it to no log, no store and no error text.
	Config []byte

	// Mode is the name the caller gave ("no-reboot", "reboot", "staged",
	// "try", "auto"). ApplyConfiguration, which sends no mode, reports
	// "reboot": the zero value of the wire is an unconditional reboot.
	Mode string
}

// ApplyObserver hears every configuration a ClusterClient sends, before the RPC
// and again with its result.
//
// The place is the client and not the callers, on purpose. Three packages and
// four call sites send a configuration through ApplyConfigurationWithMode
// (machineconfig, upgrade, rotateca), and every new function that writes one
// would be a new place to forget the observer. The client has exactly two
// methods that send one, one function, and a test pins that set whole.
//
// outcome may be nil. When it is not, it is called once with the RPC's error,
// nil on success, on every path out of the method that sent the configuration.
//
// An observer neither blocks nor makes the apply fail: whatever goes wrong in
// it is its own to log. A client without one behaves as it always did.
type ApplyObserver func(ctx context.Context, a ApplyAttempt) (outcome func(err error))

// SetApplyObserver hangs the observer on the client. It is set once, right
// after the client is built and before its first use; it is not safe to change
// while a call is running. MaintenanceClient has none: a node that is being
// provisioned has no baseline to move.
func (c *ClusterClient) SetApplyObserver(o ApplyObserver) { c.observer = o }

// observeApply tells the observer what is about to be sent and returns what to
// call with the result. It always returns something callable.
func (c *ClusterClient) observeApply(ctx context.Context, cfg []byte, mode string) func(error) {
	if c.observer == nil {
		return func(error) {}
	}
	outcome := c.observer(ctx, ApplyAttempt{
		Cluster: c.conn.target.Cluster,
		Machine: c.conn.target.Machine,
		Config:  cfg,
		Mode:    mode,
	})
	if outcome == nil {
		return func(error) {}
	}
	return outcome
}
