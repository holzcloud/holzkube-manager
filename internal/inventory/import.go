package inventory

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// Adoption, end to end (D-01 .. D-07):
//
//	1. the operator uploads a talosconfig and names a control-plane address
//	2. holzkube-manager shows the node's server-certificate fingerprint and waits
//	3. it connects with the uploaded credentials and reads the node's own
//	   machine configuration
//	4. it derives the secrets bundle from it, refusing outright if the node is
//	   a worker
//	5. it mints itself a fresh client certificate from that bundle and
//	   reconnects under it -- the connectivity proof
//	6. only then does a cluster exist, read-only locked, with its members
//	   discovered
//
// Nothing is written before step 5 succeeds. A half-adopted cluster is a state
// this product refuses to have.

// ImportRequest is what the adoption route accepts.
type ImportRequest struct {
	// Name is the operator's label for the cluster.
	Name string

	// Talosconfig is the file's bytes, uploaded or pasted. It is never a path
	// on the server (D-06).
	Talosconfig []byte

	// Endpoint is the address of a control-plane node, without a port.
	Endpoint string

	// Fingerprint is the server-certificate fingerprint the operator
	// confirmed, in tlsx.Fingerprint's form. Adoption refuses to proceed
	// without it: a trusting connection made before anybody looked is the
	// thing step 2 exists to prevent.
	Fingerprint string
}

// Fingerprint returns the certificate fingerprint of a prospective node, for
// the operator to confirm before anything trusts it (D-03).
func (s *Service) Fingerprint(ctx context.Context, endpoint string) (string, error) {
	return talos.ServerFingerprint(ctx, s.deps.Dialer, talos.Target{
		Machine: model.MachineID("unadopted:" + endpoint),
		Addr:    endpoint,
	})
}

// Import adopts an existing cluster.
func (s *Service) Import(ctx context.Context, req ImportRequest) (model.Cluster, error) {
	if strings.TrimSpace(req.Name) == "" {
		return model.Cluster{}, fmt.Errorf("inventory: a cluster needs a name")
	}
	if strings.TrimSpace(req.Endpoint) == "" {
		return model.Cluster{}, fmt.Errorf("inventory: name the address of a control-plane node")
	}
	if strings.TrimSpace(req.Fingerprint) == "" {
		return model.Cluster{}, fmt.Errorf("inventory: confirm the node's certificate fingerprint first")
	}

	tc, err := talos.ParseTalosconfig(req.Talosconfig)
	if err != nil {
		return model.Cluster{}, err
	}
	uploaded, err := tc.Creds()
	if err != nil {
		return model.Cluster{}, err
	}

	seen, err := s.Fingerprint(ctx, req.Endpoint)
	if err != nil {
		return model.Cluster{}, err
	}
	if !strings.EqualFold(seen, req.Fingerprint) {
		return model.Cluster{}, fmt.Errorf("%w: %s presents %s", talos.ErrFingerprintMismatch, req.Endpoint, seen)
	}

	id, err := newClusterID()
	if err != nil {
		return model.Cluster{}, err
	}

	// A provisional target. The machine id is not known until the node has
	// been asked, and asking needs a target -- so the first connection is made
	// under the cluster id and the address, and every later one under the UUID
	// the node reported.
	provisional := talos.Target{
		Cluster: id,
		Machine: model.MachineID("adopting:" + req.Endpoint),
		Addr:    req.Endpoint,
	}

	first, err := talos.NewClusterClient(ctx, s.deps.Dialer, provisional, uploaded, s.deps.Mode)
	if err != nil {
		return model.Cluster{}, err
	}
	defer first.Close() //nolint:errcheck // the adoption's verdict is not a close error's to change

	// The one configuration read this phase performs. Its bytes hold the
	// cluster CA private key and three joining tokens; they go straight into
	// the derivation and are never returned, logged or stored as such.
	configYAML, err := first.MachineConfigYAML(ctx)
	if err != nil {
		return model.Cluster{}, err
	}

	bundle, err := deriveSecrets(configYAML)
	if err != nil {
		return model.Cluster{}, err
	}

	clientCrt, clientKey, notAfter, err := mintClientCert(bundle)
	if err != nil {
		return model.Cluster{}, err
	}

	// The connectivity proof (D-04). It uses the certificate holzkube-manager just
	// issued itself, not the uploaded talosconfig: a file that works today
	// says nothing about whether the bundle behind it can produce a working
	// certificate tomorrow, and tomorrow is when it matters.
	own, err := talos.ClusterCreds(bundle.Certs.OS.Crt, clientCrt, clientKey)
	if err != nil {
		return model.Cluster{}, err
	}
	own.Fingerprint = req.Fingerprint

	proof, err := talos.NewClusterClient(ctx, s.deps.Dialer, provisional, own, s.deps.Mode)
	if err != nil {
		return model.Cluster{}, fmt.Errorf(
			"inventory: the certificate derived from this cluster's own secrets could not reach %s: %w",
			req.Endpoint, err)
	}
	defer proof.Close() //nolint:errcheck // as above

	if _, err := proof.Version(ctx); err != nil {
		return model.Cluster{}, fmt.Errorf(
			"inventory: the certificate derived from this cluster's own secrets was refused by %s: %w",
			req.Endpoint, err)
	}

	// Everything below writes. Nothing above it did.
	now := s.deps.Now().UTC()
	cluster := model.Cluster{
		ID:          id,
		Name:        strings.TrimSpace(req.Name),
		Origin:      model.OriginImported,
		Endpoint:    req.Endpoint,
		Fingerprint: req.Fingerprint,
		// An imported cluster is one the operator already depends on, so it is
		// adopted read-only and stays that way until they say otherwise
		// (D-21, P17).
		Locked:             true,
		ClientCertNotAfter: notAfter,
		CreatedAt:          now,
	}

	if _, err := s.deps.Store.ClusterSecrets().Put(ctx,
		secretsRecord(id, bundle, clientCrt, clientKey)); err != nil {
		return model.Cluster{}, err
	}

	stored, err := s.deps.Store.Clusters().Put(ctx, cluster)
	if err != nil {
		// The secrets record is already written. Removing it here keeps the
		// invariant D-02 rests on -- every stored bundle belongs to a stored
		// cluster -- rather than leaving cluster PKI on disk under an id
		// nothing refers to.
		if delErr := s.deps.Store.ClusterSecrets().Delete(ctx, id); delErr != nil {
			s.deps.Logger.Error("could not remove the secrets of a cluster that failed to store",
				slog.String("cluster", string(id)), slog.Any("error", delErr))
		}
		return model.Cluster{}, err
	}

	// The inventory fills itself from the membership the adopted node knows
	// (D-08). A cluster with discovery switched off reports none, and manual
	// entry stays the always-available second way in; neither is a failure of
	// the adoption, which has already succeeded at this point.
	if err := s.adoptMembers(ctx, proof, stored); err != nil {
		s.deps.Logger.Warn("cluster adopted, but its membership could not be read",
			slog.String("cluster", string(id)), slog.Any("error", err))
	}

	return stored, nil
}

