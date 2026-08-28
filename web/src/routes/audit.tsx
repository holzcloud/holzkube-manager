import { useQuery } from '@tanstack/react-query'
import { createRoute } from '@tanstack/react-router'
import { useCallback, useState } from 'react'
import { type AuditQuery, type AuditRecord, api } from '@/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
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
import { authenticatedRoute } from '@/routes/__root'

/**
 * The audit view, and with it the first dense table in the project -- the
 * pattern phase 3 and everything after it inherits (D-13).
 *
 * Paging is a button at the end of the list rather than infinite scroll. With
 * forensic data a stable page you can name and come back to is worth more than
 * a fluid one you cannot.
 */

/** The redaction marker the server writes for anything not on the allowlist. */
export const REDACTED_MARKER = '<redacted>'

const ALL_ACTIONS = '__all__'

/**
 * The *mutating* action tokens `docs/api-contract.md` § Routes defines.
 *
 * Read-only routes are deliberately absent. The audit middleware returns the
 * handler unwrapped when the request is not mutating, so no record with an
 * `auth.me`, `audit.list` or `system.status` action is ever written. Offering
 * them as filters produced the empty state -- "No audit records match these
 * filters. The log itself is untouched" -- which reads as a fact about the
 * operator's activity rather than a fact about the filter.
 */
const ACTION_TOKENS = [
  'setup.create',
  'auth.login',
  'auth.logout',
  'auth.sudo',
  'account.password',
] as const

interface Filters {
  from: string
  to: string
  action: string
}

const NO_FILTERS: Filters = { from: '', to: '', action: '' }

/** A local date-time from the filter input, as the RFC 3339 the contract wants. */
function toRFC3339(local: string): string {
  if (local === '') {
    return ''
  }
  const parsed = new Date(local)
  return Number.isNaN(parsed.getTime()) ? '' : parsed.toISOString()
}

function queryFor(filters: Filters, cursor: number | null): AuditQuery {
  const query: AuditQuery = {}
  const from = toRFC3339(filters.from)
  const to = toRFC3339(filters.to)
  if (from !== '') {
    query.from = from
  }
  if (to !== '') {
    query.to = to
  }
  if (filters.action !== '') {
    query.action = filters.action
  }
  if (cursor !== null) {
    query.cursor = cursor
  }
  return query
}

