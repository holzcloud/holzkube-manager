package inventory

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"regexp"
	"time"

	"github.com/siderolabs/talos/pkg/machinery/config"
	"github.com/siderolabs/talos/pkg/machinery/config/configloader"
	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
	"github.com/siderolabs/talos/pkg/machinery/role"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
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

// unknownDocumentKind reads a document kind out of machinery's refusal to
// decode it, and reports whether that is what went wrong.
//
// A Talos machine configuration is multi-document YAML, and since v1.14 the
// documents are where a great deal of it lives: the Kubernetes API-server CA,
// the cluster identity, the volume layout and a dozen others each have their
// own kind beside the v1alpha1 document. machinery decodes one only if THIS
// build's machinery has the kind registered, and its loader is all-or-nothing.
//
// That is why this is a refusal rather than something to work around. A real
// v1.14 cluster met a build pinned to machinery v1.13.9 and was told "Internal
// error"; the reason was DiscoveryServiceConfig, a kind v1.13.9 does not have.
//
// The first attempt at fixing that selected the v1alpha1 document and ignored
// the rest, on the reasoning that every secret adoption needs lives in it.
// That was true of v1.13 and is NOT true of v1.14 -- KubeAPIServerCAConfig is
// its own document now -- so the same approach would have derived a bundle
// with the Kubernetes CA silently missing, and refused the node as a worker.
// Wrong, and confidently wrong, which is worse than the Internal error it
// replaced.
//
// So an unknown document is what it looks like: this build cannot read this
// node's configuration, and the honest answer names the remedy rather than
// guessing at the parts it does recognise.
func unknownDocumentKind(err error) (string, bool) {
	// machinery's own wording, from its decoder:
	//   error decoding document v1alpha1/DiscoveryServiceConfig/default
	//   (line 94): "DiscoveryServiceConfig" "v1alpha1": not registered
	//
	// Matched on the message because machinery returns no typed error for it.
	// That is brittle in one direction only: if the wording changes, this stops
	// recognising the case and the answer falls back to the parse failure below
	// it, which is the behaviour before this existed rather than something
	// worse -- and TestAnUnknownDocumentIsNamedAsTooNew goes red and says so.
	m := unknownKindPattern.FindStringSubmatch(err.Error())
	if m == nil {
		return "", false
	}
	return m[1], true
}

var unknownKindPattern = regexp.MustCompile(`"([A-Za-z0-9]+)"\s+"[^"]*":\s*not registered`)

// missingSecretDocument names the first thing NewBundleFromConfig would
// dereference and find absent, or "" when all of them are there.
//
// The list is machinery's, read off bundle.go rather than guessed: it is
// exactly the set of accessors that function calls a method on without
// checking. Keeping it in this order means the name returned is the one that
// would have panicked.
func missingSecretDocument(c config.Config) string {
	switch {
	case c.K8sAPIServerCAConfig() == nil:
		return "Kubernetes API-server certificate authority"
	case c.K8sAggregatorCAConfig() == nil:
		return "Kubernetes aggregator certificate authority"
	case c.K8sServiceAccountConfig() == nil:
		return "Kubernetes service-account key"
	case c.Cluster() == nil || c.Cluster().Etcd() == nil:
		return "etcd certificate authority"
	case c.Machine() == nil || c.Machine().Security() == nil:
		return "Talos certificate authority"
	}
	return ""
}

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
		if kind, ok := unknownDocumentKind(err); ok {
			return nil, fmt.Errorf("%w: its machine configuration contains a %s document, which "+
				"this build does not know. The node is running a Talos newer than this build "+
				"was made for (%s to %s). Upgrading holzkube-manager is the remedy; adopting it "+
				"with this build would mean deriving the cluster's secrets from a configuration "+
				"only partly understood",
				talos.ErrUnsupportedVersion, kind, talos.MinSupportedVersion, talos.MaxSupportedVersion)
		}
		// The node's own configuration failed to parse. The error text comes
		// from machinery's loader and describes structure, not content, so it
		// is safe to wrap -- but the bytes themselves never appear in it.
		return nil, fmt.Errorf("inventory: the node's machine configuration could not be read: %w", err)
	}

	cfg := provider.RawV1Alpha1()
	if cfg == nil {
		return nil, fmt.Errorf("%w: the node serves no v1alpha1 machine configuration", ErrNotControlPlane)
	}

	// Asked before the bundle is derived, and machinery v1.14 turned that from
	// a preference into a requirement.
	//
	// D-05 used to be answered the other way round: derive the bundle from
	// whatever the node served, then look at what came out and refuse if the
	// Talos CA had no private key, which is what a worker looks like. Under
	// machinery v1.13 that worked, because a worker's config produced a bundle
	// with empty fields. Under v1.14 it PANICS -- NewBundleFromConfig reads
	// c.K8sAPIServerCAConfig().IssuingCA(), a worker has no API-server CA
	// config, and the nil is dereferenced. On a running instance that is a
	// crash where a refusal belongs.
	//
	// So the configuration is asked what it is. A node states its own type and
	// has no reason to be coy about it; deriving a bundle in order to find out
	// was always inference where a question would do.
	//
	// IsControlPlane covers the init type as well as the control-plane type.
	// An init node is a control plane that bootstrapped the cluster, it holds
	// the same material, and refusing it would refuse the one node a
	// single-node cluster has.
	if machineType := provider.Machine().Type(); !machineType.IsControlPlane() {
		return nil, fmt.Errorf("%w: it is a %s node, and its machine configuration carries the "+
			"Talos CA certificate without its private key, which is what a worker looks like",
			ErrNotControlPlane, machineType)
	}

	// Every accessor NewBundleFromConfig dereferences, checked before it is
	// called, because machinery v1.14 does not check them itself: it reads
	// c.K8sAPIServerCAConfig().IssuingCA() and panics on a configuration that
	// has no such document. A node hands this process bytes; a process that
	// crashes on the bytes it was handed has a worse defect than whatever was
	// wrong with them.
	//
	// It reads as an incomplete control plane rather than as an internal
	// failure, because that is what it is: the node answered, and what it
	// served does not carry what a cluster's secrets are made of.
	if missing := missingSecretDocument(provider); missing != "" {
		return nil, fmt.Errorf("%w: its machine configuration carries no %s, so the cluster's "+
			"secrets cannot be derived from it", ErrNotControlPlane, missing)
	}

	// Since machinery v1.14 this reports rather than swallows. What it can
	// fail on is a configuration whose certificate material does not load --
	// which is a statement about the node's own config, not about this
	// process, so it is wrapped like the parse failure above it and for the
	// same reason: the message describes structure and the bytes never appear
	// in it.
	bundle, err := secrets.NewBundleFromConfig(secrets.NewClock(), provider)
	if err != nil {
		return nil, fmt.Errorf("inventory: the node's own secrets could not be read from its "+
			"machine configuration: %w", err)
	}
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
