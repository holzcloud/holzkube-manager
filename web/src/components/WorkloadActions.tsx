import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '@/api'
import { Problem } from '@/components/Problem'
import { Button } from '@/components/ui/button'

/**
 * Restarting a pod (milestone v1.17, slice 4).
 *
 * "Restart" is the word an operator uses and Kubernetes has no such verb: it
 * has "delete the pod and let the controller make another". Those are the same
 * operation exactly when a controller owns the pod — and when nothing does, the
 * second is deletion.
 *
 * So the server refuses that case and says so, and this button does not try to
 * guess: it asks, and shows the refusal as something to read. A screen that
 * hid the pods nothing owns would be a screen that cannot restart the pod
 * somebody is actually looking at.
 */
export function RestartPodButton({
  clusterID,
  namespace,
  pod,
}: {
  clusterID: string
  namespace: string
  pod: string
}) {
  const queryClient = useQueryClient()

  const restart = useMutation({
    mutationFn: () => api.kubernetes.restartPod(clusterID, namespace, pod),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ['kubernetes'] }),
  })

  return (
    <div className="space-y-1">
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={restart.isPending}
        onClick={() => restart.mutate()}
      >
        {restart.isPending ? 'Restarting…' : 'Restart'}
      </Button>
      {restart.error ? <Problem error={restart.error} /> : null}
    </div>
  )
}
