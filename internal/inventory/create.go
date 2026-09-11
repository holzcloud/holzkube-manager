package inventory

import (
	"context"
	"fmt"
	"strings"

	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// Creating a cluster is the *other* path, and it is the only one that mints
// certificate authorities (D-07, INV-02).
//
// The separation is the point. Import derives a bundle from a running node and
// refuses outright if it cannot; create generates one because there is nothing
// to derive from. What must never exist is a bridge between them: an import
// that generated a bundle when the derivation failed would produce a cluster
// record that looks exactly like the real one, is accepted everywhere in the
// UI, and cannot enter a single one of that cluster's nodes.
//
// The separation is held by a test rather than by discipline --
// TestTheAdoptionPathCannotGenerateSecrets walks the adoption source and fails
// on a call to any of the generators -- and that test is why this lives in its
// own file.
//
// A created cluster has no nodes yet. Provisioning them is phase 8, which is
// also where this bundle's real purpose appears: it is what a joining node's
// machine configuration is built from.

// CreateRequest is what the create route accepts.
type CreateRequest struct {
	// Name is the operator's label for the cluster.
	Name string

	// Endpoint is the Kubernetes API endpoint the cluster will present, in the
	// https://host:port form. It is part of the generated configuration and is
	// therefore decided here rather than at first provisioning.
	Endpoint string
}

// Create mints a new cluster's PKI.
//
// Unlike an imported cluster, a created one is **not** locked (D-21). The
// asymmetry is deliberate and it is about what the operator is entitled to
// assume: an imported cluster existed and was depended on before holzkube-manager saw
// it, while a created one did not exist a second ago and there is nothing yet
// to protect from a mistake.
func (s *Service) Create(ctx context.Context, req CreateRequest) (model.Cluster, error) {
	if strings.TrimSpace(req.Name) == "" {
		return model.Cluster{}, fmt.Errorf("inventory: a cluster needs a name")
	}
	if strings.TrimSpace(req.Endpoint) == "" {
		return model.Cluster{}, fmt.Errorf("inventory: a cluster needs a Kubernetes endpoint")
	}

	// The system clock, not a fixed one: the certificate authority's validity
	// window is computed from it, and a bundle minted at a frozen date would
	// issue certificates that are already expired.
	bundle, err := secrets.NewBundle(secrets.NewClock(), nil)
	if err != nil {
		return model.Cluster{}, fmt.Errorf("inventory: generate cluster secrets: %w", err)
	}

	clientCrt, clientKey, notAfter, err := mintClientCert(bundle)
	if err != nil {
		return model.Cluster{}, err
	}

	id, err := newClusterID()
	if err != nil {
		return model.Cluster{}, err
	}

	now := s.deps.Now().UTC()
	cluster := model.Cluster{
		ID:                 id,
		Name:               strings.TrimSpace(req.Name),
		Origin:             model.OriginCreated,
		Endpoint:           req.Endpoint,
		Locked:             false,
		ClientCertNotAfter: notAfter,
		CreatedAt:          now,
	}

	if _, err := s.deps.Store.ClusterSecrets().Put(ctx,
		secretsRecord(id, bundle, clientCrt, clientKey)); err != nil {
		return model.Cluster{}, err
	}

	stored, err := s.deps.Store.Clusters().Put(ctx, cluster)
	if err != nil {
		// Same invariant as the adoption path: no stored bundle without its
		// cluster. An orphaned bundle is cluster PKI on disk that nothing
		// refers to and nothing will ever clean up.
		if delErr := s.deps.Store.ClusterSecrets().Delete(ctx, id); delErr != nil {
			return model.Cluster{}, fmt.Errorf(
				"inventory: storing the cluster failed (%w) and its secrets could not be removed: %w",
				err, delErr)
		}
		return model.Cluster{}, err
	}
	return stored, nil
}

// Talosconfig renders an admin client configuration for a cluster, so the
// operator can reach it with talosctl as well.
//
// It is minted fresh from the stored bundle rather than handing out the
// certificate holzkube-manager dials with. Two consumers sharing one credential means
// revoking either revokes both, and holzkube-manager's own access is the one that must
// not depend on what the operator did with a file.
func (s *Service) Talosconfig(ctx context.Context, id model.ClusterID) ([]byte, error) {
	sec, err := s.deps.Store.ClusterSecrets().Get(ctx, id)
	if err != nil {
		return nil, err
	}
	cluster, err := s.deps.Store.Clusters().Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return talos.RenderTalosconfig(cluster.Name, cluster.Endpoint, sec.OSCACrt, sec.OSCAKey, ClientCertTTL)
}
