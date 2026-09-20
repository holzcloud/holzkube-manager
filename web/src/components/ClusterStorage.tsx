import { useQuery } from '@tanstack/react-query'
import { api } from '@/api'
import { DataTable } from '@/components/DataTable'
import { Problem } from '@/components/Problem'

/**
 * The cluster's storage (2026-09-20).
 *
 * # Why this exists beside the claim list
 *
 * The resources screen lists PersistentVolumeClaims, which is one half of a
 * two-sided arrangement — and the half that cannot answer the questions somebody
 * has when storage is wrong. Every row here carries something no claim knows:
 *
 *   - Whether deleting the claim destroys the data. That is the VOLUME's reclaim
 *     policy, and `Retain` versus `Delete` is the whole difference.
 *   - Who is using it. Only the pods know, and finding out otherwise means
 *     reading every pod's volume list by hand.
 *   - Why a Pending claim is pending. Usually a fact about its StorageClass, and
 *     two of the three causes are not faults at all.
 *
 * # Released is the row worth the screen
 *
 * A volume whose claim is gone and whose data is still on the disk. It is
 * invisible in every namespace view — a PersistentVolume is cluster-scoped — it
 * counts against nothing, and it is either wasted storage or exactly the data
 * somebody needs back. Both readings are said.
 *
 * # Nothing here deletes
 *
 * Deleting a `Retain` volume is how data goes for good. The object-delete route
 * already does it for anybody who means it, with the name typed out; a button
 * here would be the one in this product whose mistake cannot be undone at all.
 */
