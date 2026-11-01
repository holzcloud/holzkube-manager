package hostaction

import "testing"

// The filter must drop a runner image's own warnings and keep every line about
// our units. Separate from TestUnitsVerify because that test needs
// systemd-analyze and skips without it, which would leave this unmeasured.
func TestOurLinesKeepsOurUnitsAndDropsTheHostsOwn(t *testing.T) {
	t.Parallel()

	raw := []byte("/usr/lib/systemd/system/xfs_scrub_all.service:26: Support for option CPUAccounting= has been removed and it is ignored\n" +
		"/usr/lib/systemd/system/walinuxagent.service:21: Support for option CPUAccounting= has been removed and it is ignored\n")
	if got := ourLines(raw); got != "" {
		t.Errorf("the host's own warnings survived: %q", got)
	}

	ours := "/tmp/x/holzkube-manager-host.service:50: Unknown key 'ProtectHom' in section [Service], ignoring.\n"
	if got := ourLines(append(raw, []byte(ours)...)); got != ours {
		t.Errorf("a line about our unit was lost or mixed: %q", got)
	}
}
