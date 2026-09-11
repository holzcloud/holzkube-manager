package talos

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"github.com/cosi-project/runtime/pkg/resource"
	"github.com/cosi-project/runtime/pkg/safe"
	"github.com/cosi-project/runtime/pkg/state"

	"github.com/siderolabs/talos/pkg/machinery/resources/block"
	"github.com/siderolabs/talos/pkg/machinery/resources/cluster"
	configres "github.com/siderolabs/talos/pkg/machinery/resources/config"
	"github.com/siderolabs/talos/pkg/machinery/resources/hardware"
	"github.com/siderolabs/talos/pkg/machinery/resources/k8s"
	"github.com/siderolabs/talos/pkg/machinery/resources/network"
	runtimeres "github.com/siderolabs/talos/pkg/machinery/resources/runtime"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

// The node facts holzkube-manager reads come from COSI resources rather than from unary
// RPCs wherever a resource exists (D-19, INV-13).
//
// The reason is not elegance. A dashboard that re-asks Memory() and CPUInfo()
// on a timer is blind polling, which INV-13 rules out; the resource state is
// the thing Talos already maintains, and reading it is what lets a later phase
// swap the read for a watch without changing a single caller.
//
// These readers live inside the seam for the same reason the machinery client
// does: everything above this package would otherwise have to know
// machinery's resource types, their namespaces and their ids, and the seam
// would be a comment again. What travels upward is holzkube-manager's own shapes.

// SchematicExtensionName is the virtual system extension a Factory image
// carries its schematic id in.
//
// It is a real ExtensionStatus resource on a running node, with the id in the
// version field of its metadata. Reading it is the only honest way to learn
// the schematic: it is never derivable from the Talos version, and a guess
// here is a phase 9 upgrade that silently strips a node's extensions and
// reports success (D-12).
const SchematicExtensionName = "schematic"

// NodeFacts is everything one bootstrap read learns about a node.
//
// Every field is optional in the sense that a node may not report it, and an
// empty field means exactly that -- these are read, never inferred. The
// caller decides what an absence means for the screen; this package does not
// invent a value to fill it.
type NodeFacts struct {
	UUID     model.MachineID
	Hostname string

	Manufacturer string
	ProductName  string
	SerialNumber string

	// TalosVersion as SMBIOS reports it. The Version RPC is the authoritative
	// answer and the caller usually has it already; this is what the hardware
	// resource says, and the two agreeing is a cheap sanity check.
	TalosVersion string

	Addresses []string

	CPUs      []CPU
	MemoryMiB uint64

	BlockDevices []BlockDevice
	Links        []Link

	// SchematicID is empty for a node that was not installed from a Factory
	// image. Empty is a truthful answer and is never filled in by guessing.
	SchematicID string

	// KubeletImage is the kubelet container reference, e.g.
	// "ghcr.io/siderolabs/kubelet:v1.34.1". It is where the Kubernetes version
	// comes from node-side, without ever dialling :6443 (D-20).
	KubeletImage string

	// Nodename is the node's name in Kubernetes, which is not necessarily its
	// hostname.
	Nodename string
}

// KubernetesVersion returns the version tag of KubeletImage, or "" when there
// is none to read.
//
// The kubelet's own image tag is the Kubernetes version of this node, and it
// is a fact the node holds about itself. Asking the API server instead would
// mean a Kubernetes client, a second failure mode and a dependency on the very
// thing that is broken when the operator most needs the screen (D-20).
func (f NodeFacts) KubernetesVersion() string {
	ref := f.KubeletImage

	// A digest reference pins an image without naming a version, so there is
	// nothing to report rather than a hex string masquerading as one.
	if strings.Contains(ref, "@") {
		return ""
	}

	// The last colon, not the first: a registry may carry a port, and
	// "registry:5000/kubelet:v1.34.1" cut at the first colon yields "5000".
	i := strings.LastIndex(ref, ":")
	if i < 0 {
		return ""
	}
	tag := ref[i+1:]

	// A colon inside a path segment is not a tag separator, which is what a
	// slash after it means.
	if tag == "" || strings.Contains(tag, "/") {
		return ""
	}
	return tag
}

