package audit

// Allowlist redaction of input parameters (D-14).
//
// The direction of this file is the whole point. A list of forbidden fields
// forgets the next secret: it protects against the fields somebody thought of,
// and quietly passes through the one added in phase 7 by an author who never
// read this file. An allowlist fails the other way -- a new parameter shows up
// as a marker until somebody deliberately adds it here -- and the failure is
// visible, recoverable and costs nothing. There is no list of exclusions in
// this package and there is no branch that passes an unrecognised value
// through; both absences are asserted by the plan's gate greps.
//
// The stakes are not ordinary. holzkube-manager holds cluster PKI, and D-16 keeps every
// rotated file forever, so a secret written here is written permanently: there
// is no path that deletes it and no path that could delete it without breaking
// the chain. A log that holds secrets would have to be guarded more carefully
// than the things it is meant to keep an eye on.

import (
	"encoding/json"
	"slices"
	"strings"
	"unicode/utf8"
)

// RedactedMarker replaces every value that is not explicitly permitted.
//
// The key survives, the value does not: knowing that a field was sent is
// forensically useful, and knowing what was in it is exactly the risk.
const RedactedMarker = "<redacted>"

const (
	// maxParamLen caps a permitted value. Without it a caller could park a
	// megabyte in an allowlisted field of an archive nothing ever removes.
	maxParamLen = 256

	truncationMarker = "…(truncated)"

	// maxParamItems caps a permitted list. An allowlisted field that is a list
	// is a list of identifiers -- patch ids, addresses to scan -- and a
	// hundred of them is not a longer record, it is somebody using an
	// allowlisted name as storage in an archive nothing removes.
	maxParamItems = 32
)

