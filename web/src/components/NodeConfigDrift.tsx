import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle } from 'lucide-react'
import { api, type ConfigDriftDetail, type ConfigDriftState } from '@/api'
import { Problem } from '@/components/Problem'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { formatDateTime } from '@/lib/format'
import { cn } from '@/lib/utils'

/**
 * Where one node's configuration stands against its baseline (15-09).
 *
 * The page works out nothing. The state, the sentence, the start of a drift and
 * the difference all come from the server; this shows them. Two rules from the
 * certificate screens hold here as well:
 *
 *  - A state without a judgement says so. `not-read`, `unreadable` and
 *    `baseline-damaged` mean "nobody knows", and a section that drew them like
 *    `ok` would be the false reassurance the whole feature exists to prevent.
 *    Even `ok` is worded "As last known" and carries no tick: it is the
 *    result of a reading that is already a while old.
 *  - The fixed sentence about secret values is part of every answer and so of
 *    every state. A rotated token changes nothing visible here, and saying so
 *    only when something else drifted would let "no drift" read as "nothing
 *    changed".
 *
 * Confirming is the only action. It makes the state the operator is looking at
 * the new baseline; it sends the digest of THAT state, so a node that changed
 * again after the page was drawn is refused by the server rather than confirmed
 * with the wrong picture. There is no button to put a baseline back on the node:
 * that would be a write to the cluster, and a phase of its own.
 */
export function NodeConfigDrift({ machine, className }: { machine: string; className?: string }) {
  const qc = useQueryClient()

  const query = useQuery({
    queryKey: ['config-drift', machine],
    queryFn: () => api.configDrift.get(machine),
    // The server reads the nodes every few hours; this only follows what it
    // holds, so a minute is plenty.
    refetchInterval: 60 * 1000,
  })

  const confirm = useMutation({
    mutationFn: (digest: string) => api.configDrift.confirm(machine, digest),
    // Settled, not only successful: after a refusal (the node changed again,
    // or it was confirmed meanwhile) the page must show the current state and
    // not the one that was refused.
    onSettled: async () => {
      await qc.invalidateQueries({ queryKey: ['config-drift', machine] })
      await qc.invalidateQueries({ queryKey: ['clusters'] })
    },
  })

  const data = query.data

  return (
    <Card className={className}>
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center gap-2 text-base">
          Configuration
          {data && <StateBadge state={data.state} />}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3 text-sm">
        {query.error ? <Problem error={query.error} /> : null}
        {query.isPending && !query.error ? <p className="text-muted-foreground">Reading…</p> : null}

        {data && (
          <>
            <p>{data.sentence}</p>
            <Facts data={data} />

            {data.state === 'drift' && data.diff && (
              <>
                <ul className="space-y-1">
                  {data.diff.changes.map((c) => (
                    <li key={c.path} className="font-mono text-xs">
                      <span className="mr-2 text-muted-foreground">{c.kind}</span>
                      <span className="font-medium">{c.path}</span>
                      {c.kind === 'list-changed' ? (
                        <span className="block text-muted-foreground">
                          {c.len_before} entries become {c.len_after}: {c.after}
                          {c.duplicates.length > 0 && (
                            <span className="block text-amber-700 dark:text-amber-300">
                              duplicated: {c.duplicates.join(', ')}
                            </span>
                          )}
                        </span>
                      ) : (
                        <span className="block text-muted-foreground">
                          {c.before !== '' && <span className="line-through">{c.before}</span>}{' '}
                          {c.after !== '' && <span>{c.after}</span>}
                        </span>
                      )}
                    </li>
                  ))}
                </ul>

                <div className="space-y-2">
                  <p className="text-muted-foreground">
                    Confirming makes the configuration shown here the baseline this node is compared
                    with from now on, and ends the warning. It changes nothing on the node.
                  </p>
                  {confirm.error ? <Problem error={confirm.error} /> : null}
                  <Button
                    variant="outline"
                    size="sm"
                    className="max-md:min-h-11"
                    disabled={confirm.isPending}
                    onClick={() => confirm.mutate(data.digest)}
                  >
                    Confirm as intended
                  </Button>
                </div>
              </>
            )}

            <p className="text-muted-foreground text-xs">{data.notice}</p>
          </>
        )}
      </CardContent>
    </Card>
  )
}

const BASELINE_SOURCE: Record<string, string> = {
  'first-sight': 'first seen',
  apply: 'set by an apply from here',
  confirmed: 'confirmed as intended',
}

function Facts({ data }: { data: ConfigDriftDetail }) {
  const parts: string[] = []
  if (data.baseline_taken_at) {
    const source = data.baseline_source ? BASELINE_SOURCE[data.baseline_source] : undefined
    parts.push(`Baseline: ${source ?? 'taken'} ${formatDateTime(data.baseline_taken_at)}.`)
  }
  if (data.read_at) {
    parts.push(`Last read ${formatDateTime(data.read_at)}.`)
  }
  if (data.state === 'drift' && data.drift_since) {
    parts.push(`Differs since ${formatDateTime(data.drift_since)}.`)
  }
  if (parts.length === 0) return null
  return <p className="text-muted-foreground text-xs">{parts.join(' ')}</p>
}

/**
 * One badge per state. Neutral for everything that is not a finding, amber for
 * the two that need a look, and nothing for `ok`: it has no colour and no tick,
 * because a green badge on a reading that is hours old is a claim.
 */
function StateBadge({ state }: { state: ConfigDriftState }) {
  const label = STATE_LABEL[state]
  const warn = state === 'drift' || state === 'settling'
  return (
    <Badge
      variant="outline"
      className={cn(warn && 'border-amber-500 text-amber-700 dark:text-amber-300')}
    >
      {state === 'drift' && <AlertTriangle aria-hidden="true" className="mr-1 size-3" />}
      {label}
    </Badge>
  )
}

export const STATE_LABEL: Record<ConfigDriftState, string> = {
  'not-read': 'not read yet',
  'baseline-created': 'baseline created',
  ok: 'As last known',
  settling: 'settling',
  drift: 'differs from baseline',
  unreadable: 'cannot be read',
  'baseline-damaged': 'baseline unusable',
}
