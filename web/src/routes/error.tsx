import { Link } from '@tanstack/react-router'
import { ProblemError } from '@/api'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

/**
 * The page for anything that escaped a route's own handling.
 *
 * It shows a title, one explanatory sentence, and the `instance` identifier so
 * the operator can find the matching line in the server log. No stack trace and
 * no raw response body: an internal message here would be free reconnaissance
 * against a binary that holds cluster PKI, and the server already refuses to
 * send one.
 */
export function ErrorPage({ error, notFound = false }: { error: Error; notFound?: boolean }) {
  const problem = error instanceof ProblemError ? error.problem : null

  // UAT G-01-5: an unknown URL used to render the generic failure card, which
  // is indistinguishable from a real 500 and sends an operator who merely
  // mistyped to a server log with nothing in it. A missing page is a different
  // fact and says so, including the path that was asked for.
  const path = notFound && typeof window !== 'undefined' ? window.location.pathname : null

  const title = notFound ? 'That page does not exist' : (problem?.title ?? 'Something went wrong')
  const detail = notFound
    ? 'holzkube has no screen at this address. Nothing failed — the link or the typed URL is wrong, and there is nothing in the server log to find.'
    : (problem?.detail ??
      (problem === null
        ? 'holzkube could not complete that request. The details are in the server log.'
        : undefined))

  return (
    <div className="flex min-h-dvh items-center justify-center p-6">
      <Card className="w-full max-w-lg">
        <CardHeader>
          <CardTitle>{title}</CardTitle>
          {detail !== undefined && <CardDescription>{detail}</CardDescription>}
        </CardHeader>
        <CardContent className="space-y-4">
          {path !== null && (
            <p className="text-sm text-muted-foreground">
              Requested:{' '}
              <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs text-foreground">
                {path}
              </code>
            </p>
          )}
          {problem?.instance !== undefined && (
            <p className="text-sm text-muted-foreground">
              Reference:{' '}
              <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs text-foreground">
                {problem.instance}
              </code>
              . The server log carries the same identifier.
            </p>
          )}
          <Button asChild>
            <Link to="/">Back to the dashboard</Link>
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}
