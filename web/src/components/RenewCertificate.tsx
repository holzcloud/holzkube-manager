import { useMutation, useQueryClient } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import { api } from '@/api'
import { Problem } from '@/components/Problem'
import { Button } from '@/components/ui/button'

/**
 * The button the certificate ladder has been counting down to.
 *
 * D-23's warning escalates to "every node in this cluster becomes unreachable
 * at once" and, until this shipped, there was nothing to click. A countdown to
 * a door that does not exist is worse than no countdown: it teaches an operator
 * that the warnings on this screen are not actionable, and the next one they
 * skip will be one that mattered.
 *
 * It touches no node — the certificate is minted from the cluster's own
 * authority, which this installation holds — and the server proves the new one
 * against a node before keeping it. A refusal here therefore means the old
 * certificate is still in place, which is why the failure is shown as a problem
 * to read rather than as an alarm: nothing broke.
 */
export function RenewCertificate({ clusterID }: { clusterID: string }) {
  const queryClient = useQueryClient()

  const renew = useMutation({
    mutationFn: () => api.certificate.renew(clusterID),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ['clusters'] }),
  })

  return (
    <div className="space-y-1">
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={renew.isPending}
        onClick={() => renew.mutate()}
        title="Issues this installation a fresh admin certificate for this cluster, from the cluster's own certificate authority. No node is touched or restarted: a node trusts the authority rather than any one certificate issued from it. The new certificate has to reach a node before it replaces the old one."
      >
        <RefreshCw aria-hidden="true" className="size-4" />
        {renew.isPending ? 'Renewing…' : 'Renew this cluster’s certificate'}
      </Button>

      {renew.error ? <Problem error={renew.error} /> : null}

      {renew.isSuccess && (
        <p role="status" className="text-xs text-muted-foreground">
          Renewed. It expires {new Date(renew.data.client_cert_not_after).toLocaleDateString()}, and
          no node was touched.
        </p>
      )}
    </div>
  )
}
