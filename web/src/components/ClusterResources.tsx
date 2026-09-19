import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { api, type ClusterResource } from '@/api'
import { DataTable } from '@/components/DataTable'
import { Problem } from '@/components/Problem'
import { Button } from '@/components/ui/button'

/** Which apiVersion each kind lives at, for the delete. */
const API_VERSION: Record<string, string> = {
  ConfigMap: 'v1',
  Secret: 'v1',
  PersistentVolumeClaim: 'v1',
  Ingress: 'networking.k8s.io/v1',
  HorizontalPodAutoscaler: 'autoscaling/v2',
  PodDisruptionBudget: 'policy/v1',
}

/**
 * The objects beside the workloads (2026-09-19).
 *
 * Each kind here answers a question the workload list cannot, and the ones that
 * are the REASON something else is stuck are marked: a Pending claim is why a
 * StatefulSet will not start, an Ingress with no address is why a URL does not
 * work, a disruption budget allowing nothing is why a drain refuses.
 *
 * Those come first. A list sorted by name buries the finding among forty
 * ConfigMaps, and the operator is here because something is wrong.
 */
export function ClusterResources({
  clusterID,
  namespace,
}: {
  clusterID: string
  namespace: string
}) {
  const resources = useQuery({
    queryKey: ['kubernetes', 'resources', clusterID, namespace],
    queryFn: () => api.kubernetes.resources(clusterID, namespace),
    refetchInterval: 30_000,
  })

  if (resources.error) return <Problem error={resources.error} />
  if (!resources.data) return <p className="text-muted-foreground text-sm">Reading…</p>

  // The ones that are stuck first. Everything else keeps the server's order.
  const rows = [...resources.data].sort((a, b) => Number(a.healthy) - Number(b.healthy))

  return (
    <DataTable
      label="Cluster resources"
      phone="rows"
      rows={rows}
      keyOf={(r) => `${r.kind}/${r.namespace}/${r.name}`}
      empty={
        <>
          The API server answered, and there is nothing of this sort
          {namespace === '' ? ' in this cluster' : ` in ${namespace}`}.
        </>
      }
      columns={[
        {
          key: 'name',
          label: 'Object',
          role: 'identity',
          render: (r) => (
            <span className={r.healthy ? undefined : 'text-amber-700 dark:text-amber-300'}>
              <span className="text-xs">{r.kind}</span>{' '}
              <span className="break-all font-mono text-xs">
                {r.namespace}/{r.name}
              </span>
            </span>
          ),
        },
        { key: 'summary', label: 'State', render: (r) => r.summary },
        {
          key: 'detail',
          label: 'Detail',
          role: 'detail',
          render: (r) => <span className="break-words">{r.detail || '—'}</span>,
        },
        {
          key: 'actions',
          label: 'Actions',
          role: 'actions',
          render: (r) => <RemoveObject clusterID={clusterID} resource={r} />,
        },
      ]}
    />
  )
}

/**
 * Removing one object, with the name typed back.
 *
 * Deleting is the one thing on this screen that cannot be undone by doing it
 * again, and the object is picked from a list where the rows look alike. So the
 * confirmation is the NAME rather than a yes: it is the only form of
 * confirmation that cannot be given by clicking twice in the wrong row.
 */
function RemoveObject({ clusterID, resource }: { clusterID: string; resource: ClusterResource }) {
  const queryClient = useQueryClient()
  const [asking, setAsking] = useState(false)
  const [typed, setTyped] = useState('')

  const remove = useMutation({
    mutationFn: () =>
      api.kubernetes.deleteObject(clusterID, {
        api_version: API_VERSION[resource.kind] ?? 'v1',
        kind: resource.kind,
        namespace: resource.namespace,
        name: resource.name,
      }),
    onSuccess: () => {
      setAsking(false)
      setTyped('')
      void queryClient.invalidateQueries({ queryKey: ['kubernetes'] })
    },
  })

  if (!asking) {
    return (
      <Button
        type="button"
        size="sm"
        variant="outline"
        className="max-md:h-11"
        onClick={() => setAsking(true)}
      >
        Remove
      </Button>
    )
  }

  return (
    <div className="space-y-1">
      <p className="text-sm">
        Type <span className="font-mono">{resource.name}</span> to remove it. Nothing puts it back.
      </p>
      <div className="flex flex-wrap gap-2 max-md:flex-col">
        <input
          className="h-8 w-56 rounded-lg border border-input bg-transparent px-2.5 text-base md:text-sm max-md:h-11 max-md:w-full"
          aria-label={`Type ${resource.name} to confirm`}
          value={typed}
          onChange={(event) => setTyped(event.target.value)}
        />
        <Button
          type="button"
          size="sm"
          variant="destructive"
          className="max-md:h-11"
          disabled={typed !== resource.name || remove.isPending}
          onClick={() => remove.mutate()}
        >
          {remove.isPending ? 'Removing…' : 'Remove it'}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="max-md:h-11"
          onClick={() => {
            setAsking(false)
            setTyped('')
          }}
        >
          Cancel
        </Button>
      </div>
      {remove.error ? <Problem error={remove.error} /> : null}
    </div>
  )
}
