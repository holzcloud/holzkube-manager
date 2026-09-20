import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { api } from '@/api'
import { DataTable } from '@/components/DataTable'
import { Problem } from '@/components/Problem'
import { Button } from '@/components/ui/button'

/**
 * Who may do what in this cluster (2026-09-20).
 *
 * # Why this is not a list of roles
 *
 * `kubectl get rolebindings` gives a list of names. Every question somebody has
 * about RBAC is about whether an arrangement *works*, and each of those is
 * answered by two objects disagreeing — which nothing in a cluster will tell them,
 * because RBAC has no referential integrity on purpose, so that a binding may be
 * written before its role.
 *
 *   - A binding naming a role that does not exist grants nothing at all, and looks
 *     exactly like one that grants everything it was written for.
 *   - A binding naming a ServiceAccount that was never created, or was deleted
 *     with the binding left behind, is the same shape of nothing.
 *   - "Who is an administrator here?" is not a field. It is every subject that
 *     reaches wildcard verbs on wildcard resources through some binding.
 *
 * # Administrators first, because that is the question
 *
 * And it is computed from the rules rather than from the name `cluster-admin`,
 * which would miss every hand-written role with the same power under a name
 * nobody recognises — the usual way somebody grants it by accident.
 *
 * # The built-in roles fold away
 *
 * Kubernetes ships about seventy, identical on every cluster, and they are never
 * what somebody is looking for. They are behind a button rather than filtered out
 * for good: hiding them silently would be a screen that cannot answer "does this
 * cluster still have the standard roles".
 */
