package talossim_test

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/api/storage"

	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/talos"
	"github.com/holzcloud/holzkube-manager/internal/talossim"
)

// machineryClientImport is the import path of the client the seam contains. It
// is written out rather than imported so that the guard reads the same string
// the source files under inspection contain.
const machineryClientImport = "github.com/siderolabs/talos/pkg/machinery/client"

// seamPackages are the production packages that are allowed to speak to a
// Talos node, and therefore the packages whose call sites this guard has to
// account for. It is the same set internal/talos/seam_test.go exempts, which
// is not a coincidence: the seam guard says only these may call the machinery
// client, so only these can generate drift.
var seamPackages = []string{filepath.Join("..", "talos")}

// scannedFileFloor is the number of production files the walk must reach.
//
// A guard that scans nothing passes for the wrong reason. internal/talos has
// held at least four production files since plan 02-01, so a run that sees
// fewer has lost its way to the tree rather than found a clean result -- the
// same failure mode TestNoAddressAboveTheSeam and
// TestNoDirectFileAccessOutsideFsstore defend against.
const scannedFileFloor = 4

// TestMethodCoverage fails when talossim drifts behind the production client.
//
// The gap it closes: talossim inherits all 54 MachineService methods from the
// embedded UnimplementedMachineServiceServer. A call site added in a later
// phase against a method nobody implemented here does not fail in this package
// -- it compiles, it dials, and it comes back Unimplemented at runtime in
// whatever phase happens to run it. A comment asking people to keep the two in
// sync is not a mechanism.
//
// So the enumeration is derived, not written down: the walk finds every method
// holzkube-manager's own packages call on a machinery client, and each one is then
// called against a running simulator. The check is behavioural rather than
// structural on purpose. A reflection check on the server type would pass for
// a method that exists and panics, and a hand-maintained list of "methods we
// implement" is exactly the artefact this test replaces -- it would be updated
// by the same edit that forgot to implement the method.
func TestMethodCoverage(t *testing.T) {
	t.Parallel()

	callSites, packages, files := machineryCallSites(t)

	if files < scannedFileFloor {
		t.Fatalf("scanned only %d production file(s) across %d package(s); the guard is not reaching the tree",
			files, packages)
	}
	t.Logf("scanned %d production file(s) across %d package(s); found %d machinery-client call site(s): %s",
		files, packages, len(callSites), strings.Join(callSites, ", "))

	rpcs := serviceRPCs()

	var reachable []string
	for _, name := range callSites {
		if _, ok := rpcs[name]; ok {
			reachable = append(reachable, name)
			continue
		}
		// Not every method on the machinery client is an RPC: Close and the
		// accessors are ordinary Go. Naming them here keeps the log honest
		// about what was and was not checked.
		t.Logf("%s is not an RPC on either service; nothing for the simulator to implement", name)
	}

	if len(reachable) == 0 {
		t.Fatal("the walk found no RPC call sites at all; either internal/talos stopped calling the " +
			"machinery client or the detection is broken, and both make this guard vacuous")
	}

	var missing []string
	for _, name := range reachable {
		if err := probeRPC(t, rpcs[name]); err != nil {
			missing = append(missing, fmt.Sprintf("%s (%s): %v", name, rpcs[name].service, err))
		}
	}

	if len(missing) > 0 {
		t.Fatalf("holzkube-manager calls %d MachineService/StorageService method(s) that talossim does not implement:\n  %s\n\n"+
			"The simulator has drifted behind the production client. The fix is to implement the method "+
			"in internal/talossim -- not to remove the call site. Deleting the call is the cheaper way to "+
			"make this test green and it inverts the guard: the product would keep needing the method and "+
			"the simulator would stop being an honest stand-in for a node.",
			len(missing), strings.Join(missing, "\n  "))
	}
}