// adoptMembers records every node the adopted cluster knows about.
//
// The node holzkube-manager adopted through is recorded too, from its own facts rather
// than from the membership list, so that a cluster with no discovery service
// still has at least the one node that was definitely reachable.
func (s *Service) adoptMembers(ctx context.Context, cc *talos.ClusterClient, cluster model.Cluster) error {
	facts, err := cc.NodeFacts(ctx)
	if err != nil {
		return err
	}
	if _, err := s.recordMachine(ctx, cluster.ID, facts, cluster.Endpoint, model.RoleControlPlane); err != nil {
		return err
	}

	members, err := cc.Members(ctx)
	if err != nil {
		return err
	}

	for _, m := range members {
		if model.MachineID(m.ID) == facts.UUID {
			continue
		}
		role := model.RoleWorker
		if m.ControlPlane {
			role = model.RoleControlPlane
		}
		addr := ""
		if len(m.Addresses) > 0 {
			addr = m.Addresses[0]
		}

		// A member is known by hostname and address and has not been asked
		// anything yet, so its record carries what discovery said and nothing
		// more. The supervisor confirms it, or marks it unconfirmed; either is
		// better than leaving a node the cluster knows about out of the
		// inventory.
		if _, err := s.recordMachine(ctx, cluster.ID, talos.NodeFacts{
			UUID:     model.MachineID(m.ID),
			Hostname: m.Hostname,
		}, addr, role); err != nil {
			return err
		}
	}
	return nil
}

// newClusterID mints a cluster identifier.
//
// It is random hex rather than the operator's name or the cluster's own Talos
// id: the value becomes a filename through the store's key check, and a name
// is something an operator changes. The Talos cluster id would be stable but
// is also a secret that identifies the cluster to the discovery service, and a
// secret does not belong in a URL.
func newClusterID() (model.ClusterID, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("inventory: generate cluster id: %w", err)
	}
	return model.ClusterID(hex.EncodeToString(b[:])), nil
}

// clusterCreds loads the credentials holzkube-manager dials a cluster's nodes with.
func (s *Service) clusterCreds(ctx context.Context, id model.ClusterID) (talos.Creds, error) {
	sec, err := s.deps.Store.ClusterSecrets().Get(ctx, id)
	if err != nil {
		return talos.Creds{}, err
	}
	creds, err := talos.ClusterCreds(sec.OSCACrt, sec.ClientCrt, sec.ClientKey)
	if err != nil {
		return talos.Creds{}, err
	}
	return creds, nil
}

// certificateExpiry reports how long a cluster's client certificate has left.
//
// A negative duration is an expired certificate, which is a named state and
// not a dead cluster (D-23).
func certificateExpiry(c model.Cluster, now time.Time) time.Duration {
	if c.ClientCertNotAfter.IsZero() {
		return 0
	}
	return c.ClientCertNotAfter.Sub(now)
}
