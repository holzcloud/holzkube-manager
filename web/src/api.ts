import { z } from 'zod'
import { type Problem, presentationFor, toProblemError } from '@/lib/problem'

/**
 * The single place in the frontend that calls fetch.
 *
 * Everything the CSRF contract requires is applied here, once, on every
 * mutating request: `Content-Type: application/json`, `X-Holzkube-Manager-CSRF: 1`
 * with that exact value, and same-origin credentials. All three are checked
 * simultaneously by the server and a miss is a 403 `csrf.precondition-unmet`
 * (threat T-01-36). Spreading fetch calls across components is how one of them
 * eventually forgets one of the three.
 *
 * Two answers are handled here rather than at every call site:
 *
 *   401  the session is gone. Sessions are 24 hours absolute (D-07), so this
 *        happens roughly daily during normal work. The session is dropped and
 *        the operator is taken to /login with a sentence explaining why.
 *   428  the route is destructive and the sudo window is shut (D-05). The
 *        password prompt opens and, once the window is open again, the
 *        original request is replayed with the identical body and headers, so
 *        the operator does not retype a form they already filled in.
 *
 * Note that a fresh login does NOT open the sudo window: only
 * POST /api/v1/auth/sudo does. A just-signed-in operator reaching a
 * destructive route gets a 428 exactly as an old session would.
 */

export {
  type Problem,
  ProblemError,
  problemSchema,
} from '@/lib/problem'

const CSRF_HEADER = 'X-Holzkube-Manager-CSRF'
/** Checked by value on the server, not merely for presence. Never 'true'. */
const CSRF_HEADER_VALUE = '1'

const READ_METHODS = new Set(['GET', 'HEAD', 'OPTIONS', 'TRACE'])

/**
 * An absent list and a null list are both an empty list (2026-09-20).
 *
 * `.nullish().transform(orEmpty)` fills in a MISSING field and does nothing for an explicit
 * `null` -- so a server answering `"quotas": null` failed the parse, and the
 * operator got a screenful of zod errors where the Kubernetes page should have
 * been. A nil slice in Go marshals to null, which makes that the default
 * behaviour of any list nobody remembered to initialise.
 *
 * The server is fixed and guarded (internal/kube/emptylists_test.go walks every
 * answer on an empty cluster and fails on a null anywhere). This is the second
 * half: the guard keeps the server honest, and this keeps a null from ever again
 * turning a page into an error message. A missing list and a null list mean the
 * same thing to every screen in this product, so they are read the same way.
 */
const orEmpty = <T>(value: T[] | null | undefined): T[] => value ?? []

export const auditChainSchema = z.object({
  ok: z.boolean(),
  broken_at_line: z.number(),
  file: z.string(),
})

export type AuditChain = z.infer<typeof auditChainSchema>

export const systemStatusSchema = z.object({
  setup_required: z.boolean(),
  audit_chain: auditChainSchema,

  // The two ways in, as they apply to the address this page was loaded from.
  // One instance answers on a LAN address that offers both and a public name
  // that offers only the identity provider, so the sign-in page renders from
  // these rather than from a build-time assumption.
  //
  // Defaulted, so a page served by an older binary still renders the password
  // form it has always shown instead of offering nothing at all.
  oidc_enabled: z.boolean().default(false),
  password_login: z.boolean().default(true),
  /** Where the local account still works, when THIS address refuses it. Empty
   * when there is no honest answer, and the sign-in page then says the sentence
   * without a link rather than guessing an address. */
  local_sign_in_url: z.string().default(''),

  /** The Talos version window this build was tested against, and whether this
   * instance accepts a pre-release inside it (OPS-03). Served rather than
   * written here, because a copy in this bundle drifts from the constants that
   * enforce it. */
  talos_range: z.string().default(''),
  allow_prerelease: z.boolean().default(false),
})

export type SystemStatus = z.infer<typeof systemStatusSchema>

/**
 * What this build is, and what changed in it.
 *
 * One response for both halves, because they are one claim. A version fetched
 * from one place and a changelog from another is how a "what's new" panel comes
 * to describe a release nobody is running.
 */
export const releaseChangeSchema = z.object({
  /** Decoration. Nothing branches on it, and a reader with no emoji font loses
   *  nothing the text does not already say. */
  icon: z.string().default(''),
  text: z.string(),
})

export const releaseSchema = z.object({
  version: z.string(),
  /** The first two components — "v1.16" for "v1.16.0-beta.2". Resolved by the
   *  server rather than split here: doing it in both places is how the two come
   *  to disagree about a version with a suffix. */
  series: z.string(),
  date: z.string().default(''),
  changes: z.array(releaseChangeSchema).nullish().transform(orEmpty),
})

export const versionSchema = z.object({
  version: z.string().default(''),
  releases: z.array(releaseSchema).nullish().transform(orEmpty),
})

export type ReleaseChange = z.infer<typeof releaseChangeSchema>
export type Release = z.infer<typeof releaseSchema>
export type VersionInfo = z.infer<typeof versionSchema>

export const auditRecordSchema = z.object({
  seq: z.number(),
  ts: z.string(),
  actor: z.string(),
  session: z.string(),
  src_ip: z.string(),
  cluster_id: z.string(),
  machine_id: z.string(),
  action: z.string(),
  params: z.record(z.string(), z.unknown()),
  job_id: z.string(),
  outcome: z.string(),
  prev_hash: z.string(),
  hash: z.string(),
})

export type AuditRecord = z.infer<typeof auditRecordSchema>

export const auditPageSchema = z.object({
  items: z.array(auditRecordSchema),
  // Always present, number or null. null means "no further page"; 0 is never a
  // valid cursor. Test against `!== null`, never for truthiness.
  next_cursor: z.number().nullable(),
})

/**
 * The audit page as the contract defines it. `next_cursor` is written out as
 * `number | null` rather than left to inference so that the exhaustion rule is
 * visible in the type: it is never `undefined`, and it is never absent.
 */
export interface AuditPage {
  items: AuditRecord[]
  next_cursor: number | null
}

export const meSchema = z.object({
  id: z.string(),
  username: z.string(),

  /**
   * Whether the process was started with `--dry-run`.
   *
   * It rides on the identity response rather than on `system/status`, which
   * answers before authentication: whether this instance can currently change
   * anything is not something an anonymous caller is owed, and the operator
   * this field exists for is signed in by definition.
   *
   * It reports the transport's behaviour, not the UI's. While it is true, every
   * mutating Talos RPC is refused by an interceptor before it reaches the wire.
   */
  dry_run: z.boolean(),

  /**
   * What this session may do.
   *
   * It decides what the interface offers and it is not what enforces anything:
   * every route decides for itself on the way in, so a client that got this
   * wrong would meet 403 rather than get anywhere.
   *
   * Defaulted to admin, and that default is the same decision the server makes
   * about an account with no stored role: a page served by an older binary
   * gets no field at all, and hiding half the interface from the one operator
   * of a single-account installation would be a regression dressed as a
   * permission.
   */
  role: z.string().default('admin'),

  /**
   * Whether this session was established through the identity provider.
   *
   * Signing out then has to go through the provider's RP-initiated logout, or
   * the next sign-in returns instantly from a provider session that never
   * ended -- which looks exactly like the sign-out having been ignored.
   *
   * Defaulted, so a page served by an older binary keeps behaving as it did.
   */
  sso: z.boolean().default(false),
})

export type Me = z.infer<typeof meSchema>

/* ---------------------------------------------------------------------- */
/* Accounts (V2-AUTH-02)                                                   */
/* ---------------------------------------------------------------------- */

export const USER_ROLES = ['admin', 'operator', 'reader'] as const
export type UserRoleName = (typeof USER_ROLES)[number]

/** What each role may do, in the words the settings screen uses. */
export const USER_ROLE_SENTENCE: Record<UserRoleName, string> = {
  admin:
    'Everything, including managing accounts and downloading the credentials that make this instance unnecessary.',
  operator:
    'Run the fleet: configure, upgrade, provision, reboot, reset, remove a node. Not accounts, and not cluster credentials.',
  reader: 'Look. Nothing they do changes anything.',
}

export const userSchema = z.object({
  id: z.string(),
  username: z.string(),
  role: z.string(),
  created_at: z.string(),
  /** "person" or "service". Defaulted, so a page served by an older binary
   * keeps rendering the accounts it already knows about. */
  kind: z.string().default('person'),
  /** Both are present only for a service account, and empty means never: a
   * token that has never been used is a fact worth showing. */
  token_issued_at: z.string().default(''),
  last_used_at: z.string().default(''),
  /** Whether this account signs in through the identity provider. */
  linked_identity: z.boolean().default(false),
  /** Whether this is the account making the request. */
  self: z.boolean().default(false),
})

export const usersSchema = z.object({ users: z.array(userSchema).nullish().transform(orEmpty) })

export type User = z.infer<typeof userSchema>

/**
 * A token, and the sentence that has to travel with it.
 *
 * The notice comes from the server rather than being written here, because it
 * is a statement about what the server kept — only a hash — and a client that
 * invented its own wording would be making a promise on the server's behalf.
 */
export const serviceAccountTokenSchema = z.object({
  token: z.string(),
  notice: z.string().default(''),
})

export const serviceAccountCreatedSchema = z.object({
  account: userSchema,
  token: z.string(),
  notice: z.string().default(''),
})

export type ServiceAccountToken = z.infer<typeof serviceAccountTokenSchema>
export type ServiceAccountCreated = z.infer<typeof serviceAccountCreatedSchema>

/**
 * Whether a role carries the privileges of another.
 *
 * The order is the server's, and it is duplicated here rather than fetched
 * because it decides what is rendered before any request is made. The
 * duplication is safe in the direction that matters: this side only ever hides
 * things, and the server refuses them.
 */
export function roleAtLeast(have: string, want: UserRoleName): boolean {
  const h = USER_ROLES.indexOf(have as UserRoleName)
  const w = USER_ROLES.indexOf(want)
  return h >= 0 && w >= 0 && h <= w
}

/** Where the browser is sent to start a flow against the identity provider. */
export const oidcPath = {
  signIn: '/api/v1/auth/oidc/start',
  reauthenticate: '/api/v1/auth/oidc/sudo',
  signOut: '/api/v1/auth/oidc/logout',
} as const

/* ---------------------------------------------------------------------- */
/* Image Factory                                                           */
/* ---------------------------------------------------------------------- */

export const factoryVersionsSchema = z.object({
  stable: z.array(z.string()),
  prerelease: z.array(z.string()),
  /**
   * A comparison, never the last element of the upstream list. The upstream
   * list is served ascending and ends in the current alpha, beta and rc tags,
   * so "the newest" and "the last" are different answers and only one of them
   * is safe to preselect.
   */
  newest_stable: z.string(),
  /** Version -> the reason it is listed. Frequently empty; never null. */
  broken: z.record(z.string(), z.string()),
})

export type FactoryVersions = z.infer<typeof factoryVersionsSchema>

export const factoryExtensionSchema = z.object({
  name: z.string(),
  ref: z.string(),
  digest: z.string(),
  author: z.string(),
  description: z.string(),
})

export type FactoryExtension = z.infer<typeof factoryExtensionSchema>

export const factoryExtensionsSchema = z.object({
  /**
   * Echoed back by the server. The catalog is version-scoped and there is no
   * fallback, so a list that does not say which version it belongs to is a list
   * that can be used against the wrong one.
   */
  version: z.string(),
  extensions: z.array(factoryExtensionSchema),
})

export type FactoryExtensionCatalog = z.infer<typeof factoryExtensionsSchema>

export const metaValueSchema = z.object({
  key: z.number(),
  value: z.string(),
})

export type MetaValue = z.infer<typeof metaValueSchema>

export const schematicSchema = z.object({
  id: z.string(),
  cluster: z.string(),
  name: z.string(),
  talos_version: z.string(),
  canonical: z.string(),
  extensions: z.array(z.string()),
  kernel_args: z.array(z.string()),
  meta: z.array(metaValueSchema),
  /**
   * False until the model-build probe agreed. A successful creation never sets
   * it: the Factory accepts a schematic naming an extension that does not
   * exist, assigns it an ordinary id, and refuses only when an image is asked
   * for. Rendering "created" as success is the lie this field exists to stop.
   *
   * The value may have been written by a *later* POST than the one that created
   * the record. Re-submitting an identical customisation answers `409`
   * `store.conflict` and, as a documented side effect, refreshes `usable`,
   * `probed_at` and `probe_reason` from the probe that submission just ran —
   * see `POST /api/v1/schematics` in `docs/api-contract.md`. A client must
   * therefore refetch the saved list after a create that *failed*: "the create
   * failed" no longer implies "nothing changed".
   */
  usable: z.boolean(),
  /**
   * The zero time means never probed, which is not the same as probed and
   * refused. The two must not be merged in the UI.
   *
   * It is not safe to reason about this against `created_at`. A value later
   * than the creation time does not mean a second probe ran on a schedule — it
   * means a conflicting POST refreshed it (see `usable` above), which is the
   * only mechanism that writes it after creation. There is no re-probe route.
   */
  probed_at: z.string(),
  /**
   * What the Factory said when it refused, empty otherwise -- including when
   * the probe could not reach it at all, which says nothing about the
   * schematic. A red badge with no stated cause is a verdict an operator
   * cannot act on.
   */
  probe_reason: z.string(),
  /**
   * The architecture the schematic was authored and probed against -- what
   * `usable` and `probe_reason` are statements about.
   *
   * Required rather than optional: the Go struct carries no `omitempty`, so the
   * field is always on the wire, including as `''`. A server that stopped
   * sending it is a regression this schema should surface as a decode failure
   * rather than as a silently missing qualifier.
   *
   * An empty value means a record written before the field existed. Whether
   * its verdict can be qualified after the fact depends on the probe's
   * outcome, not on the record's age:
   *
   * - the probe **refused**: `probe_reason` names the architecture verbatim,
   *   in the shape `<id> at <version>/<arch> answered HTTP <status>` (Go:
   *   `imagefactory/probe.go`, pinned by
   *   `TestRefusalReasonNamesTheArchitectureItAskedAbout`), so it is
   *   machine-parseable out of that sentence;
   * - the probe **succeeded, or never answered**: there is no sentence and
   *   nothing to read.
   *
   * Nothing here parses it, and nothing should start to. That would mean
   * reading an architecture out of prose written for an operator, which is
   * weaker and more fragile than a field and one rewording away from being
   * wrong. The row renders such a verdict unqualified rather than guessing at
   * it — which for a refused record costs nothing, because the reason it would
   * be qualifying is already on screen.
   */
  arch: z.string(),
  created_at: z.string(),
  rev: z.number(),
})

export type Schematic = z.infer<typeof schematicSchema>

export const schematicWarningSchema = z.object({
  code: z.string(),
  detail: z.string(),
})

export type SchematicWarning = z.infer<typeof schematicWarningSchema>

/** The 201 body: the stored record plus the warnings for this attempt. */
export const createdSchematicSchema = schematicSchema.extend({
  warnings: z.array(schematicWarningSchema),
})

export type CreatedSchematic = z.infer<typeof createdSchematicSchema>

export const schematicAssetsSchema = z.object({
  iso: z.string(),
  pxe: z.string(),
  disk_image: z.string(),
  cmdline: z.string(),
  /**
   * Resolved against the registry, never assembled. It is consumed by the
   * upgrade RPC, and a wrong one produces an upgrade that reports success while
   * silently dropping every system extension the node was built with.
   *
   * `null` means the registry could not be made to answer for it, and
   * `installer_error` says which of the two reasons it was. Null rather than an
   * empty string, and never absent: `warnings: []` on this route is an
   * affirmative claim that the repository name was *proven*, so an empty string
   * would carry that claim about a reference that does not exist. A null is a
   * decode-time fact — the other four references are still here and still
   * correct, and this is the one field an upstream can withhold.
   */
  installer: z.string().nullable(),
  /**
   * Why `installer` is null, in the server's own words. Absent — not null —
   * whenever the reference resolved, so its presence is the signal rather than
   * something a reader has to inspect.
   *
   * `code` is what to branch on: `upstream.factory-rejected` is a verdict about
   * this version and this repository name that no retry changes, and
   * `upstream.factory-unavailable` is a registry that did not answer, where
   * asking again may work. `detail` is the sentence to show; it already names
   * the schematic, the version and the repository names that were asked, so the
   * client never invents a second account of one failure.
   */
  installer_error: z
    .object({
      code: z.string(),
      detail: z.string(),
    })
    .optional(),
  /**
   * How sure the server is of the installer repository name above. Empty means
   * proven; a `installer.repo-fallback-unverified` entry means the reference is
   * usable but provisional — the preferred repository never answered and was
   * never ruled out.
   *
   * Required rather than optional, deliberately. A schema that tolerated the
   * field's absence would let a server regression reach the screen as "no
   * warnings", which is the silence G-02-3 was about: the reference would still
   * render and nothing would say it had not been proven.
   *
   * It says nothing at all when `installer` is null: there is no name to be sure
   * of, and `[]` there means "nothing to warn about", not "proven".
   */
  warnings: z.array(schematicWarningSchema),
})

export type SchematicAssets = z.infer<typeof schematicAssetsSchema>

export interface SchematicInput {
  name: string

  /**
   * The cluster this schematic is filed under, empty for none.
   *
   * Not a constraint. The image is the same image whichever cluster it is
   * installed into, so this is the operator's own filing -- and what it buys is
   * that the provisioning plan says so when a machine joining one cluster boots
   * a schematic filed under another. A warning on the last screen before a disk
   * is written, not a refusal.
   */
  cluster: string

