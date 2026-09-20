import { createRoute, Outlet } from '@tanstack/react-router'
import { validateKubernetesSearch } from '@/components/KubernetesSection'
import { authenticatedRoute } from '@/routes/__root'

/**
 * What Kubernetes says about a cluster (milestone v1.17, split into pages
 * 2026-09-20).
 *
 * # Why this file is now four lines of routing and nothing else
 *
 * It had grown to fifteen cards on one page — nodes, capacity, namespaces,
 * workloads, pods, usage, storage, networking, configuration, clearing out,
 * access control, identity, events, the service proxy and the manifest form. The
 * operator's words were that you scroll yourself to death, and that is the right
 * description: everything was reachable and nothing was findable, which is the
 * same defect the phone tables had (ledger 152) in a different shape.
 *
 * Each of them is a page now, under `/kubernetes/...`, and this is the layout
 * they share. Pages rather than tabs, deliberately: a link to the storage view is
 * a link somebody can send, the back button does what it looks like it does, and
 * a page that fails takes only itself down. A tab strip would have given none of
 * those, and would have cost the same to build.
 *
 * # What it refuses to do is in the shared shell, not here
 *
 * Two rules were written once on a page that showed everything, and with ten
 * pages they have to live in what the pages share or nine of them would quietly
 * break them:
 *
 * An empty list is never rendered when the API server did not answer — the claim
 * INV-08 forbids, because an empty screen reads as "nothing is running" and sends
 * an operator looking for a workload instead of an API server. That is
 * `KubernetesAnswer`.
 *
 * The cluster and namespace selection lives in the URL, so moving between pages
 * keeps it, it can be sent to somebody else, and the back button restores it.
 * That is `validateKubernetesSearch` and `useClusterSelection`.
 *
 * Both are in web/src/components/KubernetesSection.tsx, with the reasoning.
 */
export const kubernetesRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: '/kubernetes',
  validateSearch: validateKubernetesSearch,
  component: Outlet,
})
