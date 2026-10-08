import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Download } from 'lucide-react'
import { useId, useState } from 'react'
import { api } from '@/api'
import { Problem } from '@/components/Problem'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatBytes, formatDateTime } from '@/lib/format'

/**
 * Scheduled etcd snapshots (2026-10-08).
 *
 * A schedule, the list of what is stored, a download and "run now" -- on the
 * same files the snapshot before a Talos upgrade already keeps.
 *
 * What the panel will not do is let them look safer than they are. The files
 * live on the device this manager runs on; they survive a bad upgrade and a
 * lost etcd, not the loss of the device. The sentence saying so is on the
 * panel whether or not anything is wrong, and so is "overdue": a schedule that
 * stopped producing snapshots is the failure nobody notices until the day the
 * snapshot is needed.
 */

const PRESETS: { value: string; label: string }[] = [
  { value: 'off', label: 'Off' },
  { value: '6h', label: 'Every 6 hours' },
  { value: 'daily', label: 'Daily' },
  { value: 'weekly', label: 'Weekly' },
]

const KIND_LABEL: Record<string, string> = {
  scheduled: 'scheduled',
  upgrade: 'before an upgrade',
}

export function intervalLabel(interval: string): string {
  return PRESETS.find((p) => p.value === interval)?.label ?? interval
}

function ago(seconds: number): string {
  if (seconds < 90) return 'just now'
  const minutes = Math.round(seconds / 60)
  if (minutes < 90) return `${minutes} minutes ago`
  const hours = Math.round(minutes / 60)
  if (hours < 48) return `${hours} hours ago`
  return `${Math.round(hours / 24)} days ago`
}

