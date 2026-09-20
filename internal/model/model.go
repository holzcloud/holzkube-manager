// Package model holds the record types shared across holzkube-manager.
//
// UserID, ClusterID and MachineID are distinct named types on purpose. Making
// them separate types costs nothing today and is the entire multi-cluster
// insurance policy: the compiler finds every place a scope was forgotten.
package model

import "time"

// UserID identifies an operator account.
type UserID string

// ClusterID identifies a Talos cluster. Nothing in phase 1 uses it yet; it
// exists so that later phases cannot silently omit cluster scoping.
type ClusterID string

// MachineID is a machine's UUID.
type MachineID string

// SchematicID identifies an Image Factory schematic.
//
// The value is the 64-character lowercase hex SHA-256 the Factory assigns, and
// that shape is why it can be a filename unchanged: it passes the store's key
// check without escaping, so nothing has to invent a mapping between an
// identifier and a path. It is a distinct type for the same reason UserID and
// ClusterID are -- the compiler finds the place someone passes the wrong one.
type SchematicID string

// User is an operator account. holzkube-manager is a single-operator tool, but the
// record is shaped so that a future OIDC or multi-user layer has somewhere to
// go without a migration.
type User struct {
	ID           UserID    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"password_hash"`
	CreatedAt    time.Time `json:"created_at"`

	// Kind separates a person from a machine.
	//
	// It is empty on every account created before service accounts existed,
	// and OrPerson reads empty as a person -- which is what those accounts
	// are. The distinction is not cosmetic: a person signs in with a password
	// and holds a cookie, a service account presents a token on every request,
	// and each of those is refused the other's way in.
	Kind UserKind `json:"kind,omitempty"`

	// TokenHash is the SHA-256 of a service account's token, hex encoded, and
	// empty for a person.
	//
	// SHA-256 and not argon2id, and that is a decision rather than an
	// oversight. argon2id exists to make a *low-entropy* secret expensive to
	// guess; a token here is 256 bits from crypto/rand, and no amount of
	// stretching improves on that while every stretch is paid on every single
	// API call a machine makes. What matters instead is that the comparison is
	// constant-time and that the token is never stored.
	TokenHash string `json:"token_hash,omitempty"`

	// TokenIssuedAt is when the current token was minted, so an operator
	// looking at a list can see which accounts are holding old credentials.
	TokenIssuedAt time.Time `json:"token_issued_at,omitzero"`

	// TokenExpiresAt is when this token stops authenticating, and the zero
	// value means never.
	//
	// Never is the right default and stays the default: a service account is a
	// machine identity that runs a backup at three every morning, and an expiry
	// nobody is awake to renew is an outage rather than a safeguard.
	//
	// It exists for the one account that must NOT outlive its errand: the
	// break-glass token minted on the machine itself (see the break-glass
	// subcommand). That token is handed out without anybody signing in, so the
	// only thing standing between it and a forgotten credential in somebody's
	// shell history is this field.
	TokenExpiresAt time.Time `json:"token_expires_at,omitzero"`

	// LastUsedAt is the last time this account authenticated.
	//
	// It is written best-effort and throttled -- see auth.tokenUseThrottle --
	// because writing it on every request would put a store write in front of
	// every API call a machine makes, and lose a revision race with whatever
	// that call was about to do.
	LastUsedAt time.Time `json:"last_used_at,omitzero"`

	// Role is what this account is allowed to do.
	//
	// It is stored rather than derived, and it is empty on every account
	// created before roles existed. UserRole.OrAdmin is what reads it, and it
	// reads an empty value as admin: an installation that had one account had
	// an account that could do everything, and quietly demoting it on upgrade
	// would lock the operator out of their own instance.
	Role UserRole `json:"role,omitempty"`

	// Issuer and Subject bind this account to an external identity. Both are
	// empty until the operator has signed in through the provider once.
	//
	// Subject is the join key rather than the username, because a username can
	// be reassigned to a different person at the provider while `sub` is
	// defined to be stable and never reused. Issuer is stored alongside it
	// because `sub` is only unique within one issuer: without it, changing
	// providers would silently match the new provider's subjects against the
	// old one's bindings.
	Issuer  string `json:"issuer,omitempty"`
	Subject string `json:"subject,omitempty"`

	// Rev is the compare-and-swap revision. Every stored record carries one.
	Rev uint64 `json:"rev"`
}

// UserKind is what kind of thing an account is.
type UserKind string

const (
	// KindPerson signs in with a password, holds a session cookie, and is what
	// every account was before service accounts existed.
	KindPerson UserKind = "person"

	// KindService presents a token on every request and never holds a session.
	KindService UserKind = "service"
)

// OrPerson reads an unset kind as a person, which is what every account stored
// before service accounts existed is.
func (k UserKind) OrPerson() UserKind {
	if k == "" {
		return KindPerson
	}
	return k
}

// IsService reports whether this account authenticates with a token.
func (u User) IsService() bool { return u.Kind.OrPerson() == KindService }

// UserRole is what an account may do (V2-AUTH-02).
//
// Three and not more. Every role beyond these is a policy this product would
// have to keep in step with a surface that grows every phase, and the
// distinctions that actually matter in a homelab are: can this person change
// the fleet, and can they destroy part of it.
type UserRole string

const (
	// RoleReader may look and may not change anything. It is the role for the
	// person who is on call and not on the hook -- and for the dashboard left
	// open on a screen in the hallway.
	RoleReader UserRole = "reader"

	// RoleOperator may run the fleet: upgrade, configure, provision, reboot.
	// It may not do the two things whose blast radius is the instance rather
	// than a node -- manage accounts, and hand out the credentials that make
	// this instance unnecessary.
	RoleOperator UserRole = "operator"

	// RoleAdmin may do everything, including everything above.
	RoleAdmin UserRole = "admin"
)

// UserRoles is every role, most privileged first.
//
// The order is load-bearing: it is what "at least this role" is decided
// against, and a role added in the wrong place silently widens or narrows
// every route at once. TestRoleOrderIsPrivilegeOrder holds it.
func UserRoles() []UserRole { return []UserRole{RoleAdmin, RoleOperator, RoleReader} }

// Valid reports whether r is one of the three.
func (r UserRole) Valid() bool {
	for _, known := range UserRoles() {
		if r == known {
			return true
		}
	}
	return false
}

// OrAdmin reads an unset role as admin.
//
// Every account created before roles existed has an empty one, and that
// account was the only account -- it could do everything. Reading empty as
// "reader" would demote an operator out of their own instance on an upgrade,
// which is a lockout dressed up as a security improvement. Reading it as admin
// changes nothing for them and is what the record actually meant.
func (r UserRole) OrAdmin() UserRole {
	if r == "" {
		return RoleAdmin
	}
	return r
}

// AtLeast reports whether this role carries the privileges of want.
func (r UserRole) AtLeast(want UserRole) bool {
	have, wanted := -1, -1
	for i, role := range UserRoles() {
		if r.OrAdmin() == role {
			have = i
		}
		if want == role {
			wanted = i
		}
	}
	// An unknown role on either side is refused rather than ordered. A stored
	// value nobody recognises is not a privilege level, and guessing one is
	// how a typo becomes an escalation.
	if have < 0 || wanted < 0 {
		return false
	}
	return have <= wanted
}

// HasIdentityBinding reports whether this account is linked to a provider
// identity.
func (u User) HasIdentityBinding() bool {
	return u.Issuer != "" && u.Subject != ""
}

// Settings is the singleton instance-wide settings record.
type Settings struct {
	// SetupCompleted is written true when the first account is created, and
	// **nothing reads it**. That is deliberate rather than an oversight, and
	// it is written down because the field's name says the opposite.
	//
	// Whether setup has run is decided by asking whether any account exists
	// (handlers/setup.go), which is derived from the thing that actually
	// matters and cannot drift from it. A flag can: a store restored from a
	// partial backup, or a settings record written by a future version, would
	// give an answer the user list contradicts -- and then there would be two
	// rules for one question, which is the failure this codebase keeps
	// finding.
	//
	// It is kept because the record is also when setup happened, which is
	// worth having, and removing an entity costs a migration for no gain.
	SetupCompleted bool      `json:"setup_completed"`
	CreatedAt      time.Time `json:"created_at"`

	// WallLinks are the long-lived read-only links a screen in a corridor is
	// left open on.
	//
	// Kept on the settings record rather than in a store of their own, and that
	// is a judgement worth stating: they are instance-wide, there are a handful,
	// and nothing queries them except "is this token one of these". A store of
	// their own would be an interface, an implementation and a migration for a
	// list that fits on one line. The cost is that creating one contends with
	// any other settings write through Rev, which is the right trade for
	// something done twice a year.
	WallLinks []WallLink `json:"wall_links,omitempty"`

	Rev uint64 `json:"rev"`
}

// WallLink is a credential that opens exactly one route.
//
// # Why it is not a service account with RoleReader
//
// A reader may read everything: the audit archive, the names of every Secret,
// every cluster's configuration. This URL lives on a television, gets
// bookmarked, photographed and mailed around an office -- so it authorises ONE
// route, the wall, and nothing else. That is a property somebody can check by
// reading the route table, which is worth more than a role they would have to
// reason about.
//
// # Why it is not a fourth role either
//
// UserRole's own comment says three and not more, because every role beyond
// them is a policy that has to be kept in step with a surface that grows every
// phase. A credential that opens one named route needs no place in that ladder.
type WallLink struct {
	ID    string `json:"id"`
	Label string `json:"label"`

	// TokenHash is the SHA-256 of the link's token, hex encoded. The token
	// itself is shown once, at creation, and never stored -- a lost link is
	// replaced rather than recovered.
	TokenHash string `json:"token_hash,omitempty"`

	CreatedAt time.Time `json:"created_at"`

	// CreatedBy is the account that made it, so the archive and the list agree
	// about who put a screen in a corridor.
	CreatedBy string `json:"created_by"`

	// LastUsedAt answers the question this list exists for: is this link still
	// in use, and roughly since when. Written at most once a minute, because a
	// screen polls every ten seconds and a store write per poll would be a
	// write amplifier.
	LastUsedAt time.Time `json:"last_used_at,omitempty"`
}

// Session is a server-side session record. The session payload is opaque to
// everything above the store: it is whatever the session manager encoded.
type Session struct {
	ID        string    `json:"id"`
	Data      []byte    `json:"data"`
	ExpiresAt time.Time `json:"expires_at"`

	Rev uint64 `json:"rev"`
}

// Schematic is a stored Image Factory schematic: what was asked for, what the
// Factory made of it, and whether it was ever proven to build.
//
// It follows model.User's shape -- snake_case JSON tags, a CreatedAt, and a
// trailing Rev -- because every stored record in holzkube-manager does.
type Schematic struct {
	// ID is the Factory's own schematic id, which is the SHA-256 of Canonical.
	//
	// That has a consequence for Arch that is easy to miss and expensive to
	// meet by surprise: the canonical document does not contain the
	// architecture. Upstream leaves it out by design -- a schematic describes
	// what goes *into* an image, and the architecture is a path segment on the
	// asset URL rather than a field of the document. So two records that differ
	// only in architecture hash to the same id and collide, and POSTing the
	// same customisation at a second architecture answers 409 store.conflict.
	// Verified live.
	//
	// **One stored customisation therefore holds exactly one architecture's
	// verdict.** Probing the other architecture means deleting this record and
	// authoring it again; there is still no route that adds a *second* verdict,
	// because the record has room for one. That is the sentence an operator who
	// meets the 409 needs, and 02-DECISION-schematic-identity.md is where the
	// reasoning and the decided direction live.
	//
	// What a second POST does with the *one* verdict was narrowed by plan
	// 02-24 and no longer reads "refused rather than merged" across the board.
	// At the same architecture *and the same Talos version* the 409 now
	// refreshes Usable, ProbedAt and ProbeReason in place from the probe that
	// submission just ran, and refuses the label, the cluster and the Talos
	// version exactly as before. At a different architecture it is still
	// refused outright and changes nothing: that verdict is a different
	// record's worth of statement and this record has no room for it.
	//
	// TalosVersion is the second of those two, narrowed by plan 02-25, and the
	// stored field is now a condition of the refresh as well as something the
	// 409 refuses to replace. At a different version the refresh is declined
	// and the record is left byte for byte as it was, for the same reason it is
	// declined at a different architecture: this record's identity cannot vary
	// by either, because Canonical() emits neither.
	ID SchematicID `json:"id"`

	// Cluster is the cluster this schematic belongs to, empty when it is not
	// assigned to one. This is the first real user of ClusterID: a schematic
	// authored for one cluster must not silently be offered for another.
	Cluster ClusterID `json:"cluster"`

	// Name is the operator's own label. It never reaches a filesystem path --
	// the ID does.
	Name string `json:"name"`

	// TalosVersion is the version the schematic was authored and probed
	// against. It is part of the record rather than a query parameter because
	// the extension catalog is version-scoped: the same schematic may be
	// un-buildable at a different version.
	//
	// Which makes the stored verdict bound to this version, and since plan
	// 02-25 the code says so: the refresh a second POST performs is declined
	// when this field is not the version the fresh probe asked about, and the
	// record is left exactly as it was. At the same version and the same
	// architecture the refresh still happens. See ID's comment above for why
	// the two attempts are one record in the first place, and Arch's below for
	// the one way the two conditions differ.
	TalosVersion string `json:"talos_version"`

	// Arch is the architecture the schematic was authored and probed against.
	// The probe verdict is scoped to it -- ProbeReason already carries the
	// architecture inside its own sentence, which is the evidence that the
	// verdict was never architecture-neutral -- so Usable without it is a claim
	// whose subject is missing.
	//
	// It is part of the record rather than a query parameter for the same
	// reason TalosVersion is: the answer differs per architecture, so a stored
	// verdict that does not name one cannot be read back.
	//
	// It is also a field whose value this record's own identity cannot vary.
	// ID is the SHA-256 of a document with no architecture in it, so the two
	// architectures of one customisation are one record and not two -- see
	// ID's comment above for the mechanism and for what an operator has to do
	// about it. Arch names which of the two this record's verdict is about; it
	// does not make room for the other one.
	//
	// Additive and unversioned for the reason ProbeReason states at length
	// below. A record written before this field existed decodes with an empty
	// architecture, and for most such records an empty architecture is a true
	// statement: there is nothing for a migration to write.
	//
	// Most, not all -- and the difference is the probe's outcome, not the
	// record's age. A record whose probe *refused* carries the architecture
	// verbatim inside ProbeReason, in the format imagefactory/probe.go's
	// ErrSchematicNotBuildable produces: `<id> at <version>/<arch> answered
	// HTTP <status>`, pinned by
	// handlers.TestRefusalReasonNamesTheArchitectureItAskedAbout. For those
	// records the architecture is machine-parseable out of the sentence. For a
	// record whose probe succeeded, or never answered, there is no sentence and
	// nothing to read, so the verdict stays unqualified.
	//
	// This is not good news and nothing here acts on it. Recovering an
	// architecture that way means parsing prose written for an operator to
	// read -- weaker and more fragile than a field, and one reworded sentence
	// from being wrong. The correction is to the claim, not an argument for
	// building the backfill: a record whose probe refused is a record with no
	// usable verdict to qualify in the first place.
	//
	// The verdict can now be re-obtained, at this architecture only, by
	// submitting the identical customisation again: that POST answers 409 and
	// refreshes Usable, ProbedAt and ProbeReason as a side effect (plan 02-24).
	// Three things are still true and are the reason this is a mitigation
	// rather than a route, in the order the handler checks them. The refresh is
	// declined when the fresh probe does not answer either, which leaves the
	// record exactly as it was. It is declined when this field does not equal
	// the architecture the fresh probe asked about -- and a record written
	// before this field existed carries an empty one, so it is never refreshed.
	// And, since plan 02-25, it is declined when the record's TalosVersion is
	// not the version the fresh probe asked about.
	//
	// That third condition needs no old-record exemption, which is the one way
	// it differs from this one and is worth saying rather than leaving as an
	// apparent inconsistency: Arch was retrofitted additively, so an empty
	// architecture is a record written before the field existed, while
	// TalosVersion has been set since the first schematic record, so an empty
	// stored version is not an old record but a record whose version is not
	// known -- and declining it is correct.
	//
	// The verdict for the *other* architecture, and the verdict at another
	// version, are still not obtainable at all without deleting this record.
	Arch string `json:"arch"`

	// Canonical is the Factory's own normalised schematic document, stored
	// verbatim. It is the authoritative form (D-01): the id is the SHA-256 of
	// exactly these bytes, so storing the input instead would store something
	// that need not hash to the id it is filed under.
	Canonical string `json:"canonical"`

	// Extensions are the official system extension names baked in. Order is
	// preserved because the Factory preserves it, and it therefore changes the
	// id.
	Extensions []string `json:"extensions"`

	// KernelArgs are the extra kernel arguments. They reach the ISO and the
	// disk image and are ignored by the installer and initramfs, which is what
	// imagefactory.Warnings exists to say out loud.
	KernelArgs []string `json:"kernel_args"`

	// Meta are the META partition values. Same asymmetry as KernelArgs.
	Meta []MetaValue `json:"meta"`

	// Usable is false until the model-build probe confirms the schematic
	// actually builds for its Talos version and architecture.
	//
	// It is deliberately not set from a successful creation. The Factory
	// accepts a schematic naming an extension that does not exist, assigns it
	// an ordinary id, and only refuses when an image is asked for, so FACT-02
	// makes creation and validation two different events and this field
	// records the second one.
	Usable bool `json:"usable"`

	// ProbedAt is when the model-build probe last answered. A zero value means
	// it never has, which is not the same as "it answered no".
	ProbedAt time.Time `json:"probed_at"`

	// ProbeReason is what the Factory said when it refused, empty otherwise.
	//
	// "Not usable" without a reason is a verdict an operator cannot act on:
	// they cannot tell a schematic naming an extension that does not exist
	// from one asked for at a version that never had it. The probe already
	// produces the sentence -- the version, the architecture and the status the
	// Factory answered with -- and this is where it is kept so the screen can
	// show it next to the verdict it explains.
	//
	// It is set only for ErrSchematicNotBuildable. A probe that could not reach
	// the Factory says nothing about the schematic, and a reason recorded for
	// it would read as one.
	//
	// Additive and unversioned on purpose: a record written before this field
	// existed decodes with an empty reason, which is exactly what it means, so
	// there is nothing for a migration to do. Bumping the schema version would
	// force a backup and a rewrite of every data directory for a field whose
	// absence is already correct.
	ProbeReason string `json:"probe_reason"`

	CreatedAt time.Time `json:"created_at"`

	// Rev is the compare-and-swap revision. Every stored record carries one.
	Rev uint64 `json:"rev"`
}

// MetaValue is one META partition entry. Key is a uint8 because the META
// partition addresses its slots with a single byte.
type MetaValue struct {
	Key   uint8  `json:"key"`
	Value string `json:"value"`
}