// TestMethodCoverageProbeRecognisesBothCallShapes is the negative control for
// the probe itself.
//
// TestMethodCoverage passes today because every discovered call site is
// implemented, which means its failure path has never run. Worse, no
// production call site is streaming yet, so probeStream is only exercised the
// day someone adds one -- and if it were broken, that day it would report a
// working method as missing, or a missing one as working. The four cases below
// pin both call shapes in both directions against methods chosen for what they
// demonstrate rather than for what holzkube-manager calls.
func TestMethodCoverageProbeRecognisesBothCallShapes(t *testing.T) {
	t.Parallel()

	rpcs := serviceRPCs()

	for _, tc := range []struct {
		method      string
		implemented bool
	}{
		{method: "Version", implemented: true},       // unary, written
		{method: "Logs", implemented: true},          // streaming, written
		{method: "Containers", implemented: false},   // unary, inherited
		{method: "EtcdSnapshot", implemented: false}, // streaming, inherited
	} {
		target, ok := rpcs[tc.method]
		if !ok {
			t.Fatalf("%s is not an RPC on either service descriptor; the probe control is out of date", tc.method)
		}
		if target.streaming == (tc.method == "Version" || tc.method == "Containers") {
			t.Fatalf("%s is streaming=%v, which is not the call shape this case means to cover",
				tc.method, target.streaming)
		}

		err := probeRPC(t, target)
		switch {
		case tc.implemented && err != nil:
			t.Errorf("probing the implemented %s reported it missing: %v", tc.method, err)
		case !tc.implemented && err == nil:
			t.Errorf("probing the inherited %s reported it implemented; the probe cannot tell "+
				"a written method from one that answers Unimplemented", tc.method)
		}
	}
}

// rpcTarget is one RPC the simulator has to answer.
type rpcTarget struct {
	service   string
	method    string
	streaming bool
}

func (r rpcTarget) path() string { return "/" + r.service + "/" + r.method }

// serviceRPCs reads the generated gRPC service descriptors.
//
// The descriptors are what the wire actually carries, so deriving the method
// set from them means this guard cannot disagree with the protobuf about what
// exists. A method renamed upstream disappears from here rather than sitting
// in a list nobody updated.
func serviceRPCs() map[string]rpcTarget {
	rpcs := make(map[string]rpcTarget)

	for _, desc := range []*grpc.ServiceDesc{&machine.MachineService_ServiceDesc, &storage.StorageService_ServiceDesc} {
		for _, m := range desc.Methods {
			rpcs[m.MethodName] = rpcTarget{service: desc.ServiceName, method: m.MethodName}
		}
		for _, s := range desc.Streams {
			rpcs[s.StreamName] = rpcTarget{service: desc.ServiceName, method: s.StreamName, streaming: true}
		}
	}

	return rpcs
}

// probeRPC calls one RPC against a running simulator and reports whether the
// simulator answered it at all.
//
// The request is an empty message and the reply is discarded. That is
// deliberate: this guard is asking whether the method is implemented, not
// whether its response is right -- the response shapes are what the tests in
// node_test.go and stream_test.go are for. An empty message encodes to zero
// bytes, which every proto3 request accepts, so one call shape serves every
// method the walk can turn up, including ones added in later phases that this
// file will never be edited for.
//
// Each RPC gets its own node, because some of them are mutations: probing
// Shutdown on a shared simulator would power it off underneath the probes that
// came after it.
func probeRPC(t *testing.T, target rpcTarget) error {
	t.Helper()

	sim := newSim(t, talossim.Options{Hostname: "coverage-" + strings.ToLower(target.method), StreamMessages: 1})

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	// The connection is built from the simulator's own Dialer, which is the
	// transport the production client uses against it. Probing over a
	// different one would be probing a topology the product never reaches.
	creds := sim.ClientCreds()
	node := talos.Target{Machine: model.MachineID("00000000-0000-0000-0000-0000000000cc")}

	dialOpts, err := sim.Dialer().DialOptions(ctx, node, creds)
	if err != nil {
		t.Fatalf("dial options: %v", err)
	}

	conn, err := grpc.NewClient(sim.Addr(), dialOpts...)
	if err != nil {
		t.Fatalf("dial the simulator: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Errorf("close the probe connection: %v", err)
		}
	}()

	if target.streaming {
		err = probeStream(ctx, conn, target)
	} else {
		err = conn.Invoke(ctx, target.path(), &emptypb.Empty{}, &emptypb.Empty{})
	}

	if status.Code(err) == codes.Unimplemented {
		return errors.New("the simulator answered Unimplemented; the method is inherited from " +
			"UnimplementedMachineServiceServer rather than written")
	}

	// Any other status means the method ran. A FailedPrecondition from an
	// un-bootstrapped node or an InvalidArgument from an empty request are
	// both implementations answering, which is what is being asked here.
	return nil
}