export function AccessControl({ clusterID, namespace }: { clusterID: string; namespace: string }) {
  const [showBuiltIn, setShowBuiltIn] = useState(false)

  const access = useQuery({
    queryKey: ['kubernetes', 'access', clusterID, namespace],
    queryFn: () => api.kubernetes.access(clusterID, namespace),
    enabled: clusterID !== '',
    refetchInterval: 60_000,
  })

  if (access.error) return <Problem error={access.error} />
  if (!access.data) return <p className="text-muted-foreground text-sm">Reading…</p>

  const { bindings, roles, accounts, administrators, notice } = access.data

  // Broken first, then administrative: both are findings, and a list sorted by
  // name makes somebody read all of it to find them.
  const sorted = [...bindings].sort(
    (a, b) =>
      Number(a.healthy) - Number(b.healthy) || Number(b.administrative) - Number(a.administrative),
  )
  const shown = showBuiltIn ? roles : roles.filter((role) => !role.built_in)
  const hidden = roles.length - shown.length

  return (
    <div className="space-y-4">
      <div className="rounded-md border p-3">
        <p className="font-medium text-sm">Administrators of this cluster</p>
        {administrators.length === 0 ? (
          <p className="mt-1 text-muted-foreground text-sm">
            No binding here grants every verb on every resource. That is an unusual and good answer;
            the cluster’s own certificate holders are outside RBAC and are not in it.
          </p>
        ) : (
          <>
            <p className="mt-1 break-all font-mono text-xs">{administrators.join(', ')}</p>
            <p className="mt-1 text-muted-foreground text-xs">
              Everything each of these may do, they may do to anything in the cluster. This is not a
              field in Kubernetes: it is worked out from the rules, so a role with wildcard verbs on
              wildcard resources counts whatever it is called.
            </p>
          </>
        )}
      </div>

      <DataTable
        label="Bindings"
        rows={sorted}
        keyOf={(binding) => `${binding.kind}/${binding.namespace}/${binding.name}`}
        empty={
          <>
            The API server answered, and nothing grants any role
            {namespace === '' ? ' in this cluster' : ` in ${namespace}`}.
          </>
        }
        columns={[
          {
            key: 'name',
            label: 'Binding',
            role: 'identity',
            render: (binding) => (
              <span className="break-all font-mono text-xs">
                {binding.namespace === '' ? binding.name : `${binding.namespace}/${binding.name}`}
                <span className="ml-1 font-sans text-muted-foreground">
                  {binding.kind === 'ClusterRoleBinding' ? '(cluster-wide)' : ''}
                </span>
              </span>
            ),
          },
          {
            key: 'grants',
            label: 'Grants',
            render: (binding) => (
              <span
                className={
                  binding.role_exists
                    ? binding.administrative
                      ? 'break-all font-mono text-red-700 text-xs dark:text-red-300'
                      : 'break-all font-mono text-xs'
                    : 'break-all font-mono text-red-700 text-xs dark:text-red-300'
                }
              >
                {binding.role_name}
                {binding.role_exists ? '' : ' (missing)'}
              </span>
            ),
          },
          {
            key: 'to',
            label: 'To',
            render: (binding) =>
              binding.subjects.length === 0 ? (
                <span className="text-red-700 text-xs dark:text-red-300">nobody</span>
              ) : (
                <span className="break-all font-mono text-xs">
                  {binding.subjects
                    .map(
                      (subject) =>
                        `${subject.namespace === '' ? subject.name : `${subject.namespace}/${subject.name}`}${
                          // Only a ServiceAccount can be checked; saying this of
                          // a User would teach that the marking is noise.
                          subject.checkable && !subject.exists ? ' (missing)' : ''
                        }`,
                    )
                    .join(', ')}
                </span>
              ),
          },
          {
            key: 'why',
            label: 'What it does',
            render: (binding) => (
              <span
                className={
                  binding.healthy
                    ? 'text-muted-foreground text-xs'
                    : 'text-amber-700 text-xs dark:text-amber-300'
                }
              >
                {binding.summary}
              </span>
            ),
          },
        ]}
      />

      <DataTable
        label="Service accounts"
        rows={accounts}
        keyOf={(account) => `${account.namespace}/${account.name}`}
        empty="The API server answered, and there are no service accounts here — which cannot be, since every namespace gets a default one."
        columns={[
          {
            key: 'name',
            label: 'Account',
            role: 'identity',
            render: (account) => (
              <span className="break-all font-mono text-xs">
                {account.namespace}/{account.name}
              </span>
            ),
          },
          {
            key: 'used',
            label: 'Run by',
            render: (account) =>
              account.used_by.length === 0 ? (
                <span className="text-muted-foreground text-xs">nothing</span>
              ) : (
                <span className="break-all font-mono text-xs">{account.used_by.join(', ')}</span>
              ),
          },
          {
            key: 'bindings',
            label: 'Grants',
            render: (account) => (
              <span
                className={
                  account.administrative
                    ? 'text-red-700 text-xs tabular-nums dark:text-red-300'
                    : 'text-xs tabular-nums'
                }
              >
                {account.bindings}
                {account.administrative ? ' (administrative)' : ''}
              </span>
            ),
          },
          {
            key: 'why',
            label: 'Note',
            role: 'detail',
            render: (account) =>
              account.notice === '' ? (
                '—'
              ) : (
                <span className="text-muted-foreground text-xs">{account.notice}</span>
              ),
          },
        ]}
      />

      <DataTable
        label="Roles"
        phone="rows"
        rows={shown}
        keyOf={(role) => `${role.kind}/${role.namespace}/${role.name}`}
        empty={
          showBuiltIn ? (
            <>The API server answered, and the cluster has no roles at all.</>
          ) : (
            <>
              Every role in this cluster is one Kubernetes ships. Nobody has written one — show them
              below to see the standard set.
            </>
          )
        }
        columns={[
          {
            key: 'name',
            label: 'Role',
            role: 'identity',
            render: (role) => (
              <span className="break-all font-mono text-xs">
                {role.namespace === '' ? role.name : `${role.namespace}/${role.name}`}
                <span className="ml-1 font-sans text-muted-foreground">
                  {role.kind === 'ClusterRole' ? '(cluster-wide)' : ''}
                </span>
              </span>
            ),
          },
          {
            key: 'rules',
            label: 'Permits',
            render: (role) => (
              <span
                className={
                  role.administrative ? 'text-red-700 text-xs dark:text-red-300' : 'text-xs'
                }
              >
                {role.rules.join('; ') || 'nothing'}
              </span>
            ),
          },
          {
            key: 'bound',
            label: 'Granted by',
            role: 'detail',
            render: (role) =>
              role.bound === 0 ? (
                // Ordinary for a built-in role, and worth seeing for one
                // somebody wrote: it permits nothing because nothing grants it.
                <span className="text-muted-foreground text-xs">no binding</span>
              ) : (
                <span className="text-xs tabular-nums">
                  {role.bound} binding{role.bound === 1 ? '' : 's'}
                </span>
              ),
          },
        ]}
      />

      {hidden > 0 && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="max-md:h-11 max-md:w-full"
          onClick={() => setShowBuiltIn(true)}
        >
          Show the {hidden} roles Kubernetes ships
        </Button>
      )}
      {showBuiltIn && roles.some((role) => role.built_in) && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="max-md:h-11 max-md:w-full"
          onClick={() => setShowBuiltIn(false)}
        >
          Hide the roles Kubernetes ships
        </Button>
      )}

      {/* Why a binding can point at nothing and why administrative is about the
          rules. Said once, by the server, so both halves agree. */}
      <p className="text-muted-foreground text-xs">{notice}</p>
    </div>
  )
}
