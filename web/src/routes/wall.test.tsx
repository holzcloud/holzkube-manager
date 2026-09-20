import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api, type Wall } from '@/api'
import { WallView } from '@/routes/wall'

/**
 * The wall.
 *
 * A screen nobody touches, left up for weeks. The failure that matters is not a
 * wrong tile — it is a CONFIDENT one: a green screen that is forty minutes old
 * during the incident it exists for.
 */

function wrap(node: ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

const cluster = { id: 'c-1', name: 'homelab' }

function wallAt(when: Date, overrides: Partial<Wall> = {}): Wall {
  return {
    generated_at: when.toISOString(),
    nodes: [{ kind: 'Node', namespace: '', name: 'cp-1', state: 'ok', detail: 'ready' }],
    workloads: [
      {
        kind: 'Deployment',
        namespace: 'default',
        name: 'website',
        state: 'ok',
        detail: '2 of 2 ready',
      },
    ],
    cpu: { allocatable: '4', requested: '1', percent: 25 },
    memory: { allocatable: '8Gi', requested: '2Gi', percent: 25 },
    pods: { allocatable: '110', requested: '20', percent: 18 },
    warnings: [],
    summary: { ok: 2 },
    ...overrides,
  }
}

beforeEach(() => {
  // biome-ignore lint/suspicious/noExplicitAny: the clusters list carries more
  // fields than this screen reads, and it reads only the id.
  vi.spyOn(api.clusters, 'list').mockResolvedValue([cluster] as any)
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.useRealTimers()
})

describe('the wall', () => {
  it('leads with the worst thing, in words', async () => {
    vi.spyOn(api.kubernetes, 'wall').mockResolvedValue(
      wallAt(new Date(), { summary: { ok: 4, warn: 1, down: 2 } }),
    )

    wrap(<WallView />)

    // Not "2 down, 1 warn, 4 ok": from four metres that is a puzzle. The worst
    // number, in a sentence, in the largest type on the screen.
    expect(await screen.findByText('2 not running')).toBeInTheDocument()
  })

  it('says everything is running only when it is', async () => {
    vi.spyOn(api.kubernetes, 'wall').mockResolvedValue(wallAt(new Date()))

    wrap(<WallView />)

    expect(await screen.findByText('Everything is running')).toBeInTheDocument()
  })

  it('counts a node nobody is hearing from as not running', async () => {
    vi.spyOn(api.kubernetes, 'wall').mockResolvedValue(
      wallAt(new Date(), { summary: { ok: 3, unknown: 1 } }),
    )

    wrap(<WallView />)

    // Unknown is not warn and is certainly not ok. A wall that said "everything
    // is running" beside a node it cannot hear from would be lying in exactly
    // the case it exists for.
    expect(await screen.findByText('1 not running')).toBeInTheDocument()
  })

  it('says how old the answer is, always and not only when it is bad', async () => {
    vi.spyOn(api.kubernetes, 'wall').mockResolvedValue(wallAt(new Date()))

    wrap(<WallView />)

    // A number that appears only during trouble is one nobody has learned to
    // read by the time it appears.
    expect(await screen.findByText(/^as of \d+s ago$/)).toBeInTheDocument()
  })

  it('stops looking confident when the answer is old', async () => {
    // Older than three refresh intervals: one missed refresh is a network
    // blink, three is something wrong.
    vi.spyOn(api.kubernetes, 'wall').mockResolvedValue(wallAt(new Date(Date.now() - 5 * 60 * 1000)))

    wrap(<WallView />)

    expect(await screen.findByText(/this may be out of date/)).toBeInTheDocument()
    // And the good news is not shouted while it is out of date.
    expect(screen.queryByText(/^as of /)).toBeNull()
  })

  it('never shows a green screen when the cluster could not be asked', async () => {
    vi.spyOn(api.kubernetes, 'wall').mockRejectedValue(new Error('the API server did not answer'))

    wrap(<WallView />)

    // An unanswered question is not an answer. This is the one thing a wall
    // must never get wrong.
    expect(await screen.findByText('The cluster could not be asked.')).toBeInTheDocument()
    expect(screen.queryByText('Everything is running')).toBeNull()
  })

  it('shows a tile for each thing, with what it is', async () => {
    vi.spyOn(api.kubernetes, 'wall').mockResolvedValue(
      wallAt(new Date(), {
        workloads: [
          {
            kind: 'Deployment',
            namespace: 'default',
            name: 'website',
            state: 'down',
            detail: '0 of 3 ready',
          },
        ],
      }),
    )

    wrap(<WallView />)

    expect(await screen.findByText('website')).toBeInTheDocument()
    // The namespace rides with the detail: on a wall "website" alone is not an
    // address, and two namespaces with a website each is the ordinary case.
    expect(screen.getByText('default · 0 of 3 ready')).toBeInTheDocument()
  })

  it('says on the screen what a square is and what the colours mean', async () => {
    vi.spyOn(api.kubernetes, 'wall').mockResolvedValue(
      wallAt(new Date(), {
        workloads: [
          { kind: 'Deployment', namespace: 'web', name: 'a', state: 'ok', detail: '' },
          { kind: 'Deployment', namespace: 'web', name: 'b', state: 'stopped', detail: '' },
        ],
      }),
    )

    wrap(<WallView />)

    // The operator asked what the green squares meant. Having to ask is the
    // defect: a wall is read by people nobody told anything, and an answer given
    // once in a conversation is an answer nobody walking past ever gets.
    expect(await screen.findByText(/one square each/)).toBeInTheDocument()
    expect(screen.getByText(/running 1/)).toBeInTheDocument()
    expect(screen.getByText(/stopped on purpose 1/)).toBeInTheDocument()
  })

  it('does not key a colour that is not on the screen', async () => {
    vi.spyOn(api.kubernetes, 'wall').mockResolvedValue(wallAt(new Date()))

    wrap(<WallView />)

    await screen.findByText(/one square each/)
    // A key to something that is not there is one more thing to read past.
    expect(screen.queryByText(/stopped on purpose/)).toBeNull()
  })

  it('hides nothing on a cluster where everything is fine', async () => {
    // The operator's own screen: 132 workloads, all healthy. The first version
    // of this page drew only the ones that were NOT fine past sixty, so it
    // showed "SHOWING 0 OF 132" and a screen of black -- an empty screen
    // claiming nothing was running, about a cluster running everything.
    const many = Array.from({ length: 132 }, (_, i) => ({
      kind: 'Deployment',
      namespace: i % 3 === 0 ? 'cloud' : 'kube-system',
      name: `app-${i}`,
      state: 'ok',
      detail: '1 of 1 ready',
    }))
    vi.spyOn(api.kubernetes, 'wall').mockResolvedValue(wallAt(new Date(), { workloads: many }))

    const { container } = wrap(<WallView />)

    expect(await screen.findByText(/132 workloads/)).toBeInTheDocument()
    // Every one of them is drawn. A count instead of the tiles is the failure.
    expect(container.querySelectorAll('[data-row]')).toHaveLength(133) // + the node
    expect(screen.queryByText(/showing/i)).toBeNull()
  })

  it('names what needs attention and leaves the rest as a field', async () => {
    const tiles = [
      {
        kind: 'Deployment',
        namespace: 'db',
        name: 'postgres',
        state: 'down',
        detail: '0 of 3 ready',
      },
      ...Array.from({ length: 40 }, (_, i) => ({
        kind: 'Deployment',
        namespace: 'cloud',
        name: `fine-${i}`,
        state: 'ok',
        detail: '1 of 1 ready',
      })),
    ]
    vi.spyOn(api.kubernetes, 'wall').mockResolvedValue(wallAt(new Date(), { workloads: tiles }))

    wrap(<WallView />)

    // The broken one is named and readable; forty healthy names cannot be read
    // from four metres and nobody is trying to. What a wall is read for is the
    // shape: a field of green with one red in it.
    expect(await screen.findByText('Needs attention')).toBeInTheDocument()
    expect(screen.getByText('postgres')).toBeInTheDocument()
    expect(screen.queryByText('fine-0')).toBeNull()
    // And the field has landmarks rather than being anonymous squares.
    expect(screen.getByText('cloud')).toBeInTheDocument()
  })

  it('says nothing about attention when there is none to pay', async () => {
    vi.spyOn(api.kubernetes, 'wall').mockResolvedValue(wallAt(new Date()))

    wrap(<WallView />)

    await screen.findByText('Everything is running')
    expect(screen.queryByText('Needs attention')).toBeNull()
  })

  it('leaves something deliberately stopped out of the attention list', async () => {
    vi.spyOn(api.kubernetes, 'wall').mockResolvedValue(
      wallAt(new Date(), {
        workloads: [
          {
            kind: 'Deployment',
            namespace: 'default',
            name: 'paused',
            state: 'stopped',
            detail: '0 of 0 ready',
          },
        ],
      }),
    )

    wrap(<WallView />)

    // A wall that put every deliberately stopped workload in the attention list
    // is a wall asking to be ignored.
    await screen.findByText('Everything is running')
    expect(screen.queryByText('Needs attention')).toBeNull()
  })

  it('keeps the last answer up when a refresh fails, rather than blanking', async () => {
    const asked = vi
      .spyOn(api.kubernetes, 'wall')
      .mockResolvedValueOnce(wallAt(new Date()))
      .mockRejectedValue(new Error('gone'))

    wrap(<WallView />)
    await screen.findByText('Everything is running')

    // A wall that blanked on every hiccup would spend its life blank. The state
    // a moment ago, with its age on screen, beats nothing at all.
    await waitFor(() => expect(asked).toHaveBeenCalled())
    expect(screen.getByText('cp-1')).toBeInTheDocument()
  })
})