// CPU is one processor.
type CPU struct {
	Socket       string
	Manufacturer string
	ProductName  string
	Cores        uint32
	Threads      uint32
	MaxSpeedMHz  uint32
}

// BlockDevice is one disk as the block resources report it. It is a richer
// shape than Disk, which is what StorageService.Disks answers in maintenance
// mode; the two exist because the two sources exist, and merging them would
// mean inventing the fields whichever source did not supply.
type BlockDevice struct {
	Device     string
	Size       uint64
	PrettySize string
	Model      string
	Serial     string
	Transport  string
	Rotational bool
	Readonly   bool
	CDROM      bool
}

// Link is one network interface.
type Link struct {
	Name         string
	HardwareAddr string
	MTU          uint32
	Up           bool
	SpeedMbit    int
	Driver       string
	Kind         string
}

// Member is one cluster member as the node's own discovery sees it.
type Member struct {
	ID           string
	Hostname     string
	Addresses    []string
	ControlPlane bool
}

// NodeFacts reads the node's bootstrap facts in one pass.
//
// A resource that is missing is not an error: a node in a cluster with the
// discovery service off has no cluster.Member, a node that is not a Factory
// image has no schematic extension, and a node whose kubelet has never started
// has no KubeletSpec. Treating any of those as a failure would turn a partial
// answer into no answer, which is the empty dashboard INV-08 forbids.
//
// A failure to reach the node at all is an error, and the caller can tell the
// two apart because this returns one only when the connection itself failed.
func (c *ClusterClient) NodeFacts(ctx context.Context) (NodeFacts, error) {
	return readNodeFacts(ctx, c.COSI())
}

// NodeFacts reads what a node in maintenance mode can answer.
//
// The same function serves both clients on purpose: a maintenance-mode node
// serves the same resource state with fewer resources in it, so the absences
// are already the right answer. A second implementation would be a second
// place for the two to drift.
func (m *MaintenanceClient) NodeFacts(ctx context.Context) (NodeFacts, error) {
	return readNodeFacts(ctx, m.COSI())
}

