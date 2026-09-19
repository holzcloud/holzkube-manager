import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { api } from '@/api'
import { Problem } from '@/components/Problem'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'

/**
 * A cluster written down, and what it would mean.
 *
 * Nothing here applies anything, and the panel says so at the top rather than
 * at the bottom: an operator who reads "plan" and sees a button expects the
 * button to do it. Applying a template is provisioning, which is the one part
 * of this product that has never run against real hardware — driving it from a
 * YAML file would move that gap somewhere harder to see.
 *
 * What this is for is the question that actually comes first: given these
 * machines and these labels, what does this document say should happen, and
 * what does not add up?
 */
export function ClusterTemplatePanel({ clusterID }: { clusterID?: string }) {
  const [text, setText] = useState('')

  const plan = useMutation({
    mutationFn: () => api.clusterTemplates.plan(text),
  })

  return (
    <section className="space-y-3">
      <div>
        <h2 className="text-sm font-medium">Cluster template</h2>
        <p className="max-w-prose text-xs text-muted-foreground">
          A cluster described in one file, with its nodes chosen by machine class rather than listed
          by UUID — so the description survives a machine being replaced.
        </p>
        <p className="max-w-prose text-xs text-muted-foreground">
          <strong>Nothing here applies a template.</strong> This says what the document would mean
          for the machines this installation knows about right now. Building or changing a cluster
          is still the provisioning and upgrade screens, one decision at a time.
        </p>
      </div>

      {clusterID !== undefined && (
        <a
          href={api.clusterTemplates.exportPath(clusterID)}
          className="inline-flex items-center text-sm underline max-md:min-h-11"
          title="Writes this cluster down as a template. The nodes are listed by UUID rather than by class: nothing here can know which of your labels you meant as the reason a machine is in this cluster, and guessing one would produce a file that quietly selects a different set later."
        >
          Export this cluster as a template
        </a>
      )}

      <div className="space-y-1.5">
        <Label htmlFor="template-yaml">Template</Label>
        <textarea
          id="template-yaml"
          value={text}
          rows={10}
          spellCheck={false}
          className="w-full rounded-md border border-border bg-background p-2 font-mono text-xs"
          placeholder={PLACEHOLDER}
          onChange={(event) => setText(event.target.value)}
        />
      </div>

      <Button
        type="button"
        size="sm"
        disabled={text.trim() === '' || plan.isPending}
        onClick={() => plan.mutate()}
      >
        {plan.isPending ? 'Reading…' : 'What would this mean?'}
      </Button>

      {plan.error ? <Problem error={plan.error} /> : null}

      {plan.data && (
        <div
          role="status"
          aria-label="Template plan"
          className="space-y-2 rounded-md border border-border px-3 py-2"
        >
          <p className="text-sm font-medium">{plan.data.plan.sentence}</p>

          {plan.data.plan.problems.length > 0 && (
            <ul className="list-inside list-disc space-y-1 text-sm text-destructive">
              {plan.data.plan.problems.map((problem) => (
                <li key={problem}>{problem}</li>
              ))}
            </ul>
          )}

          {plan.data.plan.notes.length > 0 && (
            <ul className="list-inside list-disc space-y-1 text-xs text-muted-foreground">
              {plan.data.plan.notes.map((note) => (
                <li key={note}>{note}</li>
              ))}
            </ul>
          )}

          <dl className="grid grid-cols-[8rem_1fr] gap-x-4 gap-y-1 text-xs">
            <dt className="text-muted-foreground">Control plane</dt>
            <dd className="font-mono break-all">
              {plan.data.plan.control_plane.machines.join(', ') || '—'}
            </dd>
            <dt className="text-muted-foreground">Workers</dt>
            <dd className="font-mono break-all">
              {plan.data.plan.workers.machines.join(', ') || '—'}
            </dd>
          </dl>

          <p className="max-w-prose text-xs text-muted-foreground">{plan.data.notice}</p>
        </div>
      )}
    </section>
  )
}

const PLACEHOLDER = `kind: ClusterTemplate
name: homelab
talosVersion: v1.13.9
controlPlane:
  machineClass: rack-b
  count: 3
workers:
  machineClass: workers`
