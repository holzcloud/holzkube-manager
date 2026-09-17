'use client'

import type * as React from 'react'

import { cn } from '@/lib/utils'

function Table({ className, ...props }: React.ComponentProps<'table'>) {
  return (
    <section
      data-slot="table-container"
      // A section with a label is a landmark, which is what a scrollable
      // region is meant to be -- and unlike a div with role="region" it is the
      // element the role describes.
      aria-label="Table, scrolls sideways"
      // Focusable, because a container that scrolls and cannot be focused
      // shows the keyboard its first column and nothing else -- WCAG 2.1.1.
      // Biome's rule is about decorative tabindex on static markup and does
      // not distinguish the case where the element's own content is reachable
      // only by scrolling it. Measured at 390px, these tables are 574 and 855
      // wide in a 390 viewport.
      // biome-ignore lint/a11y/noNoninteractiveTabindex: a scroll container has to be focusable (WCAG 2.1.1)
      tabIndex={0}
      // The shadow on the right edge is the whole affordance: measured at
      // 390px these tables are 574 and 855 wide inside a 390 viewport, fully
      // reachable by swiping and with nothing saying so. A cut edge with no
      // scrollbar does not look like more content, it looks like the end --
      // which is exactly how a clipped element reads, and the two must not
      // look the same. It appears only while there is something to the right.
      className="relative w-full overflow-x-auto focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring [background:linear-gradient(to_left,var(--color-border),transparent)_right/12px_100%_no-repeat] [background-attachment:local]"
    >
      <table
        data-slot="table"
        className={cn('w-full caption-bottom text-sm', className)}
        {...props}
      />
    </section>
  )
}

function TableHeader({ className, ...props }: React.ComponentProps<'thead'>) {
  return <thead data-slot="table-header" className={cn('[&_tr]:border-b', className)} {...props} />
}

function TableBody({ className, ...props }: React.ComponentProps<'tbody'>) {
  return (
    <tbody
      data-slot="table-body"
      className={cn('[&_tr:last-child]:border-0', className)}
      {...props}
    />
  )
}

function TableFooter({ className, ...props }: React.ComponentProps<'tfoot'>) {
  return (
    <tfoot
      data-slot="table-footer"
      className={cn('border-t bg-muted/50 font-medium [&>tr]:last:border-b-0', className)}
      {...props}
    />
  )
}

function TableRow({ className, ...props }: React.ComponentProps<'tr'>) {
  return (
    <tr
      data-slot="table-row"
      className={cn(
        'border-b transition-colors hover:bg-muted/50 has-aria-expanded:bg-muted/50 data-[state=selected]:bg-muted',
        className,
      )}
      {...props}
    />
  )
}

function TableHead({ className, ...props }: React.ComponentProps<'th'>) {
  return (
    <th
      data-slot="table-head"
      className={cn(
        'h-10 px-2 text-left align-middle font-medium whitespace-nowrap text-foreground [&:has([role=checkbox])]:pr-0',
        className,
      )}
      {...props}
    />
  )
}

function TableCell({ className, ...props }: React.ComponentProps<'td'>) {
  return (
    <td
      data-slot="table-cell"
      className={cn('p-2 align-middle whitespace-nowrap [&:has([role=checkbox])]:pr-0', className)}
      {...props}
    />
  )
}

function TableCaption({ className, ...props }: React.ComponentProps<'caption'>) {
  return (
    <caption
      data-slot="table-caption"
      className={cn('mt-4 text-sm text-muted-foreground', className)}
      {...props}
    />
  )
}

export { Table, TableBody, TableCaption, TableCell, TableFooter, TableHead, TableHeader, TableRow }
