import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { api } from '@/api'
import { Problem } from '@/components/Problem'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

/**
 * Whose name this product's Kubernetes requests arrive under (2026-09-19).
 *
 * Today they arrive as `holzkube-manager` in `system:masters`, and that has two
 * consequences worth a panel of its own. The cluster's audit log records the
 * certificate's common name, so it says this product scaled a deployment — for
 * every operator, for ever — and cannot answer the one question an audit log
 * exists for. And `system:masters` bypasses RBAC, so a cluster cannot express
 * "this person may restart pods in web and nothing else".
 *
 * # The preview is the whole design
 *
 * Setting this to a name the cluster's RBAC has never heard of breaks every
 * Kubernetes screen at once, and the failure looks like the product is broken
 * rather than like a setting is wrong. So the identity is checked against the
 * CLUSTER before it can be stored, and the check is the cluster's own answer —
 * `SelfSubjectAccessReview` per permission this product actually uses.
 *
 * And the rule that makes it worth having: a refusal is never retried as the
 * admin. The panel says so, because somebody who expects a silent fallback would
 * read a refused screen as a bug.
 */
export function ActAs({ clusterID }: { clusterID: string }) {
  const queryClient = useQueryClient()
  const fieldID = useId()
  const [candidate, setCandidate] = useState('')
  const [preview, setPreview] = useState<string | null>(null)

  const current = useQuery({
    queryKey: ['kubernetes', 'identity', clusterID],
    queryFn: () => api.kubernetes.identity(clusterID),
  })

  const checked = useQuery({
    queryKey: ['kubernetes', 'identity', clusterID, 'preview', preview],
    queryFn: () => api.kubernetes.identity(clusterID, { as: preview ?? '' }),
    enabled: preview !== null && preview !== '',
  })

  const save = useMutation({
    mutationFn: (user: string) => api.kubernetes.setIdentity(clusterID, user),
    onSuccess: () => {
      setPreview(null)
      setCandidate('')
      void queryClient.invalidateQueries({ queryKey: ['kubernetes'] })
    },
  })

  return (
    <div className="space-y-3">
      <div className="space-y-1">
        <p className="text-sm">
          Acting as <span className="font-medium">{current.data?.describes ?? '…'}</span>
        </p>
        {current.data && current.data.user === '' && (
          <p className="text-muted-foreground text-sm">
            Every request arrives under this product's own certificate, in{' '}
            <code>system:masters</code>. Your cluster's audit log records that name rather than
            yours, and its RBAC cannot restrict what this does.
          </p>
        )}
        {current.data && current.data.user !== '' && current.data.missing > 0 && (
          <p className="text-amber-700 text-sm dark:text-amber-300">
            The cluster refuses {current.data.missing} of the things this product does as that
            identity. Those screens will be refused, and this will not retry them as an
            administrator.
          </p>
        )}
        {current.error ? <Problem error={current.error} /> : null}
      </div>

      <div className="flex flex-wrap items-end gap-2">
        <div className="space-y-1">
          <label htmlFor={fieldID} className="text-muted-foreground text-xs">
            Act as
          </label>
          <Input
            id={fieldID}
            className="w-72 max-md:h-11 max-md:w-full"
            placeholder="the name your cluster knows you by"
            value={candidate}
            onChange={(event) => {
              setCandidate(event.target.value)
              setPreview(null)
            }}
          />
        </div>
        <Button
          type="button"
          variant="outline"
          className="max-md:h-11"
          disabled={candidate.trim() === ''}
          onClick={() => setPreview(candidate.trim())}
        >
          Check it
        </Button>
        {/* Only after a check, and only for the text that was checked: storing
            an unchecked name is how somebody locks themselves out of their own
            cluster's screens. */}
        <Button
          type="button"
          className="max-md:h-11"
          disabled={preview !== candidate.trim() || !checked.data || save.isPending}
          onClick={() => save.mutate(candidate.trim())}
        >
          Use it
        </Button>
      </div>

      <p className="text-muted-foreground text-xs">
        This is whatever your API server calls you — for OIDC usually your email, with the prefix
        its <code>--oidc-username-prefix</code> adds. This product cannot work it out, because it is
        your cluster's authentication rather than this product's.
      </p>

      {checked.error ? <Problem error={checked.error} /> : null}

      {checked.data && preview === candidate.trim() && (
        <div className="space-y-2">
          <p className="text-sm">
            {checked.data.missing === 0 ? (
              <>
                The cluster allows <span className="font-medium">{checked.data.describes}</span>{' '}
                everything this product does.
              </>
            ) : (
              <>
                The cluster refuses {checked.data.missing} of {checked.data.permissions.length}{' '}
                things as <span className="font-medium">{checked.data.describes}</span>.
              </>
            )}
          </p>
          <ul className="space-y-1 text-sm">
            {checked.data.permissions
              .filter((p) => !p.allowed)
              .map((p) => (
                <li key={`${p.verb}-${p.resource}`}>
                  <span className="font-mono text-xs">
                    {p.verb} {p.resource}
                  </span>
                  {p.reason !== '' && <span className="text-muted-foreground"> — {p.reason}</span>}
                </li>
              ))}
          </ul>
          <p className="text-muted-foreground text-xs">{checked.data.notice}</p>
        </div>
      )}

      {current.data && current.data.user !== '' && (
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="max-md:h-11"
          disabled={save.isPending}
          onClick={() => save.mutate('')}
        >
          Go back to this product's own certificate
        </Button>
      )}

      {save.error ? <Problem error={save.error} /> : null}
    </div>
  )
}