func probeStream(ctx context.Context, conn *grpc.ClientConn, target rpcTarget) error {
	stream, err := conn.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true}, target.path())
	if err != nil {
		return err
	}
	if err := stream.SendMsg(&emptypb.Empty{}); err != nil {
		return err
	}
	if err := stream.CloseSend(); err != nil {
		return err
	}

	err = stream.RecvMsg(&emptypb.Empty{})
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

// machineryCallSites walks the production source of the packages allowed to
// reach a node and returns the method names they call on a machinery client,
// with the number of packages and files the walk covered.
//
// The detection is deliberately narrow rather than clever. It resolves which
// identifiers hold a machinery client -- from struct fields, declarations,
// parameters and client.New assignments -- and then reads the method names
// called on those. The alternative, a full type check, would need
// golang.org/x/tools in the module for a guard whose whole point is that the
// module graph stays small.
func machineryCallSites(t *testing.T) (methods []string, packages, files int) {
	t.Helper()

	found := map[string]bool{}

	for _, dir := range seamPackages {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		packages++

		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}

			path := filepath.Join(dir, name)
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			files++

			for _, m := range callsIn(file) {
				found[m] = true
			}
		}
	}

	for m := range found {
		methods = append(methods, m)
	}
	sort.Strings(methods)

	return methods, packages, files
}

// clientAccessors are the fields a machinery client exposes that are
// themselves clients. A call like c.MachineClient.Hostname(...) reaches the
// same service as c.Hostname(...) and has to be found as well.
var clientAccessors = map[string]bool{
	"MachineClient": true,
	"StorageClient": true,
}

func callsIn(file *ast.File) []string {
	holders := clientHolders(file)

	var methods []string

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		var receiver string
		switch x := sel.X.(type) {
		case *ast.Ident:
			receiver = x.Name
		case *ast.SelectorExpr:
			receiver = x.Sel.Name
		default:
			return true
		}

		if holders[receiver] || clientAccessors[receiver] {
			methods = append(methods, sel.Sel.Name)
		}
		return true
	})

	return methods
}

// clientHolders returns the identifiers in one file that hold a machinery
// client: struct fields, declared variables, parameters and the results of
// client.New.
func clientHolders(file *ast.File) map[string]bool {
	alias := importAlias(file, machineryClientImport)
	holders := map[string]bool{}

	if alias == "" {
		return holders
	}

	clientType := "*" + alias + ".Client"

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.Field:
			if typeString(node.Type) == clientType {
				for _, name := range node.Names {
					holders[name.Name] = true
				}
			}
		case *ast.ValueSpec:
			if typeString(node.Type) == clientType {
				for _, name := range node.Names {
					holders[name.Name] = true
				}
			}
		case *ast.AssignStmt:
			if !callsClientNew(node.Rhs, alias) {
				return true
			}
			for _, lhs := range node.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok {
					holders[ident.Name] = true
				}
			}
		}
		return true
	})

	return holders
}

func callsClientNew(rhs []ast.Expr, alias string) bool {
	for _, expr := range rhs {
		call, ok := expr.(*ast.CallExpr)
		if !ok {
			continue
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok {
			continue
		}
		if pkg.Name == alias && sel.Sel.Name == "New" {
			return true
		}
	}
	return false
}

func importAlias(file *ast.File, path string) string {
	for _, spec := range file.Imports {
		value, err := strconv.Unquote(spec.Path.Value)
		if err != nil || value != path {
			continue
		}
		if spec.Name != nil {
			return spec.Name.Name
		}
		return path[strings.LastIndex(path, "/")+1:]
	}
	return ""
}

func typeString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return "*" + typeString(t.X)
	case *ast.SelectorExpr:
		return typeString(t.X) + "." + t.Sel.Name
	case *ast.Ident:
		return t.Name
	default:
		return ""
	}
}