function AuditView() {
  const [draft, setDraft] = useState<Filters>(NO_FILTERS)
  const [filters, setFilters] = useState<Filters>(NO_FILTERS)
  const [pages, setPages] = useState<AuditRecord[][]>([])
  const [cursor, setCursor] = useState<number | null>(null)
  const [selected, setSelected] = useState<AuditRecord | null>(null)

  const page = useQuery({
    queryKey: ['audit', filters, cursor],
    queryFn: async () => {
      const result = await api.audit(queryFor(filters, cursor))
      // Merged by seq rather than appended. A queryFn is not guaranteed to run
      // once per logical page: the root client sets refetchOnWindowFocus with a
      // 5s staleTime and StrictMode is on, so switching browser tabs and back
      // re-ran this with a non-null cursor and appended the same page again.
      // records = pages.flat() then held every record of that page twice,
      // rendered with key={record.seq} -- duplicate React keys, and a forensic
      // table showing the same event twice. isOrphanedIntent reads the same
      // array, so the duplicates also changed which rows were flagged.
      setPages((previous) => {
        if (cursor === null) {
          return [result.items]
        }
        const seen = new Set(previous.flat().map((record) => record.seq))
        const fresh = result.items.filter((record) => !seen.has(record.seq))
        return fresh.length === 0 ? previous : [...previous, fresh]
      })
      return result
    },
  })

  const applyFilters = useCallback(() => {
    setPages([])
    setCursor(null)
    setFilters(draft)
  }, [draft])

  const clearFilters = useCallback(() => {
    setPages([])
    setCursor(null)
    setDraft(NO_FILTERS)
    setFilters(NO_FILTERS)
  }, [])

  const records = pages.flat()

  // The contract pins this: next_cursor is always present and is either a
  // number or null. It is compared against null and never tested for
  // truthiness -- a falsy check would read a hypothetical 0 as exhaustion and
  // would be right only by the accident of which sentinel was chosen.
  const nextCursor: number | null = page.data?.next_cursor ?? null
  const hasMore = page.isSuccess && page.data.next_cursor !== null

  return (
    <div className="space-y-6">
      <div>
        <h1 className="font-heading text-2xl font-semibold tracking-tight">Audit</h1>
        <p className="text-sm text-muted-foreground">
          Every mutation, newest first, in the order the hash chain records it.
        </p>
      </div>

      <form
        className="flex flex-wrap items-end gap-3"
        onSubmit={(event) => {
          event.preventDefault()
          applyFilters()
        }}
      >
        <div className="space-y-1">
          <Label htmlFor="audit-from">From</Label>
          <Input
            id="audit-from"
            type="datetime-local"
            value={draft.from}
            onChange={(event) => setDraft({ ...draft, from: event.target.value })}
          />
        </div>

        <div className="space-y-1">
          <Label htmlFor="audit-to">To</Label>
          <Input
            id="audit-to"
            type="datetime-local"
            value={draft.to}
            onChange={(event) => setDraft({ ...draft, to: event.target.value })}
          />
        </div>

        <div className="space-y-1">
          <Label htmlFor="audit-action">Action</Label>
          <Select
            value={draft.action === '' ? ALL_ACTIONS : draft.action}
            onValueChange={(value) =>
              setDraft({ ...draft, action: value === ALL_ACTIONS ? '' : value })
            }
          >
            <SelectTrigger id="audit-action" className="w-56">
              <SelectValue placeholder="Any action" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL_ACTIONS}>Any action</SelectItem>
              {ACTION_TOKENS.map((token) => (
                <SelectItem key={token} value={token}>
                  {token}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <Button type="submit">Apply filters</Button>
        <Button type="button" variant="ghost" onClick={clearFilters}>
          Clear
        </Button>
      </form>

      {page.isPending && records.length === 0 && (
        <p className="text-sm text-muted-foreground">Loading…</p>
      )}

      {page.isSuccess && records.length === 0 && (
        <p className="text-sm text-muted-foreground">
          No audit records match these filters. The log itself is untouched — widen the date range
          or clear the action filter.
        </p>
      )}

      {records.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Time</TableHead>
              <TableHead>Actor</TableHead>
              <TableHead>Action</TableHead>
              <TableHead>Outcome</TableHead>
              <TableHead>Target</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {records.map((record) => (
              <AuditRow
                key={record.seq}
                record={record}
                orphanedIntent={isOrphanedIntent(record, records)}
                onOpen={() => setSelected(record)}
              />
            ))}
          </TableBody>
        </Table>
      )}

      {hasMore && (
        <Button type="button" variant="secondary" onClick={() => setCursor(nextCursor)}>
          Load older records
        </Button>
      )}

      <AuditDetail record={selected} onClose={() => setSelected(null)} />
    </div>
  )
}

/**
 * An intent with no matching outcome means the process did not survive the
 * action. It is the forensically interesting case, so it is marked rather than
 * left to be spotted by reading sequence numbers.
 *
 * The pairing is on the immediate successor, because that is how the writer
 * emits them: `Outcome` appends the outcome with the next sequence number and
 * repeats the attempt's identifying fields. Asking instead whether *any* later
 * record shares the action hid real orphans -- a genuinely orphaned
 * `auth.login` stopped being flagged as soon as the next login appeared, and
 * with two interleaved attempts for one action neither was flagged even though
 * one of them was orphaned. Silently not showing findings is worse than not
 * having the feature.
 */
function isOrphanedIntent(record: AuditRecord, all: AuditRecord[]): boolean {
  if (record.outcome !== 'attempt') {
    return false
  }
  const successor = all.find((other) => other.seq === record.seq + 1)
  // Not loaded yet is not the same as absent: only claim an orphan when the
  // record that would disprove it is in hand.
  if (successor === undefined) {
    return false
  }
  return successor.action !== record.action || successor.outcome === 'attempt'
}

function targetOf(record: AuditRecord): string {
  if (record.machine_id !== '') {
    return record.machine_id
  }
  if (record.cluster_id !== '') {
    return record.cluster_id
  }
  if (record.job_id !== '') {
    return record.job_id
  }
  return '—'
}

function AuditRow({
  record,
  orphanedIntent,
  onOpen,
}: {
  record: AuditRecord
  orphanedIntent: boolean
  onOpen: () => void
}) {
  return (
    <TableRow
      onClick={onOpen}
      onKeyDown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault()
          onOpen()
        }
      }}
      tabIndex={0}
      role="button"
      aria-label={`Record ${record.seq}: ${record.action}`}
      className={orphanedIntent ? 'cursor-pointer bg-destructive/10' : 'cursor-pointer'}
    >
      <TableCell className="tabular-nums">{record.ts}</TableCell>
      <TableCell>{record.actor === '' ? '—' : record.actor}</TableCell>
      <TableCell className="font-mono text-xs">{record.action}</TableCell>
      <TableCell>
        <OutcomeBadge outcome={record.outcome} orphanedIntent={orphanedIntent} />
      </TableCell>
      <TableCell className="font-mono text-xs">{targetOf(record)}</TableCell>
    </TableRow>
  )
}

