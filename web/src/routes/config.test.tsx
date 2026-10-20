import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api, configViewSchema, machineSchema, previewSchema } from '@/api'
import { ProblemError } from '@/lib/problem'
import { ConfigPage } from '@/routes/config'
import { problemType } from '@/test/problem-fixtures'
import demo from '../../fixtures/demo.json'

/**
 * The configuration page and the version check between plan and apply
 * (Phase 17, VERS-02 and VERS-06; decisions D-01 and D-02).
 *
 * What is held here is the page's behaviour with the server's answers mirrored:
 * the digest of the plan travels with the apply, a refusal shows the server's
 * sentence and nothing else and leaves the operator one way forward (plan
 * again, with the same patch text), and a preview that has landed cannot be
 * applied a second time. What is NOT held: how the yellow box looks, or whether
 * the button can be pressed on a phone. That is a browser's question, and the
 * browser project does not run here.
 */

const MACHINE = 'm-cp-1'
const PATCH = 'machine: patched'
const DIGEST_1 = '11'.repeat(32)
const DIGEST_2 = '22'.repeat(32)
const SENTENCE =
  'Nothing was applied: this node changed after the plan was made. KubeletConfig.image (modified).'

const fixtures = demo as Record<string, unknown>

/** The plan's answer as the server sends it; no digest when none is given. */
function answer(digest?: string, mode: 'no-reboot' | 'reboot' = 'no-reboot') {
  return {
    machine: MACHINE,
    ...(digest === undefined ? {} : { base_digest: digest }),
    diff: { changes: [], paths: [], duplicates: false },
    verdict: {
      mode,
      reboot_required: mode === 'reboot',
      try_seconds: 0,
      sentences: [`This applies in ${mode} mode.`],
    },
    result: 'machine: {}\n',
    idempotent: true,
    valid: true,
    validation: [],
  }
}

function preview(digest: string, mode: 'no-reboot' | 'reboot' = 'no-reboot') {
  return previewSchema.parse(answer(digest, mode))
}

function refusal() {
  return new ProblemError({
    type: problemType('conflict.config-changed-since-plan'),
    title: 'The configuration changed since the plan',
    status: 409,
    detail: SENTENCE,
    code: 'conflict.config-changed-since-plan',
  })
}

function mirror() {
  const machine = machineSchema.parse(fixtures[`/api/v1/machines/${MACHINE}`])
  vi.spyOn(api.machines, 'list').mockResolvedValue([machine])
  vi.spyOn(api.config, 'get').mockResolvedValue(
    configViewSchema.parse({
      machine: MACHINE,
      raw: 'machine: {}\n',
      rendered: 'machine: {}\n',
      read_at: '2026-10-20T09:00:00Z',
    }),
  )
  vi.spyOn(api.patches, 'list').mockResolvedValue([])
  return machine
}

/** The page, a patch typed in, and a first plan made. */
async function planned() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <ConfigPage />
    </QueryClientProvider>,
  )
  // The saved-patches card has textboxes of its own and stands before the node
  // is chosen, so the patch field is the one that carries the machine
  // placeholder, found once the page has a node.
  const plan = await screen.findByRole('button', { name: 'See what would change' })
  const field = screen
    .getAllByRole('textbox')
    .find((box) => box.getAttribute('placeholder')?.startsWith('machine:'))
  if (field === undefined) throw new Error('the patch field is not on the page')
  await userEvent.type(field, PATCH)
  await userEvent.click(plan)
  await screen.findByRole('button', { name: 'Apply' })
}

const applyButton = () => screen.getByRole('button', { name: 'Apply' })