// allowlist maps an action token to the parameter paths that may appear in
// clear. Nested paths are dotted; every entry names a leaf, never a branch.
//
// Phase 1 permits exactly two things, both of them identifiers the operator
// chose for themselves and neither of them a credential. Everything else --
// passwords on setup, login and password change, and every field of every
// action added later -- is redacted until listed here on purpose.
//
// The test for whether a field belongs here is not "is it interesting" but "is
// it an identifier the operator chose, and is it certainly not a credential".
// schematic.create is the case that makes the difference concrete. Its body
// carries kernel arguments and META values, and the Image Factory itself
// refuses to enumerate schematics precisely because those may hold secrets. D-16
// keeps every rotated audit file forever and defines no deletion path, so a
// kernel argument written in clear here is written in clear permanently -- there
// is no migration and no redaction pass that could take it back without breaking
// the hash chain. So schematic.create permits the operator's own label and the
// Talos version, and the two fields that could carry a secret are left to the
// fail-closed default along with everything else.
var allowlist = map[string][]string{
	"setup.create": {"username"},
	"auth.login":   {"username"},

	// The schematic's label and the version it targets: an operator-chosen
	// name and a public version string. Deliberately NOT kernel_args, meta,
	// extensions or canonical -- see the paragraph above.
	"schematic.create": {"name", "talos_version"},

	// Listed with nothing permitted, so the table shows the full set of
	// mutations rather than leaving any of them to the default.
	"auth.logout":      {},
	"auth.sudo":        {},
	"account.password": {},

	// A deletion carries its id in the path, not in the body: there is nothing
	// here worth writing in clear.
	"schematic.delete": {},

	// The inventory, phase 3.
	//
	// cluster.import is the entry this table was most at risk of getting
	// wrong, and the risk runs in one direction only. Its body carries a
	// talosconfig: a certificate authority, a client certificate and a client
	// *private key*. That field is deliberately absent from this list, so the
	// fail-closed default writes `<redacted>` for it -- which is the whole
	// reason this file is an allowlist. Permitted here are the cluster's name,
	// the address the operator named and the fingerprint they confirmed:
	// an operator-chosen label and two facts that are already public to
	// anybody who can reach the node.
	"cluster.import":      {"name", "endpoint", "fingerprint"},
	"cluster.fingerprint": {"endpoint"},

	// A created cluster names itself and its Kubernetes endpoint. Neither is a
	// secret; what this action generates is, and that never enters a request
	// body at all.
	"cluster.create": {"name", "endpoint"},

	// Whether the lock was opened or closed is the entire content of the
	// event, and it is not a secret. A lock change with a redacted direction
	// would be a record that says something happened and not what.
	"cluster.lock": {"locked"},

	// Account management, v1.16 phase 3. Four entries, and what they permit is
	// decided by one question: six months from now, what does somebody
	// investigating need this record to say?
	//
	// The username and the role, because "an account was created" without them
	// is a record that cannot answer "who can reach this cluster and since
	// when". The password never, on any of them -- the fail-closed default
	// writes <redacted> and this is the case it was built for.
	"user.create": {"username", "role"},

	// The role a change moved somebody to. The account is in the path. A role
	// change with a redacted direction is a record of nothing, which is the
	// same argument cluster.lock makes.
	"user.role": {"role"},

	// A reset has nothing permissible in its body at all, and the entry exists
	// so the table says so rather than leaving it to be inferred. The account
	// whose password was reset is in the path, and that is the fact.
	"user.password-reset": {},

	// Likewise: the account is in the path and there is no body.
	"user.delete": {},

	// Service accounts, v1.16 phase 4. The username and the role, for the same
	// reason user.create permits them: "a machine identity was created"
	// without them cannot answer who can reach this cluster and since when.
	//
	// The token never appears, and here that is not the fail-closed default
	// doing the work -- it is not in the request body at all. It is minted
	// server-side and returned once, so there is nothing for a request log to
	// capture even in principle.
	"service-account.create": {"username", "role"},

	// A rotation has no body. The account is in the path, and the fact is that
	// its previous token stopped working at this moment -- which is exactly
	// what somebody investigating a machine that suddenly got 401s needs.
	"service-account.rotate": {},

	// Labels and machine classes, v1.16 phase 5.
	//
	// The labels themselves are permitted in clear, and that is a decision
	// rather than a default. They are the operator's own words about their own
	// machines -- "rack=b3", "storage=nvme" -- and a record saying "somebody
	// relabelled a node" without saying to what answers nothing. What makes
	// that safe here and not elsewhere is that nothing observed writes a
	// label, so a label can never be a value a node reported.
	"machine.labels": {"labels"},

	// The class as a whole: its name, what it is for, and the conditions it
	// selects on. Six months later the archive is the only place that says
	// what a class meant at the moment it was used.
	"machine-class.put": {"name", "description", "selector"},

	// The class is in the path and there is no body.
	"machine-class.delete": {},

	// The cluster-template plan, v1.16 phase 6. Nothing is permitted in clear,
	// and the reason is the body rather than the fields: it is a YAML document
	// and the audit middleware captures a decoded JSON one, so there are no
	// parameters here to allow or refuse.
	//
	// The event is worth recording anyway. A plan changes nothing, but it is
	// what somebody reads immediately before changing something, and an
	// archive that holds the change and not the question that preceded it is
	// an archive missing the half that explains it.
	"cluster-template.plan": {},

	// The kubeconfig fetch, v1.16 phase 2. The cluster is in the path and
	// there is no body, so the list is empty -- and the entry exists because
	// the event does, which is the whole reason this route is audited when the
	// talosconfig download beside it is a plain link. What it hands over is
	// system:masters on somebody's cluster, and "who asked for this and when"
	// is precisely what an archive is for.
	"cluster.kubeconfig": {},

	// The cluster this machine was added to and the address it was found at.
	// Neither is a credential; the credentials are the cluster's, and they are
	// not in this body at all.
	"machine.add": {"cluster", "addr"},

	// The three audited *reads*. They are GETs, so there is no body to permit
	// anything out of, and the table lists them with nothing rather than
	// leaving them to the default -- which is the distinction this table
	// claims to make about itself and which cmd/holzkube-managerd's
	// allowlist_test.go is what actually holds.
	"system.status": {},
	// A read with no parameters at all: the route takes nothing and the
	// response is the same for every caller who may ask.
	"system.version": {},
	"auth.me":        {},
	"audit.list":     {},

	// Listed with nothing permitted, so the table shows the full set of
	// mutations rather than leaving any of them to the default. Each carries
	// its id in the path.
	"cluster.forget":  {},
	"machine.forget":  {},
	"machine.refresh": {},

	// Node actions, phase 6.
	//
	// The *scope of a wipe* is the single most important thing this archive
	// can hold about a reset: "a reset happened on node X" and "every disk on
	// node X was wiped" are different events, and a record that cannot tell
	// them apart is a record nobody can answer a question with. None of these
	// is a credential -- they are the choices an operator made on a screen.
	//
	// `confirmation` is deliberately absent from every one of them. It is an
	// HMAC over the intent, it is the thing that authorises the action, and an
	// archive with no deletion path is the last place it belongs.
	"node.reboot":   {"cluster"},
	"node.shutdown": {"cluster"},
	"node.reset": {
		"cluster",
		"params.mode", "params.graceful", "params.reboot", "params.disks[]",
	},

	// What was confirmed, so that a token issued and never used still leaves a
	// record of what somebody was about to do. `typed` is absent: it is the
	// machine's hostname, which the record already carries, and writing back
	// what a person typed into a box is a habit worth not starting.
	"action.confirm": {"action", "params.mode", "params.graceful", "params.reboot"},

	// A cancel names its job in the path.
	"job.cancel": {},

	// Machine configuration, phase 7.
	//
	// The patch *ids* are permitted and the patch *bodies* are not. A body is
	// arbitrary configuration, configuration is where the secrets are, and
	// this archive is kept forever. A stored patch stays readable at its own
	// id for as long as it exists -- which is also forever -- so the record is
	// complete without the archive holding the bytes.
	"config.plan":  {"cluster", "patch_ids[]"},
	"config.apply": {"cluster", "mode", "patch_ids[]"},

	// The patch's name, and deliberately not its body, for the reason above.
	"patch.create": {"name"},

	// Provisioning, phase 8.
	//
	// Everything here is an address, an identifier or a choice the operator
	// made on a screen, and the one field that could be a credential --
	// `confirmation` -- is absent, as it is for every node action.
	//
	// The scan's own addresses are worth keeping in clear for the reason the
	// reset's disks are: "a scan happened" and "this installation scanned
	// 10.0.0.0/24" are different events, and the second is the one somebody
	// reviewing an unexpected connection on their network is looking for.
	"provision.scan": {"cidr", "addrs[]"},
	"provision.confirm": {
		"cluster", "addr", "uuid", "control_plane", "install_disk",
		"schematic_id", "talos_version", "fingerprint", "hostname", "patch_ids[]",
	},
	"provision.inspect": {"addr", "fingerprint"},

	// The plan and the apply take the same body, so they permit the same
	// fields: what was shown on the screen is what was submitted, and a record
	// where the two differ in shape would be a record that cannot be compared.
	"provision.plan": {
		"cluster", "addr", "uuid", "control_plane", "install_disk",
		"schematic_id", "talos_version", "fingerprint", "hostname", "patch_ids[]",
	},
	"provision.apply": {
		"cluster", "addr", "uuid", "control_plane", "install_disk",
		"schematic_id", "talos_version", "fingerprint", "hostname", "patch_ids[]",
	},

	// The operator's verdict on an unclear bootstrap, and what they looked at.
	// This record is the only account anything will ever have of a bootstrap
	// nothing could decide, and redacting the note would leave the archive
	// saying that somebody decided, without what they decided or why.
	"provision.bootstrap-resolve": {"bootstrapped", "note"},

	// Upgrades and etcd management, phase 9.
	//
	// The version is the whole content of an upgrade record. "A Talos upgrade
	// was run on this cluster" and "this cluster was taken from 1.13.9 to
	// 1.14.0 on the eleventh" are different events, and only the second one
	// answers the question somebody asks six months later about when a
	// behaviour changed.
	//
	// `confirmation` is absent from the two that carry one, as it is
	// everywhere: it is the HMAC that authorises the run.
	"upgrade.plan":            {"to"},
	"upgrade.plan-kubernetes": {"to"},
	"upgrade.confirm":         {"kind", "to"},
	"upgrade.talos":           {"to"},
	"upgrade.kubernetes":      {"to"},

	// The member removal names its member in the path and the cluster in the
	// path; there is no body worth permitting anything out of. It is listed so
	// the table shows it.
	"etcd.remove-member": {},

	// Reading what Kubernetes says about a cluster (milestone v1.17). The
	// namespace is permitted in clear because it is the one thing that says
	// what was looked at, and a namespace name is not a secret -- while
	// "<redacted>" here would leave an archive recording that somebody looked
	// at something. The cluster is in the path and on the record already.
	"cluster.kubernetes-overview": {"namespace"},

	// Cordoning a node, and draining one (milestone v1.17 slice 3). The state
	// asked for is permitted in clear because "somebody cordoned a node" and
	// "somebody uncordoned it" are different events and an archive that could
	// not tell them apart would be recording neither. The drain's two flags are
	// permitted for the same reason and a sharper one: --force and
	// --delete-local-data are the decisions that can lose work, and six months
	// later the archive is the only place it still says whether they were
	// taken.
	"cluster.kubernetes-cordon": {"unschedulable"},
	"node.drain":                {"force", "delete_local_data"},

	// The pod and workload actions (milestone v1.17 slice 4). The namespace and
	// the name are in the path and on the record already; what is worth keeping
	// in clear is the NUMBER a scale was set to, because "somebody scaled this
	// to zero" and "somebody scaled it to ten" are different events and an
	// archive that could not tell them apart would be recording neither.
	"cluster.kubernetes-restart-pod":     {},
	"cluster.kubernetes-scale":           {"replicas"},
	"cluster.kubernetes-rollout-restart": {},

	// Applying a manifest (milestone v1.17 slice 5). The manifest itself is
	// deliberately NOT in clear, and the reason is not its size: a manifest
	// carries Secrets. A `kind: Secret` in an archive is a credential in an
	// archive, and the archive is the one thing in this product that is meant
	// to be readable later by people who were not there.
	//
	// What the archive gets instead is the job's own record of which objects
	// were applied, which is what somebody reading it later actually needs.
	"cluster.kubernetes-manifest-plan":  {},
	"cluster.kubernetes-manifest-apply": {},

	// Reaching a workload through the cluster (milestone v1.17 slice 6). The
	// namespace and the service are in the path; the PORT and the PATH are kept
	// in clear, because "somebody read /healthz" and "somebody read
	// /admin/users" are different events and an archive that could not tell
	// them apart would be recording neither. That is also why the route is a
	// POST: this middleware captures the body and not the query string, so a
	// GET would have archived neither. The answer is never archived -- it is a
	// workload's response and may be anything at all.
	"cluster.kubernetes-service-proxy": {"port", "path"},

	// Reading what is wrong (Stufe 1, 2026-09-19). All four are reads and all
	// four are empty, and the emptiness is the decision rather than the absence
	// of one.
	//
	// NOTHING of a log's CONTENT may be archived. A log line is whatever the
	// workload printed -- tokens, connection strings, somebody's name -- and
	// D-16 keeps the archive forever, so a log in it is a secret in it with no
	// path that removes it. The archive records that somebody read the log of a
	// named pod, which is the event; the pod is in the route's path and is on
	// the record already.
	//
	// The container name is not archived either, and that is why the log route
	// is a GET with the container in the query while the service proxy had to
	// be a POST: there the path being fetched WAS the event, here the event is
	// the pod.
	"cluster.kubernetes-containers": {},
	"cluster.kubernetes-logs":       {},
	"cluster.kubernetes-events":     {},
	"cluster.kubernetes-pod-events": {},

	// Reading one object as YAML. WHICH object is the event -- "somebody read
	// the whole of this Deployment" -- so the four fields that name it are kept
	// in clear, and that is why the route is a POST: nothing else identifies
	// the object, not even a path segment, and this middleware reads bodies
	// rather than query strings (ledger 150).
	//
	// The rendered object itself is never archived: it is arbitrary cluster
	// configuration and the archive is kept forever.
	"cluster.kubernetes-object": {"api_version", "kind", "namespace", "name"},

	// Acting as somebody (2026-09-19). WHO this product will act as is the
	// event and is kept in clear: it decides what every later Kubernetes entry
	// in this archive means, and an archive that recorded the change without
	// the name would leave every entry after it ambiguous.
	//
	// The preview is a read and carries nothing: the candidate is in the query
	// string, and the answer is the cluster's RBAC rather than this
	// installation's.
	"cluster.kubernetes-identity":     {},
	"cluster.kubernetes-set-identity": {"user", "groups"},

	// The other workload kinds. The kind, namespace and name are in the path
	// and on the record already; the replica count is kept for the reason the
	// deployment scale's is -- "scaled to zero" and "scaled to ten" are
	// different events.
	"cluster.kubernetes-workloads":        {},
	"cluster.kubernetes-scale-workload":   {"replicas"},
	"cluster.kubernetes-restart-workload": {},

	// The objects beside the workloads, and removing one. WHAT was removed is
	// the event and is kept in clear, which is why the delete is a POST: this
	// middleware reads bodies, and a DELETE with the object in the query would
	// record that somebody deleted something (ledger 150).
	"cluster.kubernetes-resources":     {},
	"cluster.kubernetes-delete-object": {"api_version", "kind", "namespace", "name"},

	// One node's conditions, taints and how much room the scheduler has left.
	// A read; the node is in the path.
	"cluster.kubernetes-node-detail": {},

	// What nodes and pods are using. A read, and the answer is the cluster's
	// own measurement rather than anything this installation holds.
	"cluster.kubernetes-usage": {},

	// What runs and what it uses, as apps (2026-09-26). Two reads; the
	// namespace and node filters are in the query and the app is in the path,
	// which the archive records, so there is no body to carry.
	"cluster.kubernetes-apps": {},
	"cluster.kubernetes-app":  {},

	// Running a command in a container. THE COMMAND IS THE EVENT and every
	// argument is archived in clear -- which is only possible because a shell
	// with a string is refused: "sh -lc" plus one opaque argument would be a
	// record that cannot say what happened.
	//
	// The OUTPUT is never archived: it is whatever the workload printed.
	"cluster.kubernetes-exec": {"container", "command"},

	// How full the cluster is; stopping and starting; clearing out (2026-09-20).
	//
	// The kind, namespace and name of a stop or start are in the path and on
	// the record already. The sweep's LIST is kept in clear: what was removed
	// is the event, and an archive that recorded "swept 41 things" would be a
	// record nobody could check afterwards -- these deletions are the ones
	// nothing puts back.
	"cluster.kubernetes-capacity": {},

	// Storage and networking (2026-09-20). Both read and neither writes, and
	// the namespace they are narrowed to is in the query rather than the body --
	// so there is nothing here for the archive to carry. Written as an empty
	// entry deliberately, which is this table saying so rather than nobody
	// having looked.
	"cluster.kubernetes-storage": {},
	"cluster.kubernetes-network": {},

	// The power model (2026-09-26): seven verbs for a cluster, a node and an
	// app, one audited action each. No body is sent and none is read: WHAT was
	// done is the action token itself -- which is why there is one route per
	// verb rather than one with the verb as a parameter -- and what it was done
	// to is the cluster, machine or app in the path. Every entry is empty on
	// purpose, which is this table saying so rather than nobody having looked.
	"cluster.stop":          {},
	"cluster.force-stop":    {},
	"cluster.start":         {},
	"cluster.disable":       {},
	"cluster.enable":        {},
	"cluster.restart":       {},
	"cluster.force-restart": {},

	"node.stop":          {},
	"node.force-stop":    {},
	"node.start":         {},
	"node.disable":       {},
	"node.enable":        {},
	"node.restart":       {},
	"node.force-restart": {},

	"cluster.kubernetes-app-stop":          {},
	"cluster.kubernetes-app-force-stop":    {},
	"cluster.kubernetes-app-start":         {},
	"cluster.kubernetes-app-disable":       {},
	"cluster.kubernetes-app-enable":        {},
	"cluster.kubernetes-app-restart":       {},
	"cluster.kubernetes-app-force-restart": {},

	// Who may do what (2026-09-20). A read, with the namespace in the query
	// rather than the body, so there is nothing for the archive to carry. That it
	// was READ is the record worth having: somebody enumerated the cluster's
	// administrators, which is a reasonable thing to do and a reasonable thing to
	// be able to see afterwards.
	"cluster.kubernetes-access": {},

	// Every namespace, its quotas, and the cluster's own kinds (2026-09-20). A
	// read with no parameters at all: the answer IS the namespaces, so there is
	// nothing to narrow and nothing for the archive to carry.
	"cluster.kubernetes-inventory": {},

	// The wall (2026-09-20). A read with only a namespace in the query, so there
	// is nothing to carry -- and the record it leaves is the point of contention
	// worth stating: a screen in the IT office polls this every few seconds for
	// weeks, so the archive would fill with one action and nothing else. The
	// middleware records a request per call regardless; this table's job is only
	// to say what may appear in clear, and the answer is nothing.
	"cluster.wall": {},

	// The links a screen in a corridor is left open on (2026-09-20). The LABEL
	// is kept in clear and the token never is: "which screen was this" is the
	// whole question a revocation asks afterwards, and an archive that recorded
	// only that a link was created could not answer it. The token is not in the
	// body of any of the three -- it is minted on the server and returned once --
	// so there is nothing here for the archive to leak.
	"wall-link.list":                {},
	"wall-link.create":              {"label"},
	"wall-link.revoke":              {},
	"cluster.kubernetes-stop":       {},
	"cluster.kubernetes-start":      {},
	"cluster.kubernetes-sweep-plan": {},
	"cluster.kubernetes-sweep":      {"items"},

	// Renewing this installation's own client certificate for a cluster
	// (V2-OPS-02). No parameters: the cluster is on the record already and the
	// certificate itself never goes near the archive.
	"cluster.renew-client-certificate": {},

	// A snapshot is a GET: no body, nothing to permit. It is listed for the
	// same reason -- and because "somebody took a copy of this cluster's etcd"
	// is exactly the kind of event an archive exists to hold, even when the
	// record is only that it happened.
	"etcd.snapshot": {},

	// A restore, and the most consequential entry in this table. The node is
	// in the path; what is worth keeping in clear is whether the operator
	// turned off the snapshot's integrity check, because a restore from a
	// data-directory copy and a restore from an API snapshot are two different
	// claims about what the cluster now holds, and six months later the
	// archive is the only place that difference still exists.
	//
	// The snapshot itself is the request body and never reaches the archive:
	// the audit middleware captures a decoded JSON body, and this body is an
	// etcd database.
	"etcd.restore": {"skip_hash_check"},

	// Whether a node was locked, and why. The reason is the content: a lock
	// record that says a lock happened and not why is a record of nothing, and
	// the reason is an operator's own sentence about their own fleet.
	"machine.lock": {"locked", "reason"},

	// The cluster a node was removed from. The node is in the path.
	"node.remove-from-cluster": {"cluster"},

	// The support bundle, v1.15. A GET naming its cluster in the path, so
	// there is no body to permit anything out of -- and it is listed rather
	// than left to the default because "somebody took a copy of everything
	// this installation knows about a cluster" is exactly the event an archive
	// exists to hold, even when the record is only that it happened.
	"support.bundle": {},
}

