package kube_test

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
)

// Storage (2026-09-20).
//
// Every claim here is a DERIVED one: something two lists disagreeing says that
// neither list says alone. That is the whole reason this exists beside the claim
// list the resources screen already had.

func storageCluster(t *testing.T) (*kubesim.Server, *kube.Client) {
	t.Helper()

	return newCluster(t, kubesim.Options{
		StorageClasses: []kubesim.StorageClass{
			{
				Name: "longhorn", Provisioner: "driver.longhorn.io", Default: true,
				ReclaimPolicy: corev1.PersistentVolumeReclaimDelete, AllowsExpansion: true,
			},
			{
				// The one that makes a claim sit Pending for a correct reason.
				Name: "local-path", Provisioner: "rancher.io/local-path",
				ReclaimPolicy: corev1.PersistentVolumeReclaimDelete, WaitForConsumer: true,
			},
		},
		Volumes: []kubesim.Volume{
			{
				Name: "pvc-bound", Capacity: "20Gi", Phase: corev1.VolumeBound,
				Claim: "db/postgres-data", StorageClass: "longhorn",
				ReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
				AccessModes:   []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				CSIDriver:     "driver.longhorn.io",
			},
			{
				// The case this screen exists for: data on the disk, no claim.
				Name: "pvc-orphan", Capacity: "100Gi", Phase: corev1.VolumeReleased,
				StorageClass: "longhorn", ReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				CSIDriver:   "driver.longhorn.io",
			},
		},
		Resources: []kubesim.Resource{
			{
				Kind: "PersistentVolumeClaim", Namespace: "db", Name: "postgres-data",
				Phase: string(corev1.ClaimBound), Size: "20Gi",
				StorageClass: "longhorn", VolumeName: "pvc-bound",
			},
			{
				// Bound and mounted by nothing: storage being paid for.
				Kind: "PersistentVolumeClaim", Namespace: "default", Name: "forgotten",
				Phase: string(corev1.ClaimBound), Size: "5Gi", StorageClass: "longhorn",
			},
			{
				Kind: "PersistentVolumeClaim", Namespace: "default", Name: "waiting",
				Phase: string(corev1.ClaimPending), Size: "1Gi", StorageClass: "local-path",
			},
			{
				Kind: "PersistentVolumeClaim", Namespace: "default", Name: "typo",
				Phase: string(corev1.ClaimPending), Size: "1Gi", StorageClass: "longhron",
			},
		},
		Pods: []kubesim.Pod{
			{Namespace: "db", Name: "postgres-0", Phase: corev1.PodRunning, Claims: []string{"postgres-data"}},
			// A finished pod mounts nothing any more.
			{Namespace: "db", Name: "restore-once", Phase: corev1.PodSucceeded, Claims: []string{"postgres-data"}},
		},
	})
}

// TestAVolumeHoldingDataNobodyClaimsIsNamed.
//
// It is invisible in every namespace view -- a PersistentVolume is
// cluster-scoped -- it counts against nothing, and it is either wasted storage
// or exactly the data somebody wants back. Both readings have to be said.
func TestAVolumeHoldingDataNobodyClaimsIsNamed(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := storageCluster(t)

	storage, err := client.Storage(ctx, "")
	if err != nil {
		t.Fatalf("Storage: %v", err)
	}

	var orphan *kube.VolumeSummary
	for i := range storage.Volumes {
		if storage.Volumes[i].Name == "pvc-orphan" {
			orphan = &storage.Volumes[i]
		}
	}
	if orphan == nil {
		t.Fatal("the released volume is not in the answer at all")
	}
	if orphan.Claim != "" {
		t.Errorf("claim = %q, want nothing holding it", orphan.Claim)
	}
	// Retain is the field that decides whether the data survives, and it is on
	// the volume rather than on any claim.
	if orphan.ReclaimPolicy != "Retain" {
		t.Errorf("reclaim policy = %q, want Retain", orphan.ReclaimPolicy)
	}
	if !strings.Contains(orphan.Notice, "data is still on the disk") {
		t.Errorf("notice = %q, want it to say the data is still there", orphan.Notice)
	}
}

// TestAClaimSaysWhoIsUsingIt, which its own object does not know.
func TestAClaimSaysWhoIsUsingIt(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := storageCluster(t)

	storage, err := client.Storage(ctx, "")
	if err != nil {
		t.Fatalf("Storage: %v", err)
	}

	byName := map[string]kube.ClaimSummary{}
	for _, claim := range storage.Claims {
		byName[claim.Name] = claim
	}

	// One mounter, not two: the Succeeded pod mounts nothing any more, and
	// listing it is how somebody decides a claim is in use when it is not.
	used := byName["postgres-data"].UsedBy
	if len(used) != 1 || used[0] != "db/postgres-0" {
		t.Errorf("used by %v, want only the running pod", used)
	}

	// Bound and mounted by nothing is a real answer, and the interesting one.
	forgotten := byName["forgotten"]
	if len(forgotten.UsedBy) != 0 {
		t.Errorf("forgotten is used by %v, want nothing", forgotten.UsedBy)
	}
	if !strings.Contains(forgotten.Notice, "no running pod mounts it") {
		t.Errorf("notice = %q, want it to say nothing mounts it", forgotten.Notice)
	}
}