  talos_version: string
  arch: string
  extensions: string[]
  kernel_args: string[]
  meta: MetaValue[]
  secureboot: boolean
}

export interface AssetQuery {
  arch: string
  version?: string
  secureboot?: boolean
}

/**
 * The warning codes the server emits, as constants a component can key on.
 *
 * Two families, and the prefix says which. A `schematic.` code is a property of
 * the stored schematic and can be recomputed from the record at any time. An
 * `installer.` code is a fact about one resolution attempt on one request —
 * nothing persists it, so it only ever arrives on the response it describes.
 */
export const WARNING_INSTALLER_IGNORES_KERNEL_ARGS = 'schematic.installer-ignores-kernel-args'
export const WARNING_INSTALLER_IGNORES_META = 'schematic.installer-ignores-meta'

/**
 * The installer reference was reached past a candidate repository that never
 * answered, so the preferred name was unheard rather than ruled out. The
 * reference is usable but provisional, and asking again may produce a different
 * one.
 */
export const WARNING_INSTALLER_REPO_FALLBACK_UNVERIFIED = 'installer.repo-fallback-unverified'

/**
 * The same provenance for a SecureBoot request, and a separate code because the
 * fact it carries is a different one.
 *
 * `installer-secureboot` was recorded as a legacy alias of
 * `metal-installer-secureboot`, and it is not reliably one. Whether the two
 * resolve to the same image has been measured to change: they differed at the
 * pinned Talos version on 2026-08-30 and matched at that same version on
 * 2026-09-14. Neither "alias" nor "different image" is true of the pair in
 * general, which is why the answer is labelled per resolution rather than
 * settled once. An operator who copied this reference earlier is not
 * necessarily holding the same thing.
 *
 * It does not weaken the rule that a SecureBoot request is never answered with
 * an ordinary installer: both candidates behind this code are SecureBoot
 * installers.
 */
export const WARNING_INSTALLER_SECUREBOOT_REPO_FALLBACK_UNVERIFIED =
  'installer.secureboot-repo-fallback-unverified'

export interface AuditQuery {
  from?: string
  to?: string
  action?: string
  limit?: number
  cursor?: number
}

/* ---------------------------------------------------------------------- */
/* Interceptor wiring                                                      */
/* ---------------------------------------------------------------------- */

/** A destructive action waiting for the operator to re-enter their password. */
export interface SudoChallenge {
  /** What the operator was doing, named in the dialog so the ask makes sense. */
  action: string

  /**
   * Why this one is asking, when the general reason is not true of it.
   *
   * The dialog's default says the action "changes something that cannot simply
   * be undone", which is the reason the sudo window exists and is right for
   * almost everything behind it. It is not right for all of them: renewing a
   * cluster certificate keeps the old one if the new one cannot reach a node,
   * so nothing about it is hard to undo. It is gated because it touches the
   * credential that reaches a cluster's PKI, which is a different sentence —
   * and telling an operator something false about what they are about to do is
   * worse than telling them nothing.
   */
  because?: string

  /**
   * How to run this action again, when it can be run again safely.
   *
   * Present only for a request with NO BODY. The sign-in round trip unloads the
   * page, so the waiting request cannot survive it -- and carrying a body across
   * would mean putting it in sessionStorage, where for the password change that
   * is gated the same way it would be the password itself (see
   * ResumeAfterProvider). A method and a path carry nothing secret, so those are
   * carried, and an action with a body still asks the operator to repeat it.
   *
   * It is what lets the banner after the round trip offer the action itself
   * instead of a way back to the page it was on. The operator pressed a button,
   * was taken to their provider and back, and was then asked to find the button
   * and press it again -- which reads as the action having failed, and is how
   * they reported it (ledger 165).
   */
  replay?: SudoReplay

  /** Called with true once the sudo window is open, false if it was cancelled. */
  settle: (granted: boolean) => void
}

/** A request that can be re-issued: no body, so nothing secret travels. */
export interface SudoReplay {
  method: string
  path: string
}

type SudoHandler = (challenge: SudoChallenge) => void
type SessionExpiredHandler = (problem: Problem) => void

let sudoHandler: SudoHandler | null = null
let sessionExpiredHandler: SessionExpiredHandler | null = null

/** Install the component that shows the sudo prompt. Null removes it. */
export function onSudoRequired(handler: SudoHandler | null): void {
  sudoHandler = handler
}

/** Install the handler that takes the operator to /login on an expired session. */
export function onSessionExpired(handler: SessionExpiredHandler | null): void {
  sessionExpiredHandler = handler
}

/**
 * Names the pending action for the sudo prompt, in English (D-09), and says
 * why when the dialog's general reason is not true of it.
 *
 * A route that is not here still prompts — the server decides that, not this
 * table — it is just named generically.
 */
const ACTION_LABELS: ReadonlyArray<{
  /**
   * Matched against the whole path, anchored.
   *
   * A pattern and not a prefix. The first draft of the third entry used the
   * prefix `/api/v1/clusters/` and so claimed every route under it — including
   * deleting a cluster, which would have asked the operator to confirm a
   * certificate renewal while forgetting their cluster. An id in the middle of
   * a path is exactly where prefix matching stops working, and this route has
   * one.
   */
  match: RegExp
  action: string
  because?: string
}> = [
  { match: /^\/api\/v1\/account\/password$/, action: 'Change the operator password' },
  { match: /^\/api\/v1\/schematics\/[^/]+$/, action: 'Delete this schematic' },
  // The routes an operator actually meets this prompt on. Without a label the
  // dialog and the banner both say "This destructive action", which is true and
  // tells somebody nothing about the thing they are being asked to confirm --
  // and it was what the operator saw when they tried to forget a machine.
  //
  // Anchored, for the reason the entry below records: an id sits in the middle
  // of these paths, so a prefix would claim every route under it.
  { match: /^\/api\/v1\/machines\/[^/]+$/, action: 'Forget this machine' },
  { match: /^\/api\/v1\/machines\/[^/]+\/reboot$/, action: 'Reboot this machine' },
  { match: /^\/api\/v1\/machines\/[^/]+\/shutdown$/, action: 'Shut this machine down' },
  { match: /^\/api\/v1\/machines\/[^/]+\/reset$/, action: 'Reset this machine' },
  { match: /^\/api\/v1\/clusters\/[^/]+$/, action: 'Forget this cluster' },
  {
    match: /^\/api\/v1\/clusters\/[^/]+\/client-certificate$/,
    action: 'Renew this cluster’s certificate',
    because:
      'reaches the credential this installation uses to get into a cluster, so it asks for your ' +
      'password again before it runs. Nothing is lost if it fails: the certificate is proved ' +
      'against a node before it replaces the one in use',
  },
]

function challengeFor(path: string): { action: string; because?: string } {
  for (const { match, action, because } of ACTION_LABELS) {
    if (match.test(path)) {
      return { action, because }
    }
  }
  return { action: 'This destructive action' }
}

/**
 * Runs a remembered action again, after the identity-provider round trip.
 *
 * Through the ordinary pipeline on purpose: if the window is somehow shut again
 * the dialog opens as it always would, and a failure is the same problem
 * document every other call produces. A private path here would be a second
 * copy of the refusal handling, which is how the two drift apart.
 */
export async function replaySudoAction(replay: SudoReplay): Promise<void> {
  await sendPrebuilt(replay.path, buildInit(replay.method, undefined))
}

/* ---------------------------------------------------------------------- */
/* The request pipeline                                                    */
/* ---------------------------------------------------------------------- */

interface RequestOptions {
  /**
   * Whether a 401 should be treated as "the session ended while you were
   * working". GET /api/v1/auth/me is the probe that asks whether there is a
   * session at all, so its 401 is an answer, not an expiry.
   */
  interceptUnauthenticated?: boolean
  /** Whether a 428 should open the sudo prompt and replay. */
  interceptSudo?: boolean

  /**
   * A bearer token to send instead of relying on the session cookie.
   *
   * One caller: the wall, when it was opened with a kiosk link. The token is a
   * credential, so it goes in a header and never in a URL -- a URL is written
   * to the server's log, the browser's history and whatever proxy sits between.
   *
   * A request carrying one is exempt from the session-expiry interception too:
   * there is no session to expire, and a redirect to the sign-in page is the
   * one thing a wall must never do.
   */
  bearer?: string

  /**
   * Whether this request is exempt from REQUEST_CEILING_MS.
   *
   * One route is: the etcd restore, whose body is a database. The ceiling
   * exists so a browser tab does not sit forever on a server that has stopped
   * answering, and it is sized against a response arriving. An upload is a
   * different shape — it is making progress the whole time, and a ceiling on
   * it is a limit on how large somebody's etcd is allowed to be. The server
   * clears its own read deadline for the same route and for the same reason.
   *
   * It is a named option and not a number, so that adding a second exempt
   * route is a decision somebody makes here rather than a constant they raise.
   */
  unbounded?: boolean
}

/**
 * How long the browser waits for one request before giving up on it.
 *
 * Its derivation, in one sentence: it is longer than the server's largest route
 * budget so the server's own answer always wins the race, and it exists for the
 * case where there will be no answer at all.
 *
 * The number it is above is `writeTimeout` in cmd/holzkube-managerd/main.go,
 * 130 seconds -- the outermost bound the server puts on producing a response,
 * itself derived from the largest route budget plus slack. A ceiling below that
 * would abort while the server is still working and replace an honest
 * problem+json, which says what went wrong upstream and whether retrying helps,
 * with a generic network failure, which says nothing. That is strictly less
 * information than the operator has today, so this sits above it with room to
 * spare rather than tuned close to it.
 *
 * What it is for is the case the server never answers: a dropped connection, a
 * suspended laptop, a process that went away. There, the alternative is a
 * promise that never settles -- a spinner with no end and a pending state
 * nothing clears (T-02-109).
 */
const REQUEST_CEILING_MS = 150_000

function buildInit(method: string, body: unknown, bearer?: string): RequestInit {
  const headers: Record<string, string> = { Accept: 'application/json' }
  if (bearer !== undefined && bearer !== '') {
    headers.Authorization = `Bearer ${bearer}`
  }

  if (!READ_METHODS.has(method)) {
    // All three CSRF preconditions, together, on every mutating request.
    headers['Content-Type'] = 'application/json'
    headers[CSRF_HEADER] = CSRF_HEADER_VALUE
  }

  return {
    method,
    headers,
    credentials: 'same-origin',
    body: body === undefined ? undefined : JSON.stringify(body),
  }
}

/**
 * Issues one attempt with a ceiling of its own.
 *
 * The signal deliberately does NOT go into `init`. `init` is built once and
 * reused so the sudo replay is byte for byte the request that was refused, and
 * an `AbortSignal.timeout` starts counting the moment it is created -- so a
 * signal stored in `init` would arrive at the replay partly spent, or already
 * fired, and turn a retry the operator just authorised into an immediate abort
 * blaming the network. Each attempt gets a whole ceiling because each attempt is
 * a whole request.
 *
 * Do not "simplify" this back into buildInit. The property it protects is
 * invisible from there.
 */
function fetchWithCeiling(path: string, init: RequestInit, unbounded = false): Promise<Response> {
  if (unbounded) {
    return fetch(path, init)
  }
  return fetch(path, { ...init, signal: AbortSignal.timeout(REQUEST_CEILING_MS) })
}

/** Whether a rejected fetch was this module's own ceiling firing. */
function isCeilingAbort(cause: unknown): boolean {
  return (
    cause instanceof DOMException && (cause.name === 'TimeoutError' || cause.name === 'AbortError')
  )
}

/**
 * The error a request that ran out of ceiling ends as.
 *
 * It is deliberately not run through `toProblemError`: there is no response to
 * read, and inventing a problem document would put words in the server's mouth.
 * The message says what this side did and claims nothing about what the server
 * is doing, because this side does not know -- the request may have been
 * received, acted on and answered into a socket nobody is holding any more.
 */
function ceilingError(): Error {
  return new Error(
    `The server did not answer within ${Math.round(REQUEST_CEILING_MS / 1000)} seconds, ` +
      'so the request was given up on. It may or may not have been carried out.',
  )
}

async function askForSudo(path: string, init: RequestInit): Promise<boolean> {
  if (sudoHandler === null) {
    return false
  }
  const handler = sudoHandler
  // Only a request with no body is offered as a replay. See SudoChallenge.replay.
  const replay =
    init.body === undefined || init.body === null
      ? { method: init.method ?? 'GET', path }
      : undefined
  return new Promise<boolean>((resolve) => {
    handler({ ...challengeFor(path), replay, settle: resolve })
  })
}

async function send(
  method: string,
  path: string,
  body?: unknown,
  options: RequestOptions = {},
): Promise<Response> {
  // Built once and reused for the replay, so the retried request is byte for
  // byte the request that was refused -- same body, same headers, same
  // credentials mode. Rebuilding it would be the bug this design avoids.
  return sendPrebuilt(path, buildInit(method, body, options.bearer), options)
}

/**
 * The half of `send` that does not build the request.
 *
 * It is split out so that a caller with a body this module cannot build — the
 * etcd restore, whose body is a file the operator chose — gets the same
 * refusal handling as everything else: the sudo prompt and its byte-for-byte
 * replay, and the session-expiry transition. A second copy of that logic next
 * to the one route that needed it is how those two behaviours drift apart.
 */
async function sendPrebuilt(
  path: string,
  init: RequestInit,
  options: RequestOptions = {},
): Promise<Response> {
  const { interceptSudo = true, unbounded = false, bearer } = options
  // A request carrying a bearer token has no session to expire, so its 401 is
  // an answer rather than an expiry -- and a wall redirected to a sign-in page
  // is the one failure this credential exists to prevent.
  const interceptUnauthenticated =
    bearer !== undefined && bearer !== '' ? false : (options.interceptUnauthenticated ?? true)

  let response: Response
  try {
    response = await fetchWithCeiling(path, init, unbounded)
  } catch (cause) {
    if (isCeilingAbort(cause)) {
      throw ceilingError()
    }
    throw cause
  }
  if (response.ok) {
    return response
  }

  let error = await toProblemError(response)

  if (interceptSudo && presentationFor(error.problem) === 'sudo-prompt') {
    const granted = await askForSudo(path, init)
    if (!granted) {
      throw error
    }
    // A fresh ceiling, not the remainder of the first one: the operator has
    // just spent time at the password prompt, and charging that time to the
    // replay's budget would abort a request that had not started.
    try {
      response = await fetchWithCeiling(path, init, unbounded)
    } catch (cause) {
      if (isCeilingAbort(cause)) {
        throw ceilingError()
      }
      throw cause
    }
    if (response.ok) {
      return response
    }
    error = await toProblemError(response)
  }

  if (interceptUnauthenticated && presentationFor(error.problem) === 'login-transition') {
    sessionExpiredHandler?.(error.problem)
  }

  throw error
}

/**
 * Sends a request whose body is bytes rather than JSON.
 *
 * It exists for one route and says so, because the shape is easy to reach for
 * and wrong nearly everywhere: this API is JSON, and a second body encoding is
 * a second thing every future reader has to check. What makes it right here is
 * that the body is an etcd database — base64 inside a JSON envelope would cost
 * a third of its size on the wire and all of it in memory.
 *
 * A Blob and not a stream, deliberately. `send` builds the request once and
 * replays it byte for byte after the sudo prompt, and a stream cannot be read
 * twice: a restore that asked for a password would replay with an empty body.
 */
async function sendBody<T>(
  method: string,
  path: string,
  schema: z.ZodType<T>,
  body: Blob,
): Promise<T> {
  const init: RequestInit = {
    method,
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/octet-stream',
      [CSRF_HEADER]: CSRF_HEADER_VALUE,
    },
    credentials: 'same-origin',
    body,
  }

  const response = await sendPrebuilt(path, init, { unbounded: true })
  return schema.parse(await response.json())
}

/**
 * Sends a YAML document as the request body.
 *
 * A third body encoding, and the same justification as the second: this one is
 * a document a person wrote, and the alternative is escaping it into a JSON
 * string where nothing can read it back. It goes through sendPrebuilt so the
 * sudo prompt and the session-expiry transition behave exactly as everywhere
 * else.
 */
async function sendYAML<T>(
  method: string,
  path: string,
  schema: z.ZodType<T>,
  body: string,
): Promise<T> {
  const response = await sendPrebuilt(path, {
    method,
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/yaml',
      [CSRF_HEADER]: CSRF_HEADER_VALUE,
    },
    credentials: 'same-origin',
    body,
  })
  return schema.parse(await response.json())
}

async function sendJSON<T>(
  method: string,
  path: string,
  schema: z.ZodType<T>,
  body?: unknown,
  options?: RequestOptions,
): Promise<T> {
  const response = await send(method, path, body, options)
  return schema.parse(await response.json())
}

/**
 * The asset query. `arch` is always sent and is never defaulted here or on the
 * server: holzkube-manager is developed on arm64 and targets amd64, so a defaulted
 * architecture is a bug that only ever appears on someone else's machine.
 */
function assetQueryString(query: AssetQuery): string {
  const params = new URLSearchParams({ arch: query.arch })
  if (query.version !== undefined && query.version !== '') {
    params.set('version', query.version)
  }
  if (query.secureboot === true) {
    params.set('secureboot', 'true')
  }
  return `?${params.toString()}`
}

