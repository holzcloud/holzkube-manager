import { z } from 'zod'

/**
 * RFC 9457 `application/problem+json`, decoded and turned into a decision about
 * how the interface should show it (FOUND-11).
 *
 * The UI branches on `code`, never on the status alone. `code` is the part
 * `docs/api-contract.md` declares stable and closed; the status mapping behind
 * it may be refined later, and a UI that read the status would break when it
 * is. Two different codes already share 403 today.
 *
 * The other rule this module exists to enforce: a bare status number never
 * reaches the operator. Every path out of here produces a sentence.
 */

export const PROBLEM_CONTENT_TYPE = 'application/problem+json'

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
  /**
   * Seconds the server asked us to wait, from `Retry-After`. Only ever set on
   * a `ratelimit.*` answer, and only when the header was present.
   */
  readonly retryAfterSeconds: number | null

  constructor(problem: Problem, retryAfterSeconds: number | null = null) {
    super(problem.detail && problem.detail.length > 0 ? problem.detail : problem.title)
    this.name = 'ProblemError'
    this.problem = problem
    this.retryAfterSeconds = retryAfterSeconds
  }

  get code(): string {
    return this.problem.code
  }

  get status(): number {
    return this.problem.status
  }
}

/**
 * How the interface should present a problem.
 *
 * - `field-errors`  the form marks the offending fields and stays open
 * - `login-transition`  the session is gone; go to /login and say why
 * - `setup-transition`  no operator account exists yet; go to /setup (D-01)
 * - `sudo-prompt`  ask for the password again and retry the action (D-05)
 * - `wait`  a delay is in effect; show the remaining time, never a lockout
 * - `toast`  everything else, as one readable sentence
 */
export type Presentation =
  | 'field-errors'
  | 'login-transition'
  | 'setup-transition'
  | 'sudo-prompt'
  | 'wait'
  | 'toast'

/**
 * The full closed taxonomy from `docs/api-contract.md` § Error Taxonomy, one
 * entry per code prefix. Longer keys are matched first, which is what lets
 * `setup.required` (go to the wizard) and `setup.already-completed` (say so and
 * stay put) share a prefix without sharing a presentation.
 */
const PRESENTATION_BY_CODE: ReadonlyArray<readonly [string, Presentation]> = [
  ['validation.', 'field-errors'],
  ['auth.', 'login-transition'],
  ['csrf.', 'toast'],
  ['forbidden.', 'toast'],
  ['notfound.', 'toast'],
  ['method.', 'toast'],
  ['store.', 'toast'],
  ['setup.required', 'setup-transition'],
  ['setup.', 'toast'],
  ['media.', 'toast'],
  ['sudo.', 'sudo-prompt'],
  ['ratelimit.', 'wait'],
  ['internal.', 'toast'],
]

/** The presentation for a decoded problem. Anything unrecognised is a toast. */
export function presentationFor(problem: Problem): Presentation {
  for (const [prefix, presentation] of PRESENTATION_BY_CODE) {
    if (problem.code === prefix.replace(/\.$/, '') || problem.code.startsWith(prefix)) {
      return presentation
    }
  }
  return 'toast'
}

/**
 * The sentence to show. `detail` when the server sent one, otherwise `title`,
 * otherwise a plain fallback -- but never a naked status code, and never an
 * empty toast.
 */
export function messageFor(error: unknown): string {
  if (error instanceof ProblemError) {
    const { detail, title } = error.problem
    if (detail !== undefined && detail.trim() !== '') {
      return detail
    }
    if (title.trim() !== '') {
      return title
    }
    return 'holzkube refused the request but did not say why. The server log has the details.'
  }
  if (error instanceof Error && error.message.trim() !== '') {
    return error.message
  }
  return 'Something went wrong.'
}

/** Field path to reason, for a `validation.*` answer. Empty for anything else. */
export function fieldErrorsOf(error: unknown): Record<string, string> {
  if (!(error instanceof ProblemError) || error.problem.errors === undefined) {
    return {}
  }
  const fields: Record<string, string> = {}
  for (const entry of error.problem.errors) {
    fields[entry.field] = entry.reason
  }
  return fields
}

/**
 * How long the operator has to wait, in seconds.
 *
 * Rate limiting is a delay and nothing else: there is no lock, so there is
 * nothing to unlock and no recovery path to offer (D-08). Waiting is always
 * sufficient, and the interface must not suggest otherwise.
 */
export function waitSecondsOf(error: unknown): number | null {
  if (!(error instanceof ProblemError)) {
    return null
  }
  return error.retryAfterSeconds
}

/** Human phrasing for a wait, used by the login form's countdown. */
export function waitMessage(seconds: number | null): string {
  if (seconds === null || seconds <= 0) {
    return 'Too many attempts in a row. Try again in a moment.'
  }
  const unit = seconds === 1 ? 'second' : 'seconds'
  return `Too many attempts in a row. Try again in ${seconds} ${unit}.`
}

/** Parse a `Retry-After` header value in delta-seconds form. */
export function parseRetryAfter(raw: string | null): number | null {
  if (raw === null) {
    return null
  }
  const seconds = Number.parseInt(raw.trim(), 10)
  return Number.isFinite(seconds) && seconds >= 0 ? seconds : null
}

/**
 * Decode a non-ok response into a ProblemError.
 *
 * A response that is not problem+json, or whose body does not parse, still
 * produces a ProblemError with a readable title -- so the "never show a bare
 * status code" rule holds even when the server is not the one answering (a
 * proxy, a truncated response, a restart mid-request).
 */
export async function toProblemError(response: Response): Promise<ProblemError> {
  const retryAfter = parseRetryAfter(response.headers.get('Retry-After'))
  const contentType = response.headers.get('Content-Type') ?? ''

  if (contentType.includes(PROBLEM_CONTENT_TYPE)) {
    const body = await response.json().catch(() => null)
    const parsed = problemSchema.safeParse(body)
    if (parsed.success) {
      return new ProblemError(parsed.data, retryAfter)
    }
  }

  return new ProblemError(
    {
      type: 'about:blank',
      title: fallbackTitle(response.status),
      status: response.status,
      detail: fallbackDetail(response.status),
      code: `internal.unreadable-${response.status}`,
    },
    retryAfter,
  )
}

function fallbackTitle(status: number): string {
  if (status >= 500) {
    return 'holzkube could not complete that request'
  }
  if (status === 0) {
    return 'holzkube did not answer'
  }
  return 'holzkube refused that request'
}

function fallbackDetail(status: number): string {
  return `The server answered ${status} without a readable explanation. The server log carries the reason.`
}