func readNodeFacts(ctx context.Context, st state.State) (NodeFacts, error) {
	var facts NodeFacts

	info, err := safe.StateGetByID[*hardware.SystemInformation](
		ctx, st, hardware.SystemInformationID)
	switch {
	case err == nil:
		spec := info.TypedSpec()
		facts.UUID = model.MachineID(spec.UUID)
		facts.Manufacturer = spec.Manufacturer
		facts.ProductName = spec.ProductName
		facts.SerialNumber = spec.SerialNumber
		facts.TalosVersion = spec.Version
	case state.IsNotFoundError(err):
		// A node that does not report SMBIOS information has no UUID, and a
		// machine with no UUID cannot be filed in an inventory keyed on one.
		// That is a refusal rather than an empty record: filing it under a
		// made-up key is how two machines end up sharing a history.
		return NodeFacts{}, fmt.Errorf("talos: node reports no %s: %w",
			hardware.SystemInformationType, ErrNoMachineIdentity)
	default:
		return NodeFacts{}, err
	}

	if addrs, err := safe.StateGetByID[*network.NodeAddress](
		ctx, st, network.FilteredNodeAddressID(network.NodeAddressDefaultID, k8s.NodeAddressFilterNoK8s),
	); err == nil {
		facts.Addresses = prefixesToStrings(addrs.TypedSpec().Addresses)
	}
	if len(facts.Addresses) == 0 {
		if addrs, err := safe.StateGetByID[*network.NodeAddress](
			ctx, st, network.NodeAddressDefaultID,
		); err == nil {
			facts.Addresses = prefixesToStrings(addrs.TypedSpec().Addresses)
		} else if !state.IsNotFoundError(err) {
			return NodeFacts{}, err
		}
	}

	if err := eachResource(ctx, st, hardware.NamespaceName, hardware.ProcessorType,
		func(p *hardware.Processor) {
			s := p.TypedSpec()
			facts.CPUs = append(facts.CPUs, CPU{
				Socket:       cpuSocket(s.Socket, p.Metadata().ID()),
				Manufacturer: s.Manufacturer,
				ProductName:  s.ProductName,
				Cores:        s.CoreCount,
				Threads:      s.ThreadCount,
				MaxSpeedMHz:  s.MaxSpeed,
			})
		}); err != nil {
		return NodeFacts{}, err
	}

	if err := eachResource(ctx, st, hardware.NamespaceName, hardware.MemoryModuleType,
		func(m *hardware.MemoryModule) {
			// Size is in MiB per module; the node's memory is their sum, which
			// is the number an operator reads off a dashboard.
			facts.MemoryMiB += uint64(m.TypedSpec().Size)
		}); err != nil {
		return NodeFacts{}, err
	}

	if err := eachResource(ctx, st, block.NamespaceName, block.DiskType,
		func(d *block.Disk) {
			s := d.TypedSpec()
			facts.BlockDevices = append(facts.BlockDevices, BlockDevice{
				Device:     d.Metadata().ID(),
				Size:       s.Size,
				PrettySize: s.PrettySize,
				Model:      s.Model,
				Serial:     s.Serial,
				Transport:  s.Transport,
				Rotational: s.Rotational,
				Readonly:   s.Readonly,
				CDROM:      s.CDROM,
			})
		}); err != nil {
		return NodeFacts{}, err
	}

	if err := eachResource(ctx, st, network.NamespaceName, network.LinkStatusType,
		func(l *network.LinkStatus) {
			s := l.TypedSpec()
			facts.Links = append(facts.Links, Link{
				Name:         l.Metadata().ID(),
				HardwareAddr: s.HardwareAddr.String(),
				MTU:          s.MTU,
				Up:           s.LinkState,
				SpeedMbit:    s.SpeedMegabits,
				Driver:       s.Driver,
				Kind:         s.Kind,
			})
		}); err != nil {
		return NodeFacts{}, err
	}

	if err := eachResource(ctx, st, runtimeres.NamespaceName, runtimeres.ExtensionStatusType,
		func(e *runtimeres.ExtensionStatus) {
			if e.TypedSpec().Metadata.Name == SchematicExtensionName {
				facts.SchematicID = e.TypedSpec().Metadata.Version
			}
		}); err != nil {
		return NodeFacts{}, err
	}

	if spec, err := safe.StateGetByID[*k8s.KubeletSpec](ctx, st, k8s.KubeletID); err == nil {
		facts.KubeletImage = spec.TypedSpec().Image
	} else if !state.IsNotFoundError(err) {
		return NodeFacts{}, err
	}

	if name, err := safe.StateGetByID[*k8s.Nodename](ctx, st, k8s.NodenameID); err == nil {
		facts.Nodename = name.TypedSpec().Nodename
	} else if !state.IsNotFoundError(err) {
		return NodeFacts{}, err
	}

	facts.Hostname = facts.Nodename
	if host, err := safe.StateGetByID[*network.HostnameStatus](
		ctx, st, network.HostnameID); err == nil {
		facts.Hostname = host.TypedSpec().Hostname
	} else if !state.IsNotFoundError(err) {
		return NodeFacts{}, err
	}

	return facts, nil
}

// ErrNoMachineIdentity reports a node that did not answer with a UUID.
//
// It is separate from an unreachable node because the two call for different
// actions: a node that cannot be reached is retried, and a node with no
// identity is refused, because the inventory is keyed on the identity it does
// not have.
var ErrNoMachineIdentity = errors.New("talos: node reports no machine UUID")

