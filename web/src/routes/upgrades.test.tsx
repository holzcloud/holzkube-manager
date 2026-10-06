import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { EtcdMemberList, GateVerdict, UpgradePlan } from '@/api'
import {
  EtcdPanel,
  GatePanel,
  MemberTable,
  PlanView,
  SafetySnapshotStep,
  TalosUpdateBanner,
} from '@/routes/upgrades'

/**
 * The two claims this screen makes that are not about layout.
 *
 * UPG-02 asks that the gate show its inputs. A gate that only says no is a
 * gate an operator works around, and one that says yes without saying why is
 * one nobody can check — so the numbers have to be on the screen in both
 * directions, which is what these pin.
 *
 * UPG-10 asks that an etcd member be named by hostname. An operator asked to
 * confirm the removal of "8e9e05c52164694d" is confirming a string.
 */

function verdict(over: Partial<GateVerdict>): GateVerdict {
  return {
    ok: true,
    reason: '',
    input: {
      members: [],
      voting: 3,
      statuses: {},
      unreachable: [],
      alarms: [],
      max_raft_lag: 4,
    },
    ...over,
  }
}

describe('the health gate panel', () => {
  it('shows the numbers it decided on when it passes', () => {
    render(<GatePanel gate={verdict({ ok: true })} />)

    expect(screen.getByText(/passes right now/)).toBeInTheDocument()
    expect(screen.getByText('3')).toBeInTheDocument()
    expect(screen.getByText('4')).toBeInTheDocument()
  })

  it('shows the same numbers when it refuses, plus why', () => {
    render(
      <GatePanel
        gate={verdict({
          ok: false,
          reason: 'etcd has 2 voting members.',
          input: {
            members: [],
            voting: 2,
            statuses: {},
            unreachable: [],
            alarms: [{ MemberID: 1, Type: 'NOSPACE' }],
            max_raft_lag: 0,
          },
        })}
      />,
    )

    expect(screen.getByText(/refuses/)).toBeInTheDocument()
    expect(screen.getByText('etcd has 2 voting members.')).toBeInTheDocument()
    expect(screen.getByText('NOSPACE')).toBeInTheDocument()
  })

  it('says that the gate shown is not the gate that decides', () => {
    render(<GatePanel gate={verdict({})} />)
    expect(screen.getByText(/again before every node/)).toBeInTheDocument()
  })

  it('names the members that did not answer', () => {
    render(
      <GatePanel
        gate={verdict({
          ok: false,
          input: {
            members: [],
            voting: 3,
            statuses: {},
            unreachable: ['cp-3'],
            alarms: [],
            max_raft_lag: 0,
          },
        })}
      />,
    )
    expect(screen.getByText(/cp-3/)).toBeInTheDocument()
  })
})

describe('the etcd member list', () => {
  const list: EtcdMemberList = {
    members: [
      {
        id: '8e9e05c52164694d',
        name: 'cp-1 (id 8e9e05c52164694d)',
        machine: 'm1',
        learner: false,
        voting: true,
      },
      { id: 'ab12', name: 'cp-2 (learner, id ab12)', machine: '', learner: true, voting: false },
    ],
    voting_count: 1,
    tolerates: 0,
    sentence: '1 voting member(s). Losing any one of them stops the cluster accepting writes.',
  }

  it('names members by hostname, not only by hex id (UPG-10)', () => {
    render(<MemberTable list={list} onRemove={vi.fn()} />)

    expect(screen.getByText(/cp-1/)).toBeInTheDocument()
    expect(screen.getByText(/cp-2/)).toBeInTheDocument()
  })

  it('marks a learner as one, because a learner does not vote', () => {
    render(<MemberTable list={list} onRemove={vi.fn()} />)
    expect(screen.getByText('learner')).toBeInTheDocument()
    expect(screen.getByText('voting')).toBeInTheDocument()
  })

  it('will not offer to remove a voter from a cluster that cannot spare one', () => {
    render(<MemberTable list={list} onRemove={vi.fn()} />)

    const buttons = screen.getAllByRole('button', { name: 'Remove' })
    // The voter's button is disabled; the learner's is not, because removing a
    // learner does not change the quorum.
    expect(buttons[0]).toBeDisabled()
    expect(buttons[1]).not.toBeDisabled()
  })

  it('states what the member count means rather than only showing it', () => {
    render(<MemberTable list={list} onRemove={vi.fn()} />)
    expect(screen.getByText(/stops the cluster accepting writes/)).toBeInTheDocument()
  })
})

