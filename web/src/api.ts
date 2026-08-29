import { z } from 'zod'
import { type Problem, presentationFor, toProblemError } from '@/lib/problem'

/**
 * The single place in the frontend that calls fetch.
 *
 * Everything the CSRF contract requires is applied here, once, on every
 * mutating request: `Content-Type: application/json`, `X-Holzkube-CSRF: 1`
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

const CSRF_HEADER = 'X-Holzkube-CSRF'
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
})

export type Me = z.infer<typeof meSchema>

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
   */
  usable: z.boolean(),
  /**
   * The zero time means never probed, which is not the same as probed and
   * refused. The two must not be merged in the UI.
   */
  probed_at: z.string(),
  /**
   * What the Factory said when it refused, empty otherwise -- including when
   * the probe could not reach it at all, which says nothing about the
   * schematic. A red badge with no stated cause is a verdict an operator
   * cannot act on.
   */
  probe_reason: z.string(),
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
   */
  installer: z.string(),
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

/** The warning codes the server emits, as constants a component can key on. */
export const WARNING_INSTALLER_IGNORES_KERNEL_ARGS = 'schematic.installer-ignores-kernel-args'
export const WARNING_INSTALLER_IGNORES_META = 'schematic.installer-ignores-meta'

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

  let response = await fetch(path, init)
  if (response.ok) {
    return response
  }

  let error = await toProblemError(response)

  if (interceptSudo && presentationFor(error.problem) === 'sudo-prompt') {
    const granted = await askForSudo(path)
    if (!granted) {
      throw error
    }
    response = await fetch(path, init)
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
 * server: holzkube is developed on arm64 and targets amd64, so a defaulted
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
}
