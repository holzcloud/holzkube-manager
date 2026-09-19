import { useMutation } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { api } from '@/api'
import { Problem } from '@/components/Problem'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

/**
 * Running one command in a container (2026-09-19).
 *
 * Not a terminal, and the screen says so rather than looking like one that is
 * broken. A program and its arguments go, it runs, the output comes back.
 *
 * The arguments are split on spaces HERE and sent as a list, which is the one
 * place this could quietly become a shell. It does not: quoting is not
 * interpreted, so `echo "a b"` sends three arguments and one of them has a
 * quote in it. That is worse than a shell for writing clever things and better
 * for the archive, which records exactly what was sent.
 */
export function RunCommand({
  clusterID,
  namespace,
  pod,
  container,
}: {
  clusterID: string
  namespace: string
  pod: string
  container: string
}) {
  const fieldID = useId()
  const [text, setText] = useState('')

  const run = useMutation({
    mutationFn: (command: string[]) =>
      api.kubernetes.exec(clusterID, namespace, pod, container, command),
  })

  const command = text.split(' ').filter((part) => part !== '')

  return (
    <section className="space-y-2">
      <h3 className="font-medium text-sm">Run a command</h3>

      <div className="flex flex-wrap items-end gap-2">
        <div className="space-y-1">
          <label htmlFor={fieldID} className="text-muted-foreground text-xs">
            Program and arguments
          </label>
          <Input
            id={fieldID}
            className="w-72 font-mono max-md:h-11 max-md:w-full"
            placeholder="cat /etc/hosts"
            value={text}
            onChange={(event) => setText(event.target.value)}
          />
        </div>
        <Button
          type="button"
          variant="outline"
          className="max-md:h-11"
          disabled={command.length === 0 || run.isPending}
          onClick={() => run.mutate(command)}
        >
          {run.isPending ? 'Running…' : 'Run'}
        </Button>
      </div>

      <p className="text-muted-foreground text-xs">
        Not a terminal: one command runs and its output comes back. A shell with a string (
        <code>sh -c …</code>) is refused, because the record of what happened would then be hidden
        inside one argument. Every argument is kept in this installation's audit archive, and your
        cluster sees the person this acts as.
      </p>

      {run.error ? <Problem error={run.error} /> : null}

      {run.data && (
        <div className="space-y-1" role="status">
          <p className="text-muted-foreground text-xs">
            Ran as {run.data.identity} in {run.data.container}
            {run.data.truncated && ', and the output was cut'}
          </p>
          {run.data.stdout !== '' && (
            // Text in a pre, never rendered — the same rule the log and the
            // service proxy follow.
            <pre className="max-h-64 overflow-auto rounded-lg border bg-muted/40 p-2.5 font-mono text-xs whitespace-pre-wrap">
              {run.data.stdout}
            </pre>
          )}
          {run.data.stderr !== '' && (
            <pre className="max-h-64 overflow-auto rounded-lg border border-amber-600/40 p-2.5 font-mono text-amber-700 text-xs whitespace-pre-wrap dark:text-amber-300">
              {run.data.stderr}
            </pre>
          )}
          {run.data.stdout === '' && run.data.stderr === '' && (
            <p className="text-muted-foreground text-sm">It printed nothing.</p>
          )}
        </div>
      )}
    </section>
  )
}
