import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { EtcdMemberList, GateVerdict } from '@/api'
import { GatePanel, MemberTable } from '@/routes/upgrades'

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
