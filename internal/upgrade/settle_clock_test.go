package upgrade_test

import (
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

// TestASettleWaitStartedByAnUnbelievableClockIsWaitedAgain: the settle step's
// start was recorded 72 hours ago by this host's clock, which on a Raspberry Pi
// without a clock that keeps time is as likely a jump as a long absence. The
// wait is not taken as done on that number: it is waited once more, which is
// all that costs.
func TestASettleWaitStartedByAnUnbelievableClockIsWaitedAgain(t *testing.T) {
	t.Parallel()
	r := newRemovalRig(t, 3, 0)
	m := r.machine("cp-1")
	cc, err := r.deps.Connect(t.Context(), m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := cc.EtcdLeaveCluster(leaveCtx(t)); err != nil {
		t.Fatal(err)
	}
	_ = cc.Close()

	r.stage(t, m, "let etcd settle", model.StepRunning)
	j, err := r.st.Jobs().Get(t.Context(), "staged-removal")
	if err != nil {
		t.Fatal(err)
	}
	for i := range j.Steps {
		if j.Steps[i].Name == "let etcd settle" {
			j.Steps[i].StartedAt = time.Now().Add(-72 * time.Hour)
		}
	}
	if _, err := r.st.Jobs().Put(t.Context(), j); err != nil {
		t.Fatal(err)
	}

	second := r.newEngine()
	if err := second.Resume(t.Context()); err != nil {
		t.Fatal(err)
	}
	r.await(second, j.ID, model.JobSucceeded)

	if w := r.waitList(); len(w) != 1 {
		t.Errorf("the settle step waited %d times after a resume measured against an "+
			"unbelievable clock, want once", len(w))
	}
}