// TestAPendingClaimIsExplainedRatherThanReported.
//
// The claim's own events say "unbound immediate PersistentVolumeClaim", which
// tells somebody it is not bound -- the thing they already know. The two causes
// here are opposite: one is a class working as configured, the other a name that
// does not exist.
func TestAPendingClaimIsExplainedRatherThanReported(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := storageCluster(t)

	storage, err := client.Storage(ctx, "")
	if err != nil {
		t.Fatalf("Storage: %v", err)
	}

	byName := map[string]kube.ClaimSummary{}
	for _, claim := range storage.Claims {
		byName[claim.Name] = claim
	}

	// WaitForFirstConsumer: correct behaviour that looks exactly like a broken
	// provisioner, and the most misread Pending in Kubernetes.
	waiting := byName["waiting"].Notice
	if !strings.Contains(waiting, "not a fault") {
		t.Errorf("notice = %q, want it to say the class is working as configured", waiting)
	}

	// A class that does not exist will never provision anything, and no amount
	// of waiting changes that.
	typo := byName["typo"].Notice
	if !strings.Contains(typo, "does not exist") {
		t.Errorf("notice = %q, want it to say the class is not in this cluster", typo)
	}
}

// TestWhetherAClaimCanBeGrownComesFromItsClass, so a screen does not offer an
// edit the provisioner refuses.
func TestWhetherAClaimCanBeGrownComesFromItsClass(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := storageCluster(t)

	storage, err := client.Storage(ctx, "")
	if err != nil {
		t.Fatalf("Storage: %v", err)
	}

	byName := map[string]kube.ClaimSummary{}
	for _, claim := range storage.Claims {
		byName[claim.Name] = claim
	}
	if !byName["postgres-data"].Expandable {
		t.Error("a claim on a class that allows expansion is not reported as expandable")
	}
	if byName["waiting"].Expandable {
		t.Error("a claim on a class that does not allow expansion is reported as expandable")
	}
}

// TestACaptureIsNotHowFullTheFilesystemIs, said in words.
//
// Nothing in the Kubernetes API reports filesystem usage -- only something
// running inside the pod can -- and a screen headed "20Gi" invites exactly that
// reading.
func TestACaptureIsNotHowFullTheFilesystemIs(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := storageCluster(t)

	storage, err := client.Storage(ctx, "")
	if err != nil {
		t.Fatalf("Storage: %v", err)
	}
	if !strings.Contains(storage.Notice, "not how full") {
		t.Errorf("notice = %q, want it to say what a capacity is not", storage.Notice)
	}
}

// TestTwoDefaultStorageClassesAreReported: the API server picks one arbitrarily,
// which is not a choice anybody made.
func TestTwoDefaultStorageClassesAreReported(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{StorageClasses: []kubesim.StorageClass{
		{Name: "longhorn", Provisioner: "driver.longhorn.io", Default: true},
		{Name: "local-path", Provisioner: "rancher.io/local-path", Default: true},
	}})

	storage, err := client.Storage(ctx, "")
	if err != nil {
		t.Fatalf("Storage: %v", err)
	}
	if !strings.Contains(storage.Notice, "marked default") {
		t.Errorf("notice = %q, want it to report two defaults", storage.Notice)
	}
}

// TestNoDefaultStorageClassIsReported, because a claim naming none then stays
// Pending for ever rather than failing -- which nothing else will tell anybody.
func TestNoDefaultStorageClassIsReported(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{StorageClasses: []kubesim.StorageClass{
		{Name: "longhorn", Provisioner: "driver.longhorn.io"},
	}})

	storage, err := client.Storage(ctx, "")
	if err != nil {
		t.Fatalf("Storage: %v", err)
	}
	if !strings.Contains(storage.Notice, "No storage class is marked default") {
		t.Errorf("notice = %q, want it to say no class is default", storage.Notice)
	}
}

// TestVolumesAreNotHiddenByANamespace.
//
// A PersistentVolume is cluster-scoped. Narrowing them to a namespace is how a
// Released volume holding somebody's database stays invisible.
func TestVolumesAreNotHiddenByANamespace(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := storageCluster(t)

	storage, err := client.Storage(ctx, "default")
	if err != nil {
		t.Fatalf("Storage: %v", err)
	}
	if len(storage.Volumes) != 2 {
		t.Errorf("volumes = %d, want both of them even in a namespace view", len(storage.Volumes))
	}
	if len(storage.Classes) != 2 {
		t.Errorf("classes = %d, want both", len(storage.Classes))
	}
	// The claims ARE narrowed, which is what a namespace view is for.
	for _, claim := range storage.Claims {
		if claim.Namespace != "default" {
			t.Errorf("claim %s/%s is in the default-namespace answer", claim.Namespace, claim.Name)
		}
	}
}
