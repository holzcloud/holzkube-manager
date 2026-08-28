import {
  CircleCheckIcon,
  InfoIcon,
  Loader2Icon,
  OctagonXIcon,
  TriangleAlertIcon,
} from 'lucide-react'
import { Toaster as Sonner, type ToasterProps } from 'sonner'
import { useTheme } from '@/hooks/useTheme'

const Toaster = ({ ...props }: ToasterProps) => {
  // shadcn ships this wired to next-themes. holzkube has its own two-value
  // theme (D-11) and does not carry a second theming layer, so the copy in
  // this repository reads ours instead. This is what D-12 buys: the component
  // is source, not a dependency with an opinion.
  const { theme } = useTheme()

  return (
    <Sonner
      theme={theme as ToasterProps['theme']}
      className="toaster group"
      icons={{
        success: <CircleCheckIcon className="size-4" />,
        info: <InfoIcon className="size-4" />,
        warning: <TriangleAlertIcon className="size-4" />,
        error: <OctagonXIcon className="size-4" />,
        loading: <Loader2Icon className="size-4 animate-spin" />,
      }}
      style={
        {
          '--normal-bg': 'var(--popover)',
          '--normal-text': 'var(--popover-foreground)',
          '--normal-border': 'var(--border)',
          '--border-radius': 'var(--radius)',
        } as React.CSSProperties
      }
      toastOptions={{
        classNames: {
          toast: 'cn-toast',
          // UAT G-01-5: sonner parks its dismiss chip centred on the panel's
          // top-LEFT corner, so two thirds of it sits outside the toast and it
          // cuts across the corner radius -- and on a bottom-right toast that
          // is the edge facing the page. Pull it inside, on the trailing edge.
          closeButton:
            'top-3! right-3! left-auto! size-6! translate-x-0! translate-y-0! border-border bg-transparent',
        },
      }}
      {...props}
    />
  )
}

export { Toaster }
