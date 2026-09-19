package kube

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/restmapper"
	"sigs.k8s.io/yaml"
)

// Applying manifests (milestone v1.17, slice 5).
//
// # The plan comes first, and it is not a courtesy
//
// An apply that runs without a preview is the same genus as a reset without a
// confirmation: the operator finds out what it did afterwards. So this package
// answers two questions separately -- "what would this change" and "do it" --
// and the screen has to ask the first before it can ask the second.
//
// The plan is computed by asking the cluster, one object at a time, whether it
// exists. That is what makes the difference between "create" and "update"
// truthful; guessing it from the manifest would be a guess about the cluster.
//
// # Server-side apply, and why it matters here
//
// A field this product sets is a field this product owns, recorded by the API
// server under a field manager. That is what keeps an apply from silently
// taking over a value somebody else's controller is managing: a conflict comes
// back as a conflict rather than as a quiet overwrite. The field manager is
// this product's own name, so `kubectl get -o yaml --show-managed-fields` says
// who set what.
//
// # What this deliberately does not do
//
// It does not delete. `kubectl apply --prune` decides what to remove by
// comparing against a previous apply, and getting that wrong deletes things
// nobody asked about. Removing an object is a separate operation, and it is not
// in this slice.

// ErrManifestInvalid reports a document this build cannot apply.
var ErrManifestInvalid = errors.New("kube: the manifest cannot be applied")

// ErrManifestConflict reports a field somebody else owns.
var ErrManifestConflict = errors.New("kube: another field manager owns a field this apply sets")

// FieldManager is the name the API server records against every field this
// product sets.
//
// One constant, and it is this product's own name: the whole value of
// server-side apply is that the cluster can say who set a field, and a manager
// called "application/apply-patch" would say nothing.
const FieldManager = "holzkube-manager"

// MaxManifestObjects bounds how many objects one manifest may carry.
//
// Not tidiness: the plan asks the cluster about every object and the apply
// writes every object, both serially, so the number of documents in a pasted
// file decides how much upstream work one request does. Without a cap the
// route's cost is whatever somebody pasted, which is the kind of thing that
// looks fine until a manifest with five thousand documents holds a request open
// for its whole budget.
//
// The number is well above real manifests -- a rendered chart with its
// CustomResourceDefinitions is tens of objects, not hundreds -- so the refusal
// means "that is not a manifest somebody wrote" rather than "your chart is too
// big".
const MaxManifestObjects = 256

// ManifestObject is one document in a manifest, as the plan describes it.
type ManifestObject struct {
	APIVersion string `json:"api_version"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`

	// Action is what applying would do: "create" when the cluster does not
	// have it, "update" when it does. It comes from asking the cluster rather
	// than from reading the manifest.
	Action string `json:"action"`

	// Namespaced records whether this kind lives in a namespace, because a
	// manifest that names no namespace for a namespaced object is a manifest
	// that would land in "default" -- which is rarely what somebody meant.
	Namespaced bool `json:"namespaced"`
}

// ManifestPlan is what an apply would do.
type ManifestPlan struct {
	Objects []ManifestObject `json:"objects"`

	// Warnings are things that are true and worth reading before applying:
	// an object with no namespace that needs one, a kind the cluster does not
	// have.
	Warnings []string `json:"warnings"`
}

// ApplyResult is what an apply did, per object.
type ApplyResult struct {
	Applied []ManifestObject `json:"applied"`

	// Failed carries the objects that did not apply, with the reason. An apply
	// of ten objects where the sixth fails has changed five things, and a
	// result that only said "failed" would leave the operator guessing which.
	Failed []FailedObject `json:"failed"`
}

// FailedObject is one object an apply could not write, and why.
type FailedObject struct {
	Object ManifestObject `json:"object"`
	Reason string         `json:"reason"`
}

// PlanManifest says what applying a manifest would do, and changes nothing.
func (c *Client) PlanManifest(ctx context.Context, manifest []byte) (ManifestPlan, error) {
	docs, err := splitManifest(manifest)
	if err != nil {
		return ManifestPlan{}, err
	}

	mapper, err := c.restMapper()
	if err != nil {
		return ManifestPlan{}, err
	}

	var plan ManifestPlan
	for _, doc := range docs {
		object, resource, namespaced, err := c.resolve(doc, mapper)
		if err != nil {
			plan.Warnings = append(plan.Warnings, err.Error())
			continue
		}

		row := ManifestObject{
			APIVersion: doc.GetAPIVersion(),
			Kind:       doc.GetKind(),
			Namespace:  object.GetNamespace(),
			Name:       object.GetName(),
			Namespaced: namespaced,
			Action:     "create",
		}
		if namespaced && doc.GetNamespace() == "" {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf(
				"%s %q names no namespace, so it would be applied to %q",
				doc.GetKind(), doc.GetName(), object.GetNamespace()))
		}

		existing, err := c.dyn.Resource(resource).Namespace(object.GetNamespace()).
			Get(ctx, object.GetName(), metav1.GetOptions{})
		switch {
		case err == nil && existing != nil:
			row.Action = "update"
		case apierrors.IsNotFound(err):
			// Stays "create", which is the honest answer and the reason the
			// cluster is asked at all.
		case err != nil:
			plan.Warnings = append(plan.Warnings, fmt.Sprintf(
				"%s %q could not be looked up: %v", doc.GetKind(), doc.GetName(), err))
		}

		plan.Objects = append(plan.Objects, row)
	}
	return plan, nil
}

