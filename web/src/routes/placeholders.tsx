import { createRoute } from '@tanstack/react-router'
import { ComingSoon } from '@/components/ComingSoon'
import { NAV_AREAS } from '@/components/Sidebar'
import { authenticatedRoute } from '@/routes/__root'

/**
 * One route per area that does not exist yet (D-10).
 *
 * The list is derived from the navigation itself, so a nav entry can never
 * point at a route nobody registered: adding an area to NAV_AREAS with a phase
 * number produces its placeholder page automatically, and removing the phase
 * number is what a later phase does when it replaces the placeholder with the
 * real screen.
 */
export const placeholderRoutes = NAV_AREAS.filter((area) => area.phase !== null).map((area) =>
  createRoute({
    getParentRoute: () => authenticatedRoute,
    path: area.path,
    component: () => (
      <ComingSoon area={area.label} phase={area.phase as number} description={area.description} />
    ),
  }),
)
