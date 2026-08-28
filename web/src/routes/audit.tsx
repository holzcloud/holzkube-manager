import { useQuery } from '@tanstack/react-query'
import { createRoute } from '@tanstack/react-router'
import { type AuditPage, api } from '@/api'
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
 * The audit view. Filters, the detail dialog and cursor paging arrive with the
 * rest of D-13; this is the chronological table they hang off.
 */
function AuditView() {
  const page = useQuery<AuditPage>({
    queryKey: ['audit'],
    queryFn: () => api.audit(),
  })

  return (
    <div className="space-y-6">
      <div>
        <h1 className="font-heading text-2xl font-semibold tracking-tight">Audit</h1>
        <p className="text-sm text-muted-foreground">
          Every mutation, newest first, in the order the hash chain records it.
        </p>
      </div>

      {page.isPending && <p className="text-sm text-muted-foreground">Loading…</p>}

      {page.data && page.data.items.length === 0 && (
        <p className="text-sm text-muted-foreground">No audit records yet.</p>
      )}

      {page.data && page.data.items.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Time</TableHead>
              <TableHead>Actor</TableHead>
              <TableHead>Action</TableHead>
              <TableHead>Outcome</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {page.data.items.map((record) => (
              <TableRow key={record.seq}>
                <TableCell className="tabular-nums">{record.ts}</TableCell>
                <TableCell>{record.actor}</TableCell>
                <TableCell className="font-mono text-xs">{record.action}</TableCell>
                <TableCell>{record.outcome}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  )
}

export const auditRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: '/audit',
  component: AuditView,
})
