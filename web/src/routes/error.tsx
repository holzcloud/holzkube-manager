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
export function ErrorPage({ error }: { error: Error }) {
  const problem = error instanceof ProblemError ? error.problem : null

  const title = problem?.title ?? 'Something went wrong'
  const detail =
    problem?.detail ??
    (problem === null
      ? 'holzkube could not complete that request. The details are in the server log.'
      : undefined)

  return (
    <div className="flex min-h-dvh items-center justify-center p-6">
      <Card className="w-full max-w-lg">
        <CardHeader>
          <CardTitle>{title}</CardTitle>
          {detail !== undefined && <CardDescription>{detail}</CardDescription>}
        </CardHeader>
        <CardContent className="space-y-4">
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
