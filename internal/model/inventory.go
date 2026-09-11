package model

import "time"

// ClusterOrigin says how holzkube-manager came to know a cluster.
//
// It is one field rather than two record types because import and create
// produce the same thing -- a cluster with a secrets bundle -- and the only
// lasting difference is what the operator is entitled to assume about it. An
// imported cluster existed and was depended on before holzkube-manager saw it, which is
// exactly why it is adopted read-only (D-21).
type ClusterOrigin string

const (
	// OriginImported is a cluster adopted from a running control plane.
	OriginImported ClusterOrigin = "imported"

	// OriginCreated is a cluster whose PKI holzkube-manager generated itself.
	OriginCreated ClusterOrigin = "created"
)

// Cluster is a Talos cluster holzkube-manager manages.
//
// What is deliberately *not* here: the secrets bundle. It lives in its own
// entity (ClusterSecrets) so that a handler which serialises a Cluster cannot
// leak cluster PKI by forgetting a view type. That is a structural guarantee
// rather than a review habit, and it is the reason the two are separate
// records addressed by the same id.
type Cluster struct {
	ID ClusterID `json:"id"`

	// Name is the operator's label. It never reaches a filesystem path; the ID
	// does.
	Name string `json:"name"`

	Origin ClusterOrigin `json:"origin"`

	// Endpoint is the host of the control-plane node the cluster was adopted
	// through, without a port. It is a hint for the first dial and never
	// identity -- the same rule talos.Target.Addr states for a node.
	Endpoint string `json:"endpoint"`

	// Fingerprint is the SHA-256 of the server certificate the operator
	// confirmed at import (D-03), colon-separated upper-case hex.
	Fingerprint string `json:"fingerprint"`

	// Locked is the per-cluster read-only mutation lock (INV-12, P17). An
	// imported cluster starts locked; a created one does not.
	Locked bool `json:"locked"`

	// ClientCertNotAfter is when the client certificate holzkube-manager currently
	// holds stops working. Talos does not rotate client certificates, so this
	// is a date on which every node goes red in the same second unless
	// somebody acted first (P1/P5/P16) -- which is the whole reason it is a
	// stored field and not something recomputed on demand.
	ClientCertNotAfter time.Time `json:"client_cert_not_after"`

	CreatedAt time.Time `json:"created_at"`

	Rev uint64 `json:"rev"`
}

// ClusterSecrets is a cluster's PKI and joining material.
//
// Every field here is a secret. The record is never serialised to a client,
// never written to the audit log, and never named in an error message; the
// separate entity is what makes those three statements checkable rather than
// aspirational.
//
// The fields are the ones a MachineConfig for a new node has to be built from,
// which is why the bundle is a hard precondition of adoption (D-02): without
// it holzkube-manager can watch a cluster and never enter it again.
type ClusterSecrets struct {
	Cluster ClusterID `json:"cluster"`

	// The Talos OS certificate authority. Its private key is what the worker
	// nodes' MachineConfig does not carry, and its presence is therefore the
	// test for "this is a control-plane node" (D-05).
	OSCACrt []byte `json:"os_ca_crt"`
	OSCAKey []byte `json:"os_ca_key"`

	// The Kubernetes-side authorities. They are carried so that a later phase
	// can generate a joining MachineConfig without touching a node.
	K8sCACrt        []byte `json:"k8s_ca_crt"`
	K8sCAKey        []byte `json:"k8s_ca_key"`
	EtcdCACrt       []byte `json:"etcd_ca_crt"`
	EtcdCAKey       []byte `json:"etcd_ca_key"`
	AggregatorCACrt []byte `json:"aggregator_ca_crt"`
	AggregatorCAKey []byte `json:"aggregator_ca_key"`

	// ServiceAccountKey is cluster.serviceAccount.key.
	ServiceAccountKey []byte `json:"service_account_key"`

	// The three joining tokens. TalosClusterID and ClusterSecret identify the
	// cluster to the discovery service; BootstrapToken and MachineToken are
	// what a joining node authenticates with.
	TalosClusterID string `json:"talos_cluster_id"`
	ClusterSecret  string `json:"cluster_secret"`
	BootstrapToken string `json:"bootstrap_token"`
	MachineToken   string `json:"machine_token"`

	// ClientCrt and ClientKey are the certificate holzkube-manager issued itself from
	// OSCA at adoption and dials with since. They are stored rather than
	// re-derived per connection so that the expiry shown to the operator is
	// the expiry of the thing actually in use.
	ClientCrt []byte `json:"client_crt"`
	ClientKey []byte `json:"client_key"`

	Rev uint64 `json:"rev"`
}

// MachineRole is what a node does in its cluster.
type MachineRole string

const (
	// RoleControlPlane runs etcd and the Kubernetes control plane.
	RoleControlPlane MachineRole = "controlplane"

	// RoleWorker runs workloads only.
	RoleWorker MachineRole = "worker"

	// RoleUnknown is a machine whose role has not been read yet, including
	// every machine in maintenance mode. It is spelled out rather than left as
	// the empty string so that a screen showing it has something to show.
	RoleUnknown MachineRole = "unknown"
)

