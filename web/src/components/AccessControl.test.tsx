import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { type AccessControl as Answer, api, type BindingSummary } from '@/api'
import { AccessControl } from '@/components/AccessControl'

/**
 * Who may do what.
 *
 * The rows worth testing are the ones nothing in a cluster will report: a binding
 * pointing at a role that is not there, a subject that was never created, and who
 * is an administrator — which is not a field anywhere in Kubernetes.
 */

function wrap(node: ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

const adminBinding: BindingSummary = {
  kind: 'ClusterRoleBinding',
  namespace: '',
  name: 'platform-team',
  role_kind: 'ClusterRole',
  role_name: 'platform-operator',
  role_exists: true,
  subjects: [
    { kind: 'ServiceAccount', namespace: 'ci', name: 'deployer', checkable: true, exists: true },
    { kind: 'User', namespace: '', name: 'holz@holzcloud.ch', checkable: false, exists: false },
  ],
  administrative: true,
  summary: 'Grants platform-operator, which permits every verb on every resource.',
  healthy: true,
  created_at: '',
}

const answer: Answer = {
  bindings: [
    adminBinding,
    {
      kind: 'RoleBinding',
      namespace: 'web',
      name: 'web-readers',
      role_kind: 'Role',
      role_name: 'pod-readr',
      role_exists: false,
      subjects: [
        { kind: 'ServiceAccount', namespace: 'web', name: 'reader', checkable: true, exists: true },
      ],
      administrative: false,
      summary: 'It names Role pod-readr, which does not exist, so it grants nothing at all.',
      healthy: false,
      created_at: '',
    },
  ],
  roles: [
    {
      kind: 'ClusterRole',
      namespace: '',
      name: 'platform-operator',
      rules: ['everything on every resource'],
      administrative: true,
      bound: 1,
      built_in: false,
      created_at: '',
    },
    {
      kind: 'ClusterRole',
      namespace: '',
      name: 'cluster-admin',
      rules: ['everything on every resource'],
      administrative: true,
      bound: 0,
      built_in: true,
      created_at: '',
    },
  ],
  accounts: [
    {
      namespace: 'default',
      name: 'default',
      used_by: ['default/bare'],
      bindings: 1,
      administrative: true,
      notice:
        'This is the account every pod in default that names none runs as, and it is bound to a role permitting every verb on every resource.',
      created_at: '',
    },
  ],
  administrators: ['ServiceAccount ci/deployer', 'User holz@holzcloud.ch'],
  notice: 'A binding that names a role which does not exist grants nothing.',
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('who may do what', () => {
  it('answers who the administrators are, because that is not a field', async () => {
    vi.spyOn(api.kubernetes, 'access').mockResolvedValue(answer)

    wrap(<AccessControl clusterID="c-1" namespace="" />)

    expect(
      await screen.findByText('ServiceAccount ci/deployer, User holz@holzcloud.ch'),
    ).toBeInTheDocument()
  })

  it('says so when nothing grants everything', async () => {
    vi.spyOn(api.kubernetes, 'access').mockResolvedValue({ ...answer, administrators: [] })

    wrap(<AccessControl clusterID="c-1" namespace="" />)

    // An empty answer is a claim, and this one needs its caveat: the cluster's
    // own certificate holders are outside RBAC entirely.
    expect(await screen.findByText(/outside RBAC and are not in it/)).toBeInTheDocument()
  })

  it('puts a binding that grants nothing first, and marks the missing role', async () => {
    vi.spyOn(api.kubernetes, 'access').mockResolvedValue(answer)

    wrap(<AccessControl clusterID="c-1" namespace="" />)

    const rows = await screen.findAllByText(/platform-team|web\/web-readers/)
    expect(rows[0]).toHaveTextContent('web/web-readers')
    expect(screen.getByText('pod-readr (missing)')).toBeInTheDocument()
  })

  it('marks a missing service account and leaves a user alone', async () => {
    vi.spyOn(api.kubernetes, 'access').mockResolvedValue({
      ...answer,
      bindings: [
        {
          ...adminBinding,
          subjects: [
            {
              kind: 'ServiceAccount',
              namespace: 'web',
              name: 'gone',
              checkable: true,
              exists: false,
            },
            {
              kind: 'User',
              namespace: '',
              name: 'somebody@example.invalid',
              checkable: false,
              exists: false,
            },
          ],
        },
      ],
    })

    wrap(<AccessControl clusterID="c-1" namespace="" />)

    // The ServiceAccount is marked and the User is not, although both carry
    // exists: false — a screen that marked the User too would teach that the
    // marking is noise.
    expect(await screen.findByText(/web\/gone \(missing\)/)).toBeInTheDocument()
    expect(screen.queryByText(/somebody@example.invalid \(missing\)/)).not.toBeInTheDocument()
  })

  it('hides the roles Kubernetes ships behind a button rather than filtering them away', async () => {
    vi.spyOn(api.kubernetes, 'access').mockResolvedValue(answer)

    wrap(<AccessControl clusterID="c-1" namespace="" />)

    // Hidden, but the screen says how many and can still answer "does this
    // cluster have the standard roles".
    expect(await screen.findByRole('button', { name: /Show the 1 roles/ })).toBeInTheDocument()
    expect(screen.queryByText(/cluster-admin/)).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: /Show the 1 roles/ }))
    expect(await screen.findByText(/cluster-admin/)).toBeInTheDocument()
  })

  it('says what runs as the default account when it is administrative', async () => {
    vi.spyOn(api.kubernetes, 'access').mockResolvedValue(answer)

    wrap(<AccessControl clusterID="c-1" namespace="" />)

    expect(await screen.findByText('default/bare')).toBeInTheDocument()
    expect(screen.getByText('1 (administrative)')).toBeInTheDocument()
  })
})