// Listed reports whether an action has an entry in the table.
//
// It is not used by the redaction -- an action with no entry redacts
// everything, which is the fail-closed default and is correct. It exists so
// that the composition root can assert the other half of the claim this table
// makes about itself: that it shows the **full set** of mutations, including
// the ones that permit nothing.
//
// The difference matters because the two look identical from inside this
// package. An action deliberately listed as permitting nothing and an action
// whose author never read this file both redact everything; only the table can
// tell them apart, and only something that knows every action there is can
// tell whether the table is complete.
func Listed(action string) bool {
	_, ok := allowlist[action]
	return ok
}

// ListedActions returns every action the table names, in no particular order.
//
// It is for the guard described on Listed, in the other direction: an entry for
// an action nothing emits is a decision about a field that no longer exists.
func ListedActions() []string {
	out := make([]string, 0, len(allowlist))
	for action := range allowlist {
		out = append(out, action)
	}
	return out
}

// Params returns the parameters as they may be written to the log.
//
// An action with no entry in the table redacts everything. That is the default
// on purpose: forgetting to extend the allowlist costs a useful record, while
// the opposite default would cost a secret.
func Params(action string, raw map[string]any) map[string]any {
	// Allowlist entries are authored as dotted paths and are split into
	// segments here, because a dot in the table means "descend" while a dot in
	// a JSON key is just a character. Comparing the joined strings conflated
	// the two: an entry "user.name", meant to permit {"user":{"name":...}},
	// also permitted a body sending the flat key {"user.name": "..."}. JSON
	// object keys may contain dots and readBody decodes whatever arrives, so
	// the two namespaces have to be kept apart.
	permitted := make([]leafRule, 0, len(allowlist[action]))
	for _, f := range allowlist[action] {
		rule := leafRule{}
		if strings.HasSuffix(f, listSuffix) {
			rule.list = true
			f = strings.TrimSuffix(f, listSuffix)
		}
		rule.path = strings.Split(f, ".")
		permitted = append(permitted, rule)
	}

	out := make(map[string]any, len(raw))
	for k, v := range raw {
		out[k] = redactValue([]string{k}, permitted, v)
	}
	return out
}