function auditQueryString(query: AuditQuery): string {
  const params = new URLSearchParams()
  if (query.from !== undefined && query.from !== '') {
    params.set('from', query.from)
  }
  if (query.to !== undefined && query.to !== '') {
    params.set('to', query.to)
  }
  if (query.action !== undefined && query.action !== '') {
    params.set('action', query.action)
  }
  if (query.limit !== undefined) {
    params.set('limit', String(query.limit))
  }
  // 0 is never a valid cursor and is never sent.
  if (query.cursor !== undefined && query.cursor !== null) {
    params.set('cursor', String(query.cursor))
  }
  const encoded = params.toString()
  return encoded === '' ? '' : `?${encoded}`
}

/* ---------------------------------------------------------------------- */
/* Inventory                                                               */
/* ---------------------------------------------------------------------- */

/**
 * The level a fact depends on, as a string on the wire.
 *
 * The server's Go type is an iota, and it is deliberately not serialised as a
 * number: the order of the constants is an implementation detail, and a number
 * here would publish it as a contract.
 */
export const healthLevelSchema = z.enum(['none', 'node', 'etcd', 'k8s'])

export type HealthLevel = z.infer<typeof healthLevelSchema>

/** The observer's state machine. `degraded` and `down` differ in what they
 * claim, not in how bad they are: a degraded node is answering with some of
 * its layers gone, a down node is answering nothing. */
export const stageSchema = z.enum(['unknown', 'connecting', 'watching', 'degraded', 'down'])

export type Stage = z.infer<typeof stageSchema>

/**
 * One fact, with its provenance.
 *
 * The three states are distinguishable and all three are meant to be rendered:
 *
 * - `available: true`  — confirmed right now.
 * - `available: false` with `stale_since` — known but old. **The value is still
 *   there and must still be shown**, muted, with its age. Hiding it is
 *   indistinguishable from null, and that is exactly how the empty dashboard
 *   INV-08 forbids comes about.
 * - `available: false` with `stale_since: null` — never read. Render an em dash
 *   and the `unavailable_reason` beside it.
 *
 * The age is computed here, from `stale_since`. The server deliberately holds
 * no staleness threshold of its own: two clocks would disagree.
 */
export function fieldSchema<T extends z.ZodType>(value: T) {
  return z.object({
    // Absent when the value is the type's zero, because the server omits it.
    // The default keeps every consumer from having to think about that.
    value: value.optional(),
    level: healthLevelSchema,
    available: z.boolean(),
    stale_since: z.string().nullish(),
    unavailable_reason: z.string().default(''),
  })
}

export interface Field<T> {
  value?: T
  level: HealthLevel
  available: boolean
  stale_since?: string | null
  unavailable_reason: string
}

export const cpuSchema = z.object({
  /** The board slot. It is the only field that tells two identical processors
   * apart, which is why a list of them can be keyed on it. */
  socket: z.string().default(''),
  manufacturer: z.string().default(''),
  product_name: z.string().default(''),
  cores: z.number().default(0),
  threads: z.number().default(0),
  max_speed_mhz: z.number().default(0),
})

export const diskSchema = z.object({
  device: z.string(),
  size: z.number().default(0),
  pretty_size: z.string().default(''),
  model: z.string().default(''),
  serial: z.string().default(''),
  transport: z.string().default(''),
  rotational: z.boolean().default(false),
  readonly: z.boolean().default(false),
  cdrom: z.boolean().default(false),
})

export const interfaceSchema = z.object({
  name: z.string(),
  hardware_addr: z.string().default(''),
  mtu: z.number().default(0),
  up: z.boolean().default(false),
  speed_mbit: z.number().default(0),
  addresses: z.array(z.string()).nullish().transform(orEmpty),
  driver: z.string().default(''),
  kind: z.string().default(''),
})

export const serviceSchema = z.object({
  id: z.string(),
  running: z.boolean().default(false),
  healthy: z.boolean().default(false),
  state: z.string().default(''),
  /** "does not report health" is not "reported unhealthy". Collapsing the two
   * paints a healthy node red. */
  health_unknown: z.boolean().default(false),
})

export const compatibilitySchema = z.object({
  known: z.boolean(),
  supported: z.boolean(),
  headroom_minors: z.number(),
  sentence: z.string(),
})

/** The state of a node's resource subscription (INV-13, D-19).
 *
 * It is deliberately not part of `stage`. A node whose watch is down is still
 * being read by the heartbeat — it is up to a heartbeat behind, which is what
 * this product did before watches existed. Colouring it like a failure would
 * put an alarm on a working machine. */
export const watchSchema = z.object({
  /** True only once the subscription has delivered the node's current
   * contents. A stream that has been opened has not yet said anything. */
  live: z.boolean().default(false),
  since: z.string().default(''),
  reason: z.string().default(''),
  /** How often this node's watch has had to be rebuilt since the server
   * started. A watch that is live and has restarted two hundred times looks
   * healthy at every instant and delivers nothing between them. */
  restarts: z.number().default(0),
})

export const machineSchema = z.object({
  id: z.string(),
  cluster: z.string().default(''),
  role: z.string(),
  stage: stageSchema,
  /** A different machine answered at this one's last known address. It is the
   * explanation for a node that stopped being found without anything breaking. */
  lost_addr: z.boolean().default(false),
  /** The cluster's client certificate has expired. This is not a dead node,
   * and showing it as one sends the operator to the wrong repair. */
  certificate_expired: z.boolean().default(false),
  /** Rolling operations skip this node (UPG-14). Not a Field: it is
   * holzkube-manager's own note about what it should not do, and it is true
   * whether or not the node is answering — which is when it matters most. */
  locked: z.boolean().default(false),
  lock_reason: z.string().default(''),
  /** OPS-03. Derived from the last version this node reported, not from a
   * failed connection: a node outside the range is refused at connect time, so
   * a marking that depended on connecting would be blank for exactly the nodes
   * it exists to mark. */
  unsupported_version: z.boolean().default(false),
  pre_release: z.boolean().default(false),
  version_notice: z.string().default(''),
  adopted_at: z.string(),

  /**
   * When this machine last answered anything at all — which is not the same as
   * when it was last read.
   *
   * A node whose connection succeeds and whose facts read fails moves this and
   * leaves its readings alone. So a `seen_at` that is newer than the readings
   * is the shape of a node that is present and cannot be read, which is a
   * different repair from one that is absent.
   */
  seen_at: z.string().optional(),
  watch: watchSchema.default({ live: false, since: '', reason: '', restarts: 0 }),

  /** The operator's own words about this machine. Never a Field: nothing
   * observed them, so there is no level they could be unavailable at — and no
   * observation overwrites one, which is what makes a selector over them
   * stable across a reboot. */
  labels: z.record(z.string(), z.string()).default({}),

  hostname: fieldSchema(z.string()),
  addr: fieldSchema(z.string()),
  talos_version: fieldSchema(z.string()),
  kubernetes_version: fieldSchema(z.string()),
  schematic_id: fieldSchema(z.string()),
  manufacturer: fieldSchema(z.string()),
  product_name: fieldSchema(z.string()),
  serial_number: fieldSchema(z.string()),
  memory_mib: fieldSchema(z.number()),
  cpus: fieldSchema(z.array(cpuSchema)),
  disks: fieldSchema(z.array(diskSchema)),
  interfaces: fieldSchema(z.array(interfaceSchema)),
  services: fieldSchema(z.array(serviceSchema)),
  etcd_member: fieldSchema(z.boolean()),
  compatibility: fieldSchema(compatibilitySchema),
})

export type Machine = z.infer<typeof machineSchema>

/**
 * One node's hardware, read live from the node at request time (2026-09-26).
 *
 * Nothing here is stored. Percentages and rates are the difference between two
 * readings of the node's own counters, and `rates_over_seconds` says how far
 * apart those readings were. A sensor the node does not report is absent rather
 * than zero: a missing fan and a stopped fan are different findings.
 */
const nullableNumber = z
  .number()
  .nullish()
  .transform((v) => v ?? null)

export const hardwareSchema = z.object({
  machine: z.string(),
  hostname: z.string().default(''),
  observed_at: z.string(),
  uptime_seconds: z.number().default(0),
  rates_over_seconds: z.number().default(0),
  cpu: z.object({
    model: z.string().default(''),
    cores: z.number().default(0),
    threads: z.number().default(0),
    usage_percent: z.number().default(0),
    iowait_percent: z.number().default(0),
    per_core: z.array(z.number()).nullish().transform(orEmpty),
    load1: z.number().default(0),
    load5: z.number().default(0),
    load15: z.number().default(0),
  }),
  memory: z.object({
    total_bytes: z.number().default(0),
    used_bytes: z.number().default(0),
    cache_bytes: z.number().default(0),
    available_bytes: z.number().default(0),
    swap_total_bytes: z.number().default(0),
    swap_used_bytes: z.number().default(0),
  }),
  filesystems: z
    .array(
      z.object({
        mount: z.string(),
        device: z.string().default(''),
        size_bytes: z.number().default(0),
        used_bytes: z.number().default(0),
      }),
    )
    .nullish()
    .transform(orEmpty),
  disks: z
    .array(
      z.object({
        name: z.string(),
        model: z.string().default(''),
        size_bytes: z.number().default(0),
        read_bytes_per_sec: z.number().default(0),
        write_bytes_per_sec: z.number().default(0),
        temperature_c: nullableNumber,
      }),
    )
    .nullish()
    .transform(orEmpty),
  network: z
    .array(
      z.object({
        name: z.string(),
        up: z.boolean().default(false),
        speed_mbit: z.number().default(0),
        rx_bytes_per_sec: z.number().default(0),
        tx_bytes_per_sec: z.number().default(0),
      }),
    )
    .nullish()
    .transform(orEmpty),
  temperatures: z
    .array(
      z.object({
        chip: z.string().default(''),
        kind: z.string().default('other'),
        label: z.string().default(''),
        celsius: z.number(),
        high_c: nullableNumber,
        critical_c: nullableNumber,
      }),
    )
    .nullish()
    .transform(orEmpty),
  fans: z
    .array(
      z.object({
        chip: z.string().default(''),
        label: z.string().default(''),
        rpm: z.number().default(0),
      }),
    )
    .nullish()
    .transform(orEmpty),
  sensors_notice: z.string().default(''),
})

export type Hardware = z.infer<typeof hardwareSchema>
export type Temperature = Hardware['temperatures'][number]

export const clusterSchema = z.object({
  id: z.string(),
  name: z.string(),
  origin: z.string(),
  endpoint: z.string(),
  locked: z.boolean(),
  created_at: z.string(),
  client_cert_not_after: z.string(),
  /** May be negative: that is the expired state, and it is not the same thing
   * as a cluster that is down. */
  client_cert_days_left: z.number(),
  certificate_warning: z.string().default(''),
  certificate_urgency: z.enum(['none', 'badge', 'banner', 'critical']),
  nodes: z.number(),
  control_plane: z.number(),
  workers: z.number(),
  healthy: z.number(),
  degraded: z.number(),
  down: z.number(),
  /** Nodes nobody has had an answer from YET — an observer that has not run
   * or is still connecting. Not a failure, and kept out of `down` for that
   * reason: counting them there put "1 not answering" on every card right
   * after an import or a restart. */
  checking: z.number(),
})

export type Cluster = z.infer<typeof clusterSchema>

export const clustersSchema = z.object({ clusters: z.array(clusterSchema) })
export const machinesSchema = z.object({ machines: z.array(machineSchema) })

/* ---------------------------------------------------------------------- */
/* Machine classes (V2 phase 5)                                            */
/* ---------------------------------------------------------------------- */

export const labelSelectorSchema = z.object({
  equals: z.record(z.string(), z.string()).default({}),
  present: z.array(z.string()).nullish().transform(orEmpty),
  absent: z.array(z.string()).nullish().transform(orEmpty),
})

export const machineClassSchema = z.object({
  id: z.string(),
  name: z.string(),
  description: z.string().default(''),
  selector: labelSelectorSchema,
  /** The selector in words. The data is there too; this is what is read. */
  sentence: z.string().default(''),
  /** Which machines this class names right now. A class is a question
   * re-answered on every read, not a group somebody is added to. */
  machines: z.array(z.string()).nullish().transform(orEmpty),
  count: z.number().default(0),
})

export const machineClassesSchema = z.object({
  classes: z.array(machineClassSchema).nullish().transform(orEmpty),
})

export type LabelSelector = z.infer<typeof labelSelectorSchema>
export type MachineClass = z.infer<typeof machineClassSchema>

/* ---------------------------------------------------------------------- */
/* Cluster templates (V2 phase 6)                                          */
/* ---------------------------------------------------------------------- */

export const nodeSetPlanSchema = z.object({
  /** A machine class id, or "named" when the template listed machines. */
  source: z.string().default(''),
  machines: z.array(z.string()).nullish().transform(orEmpty),
  /** What the template asked for; 0 means all of them. */
  wanted: z.number().default(0),
})

export const templatePlanSchema = z.object({
  cluster: z.string(),
  exists: z.boolean().default(false),
  control_plane: nodeSetPlanSchema,
  workers: nodeSetPlanSchema,
  /** Why it cannot be applied as written. Empty means it can. */
  problems: z.array(z.string()).nullish().transform(orEmpty),
  /** Worth knowing and not a problem. */
  notes: z.array(z.string()).nullish().transform(orEmpty),
  sentence: z.string().default(''),
})

export const templatePlanResponseSchema = z.object({
  plan: templatePlanSchema,
  /** What these routes deliberately do not do. It comes from the server
   * because it is a statement about the server's limits. */
  notice: z.string().default(''),
})

export type TemplatePlan = z.infer<typeof templatePlanSchema>

/* ---------------------------------------------------------------------- */
/* Cluster scale (V2 phase 8)                                              */
/* ---------------------------------------------------------------------- */

export const scaleRemovalSchema = z.object({
  machine: z.string(),
  name: z.string().default(''),
  role: z.string().default(''),
  allowed: z.boolean().default(false),
  /** Why not, in the words of whatever refused. Never this screen's own
   * paraphrase: two accounts of one condition is how an operator comes to
   * believe the screen and the server disagree. */
  reason: z.string().default(''),
})

export const scaleCandidateSchema = z.object({
  machine: z.string(),
  name: z.string().default(''),
  ready: z.boolean().default(false),
  reason: z.string().default(''),
})

export const scalePlanSchema = z.object({
  cluster: z.string(),
  name: z.string().default(''),
  control_plane: z.number().default(0),
  workers: z.number().default(0),
  /** Whether etcd was read at all. A voting count of 0 that means "none" and
   * one that means "not asked" are different answers. */
  members_known: z.boolean().default(false),
  voting: z.number().default(0),
  tolerates: z.number().default(0),
  members_problem: z.string().default(''),
  removals: z.array(scaleRemovalSchema).nullish().transform(orEmpty),
  additions: z.array(scaleCandidateSchema).nullish().transform(orEmpty),
  /** The arithmetic in words. It is why the screen exists. */
  advice: z.array(z.string()).nullish().transform(orEmpty),
  sentence: z.string().default(''),
})

export const scalePlanResponseSchema = z.object({
  plan: scalePlanSchema,
  notice: z.string().default(''),
})

export type ScalePlan = z.infer<typeof scalePlanSchema>

export const fingerprintSchema = z.object({
  endpoint: z.string(),
  fingerprint: z.string(),
})

export interface ImportInput {
  name: string
  talosconfig: string
  endpoint: string
  fingerprint: string
}

/* ---------------------------------------------------------------------- */
/* Provisioning                                                            */
/* ---------------------------------------------------------------------- */

/**
 * What a scan found at one address.
 *
 * `state` is never "nothing" for an address that answered, and that is the
 * finding this whole scan exists to get right: "nothing found" and "there is
 * already a node here" lead to opposite actions, and a screen that conflated
 * them would send an operator to check a cable while a running node sits at
 * the address they were about to overwrite.
 */
export const foundSchema = z.object({
  addr: z.string(),
  state: z.enum(['maintenance', 'configured', 'answered']),
  /** The name on the certificate the machine presented. Nothing has verified
   * it — see the maintenance notice. */
  hostname: z.string().default(''),
  fingerprint: z.string().default(''),
  /** This installation already manages a machine at this address. */
  known: z.boolean().default(false),
  detail: z.string().default(''),
})

export type Found = z.infer<typeof foundSchema>

export const scanSchema = z.object({
  found: z.array(foundSchema).nullish().transform(orEmpty),
  notices: z.array(z.string()).nullish().transform(orEmpty),
})

export const candidateDiskSchema = z.object({
  device: z.string(),
  size: z.number().default(0),
  pretty_size: z.string().default(''),
  model: z.string().default(''),
  serial: z.string().default(''),
  transport: z.string().default(''),
  /** Talos is, or would be, installed here. Shown rather than pre-selected. */
  system: z.boolean().default(false),
})

export const candidateSchema = z.object({
  addr: z.string(),
  uuid: z.string(),
  talos_version: z.string().default(''),
  hostname: z.string().default(''),
  fingerprint: z.string().default(''),
  /** The identifier printed on a label, for telling two identical machines
   * apart when the UUID cannot be read off either of them. */
  macs: z.array(z.string()).nullish().transform(orEmpty),
  disks: z.array(candidateDiskSchema).nullish().transform(orEmpty),
  warnings: z.array(z.string()).nullish().transform(orEmpty),
})

export type Candidate = z.infer<typeof candidateSchema>
export type CandidateDisk = z.infer<typeof candidateDiskSchema>

