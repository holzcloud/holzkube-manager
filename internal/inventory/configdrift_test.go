package inventory_test

import (
	"testing"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// TestTheClusterViewCarriesWhatTheDriftFunctionGives: the view of a cluster
// carries what the function says for THAT cluster, in the list and for one
// cluster, and nothing of another cluster's answer.
func TestTheClusterViewCarriesWhatTheDriftFunctionGives(t *testing.T) {
	t.Parallel()

	f := newFixture(t, talossim.Options{ControlPlane: true})
	ctx := testContext(t)
	c := f.importCluster(ctx, t)

	other, err := f.store.Clusters().Put(ctx, model.Cluster{ID: "other", Name: "other", Origin: model.OriginImported})
	if err != nil {
		t.Fatalf("lay a second cluster: %v", err)
	}

	f.svc.SetConfigDrift(func(cl model.Cluster, now time.Time) *model.ConfigDriftHealth {
		if cl.ID != c.ID {
			return nil
		}
		return &model.ConfigDriftHealth{
			CheckedAt: now, Nodes: 7, ReadNodes: 5, Drifted: 2,
			Items: []model.NodeConfigDrift{{Node: "cp-1", State: "drift"}},
		}
	})

	all, err := f.svc.Clusters(ctx)
	if err != nil || len(all) != 2 {
		t.Fatalf("Clusters = %d entries, %v; want both clusters", len(all), err)
	}
	for _, v := range all {
		switch v.ID {
		case c.ID:
			if v.ConfigDrift == nil || v.ConfigDrift.Nodes != 7 || v.ConfigDrift.Drifted != 2 {
				t.Errorf("Clusters: the cluster the function answered for has %+v", v.ConfigDrift)
			}
		case other.ID:
			if v.ConfigDrift != nil {
				t.Errorf("Clusters: another cluster carries %+v, which the function did not give it", v.ConfigDrift)
			}
		default:
			t.Errorf("Clusters: unexpected cluster %s", v.ID)
		}
	}

	v, err := f.svc.Cluster(ctx, c.ID)
	if err != nil || v.ConfigDrift == nil || v.ConfigDrift.ReadNodes != 5 {
		t.Errorf("Cluster: %+v, %v; want the view the function gave", v.ConfigDrift, err)
	}
	v, err = f.svc.Cluster(ctx, other.ID)
	if err != nil || v.ConfigDrift != nil {
		t.Errorf("Cluster of the other: %+v, %v; want none", v.ConfigDrift, err)
	}

	f.svc.SetConfigDrift(nil)
	v, err = f.svc.Cluster(ctx, c.ID)
	if err != nil || v.ConfigDrift != nil {
		t.Errorf("after SetConfigDrift(nil) the view still carries %+v (%v)", v.ConfigDrift, err)
	}
}

// TestWithoutAFunctionThereIsNoConfigDrift: an inventory nobody has wired the
// reader to says nothing about drift, which the card shows as "not read yet".
func TestWithoutAFunctionThereIsNoConfigDrift(t *testing.T) {
	t.Parallel()

	f := newFixture(t, talossim.Options{ControlPlane: true})
	ctx := testContext(t)
	c := f.importCluster(ctx, t)

	all, err := f.svc.Clusters(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("Clusters = %d entries, %v", len(all), err)
	}
	if all[0].ConfigDrift != nil {
		t.Errorf("Clusters: ConfigDrift is %+v without a function", all[0].ConfigDrift)
	}
	v, err := f.svc.Cluster(ctx, c.ID)
	if err != nil || v.ConfigDrift != nil {
		t.Errorf("Cluster: ConfigDrift is %+v (%v) without a function", v.ConfigDrift, err)
	}
}
