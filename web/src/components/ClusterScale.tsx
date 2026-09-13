import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { api } from '@/api'
import { Problem } from '@/components/Problem'
import { Button } from '@/components/ui/button'

/**
 * What changing this cluster's size would mean.
 *
 * Both halves of scaling already existed — adding a node is provisioning,
 * removing one is a button on that node's page — and neither could answer the
 * question that comes before them: given this cluster, which of these nodes can
 * go? An operator had to hold etcd's membership, the inventory and the
 * arithmetic of a majority in their head at once, and the arithmetic is the
 * part that goes wrong in a direction that feels like caution. Four voting
 * members feel safer than three and tolerate exactly the same single loss.
 *
 * Every sentence below comes from the server, refusals included. This screen
 * decides nothing: a refusal shown here is the one the removal route would
 * produce, because it is produced by the same function. A screen with its own
 * account of the rule looks right until somebody clicks.
 *
 * It is asked for rather than loaded with the page. Answering costs one etcd
 * read against a control-plane node, and a cluster list that made one per card
 * every thirty seconds would be a fleet overview quietly polling every cluster
 * it can see.
 */
export function ClusterScalePanel({ clusterID }: { clusterID: string }) {
  const [asked, setAsked] = useState(false)

  const scale = useQuery({
    queryKey: ['cluster-scale', clusterID],
    queryFn: () => api.scale.plan(clusterID),
    enabled: asked,
  })

  if (!asked) {
    return (
      <Button type="button" variant="outline" size="sm" onClick={() => setAsked(true)}>
        What can this cluster spare?
      </Button>
    )
  }
  if (scale.error) {
    return <Problem error={scale.error} />
  }
  if (!scale.data) {
    return <p className="text-xs text-muted-foreground">Reading etcd…</p>
  }

  const { plan, notice } = scale.data

  return (
    <section className="space-y-3">
      <div>
        <h3 className="text-sm font-medium">Cluster size</h3>
        <p className="max-w-prose text-xs text-muted-foreground">{notice}</p>
      </div>

      <p role="status" aria-label="Cluster size" className="text-sm font-medium">
        {plan.sentence}
      </p>

      {plan.advice.length > 0 && (
        <ul className="list-inside list-disc space-y-1 text-xs text-muted-foreground">
          {plan.advice.map((line) => (
            <li key={line} className="max-w-prose">
              {line}
            </li>
          ))}
        </ul>
      )}

      <div>
        <h4 className="text-xs font-medium">Nodes</h4>
        <table className="w-full text-xs">
          <thead>
            <tr className="text-left text-muted-foreground">
              <th className="py-1 font-normal">Node</th>
              <th className="py-1 font-normal">Role</th>
              <th className="py-1 font-normal">May be removed</th>
            </tr>
          </thead>
          <tbody>
            {plan.removals.map((removal) => (
              <tr key={removal.machine} className="border-t border-border align-top">
                <td className="py-1 pr-2 font-mono break-all">{removal.name}</td>
                <td className="py-1 pr-2">{removal.role}</td>
                <td className="py-1">
                  {removal.allowed ? (
                    <span>yes</span>
                  ) : (
                    // The server's own sentence, unchanged. It is longer than a
                    // table cell wants and it is the only thing on this screen
                    // that tells an operator what to do instead.
                    <span className="text-destructive">{removal.reason}</span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {plan.additions.length > 0 && (
        <div>
          <h4 className="text-xs font-medium">Machines that could join</h4>
          <ul className="space-y-1 text-xs">
            {plan.additions.map((candidate) => (
              <li key={candidate.machine} className="border-t border-border py-1">
                <span className="font-mono break-all">{candidate.name}</span>{' '}
                {candidate.ready ? (
                  <span className="text-muted-foreground">— ready</span>
                ) : (
                  // Listed rather than hidden: "there is nothing to add" and
                  // "there are two machines and both are unreachable" send an
                  // operator to different places.
                  <span className="text-muted-foreground">— {candidate.reason}</span>
                )}
              </li>
            ))}
          </ul>
        </div>
      )}
    </section>
  )
}
