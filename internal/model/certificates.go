package model

import "time"

// NodeCertificate is one certificate on one control-plane node, with the date
// it stops working. It carries a name, a role and dates and nothing that could
// be used to impersonate the node: the keys Talos keeps beside these
// certificates never get this far.
type NodeCertificate struct {
	Machine MachineID `json:"machine"`
	Node    string    `json:"node"`

	// Name is the role ("etcd peer", "Kubernetes authority"); Kind is
	// "authority" (nothing renews it) or "issued" (Talos renews it).
	Name string `json:"name"`
	Kind string `json:"kind"`

	Subject  string    `json:"subject"`
	NotAfter time.Time `json:"not_after"`

	// DaysLeft is whole days, negative once it has expired. It is computed when
	// the view is built, not when the certificate was read, so an old reading
	// still counts down.
	DaysLeft int `json:"days_left"`

	// Urgency is the rung of the ladder this certificate stands on: none,
	// badge, banner or critical.
	Urgency string `json:"urgency"`
}

// CertificateHealth is what a cluster's overview says about the certificates its
// control-plane nodes hold. It is derived from the last reading and never
// stored.
type CertificateHealth struct {
	// CheckedAt is when the nodes were last read; ReadNodes of ControlPlanes
	// answered. A cluster whose nodes did not answer is not reported as fine.
	CheckedAt    time.Time `json:"checked_at"`
	ReadNodes    int       `json:"read_nodes"`
	ControlPlane int       `json:"control_planes"`

	// Urgency and Warning are the worst certificate's rung and a sentence about
	// it, in the same words the client-certificate warning uses.
	Urgency string `json:"urgency"`
	Warning string `json:"warning,omitempty"`

	// Items are every certificate read, soonest expiry first.
	Items []NodeCertificate `json:"items"`
}
