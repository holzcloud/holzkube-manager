package talos

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/cosi-project/runtime/pkg/safe"
	"github.com/cosi-project/runtime/pkg/state"

	"github.com/siderolabs/talos/pkg/machinery/resources/secrets"
)

// What a node's certificates say about when they stop working.
//
// Talos keeps every certificate it uses as a COSI resource, and the resource
// carries the private key next to the certificate. This file is where that
// stops: the key is never copied out of the resource, only the parsed
// certificate's name and dates leave this package, so a caller cannot log,
// return or store a key it was never handed.

// CertificateKind says whether a certificate is an authority or something an
// authority issued. The difference is who renews it: Talos rotates the issued
// ones by itself, and nothing rotates an authority.
type CertificateKind string

const (
	// CertificateAuthority is a root: Talos never renews it, so its expiry is
	// the one an operator has to act on.
	CertificateAuthority CertificateKind = "authority"

	// CertificateIssued is a leaf Talos renews on its own. One that is close to
	// expiry means a rotation is stuck, which is worth knowing and different
	// from an authority running out.
	CertificateIssued CertificateKind = "issued"
)

// Certificate is one certificate on one node.
type Certificate struct {
	// Name is holzkube-manager's name for the role, stable across nodes: "Talos API
	// authority", "etcd peer", and so on.
	Name string `json:"name"`

	Kind CertificateKind `json:"kind"`

	// Subject is the certificate's common name, to tell two of the same role
	// apart (an authority that was rotated carries two).
	Subject string `json:"subject"`

	NotAfter time.Time `json:"not_after"`
}

// Certificates reads the certificates a node holds: the Talos API server
// certificate and authority, and on a control plane the etcd and Kubernetes
// ones. A resource the node does not have (a worker has no etcd) is left out,
// not reported as a failure.
func (c *ClusterClient) Certificates(ctx context.Context) ([]Certificate, error) {
	st := c.COSI()
	var out []Certificate

	add := func(name string, kind CertificateKind, pems ...[]byte) {
		for _, p := range pems {
			if cert, ok := parseCertificate(p); ok {
				out = append(out, Certificate{
					Name: name, Kind: kind,
					Subject: cert.Subject.CommonName, NotAfter: cert.NotAfter,
				})
			}
		}
	}

	readers := []func(context.Context, state.State) error{
		func(ctx context.Context, st state.State) error {
			r, err := safe.StateGetByID[*secrets.OSRoot](ctx, st, secrets.OSRootID)
			if err != nil {
				return err
			}
			if ca := r.TypedSpec().IssuingCA; ca != nil {
				add("Talos API authority", CertificateAuthority, ca.Crt)
			}
			return nil
		},
		func(ctx context.Context, st state.State) error {
			r, err := safe.StateGetByID[*secrets.API](ctx, st, secrets.APIID)
			if err != nil {
				return err
			}
			if s := r.TypedSpec().Server; s != nil {
				add("Talos API server", CertificateIssued, s.Crt)
			}
			return nil
		},
		func(ctx context.Context, st state.State) error {
			r, err := safe.StateGetByID[*secrets.EtcdRoot](ctx, st, secrets.EtcdRootID)
			if err != nil {
				return err
			}
			if ca := r.TypedSpec().EtcdCA; ca != nil {
				add("etcd authority", CertificateAuthority, ca.Crt)
			}
			return nil
		},
		func(ctx context.Context, st state.State) error {
			r, err := safe.StateGetByID[*secrets.Etcd](ctx, st, secrets.EtcdID)
			if err != nil {
				return err
			}
			spec := r.TypedSpec()
			if spec.Etcd != nil {
				add("etcd server", CertificateIssued, spec.Etcd.Crt)
			}
			if spec.EtcdPeer != nil {
				add("etcd peer", CertificateIssued, spec.EtcdPeer.Crt)
			}
			return nil
		},
		func(ctx context.Context, st state.State) error {
			r, err := safe.StateGetByID[*secrets.KubernetesRoot](ctx, st, secrets.KubernetesRootID)
			if err != nil {
				return err
			}
			if ca := r.TypedSpec().IssuingCA; ca != nil {
				add("Kubernetes authority", CertificateAuthority, ca.Crt)
			}
			return nil
		},
		func(ctx context.Context, st state.State) error {
			r, err := safe.StateGetByID[*secrets.KubernetesDynamicCerts](ctx, st, secrets.KubernetesDynamicCertsID)
			if err != nil {
				return err
			}
			if s := r.TypedSpec().APIServer; s != nil {
				add("Kubernetes API server", CertificateIssued, s.Crt)
			}
			return nil
		},
	}

	for _, read := range readers {
		if err := read(ctx, st); err != nil && !state.IsNotFoundError(err) {
			return nil, fmt.Errorf("read node certificates: %w", err)
		}
	}
	return out, nil
}

// parseCertificate returns the first certificate in a PEM block, and false for
// anything that is not one: a node that serves an empty field is a node with no
// certificate to report, not an error.
func parseCertificate(pemBytes []byte) (*x509.Certificate, bool) {
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, false
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, false
	}
	return cert, true
}
