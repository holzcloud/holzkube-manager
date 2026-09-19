import type React from 'react'
import type { ReactNode } from 'react'
import { useIsPhone } from '@/hooks/useIsPhone'
import { cn } from '@/lib/utils'

/**
 * One set of rows, in the two shapes a table has to have (2026-09-19).
 *
 * # Why this exists
 *
 * The operator photographed /kubernetes on a phone: a "Scale" field and a "Roll
 * pods" button, with the deployment's namespace and name scrolled off the left
 * edge. The buttons were reachable. Which deployment they would act on was not
 * knowable. A sideways-scrolling table does not merely read badly on a phone —
 * it detaches an action from its subject, which is a different and worse defect.
 *
 * So below `md` a table is not a table. Above it, it is exactly the table it
 * always was: a wide table on a desk is what tables are for, and the two
 * presentations are built from one column list so they cannot say different
 * things.
 *
 * # The two phone shapes, and why there are two
 *
 * Current guidance is that there is no single right answer — the pattern follows
 * the data and the job (NN/g, "Mobile Tables"). Two jobs here, so two shapes,
 * and the operator chose this split on 2026-09-19:
 *
 *   - `cards`: for rows you act on one at a time — nodes, pods, deployments,
 *     clusters. Each row becomes a card: its identity as the heading, its
 *     figures as label/value pairs, its buttons full width at the bottom, under
 *     the name they belong to.
 *   - `rows`: for rows you scan — the audit archive, the image catalogue. The
 *     two columns that matter stay visible and the rest folds into a disclosure,
 *     because a hundred cards is a scroll nobody finishes. This is the
 *     "priority+" pattern.
 *
 * # What a column has to say
 *
 * `role` is not styling, it is what the column IS, and the phone shapes read it:
 * `identity` is what names the row and can never be the thing that scrolls away;
 * `actions` is what does something to it. A column list without those two marked
 * would leave this guessing, so `identity` is required by the type.
 */

export type ColumnRole = 'identity' | 'summary' | 'detail' | 'actions'

export interface Column<T> {
  /** Stable key; also the header id a phone card labels its value with. */
  key: string
  /** The header, and the label beside the value on a phone. */
  label: string
  render: (row: T) => ReactNode
  /**
   * What this column is to the row.
   *
   * `summary` is the default because it is the safe one: shown everywhere. A
   * column marked `detail` is hidden behind a disclosure on a phone, which is a
   * decision about what somebody can afford not to see.
   */
  role?: ColumnRole
  /** Classes for the desk table's cell, e.g. a right-aligned number. */
  className?: string
  /** Left out of the phone presentation entirely: pure desk chrome. */
  deskOnly?: boolean
}

export interface DataTableProps<T> {
  columns: Column<T>[]
  rows: T[]
  keyOf: (row: T) => string
  /** Which phone shape. See the two jobs above. */
  phone?: 'cards' | 'rows'
  /**
   * What to say when there are none — and it must be a sentence about the
   * cluster, not "No data". An empty list is a claim (INV-08, ledger 140), so
   * the caller says whose claim it is.
   */
  empty: ReactNode
  /** Accessible name for the table and the phone list. */
  label: string
  className?: string
  /**
   * What tapping the row does, when the row itself is the target.
   *
   * The audit archive works this way: a record opens a detail dialog. Where it
   * is set, the row becomes a real button in BOTH presentations -- on a phone
   * that replaces the disclosure, because "more" is already the dialog and two
   * ways to expand one row would be two answers to one question.
   */
  onRowClick?: (row: T) => void
  /** The button's accessible name, required when onRowClick is set. */
  rowLabel?: (row: T) => string
}