/** A Field<T> as the API serialises one: a value with its own availability. */
function field(value: unknown, level = 'node') {
  return { value, level, available: true, unavailable_reason: '' }
}

/** One control-plane machine, in the shape machineSchema requires. */
function controlPlaneFixture() {
  return {
    id: 'cp-1-uuid',
    cluster: 'c-1',
    role: 'controlplane',
    stage: 'watching',
    adopted_at: '2026-09-12T00:00:00Z',
    hostname: field('cp-1', 'none'),
    addr: field('10.0.0.1', 'none'),
    talos_version: field('v1.13.9'),
    kubernetes_version: field('v1.34.0', 'k8s'),
    schematic_id: field(''),
    manufacturer: field(''),
    product_name: field(''),
    serial_number: field(''),
    memory_mib: field(0),
    cpus: field([]),
    disks: field([]),
    interfaces: field([]),
    services: field([]),
    etcd_member: field(false, 'etcd'),
    compatibility: field({ known: true, supported: true, headroom_minors: 1, sentence: '' }),
  }
}

/**
 * What the snapshot card says, and what it asks for before it restores.
 *
 * A backup button with no restore behind it is a promise the operator
 * discovers is empty during the disaster. There is a restore now; what is said
 * here — where somebody forms the belief, next to the download — is what it
 * costs and what it leaves for them to finish.
 */
