package inventory

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"time"

	"github.com/siderolabs/talos/pkg/machinery/config/configloader"
	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
	"github.com/siderolabs/talos/pkg/machinery/role"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

// The secrets bundle is derived from a control-plane node's own machine
// configuration (D-01), and nothing else in this package is allowed to make
// one up.
//
// The operator has a talosconfig, because they use talosctl today. They may
// never have seen a secrets.yaml: it was written by the original
// `talosctl gen config` and is wherever that left it. An adoption path that
// demanded it would be unusable for the one cluster this product was written
// for.
//
// There is no "generate if missing" fallback anywhere below, and the absence
// is load-bearing rather than an oversight. A failed derivation that silently
// generated fresh CA material would produce a cluster record that looks like
// the real one and cannot enter a single one of its nodes (D-07).

// ClientCertTTL is how long the certificate holzkube-manager mints for itself is valid.
//
// One year matches what talosctl issues and is what the expiry warnings in
// D-23 are calibrated against. It is deliberately not "forever": a credential
// that never expires is one that is never noticed, and the warning ladder is
// the feature.
const ClientCertTTL = 365 * 24 * time.Hour

// deriveSecrets reads a node's machine configuration and returns the cluster's
// secrets bundle.
//
// configYAML holds the cluster CA private key and three joining tokens. It is
// never logged, never returned to a caller and never named in an error: the
// only thing that leaves this function is the bundle, and the only thing that
// leaves the import path is a record in the secrets entity.
func deriveSecrets(configYAML []byte) (*secrets.Bundle, error) {
	provider, err := configloader.NewFromBytes(configYAML)
	if err != nil {
		// The node's own configuration failed to parse. The error text comes
		// from machinery's loader and describes structure, not content, so it
		// is safe to wrap -- but the bytes themselves never appear in it.
		return nil, fmt.Errorf("inventory: the node's machine configuration could not be read: %w", err)
	}

	cfg := provider.RawV1Alpha1()
	if cfg == nil {
		return nil, fmt.Errorf("%w: the node serves no v1alpha1 machine configuration", ErrNotControlPlane)
	}

	bundle := secrets.NewBundleFromConfig(secrets.NewClock(), provider)
	if err := controlPlaneMaterialPresent(bundle); err != nil {
		return nil, err
	}
	return bundle, nil
}

// controlPlaneMaterialPresent is D-05, as a check rather than as a hope.
//
// A worker's machine configuration carries `.machine.ca` with the certificate
// and an empty key -- which is exactly right for a worker and useless for
// adoption. The refusal names the remedy, because "adoption failed" without
// "name a control-plane node" is a message an operator cannot act on.
func controlPlaneMaterialPresent(b *secrets.Bundle) error {
	if b == nil || b.Certs == nil || b.Certs.OS == nil || len(b.Certs.OS.Key) == 0 {
		return fmt.Errorf("%w: its machine configuration carries the Talos CA certificate without its private key, "+
			"which is what a worker looks like", ErrNotControlPlane)
	}
	if b.Certs.K8s == nil || len(b.Certs.K8s.Key) == 0 {
		return fmt.Errorf("%w: its machine configuration carries no Kubernetes CA private key", ErrNotControlPlane)
	}
	return nil
}

// mintClientCert issues holzkube-manager its own admin certificate from the derived
// bundle.
//
// This is the certificate every later connection uses, and minting it is what
// turns "the uploaded file worked" into "holzkube-manager can let itself in". A
// talosconfig that works today proves nothing about the bundle a new
// certificate has to come from tomorrow (D-04).
func mintClientCert(b *secrets.Bundle) (crt, key []byte, notAfter time.Time, err error) {
	pair, err := b.GenerateTalosAPIClientCertificateWithTTL(role.MakeSet(role.Admin), ClientCertTTL)
	if err != nil {
		return nil, nil, time.Time{}, fmt.Errorf("inventory: issue client certificate from the derived bundle: %w", err)
	}

	loaded, err := tls.X509KeyPair(pair.Crt, pair.Key)
	if err != nil {
		return nil, nil, time.Time{}, fmt.Errorf("inventory: the issued client certificate does not load: %w", err)
	}
	leaf, err := x509.ParseCertificate(loaded.Certificate[0])
	if err != nil {
		return nil, nil, time.Time{}, fmt.Errorf("inventory: the issued client certificate does not parse: %w", err)
	}

	return pair.Crt, pair.Key, leaf.NotAfter, nil
}

// secretsRecord flattens a bundle into the stored shape.
//
// It is a field-by-field copy rather than a marshalled bundle so that the
// record's schema is holzkube-manager's and not machinery's: an upstream change to the
// bundle type would otherwise reshape a stored record silently, and that
// record is the only way back into the cluster.
func secretsRecord(id model.ClusterID, b *secrets.Bundle, clientCrt, clientKey []byte) model.ClusterSecrets {
	rec := model.ClusterSecrets{
		Cluster:   id,
		ClientCrt: clientCrt,
		ClientKey: clientKey,
	}

	if b.Certs != nil {
		if b.Certs.OS != nil {
			rec.OSCACrt, rec.OSCAKey = b.Certs.OS.Crt, b.Certs.OS.Key
		}
		if b.Certs.K8s != nil {
			rec.K8sCACrt, rec.K8sCAKey = b.Certs.K8s.Crt, b.Certs.K8s.Key
		}
		if b.Certs.Etcd != nil {
			rec.EtcdCACrt, rec.EtcdCAKey = b.Certs.Etcd.Crt, b.Certs.Etcd.Key
		}
		if b.Certs.K8sAggregator != nil {
			rec.AggregatorCACrt, rec.AggregatorCAKey = b.Certs.K8sAggregator.Crt, b.Certs.K8sAggregator.Key
		}
		if b.Certs.K8sServiceAccount != nil {
			rec.ServiceAccountKey = b.Certs.K8sServiceAccount.Key
		}
	}
	if b.Cluster != nil {
		rec.TalosClusterID, rec.ClusterSecret = b.Cluster.ID, b.Cluster.Secret
	}
	if b.Secrets != nil {
		rec.BootstrapToken = b.Secrets.BootstrapToken
	}
	if b.TrustdInfo != nil {
		rec.MachineToken = b.TrustdInfo.Token
	}
	return rec
}
