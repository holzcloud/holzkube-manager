/**
 * The problem-type base, for fixtures.
 *
 * `docs/api-contract.md` roots the taxonomy here and the Go constant
 * `httpapi.ProblemBaseURI` is its one authority; the web suite cannot import
 * that, so this is the single place the string is spelled on this side of the
 * seam. It exists because the alternative -- eight literals across three test
 * files -- is what made re-rooting the taxonomy a repository-wide edit rather
 * than a one-line one.
 *
 * Nothing in `src/` outside tests needs this. The interface branches on
 * `problem.code` and never on `problem.type` (see `lib/problem.ts`), which is
 * exactly why the base could move at all.
 */
export const PROBLEM_BASE_URI = 'urn:holzkube-manager:problem:'

/** A problem type for a fixture body, composed from the base. */
export function problemType(suffix: string): string {
  return `${PROBLEM_BASE_URI}${suffix}`
}
