import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { api, type Machine } from '@/api'
import { Problem } from '@/components/Problem'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

/**
 * The operator's own words about a machine.
 *
 * Everything else on a node's page is something the node said about itself.
 * These are not, and the editor says so, because the difference is what makes a
 * label safe to build a machine class on: no observation ever overwrites one.
 *
 * The write replaces the whole set rather than merging, which is why this edits
 * a local copy and sends it entire. A merge cannot remove anything, and a form
 * on top of one would leave a deleted row still there after the save.
 */
export function LabelEditor({ machine }: { machine: Machine }) {
  const queryClient = useQueryClient()

  // Rows and not a map, because a map cannot hold a half-typed key — two rows
  // mid-edit would collide on "" and one would vanish under the operator's
  // hands.
  const [rows, setRows] = useState<{ key: string; value: string }[]>(() =>
    Object.entries(machine.labels).map(([key, value]) => ({ key, value })),
  )
  const [dirty, setDirty] = useState(false)

  const save = useMutation({
    mutationFn: () => {
      const labels: Record<string, string> = {}
      for (const row of rows) {
        if (row.key.trim() !== '') {
          labels[row.key] = row.value
        }
      }
      return api.labels.set(machine.id, labels)
    },
    onSuccess: () => {
      setDirty(false)
      void queryClient.invalidateQueries({ queryKey: ['machine', machine.id] })
      void queryClient.invalidateQueries({ queryKey: ['machines'] })
    },
  })

  const edit = (next: { key: string; value: string }[]) => {
    setRows(next)
    setDirty(true)
  }

  return (
    <div className="space-y-2">
      <p className="max-w-prose text-xs text-muted-foreground">
        Your words, not the node's. Nothing this machine reports ever changes them, which is what
        makes a machine class built on them stay the same set across a reboot.
      </p>

      {rows.length === 0 && <p className="text-sm text-muted-foreground">No labels.</p>}

      {rows.map((row, index) => (
        // biome-ignore lint/suspicious/noArrayIndexKey: rows are positional while a key is being typed
        <div key={index} className="flex items-center gap-2">
          <Label htmlFor={`label-key-${index}`} className="sr-only">
            Label {index + 1} key
          </Label>
          <Input
            id={`label-key-${index}`}
            value={row.key}
            className="h-8 w-40"
            placeholder="rack"
            onChange={(event) =>
              edit(rows.map((r, i) => (i === index ? { ...r, key: event.target.value } : r)))
            }
          />
          <Label htmlFor={`label-value-${index}`} className="sr-only">
            Label {index + 1} value
          </Label>
          <Input
            id={`label-value-${index}`}
            value={row.value}
            className="h-8 w-40"
            placeholder="b3"
            onChange={(event) =>
              edit(rows.map((r, i) => (i === index ? { ...r, value: event.target.value } : r)))
            }
          />
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={() => edit(rows.filter((_, i) => i !== index))}
          >
            Remove
          </Button>
        </div>
      ))}

      <div className="flex gap-2">
        <Button
          type="button"
          size="sm"
          variant="secondary"
          onClick={() => edit([...rows, { key: '', value: '' }])}
        >
          Add label
        </Button>
        <Button
          type="button"
          size="sm"
          disabled={!dirty || save.isPending}
          onClick={() => save.mutate()}
        >
          {save.isPending ? 'Saving…' : 'Save labels'}
        </Button>
      </div>

      {save.error ? <Problem error={save.error} /> : null}
    </div>
  )
}