describe('the etcd snapshot card', () => {
  function wrapEtcd() {
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL) => {
        const path = String(input)
        const body = path.includes('/machines')
          ? { machines: [controlPlaneFixture()] }
          : { members: [], voting_count: 0, tolerates: 0, sentence: '' }
        return Promise.resolve(
          new Response(JSON.stringify(body), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          }),
        )
      }),
    )

    const client = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    })
    return render(
      <QueryClientProvider client={client}>
        <EtcdPanel cluster="c-1" />
      </QueryClientProvider>,
    )
  }

  it('offers the snapshot as a link the browser streams to disk', () => {
    wrapEtcd()
    expect(screen.getByRole('link', { name: /etcd snapshot/i })).toHaveAttribute(
      'href',
      expect.stringContaining('/api/v1/clusters/c-1/etcd/snapshot'),
    )
  })

  /**
   * This screen used to say the product does not restore a snapshot, with an
   * argument for why a browser is the wrong place for one: a restore runs on
   * exactly one control-plane node, and doing it on two produces two clusters
   * that each believe they are the original.
   *
   * The argument was not wrong and it is not dropped. It is answered in the
   * shape of the operation -- one named node, confirmed by that node's own id
   * -- and the part of it that is still true is what these tests pin: what the
   * restore costs, and what it does not do for the operator afterwards.
   */
  it('says what a restore costs before offering one', () => {
    wrapEtcd()
    expect(screen.getByText(/Everything written since it was taken is gone/i)).toBeInTheDocument()
    expect(screen.getByText(/reset and rejoined/i)).toBeInTheDocument()
    expect(screen.getByText(/Nothing here does that for you/i)).toBeInTheDocument()
  })

  it("will not restore until a node, a file and that node's own id are all given", async () => {
    const user = userEvent.setup()
    wrapEtcd()

    const button = screen.getByRole('button', { name: /Restore etcd from this snapshot/i })
    expect(button).toBeDisabled()

    // A node chosen and a file chosen is still not enough: the confirmation is
    // the node's id, because the question is not "did you mean to do this" but
    // "did you mean to do it here".
    await user.click(screen.getByRole('combobox', { name: /Restore onto/i }))
    await user.click(await screen.findByRole('option', { name: /cp-1/ }))

    const input = screen.getByLabelText(/^Snapshot$/i)
    await user.upload(input, new File(['a snapshot'], 'etcd.snapshot'))
    expect(button).toBeDisabled()

    await user.type(screen.getByLabelText(/Type the node's id to confirm/i), 'wrong-id')
    expect(button).toBeDisabled()
  })

  it('warns that skipping the integrity check is for a data-directory copy only', () => {
    wrapEtcd()
    expect(screen.getByLabelText(/Skip the snapshot's integrity check/i)).not.toBeChecked()
    expect(screen.getByText(/had already lost quorum/i)).toBeInTheDocument()
  })
})

/**
 * Is there a newer Talos, and the snapshot a start requires.
 *
 * The banner starts nothing -- it names the next run and opens the plan -- and
 * the start button is not pressable without a fresh snapshot, in the browser
 * as well as on the server. The server's refusal is the rule
 * (internal/httpapi, TestATalosUpgradeCannotStartWithoutAFreshSnapshot); these
 * pin that the screen says it before somebody types a cluster's name.
 */
describe('the Talos update banner and the snapshot step', () => {
  function stub(responses: Record<string, unknown>, calls: string[] = []) {
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const path = String(input)
        calls.push(`${init?.method ?? 'GET'} ${path}`)
        const key = Object.keys(responses).find((k) => path.includes(k))
        return Promise.resolve(
          new Response(JSON.stringify(key ? responses[key] : {}), {
            status: key ? 200 : 404,
            headers: { 'Content-Type': 'application/json' },
          }),
        )
      }),
    )
  }

  function inClient(ui: React.ReactElement) {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    })
    return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>)
  }

  const behind = {
    checked_at: '2026-10-06T12:00:00Z',
    notice: '',
    clusters: [
      {
        cluster: 'c-1',
        name: 'homelab',
        current: ['v1.12.6'],
        newest: 'v1.14.2',
        available: true,
        next: 'v1.13.9',
        runs: 2,
        notes_url: 'https://github.com/siderolabs/talos/releases/tag/v1.13.9',
      },
    ],
  }

  it('names the update, the next run and the release notes, and plans only when asked', async () => {
    const user = userEvent.setup()
    stub({ '/upgrade/talos/check': behind })
    const onPlan = vi.fn()
    inClient(<TalosUpdateBanner cluster="c-1" onPlan={onPlan} planning={false} />)

    expect(await screen.findByText(/Talos v1\.14\.2 is available/)).toBeInTheDocument()
    // More than one minor behind: the first run is not the newest version.
    expect(screen.getByText(/2 runs; the first installs v1\.13\.9/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Release notes of v1\.13\.9/ })).toHaveAttribute(
      'href',
      'https://github.com/siderolabs/talos/releases/tag/v1.13.9',
    )
    expect(onPlan).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: /Plan the update to v1\.13\.9/ }))
    expect(onPlan).toHaveBeenCalledWith('v1.13.9')
  })

  it('says why there is nothing to offer instead of staying silent', async () => {
    stub({
      '/upgrade/talos/check': {
        checked_at: '',
        notice: '',
        clusters: [
          {
            cluster: 'c-1',
            name: 'homelab',
            current: ['v1.14.2'],
            newest: 'v1.14.2',
            available: false,
            reason: 'Every node already runs the newest release this installation supports.',
          },
        ],
      },
    })
    inClient(<TalosUpdateBanner cluster="c-1" onPlan={vi.fn()} planning={false} />)

    expect(await screen.findByText(/already runs the newest release/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Plan the update/ })).not.toBeInTheDocument()
  })

  it('asks the server again when told to check', async () => {
    const user = userEvent.setup()
    const calls: string[] = []
    stub({ '/upgrade/talos/check': behind }, calls)
    inClient(<TalosUpdateBanner cluster="c-1" onPlan={vi.fn()} planning={false} />)
    await screen.findByText(/is available/)
    const before = calls.filter((c) => c.includes('talos/check')).length

    await user.click(screen.getByRole('button', { name: /Check for a Talos update/ }))
    await vi.waitFor(() =>
      expect(calls.filter((c) => c.includes('talos/check')).length).toBeGreaterThan(before),
    )
  })

  const none = {
    snapshot: { cluster: 'c-1', present: false, fresh: false, max_age_minutes: 60 },
    available: true,
  }
  const fresh = {
    snapshot: {
      cluster: 'c-1',
      present: true,
      fresh: true,
      taken_at: '2026-10-06T12:00:00Z',
      valid_until: '2026-10-06T13:00:00Z',
      bytes: 5_242_880,
      max_age_minutes: 60,
    },
    available: true,
  }

  it('says a snapshot is required first, and takes one on request', async () => {
    const user = userEvent.setup()
    const calls: string[] = []
    stub({ '/upgrade/snapshot': none }, calls)
    inClient(<SafetySnapshotStep cluster="c-1" />)

    expect(await screen.findByText(/snapshot is required first/)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /Take the snapshot now/ }))
    await vi.waitFor(() => expect(calls).toContain('POST /api/v1/clusters/c-1/upgrade/snapshot'))
  })

  it('shows a fresh snapshot with its size and how long it is good for', async () => {
    stub({ '/upgrade/snapshot': fresh })
    inClient(<SafetySnapshotStep cluster="c-1" />)
    expect(await screen.findByText(/snapshot is fresh/)).toBeInTheDocument()
    expect(screen.getByText(/5\.0 MiB/)).toBeInTheDocument()
    expect(screen.getByText(/not in backups/)).toBeInTheDocument()
  })

  it('offers no way to take one on an instance that has nowhere to keep it', async () => {
    stub({ '/upgrade/snapshot': { ...none, available: false } })
    inClient(<SafetySnapshotStep cluster="c-1" />)
    expect(await screen.findByText(/nowhere to keep a snapshot/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Take the snapshot now/ })).toBeDisabled()
  })

  const plan = {
    cluster: 'c-1',
    to: 'v1.14.2',
    chain: [],
    strand: { blocked: false, sentence: 'Kubernetes stays supported.', remedy: '' },
    gate_preview: {
      ok: true,
      reason: '',
      input: { members: [], voting: 3, statuses: {}, unreachable: [], alarms: [], max_raft_lag: 0 },
    },
    nodes: [],
    blocked: false,
    block_reason: '',
  } as unknown as UpgradePlan

  function planView(snapshotRequired: boolean) {
    inClient(
      <PlanView
        plan={plan}
        cluster="c-1"
        snapshotRequired={snapshotRequired}
        clusterName="homelab"
        typed="homelab"
        onTyped={vi.fn()}
        onStart={vi.fn()}
        starting={false}
        startError={null}
        started={false}
      />,
    )
  }

  it('does not let a Talos upgrade start on a typed name alone', async () => {
    stub({ '/upgrade/snapshot': none })
    planView(true)
    await screen.findByText(/snapshot is required first/)
    expect(screen.getByRole('button', { name: /Start the rolling upgrade/ })).toBeDisabled()
  })

  it('lets it start once the snapshot is fresh', async () => {
    stub({ '/upgrade/snapshot': fresh })
    planView(true)
    await screen.findByText(/snapshot is fresh/)
    expect(screen.getByRole('button', { name: /Start the rolling upgrade/ })).toBeEnabled()
  })

  it('does not hold a Kubernetes upgrade for one', async () => {
    stub({ '/upgrade/snapshot': none })
    planView(false)
    expect(screen.queryByText(/snapshot is required/)).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Start the rolling upgrade/ })).toBeEnabled()
  })
})