export const previewProvisionSchema = z.object({
  warnings: z.array(z.string()).nullish().transform(orEmpty),
  /** The exact `.machine.install.image` this plan writes. Shown because its
   * failure is silent: the install succeeds, the node joins, and the system
   * extensions are simply gone. */
  install_image: z.string(),
  control_plane_after: z.number().default(0),
  /** This run initialises etcd, which is the one irreversible thing in it that
   * is not about this machine. */
  bootstrap: z.boolean().default(false),
  cni_notice: z.string().default(''),
  maintenance_warning: z.string().default(''),
})

export type ProvisionPreview = z.infer<typeof previewProvisionSchema>

export interface ProvisionRequest {
  cluster: string
  addr: string
  uuid: string
  control_plane: boolean
  install_disk: string
  schematic_id?: string
  talos_version: string
  fingerprint?: string
  hostname?: string
  patch_ids?: string[]

  /**
   * Whether this machine booted the SecureBoot variant of its schematic.
   *
   * A schematic id does not carry it — one id resolves under `metal-installer`
   * and `metal-installer-secureboot` to two different images, picked by
   * repository name alone — so the only party who knows is whoever wrote the
   * USB stick. It selects the installer, which Talos requires to match: the
   * ordinary installer does not produce a SecureBoot node.
   */
  secureboot?: boolean

  /**
   * Encrypt the node's system volumes at install.
   *
   * At install, and only then. Talos encrypts a system volume when the volume
   * is empty, so this is the one moment in a machine's life when the answer
   * can still be yes — a node that is already installed keeps its plaintext
   * partitions and reports nothing.
   */
  encryption?: {
    state: boolean
    ephemeral: boolean
    /** `nodeID` or `tpm`. The server answers `static` and `kms` with reasons. */
    kind: string
  }
}

export const bootstrapIntentSchema = z.object({
  cluster: z.string(),
  machine: z.string().default(''),
  addr: z.string().default(''),
  started_at: z.string().default(''),
  outcome: z.string().default(''),
  detail: z.string().default(''),
})

export type BootstrapIntent = z.infer<typeof bootstrapIntentSchema>

export const bootstrapRecoverySchema = z.object({
  pending: z.array(bootstrapIntentSchema).nullish().transform(orEmpty),
  guidance: z.string().default(''),
})

export const noticesSchema = z.object({ notices: z.array(z.string()).nullish().transform(orEmpty) })

/* ---------------------------------------------------------------------- */
/* Upgrades and etcd                                                       */
/* ---------------------------------------------------------------------- */

export const upgradeStepSchema = z.object({
  to: z.object({
    Major: z.number(),
    Minor: z.number(),
    Patch: z.number(),
    Pre: z.string().default(''),
  }),
  /** Why this step exists. The middle steps of a chain are the ones an
   * operator asks about, because they are not the version anybody wanted —
   * they are the version Talos requires passing through. */
  why: z.string(),
})

export const strandCheckSchema = z.object({
  blocked: z.boolean(),
  sentence: z.string(),
  remedy: z.string().default(''),
})

export const etcdStatusSchema = z.object({
  MemberID: z.number().optional(),
  Leader: z.number().optional(),
  RaftIndex: z.number().optional(),
  RaftAppliedIndex: z.number().optional(),
  IsLearner: z.boolean().optional(),
  Errors: z.array(z.string()).nullish(),
})

export const gateVerdictSchema = z.object({
  ok: z.boolean(),
  reason: z.string().default(''),
  /** What the gate read. It is always present, including when the gate says
   * yes: "why did it let that through" deserves the same answer as "why did it
   * not". */
  input: z.object({
    members: z
      .array(z.object({ ID: z.number(), Hostname: z.string(), IsLearner: z.boolean() }))
      .nullish(),
    voting: z.number().default(0),
    statuses: z.record(z.string(), etcdStatusSchema).nullish(),
    unreachable: z.array(z.string()).nullish(),
    alarms: z.array(z.object({ MemberID: z.number(), Type: z.string() })).nullish(),
    max_raft_lag: z.number().default(0),
  }),
})

export const nodePlanSchema = z.object({
  machine: z.string(),
  hostname: z.string().default(''),
  role: z.string(),
  from: z.string().default(''),
  schematic: z.string().default(''),
  schematic_sentence: z.string().default(''),
  /** The exact installer this node's upgrade will use. Per node, because two
   * nodes in one cluster can have been built from different schematics. */
  installer: z.string().default(''),
  skipped: z.boolean().default(false),
  skip_reason: z.string().default(''),
  /** UPG-04: both lists, because a diff an operator cannot see is a diff they
   * have to take on trust. The upgrade call carries an installer image and
   * nothing else — kernel arguments are written at install time from the
   * machine configuration — so a difference here is silently discarded. */
  kernel_args: z
    .object({
      drifted: z.boolean(),
      in_config: z.array(z.string()).nullish(),
      on_node: z.array(z.string()).nullish(),
      only_on_node: z.array(z.string()).nullish(),
      only_in_config: z.array(z.string()).nullish(),
      sentence: z.string().default(''),
    })
    .nullish(),
  blocked: z.boolean().default(false),
  block_reason: z.string().default(''),
})

export const upgradePlanSchema = z.object({
  cluster: z.string(),
  to: z.string(),
  /** Every version that has to be passed through. A target more than one minor
   * away is several runs, and this is the list of them. */
  chain: z.array(upgradeStepSchema).nullish(),
  strand: strandCheckSchema,
  gate_preview: gateVerdictSchema,
  nodes: z.array(nodePlanSchema).nullish().transform(orEmpty),
  blocked: z.boolean().default(false),
  block_reason: z.string().default(''),
})

export type UpgradePlan = z.infer<typeof upgradePlanSchema>
export type NodePlan = z.infer<typeof nodePlanSchema>
export type GateVerdict = z.infer<typeof gateVerdictSchema>

export const etcdMemberSchema = z.object({
  /** Hex, as a string. A 64-bit member id does not survive a JSON number in a
   * browser, so the string is the canonical form above the seam. */
  id: z.string(),
  /** Hostname first, the hex id behind it. An operator asked to confirm the
   * removal of "8e9e05c52164694d" is confirming a string, not a machine. */
  name: z.string(),
  machine: z.string().default(''),
  learner: z.boolean().default(false),
  voting: z.boolean().default(false),
})

export const etcdMemberListSchema = z.object({
  members: z.array(etcdMemberSchema).nullish().transform(orEmpty),
  voting_count: z.number().default(0),
  /** How many members may be lost before the cluster stops accepting writes. */
  tolerates: z.number().default(0),
  sentence: z.string().default(''),
})

export type EtcdMemberList = z.infer<typeof etcdMemberListSchema>
export type EtcdMember = z.infer<typeof etcdMemberSchema>

/**
 * What a restore answers.
 *
 * `uploaded_bytes` is the whole of the verdict a client can check for itself: a
 * transfer that moved nothing and reported success is the failure worth
 * catching, and it is the same reason the snapshot download reports a length.
 */
export const etcdRestoredSchema = z.object({
  uploaded_bytes: z.number().default(0),
  notice: z.string().default(''),
})

export type EtcdRestored = z.infer<typeof etcdRestoredSchema>

export const releasesSchema = z.object({
  releases: z.array(z.string()).nullish().transform(orEmpty),
  notice: z.string().default(''),
})

/* ---------------------------------------------------------------------- */
/* Jobs and node actions                                                   */
/* ---------------------------------------------------------------------- */

export const jobStateSchema = z.enum([
  'pending',
  'running',
  /**
   * The state that makes this engine worth having. A job interrupted inside a
   * step that cannot be checked afterwards is neither resumed nor failed — a
   * person decides, because both guesses are wrong in a way that matters:
   * retrying a reset wipes twice, failing reports an intact node that is not.
   */
  'parked',
  'succeeded',
  'failed',
  'cancelled',
])

export type JobState = z.infer<typeof jobStateSchema>

export const stepStateSchema = z.enum(['pending', 'running', 'done', 'failed', 'skipped'])

export const jobStepSchema = z.object({
  name: z.string(),
  state: stepStateSchema,
  /** Whether this step had a paired "did it happen?" check when it ran. A step
   * without one parks the job if it is interrupted. */
  verifiable: z.boolean().default(false),
  started_at: z.string().optional(),
  finished_at: z.string().optional(),
  detail: z.string().default(''),
})

export const jobSchema = z.object({
  id: z.string(),
  kind: z.string(),
  cluster: z.string().default(''),
  machine: z.string().default(''),
  state: jobStateSchema,
  steps: z.array(jobStepSchema).nullish().transform(orEmpty),
  current: z.number().default(0),
  params: z.record(z.string(), z.string()).default({}),
  cancel_requested: z.boolean().default(false),
  actor: z.string().default(''),
  /** What a person has to decide. Empty unless the job is parked. */
  parked_reason: z.string().default(''),
  created_at: z.string(),
  started_at: z.string().optional(),
  finished_at: z.string().optional(),
  rev: z.number(),
})

export type Job = z.infer<typeof jobSchema>

export const jobsSchema = z.object({ jobs: z.array(jobSchema) })

export const acceptedJobSchema = z.object({
  job: jobSchema,
  /** Where to watch this job's progress on the event stream. */
  topic: z.string(),
})

export type AcceptedJob = z.infer<typeof acceptedJobSchema>

/**
 * The seven things that can be done to a cluster, a node or an app's power
 * (2026-09-26): one vocabulary for all three, so the menu reads the same
 * wherever it appears. The server says which are possible now and, for each
 * that is not, why -- the screen never guesses.
 */
export const powerActions = [
  'stop',
  'force-stop',
  'start',
  'disable',
  'enable',
  'restart',
  'force-restart',
] as const

export type PowerAction = (typeof powerActions)[number]

export const powerSchema = z.object({
  state: z.string().default('unknown'),
  disabled: z.boolean().default(false),
  actions: z
    .array(
      z.object({
        action: z.string(),
        available: z.boolean().default(false),
        reason: z.string().default(''),
        sudo: z.boolean().default(false),
      }),
    )
    .nullish()
    .transform(orEmpty),
})

export type Power = z.infer<typeof powerSchema>

/** A node or cluster action is a job; an app action is done when it answers. */
export const powerResultSchema = z.union([
  acceptedJobSchema,
  z.object({ message: z.string().default('') }),
])

export type PowerResult = z.infer<typeof powerResultSchema>

export type PowerTarget =
  | { kind: 'cluster'; cluster: string; name: string }
  | { kind: 'node'; machine: string; name: string }
  | { kind: 'app'; cluster: string; namespace: string; appKind: string; name: string }

export function powerPath(target: PowerTarget): string {
  const e = encodeURIComponent
  switch (target.kind) {
    case 'cluster':
      return `/api/v1/clusters/${e(target.cluster)}/power`
    case 'node':
      return `/api/v1/machines/${e(target.machine)}/power`
    case 'app':
      return `/api/v1/clusters/${e(target.cluster)}/kubernetes/apps/${e(target.namespace)}/${e(target.appKind)}/${e(target.name)}/power`
  }
}

/**
 * What rotating a cluster's Talos certificate authority would do.
 *
 * The passes and the warnings come from the server rather than being written
 * here, and that is deliberate: they are statements about what this build does
 * and what it cannot prove, and a copy in the browser is a copy that drifts
 * from the operation it describes.
 */
export const authorityPreviewSchema = z.object({
  cluster: z.string(),
  name: z.string(),
  /** The cluster's name. Typing it is what the server checks before it will
   * issue a confirmation at all. */
  confirm_phrase: z.string(),
  nodes: z.array(
    z.object({
      id: z.string(),
      hostname: z.string().default(''),
      role: z.string(),
    }),
  ),
  /** A rotation was started on this cluster and did not finish. Submitting
   * continues it rather than starting a second one. */
  in_progress: z.boolean(),
  /** Adopted read-only: the route refuses while this holds, so the button says
   * so instead of offering a click that ends in a 403. */
  locked: z.boolean(),
  passes: z.array(z.string()),
  warnings: z.array(z.string()),
})

export type AuthorityPreview = z.infer<typeof authorityPreviewSchema>

/** The rotation's own confirmation answer: the same token shape, with the
 * cluster echoed instead of a machine's parameters. */
export const authorityConfirmationSchema = z.object({
  token: z.string(),
  expires: z.string(),
  action: z.string(),
  cluster: z.string(),
})

/**
 * What Kubernetes says about a cluster (milestone v1.17).
 *
 * The cluster's own view, which is allowed to disagree with the inventory's —
 * and where it does, that disagreement is the information. The inventory knows
 * what the machine API says about a machine; this knows whether the kubelet
 * registered, what the scheduler will do with the node, and whether somebody
 * cordoned it.
 */
export const kubernetesNodeSchema = z.object({
  name: z.string(),
  /** True, False or Unknown. Unknown is a real answer: a kubelet that stopped
   * reporting is unheard from, not NotReady. */
  ready: z.string(),
  /** Somebody cordoned it. A decision, not a state it fell into. */
  unschedulable: z.boolean(),
  roles: z.array(z.string()).nullish().transform(orEmpty),
  kubelet_version: z.string().default(''),
  os_image: z.string().default(''),
  created_at: z.string().default(''),
  internal_address: z.string().default(''),
  container_runtime: z.string().default(''),
})

export const kubernetesPodSchema = z.object({
  namespace: z.string(),
  name: z.string(),
  node: z.string().default(''),
  /** The pod's own claim about its lifecycle. */
  phase: z.string(),
  /** Whether its containers pass their probes. `Running` with 1 of 2 ready is
   * the most misread state in Kubernetes, so both numbers are shown. */
  ready: z.number(),
  containers: z.number(),
  /** Summed across containers, the way kubectl shows it. */
  restarts: z.number(),
  /** CrashLoopBackOff, ImagePullBackOff, Evicted — empty when nothing is
   * wrong. */
  reason: z.string().default(''),
  created_at: z.string().default(''),
  /** What this pod reserved out of its node — requests, because that is what
   * the scheduler holds and therefore why a node is full. Empty means the pod
   * asked for nothing, which is a fact about the pod rather than a zero. */
  cpu_request: z.string().default(''),
  memory_request: z.string().default(''),
  cpu_limit: z.string().default(''),
  memory_limit: z.string().default(''),
})

export const kubernetesDeploymentSchema = z.object({
  namespace: z.string(),
  name: z.string(),
  /** What somebody asked for. */
  desired: z.number(),
  /** What is actually running and passing its probes. Both are here for the
   * reason a pod carries phase and readiness: three asked for and one ready is
   * the state somebody is on this screen about. */
  ready: z.number(),
  updated: z.number(),
  available: z.number(),
  image: z.string().default(''),
  created_at: z.string().default(''),
})

/** One service, and its ports, because reaching one needs a port. */
export const kubernetesServiceSchema = z.object({
  namespace: z.string(),
  name: z.string(),
  /** ClusterIP, NodePort, LoadBalancer or ExternalName — it answers "why can I
   * not reach this from outside", which is the question that brings somebody
   * here. */
  type: z.string().default(''),
  cluster_ip: z.string().default(''),
  ports: z
    .array(
      z.object({
        name: z.string().default(''),
        port: z.number(),
        protocol: z.string().default(''),
      }),
    )
    .nullish()
    .transform(orEmpty),
})

/** What a workload answered. The body is TEXT and there is no content type:
 * serving a pod's own markup from this origin is how a pod would script the
 * interface holding the operator's session. */
export const proxyResponseSchema = z.object({
  status: z.number(),
  body: z.string().default(''),
  truncated: z.boolean().default(false),
})

export type KubernetesService = z.infer<typeof kubernetesServiceSchema>
export type ProxyResponse = z.infer<typeof proxyResponseSchema>

/** One container of a pod, and the cluster's own account of how it is doing. */
export const containerSchema = z.object({
  name: z.string(),
  image: z.string().default(''),
  init: z.boolean().default(false),
  ready: z.boolean().default(false),
  started: z.boolean().default(false),
  restarts: z.number().default(0),
  state: z.string().default(''),
  reason: z.string().default(''),
  message: z.string().default(''),
  /** How the PREVIOUS run ended — the field a crash loop is diagnosed from. */
  last_state: z.string().default(''),
  last_reason: z.string().default(''),
  last_exit_code: z.number().default(0),
  last_finished_at: z.string().default(''),
  /** Whether a previous log can be asked for at all, so the screen offers that
   * button only when it would answer. */
  has_previous: z.boolean().default(false),
  cpu_request: z.string().default(''),
  memory_request: z.string().default(''),
  cpu_limit: z.string().default(''),
  memory_limit: z.string().default(''),
  /** The server's one-line answer to "what is wrong with it". Computed there so
   * the sentence about an exit code is written once. */
  explanation: z.string().default(''),
})

export const containersSchema = z.object({
  containers: z.array(containerSchema).nullish().transform(orEmpty),
})

export const podLogSchema = z.object({
  namespace: z.string(),
  name: z.string(),
  container: z.string(),
  /** Which of the two logs this is, so the screen cannot label the dead
   * container's output as the running one's. */
  previous: z.boolean().default(false),
  lines: z.array(z.string()).nullish().transform(orEmpty),
  /** Cut at the START: the end of a log is the part that explains the failure. */
  truncated: z.boolean().default(false),
})

export const clusterEventSchema = z.object({
  type: z.string().default(''),
  reason: z.string().default(''),
  message: z.string().default(''),
  object: z.string().default(''),
  count: z.number().default(0),
  first_seen: z.string().default(''),
  last_seen: z.string().default(''),
})

