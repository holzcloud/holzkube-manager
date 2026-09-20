import { useQuery } from '@tanstack/react-query'
import { api } from '@/api'
import { DataTable } from '@/components/DataTable'
import { Problem } from '@/components/Problem'

/**
 * The cluster's networking (2026-09-20).
 *
 * # The one fact that makes this more than `kubectl get svc`
 *
 * A Service with no endpoints is the commonest broken thing in Kubernetes, and it
 * looks completely healthy in every list: a name, a type, a ClusterIP, its ports.
 * Nothing about it says no pod matches its selector — so every request to it is
 * refused instantly, and the workload that calls it reports a connection error
 * pointing at itself.
 *
 * So the endpoints are counted per Service, and a Service with none is said to be
 * broken, with its selector named. "Nothing matches" without saying what does not
 * match leaves somebody where they were.
 *
 * # And a selector can be right while the pods are not ready
 *
 * An unready endpoint is excluded from load balancing entirely, so three
 * endpoints of which none are ready serves nothing while reporting three. Both
 * numbers are shown for that reason.
 *
 * # The dangerous default in policies is having none
 *
 * A namespace with no NetworkPolicy accepts traffic from every pod in the
 * cluster. That is Kubernetes's default and plenty of clusters run that way on
 * purpose — but it is invisible, and "we have policies" is usually believed about
 * a cluster where two namespaces have them and eleven do not. So the namespaces
 * with none are listed, not counted.
 *
 * A policy whose selector matches no pod is worse than no policy, because
 * somebody believes it is in force. That row is marked unhealthy.
 */
export function ClusterNetwork({ clusterID, namespace }: { clusterID: string; namespace: string }) {
  const network = useQuery({
    queryKey: ['kubernetes', 'network', clusterID, namespace],
    queryFn: () => api.kubernetes.network(clusterID, namespace),
    enabled: clusterID !== '',
    refetchInterval: 30_000,
  })

  if (network.error) return <Problem error={network.error} />
  if (!network.data) return <p className="text-muted-foreground text-sm">Reading…</p>

  const { services, policies, unprotected, ingress_classes, default_ingress_class, notice } =
    network.data

  // Broken first: the question is "what is wrong with the networking", and a list
  // sorted by name makes somebody read all of it to find out.
  const sorted = [...services].sort((a, b) => Number(a.healthy) - Number(b.healthy))

  return (
    <div className="space-y-4">
      <DataTable
        label="Services"
        rows={sorted}
        keyOf={(service) => `${service.namespace}/${service.name}`}
        empty={
          <>
            The API server answered, and there are no services
            {namespace === '' ? ' in this cluster' : ` in ${namespace}`}.
          </>
        }
        columns={[
          {
            key: 'name',
            label: 'Service',
            role: 'identity',
            render: (service) => (
              <span className="break-all font-mono text-xs">
                {service.namespace}/{service.name}
              </span>
            ),
          },
          {
            key: 'type',
            label: 'Type',
            render: (service) => <span className="text-xs">{service.type}</span>,
          },
          {
            key: 'endpoints',
            label: 'Behind it',
            // The finding column. Both numbers, because an unready endpoint is
            // not in the load balancer and three of them serve nothing.
            render: (service) => (
              <span
                className={
                  service.healthy
                    ? 'text-xs tabular-nums'
                    : 'text-xs text-red-700 tabular-nums dark:text-red-300'
                }
              >
                {service.type === 'ExternalName'
                  ? '—'
                  : service.endpoints === 0
                    ? 'nothing'
                    : `${service.ready_endpoints} of ${service.endpoints} ready`}
              </span>
            ),
          },
          {
            key: 'address',
            label: 'Reachable at',
            render: (service) => (
              <span className="break-all font-mono text-xs">
                {service.external || service.cluster_ip || '—'}
                {service.external === '' && service.cluster_ip !== '' ? (
                  <span className="font-sans text-muted-foreground"> (inside only)</span>
                ) : null}
              </span>
            ),
          },
          {
            key: 'ports',
            label: 'Ports',
            role: 'detail',
            render: (service) => (
              <span className="break-all font-mono text-xs">{service.ports.join(', ') || '—'}</span>
            ),
          },
          {
            key: 'selector',
            label: 'Selects',
            role: 'detail',
            render: (service) => (
              <span className="break-all font-mono text-xs">
                {service.selector || 'nothing (managed by hand)'}
              </span>
            ),
          },
          {
            key: 'why',
            label: 'Note',
            role: 'detail',
            render: (service) =>
              service.notice === '' ? (
                '—'
              ) : (
                <span className="text-muted-foreground text-xs">{service.notice}</span>
              ),
          },
        ]}
      />

      <DataTable
        label="Network policies"
        rows={policies}
        keyOf={(policy) => `${policy.namespace}/${policy.name}`}
        empty="The API server answered, and no NetworkPolicy exists here: every pod may reach every other pod, which is Kubernetes’s default rather than a setting somebody chose."
        columns={[
          {
            key: 'name',
            label: 'Policy',
            role: 'identity',
            render: (policy) => (
              <span className="break-all font-mono text-xs">
                {policy.namespace}/{policy.name}
              </span>
            ),
          },
          {
            key: 'applies',
            label: 'Applies to',
            render: (policy) => (
              <span className="break-all font-mono text-xs">{policy.applies}</span>
            ),
          },
          {
            key: 'selects',
            label: 'Pods',
            render: (policy) => (
              <span
                className={
                  policy.healthy
                    ? 'text-xs tabular-nums'
                    : 'text-xs text-red-700 tabular-nums dark:text-red-300'
                }
              >
                {policy.selects}
              </span>
            ),
          },
          {
            key: 'types',
            label: 'Direction',
            role: 'detail',
            render: (policy) => <span className="text-xs">{policy.types.join(' and ')}</span>,
          },
          {
            key: 'why',
            label: 'Note',
            render: (policy) => (
              <span className="text-muted-foreground text-xs">{policy.summary}</span>
            ),
          },
        ]}
      />

      {unprotected.length > 0 && (
        <p className="text-sm">
          <span className="text-amber-700 dark:text-amber-300">
            {unprotected.length === 1
              ? 'One namespace has pods and no NetworkPolicy'
              : `${unprotected.length} namespaces have pods and no NetworkPolicy`}
          </span>
          , so every pod in the cluster may reach them:{' '}
          <span className="break-all font-mono text-xs">{unprotected.join(', ')}</span>. That is
          Kubernetes’s default rather than a setting somebody chose.
        </p>
      )}

      <p className="text-muted-foreground text-xs">
        {ingress_classes.length === 0
          ? 'The cluster has no IngressClass, so no Ingress in it will be picked up by any controller.'
          : default_ingress_class === ''
            ? `Ingress classes: ${ingress_classes.join(', ')} — and none is the default, so an Ingress naming no class will never be picked up.`
            : `Ingress classes: ${ingress_classes.join(', ')}. An Ingress naming none uses ${default_ingress_class}.`}
      </p>

      {/* Where the endpoint numbers come from, said once by the server. */}
      <p className="text-muted-foreground text-xs">{notice}</p>
    </div>
  )
}