beforeEach(() => {
  vi.restoreAllMocks()
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('the preview carries the digest of the configuration it was made from', () => {
  it('refuses an answer of the plan that has no base_digest', () => {
    expect(previewSchema.safeParse(answer(DIGEST_1)).success).toBe(true)
    expect(previewSchema.safeParse(answer(undefined)).success).toBe(false)
  })

  it('sends the digest of the plan it was applied from', async () => {
    const machine = mirror()
    vi.spyOn(api.config, 'plan').mockResolvedValue(preview(DIGEST_1))
    const apply = vi
      .spyOn(api.config, 'apply')
      .mockResolvedValue({ mode: 'no-reboot', details: '' })
    await planned()

    await userEvent.click(applyButton())

    expect(apply).toHaveBeenCalledTimes(1)
    expect(apply).toHaveBeenCalledWith(machine.id, [PATCH], 'no-reboot', machine.cluster, DIGEST_1)
    expect(apply.mock.calls[0]).toHaveLength(5)
  })
})

describe('an apply the server refused because the node changed since the plan', () => {
  it("shows the server's sentence, locks Apply and offers to plan again when the apply is refused", async () => {
    mirror()
    vi.spyOn(api.config, 'plan').mockResolvedValue(preview(DIGEST_1))
    vi.spyOn(api.config, 'apply').mockRejectedValue(refusal())
    await planned()

    await userEvent.click(applyButton())

    // The sentence is the server's, word for word, in an alert.
    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent(SENTENCE)
    expect(applyButton()).toBeDisabled()
    expect(screen.getByRole('button', { name: 'See what would change again' })).toBeEnabled()
    // The preview stays readable under the box.
    expect(screen.getByText('This applies in no-reboot mode.')).toBeInTheDocument()
    // D-01: there is no way to apply anyway.
    expect(screen.queryByRole('button', { name: /anyway|overwrite|force/i })).toBeNull()
  })

  it('sends the new digest after planning again, not the old one', async () => {
    const machine = mirror()
    const plan = vi
      .spyOn(api.config, 'plan')
      .mockResolvedValueOnce(preview(DIGEST_1))
      .mockResolvedValueOnce(preview(DIGEST_2, 'reboot'))
    const apply = vi
      .spyOn(api.config, 'apply')
      .mockRejectedValueOnce(refusal())
      .mockResolvedValueOnce({ mode: 'reboot', details: '' })
    await planned()
    await userEvent.click(applyButton())
    await screen.findByRole('alert')

    await userEvent.click(screen.getByRole('button', { name: 'See what would change again' }))

    // The sentence stands until the new plan is there, and is gone after it.
    await screen.findByText('This applies in reboot mode.')
    expect(screen.queryByRole('alert')).toBeNull()
    expect(plan).toHaveBeenCalledTimes(2)
    expect(plan.mock.calls[1]).toEqual(plan.mock.calls[0])
    expect(plan.mock.calls[1]?.[1]).toEqual([PATCH])
    expect(applyButton()).toBeEnabled()

    await userEvent.click(applyButton())

    expect(apply).toHaveBeenCalledTimes(2)
    expect(apply).toHaveBeenLastCalledWith(machine.id, [PATCH], 'reboot', machine.cluster, DIGEST_2)
  })

  it('keeps the sentence on the page until the new plan has arrived', async () => {
    mirror()
    let finish: (p: ReturnType<typeof preview>) => void = () => {}
    vi.spyOn(api.config, 'plan')
      .mockResolvedValueOnce(preview(DIGEST_1))
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            finish = resolve
          }),
      )
    vi.spyOn(api.config, 'apply').mockRejectedValue(refusal())
    await planned()
    await userEvent.click(applyButton())
    await screen.findByRole('alert')

    await userEvent.click(screen.getByRole('button', { name: 'See what would change again' }))

    expect(screen.getByRole('alert')).toHaveTextContent(SENTENCE)
    expect(applyButton()).toBeDisabled()
    finish(preview(DIGEST_2))
    await screen.findByRole('button', { name: 'Apply' })
    await vi.waitFor(() => expect(screen.queryByRole('alert')).toBeNull())
  })
})

describe('an apply that landed', () => {
  it('cannot apply the same preview a second time after it landed', async () => {
    mirror()
    vi.spyOn(api.config, 'plan').mockResolvedValue(preview(DIGEST_1))
    const apply = vi
      .spyOn(api.config, 'apply')
      .mockResolvedValue({ mode: 'no-reboot', details: '' })
    await planned()

    await userEvent.click(applyButton())

    await screen.findByText(/Applied in no-reboot mode/)
    expect(applyButton()).toBeDisabled()
    await userEvent.click(applyButton())
    expect(apply).toHaveBeenCalledTimes(1)
  })

  it('starts from the new verdict once the operator plans again', async () => {
    const machine = mirror()
    vi.spyOn(api.config, 'plan')
      .mockResolvedValueOnce(preview(DIGEST_1))
      .mockResolvedValueOnce(preview(DIGEST_2, 'reboot'))
    const apply = vi.spyOn(api.config, 'apply').mockResolvedValue({ mode: 'x', details: '' })
    await planned()
    await userEvent.click(applyButton())
    await screen.findByText(/Applied in/)

    await userEvent.click(screen.getByRole('button', { name: 'See what would change' }))

    await screen.findByText('This applies in reboot mode.')
    expect(screen.queryByText(/Applied in/)).toBeNull()
    expect(applyButton()).toBeEnabled()
    await userEvent.click(applyButton())
    expect(apply).toHaveBeenLastCalledWith(machine.id, [PATCH], 'reboot', machine.cluster, DIGEST_2)
  })
})

describe('any other failure of the apply', () => {
  it('leaves Apply usable and says why for any other failure', async () => {
    mirror()
    vi.spyOn(api.config, 'plan').mockResolvedValue(preview(DIGEST_1))
    vi.spyOn(api.config, 'apply').mockRejectedValue(new Error('the node did not answer'))
    await planned()

    await userEvent.click(applyButton())

    const message = await screen.findByText('the node did not answer')
    expect(message).toHaveClass('text-destructive')
    expect(screen.queryByRole('alert')).toBeNull()
    expect(screen.queryByRole('button', { name: 'See what would change again' })).toBeNull()
    expect(applyButton()).toBeEnabled()
  })

  it('does not take another conflict for the refusal, whatever its status', async () => {
    mirror()
    vi.spyOn(api.config, 'plan').mockResolvedValue(preview(DIGEST_1))
    vi.spyOn(api.config, 'apply').mockRejectedValue(
      new ProblemError({
        type: problemType('store.conflict'),
        title: 'Conflict',
        status: 409,
        detail: 'Another apply on this node is still running.',
        code: 'store.conflict',
      }),
    )
    await planned()

    await userEvent.click(applyButton())

    await screen.findByText('Another apply on this node is still running.')
    expect(screen.queryByRole('button', { name: 'See what would change again' })).toBeNull()
    expect(applyButton()).toBeEnabled()
  })

  it('drops the refusal when the new plan fails, and shows the failure of the plan', async () => {
    mirror()
    vi.spyOn(api.config, 'plan')
      .mockResolvedValueOnce(preview(DIGEST_1))
      .mockRejectedValueOnce(new Error('the node could not be read'))
    vi.spyOn(api.config, 'apply').mockRejectedValue(refusal())
    await planned()
    await userEvent.click(applyButton())
    await screen.findByRole('alert')

    await userEvent.click(screen.getByRole('button', { name: 'See what would change again' }))

    await screen.findByText('the node could not be read')
    expect(screen.queryByRole('alert')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Apply' })).toBeNull()
  })
})
