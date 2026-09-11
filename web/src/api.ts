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
})

export type SystemStatus = z.infer<typeof systemStatusSchema>

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
 * `metal-installer-secureboot`, and it is not one: at the pinned Talos version
 * the two names resolve to two different images, while at the oldest supported
 * version they resolve to the same one. Neither "alias" nor "different image" is
 * true of the pair in general, so the answer is labelled per resolution rather
 * than settled once. An operator who copied this reference earlier is not
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
  /** Called with true once the sudo window is open, false if it was cancelled. */
  settle: (granted: boolean) => void
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

/** Names the pending action for the sudo prompt, in English (D-09). */
const ACTION_LABELS: ReadonlyArray<readonly [string, string]> = [
  ['/api/v1/account/password', 'Change the operator password'],
  ['/api/v1/schematics/', 'Delete this schematic'],
]

function labelFor(path: string): string {
  for (const [prefix, label] of ACTION_LABELS) {
    if (path.startsWith(prefix)) {
      return label
    }
  }
  return 'This destructive action'
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

function buildInit(method: string, body: unknown): RequestInit {
  const headers: Record<string, string> = { Accept: 'application/json' }

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
function fetchWithCeiling(path: string, init: RequestInit): Promise<Response> {
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

async function askForSudo(path: string): Promise<boolean> {
  if (sudoHandler === null) {
    return false
  }
  const handler = sudoHandler
  return new Promise<boolean>((resolve) => {
    handler({ action: labelFor(path), settle: resolve })
  })
}

async function send(
  method: string,
  path: string,
  body?: unknown,
  options: RequestOptions = {},
): Promise<Response> {
  const { interceptUnauthenticated = true, interceptSudo = true } = options

  // Built once and reused for the replay, so the retried request is byte for
  // byte the request that was refused -- same body, same headers, same
  // credentials mode. Rebuilding it would be the bug this design avoids.
  const init = buildInit(method, body)

  let response: Response
  try {
    response = await fetchWithCeiling(path, init)
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
    const granted = await askForSudo(path)
    if (!granted) {
      throw error
    }
    // A fresh ceiling, not the remainder of the first one: the operator has
    // just spent time at the password prompt, and charging that time to the
    // replay's budget would abort a request that had not started.
    try {
      response = await fetchWithCeiling(path, init)
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
  addresses: z.array(z.string()).default([]),
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
  adopted_at: z.string(),

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
})

export type Cluster = z.infer<typeof clusterSchema>

export const clustersSchema = z.object({ clusters: z.array(clusterSchema) })
export const machinesSchema = z.object({ machines: z.array(machineSchema) })

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

export const api = {
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

    forget: async (id: string): Promise<void> => {
      await send('DELETE', `/api/v1/machines/${encodeURIComponent(id)}`)
    },
  },
}