// leafRule is one allowlist entry, parsed.
//
// list records that the entry was written with a `[]` suffix, which is the
// author saying this field holds a list of identifiers rather than one. It is
// spelled out per entry rather than inferred from what arrives, because
// "username" is a scalar field and a caller sending a list there is a caller
// sending a shape that field does not have -- which is exactly the smuggling
// the allowlist is for.
type leafRule struct {
	path []string
	list bool
}

// listSuffix marks an allowlist entry as naming a list of scalars.
const listSuffix = "[]"

// redactValue decides one node of the parameter tree.
//
// There is no branch that returns an unrecognised value unchanged. Everything
// either matches an allowlisted leaf path and is a scalar, or is a container on
// the way to one, or becomes the marker.
func redactValue(segments []string, permitted []leafRule, v any) any {
	if leaf, ok := permittedLeaf(segments, permitted); ok {
		if s, ok := permittedValue(v, leaf.list); ok {
			return s
		}
		// An allowlist entry names a leaf. A caller that sends an object where
		// a string was expected would otherwise smuggle arbitrary content past
		// the list under a permitted name -- and walking it would publish its
		// shape, which for a secrets bundle is itself a map of where to look.
		return RedactedMarker
	}

	if m, ok := v.(map[string]any); ok && leadsToPermitted(segments, permitted) {
		out := make(map[string]any, len(m))
		for k, vv := range m {
			// Full slice expression: without capping the capacity, every key
			// of this object would append into the same backing array and
			// siblings would overwrite each other's segment.
			child := append(segments[:len(segments):len(segments)], k)
			out[k] = redactValue(child, permitted, vv)
		}
		return out
	}

	// An unknown branch is replaced whole, keys included. Walking it would
	// publish its shape, and the shape of a secrets bundle is itself a map of
	// where to look.
	return RedactedMarker
}

