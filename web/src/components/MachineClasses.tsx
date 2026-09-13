import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { api, type LabelSelector, type MachineClass } from '@/api'
import { Problem } from '@/components/Problem'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

/**
 * Named sets of machines, selected by label.
 *
 * A class is a question and not a group: a machine joins it by being labelled
 * and leaves by being unlabelled, and the membership is re-answered on every
 * read. That is why each row shows the count it names *now* rather than a list
 * somebody maintained — and why a class that names nothing is shown as such
 * rather than hidden. A class matching nothing is usually a label that has not
 * been written yet, and that is a thing to see.
 */
export function MachineClasses() {
  const queryClient = useQueryClient()
  const classes = useQuery({
    queryKey: ['machine-classes'],
    queryFn: () => api.machineClasses.list(),
  })

  const refresh = () => queryClient.invalidateQueries({ queryKey: ['machine-classes'] })

  return (
    <section className="space-y-3">
      <div>
        <h2 className="text-sm font-medium">Machine classes</h2>
        <p className="max-w-prose text-xs text-muted-foreground">
          A named set of machines, chosen by their labels. Every condition has to hold, and a class
          with no conditions is refused rather than stored — it would match nothing, and the reading
          that makes it match everything is the one that costs a cluster.
        </p>
      </div>

      {classes.error ? <Problem error={classes.error} /> : null}

      {classes.data?.length === 0 && (
        <p className="text-sm text-muted-foreground">No machine classes yet.</p>
      )}

      {classes.data?.map((c) => (
        <ClassRow key={c.id} machineClass={c} onChanged={refresh} />
      ))}

      <NewClassForm onCreated={refresh} />
    </section>
  )
}

function ClassRow({
  machineClass,
  onChanged,
}: {
  machineClass: MachineClass
  onChanged: () => void
}) {
  const remove = useMutation({
    mutationFn: () => api.machineClasses.remove(machineClass.id),
    onSuccess: onChanged,
  })

  return (
    <div className="flex items-start justify-between gap-3 rounded-md border border-border px-3 py-2">
      <div className="min-w-0">
        <p className="text-sm font-medium">{machineClass.name}</p>
        {machineClass.description !== '' && (
          <p className="text-xs text-muted-foreground">{machineClass.description}</p>
        )}
        <p className="text-xs text-muted-foreground">{machineClass.sentence}</p>
        <p className="text-xs text-muted-foreground">
          {machineClass.count === 0
            ? 'names no machines right now'
            : `names ${machineClass.count} machine${machineClass.count === 1 ? '' : 's'} right now`}
        </p>
      </div>

      <Button
        type="button"
        size="sm"
        variant="ghost"
        disabled={remove.isPending}
        onClick={() => remove.mutate()}
      >
        Remove
      </Button>

      {remove.error ? <Problem error={remove.error} /> : null}
    </div>
  )
}

/**
 * Creating a class.
 *
 * The selector is entered as `key=value` lines, one condition each, because a
 * form with three lists is a form nobody fills in. `key=` is presence and
 * `!key` is absence, which covers what the server accepts and nothing it does
 * not.
 */
function NewClassForm({ onCreated }: { onCreated: () => void }) {
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [conditions, setConditions] = useState('')

  const create = useMutation({
    mutationFn: () =>
      api.machineClasses.put(slug(name), {
        name,
        description,
        selector: parseConditions(conditions),
      }),
    onSuccess: () => {
      setName('')
      setDescription('')
      setConditions('')
      onCreated()
    },
  })

  return (
    <form
      className="flex flex-wrap items-end gap-2"
      onSubmit={(event) => {
        event.preventDefault()
        create.mutate()
      }}
    >
      <div className="space-y-1.5">
        <Label htmlFor="class-name">New class</Label>
        <Input
          id="class-name"
          value={name}
          className="h-8 w-40"
          onChange={(event) => setName(event.target.value)}
        />
      </div>

      <div className="space-y-1.5">
        <Label htmlFor="class-description">What it is for</Label>
        <Input
          id="class-description"
          value={description}
          className="h-8 w-56"
          onChange={(event) => setDescription(event.target.value)}
        />
      </div>

      <div className="space-y-1.5">
        <Label htmlFor="class-conditions">Conditions</Label>
        <Input
          id="class-conditions"
          value={conditions}
          className="h-8 w-72 font-mono text-xs"
          placeholder="rack=b3, storage=, !decommissioned"
          onChange={(event) => setConditions(event.target.value)}
        />
      </div>

      <Button type="submit" size="sm" disabled={name.trim() === '' || create.isPending}>
        {create.isPending ? 'Creating…' : 'Create class'}
      </Button>

      {create.error ? <Problem error={create.error} /> : null}
    </form>
  )
}

/**
 * Turns `rack=b3, storage=, !decommissioned` into a selector.
 *
 * Exported so the parsing is testable on its own: it is the one piece of this
 * screen where a mistake produces a selector that quietly names the wrong
 * machines rather than an error.
 */
export function parseConditions(input: string): LabelSelector {
  const selector: LabelSelector = { equals: {}, present: [], absent: [] }

  for (const raw of input.split(',')) {
    const piece = raw.trim()
    if (piece === '') {
      continue
    }
    if (piece.startsWith('!')) {
      const key = piece.slice(1).trim()
      if (key !== '') {
        selector.absent.push(key)
      }
      continue
    }
    const at = piece.indexOf('=')
    if (at < 0) {
      // No '=' at all is presence too. Writing `storage` and meaning "has a
      // storage label" is the obvious reading, and refusing it would be
      // refusing the shorter spelling of something already supported.
      selector.present.push(piece)
      continue
    }
    const key = piece.slice(0, at).trim()
    const value = piece.slice(at + 1).trim()
    if (key === '') {
      continue
    }
    if (value === '') {
      selector.present.push(key)
    } else {
      selector.equals[key] = value
    }
  }
  return selector
}

/** An id from a name: lowercase, and nothing a path segment has to escape. */
export function slug(name: string): string {
  return (
    name
      .trim()
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '') || 'class'
  )
}
