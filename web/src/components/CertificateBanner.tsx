import { useQuery } from '@tanstack/react-query'
import { AlertTriangle } from 'lucide-react'
import { api, type Cluster } from '@/api'
import { cn } from '@/lib/utils'

/**
 * The certificate ladder, on every page (D-23).
 *
 * Talos does not rotate client certificates. When the one holzkube-manager holds
 * expires, every node in that cluster becomes unreachable in the same second,
 * and an operator who was never warned reads five red nodes as a dead cluster
 * and starts the wrong repair. The warning therefore escalates rather than
 * appearing once: a badge on the cluster page at 90 days, this banner at 30,
 * and an urgent version of it at 7 or already expired.
 *
 * Only `banner` and `critical` reach this component. `badge` deliberately does
 * not: a banner on every page three months early is a banner people learn to
 * scroll past, and the rung that matters would then be invisible.
 */
export function CertificateBanner({ className }: { className?: string }) {
  const { data } = useQuery({
    queryKey: ['clusters'],
    queryFn: () => api.clusters.list(),
    // Quiet: this is a date, not a live measurement. Re-asking every few
    // seconds would be polling for something that changes once a year.
    refetchInterval: 5 * 60 * 1000,
    retry: false,
  })

  const warning = (data ?? []).filter(
    (c) => c.certificate_urgency === 'banner' || c.certificate_urgency === 'critical',
  )
  if (warning.length === 0) {
    return null
  }

  return (
    <div className={cn('flex flex-col gap-2', className)}>
      {warning.map((c) => (
        <ClusterCertificateWarning key={c.id} cluster={c} />
      ))}
    </div>
  )
}

function ClusterCertificateWarning({ cluster }: { cluster: Cluster }) {
  const critical = cluster.certificate_urgency === 'critical'
  const expired = cluster.client_cert_days_left <= 0

  return (
    <div
      role="status"
      className={cn(
        'flex items-start gap-2 rounded-md border px-3 py-2 text-sm',
        critical
          ? 'border-red-600/40 bg-red-600/10 text-red-700 dark:text-red-300'
          : 'border-amber-600/40 bg-amber-600/10 text-amber-700 dark:text-amber-300',
      )}
    >
      <AlertTriangle aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
      <div>
        <p>{cluster.certificate_warning}</p>
        {!expired && (
          <p className="mt-0.5 text-xs opacity-80">
            {cluster.client_cert_days_left} day{cluster.client_cert_days_left === 1 ? '' : 's'}{' '}
            left, expiring {new Date(cluster.client_cert_not_after).toLocaleDateString()}.
          </p>
        )}
      </div>
    </div>
  )
}
