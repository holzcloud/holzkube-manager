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

/**
 * rollUp is what the server does, mirrored here so a fixture cannot describe a
 * cluster whose two views of itself disagree. The rule itself is tested in Go
 * (internal/kube/wall_test.go); what these tests are about is what gets DRAWN.
 */
function rollUp(workloads: Wall['workloads']): Wall['namespaces'] {
  const order = ['down', 'unknown', 'warn', 'stopped', 'ok']
  const byName = new Map<string, Wall['namespaces'][number]>()
  for (const tile of workloads) {
    const name = tile.namespace === '' ? '—' : tile.namespace
    const into = byName.get(name) ?? { name, total: 0, state: 'ok', worst: '', stopped: 0 }
    into.total += 1
    if (tile.state === 'stopped') {
      into.stopped += 1
    } else if (order.indexOf(tile.state) < order.indexOf(into.state)) {
      into.state = tile.state
      into.worst = `${tile.name} · ${tile.detail}`
    }
    byName.set(name, into)
  }
  const out = [...byName.values()].map((tile) =>
    tile.stopped === tile.total && tile.state === 'ok' ? { ...tile, state: 'stopped' } : tile,
  )
  out.sort((a, b) =>
    order.indexOf(a.state) !== order.indexOf(b.state)
      ? order.indexOf(a.state) - order.indexOf(b.state)
      : b.total - a.total || a.name.localeCompare(b.name),
  )
  return out
}