export function DataTable<T>({
  columns,
  rows,
  keyOf,
  phone = 'cards',
  empty,
  label,
  className,
  onRowClick,
  rowLabel,
}: DataTableProps<T>) {
  const phoneWidth = useIsPhone()

  if (rows.length === 0) {
    return <p className="text-muted-foreground text-sm">{empty}</p>
  }

  // One presentation, not two hidden by CSS: see useIsPhone for why a document
  // carrying both copies of every control is a problem beyond tidiness.
  if (phoneWidth) {
    return (
      <ul aria-label={label} className={cn('space-y-2', className)}>
        {rows.map((row) =>
          onRowClick !== undefined ? (
            <PhoneButtonRow
              key={keyOf(row)}
              columns={columns}
              row={row}
              label={rowLabel?.(row) ?? ''}
              onClick={() => onRowClick(row)}
            />
          ) : phone === 'cards' ? (
            <PhoneCard key={keyOf(row)} columns={columns} row={row} />
          ) : (
            <PhoneRow key={keyOf(row)} columns={columns} row={row} />
          ),
        )}
      </ul>
    )
  }

  return (
    <>
      <div className={className}>
        <table className="w-full caption-bottom text-sm" aria-label={label}>
          <thead>
            <tr className="border-b text-left text-muted-foreground">
              {columns.map((column) => (
                <th key={column.key} scope="col" className="py-1 pr-4 font-medium">
                  {column.label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr
                key={keyOf(row)}
                className={cn('border-b last:border-0', onRowClick && 'cursor-pointer')}
                {...(onRowClick
                  ? {
                      role: 'button',
                      tabIndex: 0,
                      'aria-label': rowLabel?.(row),
                      onClick: () => onRowClick(row),
                      onKeyDown: (event: React.KeyboardEvent) => {
                        if (event.key === 'Enter' || event.key === ' ') {
                          event.preventDefault()
                          onRowClick(row)
                        }
                      },
                    }
                  : {})}
              >
                {columns.map((column) => (
                  <td key={column.key} className={cn('py-2 pr-4 align-top', column.className)}>
                    {column.render(row)}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  )
}

function visible<T>(columns: Column<T>[], role: ColumnRole) {
  return columns.filter((c) => !c.deskOnly && (c.role ?? 'summary') === role)
}

/**
 * One row as a card: name on top, figures beneath it, buttons at the bottom.
 *
 * The order is the point. The identity comes first because it is what the rest
 * of the card is about, and the actions come last because a button above the
 * name it acts on is the same detachment in vertical form.
 */
function PhoneCard<T>({ columns, row }: { columns: Column<T>[]; row: T }) {
  const identity = visible(columns, 'identity')
  const facts = [...visible(columns, 'summary'), ...visible(columns, 'detail')]
  const actions = visible(columns, 'actions')

  return (
    <li data-row className="rounded-lg border p-3">
      <div className="space-y-0.5">
        {identity.map((column) => (
          <div key={column.key} className="break-words font-medium text-sm">
            {column.render(row)}
          </div>
        ))}
      </div>

      {facts.length > 0 && (
        <dl className="mt-2 grid grid-cols-[minmax(0,auto)_minmax(0,1fr)] gap-x-3 gap-y-1 text-sm">
          {facts.map((column) => (
            <div key={column.key} className="contents">
              <dt className="text-muted-foreground">{column.label}</dt>
              <dd className="break-words">{column.render(row)}</dd>
            </div>
          ))}
        </dl>
      )}

      {actions.length > 0 && (
        <div className="mt-3 flex flex-col gap-2 border-t pt-3">
          {actions.map((column) => (
            <div key={column.key}>{column.render(row)}</div>
          ))}
        </div>
      )}
    </li>
  )
}

/**
 * One row in priority+: what identifies it, what you are scanning for, and the
 * rest behind a disclosure.
 *
 * `<details>` rather than a button and state, because the browser already
 * implements this — including the keyboard, the accessible name and the
 * expanded state that a hand-rolled version gets wrong.
 */
function PhoneRow<T>({ columns, row }: { columns: Column<T>[]; row: T }) {
  const identity = visible(columns, 'identity')
  const summary = visible(columns, 'summary')
  const detail = visible(columns, 'detail')
  const actions = visible(columns, 'actions')
  const foldable = detail.length > 0 || actions.length > 0

  const head = (
    <div className="min-w-0 flex-1 space-y-0.5 text-left">
      {identity.map((column) => (
        <div key={column.key} className="break-words font-medium text-sm">
          {column.render(row)}
        </div>
      ))}
      {summary.length > 0 && (
        <div className="flex flex-wrap gap-x-3 text-muted-foreground text-xs">
          {summary.map((column) => (
            <span key={column.key} className="break-words">
              {column.label}: {column.render(row)}
            </span>
          ))}
        </div>
      )}
    </div>
  )

  if (!foldable) {
    return (
      <li data-row className="border-b py-2 last:border-0">
        {head}
      </li>
    )
  }

  return (
    <li data-row className="border-b last:border-0">
      <details className="group">
        {/* min-h-11 is 44px: the row itself is the target that opens it, so it
            has to be big enough for a thumb. */}
        <summary className="flex min-h-11 cursor-pointer list-none items-center gap-2 py-2 marker:content-none">
          {head}
          <span
            aria-hidden="true"
            className="shrink-0 text-muted-foreground text-xs transition-transform group-open:rotate-90"
          >
            ▶
          </span>
        </summary>

        <dl className="grid grid-cols-[minmax(0,auto)_minmax(0,1fr)] gap-x-3 gap-y-1 pb-3 text-sm">
          {detail.map((column) => (
            <div key={column.key} className="contents">
              <dt className="text-muted-foreground">{column.label}</dt>
              <dd className="break-words">{column.render(row)}</dd>
            </div>
          ))}
        </dl>

        {actions.length > 0 && (
          <div className="flex flex-col gap-2 pb-3">
            {actions.map((column) => (
              <div key={column.key}>{column.render(row)}</div>
            ))}
          </div>
        )}
      </details>
    </li>
  )
}

/**
 * One row that IS a button, for a list where tapping opens something.
 *
 * Everything visible sits inside the button, so the accessible name and the
 * target are the same box — which is the property the photographed screen
 * lacked in the other direction: there the target was there and the subject
 * was not.
 */
function PhoneButtonRow<T>({
  columns,
  row,
  label,
  onClick,
}: {
  columns: Column<T>[]
  row: T
  label: string
  onClick: () => void
}) {
  const identity = visible(columns, 'identity')
  const facts = [...visible(columns, 'summary'), ...visible(columns, 'detail')]

  return (
    <li data-row className="border-b last:border-0">
      <button
        type="button"
        onClick={onClick}
        aria-label={label}
        className="flex min-h-11 w-full flex-col items-start gap-0.5 py-2 text-left"
      >
        {identity.map((column) => (
          <span key={column.key} className="break-all font-medium text-sm">
            {column.render(row)}
          </span>
        ))}
        <span className="flex flex-wrap gap-x-3 text-muted-foreground text-xs">
          {facts.map((column) => (
            <span key={column.key} className="break-words">
              {column.label}: {column.render(row)}
            </span>
          ))}
        </span>
      </button>
    </li>
  )
}