export const clusterEventsSchema = z.object({
  events: z.array(clusterEventSchema).nullish().transform(orEmpty),
  /** Why an empty list is not a claim that nothing happened. */
  notice: z.string().default(''),
})

export const describedSchema = z.object({
  api_version: z.string().default(''),
  kind: z.string().default(''),
  namespace: z.string().default(''),
  name: z.string().default(''),
  yaml: z.string().default(''),
  truncated: z.boolean().default(false),
  /** What was left out and why. An object silently missing fields is how
   * somebody concludes a field is not set when it is. */
  notice: z.string().default(''),
})

export const permissionSchema = z.object({
  verb: z.string(),
  resource: z.string(),
  namespace: z.string().default(''),
  allowed: z.boolean().default(false),
  /** The authoriser's own sentence. An RBAC denial usually gives none, which is
   * itself worth showing rather than replacing with a guess. */
  reason: z.string().default(''),
})

export const kubeIdentitySchema = z.object({
  user: z.string().default(''),
  groups: z.array(z.string()).nullish().transform(orEmpty),
  /** What to put on the screen, including the case where it is this product's
   * own certificate. */
  describes: z.string().default(''),
  permissions: z.array(permissionSchema).nullish().transform(orEmpty),
  missing: z.number().default(0),
  notice: z.string().default(''),
})

export const workloadSchema = z.object({
  kind: z.string(),
  namespace: z.string(),
  name: z.string(),
  desired: z.number().default(0),
  ready: z.number().default(0),
  /** The honest one-line state, written per kind on the server: "3 of 3 ready"
   * is wrong for a CronJob and misleading for a Job. */
  summary: z.string().default(''),
  image: z.string().default(''),
  created_at: z.string().default(''),
  schedule: z.string().default(''),
  suspended: z.boolean().default(false),
  last_run: z.string().default(''),
  /** Whether this kind takes a replica count at all — a DaemonSet does not. */
  scalable: z.boolean().default(false),
  /** Whether `rollout restart` applies — a Job has no pod template. */
  rollable: z.boolean().default(false),
  /** Whether this kind has a stop at all — a DaemonSet does not, because its
   * size is how many nodes match rather than a count somebody set. */
  stoppable: z.boolean().default(false),
  stopped: z.boolean().default(false),
  /** What a start would bring back, read from the annotation the stop wrote.
   * Zero means nothing was written down and a start would run one. */
  would_start_with: z.number().default(0),
})

export const workloadsSchema = z.object({
  workloads: z.array(workloadSchema).nullish().transform(orEmpty),
})

export const clusterResourceSchema = z.object({
  kind: z.string(),
  namespace: z.string(),
  name: z.string(),
  /** The answer this kind exists to give, written per kind on the server. */
  summary: z.string().default(''),
  detail: z.string().default(''),
  /** False when this object is the reason something else is stuck: a Pending
   * claim, an Ingress no controller claimed, a budget allowing no disruption. */
  healthy: z.boolean().default(true),
  created_at: z.string().default(''),
})

export const clusterResourcesSchema = z.object({
  resources: z.array(clusterResourceSchema).nullish().transform(orEmpty),
})

export const nodeConditionSchema = z.object({
  type: z.string(),
  status: z.string(),
  reason: z.string().default(''),
  message: z.string().default(''),
  /** True when this condition is the one stopping work. Ready is bad when
   * False; a pressure is bad when True — the inversion a naive screen gets
   * backwards. */
  bad: z.boolean().default(false),
})

export const nodeTaintSchema = z.object({
  key: z.string(),
  value: z.string().default(''),
  effect: z.string().default(''),
  /** What the effect does, in words: NoSchedule keeps pods off, NoExecute
   * removes the ones already there. */
  explanation: z.string().default(''),
})

export const nodeDetailSchema = z.object({
  name: z.string(),
  conditions: z.array(nodeConditionSchema).nullish().transform(orEmpty),
  taints: z.array(nodeTaintSchema).nullish().transform(orEmpty),
  /** The taints worth pointing at — the control-plane one is ordinary and is
   * left out, so a first visit does not look like a misconfiguration. */
  explaining_taints: z.array(nodeTaintSchema).nullish().transform(orEmpty),
  cpu_allocatable: z.string().default(''),
  cpu_requested: z.string().default(''),
  memory_allocatable: z.string().default(''),
  memory_requested: z.string().default(''),
  pods_running: z.number().default(0),
  pod_capacity: z.number().default(0),
  /** Why "requested" is not "used". */
  notice: z.string().default(''),
  labels: z.record(z.string(), z.string()).default({}),
})

export const usageSchema = z.object({
  namespace: z.string().default(''),
  name: z.string(),
  cpu_millis: z.number().default(0),
  memory_bytes: z.number().default(0),
  cpu: z.string().default(''),
  memory: z.string().default(''),
})

export const clusterUsageSchema = z.object({
  /** False when no metrics-server is installed — which is most clusters, and is
   * not the same as usage being zero. */
  collecting: z.boolean().default(false),
  nodes: z.array(usageSchema).nullish().transform(orEmpty),
  pods: z.array(usageSchema).nullish().transform(orEmpty),
  notice: z.string().default(''),
})

export const execResultSchema = z.object({
  namespace: z.string(),
  pod: z.string(),
  container: z.string(),
  command: z.array(z.string()).nullish().transform(orEmpty),
  stdout: z.string().default(''),
  stderr: z.string().default(''),
  truncated: z.boolean().default(false),
  /** Who the cluster saw run it. Carried so the screen can say it rather than
   * implying the product ran it. */
  identity: z.string().default(''),
})

export type ExecResult = z.infer<typeof execResultSchema>
export const capacitySchema = z.object({
  allocatable: z.string().default(''),
  requested: z.string().default(''),
  /** -1 when allocatable is unknown: a node that has not reported is not a node
   * at 0%. */
  percent: z.number().default(-1),
})

export const nodeCapacitySchema = z.object({
  name: z.string(),
  ready: z.boolean().default(false),
  cordoned: z.boolean().default(false),
  cpu: capacitySchema,
  memory: capacitySchema,
  pods: capacitySchema,
  pods_running: z.number().default(0),
})

export const clusterCapacitySchema = z.object({
  cpu: capacitySchema,
  memory: capacitySchema,
  pods: capacitySchema,
  nodes: z.array(nodeCapacitySchema).nullish().transform(orEmpty),
  notice: z.string().default(''),
})

export const stoppedStateSchema = z.object({
  stopped: z.boolean().default(false),
  /** What a start would restore. Zero means nothing was written down. */
  would_start_with: z.number().default(0),
})

export const sweepableSchema = z.object({
  kind: z.string(),
  namespace: z.string().default(''),
  name: z.string(),
  /** Why this one qualifies. A list of names with no reasons is a list nobody
   * can check before pressing the button. */
  reason: z.string().default(''),
  age: z.string().default(''),
})

export const sweepPlanSchema = z.object({
  items: z.array(sweepableSchema).nullish().transform(orEmpty),
  notice: z.string().default(''),
})

export const volumeSummarySchema = z.object({
  name: z.string(),
  capacity: z.string().default(''),
  /** Bound, Available, Released or Failed. */
  phase: z.string().default(''),
  /** "namespace/name", or empty when nothing holds it. */
  claim: z.string().default(''),
  storage_class: z.string().default(''),
  /** What happens to the data when the claim goes: Delete destroys it, Retain
   * keeps it. The field somebody needs before deleting and the one no claim
   * carries. */
  reclaim_policy: z.string().default(''),
  access_modes: z.array(z.string()).nullish().transform(orEmpty),
  driver: z.string().default(''),
  notice: z.string().default(''),
  created_at: z.string().default(''),
})

export const claimSummarySchema = z.object({
  namespace: z.string(),
  name: z.string(),
  phase: z.string().default(''),
  requested: z.string().default(''),
  capacity: z.string().default(''),
  storage_class: z.string().default(''),
  volume: z.string().default(''),
  access_modes: z.array(z.string()).nullish().transform(orEmpty),
  /** Every running pod that mounts it. Empty is a real answer: storage being
   * paid for and not used. */
  used_by: z.array(z.string()).nullish().transform(orEmpty),
  expandable: z.boolean().default(false),
  notice: z.string().default(''),
  created_at: z.string().default(''),
})

export const storageClassSummarySchema = z.object({
  name: z.string(),
  provisioner: z.string().default(''),
  default: z.boolean().default(false),
  reclaim_policy: z.string().default(''),
  /** Immediate or WaitForFirstConsumer. The second makes a claim sit Pending
   * until a pod schedules, which is correct and looks like a fault. */
  binding_mode: z.string().default(''),
  allows_expansion: z.boolean().default(false),
  created_at: z.string().default(''),
})

export const clusterStorageSchema = z.object({
  volumes: z.array(volumeSummarySchema).nullish().transform(orEmpty),
  claims: z.array(claimSummarySchema).nullish().transform(orEmpty),
  classes: z.array(storageClassSummarySchema).nullish().transform(orEmpty),
  notice: z.string().default(''),
})

export const serviceSummarySchema = z.object({
  namespace: z.string(),
  name: z.string(),
  type: z.string().default(''),
  cluster_ip: z.string().default(''),
  /** What reaches it from outside. Empty for a plain ClusterIP, which is the
   * answer to "why can I not reach this from my laptop". */
  external: z.string().default(''),
  ports: z.array(z.string()).nullish().transform(orEmpty),
  selector: z.string().default(''),
  /** How many addresses are behind it, and how many of those are READY. An
   * unready endpoint is excluded from load balancing entirely. */
  endpoints: z.number().default(0),
  ready_endpoints: z.number().default(0),
  healthy: z.boolean().default(true),
  notice: z.string().default(''),
  created_at: z.string().default(''),
})

export const policySummarySchema = z.object({
  namespace: z.string(),
  name: z.string(),
  /** The pod selector in words: "every pod in the namespace" for an empty one,
   * which is the case that surprises people. */
  applies: z.string().default(''),
  types: z.array(z.string()).nullish().transform(orEmpty),
  /** How many pods it currently matches. Zero means somebody believes something
   * is protected and nothing is. */
  selects: z.number().default(0),
  isolating: z.boolean().default(false),
  summary: z.string().default(''),
  healthy: z.boolean().default(true),
  created_at: z.string().default(''),
})

export const clusterNetworkSchema = z.object({
  services: z.array(serviceSummarySchema).nullish().transform(orEmpty),
  policies: z.array(policySummarySchema).nullish().transform(orEmpty),
  /** Namespaces with pods and no NetworkPolicy: they accept traffic from every
   * pod in the cluster. */
  unprotected: z.array(z.string()).nullish().transform(orEmpty),
  ingress_classes: z.array(z.string()).nullish().transform(orEmpty),
  default_ingress_class: z.string().default(''),
  notice: z.string().default(''),
})

export const rbacSubjectSchema = z.object({
  /** User, Group or ServiceAccount. */
  kind: z.string().default(''),
  namespace: z.string().default(''),
  name: z.string().default(''),
  /** Whether `exists` means anything: only a ServiceAccount can be checked, and
   * a column saying "does not exist" for every OIDC user would teach that the
   * column is noise. */
  checkable: z.boolean().default(false),
  exists: z.boolean().default(false),
})

export const bindingSummarySchema = z.object({
  kind: z.string().default(''),
  namespace: z.string().default(''),
  name: z.string().default(''),
  role_kind: z.string().default(''),
  role_name: z.string().default(''),
  /** False for a binding that grants nothing because the role it names is not
   * there. RBAC has no referential integrity, so nothing else says this. */
  role_exists: z.boolean().default(false),
  subjects: z.array(rbacSubjectSchema).nullish().transform(orEmpty),
  /** Wildcard verbs on wildcard resources, whatever the role is called. */
  administrative: z.boolean().default(false),
  summary: z.string().default(''),
  healthy: z.boolean().default(true),
  created_at: z.string().default(''),
})

export const roleSummarySchema = z.object({
  kind: z.string().default(''),
  namespace: z.string().default(''),
  name: z.string().default(''),
  rules: z.array(z.string()).nullish().transform(orEmpty),
  administrative: z.boolean().default(false),
  bound: z.number().default(0),
  /** A role Kubernetes ships. There are about seventy and they are the same on
   * every cluster, so a screen can fold them away. */
  built_in: z.boolean().default(false),
  created_at: z.string().default(''),
})

export const serviceAccountSummarySchema = z.object({
  namespace: z.string().default(''),
  name: z.string().default(''),
  /** Every running pod that runs as it. The "default" account is what every pod
   * naming none runs as. */
  used_by: z.array(z.string()).nullish().transform(orEmpty),
  bindings: z.number().default(0),
  administrative: z.boolean().default(false),
  notice: z.string().default(''),
  created_at: z.string().default(''),
})

export const accessControlSchema = z.object({
  bindings: z.array(bindingSummarySchema).nullish().transform(orEmpty),
  roles: z.array(roleSummarySchema).nullish().transform(orEmpty),
  accounts: z.array(serviceAccountSummarySchema).nullish().transform(orEmpty),
  /** Every subject reaching wildcard-on-wildcard through some binding. Not a
   * field anywhere in Kubernetes, and the first thing anybody wants to know. */
  administrators: z.array(z.string()).nullish().transform(orEmpty),
  notice: z.string().default(''),
})

export const quotaLineSchema = z.object({
  quota: z.string().default(''),
  resource: z.string().default(''),
  used: z.string().default(''),
  hard: z.string().default(''),
  /** Of hard, rounded; -1 when hard cannot be read as a number. */
  percent: z.number().default(-1),
})

export const namespaceSummarySchema = z.object({
  name: z.string(),
  /** Active or Terminating. */
  phase: z.string().default(''),
  pods_running: z.number().default(0),
  quotas: z.array(quotaLineSchema).nullish().transform(orEmpty),
  /** Whether pods that set no requests get defaults. With a compute quota and
   * without this, every such pod is refused. */
  has_limit_range: z.boolean().default(false),
  healthy: z.boolean().default(true),
  notice: z.string().default(''),
  created_at: z.string().default(''),
})

export const customKindSchema = z.object({
  group: z.string().default(''),
  kind: z.string().default(''),
  versions: z.array(z.string()).nullish().transform(orEmpty),
  stored: z.string().default(''),
  scope: z.string().default(''),
  /** Whether the API server is serving it. A definition that is not established
   * is a kind every manifest naming it is refused for. */
  established: z.boolean().default(false),
  notice: z.string().default(''),
  created_at: z.string().default(''),
})

export const inventorySchema = z.object({
  namespaces: z.array(namespaceSummarySchema).nullish().transform(orEmpty),
  custom_kinds: z.array(customKindSchema).nullish().transform(orEmpty),
  notice: z.string().default(''),
})

export type Inventory = z.infer<typeof inventorySchema>
export type NamespaceSummary = z.infer<typeof namespaceSummarySchema>
export type CustomKind = z.infer<typeof customKindSchema>

export type AccessControl = z.infer<typeof accessControlSchema>
export type BindingSummary = z.infer<typeof bindingSummarySchema>
export type RoleSummary = z.infer<typeof roleSummarySchema>
export type ServiceAccountSummary = z.infer<typeof serviceAccountSummarySchema>

export type ClusterStorage = z.infer<typeof clusterStorageSchema>
export type VolumeSummary = z.infer<typeof volumeSummarySchema>
export type ClaimSummary = z.infer<typeof claimSummarySchema>
export type StorageClassSummary = z.infer<typeof storageClassSummarySchema>
export type ClusterNetwork = z.infer<typeof clusterNetworkSchema>
export type ServiceSummary = z.infer<typeof serviceSummarySchema>
export type PolicySummary = z.infer<typeof policySummarySchema>

export const wallTileSchema = z.object({
  kind: z.string().default(''),
  namespace: z.string().default(''),
  name: z.string().default(''),
  /** ok, warn, down, stopped or unknown. Decided by the server so that nothing
   * on the wall works a colour out from four numbers. */
  state: z.string().default('unknown'),
  detail: z.string().default(''),
})

export const wallWarningSchema = z.object({
  object: z.string().default(''),
  reason: z.string().default(''),
  message: z.string().default(''),
  count: z.number().default(0),
  /** WHEN, as an instant. The age is worked out in the browser: this screen
   * keeps the last answer up when a refresh fails, and an age baked in at the
   * server would freeze at the moment the daemon stopped answering. */
  last_seen: z.string().default(''),
})

export const wallNamespaceSchema = z.object({
  name: z.string().default(''),
  total: z.number().default(0),
  state: z.string().default('unknown'),
  /** The thing that decided the colour, as "postgres · 2 of 3 ready". Empty
   * when nothing is wrong: a tile that always carries a sentence is a tile
   * whose sentence nobody reads. */
  worst: z.string().default(''),
  stopped: z.number().default(0),
})

export const wallSchema = z.object({
  /** When this was true. The screen says how old it is and goes visibly stale,
   * because a wall that cannot go stale lies during exactly the incident it
   * exists for. */
  generated_at: z.string().default(''),
  nodes: z.array(wallTileSchema).nullish().transform(orEmpty),
  workloads: z.array(wallTileSchema).nullish().transform(orEmpty),
  namespaces: z.array(wallNamespaceSchema).nullish().transform(orEmpty),
  cpu: capacitySchema,
  memory: capacitySchema,
  pods: capacitySchema,
  warnings: z.array(wallWarningSchema).nullish().transform(orEmpty),
  summary: z
    .record(z.string(), z.number())
    .nullish()
    .transform((v) => v ?? {}),
})

