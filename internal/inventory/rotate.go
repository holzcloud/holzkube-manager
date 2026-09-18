package inventory

import (
	"context"
	"errors"
	"fmt"
	"time"

	cryptox509 "github.com/siderolabs/crypto/x509"
	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// Renewing the certificate holzkube-manager dials a cluster with (V2-OPS-02).
//
// This is the half of "CA rotation" that this product can actually carry out
// and prove, and it is the half the escalating warning on the cluster card has
// been pointing at since D-23 shipped: "the client certificate for homelab
// expires within a week. When it does, every node in this cluster becomes
// unreachable at once." That ladder counted down to a door that did not exist.
//
// It touches no node. The certificate is minted from the cluster's own Talos
// certificate authority, which this installation holds, and a node trusts that
// authority rather than any particular certificate issued from it -- so a
// fresh one is accepted the moment it is presented, with nothing rolled and
// nothing restarted. That is what makes this provable here and what makes it
// safe; it is a different operation from rotating the authority itself, which
// changes what every node trusts and is not built (see README, "What this
// product does not do").

// ErrNoCertificateAuthority reports a cluster whose stored bundle cannot issue
// a certificate.
//
// It is its own error because the repair is specific and nothing else in this
// package produces it: the cluster was adopted from a talosconfig that carried
// an admin certificate and no authority key, which is enough to talk to the
// cluster and not enough to mint anything new.
var ErrNoCertificateAuthority = errors.New("inventory: this cluster's stored bundle has no certificate authority key")

// ErrCertificateRejected reports a newly minted certificate that no node would
// accept.
var ErrCertificateRejected = errors.New("inventory: no node accepted the new certificate")

// RenewClientCertificate issues holzkube-manager a fresh admin certificate for
// one cluster and proves it works before keeping it.
//
// Before keeping it, and that ordering is the whole operation. Minting is two
// lines; the way this goes wrong is replacing a working credential with one
// that is not, and then finding out at the moment the old one expires -- which
// is precisely when nobody can get in to fix it. So the new certificate is
// used to reach a node first, and the store is written only after a node has
// answered through it.
//
// The check is a real connection rather than a local verification against the
// authority. Checking the signature would only prove this installation's
// arithmetic is consistent with itself; what has to be true is that the nodes
// still trust the authority in this store, and the only thing that can say so
// is a node.
func (s *Service) RenewClientCertificate(ctx context.Context, id model.ClusterID) (model.Cluster, error) {
	sec, err := s.deps.Store.ClusterSecrets().Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return model.Cluster{}, ErrNotFound
		}
		return model.Cluster{}, err
	}

	if len(sec.OSCACrt) == 0 || len(sec.OSCAKey) == 0 {
		return model.Cluster{}, fmt.Errorf("%w. It was adopted from a talosconfig carrying an "+
			"admin certificate and no authority key, which is enough to reach the cluster and "+
			"not enough to issue anything new. Import it again with a talosconfig that carries "+
			"the authority, or take a fresh one from a control-plane node with `talosctl config "+
			"new`", ErrNoCertificateAuthority)
	}

	crt, key, notAfter, err := mintFrom(sec.OSCACrt, sec.OSCAKey)
	if err != nil {
		return model.Cluster{}, err
	}

	if err := s.provesItself(ctx, id, sec.OSCACrt, crt, key); err != nil {
		return model.Cluster{}, err
	}

	// Only now. Both records, and the secrets first: a cluster record claiming
	// an expiry the stored certificate does not have is a countdown against
	// the wrong certificate, and this ladder is the one thing an operator
	// trusts to tell them when to act.
	sec.ClientCrt, sec.ClientKey = crt, key
	if _, err := s.deps.Store.ClusterSecrets().Put(ctx, sec); err != nil {
		return model.Cluster{}, err
	}

	cluster, err := s.deps.Store.Clusters().Get(ctx, id)
	if err != nil {
		return model.Cluster{}, err
	}
	cluster.ClientCertNotAfter = notAfter
	if _, err := s.deps.Store.Clusters().Put(ctx, cluster); err != nil {
		return model.Cluster{}, err
	}

	// Connections already open keep the old certificate until they are
	// replaced, and that is fine rather than a loose end: the old certificate
	// is still valid -- this ran before it expired, which is the point of the
	// ladder -- and every connection opened after this reads the store.
	return cluster, nil
}

