import { describe, expect, it } from 'vitest'
import {
  fieldErrorsOf,
  messageFor,
  type Presentation,
  type Problem,
  ProblemError,
  parseRetryAfter,
  presentationFor,
  toProblemError,
  waitMessage,
  waitSecondsOf,
} from './problem'

/**
 * The error taxonomy is closed and stable (`docs/api-contract.md` § Error
 * Taxonomy). Every code prefix in it gets a case here, so that adding an entry
 * server-side without deciding how the UI shows it fails a test rather than
 * silently landing in the default bucket.
 */

function problem(code: string, status: number, extra: Partial<Problem> = {}): Problem {
  return {
    type: `https://holzkube.dev/problems/${code.split('.')[0]}`,
    title: 'Something happened',
    status,
    code,
    ...extra,
  }
}

const TAXONOMY: ReadonlyArray<readonly [string, number, Presentation]> = [
  ['validation.failed', 400, 'field-errors'],
  ['auth.unauthenticated', 401, 'login-transition'],
  ['csrf.precondition-unmet', 403, 'toast'],
  ['forbidden.denied', 403, 'toast'],
  ['notfound.resource', 404, 'toast'],
  ['method.not-allowed', 405, 'toast'],
  ['store.conflict', 409, 'toast'],
  ['setup.already-completed', 409, 'toast'],
  ['media.unsupported', 415, 'toast'],
  ['sudo.required', 428, 'sudo-prompt'],
  ['ratelimit.delayed', 429, 'wait'],
  ['internal.unexpected', 500, 'toast'],
  ['setup.required', 503, 'setup-transition'],
]

describe('presentationFor', () => {
  for (const [code, status, expected] of TAXONOMY) {
    it(`maps ${code} to ${expected}`, () => {
      expect(presentationFor(problem(code, status))).toBe(expected)
    })
  }

  it('covers every code prefix the contract lists', () => {
    const prefixes = new Set(TAXONOMY.map(([code]) => code.split('.')[0]))
    expect([...prefixes].sort()).toEqual([
      'auth',
      'csrf',
      'forbidden',
      'internal',
      'media',
      'method',
      'notfound',
      'ratelimit',
      'setup',
      'store',
      'sudo',
      'validation',
    ])
  })

  it('separates the two setup codes despite the shared prefix', () => {
    expect(presentationFor(problem('setup.required', 503))).toBe('setup-transition')
    expect(presentationFor(problem('setup.already-completed', 409))).toBe('toast')
  })

  it('falls back to a toast for a code nobody taught it', () => {
    expect(presentationFor(problem('something.new', 418))).toBe('toast')
  })
})

describe('messageFor', () => {
  it('prefers detail when the server sent one', () => {
    const error = new ProblemError(
      problem('store.conflict', 409, {
        title: 'Request conflicts with the current state',
        detail: 'An operator account already exists. Setup can only run once.',
      }),
    )
    expect(messageFor(error)).toBe('An operator account already exists. Setup can only run once.')
  })

  it('falls back to title when detail is missing', () => {
    const error = new ProblemError(
      problem('internal.unexpected', 500, { title: 'holzkube could not complete that request' }),
    )
    expect(messageFor(error)).toBe('holzkube could not complete that request')
  })

  it('falls back to title when detail is present but empty', () => {
    const error = new ProblemError(
      problem('internal.unexpected', 500, { title: 'Unexpected failure', detail: '   ' }),
    )
    expect(messageFor(error)).toBe('Unexpected failure')
  })

  it('never returns an empty string, even for a problem with no text at all', () => {
    const error = new ProblemError(problem('internal.unexpected', 500, { title: '' }))
    expect(messageFor(error).length).toBeGreaterThan(0)
    expect(messageFor(error)).not.toMatch(/^\d+$/)
  })

  it('handles a plain Error and an unknown throw without showing a status code', () => {
    expect(messageFor(new Error('the network went away'))).toBe('the network went away')
    expect(messageFor('not an error at all')).toBe('Something went wrong.')
    expect(messageFor(undefined)).toBe('Something went wrong.')
  })
})

describe('toProblemError', () => {
  function response(status: number, body: unknown, headers: Record<string, string>): Response {
    return new Response(body === null ? null : JSON.stringify(body), { status, headers })
  }

  it('decodes a well-formed problem+json body', async () => {
    const error = await toProblemError(
      response(428, problem('sudo.required', 428, { detail: 'This action is destructive.' }), {
        'Content-Type': 'application/problem+json',
      }),
    )
    expect(error).toBeInstanceOf(ProblemError)
    expect(error.code).toBe('sudo.required')
    expect(error.message).toBe('This action is destructive.')
  })

  it('produces a sentence, not a bare status, for a response with no usable body', async () => {
    const error = await toProblemError(response(502, null, { 'Content-Type': 'text/html' }))

    expect(error).toBeInstanceOf(ProblemError)
    expect(error.status).toBe(502)
    expect(error.message).not.toMatch(/^\d+$/)
    expect(error.message.length).toBeGreaterThan(20)
    // Still routable: an unreadable answer is a toast, not a silent no-op.
    expect(presentationFor(error.problem)).toBe('toast')
  })

  it('produces a sentence for problem+json whose body does not match the schema', async () => {
    const error = await toProblemError(
      response(500, { nonsense: true }, { 'Content-Type': 'application/problem+json' }),
    )
    expect(error.message.length).toBeGreaterThan(20)
    expect(error.status).toBe(500)
  })

  it('carries Retry-After through as seconds', async () => {
    const error = await toProblemError(
      response(429, problem('ratelimit.delayed', 429), {
        'Content-Type': 'application/problem+json',
        'Retry-After': '8',
      }),
    )
    expect(waitSecondsOf(error)).toBe(8)
  })
})

describe('fieldErrorsOf', () => {
  it('turns errors[] into a field-to-reason map', () => {
    const error = new ProblemError(
      problem('validation.failed', 400, {
        errors: [
          { field: 'password', reason: 'must be at least 12 characters' },
          { field: 'username', reason: 'must not be empty' },
        ],
      }),
    )
    expect(fieldErrorsOf(error)).toEqual({
      password: 'must be at least 12 characters',
      username: 'must not be empty',
    })
  })

  it('is empty for a problem without errors[] and for a non-problem throw', () => {
    expect(fieldErrorsOf(new ProblemError(problem('internal.unexpected', 500)))).toEqual({})
    expect(fieldErrorsOf(new Error('boom'))).toEqual({})
  })
})

describe('rate limiting is a delay, never a lock', () => {
  it('parses delta-seconds and rejects anything else', () => {
    expect(parseRetryAfter('30')).toBe(30)
    expect(parseRetryAfter(' 0 ')).toBe(0)
    expect(parseRetryAfter(null)).toBeNull()
    expect(parseRetryAfter('Wed, 21 Oct 2015 07:28:00 GMT')).toBeNull()
    expect(parseRetryAfter('-5')).toBeNull()
  })

  it('phrases the wait without ever implying a locked account', () => {
    for (const seconds of [null, 0, 1, 30]) {
      const text = waitMessage(seconds)
      expect(text).not.toMatch(/lock|locked|disabled|suspend|blocked/i)
      expect(text.length).toBeGreaterThan(0)
    }
    expect(waitMessage(1)).toContain('1 second')
    expect(waitMessage(30)).toContain('30 seconds')
  })
})