function wallAt(when: Date, overrides: Partial<Wall> = {}): Wall {
  const base = {
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
    namespaces: [] as Wall['namespaces'],
    warnings: [],
    summary: { ok: 2 },
    ...overrides,
  }
  // Unless a test says otherwise, the roll-up follows from the workloads.
  return base.namespaces.length > 0 ? base : { ...base, namespaces: rollUp(base.workloads) }
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
    // And the size of the cluster beside it, so the headline has a denominator:
    // "everything is running" says much more when everything is 132 things.
    expect(await screen.findByText(/^as of \d+s ago · 1 workload, 1 node$/)).toBeInTheDocument()
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
    expect(await screen.findByText(/worst workload in it/)).toBeInTheDocument()
    expect(screen.getByText('running')).toBeInTheDocument()
    expect(screen.getByText('switched off on purpose')).toBeInTheDocument()
  })

  it('does not key a colour that is not on the screen', async () => {
    vi.spyOn(api.kubernetes, 'wall').mockResolvedValue(wallAt(new Date()))

    wrap(<WallView />)

    await screen.findByText(/worst workload in it/)
    // A key to something that is not there is one more thing to read past.
    expect(screen.queryByText(/switched off on purpose/)).toBeNull()
  })

  it('says why a warning happened, not only that one did', async () => {
    vi.spyOn(api.kubernetes, 'wall').mockResolvedValue(
      wallAt(new Date(), {
        warnings: [
          {
            object: 'Pod/dupl-test',
            reason: 'Failed',
            message: 'Error: ImagePullBackOff',
            count: 3,
            last_seen: '',
          },
        ],
      }),
    )

    wrap(<WallView />)

    // "Failed Pod/dupl-test" names a pod and says nothing at all about what
    // happened to it. The operator asked what it meant, and the answer had been
    // fetched, carried through the API and then not drawn.
    expect(await screen.findByText(/Error: ImagePullBackOff/)).toBeInTheDocument()
  })

  it('says how long ago a warning was, not when it was', async () => {
    const seen = new Date(Date.now() - 5 * 60 * 1000)
    vi.spyOn(api.kubernetes, 'wall').mockResolvedValue(
      wallAt(new Date(), {
        warnings: [
          {
            object: 'Pod/api-7c9',
            reason: 'FailedScheduling',
            message: 'insufficient memory',
            count: 340,
            last_seen: seen.toISOString(),
          },
        ],
      }),
    )

    wrap(<WallView />)

    // The first photograph of this column read "2026-09-20T09:58:00Z" across a
    // television. An instant is not something anybody reads from four metres,
    // and the field used to be called `age` while carrying one.
    expect(await screen.findByText('5 min ago')).toBeInTheDocument()
    expect(screen.queryByText(seen.toISOString())).toBeNull()
  })

  it('says a square is a workload and not a pod', async () => {
    vi.spyOn(api.kubernetes, 'wall').mockResolvedValue(wallAt(new Date()))

    wrap(<WallView />)

    // Asked twice, then asked whether the squares were pods. "Workload" is this
    // product's word and not the operator's.
    expect(await screen.findByText(/not a single pod/)).toBeInTheDocument()
  })

  it('accounts for every workload on a cluster where everything is fine', async () => {
    // The operator's own screen: 132 workloads, all healthy. The first version
    // of this page drew only the ones that were NOT fine past sixty, so it
    // showed "SHOWING 0 OF 132" and a screen of black -- an empty screen
    // claiming nothing was running, about a cluster running everything.
    //
    // The tiles are rolled up per namespace now, which is a different thing from
    // being hidden: the total is stated, every tile carries its own count, and
    // the counts have to ADD UP to the total. That is the claim this test makes,
    // and it is the one that would have caught the original defect.
    const many = Array.from({ length: 132 }, (_, i) => ({
      kind: 'Deployment',
      namespace: i % 3 === 0 ? 'cloud' : 'kube-system',
      name: `app-${i}`,
      state: 'ok',
      detail: '1 of 1 ready',
    }))
    vi.spyOn(api.kubernetes, 'wall').mockResolvedValue(wallAt(new Date(), { workloads: many }))

    const { container } = wrap(<WallView />)

    expect(await screen.findByText(/2 namespaces — 132 workloads/)).toBeInTheDocument()
    expect(screen.queryByText(/showing/i)).toBeNull()

    // 88 in kube-system and 44 in cloud, both on the screen, summing to 132.
    const counts = [...container.querySelectorAll('[data-state] .tabular-nums')].map((node) =>
      Number(node.textContent),
    )
    expect(counts.filter((n) => Number.isFinite(n)).reduce((a, b) => a + b, 0)).toBe(132)
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
    // And the roll-up has landmarks rather than being anonymous squares: the
    // namespace is named, and the one that is not green says who decided that.
    expect(screen.getByText('cloud')).toBeInTheDocument()
    expect(screen.getByText('db')).toBeInTheDocument()
    expect(screen.getByText('postgres · 0 of 3 ready')).toBeInTheDocument()
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

  it('names the nodes whether or not anything is wrong with them', async () => {
    vi.spyOn(api.kubernetes, 'wall').mockResolvedValue(
      wallAt(new Date(), {
        nodes: [
          { kind: 'Node', namespace: '', name: 'srv-node-01', state: 'ok', detail: 'ready' },
          { kind: 'Node', namespace: '', name: 'srv-node-02', state: 'ok', detail: 'ready' },
        ],
      }),
    )

    wrap(<WallView />)

    // A wall that only mentioned a node once it had already failed would be a
    // wall that never showed the thing it is most often consulted about. Three
    // machines the operator can walk over to; they are named, always.
    expect(await screen.findByText('srv-node-01')).toBeInTheDocument()
    expect(screen.getByText('srv-node-02')).toBeInTheDocument()
    expect(screen.getByText('Nodes')).toBeInTheDocument()
  })

  it('sizes a tile so a long hostname fits on it', async () => {
    vi.spyOn(api.kubernetes, 'wall').mockResolvedValue(
      wallAt(new Date(), {
        nodes: [
          {
            kind: 'Node',
            name: 'srv-node-01.homelab.example',
            namespace: '',
            state: 'ok',
            detail: 'ready',
          },
          {
            kind: 'Node',
            name: 'srv-node-02.homelab.example',
            namespace: '',
            state: 'ok',
            detail: 'ready',
          },
          {
            kind: 'Node',
            name: 'srv-node-03.homelab.example',
            namespace: '',
            state: 'ok',
            detail: 'ready',
          },
        ],
      }),
    )

    const { container } = wrap(<WallView />)
    await screen.findByText('srv-node-01.homelab.example')

    // Three tiles used to get the largest type purely because there were three
    // of them, and the name then broke mid-word across three lines:
    // "srv-node- / 02.homelab.exampl / e". A hostname is ONE word, so wrapping
    // cannot rescue it -- only the type size can. This screen has mangled a
    // node's name three times now.
    const tile = container.querySelector('[data-state="ok"]')
    expect(tile?.className).not.toContain('text-[2.9vmin]')
  })

  it('is legible on a phone and not only on a television', async () => {
    vi.spyOn(api.kubernetes, 'wall').mockResolvedValue(wallAt(new Date()))

    const { container } = wrap(<WallView />)
    await screen.findByText('Everything is running')

    // vmin is the right unit for the screen this page is FOR and the wrong one
    // for the screen the operator actually looked at it on first: on a 390px
    // phone, 1.4vmin is five pixels. The photograph showed the whole wall
    // rendered correctly and unreadable.
    //
    // So every vmin size is behind `md:` and carries a phone-sized base. A bare
    // `text-[…vmin]` is the regression, and it cannot be seen in jsdom -- which
    // has no viewport -- so it is checked as the class it is.
    const offenders: string[] = []
    for (const node of container.querySelectorAll('*')) {
      const classes = typeof node.className === 'string' ? node.className : ''
      for (const bare of classes.match(/(?<!md:)\btext-\[[0-9.]+vmin\]/g) ?? []) {
        offenders.push(`${bare} in "${classes}"`)
      }
    }
    expect(offenders).toEqual([])
  })

  it('fills its tiles with solid colour rather than a tint', async () => {
    vi.spyOn(api.kubernetes, 'wall').mockResolvedValue(
      wallAt(new Date(), {
        workloads: [
          { kind: 'Deployment', namespace: 'db', name: 'pg', state: 'down', detail: '0 of 3' },
        ],
      }),
    )

    const { container } = wrap(<WallView />)
    await screen.findByText('pg')

    // This screen is read at four metres, off-axis, on a television nobody has
    // calibrated, in a lit room. The first version tinted its tiles at fifteen
    // per cent opacity -- legible on a laptop at arm's length, and a field of
    // dark grey in the photograph the operator sent from across the room.
    for (const tile of container.querySelectorAll('[data-state]')) {
      const classes = tile.className
      expect(classes).not.toMatch(/bg-[a-z]+-\d+\/\d+/)
    }
    expect(container.querySelector('[data-state="down"]')?.className).toContain('bg-red-700')
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