export const wallLinkSchema = z.object({
  id: z.string(),
  label: z.string().default(''),
  created_at: z.string().default(''),
  created_by: z.string().default(''),
  /** Empty for a link no screen has ever used. That is the question the list
   * exists for: is this still on a wall somewhere? */
  last_used_at: z.string().default(''),
})

export const wallLinksSchema = z.object({
  links: z.array(wallLinkSchema).nullish().transform(orEmpty),
})

export const createdWallLinkSchema = z.object({
  link: wallLinkSchema,
  /** Shown once and never again: only the hash is stored. */
  token: z.string(),
  notice: z.string().default(''),
})

export type WallLink = z.infer<typeof wallLinkSchema>

export type Wall = z.infer<typeof wallSchema>
export type WallTile = z.infer<typeof wallTileSchema>
export type WallNamespace = z.infer<typeof wallNamespaceSchema>
export type WallWarning = z.infer<typeof wallWarningSchema>

export type Capacity = z.infer<typeof capacitySchema>
export type ClusterCapacity = z.infer<typeof clusterCapacitySchema>
export type NodeCapacity = z.infer<typeof nodeCapacitySchema>
export type Sweepable = z.infer<typeof sweepableSchema>
export type ClusterUsage = z.infer<typeof clusterUsageSchema>

/**
 * An app is what somebody deployed -- a Deployment, StatefulSet, DaemonSet,
 * CronJob, Job, or a pod nothing owns -- with what its pods are using right now,
 * read from each node's kubelet (2026-09-26). `usage_known` false is "nobody
 * could say", which is not the same as zero.
 */
export const appSchema = z.object({
  namespace: z.string(),
  kind: z.string(),
  name: z.string(),
  pods: z.number().default(0),
  ready: z.number().default(0),
  restarts: z.number().default(0),
  nodes: z.array(z.string()).nullish().transform(orEmpty),
  cpu_millis: z.number().default(0),
  memory_bytes: z.number().default(0),
  cpu_request_millis: z.number().default(0),
  cpu_limit_millis: z.number().default(0),
  memory_request_bytes: z.number().default(0),
  memory_limit_bytes: z.number().default(0),
  usage_known: z.boolean().default(false),
  images: z.array(z.string()).nullish().transform(orEmpty),
})

export type App = z.infer<typeof appSchema>

export const appsSchema = z.object({
  collected_at: z.string().default(''),
  usage_available: z.boolean().default(false),
  notice: z.string().default(''),
  apps: z.array(appSchema).nullish().transform(orEmpty),
})

export type Apps = z.infer<typeof appsSchema>

const appContainerSchema = z.object({
  name: z.string(),
  image: z.string().default(''),
  state: z.string().default(''),
  restarts: z.number().default(0),
  cpu_millis: z.number().default(0),
  memory_bytes: z.number().default(0),
  usage_known: z.boolean().default(false),
  cpu_request_millis: z.number().default(0),
  cpu_limit_millis: z.number().default(0),
  memory_request_bytes: z.number().default(0),
  memory_limit_bytes: z.number().default(0),
})

export const appPodSchema = z.object({
  name: z.string(),
  node: z.string().default(''),
  phase: z.string().default(''),
  ready: z.string().default(''),
  restarts: z.number().default(0),
  started_at: z.string().default(''),
  ip: z.string().default(''),
  cpu_millis: z.number().default(0),
  memory_bytes: z.number().default(0),
  usage_known: z.boolean().default(false),
  containers: z.array(appContainerSchema).nullish().transform(orEmpty),
})

export type AppPod = z.infer<typeof appPodSchema>

export const appDetailSchema = z.object({
  collected_at: z.string().default(''),
  usage_available: z.boolean().default(false),
  notice: z.string().default(''),
  app: appSchema,
  created_at: z.string().default(''),
  pods: z.array(appPodSchema).nullish().transform(orEmpty),
  services: z
    .array(
      z.object({
        name: z.string(),
        type: z.string().default(''),
        cluster_ip: z.string().default(''),
        ports: z.array(z.string()).nullish().transform(orEmpty),
      }),
    )
    .nullish()
    .transform(orEmpty),
  events: z
    .array(
      z.object({
        type: z.string().default(''),
        reason: z.string().default(''),
        message: z.string().default(''),
        last_seen: z.string().default(''),
        count: z.number().default(0),
        object: z.string().default(''),
      }),
    )
    .nullish()
    .transform(orEmpty),
})

export type AppDetail = z.infer<typeof appDetailSchema>
export type NodeDetail = z.infer<typeof nodeDetailSchema>
export type ClusterResource = z.infer<typeof clusterResourceSchema>
export type Workload = z.infer<typeof workloadSchema>
export type KubeIdentity = z.infer<typeof kubeIdentitySchema>
export type Described = z.infer<typeof describedSchema>
export type KubeContainer = z.infer<typeof containerSchema>
export type PodLog = z.infer<typeof podLogSchema>
export type ClusterEvent = z.infer<typeof clusterEventSchema>

export const kubernetesOverviewSchema = z.object({
  cluster: z.string(),
  /** The API server's own version: the cheapest proof that the whole path
   * works — address, TLS, certificate, authorisation. */
  server_version: z.string(),
  nodes: z.array(kubernetesNodeSchema),
  pods: z.array(kubernetesPodSchema),
  deployments: z.array(kubernetesDeploymentSchema),
  services: z.array(kubernetesServiceSchema).nullish().transform(orEmpty),
  namespaces: z.array(z.string()).nullish().transform(orEmpty),
  /** Echoed back, so a screen cannot show one namespace's pods under
   * another's heading. */
  namespace: z.string().default(''),
})

/** One document of a manifest, as the plan describes it. */
export const manifestObjectSchema = z.object({
  api_version: z.string().default(''),
  kind: z.string().default(''),
  namespace: z.string().default(''),
  name: z.string().default(''),
  /** `create` or `update` in a plan, `applied` in a result — and in a plan it
   * comes from asking the cluster, not from reading the manifest. The manifest
   * looks identical either way. */
  action: z.string().default(''),
  /** Whether the kind lives in a namespace, according to the cluster's own
   * discovery. */
  namespaced: z.boolean().default(false),
})

export const manifestPlanSchema = z.object({
  objects: z.array(manifestObjectSchema).nullish().transform(orEmpty),
  /** Things that are true and worth reading before applying: an object that
   * would land in `default`, a kind this cluster does not have. */
  warnings: z.array(z.string()).nullish().transform(orEmpty),
})

export const manifestApplyResultSchema = z.object({
  applied: z.array(manifestObjectSchema).nullish().transform(orEmpty),
  failed: z
    .array(z.object({ object: manifestObjectSchema, reason: z.string().default('') }))
    .nullish()
    .transform(orEmpty),
  /** False when anything failed. The server answers 200 with the per-object
   * truth either way: an apply of ten where the sixth conflicted changed five
   * things, and one status code cannot say which five. */
  fully_applied: z.boolean().default(false),
})

export type ManifestObject = z.infer<typeof manifestObjectSchema>
export type ManifestPlan = z.infer<typeof manifestPlanSchema>
export type ManifestApplyResult = z.infer<typeof manifestApplyResultSchema>

export type KubernetesOverview = z.infer<typeof kubernetesOverviewSchema>
export type KubernetesNode = z.infer<typeof kubernetesNodeSchema>
export type KubernetesPod = z.infer<typeof kubernetesPodSchema>
export type KubernetesDeployment = z.infer<typeof kubernetesDeploymentSchema>

export const confirmationSchema = z.object({
  token: z.string(),
  expires: z.string(),
  /** Echoed back, so the screen can show what this token authorises rather
   * than what it believes it asked for. */
  action: z.string(),
  params: z.record(z.string(), z.string()).default({}),
})

export const resetModeSchema = z.object({
  mode: z.string(),
  label: z.string(),
  description: z.string(),
  needs_disks: z.boolean().default(false),
})

export const resetPreviewSchema = z.object({
  machine: z.string(),
  hostname: z.string(),
  /** What has to be typed: the machine's own name, because that is the thing
   * an operator can check against the machine in front of them. */
  confirm_phrase: z.string(),
  disks: z.array(diskSchema).nullish().transform(orEmpty),
  /** Least destructive first. A list whose first option wipes the machine is a
   * list somebody will click through. */
  modes: z.array(resetModeSchema),
  defaults: z.object({
    mode: z.string(),
    graceful: z.boolean(),
    reboot: z.boolean(),
  }),
  /** How talosctl's own defaults differ, said out loud: it defaults to wiping
   * every disk and leaving the machine off. */
  talos_default_warning: z.string(),
})

export type ResetPreview = z.infer<typeof resetPreviewSchema>

/* ---------------------------------------------------------------------- */
/* Machine configuration                                                   */
/* ---------------------------------------------------------------------- */

export const configViewSchema = z.object({
  machine: z.string(),
  /**
   * Both forms are **already redacted by the server**. There is no unredacted
   * field and no flag that produces one: a machine configuration carries the
   * cluster CA private key, so "look at a node's config" would otherwise be
   * the same feature as "hand the cluster over".
   */
  raw: z.string(),
  rendered: z.string(),
  read_at: z.string(),
  /** A configuration is waiting for this node's next boot, as far as this
   * process knows. The qualifier is real: the record is in memory, so a
   * restart forgets it and a reboot outside holzkube-manager clears it on the node
   * without clearing it here. */
  staged_pending: z.boolean().default(false),
  staged_at: z.string().optional(),
})

export type ConfigView = z.infer<typeof configViewSchema>

export const changeSchema = z.object({
  path: z.string(),
  kind: z.enum(['added', 'removed', 'modified', 'list-changed']),
  before: z.string().default(''),
  after: z.string().default(''),
  len_before: z.number().default(0),
  len_after: z.number().default(0),
  /** Values that appear twice in the list after the change. Nearly always a
   * patch that was applied a second time. */
  duplicates: z.array(z.string()).nullish().transform(orEmpty),
})

export const verdictSchema = z.object({
  mode: z.enum(['no-reboot', 'reboot', 'staged', 'try']),
  reboot_required: z.boolean(),
  reboot_paths: z.array(z.string()).nullish().transform(orEmpty),
  /** Paths under .machine.install: they apply and change nothing until the
   * next install or upgrade. */
  install_only: z.array(z.string()).nullish().transform(orEmpty),
  network_paths: z.array(z.string()).nullish().transform(orEmpty),
  /** The countdown for a `try` apply. It is the same number the node uses, or
   * the screen would be lying about how long is left. */
  try_seconds: z.number().default(0),
  sentences: z.array(z.string()).nullish().transform(orEmpty),
})

export const previewSchema = z.object({
  machine: z.string(),
  diff: z.object({
    changes: z.array(changeSchema).nullish().transform(orEmpty),
    paths: z.array(z.string()).nullish().transform(orEmpty),
    duplicates: z.boolean().default(false),
  }),
  verdict: verdictSchema,
  result: z.string(),
  /** False is nearly always a patch that appends to a list. */
  idempotent: z.boolean(),
  valid: z.boolean(),
  validation: z.array(z.string()).nullish().transform(orEmpty),
})

export type ConfigPreview = z.infer<typeof previewSchema>

export const patchSchema = z.object({
  id: z.string(),
  name: z.string(),
  cluster: z.string().default(''),
  version: z.number(),
  parent: z.string().default(''),
  /** Marked, never deleted: "what exactly was applied in March" only has an
   * answer if the thing applied still exists. */
  superseded: z.boolean().default(false),
  body: z.string(),
  description: z.string().default(''),
  author: z.string().default(''),
  created_at: z.string(),
  rev: z.number(),
})

export type Patch = z.infer<typeof patchSchema>

export const patchesSchema = z.object({ patches: z.array(patchSchema) })

export const applyResultSchema = z.object({
  mode: z.string(),
  details: z.string().default(''),
})

