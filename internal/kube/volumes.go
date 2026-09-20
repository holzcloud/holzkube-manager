package kube

import (
	"context"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Storage, as the thing it actually is (2026-09-20).
//
// # Why the claim list was not enough
//
// The resources screen lists PersistentVolumeClaims, which is one half of a
// two-sided arrangement, and the half that cannot answer the questions somebody
// has when storage is wrong:
//
//   - "Is this data still there?" A claim says Bound. Whether the VOLUME it is
//     bound to has `Retain` or `Delete` as its reclaim policy is what decides
//     whether deleting the claim destroys the data, and that is on the volume.
//   - "Who is using this?" A claim does not know. The pods know, and finding out
//     otherwise means reading every pod's volume list by hand.
//   - "Why is this claim Pending?" Usually because no StorageClass matched, or
//     the default one is not what somebody assumed, or `WaitForFirstConsumer`
//     means it is waiting for a pod rather than broken. All three are facts
//     about classes, not claims.
//   - "Can I grow it?" `allowVolumeExpansion` on the class, and nowhere else.
//
// # Released is the state worth a screen of its own
//
// A `Released` volume is one whose claim is gone and whose data is still on the
// disk, held because the reclaim policy said `Retain`. It is invisible in every
// namespace view -- a PersistentVolume is cluster-scoped -- it counts against
// nothing, and it is the single most common way a Talos cluster quietly fills
// its storage backend. It is also, occasionally, exactly the data somebody needs
// back. So it is named, with both readings said.
//
// # Nothing here deletes
//
// Deleting a PersistentVolume with `Retain` is how data goes away for good, and
// the existing object-delete route already does it for anybody who means it,
// with the name typed out. A "clean up volumes" button would be the one button
// in this product whose mistake cannot be undone at all, so there isn't one.

// VolumeSummary is one PersistentVolume, in the fields that decide whether the
// data survives.
type VolumeSummary struct {
	Name     string `json:"name"`
	Capacity string `json:"capacity"`

	// Phase is Bound, Available, Released or Failed.
	Phase string `json:"phase"`

	// Claim is "namespace/name", or empty when nothing holds it.
	Claim string `json:"claim"`

	StorageClass string `json:"storage_class"`

	// ReclaimPolicy is what happens to the data when the claim goes away:
	// Delete destroys it, Retain keeps it. This is the field somebody needs
	// before deleting anything and the one no claim can tell them.
	ReclaimPolicy string `json:"reclaim_policy"`

	// AccessModes in Kubernetes's own short words (RWO, RWX, ROX), because a
	// volume that cannot be mounted twice is why a Deployment will not roll.
	AccessModes []string `json:"access_modes"`

	// Driver is the CSI driver's name, or the in-tree kind for a local volume.
	Driver string `json:"driver"`

	// Notice is said only when this volume is something to look at: data held
	// with no claim, or a volume that failed.
	Notice    string `json:"notice"`
	CreatedAt string `json:"created_at"`
}

// ClaimSummary is one PersistentVolumeClaim with the two things its own object
// does not say: what it is bound to, and who is using it.
type ClaimSummary struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`

	Phase        string   `json:"phase"`
	Requested    string   `json:"requested"`
	Capacity     string   `json:"capacity"`
	StorageClass string   `json:"storage_class"`
	Volume       string   `json:"volume"`
	AccessModes  []string `json:"access_modes"`

	// UsedBy is every pod that mounts this claim, "namespace/name". Empty is a
	// real answer and an interesting one: a bound claim nothing mounts is
	// storage being paid for and not used.
	UsedBy []string `json:"used_by"`

	// Expandable says whether its class allows growing it, so a screen does not
	// offer an edit the provisioner refuses.
	Expandable bool `json:"expandable"`

	// Notice explains a Pending claim in words, because the claim's own events
	// say "unbound immediate PersistentVolumeClaim" and nothing else.
	Notice    string `json:"notice"`
	CreatedAt string `json:"created_at"`
}

// StorageClassSummary is one class, and whether it is the one a claim with no
// class named gets.
type StorageClassSummary struct {
	Name        string `json:"name"`
	Provisioner string `json:"provisioner"`

	// Default is the class a claim that names none will use. More than one
	// marked default is a misconfiguration the cluster resolves arbitrarily,
	// which is why the notice counts them.
	Default bool `json:"default"`

	ReclaimPolicy string `json:"reclaim_policy"`

	// BindingMode is Immediate or WaitForFirstConsumer. The second one makes a
	// claim sit Pending until a pod schedules, which is correct behaviour that
	// looks exactly like a broken provisioner.
	BindingMode string `json:"binding_mode"`

	AllowsExpansion bool   `json:"allows_expansion"`
	CreatedAt       string `json:"created_at"`
}

// Storage is everything about the cluster's storage, on one answer.
type Storage struct {
	Volumes []VolumeSummary       `json:"volumes"`
	Claims  []ClaimSummary        `json:"claims"`
	Classes []StorageClassSummary `json:"classes"`

	// Notice says what the numbers here are and are not -- a capacity is what
	// was PROVISIONED, never how full the filesystem on it is.
	Notice string `json:"notice"`
}

// Storage reads the cluster's volumes, claims and classes.
//
// The namespace narrows the CLAIMS only. Volumes and classes are cluster-scoped,
// and hiding them in a namespace view is how a Released volume holding somebody's
// database stays invisible.
func (c *Client) Storage(ctx context.Context, namespace string) (Storage, error) {
	out := Storage{
		Notice: "A capacity here is what was provisioned, not how full the filesystem on it is: " +
			"nothing in the Kubernetes API reports that, and only something running inside the pod " +
			"can. A volume's reclaim policy is what decides whether deleting its claim destroys the " +
			"data.",
	}

	classes, err := c.cs.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return Storage{}, fmt.Errorf("kube: listing storage classes: %w", err)
	}
	expandable := map[string]bool{}
	defaults := 0
	for _, class := range classes.Items {
		row := StorageClassSummary{
			Name:            class.Name,
			Provisioner:     class.Provisioner,
			Default:         class.Annotations["storageclass.kubernetes.io/is-default-class"] == "true",
			AllowsExpansion: class.AllowVolumeExpansion != nil && *class.AllowVolumeExpansion,
			CreatedAt:       stamp(class.CreationTimestamp),
		}
		if class.ReclaimPolicy != nil {
			row.ReclaimPolicy = string(*class.ReclaimPolicy)
		}
		if class.VolumeBindingMode != nil {
			row.BindingMode = string(*class.VolumeBindingMode)
		}
		if row.Default {
			defaults++
		}
		expandable[class.Name] = row.AllowsExpansion
		out.Classes = append(out.Classes, row)
	}
	if defaults > 1 {
		out.Notice += fmt.Sprintf(" %d storage classes are marked default; a claim that names no "+
			"class gets whichever one the API server picks, which is not a choice anybody made.",
			defaults)
	}
	if defaults == 0 && len(classes.Items) > 0 {
		out.Notice += " No storage class is marked default, so a claim that names none will stay " +
			"Pending for ever rather than fail."
	}

	volumes, err := c.cs.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return Storage{}, fmt.Errorf("kube: listing persistent volumes: %w", err)
	}
	for _, volume := range volumes.Items {
		row := VolumeSummary{
			Name:          volume.Name,
			Capacity:      quantityOf(volume.Spec.Capacity, corev1.ResourceStorage),
			Phase:         string(volume.Status.Phase),
			StorageClass:  volume.Spec.StorageClassName,
			ReclaimPolicy: string(volume.Spec.PersistentVolumeReclaimPolicy),
			AccessModes:   shortAccessModes(volume.Spec.AccessModes),
			Driver:        driverOf(volume.Spec.PersistentVolumeSource),
			CreatedAt:     stamp(volume.CreationTimestamp),
		}
		if ref := volume.Spec.ClaimRef; ref != nil {
			row.Claim = ref.Namespace + "/" + ref.Name
		}
		switch volume.Status.Phase {
		case corev1.VolumeReleased:
			// The case this screen exists for. Both readings, because it is
			// either wasted storage or exactly the data somebody wants back.
			row.Notice = "Its claim is gone and the data is still on the disk, kept because the " +
				"reclaim policy is Retain. Nothing counts it and nothing will use it until " +
				"somebody either makes a claim for it or deletes it."
		case corev1.VolumeFailed:
			row.Notice = "The provisioner could not reclaim it. The data is still there and no " +
				"claim can bind to it; " + volume.Status.Message
		case corev1.VolumeAvailable:
			row.Notice = "Nothing holds it. A claim asking for this class and size will bind to it."
		}
		out.Volumes = append(out.Volumes, row)
	}

	claims, err := c.cs.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return Storage{}, fmt.Errorf("kube: listing claims: %w", err)
	}

	// Who mounts what. One pod list, because the alternative is asking the
	// question once per claim and a cluster has more claims than it has pods.
	pods, err := c.cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return Storage{}, fmt.Errorf("kube: listing pods: %w", err)
	}
	mounters := map[string][]string{}
	for _, pod := range pods.Items {
		// A finished pod mounts nothing any more, and listing it as a user of a
		// claim is how somebody decides a claim is in use when it is not.
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		for _, volume := range pod.Spec.Volumes {
			if volume.PersistentVolumeClaim == nil {
				continue
			}
			key := pod.Namespace + "/" + volume.PersistentVolumeClaim.ClaimName
			mounters[key] = append(mounters[key], pod.Namespace+"/"+pod.Name)
		}
	}

	for _, claim := range claims.Items {
		row := ClaimSummary{
			Namespace:   claim.Namespace,
			Name:        claim.Name,
			Phase:       string(claim.Status.Phase),
			Requested:   quantityOf(claim.Spec.Resources.Requests, corev1.ResourceStorage),
			Capacity:    quantityOf(claim.Status.Capacity, corev1.ResourceStorage),
			Volume:      claim.Spec.VolumeName,
			AccessModes: shortAccessModes(claim.Spec.AccessModes),
			UsedBy:      mounters[claim.Namespace+"/"+claim.Name],
			CreatedAt:   stamp(claim.CreationTimestamp),
		}
		if claim.Spec.StorageClassName != nil {
			row.StorageClass = *claim.Spec.StorageClassName
		}
		row.Expandable = expandable[row.StorageClass]
		sort.Strings(row.UsedBy)

		if claim.Status.Phase == corev1.ClaimPending {
			row.Notice = pendingClaimReason(row.StorageClass, classes.Items)
		}
		if claim.Status.Phase == corev1.ClaimBound && len(row.UsedBy) == 0 {
			// Not a fault, and worth saying: this is storage being paid for and
			// not used, and it is how a cluster's disk bill grows quietly.
			row.Notice = "Bound, and no running pod mounts it."
		}
		out.Claims = append(out.Claims, row)
	}

	return out, nil
}

// pendingClaimReason explains a Pending claim in words.
//
// The claim's own events say "unbound immediate PersistentVolumeClaim", which
// tells somebody that it is not bound -- the thing they already know. The three
// real causes are all facts about classes, and two of them are not faults at all.
func pendingClaimReason(class string, classes []storagev1.StorageClass) string {
	if class == "" {
		for _, candidate := range classes {
			if candidate.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
				return "It names no storage class, so it is waiting for the default one (" +
					candidate.Name + ") to provision it."
			}
		}
		return "It names no storage class and no class is marked default, so nothing will ever " +
			"provision it. Either name a class on the claim or mark one default."
	}
	for _, candidate := range classes {
		if candidate.Name != class {
			continue
		}
		if candidate.VolumeBindingMode != nil &&
			*candidate.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer {
			// Correct behaviour that looks exactly like a broken provisioner,
			// and the most misread Pending in Kubernetes.
			return "Its class waits for a consumer, so it stays Pending until a pod that uses it " +
				"is scheduled. That is the class working as configured, not a fault."
		}
		return "Its class (" + class + ") has not provisioned it yet. If it stays here, the " +
			"provisioner " + candidate.Provisioner + " is the thing to look at."
	}
	return "Its class (" + class + ") does not exist in this cluster, so nothing will provision " +
		"it. The name is probably a typo, or the class was removed after the claim was written."
}

// shortAccessModes uses Kubernetes's own abbreviations.
//
// RWO and RWX are the distinction that decides whether a Deployment can roll --
// a new pod cannot mount an RWO volume while the old one still holds it -- and
// the long names are what make that hard to see in a list.
func shortAccessModes(modes []corev1.PersistentVolumeAccessMode) []string {
	out := make([]string, 0, len(modes))
	for _, mode := range modes {
		switch mode {
		case corev1.ReadWriteOnce:
			out = append(out, "RWO")
		case corev1.ReadOnlyMany:
			out = append(out, "ROX")
		case corev1.ReadWriteMany:
			out = append(out, "RWX")
		case corev1.ReadWriteOncePod:
			out = append(out, "RWOP")
		default:
			out = append(out, string(mode))
		}
	}
	return out
}

// driverOf names what actually backs a volume.
//
// Only the sources a Talos cluster realistically has. Everything else answers
// its own kind rather than being guessed at, because "unknown" would hide a
// perfectly ordinary volume type this build has not been taught.
func driverOf(source corev1.PersistentVolumeSource) string {
	switch {
	case source.CSI != nil:
		return source.CSI.Driver
	case source.Local != nil:
		return "local (" + source.Local.Path + ")"
	case source.HostPath != nil:
		return "hostPath (" + source.HostPath.Path + ")"
	case source.NFS != nil:
		return "nfs (" + source.NFS.Server + ":" + source.NFS.Path + ")"
	case source.ISCSI != nil:
		return "iscsi"
	case source.FC != nil:
		return "fibre channel"
	default:
		return ""
	}
}

func quantityOf(list corev1.ResourceList, name corev1.ResourceName) string {
	if q, ok := list[name]; ok {
		return q.String()
	}
	return ""
}