// ApplyManifest applies a manifest with server-side apply.
//
// Every object is attempted, and the ones that fail are reported with their
// reason rather than stopping the rest: an apply of ten objects that gave up on
// the sixth would leave five applied and nothing said about which.
func (c *Client) ApplyManifest(ctx context.Context, manifest []byte) (ApplyResult, error) {
	docs, err := splitManifest(manifest)
	if err != nil {
		return ApplyResult{}, err
	}

	mapper, err := c.restMapper()
	if err != nil {
		return ApplyResult{}, err
	}

	var result ApplyResult
	for _, doc := range docs {
		object, resource, namespaced, err := c.resolve(doc, mapper)
		row := ManifestObject{
			APIVersion: doc.GetAPIVersion(),
			Kind:       doc.GetKind(),
			Name:       doc.GetName(),
			Namespaced: namespaced,
			Action:     "apply",
		}
		if err != nil {
			result.Failed = append(result.Failed, FailedObject{Object: row, Reason: err.Error()})
			continue
		}
		row.Namespace = object.GetNamespace()

		applied, err := c.dyn.Resource(resource).Namespace(object.GetNamespace()).
			Apply(ctx, object.GetName(), object, metav1.ApplyOptions{FieldManager: FieldManager})
		switch {
		case err == nil:
			row.Action = "applied"
			if applied != nil {
				row.Name = applied.GetName()
			}
			result.Applied = append(result.Applied, row)
		case apierrors.IsConflict(err):
			// A field another manager owns. Reported rather than forced,
			// because forcing means taking a value away from whatever is
			// managing it -- usually a controller that will set it back, which
			// is a fight this product must not start on its own.
			result.Failed = append(result.Failed, FailedObject{
				Object: row,
				Reason: fmt.Sprintf("%v: another field manager owns a field this object sets. "+
					"Whatever manages it will set it back, so this is a decision rather than a "+
					"retry", err),
			})
		default:
			result.Failed = append(result.Failed, FailedObject{Object: row, Reason: err.Error()})
		}
	}

	if len(result.Failed) > 0 {
		return result, fmt.Errorf("%w: %d of %d objects did not apply",
			ErrManifestInvalid, len(result.Failed), len(docs))
	}
	return result, nil
}

// resolve turns one document into the object to send and the resource to send
// it to.
//
// The mapping from a kind to a resource comes from the cluster's own discovery,
// not from a table in this binary: a cluster with a CustomResourceDefinition
// has kinds this build has never heard of, and refusing them would make this
// useless for exactly the manifests people actually apply.
func (c *Client) resolve(
	doc *unstructured.Unstructured, mapper *restmapper.DeferredDiscoveryRESTMapper,
) (*unstructured.Unstructured, schema.GroupVersionResource, bool, error) {
	gvk := doc.GroupVersionKind()
	if gvk.Kind == "" || doc.GetName() == "" {
		return nil, schema.GroupVersionResource{}, false, fmt.Errorf(
			"%w: a document with no kind or no name", ErrManifestInvalid)
	}

	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, schema.GroupVersionResource{}, false, fmt.Errorf(
			"%w: this cluster does not have %s %s: %w",
			ErrManifestInvalid, doc.GetAPIVersion(), gvk.Kind, err)
	}

	namespaced := mapping.Scope.Name() == "namespace"
	object := doc.DeepCopy()
	if namespaced && object.GetNamespace() == "" {
		// Kubernetes would do this itself; doing it here is what lets the plan
		// say which namespace the object would land in, rather than leaving
		// the operator to find out.
		object.SetNamespace("default")
	}
	if !namespaced {
		object.SetNamespace("")
	}
	return object, mapping.Resource, namespaced, nil
}

// restMapper is the cluster's own kind-to-resource mapping.
func (c *Client) restMapper() (*restmapper.DeferredDiscoveryRESTMapper, error) {
	if c.mapper == nil {
		return nil, errors.New("kube: this client was built without discovery")
	}
	return c.mapper, nil
}

// splitManifest reads a multi-document YAML manifest.
//
// Empty documents are skipped rather than refused: a manifest that ends with
// `---`, or one assembled by concatenating files, has them, and refusing would
// make this reject manifests every other tool accepts.
func splitManifest(manifest []byte) ([]*unstructured.Unstructured, error) {
	if len(bytes.TrimSpace(manifest)) == 0 {
		return nil, fmt.Errorf("%w: it is empty", ErrManifestInvalid)
	}

	var out []*unstructured.Unstructured
	for _, raw := range strings.Split(string(manifest), "\n---") {
		if strings.TrimSpace(raw) == "" {
			continue
		}

		converted, err := yaml.YAMLToJSON([]byte(raw))
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrManifestInvalid, err)
		}

		object := &unstructured.Unstructured{}
		if err := object.UnmarshalJSON(converted); err != nil {
			if errors.Is(err, io.EOF) {
				continue
			}
			return nil, fmt.Errorf("%w: %w", ErrManifestInvalid, err)
		}
		if len(object.Object) == 0 {
			continue
		}
		out = append(out, object)
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("%w: it contains no objects", ErrManifestInvalid)
	}
	if len(out) > MaxManifestObjects {
		return nil, fmt.Errorf("%w: it carries %d objects and this route applies at most %d. "+
			"Apply it in parts, or from a machine with kubectl",
			ErrManifestInvalid, len(out), MaxManifestObjects)
	}
	return out, nil
}
