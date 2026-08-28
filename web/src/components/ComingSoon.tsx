import { Construction } from 'lucide-react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

/**
 * The honest placeholder (D-10).
 *
 * The navigation entry is visible and clickable from phase 1; only the content
 * is empty, and it says so. No dead link, no blank frame, and above all no
 * invented preview of data that does not exist yet -- a fake table here would
 * be indistinguishable from a broken real one later.
 */
export function ComingSoon({
  area,
  phase,
  description,
}: {
  area: string
  phase: number
  description: string
}) {
  return (
    <Card className="max-w-2xl">
      <CardHeader>
        <div className="flex items-center gap-2">
          <Construction aria-hidden="true" className="size-5 text-muted-foreground" />
          <CardTitle>{area}</CardTitle>
        </div>
        <CardDescription>Coming in phase {phase}.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3 text-sm text-muted-foreground">
        <p>{description}</p>
        <p>
          Nothing here is built yet. This page exists so the navigation is complete from the first
          release, and so that later phases add a screen rather than rebuild the shell.
        </p>
      </CardContent>
    </Card>
  )
}
