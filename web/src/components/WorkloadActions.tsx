import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { api } from '@/api'
import { Problem } from '@/components/Problem'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

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

/**
 * Scaling a deployment, and rolling its pods (milestone v1.17, slice 4).
 *
 * Two operations that look similar and are not. Scaling changes how many there
 * are; a rollout restart replaces the ones there are, under the deployment's
 * own strategy — its surge, its maxUnavailable, its readiness probes — so the
 * workload stays up while it happens. Deleting the pods one at a time would
 * take it down, which is why both exist and why the labels say which is which.
 *
 * Zero is a real replica count and the field allows it: it is how a workload is
 * switched off. The number is only sent when somebody changed it, so the button
 * cannot re-apply the count it already has as though it were an action.
 */
export function DeploymentActions({
  clusterID,
  namespace,
  deployment,
  desired,
}: {
  clusterID: string
  namespace: string
  deployment: string
  desired: number
}) {
  const [replicas, setReplicas] = useState(String(desired))
  const queryClient = useQueryClient()

  const invalidate = () => void queryClient.invalidateQueries({ queryKey: ['kubernetes'] })

  const scale = useMutation({
    mutationFn: (next: number) => api.kubernetes.scale(clusterID, namespace, deployment, next),
    onSuccess: invalidate,
  })

  const roll = useMutation({
    mutationFn: () => api.kubernetes.rolloutRestart(clusterID, namespace, deployment),
    onSuccess: invalidate,
  })

  const parsed = Number.parseInt(replicas, 10)
  const valid = Number.isInteger(parsed) && parsed >= 0
  const changed = valid && parsed !== desired

  return (
    <div className="space-y-1">
      <div className="flex flex-wrap items-center gap-2">
        <Input
          className="w-20"
          aria-label={`Replicas for ${deployment}`}
          inputMode="numeric"
          value={replicas}
          onChange={(event) => setReplicas(event.target.value)}
        />
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={!changed || scale.isPending}
          onClick={() => scale.mutate(parsed)}
        >
          {scale.isPending ? 'Scaling…' : 'Scale'}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={roll.isPending}
          onClick={() => roll.mutate()}
        >
          {roll.isPending ? 'Rolling…' : 'Roll pods'}
        </Button>
      </div>

      {/* The reason a disabled button is disabled, beside it: a phone has no
          hover, so a title attribute is not a reason there. */}
      {!valid && (
        <p className="text-muted-foreground text-xs">
          A replica count is a whole number, zero or more. Zero switches the workload off.
        </p>
      )}
      {valid && !changed && (
        <p className="text-muted-foreground text-xs">
          It already runs {desired}. Change the number to scale it.
        </p>
      )}

      {scale.error ? <Problem error={scale.error} /> : null}
      {roll.error ? <Problem error={roll.error} /> : null}
      {roll.isSuccess && (
        <p role="status" className="text-muted-foreground text-xs">
          Rolling. The deployment replaces its pods under its own strategy, so the workload stays up
          while it happens.
        </p>
      )}
    </div>
  )
}