export const api = {
  /** The running build and its changelog. Behind the session gate, which is
   *  where the sidebar that shows it already is. */
  version: (): Promise<VersionInfo> => sendJSON('GET', '/api/v1/system/version', versionSchema),

  status: (): Promise<SystemStatus> =>
    sendJSON('GET', '/api/v1/system/status', systemStatusSchema, undefined, {
      interceptUnauthenticated: false,
    }),

  setup: async (username: string, password: string): Promise<void> => {
    await send(
      'POST',
      '/api/v1/setup',
      { username, password },
      {
        interceptUnauthenticated: false,
      },
    )
  },

  login: async (username: string, password: string): Promise<void> => {
    await send(
      'POST',
      '/api/v1/auth/login',
      { username, password },
      {
        interceptUnauthenticated: false,
      },
    )
  },

  logout: async (): Promise<void> => {
    await send('POST', '/api/v1/auth/logout', {}, { interceptUnauthenticated: false })
  },

  /**
   * The session probe. Its 401 means "not signed in", which is an answer and
   * not an expiry, so it never triggers the login transition itself.
   */
  me: (): Promise<Me> =>
    sendJSON('GET', '/api/v1/auth/me', meSchema, undefined, {
      interceptUnauthenticated: false,
    }),

  /**
   * Re-authenticate to open the five-minute sudo window (D-05). A wrong
   * password here is 401 -- the session is fine, the credential was not -- and
   * must not be turned into a login transition, so the interceptor is off.
   */
  sudo: async (password: string): Promise<void> => {
    await send(
      'POST',
      '/api/v1/auth/sudo',
      { password },
      {
        interceptUnauthenticated: false,
        interceptSudo: false,
      },
    )
  },

  changePassword: async (currentPassword: string, newPassword: string): Promise<void> => {
    await send('POST', '/api/v1/account/password', {
      current_password: currentPassword,
      new_password: newPassword,
    })
  },

  audit: (query: AuditQuery = {}): Promise<AuditPage> =>
    sendJSON('GET', `/api/v1/audit${auditQueryString(query)}`, auditPageSchema),

  factory: {
    versions: (): Promise<FactoryVersions> =>
      sendJSON('GET', '/api/v1/factory/versions', factoryVersionsSchema),

    /**
     * The catalog for exactly this version. There is no unscoped call and no
     * cached fallback, because an extension list from the wrong version
     * validates successfully and produces an image that will not build.
     */
    extensions: (version: string): Promise<FactoryExtensionCatalog> =>
      sendJSON(
        'GET',
        `/api/v1/factory/extensions?version=${encodeURIComponent(version)}`,
        factoryExtensionsSchema,
      ),
  },

  schematics: {
    list: (): Promise<Schematic[]> =>
      sendJSON('GET', '/api/v1/schematics', z.array(schematicSchema)),

    get: (id: string): Promise<Schematic> =>
      sendJSON('GET', `/api/v1/schematics/${encodeURIComponent(id)}`, schematicSchema),

    create: (input: SchematicInput): Promise<CreatedSchematic> =>
      sendJSON('POST', '/api/v1/schematics', createdSchematicSchema, input),

    assets: (id: string, query: AssetQuery): Promise<SchematicAssets> =>
      sendJSON(
        'GET',
        `/api/v1/schematics/${encodeURIComponent(id)}/assets${assetQueryString(query)}`,
        schematicAssetsSchema,
      ),

    /**
     * Destructive, so the 428 interceptor above opens the sudo dialog and
     * replays this exact request once the window is open. This screen therefore
     * has no confirmation of its own -- a second one would train the operator
     * to click past the first.
     */
    remove: async (id: string): Promise<void> => {
      await send('DELETE', `/api/v1/schematics/${encodeURIComponent(id)}`)
    },
  },

  clusters: {
    list: async (): Promise<Cluster[]> =>
      (await sendJSON('GET', '/api/v1/clusters', clustersSchema)).clusters,

    get: (id: string): Promise<Cluster> =>
      sendJSON('GET', `/api/v1/clusters/${encodeURIComponent(id)}`, clusterSchema),

    /**
     * Step one of the adoption: read the certificate the operator is about to
     * trust. Nothing is stored and nothing is trusted by this call — it exists
     * so that what they confirm is the certificate that will actually be used.
     */
    fingerprint: (endpoint: string): Promise<{ endpoint: string; fingerprint: string }> =>
      sendJSON('POST', '/api/v1/clusters/fingerprint', fingerprintSchema, { endpoint }),

    /** Step two. The talosconfig travels in the body, uploaded or pasted;
     * there is deliberately no way to name a path on the server. */
    import: (input: ImportInput): Promise<Cluster> =>
      sendJSON('POST', '/api/v1/clusters', clusterSchema, input),

    /**
     * The *other* path onto a cluster record, and the only one that mints a
     * certificate authority. It is a separate route and not a mode of the
     * import for a reason a test can check: an import that quietly generated
     * its own PKI would produce a cluster that looks right and opens nothing.
     */
    create: (name: string, endpoint: string): Promise<Cluster> =>
      sendJSON('POST', '/api/v1/clusters/create', clusterSchema, { name, endpoint }),

    /** Where the browser downloads an admin talosconfig for a cluster. The
     * certificate in it is minted on demand and is not the one holzkube-manager
     * dials with. */
    talosconfigPath: (id: string): string =>
      `/api/v1/clusters/${encodeURIComponent(id)}/talosconfig`,

    /**
     * The support bundle, as a path rather than a fetch.
     *
     * It is a plain link for the same reason the talosconfig is: the response
     * is a file the operator is going to keep, and pulling tens of megabytes
     * of gzip through `fetch` only to hand it back to the browser as a blob
     * buffers the whole archive in the tab — during the incident the archive
     * is being collected for. A GET carries the session cookie and needs no
     * CSRF header, so the browser's own download is both simpler and better
     * behaved than anything this file could do.
     */
    supportBundlePath: (id: string): string =>
      `/api/v1/clusters/${encodeURIComponent(id)}/support-bundle`,

    /**
     * The cluster's admin kubeconfig, as a path rather than a fetch, for the
     * same reason as the two above.
     *
     * Unlike them it reaches a node: the kubeconfig is rendered by a
     * control-plane node from the machine configuration it is running, not by
     * holzkube-manager from anything it stores. So this one can fail because
     * the cluster is unreachable, and the browser shows the problem document
     * rather than downloading it — which is the ordinary behaviour of a link
     * to something that is not there, and better than a spinner.
     */
    kubeconfigPath: (id: string): string => `/api/v1/clusters/${encodeURIComponent(id)}/kubeconfig`,

    /**
     * Unlocking is destructive: it is what makes every other destructive route
     * reachable on this cluster. The 428 interceptor opens the password prompt
     * and replays this request, so this screen needs no confirmation of its
     * own.
     */
    setLock: (id: string, locked: boolean): Promise<Cluster> =>
      sendJSON('POST', `/api/v1/clusters/${encodeURIComponent(id)}/lock`, clusterSchema, {
        locked,
      }),

    forget: async (id: string): Promise<void> => {
      await send('DELETE', `/api/v1/clusters/${encodeURIComponent(id)}`)
    },
  },

  power: {
    get: (target: PowerTarget): Promise<Power> => sendJSON('GET', powerPath(target), powerSchema),
    run: (target: PowerTarget, action: PowerAction): Promise<PowerResult> =>
      // The body is empty and required: the CSRF layer wants a JSON content
      // type on every mutating route, and only a body carries one.
      sendJSON('POST', `${powerPath(target)}/${action}`, powerResultSchema, {}),
  },

  machines: {
    list: async (): Promise<Machine[]> =>
      (await sendJSON('GET', '/api/v1/machines', machinesSchema)).machines,

    get: (id: string): Promise<Machine> =>
      sendJSON('GET', `/api/v1/machines/${encodeURIComponent(id)}`, machineSchema),

    add: (cluster: string, addr: string): Promise<Machine> =>
      sendJSON('POST', '/api/v1/machines', machineSchema, { cluster, addr }),

    /** One observation pass, now. It never fails because the node is down: an
     * unreachable node comes back as a record whose fields are stale. */
    refresh: (id: string): Promise<Machine> =>
      sendJSON('POST', `/api/v1/machines/${encodeURIComponent(id)}/refresh`, machineSchema),

    /** The node's hardware as it is now: load, temperatures, fans, memory,
     * disks, network. Read from the node on every call; nothing is cached but
     * the previous counters the rates are computed against. */
    hardware: (id: string): Promise<Hardware> =>
      sendJSON('GET', `/api/v1/machines/${encodeURIComponent(id)}/hardware`, hardwareSchema),

    forget: async (id: string): Promise<void> => {
      await send('DELETE', `/api/v1/machines/${encodeURIComponent(id)}`)
    },

    resetPreview: (id: string): Promise<ResetPreview> =>
      sendJSON(
        'GET',
        `/api/v1/machines/${encodeURIComponent(id)}/reset-preview`,
        resetPreviewSchema,
      ),

    /**
     * Ask the server to confirm an action.
     *
     * The token that comes back is bound to this action, this machine and
     * these exact parameters. Submitting anything else with it is refused —
     * which is what makes the dialog a gate rather than decoration.
     */
    confirm: (
      id: string,
      action: string,
      params: Record<string, string>,
      typed: string,
    ): Promise<{ token: string; expires: string }> =>
      sendJSON('POST', `/api/v1/machines/${encodeURIComponent(id)}/confirm`, confirmationSchema, {
        action,
        params,
        typed,
      }),

    /**
     * Take a node out of its cluster for good (UPG-13).
     *
     * Unlike the three above this is not a job: it is synchronous, because
     * every step of it is a call this process makes and waits for — forfeit
     * leadership if it holds it, leave etcd, wait out the eviction gap, then
     * forget the record. What comes back is the notice about cordon and drain,
     * which the screen has to show afterwards as well as before: an operator
     * who read it on the way in has already stopped reading by the time it
     * matters.
     */
    removeFromCluster: (
      id: string,
      cluster: string,
      confirmation: string,
    ): Promise<{ machine: string; notice: string }> =>
      sendJSON(
        'POST',
        `/api/v1/machines/${encodeURIComponent(id)}/remove-from-cluster`,
        z.object({ machine: z.string(), notice: z.string() }),
        { cluster, confirmation },
      ),

    /**
     * Every node action answers 202 with a job id. Nothing has happened yet
     * when this resolves; the job is where it happens.
     */
    action: (
      id: string,
      kind: 'reboot' | 'shutdown' | 'reset',
      confirmation: string,
      params: Record<string, string> = {},
      cluster = '',
    ): Promise<AcceptedJob> =>
      sendJSON('POST', `/api/v1/machines/${encodeURIComponent(id)}/${kind}`, acceptedJobSchema, {
        confirmation,
        params,
        cluster,
      }),
  },

  config: {
    get: (machine: string): Promise<ConfigView> =>
      sendJSON('GET', `/api/v1/machines/${encodeURIComponent(machine)}/config`, configViewSchema),

    /** Reads the node, merges locally, and answers with the diff, the apply
     * mode and the validation. It changes nothing. */
    plan: (machine: string, patches: string[], patchIDs: string[] = []): Promise<ConfigPreview> =>
      sendJSON(
        'POST',
        `/api/v1/machines/${encodeURIComponent(machine)}/config/plan`,
        previewSchema,
        { patches, patch_ids: patchIDs, cluster: '', mode: '' },
      ),

    apply: (
      machine: string,
      patches: string[],
      mode: string,
      cluster: string,
      patchIDs: string[] = [],
    ): Promise<{ mode: string; details: string }> =>
      sendJSON(
        'POST',
        `/api/v1/machines/${encodeURIComponent(machine)}/config/apply`,
        applyResultSchema,
        { patches, patch_ids: patchIDs, mode, cluster },
      ),
  },

  machineLock: {
    /** A lock is holzkube-manager's own note about what it should not do, so it
     * works on a node that is down — which is precisely when somebody wants to
     * set one. The reason is required: a lock nobody can explain is a lock the
     * next person clears because it is in the way. */
    set: (machine: string, locked: boolean, reason: string): Promise<Machine> =>
      sendJSON('POST', `/api/v1/machines/${encodeURIComponent(machine)}/lock`, machineSchema, {
        locked,
        reason,
      }),
  },

  patches: {
    list: async (): Promise<Patch[]> =>
      (await sendJSON('GET', '/api/v1/patches', patchesSchema)).patches,

    get: (id: string): Promise<Patch> =>
      sendJSON('GET', `/api/v1/patches/${encodeURIComponent(id)}`, patchSchema),

    /** Creating with a `parent` is an edit: it writes a new version and marks
     * the old one superseded rather than rewriting it. */
    create: (input: {
      name: string
      body: string
      cluster?: string
      description?: string
      parent?: string
    }): Promise<Patch> =>
      sendJSON('POST', '/api/v1/patches', patchSchema, {
        name: input.name,
        body: input.body,
        cluster: input.cluster ?? '',
        description: input.description ?? '',
        parent: input.parent ?? '',
      }),
  },

  provision: {
    /** The sentences the wizard opens with. They describe how a machine boots
     * rather than anything this product does, so they come from the server:
     * a copy in this bundle is a copy that drifts from the behaviour it
     * describes. */
    notices: async (): Promise<string[]> =>
      (await sendJSON('GET', '/api/v1/provision/notices', noticesSchema)).notices,

    /** A POST because it carries a subnet and a list of addresses, not because
     * it mutates. Nothing on any machine changes. */
    scan: (cidr: string, addrs: string[] = []) =>
      sendJSON('POST', '/api/v1/provision/scan', scanSchema, { cidr, addrs }),

    /** The provisioning path's own confirmation. It is a separate route and
     * not the machine-scoped one because a machine being provisioned is not in
     * the inventory: there is no hostname to type, and what is typed is the
     * disk that gets written. */
    confirm: (req: ProvisionRequest, typed: string): Promise<{ token: string; expires: string }> =>
      sendJSON('POST', '/api/v1/provision/confirm', confirmationSchema, { ...req, typed }),

    inspect: (addr: string, fingerprint = ''): Promise<Candidate> =>
      sendJSON('POST', '/api/v1/provision/inspect', candidateSchema, { addr, fingerprint }),

    /** The last screen before the apply. It validates, warns, and says exactly
     * what would be written — and it writes nothing. */
    plan: (req: ProvisionRequest): Promise<ProvisionPreview> =>
      sendJSON('POST', '/api/v1/provision/plan', previewProvisionSchema, req),

    apply: (req: ProvisionRequest, confirmation: string): Promise<AcceptedJob> =>
      sendJSON('POST', '/api/v1/provision/apply', acceptedJobSchema, { ...req, confirmation }),

    recovery: () =>
      sendJSON('GET', '/api/v1/provision/bootstrap-recovery', bootstrapRecoverySchema),

    /** `bootstrapped` is what a person found by looking. There is no default:
     * guessing here is the operation the whole bootstrap record exists to
     * prevent. */
    resolve: (cluster: string, bootstrapped: boolean, note: string) =>
      sendJSON(
        'POST',
        `/api/v1/provision/bootstrap-recovery/${encodeURIComponent(cluster)}`,
        z.object({ cluster: z.string(), bootstrapped: z.boolean() }),
        { bootstrapped, note },
      ),
  },

  upgrades: {
    /** No "latest", and no pre-releases. Both absences are the server's, and
     * the notice it returns says why. */
    releases: () => sendJSON('GET', '/api/v1/upgrade/releases', releasesSchema),

    plan: (cluster: string, to: string): Promise<UpgradePlan> =>
      sendJSON(
        'POST',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/upgrade/plan`,
        upgradePlanSchema,
        { to },
      ),

    planKubernetes: (cluster: string, to: string): Promise<UpgradePlan> =>
      sendJSON(
        'POST',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/upgrade/kubernetes/plan`,
        upgradePlanSchema,
        { to },
      ),

    /** What is typed is the cluster's name. A rolling upgrade is not about one
     * machine, so there is no hostname to type — what is at risk is the
     * cluster. */
    confirm: (
      cluster: string,
      kind: string,
      to: string,
      typed: string,
    ): Promise<{ token: string; expires: string }> =>
      sendJSON(
        'POST',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/upgrade/confirm`,
        confirmationSchema,
        { kind, to, typed },
      ),

    start: (cluster: string, to: string, confirmation: string, kubernetes = false) =>
      sendJSON(
        'POST',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/upgrade${kubernetes ? '/kubernetes' : ''}`,
        acceptedJobSchema,
        { to, confirmation },
      ),
  },

  etcd: {
    members: (cluster: string): Promise<EtcdMemberList> =>
      sendJSON(
        'GET',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/etcd/members`,
        etcdMemberListSchema,
      ),

    removeMember: async (cluster: string, id: string): Promise<void> => {
      await sendJSON(
        'DELETE',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/etcd/members/${encodeURIComponent(id)}`,
        z.unknown(),
      )
    },

    /** The snapshot is bytes, not JSON. It is a link rather than a fetch: the
     * browser streams it to disk instead of holding a database in memory. */
    snapshotURL: (cluster: string) =>
      `/api/v1/clusters/${encodeURIComponent(cluster)}/etcd/snapshot`,

    /**
     * Restore one control-plane node's etcd from a snapshot.
     *
     * The file is the request body rather than a field in a JSON envelope, for
     * the reason the route's own comment gives: base64 inside JSON costs a
     * third of a database's size on the wire and all of it in memory. The two
     * small values that go with it ride in the query string instead.
     *
     * `machine` is repeated in the query deliberately. It is the typed
     * confirmation, and unlike every other one in this product it is not a
     * word but the node's own id: the question is not "did you mean to do
     * this" but "did you mean to do it here", because a restore aimed at the
     * wrong control-plane node makes that node's data the cluster's.
     */
    restore: (machine: string, snapshot: Blob, skipHashCheck: boolean): Promise<EtcdRestored> => {
      const query = new URLSearchParams({ machine })
      if (skipHashCheck) {
        query.set('skip_hash_check', 'true')
      }
      return sendBody(
        'POST',
        `/api/v1/machines/${encodeURIComponent(machine)}/etcd/restore?${query}`,
        etcdRestoredSchema,
        snapshot,
      )
    },
  },

  users: {
    list: async (): Promise<User[]> => (await sendJSON('GET', '/api/v1/users', usersSchema)).users,

    create: (username: string, password: string, role: UserRoleName): Promise<User> =>
      sendJSON('POST', '/api/v1/users', userSchema, { username, password, role }),

    setRole: (id: string, role: UserRoleName): Promise<User> =>
      sendJSON('POST', `/api/v1/users/${encodeURIComponent(id)}/role`, userSchema, { role }),

    resetPassword: async (id: string, password: string): Promise<void> => {
      await sendJSON('POST', `/api/v1/users/${encodeURIComponent(id)}/password`, z.unknown(), {
        password,
      })
    },

    remove: async (id: string): Promise<void> => {
      await sendJSON('DELETE', `/api/v1/users/${encodeURIComponent(id)}`, z.unknown())
    },
  },

  serviceAccounts: {
    /**
     * Mints a machine identity and its one token.
     *
     * The token is in this response and in nothing else, ever: only its hash
     * is stored. There is deliberately no `get` here, because there is no
     * route that could answer one.
     */
    create: (username: string, role: UserRoleName): Promise<ServiceAccountCreated> =>
      sendJSON('POST', '/api/v1/service-accounts', serviceAccountCreatedSchema, {
        username,
        role,
      }),

    /** Rotating is the only revocation this product has: the old token stops
     * working at the moment the new one is minted. */
    rotate: (id: string): Promise<ServiceAccountToken> =>
      sendJSON(
        'POST',
        `/api/v1/service-accounts/${encodeURIComponent(id)}/token`,
        serviceAccountTokenSchema,
      ),
  },

  /**
   * The links a screen in a corridor is left open on.
   *
   * Administrator only in both directions: creating one hands out a credential
   * that will sit on a television for months, and revoking one turns a screen
   * off from across the building.
   */
  wallLinks: {
    list: () => sendJSON('GET', '/api/v1/wall-links', wallLinksSchema),
    create: (label: string) =>
      sendJSON('POST', '/api/v1/wall-links', createdWallLinkSchema, { label }),
    revoke: (id: string) =>
      send('DELETE', `/api/v1/wall-links/${encodeURIComponent(id)}`).then(() => undefined),
  },

  kubernetes: {
    /**
     * What one cluster's own API server says about it.
     *
     * A namespace filters the pods, and the filter goes to the API SERVER: a
     * cluster with ten thousand pods must not send all of them so that three
     * can be shown.
     *
     * An API server that does not answer comes back as `upstream.*` and not as
     * an empty list, because an empty list is a claim that nothing is running.
     */
    /**
     * Stop or resume scheduling onto one node.
     *
     * The state is sent rather than a verb, so a retry is the same request: a
     * client that lost the response can send it again without wondering
     * whether it toggled.
     */
    cordon: (cluster: string, node: string, unschedulable: boolean): Promise<KubernetesNode> =>
      sendJSON(
        'POST',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/nodes/${encodeURIComponent(node)}/cordon`,
        kubernetesNodeSchema,
        { unschedulable },
      ),

    /**
     * Drain a node: cordon it, then evict what it carries.
     *
     * It comes back as a job, because a drain waits out each pod's termination
     * grace period and can outlast any response the server holds open — and
     * because an interrupted drain leaves a cordoned node with some pods
     * moved, which is a state somebody has to finish rather than guess about.
     *
     * The two flags are the decisions the drain refuses to take on its own: a
     * pod nothing owns is gone for good if it is evicted, and a pod with local
     * storage loses that storage when it moves.
     */
    drain: (
      cluster: string,
      node: string,
      opts: { force: boolean; delete_local_data: boolean },
    ): Promise<AcceptedJob> =>
      sendJSON(
        'POST',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/nodes/${encodeURIComponent(node)}/drain`,
        acceptedJobSchema,
        opts,
      ),

    /**
     * Restart one pod: delete it and let its controller make another.
     *
     * Refused when nothing owns the pod, because then deleting it is not a
     * restart — it is deletion, and the word would have been a lie.
     */
    restartPod: async (cluster: string, namespace: string, pod: string): Promise<void> => {
      await send(
        'POST',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/pods/${encodeURIComponent(namespace)}/${encodeURIComponent(pod)}/restart`,
      )
    },

    /** Set a deployment's replica count. Zero is a real answer: it is how a
     * workload is switched off. */
    scale: async (
      cluster: string,
      namespace: string,
      deployment: string,
      replicas: number,
    ): Promise<void> => {
      await send(
        'POST',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/deployments/${encodeURIComponent(namespace)}/${encodeURIComponent(deployment)}/scale`,
        { replicas },
      )
    },

    /**
     * Replace a deployment's pods under the deployment's own strategy.
     *
     * The difference from restarting the pods one by one: the deployment
     * controller does the replacing, with its surge, its maxUnavailable and its
     * readiness probes — so the workload stays up while it happens.
     */
    rolloutRestart: async (
      cluster: string,
      namespace: string,
      deployment: string,
    ): Promise<void> => {
      await send(
        'POST',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/deployments/${encodeURIComponent(namespace)}/${encodeURIComponent(deployment)}/restart`,
      )
    },

    /**
     * What applying a manifest would do. It changes nothing.
     *
     * Each object's action is decided by asking the cluster whether that object
     * exists — `create` and `update` cannot be read off a manifest, which looks
     * identical either way.
     */
    planManifest: (cluster: string, manifest: string): Promise<ManifestPlan> =>
      sendJSON(
        'POST',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/manifest/plan`,
        manifestPlanSchema,
        { manifest },
      ),

    /**
     * Apply a manifest with server-side apply, under this product's own field
     * manager.
     *
     * A conflict means another field manager — usually a controller — owns a
     * field this apply sets. It comes back as something to read rather than
     * being forced: forcing takes the value away from whatever is managing it,
     * and that thing will set it back.
     */
    applyManifest: (cluster: string, manifest: string): Promise<ManifestApplyResult> =>
      sendJSON(
        'POST',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/manifest/apply`,
        manifestApplyResultSchema,
        { manifest },
      ),

    /**
     * Fetch one path from a service in the cluster.
     *
     * A POST for a read, and the reason is the record rather than REST taste:
     * the audit archive captures request bodies and not query strings, and on
     * this route the port and the path ARE the event — "somebody read /healthz"
     * and "somebody read /admin/users" are not the same entry. A path in a query
     * string would also sit in the browser's history and the daemon's access log
     * for no benefit.
     *
     * What comes back is text, never rendered: the workload's own content type
     * is deliberately not carried, because serving a pod's markup from this
     * origin would let any pod script this interface.
     */
    proxyService: (
      cluster: string,
      namespace: string,
      service: string,
      port: string,
      path: string,
    ): Promise<ProxyResponse> =>
      sendJSON(
        'POST',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/services/${encodeURIComponent(namespace)}/${encodeURIComponent(service)}/proxy`,
        proxyResponseSchema,
        { port, path },
      ),

    /**
     * Who this product acts as against the cluster, and what that identity may
     * do — asked of the CLUSTER, which is the only honest source: RBAC is its
     * own arrangement of roles and bindings.
     *
     * `as` previews a candidate without storing it, so nobody finds out by
     * saving it and watching every screen break.
     */
    identity: (cluster: string, opts?: { as?: string; namespace?: string }) => {
      const query = new URLSearchParams()
      if (opts?.as) query.set('as', opts.as)
      if (opts?.namespace) query.set('namespace', opts.namespace)
      const suffix = query.toString() === '' ? '' : `?${query.toString()}`
      return sendJSON(
        'GET',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/identity${suffix}`,
        kubeIdentitySchema,
      )
    },

    /** Store whose name this cluster's requests arrive under. An empty user
     * goes back to this product's own certificate. */
    setIdentity: async (cluster: string, user: string, groups: string[] = []): Promise<void> => {
      await send('POST', `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/identity`, {
        user,
        groups,
      })
    },

    /** Everything that runs, across all five kinds. A cluster is not only its
     * Deployments: the storage layer is a DaemonSet, the backup a CronJob. */
    workloads: async (cluster: string, namespace?: string) =>
      (
        await sendJSON(
          'GET',
          `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/workloads` +
            (namespace === undefined || namespace === ''
              ? ''
              : `?namespace=${encodeURIComponent(namespace)}`),
          workloadsSchema,
        )
      ).workloads,

    scaleWorkload: async (
      cluster: string,
      kind: string,
      namespace: string,
      name: string,
      replicas: number,
    ): Promise<void> => {
      await send(
        'POST',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/workloads/${encodeURIComponent(kind)}/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/scale`,
        { replicas },
      )
    },

    restartWorkload: async (
      cluster: string,
      kind: string,
      namespace: string,
      name: string,
    ): Promise<void> => {
      await send(
        'POST',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/workloads/${encodeURIComponent(kind)}/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/restart`,
      )
    },

    /** The objects beside the workloads: ConfigMaps, Secrets (names only),
     * claims, Ingresses, autoscalers and disruption budgets. */
    resources: async (cluster: string, namespace?: string) =>
      (
        await sendJSON(
          'GET',
          `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/resources` +
            (namespace === undefined || namespace === ''
              ? ''
              : `?namespace=${encodeURIComponent(namespace)}`),
          clusterResourcesSchema,
        )
      ).resources,

    /** Remove one object. A Namespace is refused: deleting one removes
     * everything inside it and nothing stops it once it starts. */
    deleteObject: async (
      cluster: string,
      object: { api_version: string; kind: string; namespace?: string; name: string },
    ): Promise<void> => {
      await send('POST', `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/delete`, {
        namespace: '',
        ...object,
      })
    },

    /** One node's conditions, taints and how much room the scheduler has left
     * — the fields that say why nothing will go there. */
    nodeDetail: (cluster: string, node: string) =>
      sendJSON(
        'GET',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/nodes/${encodeURIComponent(node)}`,
        nodeDetailSchema,
      ),

    /** Every app with what it uses now, optionally narrowed to one namespace
     * or to the pods on one node. */
    apps: (cluster: string, filter: { namespace?: string; node?: string } = {}) => {
      const params = new URLSearchParams()
      if (filter.namespace) params.set('namespace', filter.namespace)
      if (filter.node) params.set('node', filter.node)
      const query = params.toString()
      return sendJSON(
        'GET',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/apps${query ? `?${query}` : ''}`,
        appsSchema,
      )
    },

    /** One app: its pods and containers with their usage, the services that
     * reach it, and what happened to it recently. */
    app: (cluster: string, namespace: string, kind: string, name: string) =>
      sendJSON(
        'GET',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/apps/${encodeURIComponent(namespace)}/${encodeURIComponent(kind)}/${encodeURIComponent(name)}`,
        appDetailSchema,
      ),

    /** What nodes and pods are USING — a different number from what they
     * requested, and absent unless a metrics-server is installed. */
    usage: (cluster: string, namespace?: string) =>
      sendJSON(
        'GET',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/usage` +
          (namespace === undefined || namespace === ''
            ? ''
            : `?namespace=${encodeURIComponent(namespace)}`),
        clusterUsageSchema,
      ),

    /**
     * Run one command in a container.
     *
     * A LIST, not a string: a string would have to be split by something, and
     * whatever split it would be a shell. `sh -c "…"` is refused by the server,
     * because an archive holding "sh -lc" plus one opaque argument cannot say
     * what happened. It also refuses unless this cluster acts as a person.
     */
    exec: (
      cluster: string,
      namespace: string,
      pod: string,
      container: string,
      command: string[],
    ): Promise<ExecResult> =>
      sendJSON(
        'POST',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/pods/${encodeURIComponent(namespace)}/${encodeURIComponent(pod)}/exec`,
        execResultSchema,
        { container, command },
      ),

    /**
     * Everything one screen in the IT office shows, in one answer.
     *
     * `link` is the kiosk credential, sent as a bearer token. It goes in a
     * HEADER and never in the query: a URL is written to the server's log, to
     * the browser's history and to whatever proxy sits between, and a
     * credential in all three places is a credential that outlives its screen.
     * It lives in the page's own address, which is the bookmark, and travels
     * from there to this header and nowhere else.
     */
    wall: (cluster: string, namespace?: string, link?: string) =>
      sendJSON(
        'GET',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/wall` +
          (namespace === undefined || namespace === ''
            ? ''
            : `?namespace=${encodeURIComponent(namespace)}`),
        wallSchema,
        undefined,
        link === undefined || link === '' ? undefined : { bearer: link },
      ),

    /** Every namespace, its quotas, and the cluster's own kinds. No namespace
     * parameter: the answer IS the namespaces. */
    inventory: (cluster: string) =>
      sendJSON(
        'GET',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/inventory`,
        inventorySchema,
      ),

    /** Who may do what, and every binding that grants nothing. */
    access: (cluster: string, namespace?: string) =>
      sendJSON(
        'GET',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/access` +
          (namespace === undefined || namespace === ''
            ? ''
            : `?namespace=${encodeURIComponent(namespace)}`),
        accessControlSchema,
      ),

    /** The cluster's volumes, claims and classes. The namespace narrows the
     * CLAIMS only: volumes and classes are cluster-scoped, and hiding them in a
     * namespace view is how a Released volume holding a database stays
     * invisible. */
    storage: (cluster: string, namespace?: string) =>
      sendJSON(
        'GET',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/storage` +
          (namespace === undefined || namespace === ''
            ? ''
            : `?namespace=${encodeURIComponent(namespace)}`),
        clusterStorageSchema,
      ),

    /** The services, what is actually behind them, and what may reach them. */
    network: (cluster: string, namespace?: string) =>
      sendJSON(
        'GET',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/network` +
          (namespace === undefined || namespace === ''
            ? ''
            : `?namespace=${encodeURIComponent(namespace)}`),
        clusterNetworkSchema,
      ),

    /** How full the cluster and each node is — allocatable against what pods
     * REQUESTED. This exists on every cluster, unlike usage. */
    capacity: (cluster: string) =>
      sendJSON(
        'GET',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/capacity`,
        clusterCapacitySchema,
      ),

    /** Stop a workload: scale it to zero, remembering the count. */
    stopWorkload: (cluster: string, kind: string, namespace: string, name: string) =>
      sendJSON(
        'POST',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/workloads/${encodeURIComponent(kind)}/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/stop`,
        stoppedStateSchema,
      ),

    /** Start it again, at the count it was running. */
    startWorkload: (cluster: string, kind: string, namespace: string, name: string) =>
      sendJSON(
        'POST',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/workloads/${encodeURIComponent(kind)}/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/start`,
        stoppedStateSchema,
      ),

    /** What clearing out would remove. It changes nothing. */
    sweepPlan: (cluster: string, namespace?: string) =>
      sendJSON(
        'GET',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/sweep` +
          (namespace === undefined || namespace === ''
            ? ''
            : `?namespace=${encodeURIComponent(namespace)}`),
        sweepPlanSchema,
      ),

    /** Remove exactly what the plan listed — passed back rather than
     * recomputed, so nothing is removed that was not in the list shown. */
    sweep: (cluster: string, items: Sweepable[]) =>
      sendJSON(
        'POST',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/sweep`,
        z.object({
          removed: z.number(),
          failed: z
            .array(z.object({ reason: z.string() }))
            .nullish()
            .transform(orEmpty),
        }),
        { items },
      ),

    /** Which containers a pod has, and how each of them is doing. */
    containers: async (cluster: string, namespace: string, pod: string) =>
      (
        await sendJSON(
          'GET',
          `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/pods/${encodeURIComponent(namespace)}/${encodeURIComponent(pod)}/containers`,
          containersSchema,
        )
      ).containers,

    /**
     * One container's output.
     *
     * `previous` is the one that matters: a pod in CrashLoopBackOff has printed
     * nothing in its current container, and everything that explains the crash
     * belongs to the run that already ended.
     */
    logs: (
      cluster: string,
      namespace: string,
      pod: string,
      opts: { container: string; previous?: boolean; tail?: number },
    ): Promise<PodLog> => {
      const query = new URLSearchParams({ container: opts.container })
      if (opts.previous) query.set('previous', 'true')
      if (opts.tail !== undefined) query.set('tail', String(opts.tail))
      return sendJSON(
        'GET',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/pods/${encodeURIComponent(namespace)}/${encodeURIComponent(pod)}/log?${query.toString()}`,
        podLogSchema,
      )
    },

    /**
     * One object as YAML — the answer to the question the lists do not show.
     *
     * A POST for a read, because the audit archive captures bodies and not
     * query strings, and nothing else here names the object. A Secret is
     * refused: its data is base64, not encryption.
     */
    object: (
      cluster: string,
      object: { api_version: string; kind: string; namespace?: string; name: string },
    ): Promise<Described> =>
      sendJSON(
        'POST',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/object`,
        describedSchema,
        { namespace: '', ...object },
      ),

    /** What the cluster has reported recently, which is where "why is this pod
     * Pending" is answered and nowhere else. */
    events: (cluster: string, namespace?: string) =>
      sendJSON(
        'GET',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/events` +
          (namespace === undefined || namespace === ''
            ? ''
            : `?namespace=${encodeURIComponent(namespace)}`),
        clusterEventsSchema,
      ),

    /** The same, about one pod, filtered by the server. */
    podEvents: (cluster: string, namespace: string, pod: string) =>
      sendJSON(
        'GET',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes/pods/${encodeURIComponent(namespace)}/${encodeURIComponent(pod)}/events`,
        clusterEventsSchema,
      ),

    overview: (cluster: string, namespace?: string): Promise<KubernetesOverview> =>
      sendJSON(
        'GET',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/kubernetes` +
          (namespace === undefined || namespace === ''
            ? ''
            : `?namespace=${encodeURIComponent(namespace)}`),
        kubernetesOverviewSchema,
      ),
  },

  authority: {
    /**
     * What a rotation would do, and what it cannot promise.
     *
     * Read before anything is offered, because the dialog IS the operation's
     * safety: this is the one thing in the product that can leave a cluster
     * trusting nobody, and the four passes and the warnings are the part an
     * operator has to have read.
     */
    preview: (cluster: string): Promise<AuthorityPreview> =>
      sendJSON(
        'GET',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/authority`,
        authorityPreviewSchema,
      ),

    /**
     * Type the cluster's name to get a token.
     *
     * The token is bound to this cluster and this action: a confirmation for
     * rebooting a node, or for another cluster, does not authorise it. The
     * server re-derives what it authorises from the request rather than
     * reading it out of the token.
     */
    confirm: (cluster: string, typed: string): Promise<{ token: string; expires: string }> =>
      sendJSON(
        'POST',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/authority/confirm`,
        authorityConfirmationSchema,
        { typed },
      ),

    /** Start the rotation. It comes back as a job, because it is four passes
     * over every node and the browser is not what carries them. */
    rotate: (cluster: string, confirmation: string): Promise<AcceptedJob> =>
      sendJSON(
        'POST',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/authority`,
        acceptedJobSchema,
        { confirmation },
      ),
  },

  certificate: {
    /**
     * Issues this installation a fresh admin certificate for one cluster.
     *
     * It touches no node. The certificate is minted from the cluster's own
     * Talos certificate authority, which this installation holds, and a node
     * trusts the authority rather than any particular certificate issued from
     * it — so a fresh one works the moment it is presented.
     *
     * The server proves it against a node before keeping it, and answers
     * `conflict.certificate-rejected` when it could not. That is the *safe*
     * outcome and not a broken cluster: nothing changed.
     */
    renew: (cluster: string): Promise<Cluster> =>
      sendJSON(
        'POST',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/client-certificate`,
        clusterSchema,
      ),
  },

  scale: {
    /**
     * What changing this cluster's size would mean.
     *
     * A read, and there is deliberately no companion that scales anything:
     * removing a node is that node's own action and adding one is
     * provisioning. What was missing was never a button.
     */
    plan: (cluster: string): Promise<z.infer<typeof scalePlanResponseSchema>> =>
      sendJSON(
        'GET',
        `/api/v1/clusters/${encodeURIComponent(cluster)}/scale`,
        scalePlanResponseSchema,
      ),
  },

  clusterTemplates: {
    /**
     * What a template would mean for the fleet as it is now.
     *
     * The body is the YAML itself rather than a JSON envelope: the document is
     * one an operator wrote in an editor and keeps in a repository, and
     * escaping every newline to post it produces a file nobody can read in a
     * request log.
     *
     * There is deliberately no `apply` here, because there is no route that
     * applies one.
     */
    plan: (yaml: string): Promise<z.infer<typeof templatePlanResponseSchema>> =>
      sendYAML('POST', '/api/v1/cluster-templates/plan', templatePlanResponseSchema, yaml),

    /** Where the browser downloads a cluster written down as a template. */
    exportPath: (id: string): string => `/api/v1/clusters/${encodeURIComponent(id)}/template`,
  },

  labels: {
    /**
     * Replaces a machine's labels. Replaces, and not merges: a merge cannot
     * remove anything, so an operator who deleted a row from a form would find
     * it still there afterwards.
     */
    set: (id: string, labels: Record<string, string>): Promise<Machine> =>
      sendJSON('PUT', `/api/v1/machines/${encodeURIComponent(id)}/labels`, machineSchema, {
        labels,
      }),
  },

  machineClasses: {
    list: async (): Promise<MachineClass[]> =>
      (await sendJSON('GET', '/api/v1/machine-classes', machineClassesSchema)).classes,

    put: (
      id: string,
      body: { name: string; description: string; selector: LabelSelector },
    ): Promise<MachineClass> =>
      sendJSON(
        'PUT',
        `/api/v1/machine-classes/${encodeURIComponent(id)}`,
        machineClassSchema,
        body,
      ),

    remove: async (id: string): Promise<void> => {
      await sendJSON('DELETE', `/api/v1/machine-classes/${encodeURIComponent(id)}`, z.unknown())
    },
  },

  jobs: {
    list: async (): Promise<Job[]> => (await sendJSON('GET', '/api/v1/jobs', jobsSchema)).jobs,

    get: (id: string): Promise<Job> =>
      sendJSON('GET', `/api/v1/jobs/${encodeURIComponent(id)}`, jobSchema),

    /**
     * Stop at the next step boundary. Deliberately not a destructive route:
     * an operator watching something go wrong should not have to find their
     * password before they can stop it.
     */
    cancel: (id: string): Promise<Job> =>
      sendJSON('POST', `/api/v1/jobs/${encodeURIComponent(id)}/cancel`, jobSchema),
  },
}