// permittedLeaf returns the rule for an allowlisted leaf named exactly.
func permittedLeaf(segments []string, permitted []leafRule) (leafRule, bool) {
	for _, p := range permitted {
		if slices.Equal(p.path, segments) {
			return p, true
		}
	}
	return leafRule{}, false
}

// leadsToPermitted reports whether any allowlisted path continues below here.
func leadsToPermitted(segments []string, permitted []leafRule) bool {
	for _, p := range permitted {
		if len(p.path) > len(segments) && slices.Equal(p.path[:len(segments)], segments) {
			return true
		}
	}
	return false
}

// permittedValue accepts what a leaf may carry.
//
// list is the entry's own `[]` marking. A field marked as a list accepts a
// list of scalars, and the reason it exists at all is a record that is
// otherwise incomplete: `config.apply` names its patches by id, and a run
// whose ids are `<redacted>` is a record that says a configuration was applied
// and cannot say which one. It is not an exemption from the rule -- every
// element goes through this function itself, an over-long list is refused, and
// a list holding an object or another list is refused whole.
//
// A field not marked as a list refuses one, exactly as before: "username" is a
// scalar field, and a caller sending a list there is sending a shape that
// field does not have.
//
// An object under a permitted name is always the marker, marked or not:
// walking it would publish its shape, and the shape of a secrets bundle is
// itself a map of where to look.
func permittedValue(v any, list bool) (any, bool) {
	switch t := v.(type) {
	case nil:
		return nil, true
	case bool:
		return t, true
	case json.Number:
		return t, true
	case float64, int, int64, uint64:
		return t, true
	case string:
		return capLength(t), true
	case []any:
		if !list || len(t) > maxParamItems {
			return nil, false
		}
		out := make([]any, 0, len(t))
		for _, item := range t {
			// Never `list` again: a list of lists is a structure, and this
			// permits one list of scalars rather than a tree of them.
			s, ok := permittedValue(item, false)
			if !ok {
				// One element that is not a scalar refuses the whole list
				// rather than a list with a marker in it: a partially redacted
				// list reads as a complete one.
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	default:
		return nil, false
	}
}

func capLength(s string) string {
	if len(s) <= maxParamLen {
		return s
	}
	// Cut on a rune boundary. Half a rune is invalid UTF-8, and the canonical
	// form -- which the hash is taken over -- has to stay well-defined.
	cut := maxParamLen
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + truncationMarker
}

// maxCodeLen bounds a taxonomy code. The longest one the taxonomy defines is
// well under this; the cap is here so that a value which merely happens to be
// token-shaped still cannot be arbitrarily long.
const maxCodeLen = 64

// OutcomeCause returns a failure reason as it may be written to the log.
//
// An outcome names a stable taxonomy code -- "sudo.required",
// "validation.failed", "http.500" -- and never free text. This is the door that
// enforces it, rather than whichever call site happened to remember: Outcome is
// exported, and the first caller to pass a real error would otherwise write a
// filesystem path or a store message straight into an archive that is kept
// forever and has no deletion path.
//
// A value that is not code-shaped is redacted rather than truncated. Truncating
// would keep the first 256 characters of exactly the free text this rejects.
func OutcomeCause(s string) string {
	if !isCodeToken(s) {
		return RedactedMarker
	}
	return s
}

// isCodeToken checks the shape of a taxonomy code before it is copied into a
// permanent record: lowercase, digits and the three separators the taxonomy
// uses, and nothing else. It is deliberately a duplicate of the check the audit
// middleware runs on the way out of a problem response; the invariant belongs
// to this package, and a check that lives only in the caller is a check the
// next caller does not have.
func isCodeToken(s string) bool {
	if s == "" || len(s) > maxCodeLen {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}