// mintFrom issues an admin certificate from a Talos certificate authority.
func mintFrom(caCrt, caKey []byte) (crt, key []byte, notAfter time.Time, err error) {
	bundle := &secrets.Bundle{
		Clock: secrets.NewClock(),
		Certs: &secrets.Certs{
			OS: &cryptox509.PEMEncodedCertificateAndKey{Crt: caCrt, Key: caKey},
		},
	}
	return mintClientCert(bundle)
}

// provesItself opens a connection with the new certificate and makes one call.
//
// Every machine in the cluster is tried, because "this certificate does not
// work" and "the node I happened to pick is switched off" are different
// findings and only the first one is a reason to throw the certificate away.
func (s *Service) provesItself(ctx context.Context, id model.ClusterID, ca, crt, key []byte) error {
	creds, err := talos.ClusterCreds(ca, crt, key)
	if err != nil {
		return err
	}

	machines, err := s.MachinesOf(ctx, id)
	if err != nil {
		return err
	}
	if len(machines) == 0 {
		return fmt.Errorf("%w: this cluster has no machines in the inventory, so there is "+
			"nothing to try the new certificate against. Refresh the cluster first",
			ErrCertificateRejected)
	}

	var last error
	for _, m := range machines {
		cc, err := talos.NewClusterClient(ctx, s.deps.Dialer, talos.Target{
			Cluster: id, Machine: m.ID, Addr: m.Addr,
		}, creds, s.deps.Mode)
		if err != nil {
			last = err
			continue
		}
		_ = cc.Close()
		return nil
	}

	return fmt.Errorf("%w, so the old one has been kept and nothing has changed. The last "+
		"attempt said: %w. Either every node is unreachable, or the certificate authority in "+
		"this installation's store is no longer the one the cluster trusts -- which is what an "+
		"authority rotated outside holzkube-manager looks like from here",
		ErrCertificateRejected, last)
}

// AdoptAuthority is pass 3 of a CA rotation: this installation stops holding
// the cluster's old authority and starts holding the new one.
//
// It is here rather than in the rotation package because the minting and the
// proof already live here -- V2-OPS-02's first half -- and a second
// implementation of "mint, prove, then keep" would be a second place for that
// ORDER to be got wrong. The order is the safety property: the certificate is
// used to reach a node before the store is written, so a rotation that got
// this far and cannot get in leaves the old credential in place and fails
// loudly, rather than replacing a working credential with one nobody accepts.
//
// The new authority is the trust anchor for the proof as well as the issuer of
// the certificate, because by this point in the rotation every node issues
// from it: a proof against the old anchor would be testing the state the
// rotation has just left.
func (s *Service) AdoptAuthority(ctx context.Context, id model.ClusterID, caCrt, caKey []byte) error {
	sec, err := s.deps.Store.ClusterSecrets().Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	if len(caCrt) == 0 || len(caKey) == 0 {
		return fmt.Errorf("%w: the rotation passed no authority", ErrNoCertificateAuthority)
	}

	crt, key, notAfter, err := mintFrom(caCrt, caKey)
	if err != nil {
		return err
	}
	if err := s.provesItself(ctx, id, caCrt, crt, key); err != nil {
		return err
	}

	sec.OSCACrt, sec.OSCAKey = caCrt, caKey
	sec.ClientCrt, sec.ClientKey = crt, key
	if _, err := s.deps.Store.ClusterSecrets().Put(ctx, sec); err != nil {
		return err
	}

	cluster, err := s.deps.Store.Clusters().Get(ctx, id)
	if err != nil {
		return err
	}
	cluster.ClientCertNotAfter = notAfter
	_, err = s.deps.Store.Clusters().Put(ctx, cluster)
	return err
}