// Machine is one node in the inventory.
//
// The record is keyed by UUID and flat: Cluster is a nullable field rather
// than a directory the record lives under (D-10). Nesting would make assigning
// a machine to a cluster a file move -- a rename race in the middle of
// provisioning -- would make a UUID lookup a scan of every cluster, and would
// make "not in a cluster" a special case instead of the ordinary state every
// machine passes through.
type Machine struct {
	// ID is hardware.SystemInformation.UUID. The UUID always wins against the
	// address: a node keeps this record across an address change, a reboot and
	// a total outage.
	ID MachineID `json:"id"`

	// Cluster is empty for a machine that belongs to no cluster.
	Cluster ClusterID `json:"cluster,omitempty"`

	Hostname string      `json:"hostname"`
	Role     MachineRole `json:"role"`

	// Addr is the last address this machine answered on, without a port. It is
	// a hint (talos.Target.Addr) and is rewritten whenever the machine is seen
	// somewhere else.
	Addr string `json:"addr"`

	// Locked marks a node upgrades skip (UPG-14).
	//
	// It is per machine and is a different thing from the cluster's read-only
	// lock: that one refuses every mutation against every node in the cluster,
	// this one says "not this one, for now" about a rolling operation that
	// walks nodes. A node with a workload that must not move, or one somebody
	// is already looking at, is the ordinary case -- and stopping a whole
	// upgrade because of it would make this a blunt instrument nobody uses.
	Locked bool `json:"locked,omitempty"`

	// LockReason is why, written by whoever set it. A lock nobody can explain
	// is a lock that gets cleared by the next person who finds it in the way.
	LockReason string `json:"lock_reason,omitempty"`

	// LostAddrAt marks that a *different* machine answered at Addr. The record
	// is never overwritten in that case and a new one is created for the
	// stranger (D-10); this field is what lets the screen say why this machine
	// stopped being found.
	LostAddrAt time.Time `json:"lost_addr_at,omitzero"`

	// Snapshot is the last confirmed set of facts, persisted so that a restart
	// of holzkube-manager -- which most likely happens during the outage the operator
	// is trying to understand -- shows the last known state rather than an
	// empty page (D-16).
	Snapshot MachineSnapshot `json:"snapshot"`

	// AdoptedAt is when this record was created; SeenAt is when the machine
	// last answered anything at all.
	AdoptedAt time.Time `json:"adopted_at"`
	SeenAt    time.Time `json:"seen_at,omitzero"`

	Rev uint64 `json:"rev"`
}

// MachineSnapshot is what a node last said about itself.
//
// ObservedAt is the one clock in the record. Everything an API response says
// about staleness is derived from it, so there is never a second timestamp to
// disagree with the first.
type MachineSnapshot struct {
	ObservedAt time.Time `json:"observed_at,omitzero"`

	TalosVersion      string `json:"talos_version,omitempty"`
	KubernetesVersion string `json:"kubernetes_version,omitempty"`

	// SchematicID is the Image Factory schematic this node was installed from,
	// read off the node and never guessed (D-12). Empty means the node did not
	// report one, which is the truth for a node that was not built from a
	// Factory image -- and a guess here means a phase 9 upgrade that deletes a
	// node's extensions and reports success.
	SchematicID string `json:"schematic_id,omitempty"`

	Manufacturer string `json:"manufacturer,omitempty"`
	ProductName  string `json:"product_name,omitempty"`
	SerialNumber string `json:"serial_number,omitempty"`

	CPUs       []CPU       `json:"cpus,omitempty"`
	MemoryMiB  uint64      `json:"memory_mib,omitempty"`
	Disks      []Disk      `json:"disks,omitempty"`
	Interfaces []Interface `json:"interfaces,omitempty"`
	Services   []Service   `json:"services,omitempty"`

	// EtcdMember reports that this node is in the etcd member list. It is a
	// LevelEtcd fact and is the one field in the snapshot that a dead etcd
	// makes unknowable.
	EtcdMember bool `json:"etcd_member,omitempty"`
}

// CPU is one processor as the node reports it.
type CPU struct {
	// Socket is the board slot the processor sits in. It is carried because it
	// is the only field that distinguishes two identical processors from each
	// other, which a list keyed on anything else cannot do.
	Socket       string `json:"socket,omitempty"`
	Manufacturer string `json:"manufacturer,omitempty"`
	ProductName  string `json:"product_name,omitempty"`
	Cores        uint32 `json:"cores,omitempty"`
	Threads      uint32 `json:"threads,omitempty"`
	MaxSpeedMHz  uint32 `json:"max_speed_mhz,omitempty"`
}

// Disk is one block device as the node reports it.
type Disk struct {
	Device     string `json:"device"`
	Size       uint64 `json:"size,omitempty"`
	PrettySize string `json:"pretty_size,omitempty"`
	Model      string `json:"model,omitempty"`
	Serial     string `json:"serial,omitempty"`
	Transport  string `json:"transport,omitempty"`
	Rotational bool   `json:"rotational,omitempty"`
	Readonly   bool   `json:"readonly,omitempty"`
	CDROM      bool   `json:"cdrom,omitempty"`
}

// Interface is one network link as the node reports it.
type Interface struct {
	Name         string   `json:"name"`
	HardwareAddr string   `json:"hardware_addr,omitempty"`
	MTU          uint32   `json:"mtu,omitempty"`
	Up           bool     `json:"up"`
	SpeedMbit    int      `json:"speed_mbit,omitempty"`
	Addresses    []string `json:"addresses,omitempty"`
	Driver       string   `json:"driver,omitempty"`
	Kind         string   `json:"kind,omitempty"`
}

// Service is one Talos service and its state.
type Service struct {
	ID      string `json:"id"`
	Running bool   `json:"running"`
	Healthy bool   `json:"healthy"`
	State   string `json:"state,omitempty"`

	// HealthUnknown distinguishes "the service does not report health" from
	// "the service reported unhealthy". Talos says so explicitly and
	// collapsing the two would paint a healthy node red.
	HealthUnknown bool `json:"health_unknown,omitempty"`
}