export function ClusterStorage({ clusterID, namespace }: { clusterID: string; namespace: string }) {
  const storage = useQuery({
    queryKey: ['kubernetes', 'storage', clusterID, namespace],
    queryFn: () => api.kubernetes.storage(clusterID, namespace),
    enabled: clusterID !== '',
    refetchInterval: 30_000,
  })

  if (storage.error) return <Problem error={storage.error} />
  if (!storage.data) return <p className="text-muted-foreground text-sm">Reading…</p>

  const { volumes, claims, classes, notice } = storage.data

  return (
    <div className="space-y-4">
      <DataTable
        label="Claims"
        rows={claims}
        keyOf={(claim) => `${claim.namespace}/${claim.name}`}
        empty={
          <>
            The API server answered, and nothing claims storage
            {namespace === '' ? ' in this cluster' : ` in ${namespace}`}.
          </>
        }
        columns={[
          {
            key: 'name',
            label: 'Claim',
            role: 'identity',
            render: (claim) => (
              <span className="break-all font-mono text-xs">
                {claim.namespace}/{claim.name}
              </span>
            ),
          },
          {
            key: 'phase',
            label: 'Phase',
            render: (claim) => (
              <span
                className={
                  claim.phase === 'Bound' ? undefined : 'text-amber-700 dark:text-amber-300'
                }
              >
                {claim.phase}
              </span>
            ),
          },
          {
            key: 'size',
            label: 'Size',
            render: (claim) => (
              <span className="tabular-nums text-xs">
                {claim.capacity || claim.requested || '—'}
                {claim.capacity !== '' &&
                claim.requested !== '' &&
                claim.capacity !== claim.requested
                  ? ` (asked for ${claim.requested})`
                  : ''}
              </span>
            ),
          },
          {
            key: 'used',
            label: 'Used by',
            // The question a claim cannot answer about itself.
            render: (claim) =>
              claim.used_by.length === 0 ? (
                <span className="text-muted-foreground text-xs">nothing</span>
              ) : (
                <span className="break-all font-mono text-xs">{claim.used_by.join(', ')}</span>
              ),
          },
          {
            key: 'class',
            label: 'Class',
            role: 'detail',
            render: (claim) => (
              <span className="text-xs">
                {claim.storage_class || '—'}
                {claim.expandable ? ' (can grow)' : ''}
              </span>
            ),
          },
          {
            key: 'volume',
            label: 'Volume',
            role: 'detail',
            render: (claim) => (
              <span className="break-all font-mono text-xs">{claim.volume || '—'}</span>
            ),
          },
          {
            key: 'why',
            label: 'Note',
            role: 'detail',
            render: (claim) =>
              claim.notice === '' ? (
                '—'
              ) : (
                <span className="text-muted-foreground text-xs">{claim.notice}</span>
              ),
          },
        ]}
      />

      <DataTable
        label="Volumes"
        rows={volumes}
        keyOf={(volume) => volume.name}
        empty="The API server answered, and the cluster has no persistent volumes."
        columns={[
          {
            key: 'name',
            label: 'Volume',
            role: 'identity',
            render: (volume) => <span className="break-all font-mono text-xs">{volume.name}</span>,
          },
          {
            key: 'phase',
            label: 'Phase',
            render: (volume) => (
              <span
                className={
                  volume.phase === 'Bound' ? undefined : 'text-amber-700 dark:text-amber-300'
                }
              >
                {volume.phase}
              </span>
            ),
          },
          {
            key: 'capacity',
            label: 'Capacity',
            render: (volume) => <span className="tabular-nums text-xs">{volume.capacity}</span>,
          },
          {
            key: 'reclaim',
            label: 'On delete',
            // Said as what happens rather than as the API's word, because
            // "Delete"/"Retain" in a column headed "Reclaim policy" is the most
            // consequential field in this table and the least readable.
            render: (volume) =>
              volume.reclaim_policy === 'Retain' ? (
                <span className="text-xs">data kept</span>
              ) : volume.reclaim_policy === 'Delete' ? (
                <span className="text-xs text-red-700 dark:text-red-300">data destroyed</span>
              ) : (
                <span className="text-xs">{volume.reclaim_policy || '—'}</span>
              ),
          },
          {
            key: 'claim',
            label: 'Claimed by',
            render: (volume) =>
              volume.claim === '' ? (
                <span className="text-muted-foreground text-xs">nothing</span>
              ) : (
                <span className="break-all font-mono text-xs">{volume.claim}</span>
              ),
          },
          {
            key: 'driver',
            label: 'Backed by',
            role: 'detail',
            render: (volume) => (
              <span className="break-all font-mono text-xs">{volume.driver || '—'}</span>
            ),
          },
          {
            key: 'modes',
            label: 'Access',
            role: 'detail',
            render: (volume) => (
              <span className="text-xs">{volume.access_modes.join(', ') || '—'}</span>
            ),
          },
          {
            key: 'why',
            label: 'Note',
            role: 'detail',
            render: (volume) =>
              volume.notice === '' ? (
                '—'
              ) : (
                <span className="text-muted-foreground text-xs">{volume.notice}</span>
              ),
          },
        ]}
      />

      <DataTable
        label="Storage classes"
        phone="rows"
        rows={classes}
        keyOf={(storageClass) => storageClass.name}
        empty="The API server answered, and the cluster has no storage classes: every claim in it will stay Pending."
        columns={[
          {
            key: 'name',
            label: 'Class',
            role: 'identity',
            render: (storageClass) => (
              <span className="break-all font-mono text-xs">
                {storageClass.name}
                {storageClass.default ? (
                  <span className="ml-1 font-sans text-muted-foreground">(default)</span>
                ) : null}
              </span>
            ),
          },
          {
            key: 'provisioner',
            label: 'Provisioner',
            render: (storageClass) => (
              <span className="break-all font-mono text-xs">{storageClass.provisioner}</span>
            ),
          },
          {
            key: 'binding',
            label: 'Binds',
            role: 'detail',
            render: (storageClass) =>
              storageClass.binding_mode === 'WaitForFirstConsumer' ? (
                // The one that makes a claim look broken while working.
                <span className="text-xs">when a pod needs it</span>
              ) : (
                <span className="text-xs">immediately</span>
              ),
          },
          {
            key: 'expand',
            label: 'Can grow',
            role: 'detail',
            render: (storageClass) => (storageClass.allows_expansion ? 'yes' : 'no'),
          },
        ]}
      />

      {/* What a capacity here IS and is not. Nothing in the Kubernetes API
          reports how full a filesystem is; only something inside the pod can. */}
      <p className="text-muted-foreground text-xs">{notice}</p>
    </div>
  )
}