export function BackupsPanel({ cluster }: { cluster: string }) {
  const qc = useQueryClient()
  const intervalID = useId()
  const keepID = useId()
  const [preset, setPreset] = useState<string | null>(null)
  const [keep, setKeep] = useState<string | null>(null)

  const query = useQuery({
    queryKey: ['backups', cluster],
    queryFn: () => api.backups.state(cluster),
    // A run takes as long as the database is large; follow it while it goes.
    refetchInterval: (q) => (q.state.data?.state?.status.last_result === 'running' ? 3000 : false),
  })

  const save = useMutation({
    mutationFn: (v: { interval: string; keep: number }) =>
      api.backups.setSchedule(cluster, v.interval, v.keep),
    onSuccess: async () => {
      setPreset(null)
      setKeep(null)
      await qc.invalidateQueries({ queryKey: ['backups', cluster] })
      await qc.invalidateQueries({ queryKey: ['clusters'] })
    },
  })

  const run = useMutation({
    mutationFn: () => api.backups.run(cluster),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['backups', cluster] }),
  })

  const data = query.data
  const state = data?.state
  const health = state?.health
  const shownInterval = preset ?? data?.schedule.interval ?? 'off'
  const shownKeep = keep ?? String(data?.schedule.keep ?? 7)
  const keepNumber = Number(shownKeep)
  const maxKeep = data?.max_keep ?? 60
  const keepValid = Number.isInteger(keepNumber) && keepNumber >= 1 && keepNumber <= maxKeep
  const dirty =
    data !== undefined &&
    (shownInterval !== data.schedule.interval ||
      (shownInterval !== 'off' && keepNumber !== data.schedule.keep))

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center gap-2 text-base">
          Backups
          {health?.enabled && (
            <Badge variant="outline">{intervalLabel(health.interval).toLowerCase()}</Badge>
          )}
          {health?.overdue && (
            <Badge
              variant="outline"
              className="border-amber-500 text-amber-700 dark:text-amber-300"
            >
              <AlertTriangle aria-hidden="true" className="mr-1 size-3" />
              backup overdue
            </Badge>
          )}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {query.error ? <Problem error={query.error} /> : null}

        {data && !data.available && (
          <p className="text-sm text-muted-foreground">
            This instance has nowhere to keep snapshots, so nothing here can run.
          </p>
        )}

        {data?.available && (
          <>
            <div className="space-y-1 text-sm">
              {health?.overdue ? (
                <p className="text-amber-700 dark:text-amber-300">
                  {health.last_success_at
                    ? `The last scheduled snapshot is from ${formatDateTime(health.last_success_at)}, more than two intervals ago.`
                    : 'The schedule has not produced a snapshot yet, and two intervals have passed.'}
                </p>
              ) : null}
              <p>
                <span className="text-muted-foreground">Last scheduled snapshot: </span>
                {health?.last_success_at ? formatDateTime(health.last_success_at) : 'none yet'}
              </p>
              <p>
                <span className="text-muted-foreground">Last run: </span>
                {runSentence(state?.status.last_result ?? '', state?.status.last_reason ?? '')}
                {state?.status.last_attempt_at
                  ? ` (${formatDateTime(state.status.last_attempt_at)})`
                  : ''}
              </p>
              {state?.next_due_at && health?.enabled && (
                <p>
                  <span className="text-muted-foreground">Next run: </span>
                  {formatDateTime(state.next_due_at)}
                </p>
              )}
              {state?.free_bytes != null && (
                <p>
                  <span className="text-muted-foreground">Free where they are kept: </span>
                  {formatBytes(state.free_bytes)}. A snapshot is skipped, with a reason here, when
                  less than twice its size is free.
                </p>
              )}
            </div>

            <form
              className="flex flex-wrap items-end gap-3"
              onSubmit={(e) => {
                e.preventDefault()
                if (dirty && keepValid) {
                  save.mutate({ interval: shownInterval, keep: keepNumber })
                }
              }}
            >
              <div className="space-y-1">
                <Label htmlFor={intervalID}>Schedule</Label>
                <Select value={shownInterval} onValueChange={setPreset}>
                  <SelectTrigger id={intervalID} className="w-44 max-md:h-11" aria-label="Schedule">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {PRESETS.map((p) => (
                      <SelectItem key={p.value} value={p.value}>
                        {p.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1">
                <Label htmlFor={keepID}>Keep the newest</Label>
                <Input
                  id={keepID}
                  type="number"
                  inputMode="numeric"
                  min={1}
                  max={maxKeep}
                  className="w-24 max-md:h-11"
                  value={shownKeep}
                  disabled={shownInterval === 'off'}
                  onChange={(e) => setKeep(e.target.value)}
                />
              </div>
              <Button
                type="submit"
                className="max-md:h-11"
                disabled={!dirty || !keepValid || save.isPending}
              >
                {save.isPending ? 'Saving…' : 'Save the schedule'}
              </Button>
              <Button
                type="button"
                variant="outline"
                className="max-md:h-11"
                disabled={run.isPending || state?.status.last_result === 'running'}
                onClick={() => run.mutate()}
              >
                {run.isPending || state?.status.last_result === 'running'
                  ? 'Taking a snapshot…'
                  : 'Run now'}
              </Button>
            </form>
            {!keepValid && shownInterval !== 'off' && (
              <p role="alert" className="text-sm text-red-700 dark:text-red-300">
                Keep between 1 and {maxKeep} snapshots.
              </p>
            )}
            {save.error ? <Problem error={save.error} /> : null}
            {run.error ? <Problem error={run.error} /> : null}

            {state && state.snapshots.length > 0 ? (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Taken</TableHead>
                    <TableHead>Kind</TableHead>
                    <TableHead>Size</TableHead>
                    <TableHead>SHA-256</TableHead>
                    <TableHead>
                      <span className="sr-only">Download</span>
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {state.snapshots.map((s) => (
                    <TableRow key={s.id}>
                      <TableCell>
                        {formatDateTime(s.taken_at)}
                        {health?.age_seconds != null && s.id === state.snapshots[0]?.id && (
                          <span className="ml-2 text-xs text-muted-foreground">
                            {ago(health.age_seconds)}
                          </span>
                        )}
                      </TableCell>
                      <TableCell>{KIND_LABEL[s.kind] ?? s.kind}</TableCell>
                      <TableCell>{formatBytes(s.bytes)}</TableCell>
                      <TableCell className="font-mono text-xs" title={s.sha256 || undefined}>
                        {s.sha256 ? s.sha256.slice(0, 16) : 'not recorded'}
                      </TableCell>
                      <TableCell>
                        <a
                          className="inline-flex items-center gap-1 text-sm underline max-md:min-h-11"
                          href={api.backups.downloadURL(cluster, s.id)}
                          aria-label={`Download the snapshot from ${formatDateTime(s.taken_at)}`}
                        >
                          <Download aria-hidden="true" className="size-4" />
                          Download
                        </a>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            ) : (
              <p className="text-sm text-muted-foreground">No snapshots are stored yet.</p>
            )}

            <p className="max-w-prose text-xs text-muted-foreground">
              These snapshots are kept on the device this manager runs on. They survive a bad
              upgrade and a lost etcd; they do not survive losing that device or its SD card. Copy
              the ones that matter somewhere else — a download needs an administrator. Each one
              holds every Kubernetes secret of the cluster, and the download is recorded in the
              audit log. Snapshots taken before an upgrade are kept separately (the newest two) and
              are not counted against the number above.
            </p>
          </>
        )}
      </CardContent>
    </Card>
  )
}

function runSentence(result: string, reason: string): string {
  switch (result) {
    case 'ok':
      return 'succeeded'
    case 'running':
      return 'running'
    case 'skipped':
      return `skipped — ${reason}`
    case 'failed':
      return `failed — ${reason}`
    default:
      return 'none yet'
  }
}
