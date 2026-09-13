/**
 * A failure, rendered as one line.
 *
 * It lived as an identical private copy in the provisioning and upgrades
 * screens, and the accounts screen would have been the third. Two copies of
 * six lines is cheaper than the import; three is where that stops being true,
 * and three is also where they start to drift.
 *
 * The `unknown` parameter is deliberate. Every caller hands it a react-query
 * error, which is typed `unknown` because a mutation can reject with anything
 * — and a component that demanded an Error would push a cast to each call
 * site, where the cast would eventually be wrong.
 */
export function Problem({ error }: { error: unknown }) {
  return (
    <p className="max-w-prose text-sm text-destructive">
      {error instanceof Error ? error.message : String(error)}
    </p>
  )
}
