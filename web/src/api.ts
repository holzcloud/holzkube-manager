import { z } from 'zod'

/**
 * The single place in the frontend that calls fetch.
 *
 * Everything the CSRF contract requires is applied here, once: a JSON
 * content type, the custom header, and same-origin credentials. Spreading
 * fetch calls across components is how one of them eventually forgets.
 */

const CSRF_HEADER = 'X-Holzkube-CSRF'
const PROBLEM_CONTENT_TYPE = 'application/problem+json'

export const problemSchema = z.object({
  type: z.string(),
  title: z.string(),
  status: z.number(),
  detail: z.string().optional(),
  instance: z.string().optional(),
  code: z.string(),
  errors: z.array(z.object({ field: z.string(), reason: z.string() })).optional(),
})

export type Problem = z.infer<typeof problemSchema>

/** A typed error carrying the decoded problem+json body. */
export class ProblemError extends Error {
  readonly problem: Problem

  constructor(problem: Problem) {
    super(problem.detail && problem.detail.length > 0 ? problem.detail : problem.title)
    this.name = 'ProblemError'
    this.problem = problem
  }

  get code(): string {
    return this.problem.code
  }
}

export const auditChainSchema = z.object({
  ok: z.boolean(),
  broken_at_line: z.number(),
  file: z.string(),
})

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

export type AuditPage = z.infer<typeof auditPageSchema>

export const meSchema = z.object({
  id: z.string(),
  username: z.string(),
})

export type Me = z.infer<typeof meSchema>

async function request(method: string, path: string, body?: unknown): Promise<Response> {
  const headers: Record<string, string> = {
    Accept: 'application/json',
  }
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
  }
  headers[CSRF_HEADER] = '1'

  const response = await fetch(path, {
    method,
    headers,
    credentials: 'same-origin',
    body: body === undefined ? undefined : JSON.stringify(body),
  })

  if (!response.ok) {
    throw await toProblemError(response)
  }
  return response
}

async function toProblemError(response: Response): Promise<Error> {
  const contentType = response.headers.get('Content-Type') ?? ''
  if (contentType.includes(PROBLEM_CONTENT_TYPE)) {
    const parsed = problemSchema.safeParse(await response.json().catch(() => null))
    if (parsed.success) {
      return new ProblemError(parsed.data)
    }
  }
  return new Error(`Request failed with status ${response.status}`)
}

async function requestJSON<T>(
  method: string,
  path: string,
  schema: z.ZodType<T>,
  body?: unknown,
): Promise<T> {
  const response = await request(method, path, body)
  return schema.parse(await response.json())
}

export const api = {
  status: (): Promise<SystemStatus> =>
    requestJSON('GET', '/api/v1/system/status', systemStatusSchema),

  setup: async (username: string, password: string): Promise<void> => {
    await request('POST', '/api/v1/setup', { username, password })
  },

  login: async (username: string, password: string): Promise<void> => {
    await request('POST', '/api/v1/auth/login', { username, password })
  },

  logout: async (): Promise<void> => {
    await request('POST', '/api/v1/auth/logout', {})
  },

  me: (): Promise<Me> => requestJSON('GET', '/api/v1/auth/me', meSchema),

  audit: (): Promise<AuditPage> => requestJSON('GET', '/api/v1/audit', auditPageSchema),
}
