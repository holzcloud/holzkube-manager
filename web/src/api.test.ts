import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, onSudoRequired, type SudoChallenge } from '@/api'

/**
 * The request pipeline's own tests, at the one place in the frontend that calls
 * fetch.
 *
 * What is under test here is the ceiling: every request carries one, it is
 * created fresh for each attempt, and a server that never answers ends as a
 * stated failure rather than as a promise that never settles.
 *
 * The fetch stub is the whole fixture. No server runs and no timer is advanced
 * beyond what a test needs -- the never-settling case asserts on the signal that
 * was handed to fetch rather than on waiting out a 150-second ceiling, which is
 * the difference between a test and an afternoon.
 */

afterEach(() => {
  vi.unstubAllGlobals()
  onSudoRequired(null)
})

/** A JSON response the pipeline reads as success. */
function ok(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })
}

/**
 * A fetch that accepts the request and then says nothing, exactly as a throttled
 * upstream or a laptop that went to sleep does. It settles only when the signal
 * it was handed fires, and then with the `TimeoutError` the platform raises for
 * an `AbortSignal.timeout`.
 */
function neverAnswers(init: RequestInit): Promise<Response> {
  return new Promise<Response>((_resolve, reject) => {
    init.signal?.addEventListener('abort', () => {
      reject(new DOMException('The operation timed out.', 'TimeoutError'))
    })
  })
}

/** The 428 that opens the sudo prompt, in the shape the server sends it. */
function sudoRequired(): Response {
  return new Response(
    JSON.stringify({
      type: 'https://holzkube.example/problems/sudo.required',
      title: 'Confirm your password',
      status: 428,
      code: 'sudo.required',
      detail: 'This action needs a fresh password confirmation.',
    }),
    { status: 428, headers: { 'Content-Type': 'application/problem+json' } },
  )
}

describe('every request carries a ceiling', () => {
  it('hands fetch an abort signal on every call', async () => {
    const signals: Array<AbortSignal | null | undefined> = []
    vi.stubGlobal(
      'fetch',
      vi.fn((_path: string, init: RequestInit) => {
        signals.push(init.signal)
        return Promise.resolve(ok([]))
      }),
    )

    await api.schematics.list()

    expect(signals).toHaveLength(1)
    expect(signals[0]).toBeInstanceOf(AbortSignal)
  })

  /**
   * The ceiling has to be longer than the largest budget the server declares, or
   * it aborts while the server is still working and replaces a problem+json --
   * which says what went wrong upstream and whether retrying helps -- with a
   * generic network failure that says nothing.
   *
   * 130 seconds is `writeTimeout` in cmd/holzkube-managerd/main.go, the
   * outermost bound the server puts on producing any response at all. This
   * asserts the relation rather than the value, because the value is allowed to
   * move and the relation is not.
   */
  it("sets the ceiling above the server's outermost response bound", async () => {
    const ceiling = vi.spyOn(AbortSignal, 'timeout')
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(ok([]))),
    )

    await api.schematics.list()

    expect(ceiling).toHaveBeenCalledTimes(1)
    const call = ceiling.mock.calls[0]
    expect(call).toBeDefined()
    expect(call?.[0]).toBeGreaterThan(130_000)
    ceiling.mockRestore()
  })

  /**
   * The failure this case exists for is the one nothing else can see: a request
   * whose server accepts the connection and then says nothing. Without a ceiling
   * the returned promise never settles, so the caller's pending state never
   * clears and the operator reads a spinner that has no end and no error
   * (T-02-109).
   *
   * The ceiling itself is 150 seconds and waiting it out would make this an
   * afternoon rather than a test. Fake timers cannot help: `AbortSignal.timeout`
   * is a platform primitive with its own clock, not a `setTimeout` vitest can
   * advance. So the *firing* is driven directly -- the signal handed to fetch is
   * one this test aborts -- and everything downstream of it is the production
   * path: fetch rejects with the `TimeoutError` the platform raises, and the
   * pipeline classifies and reports it.
   */
  it('rejects with the ceiling named when the server never answers', async () => {
    const controller = new AbortController()
    const ceiling = vi.spyOn(AbortSignal, 'timeout').mockReturnValue(controller.signal)
    vi.stubGlobal(
      'fetch',
      vi.fn((_path: string, init: RequestInit) => neverAnswers(init)),
    )

    const pending = api.schematics.list()
    controller.abort()

    await expect(pending).rejects.toThrow(/did not answer within \d+ seconds/i)
    ceiling.mockRestore()
  })

  /**
   * The message must not claim anything about the server's state. A request
   * given up on may have been received, acted on and answered into a socket
   * nobody is holding any more, and a sentence that says "the server failed"
   * would be asserting something this side cannot know.
   */
  it('does not dress the abort up as a server problem', async () => {
    const controller = new AbortController()
    const ceiling = vi.spyOn(AbortSignal, 'timeout').mockReturnValue(controller.signal)
    vi.stubGlobal(
      'fetch',
      vi.fn((_path: string, init: RequestInit) => neverAnswers(init)),
    )

    const pending = api.schematics.list().then(
      () => null,
      (cause: unknown) => cause,
    )
    controller.abort()
    const cause = await pending

    expect(cause).toBeInstanceOf(Error)
    const message = (cause as Error).message
    expect(message).toMatch(/given up on/i)
    expect(message).toMatch(/may or may not have been carried out/i)
    // Not a ProblemError: there was no response to read, so there is no problem
    // document and nothing to put words into the server's mouth with.
    expect(cause).not.toHaveProperty('problem')
    ceiling.mockRestore()
  })

  /**
   * The sudo replay is the reason the signal cannot live in `init`.
   *
   * `init` is built once and reused so the replayed request is byte for byte the
   * one that was refused. An `AbortSignal.timeout` starts counting when it is
   * created, so a signal stored there would reach the replay partly spent -- or
   * already fired, after an operator who took their time at the password prompt
   * -- and abort a request that had not started.
   */
  it('gives the sudo replay a fresh ceiling rather than the remainder of the first', async () => {
    const signals: Array<AbortSignal | null | undefined> = []
    let call = 0
    vi.stubGlobal(
      'fetch',
      vi.fn((_path: string, init: RequestInit) => {
        signals.push(init.signal)
        call += 1
        return Promise.resolve(call === 1 ? sudoRequired() : ok({}))
      }),
    )
    onSudoRequired((challenge: SudoChallenge) => {
      challenge.settle(true)
    })

    await api.schematics.remove('a'.repeat(64))

    expect(signals).toHaveLength(2)
    expect(signals[0]).toBeInstanceOf(AbortSignal)
    expect(signals[1]).toBeInstanceOf(AbortSignal)
    expect(signals[1]).not.toBe(signals[0])
  })
})
