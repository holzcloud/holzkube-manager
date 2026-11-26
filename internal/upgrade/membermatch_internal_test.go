package upgrade

import (
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

// TestAMemberIsMatchedByThePeerAddressWhenTheNamesDiffer: the inventory's
// hostname is not the one etcd lists, and the address in the member's peer URL
// is the machine's.
func TestAMemberIsMatchedByThePeerAddressWhenTheNamesDiffer(t *testing.T) {
	t.Parallel()

	e := Member{Hostname: "cp-1", PeerURLs: []string{"https://192.168.1.10:2380"}}
	if !matchesMachine(e, model.Machine{ID: "m1", Hostname: "cp-1-old", Addr: "192.168.1.10"}) {
		t.Error("a member whose peer URL carries the machine's address was not matched")
	}
	if matchesMachine(e, model.Machine{ID: "m2", Hostname: "cp-2", Addr: "192.168.1.11"}) {
		t.Error("a member was matched to a machine with another name and another address")
	}
	if !matchesMachine(e, model.Machine{ID: "m1", Hostname: "CP-1"}) {
		t.Error("the hostname match is no longer case-insensitive")
	}
}