// Members reports the cluster membership as this node knows it.
//
// An empty list is a valid answer: a cluster whose discovery service is turned
// off has no cluster.Member resources at all, and the caller falls back to the
// etcd member list or to manual entry (D-08). It is not an error, and treating
// it as one would make a working configuration look broken.
func (c *ClusterClient) Members(ctx context.Context) ([]Member, error) {
	var out []Member
	err := eachResource(ctx, c.COSI(), cluster.NamespaceName, cluster.MemberType,
		func(m *cluster.Member) {
			s := m.TypedSpec()
			out = append(out, Member{
				ID:           m.Metadata().ID(),
				Hostname:     s.Hostname,
				Addresses:    addrsToStrings(s.Addresses),
				ControlPlane: s.ControlPlane != nil,
			})
		})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// MachineConfigYAML returns the node's active machine configuration as YAML.
//
// This is the one read in phase 3 that touches the configuration, and it
// exists for exactly one purpose: deriving the secrets bundle at adoption
// (D-01). The bytes contain the cluster CA private key and three joining
// tokens, so a caller that logs them, returns them or puts them in an error
// message has leaked the cluster -- which is why nothing above the import
// service ever sees the return value of this method.
// KernelCmdline is the command line the node actually booted with.
//
// It is read separately from NodeFacts and not folded into it, because only
// one caller needs it and it is the expensive kind of fact: a fact whose whole
// value is being compared against a *different* source of truth. NodeFacts is
// the node describing itself; this is evidence for an argument about a
// disagreement (UPG-04).
func (c *ClusterClient) KernelCmdline(ctx context.Context) (string, error) {
	cmdline, err := safe.StateGetByID[*runtimeres.KernelCmdline](
		ctx, c.COSI(), runtimeres.KernelCmdlineID)
	if err != nil {
		return "", err
	}
	return cmdline.TypedSpec().Cmdline, nil
}

func (c *ClusterClient) MachineConfigYAML(ctx context.Context) ([]byte, error) {
	cfg, err := safe.StateGetByID[*configres.MachineConfig](ctx, c.COSI(), configres.ActiveID)
	if err != nil {
		return nil, err
	}
	raw, err := cfg.Container().Bytes()
	if err != nil {
		return nil, fmt.Errorf("talos: encode machine config of %s: %w", c.conn.target.Machine, err)
	}
	return raw, nil
}

// eachResource lists one resource type and calls fn for each item.
//
// A missing resource type is not an error, for the reason NodeFacts states: a
// node that has never run a kubelet has no KubeletSpec, and a node outside a
// cluster has no Members. The distinction this keeps is between "the node
// answered and has none" and "the node did not answer", and only the second is
// an error.
func eachResource[T resource.Resource](
	ctx context.Context,
	st state.State,
	namespace resource.Namespace,
	typ resource.Type,
	fn func(T),
) error {
	list, err := safe.StateList[T](ctx, st, resource.NewMetadata(namespace, typ, "", resource.VersionUndefined))
	if err != nil {
		if state.IsNotFoundError(err) {
			return nil
		}
		return err
	}
	for item := range list.All() {
		fn(item)
	}
	return nil
}

// cpuSocket falls back to the resource's own id when SMBIOS reports no socket
// name. Something unique per processor is needed -- two identical processors
// are otherwise indistinguishable -- and the id is unique by construction.
func cpuSocket(socket, id string) string {
	if socket != "" {
		return socket
	}
	return id
}

func prefixesToStrings(in []netip.Prefix) []string {
	out := make([]string, 0, len(in))
	for _, p := range in {
		out = append(out, p.Addr().String())
	}
	return out
}

func addrsToStrings(in []netip.Addr) []string {
	out := make([]string, 0, len(in))
	for _, a := range in {
		out = append(out, a.String())
	}
	return out
}
