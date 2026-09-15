package inventory

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/siderolabs/talos/pkg/machinery/config/configloader"
	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
	"github.com/siderolabs/talos/pkg/machinery/role"
	"gopkg.in/yaml.v3"

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

// v1alpha1Document returns the machine-configuration document out of what a
// node serves, discarding every other document in it.
//
// A Talos machine configuration is a multi-document YAML file. One document is
// the v1alpha1 Config that carries `.machine` and `.cluster` -- and therefore
// every secret adoption needs. The rest are separate typed documents:
// VolumeConfig, KubeSpanConfig, UserVolumeConfig and a growing list of others,
// each of which machinery decodes only if this build's machinery knows the
// kind.
//
// That last clause is why this function exists. machinery's loader is
// all-or-nothing: one document whose kind is not registered fails the whole
// parse, and it has no option to tolerate one. So a real cluster refused
// adoption with
//
//	error decoding document v1alpha1/DiscoveryServiceConfig/default (line 94):
//	  "DiscoveryServiceConfig" "v1alpha1": not registered
//
// -- a document type this build's machinery (v1.13.9) does not have at all,
// belonging to a Talos newer than the pin. Nothing in adoption reads it.
// Nothing in adoption reads ANY of the sibling documents: the bundle comes from
// v1alpha1 alone. The adoption was refused over a document it does not look at.
//
// Selecting one document instead of tolerating failures is deliberate, and the
// difference matters when the next kind appears. Tolerating would mean
// adoption's success depends on which unknown documents a node happens to
// carry; selecting means it depends on the one document it actually reads.
// A manager that has to know every document kind Talos will ever ship is a
// manager that breaks on every Talos release.
//
// The discriminator is machinery's own, from its decoder: the machine
// configuration is the document with `version: v1alpha1` and NO `kind`. Every
// typed document carries a kind.
func v1alpha1Document(configYAML []byte) ([]byte, error) {
	dec := yaml.NewDecoder(bytes.NewReader(configYAML))

	for {
		var doc yaml.Node
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			// Structure, not content: the message names a line and a YAML
			// shape and never the bytes.
			return nil, fmt.Errorf("inventory: the node's machine configuration is not readable YAML: %w", err)
		}

		version, kind := documentIdentity(&doc)
		if version != "v1alpha1" || kind != "" {
			continue
		}

		out, err := yaml.Marshal(&doc)
		if err != nil {
			return nil, fmt.Errorf("inventory: re-encoding the machine configuration document: %w", err)
		}
		return out, nil
	}

	// Not an internal failure and not a parse failure: it is the node being
	// the wrong node, which is the refusal D-05 already owns. A machine with
	// no v1alpha1 document is one that has never been configured.
	return nil, fmt.Errorf("%w: the node serves no v1alpha1 machine configuration document", ErrNotControlPlane)
}

// documentIdentity reads the two top-level keys that decide what a document is.
//
// Missing keys read as empty, which is the right answer rather than an error:
// a document with neither is not the one being looked for, and saying so by
// returning empty strings lets the caller skip it like any other.
func documentIdentity(doc *yaml.Node) (version, kind string) {
	node := doc
	if node.Kind == yaml.DocumentNode {
		if len(node.Content) == 0 {
			return "", ""
		}
		node = node.Content[0]
	}
	if node.Kind != yaml.MappingNode {
		return "", ""
	}

	for i := 0; i+1 < len(node.Content); i += 2 {
		switch node.Content[i].Value {
		case "version":
			version = node.Content[i+1].Value
		case "kind":
			kind = node.Content[i+1].Value
		}
	}
	return version, kind
}

// deriveSecrets reads a node's machine configuration and returns the cluster's
// secrets bundle.
//
// configYAML holds the cluster CA private key and three joining tokens. It is
// never logged, never returned to a caller and never named in an error: the
// only thing that leaves this function is the bundle, and the only thing that
// leaves the import path is a record in the secrets entity.
func deriveSecrets(configYAML []byte) (*secrets.Bundle, error) {
	machineConfig, err := v1alpha1Document(configYAML)
	if err != nil {
		return nil, err
	}

	provider, err := configloader.NewFromBytes(machineConfig)
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
