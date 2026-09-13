import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { EtcdMemberList, GateVerdict } from '@/api'
import { EtcdPanel, GatePanel, MemberTable } from '@/routes/upgrades'

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
