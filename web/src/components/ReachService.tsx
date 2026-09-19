import { useMutation } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { api, type KubernetesService } from '@/api'
import { Problem } from '@/components/Problem'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

/**
 * Reaching a service in the cluster (milestone v1.17, slice 6).
 *
 * It fetches. It does not host. The answer appears as TEXT in a `<pre>`, never
 * as a rendered page, and that is not a styling choice: rendering a workload's
 * HTML here would put that pod's markup in this daemon's origin — the origin
 * holding the operator's session — so any pod in the cluster could script this
 * interface. The server already refuses to carry the workload's content type;
 * this screen is the other half of the same decision.
 *
 * What it is good for is what an operator actually wants from a management
 * screen: `/healthz`, `/readyz`, `/metrics`, a JSON endpoint. What it is not is
 * a way to use a workload's web interface through here.
 *
 * The port comes from a list rather than a box, because a service's ports are
 * the thing somebody would otherwise go to `kubectl` to look up.
 */
export function ReachService({
  clusterID,
  services,
}: {
  clusterID: string
  services: KubernetesService[]
}) {
  const [target, setTarget] = useState('')
  const [port, setPort] = useState('')
  const [path, setPath] = useState('/healthz')
  const pathID = useId()

  const chosen = services.find((service) => `${service.namespace}/${service.name}` === target)

  const fetchIt = useMutation({
    mutationFn: () => {
      if (!chosen) throw new Error('no service chosen')
      return api.kubernetes.proxyService(clusterID, chosen.namespace, chosen.name, port, path)
    },
  })

  const pick = (next: string) => {
    setTarget(next)
    // The port belonged to the previous service. Carrying it over would send a
    // request to a port this service does not have and report the workload's
    // answer to it.
    const service = services.find((s) => `${s.namespace}/${s.name}` === next)
    setPort(service?.ports.length === 1 ? String(service.ports[0]?.port ?? '') : '')
    fetchIt.reset()
  }

  if (services.length === 0) {
    return (
      <p className="text-muted-foreground text-sm">
        The API server answered, and this cluster has no services to reach.
      </p>
    )
  }

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-end gap-2">
        <div className="space-y-1">
          <span className="text-muted-foreground text-xs">Service</span>
          <Select value={target} onValueChange={pick}>
            <SelectTrigger className="w-64" aria-label="Service">
              <SelectValue placeholder="choose a service" />
            </SelectTrigger>
            <SelectContent>
              {services.map((service) => (
                <SelectItem
                  key={`${service.namespace}/${service.name}`}
                  value={`${service.namespace}/${service.name}`}
                >
                  {service.namespace}/{service.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="space-y-1">
          <span className="text-muted-foreground text-xs">Port</span>
          <Select value={port} onValueChange={setPort} disabled={!chosen}>
            <SelectTrigger className="w-32" aria-label="Port">
              <SelectValue placeholder="port" />
            </SelectTrigger>
            <SelectContent>
              {(chosen?.ports ?? []).map((p) => (
                <SelectItem key={p.port} value={String(p.port)}>
                  {p.name === '' ? p.port : `${p.port} (${p.name})`}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="space-y-1">
          <label htmlFor={pathID} className="text-muted-foreground text-xs">
            Path
          </label>
          <Input
            id={pathID}
            className="w-56 font-mono"
            value={path}
            onChange={(event) => setPath(event.target.value)}
          />
        </div>

        <Button
          type="button"
          variant="outline"
          disabled={!chosen || port === '' || fetchIt.isPending}
          onClick={() => fetchIt.mutate()}
        >
          {fetchIt.isPending ? 'Reaching…' : 'Fetch'}
        </Button>
      </div>

      <p className="text-muted-foreground text-xs">
        A GET, and the answer is shown as text. This reaches a health or metrics endpoint; it is not
        a way to use a workload's web interface from here.
      </p>

      {chosen && chosen.ports.length === 0 && (
        <p className="text-muted-foreground text-xs">
          {chosen.name} exposes no ports, so there is nothing to reach on it.
        </p>
      )}

      {fetchIt.error ? <Problem error={fetchIt.error} /> : null}

      {fetchIt.data && (
        <div className="space-y-1">
          <p className="text-sm" role="status">
            The workload answered {fetchIt.data.status}
            {fetchIt.data.truncated ? ', and the body was cut at 1 MiB' : ''}.
          </p>
          {/* Text, in a pre. Never dangerouslySetInnerHTML, never an iframe:
              either would be this product hosting a pod's markup on its own
              origin. */}
          <pre className="max-h-96 overflow-auto rounded-lg border bg-muted/40 p-2.5 font-mono text-xs whitespace-pre-wrap">
            {fetchIt.data.body === ''
              ? '(the workload answered with an empty body)'
              : fetchIt.data.body}
          </pre>
        </div>
      )}
    </div>
  )
}