function OutcomeBadge({ outcome, orphanedIntent }: { outcome: string; orphanedIntent: boolean }) {
  if (orphanedIntent) {
    return <Badge variant="destructive">attempt — no outcome</Badge>
  }
  if (outcome === 'error') {
    return <Badge variant="destructive">error</Badge>
  }
  if (outcome === 'success') {
    return <Badge variant="secondary">success</Badge>
  }
  return <Badge variant="outline">{outcome}</Badge>
}

/** A session token is already truncated by the server; shorten further to read. */
function shortSession(session: string): string {
  if (session === '') {
    return '—'
  }
  return session.length <= 12 ? session : `${session.slice(0, 12)}…`
}

function AuditDetail({ record, onClose }: { record: AuditRecord | null; onClose: () => void }) {
  return (
    <Dialog
      open={record !== null}
      onOpenChange={(open) => {
        if (!open) {
          onClose()
        }
      }}
    >
      <DialogContent className="max-w-2xl">
        {record !== null && (
          <>
            <DialogHeader>
              <DialogTitle>
                Record {record.seq} — {record.action}
              </DialogTitle>
              <DialogDescription>
                Every field of this record, exactly as it is hashed into the chain.
              </DialogDescription>
            </DialogHeader>

            <dl className="grid grid-cols-[minmax(0,9rem)_1fr] gap-x-4 gap-y-2 text-sm">
              <Field label="seq" value={String(record.seq)} mono />
              <Field label="Time" value={record.ts} />
              <Field label="Actor" value={record.actor === '' ? '—' : record.actor} />
              <Field label="Outcome" value={record.outcome} />
              <Field label="Request ID" value={requestIdOf(record)} mono />
              <Field label="Source IP" value={record.src_ip === '' ? '—' : record.src_ip} mono />
              <Field label="Session" value={shortSession(record.session)} mono />
              <Field label="Cluster" value={record.cluster_id === '' ? '—' : record.cluster_id} />
              <Field label="Machine" value={record.machine_id === '' ? '—' : record.machine_id} />
              <Field label="Job" value={record.job_id === '' ? '—' : record.job_id} />
              <Field label="prev_hash" value={record.prev_hash} mono />
              <Field label="hash" value={record.hash} mono />
            </dl>

            <div>
              <h3 className="mb-2 text-sm font-medium">Parameters</h3>
              <Params params={record.params} />
            </div>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}

function requestIdOf(record: AuditRecord): string {
  const value = record.params.request_id
  return typeof value === 'string' && value !== '' ? value : '—'
}

function Field({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className={mono === true ? 'break-all font-mono text-xs' : 'break-words'}>{value}</dd>
    </>
  )
}

/**
 * Redacted values are shown as redacted rather than as an empty field. The
 * operator should be able to see that something was there and was deliberately
 * not kept -- an empty cell reads as "nothing was sent", which is a different
 * and wrong statement.
 */
function Params({ params }: { params: Record<string, unknown> }) {
  const entries = Object.entries(params).filter(([key]) => key !== 'request_id')

  if (entries.length === 0) {
    return <p className="text-sm text-muted-foreground">No parameters were recorded.</p>
  }

  return (
    <dl className="grid grid-cols-[minmax(0,9rem)_1fr] gap-x-4 gap-y-2 text-sm">
      {entries.map(([key, value]) => (
        <div key={key} className="contents">
          <dt className="font-mono text-xs text-muted-foreground">{key}</dt>
          <dd>
            {value === REDACTED_MARKER ? (
              <Badge variant="outline" className="font-normal">
                redacted — recorded as sent, value deliberately not kept
              </Badge>
            ) : (
              <span className="break-all font-mono text-xs">{JSON.stringify(value)}</span>
            )}
          </dd>
        </div>
      ))}
    </dl>
  )
}

export const auditRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: '/audit',
  component: AuditView,
})

export { AuditView }
