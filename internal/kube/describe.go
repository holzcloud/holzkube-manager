package kube

import (
	"context"
	"errors"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

// Reading an object as YAML (2026-09-19).
//
// # Why this is in the reading tier and not the editing one
//
// `kubectl get -o yaml` is how anybody finds the field a screen does not show:
// a toleration, a node selector, an ownerReference, the annotation somebody's
// controller reads. A management product that renders twelve fields and hides
// the object cannot answer the thirteenth question, and the operator goes back
// to kubectl for it -- which is the failure mode this whole half exists to
// remove.
//
// It reads. Nothing here writes: editing an object is ApplyManifest, where the
// plan, the field manager and the conflict handling already live.
//
// # Secrets, and why this refuses rather than redacts
//
// A Secret's `data` is base64, not encryption. Rendering one as YAML puts the
// credential on the screen, in the browser's memory, and in whatever the
// operator pastes it into next.
//
// Redacting the values was the obvious alternative and it is worse: a screen
// that shows a Secret with `<redacted>` in it teaches that looking at Secrets
// here is safe, and the first time the redaction misses a field -- a new
// `stringData`, an annotation somebody stuffed a token into -- it is a
// credential on a screen that promised it was not. Refusing the kind outright
// has no such failure mode. The metadata is still shown, because "which keys
// does this Secret have" is a real question that does not need the values.
//
// A cluster's own RBAC is the other half of this, and it is not here yet: this
// product holds one powerful identity (ledger 151 and the impersonation work
// that has not been done), so the refusal is this product's own rather than the
// cluster's.

// ErrRefusedKind reports a kind this product will not render.
var ErrRefusedKind = errors.New("kube: this product does not show that kind")

// MaxDescribeBytes bounds one rendered object.
//
// A ConfigMap can hold a megabyte of someone's configuration file, and a
// rendered YAML of it is no more useful for being complete.
const MaxDescribeBytes = 256 << 10

// Described is one object as YAML.
type Described struct {
	APIVersion string `json:"api_version"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	YAML       string `json:"yaml"`

	// Truncated says the document was cut, so nobody reads a partial object as
	// a whole one and concludes a field is absent.
	Truncated bool `json:"truncated"`

	// Notice carries what was left out and why, when anything was.
	Notice string `json:"notice"`
}

// Describe renders one object as YAML.
//
// The kind is resolved through the cluster's own discovery, the same way a
// manifest's is, so this works for a CustomResourceDefinition this build has
// never heard of.
func (c *Client) Describe(ctx context.Context, apiVersion, kind, namespace, name string) (Described, error) {
	if strings.EqualFold(kind, "Secret") {
		return Described{}, fmt.Errorf(
			"%w: a Secret's data is base64, not encryption, so showing it here would put the "+
				"credential on the screen. Its metadata is on the object that uses it; the value "+
				"belongs in whatever created it", ErrRefusedKind)
	}

	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return Described{}, fmt.Errorf("%w: %q is not an apiVersion", ErrManifestInvalid, apiVersion)
	}

	mapper, err := c.restMapper()
	if err != nil {
		return Described{}, err
	}
	mapping, err := mapper.RESTMapping(schema.GroupKind{Group: gv.Group, Kind: kind}, gv.Version)
	if err != nil {
		return Described{}, fmt.Errorf("%w: this cluster does not have %s %s: %w",
			ErrManifestInvalid, apiVersion, kind, err)
	}

	if mapping.Scope.Name() != "namespace" {
		namespace = ""
	}
	object, err := c.dyn.Resource(mapping.Resource).Namespace(namespace).
		Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return Described{}, fmt.Errorf("%w: %s %s/%s", ErrNoSuchWorkload, kind, namespace, name)
		}
		return Described{}, fmt.Errorf("kube: reading %s %s/%s: %w", kind, namespace, name, err)
	}

	notice := tidy(object)

	body, err := yaml.Marshal(object.Object)
	if err != nil {
		return Described{}, fmt.Errorf("kube: rendering %s %s/%s: %w", kind, namespace, name, err)
	}

	out := Described{
		APIVersion: object.GetAPIVersion(),
		Kind:       object.GetKind(),
		Namespace:  object.GetNamespace(),
		Name:       object.GetName(),
		YAML:       string(body),
		Notice:     notice,
	}
	if len(body) > MaxDescribeBytes {
		out.YAML = string(body[:MaxDescribeBytes])
		out.Truncated = true
	}
	return out, nil
}

// tidy removes the two fields that are noise, and says it did.
//
// `managedFields` is usually longer than the object and is never what somebody
// opened this for; the last-applied annotation is a second copy of the whole
// object inside itself. Both are REMOVED rather than hidden by the screen,
// because a screen that hides them would still have sent them to the browser.
//
// The removal is announced, because an object silently missing fields is how
// somebody concludes a field is not set when it is.
func tidy(object *unstructured.Unstructured) string {
	var removed []string

	if len(object.GetManagedFields()) > 0 {
		object.SetManagedFields(nil)
		removed = append(removed, "managedFields")
	}
	annotations := object.GetAnnotations()
	if _, ok := annotations[corev1LastApplied]; ok {
		delete(annotations, corev1LastApplied)
		object.SetAnnotations(annotations)
		removed = append(removed, corev1LastApplied)
	}

	if len(removed) == 0 {
		return ""
	}
	return "Left out, because they are bookkeeping rather than configuration: " +
		strings.Join(removed, ", ") + "."
}

// corev1LastApplied is kubectl's own annotation, which holds a second copy of
// the entire object.
const corev1LastApplied = "kubectl.kubernetes.io/last-applied-configuration"
