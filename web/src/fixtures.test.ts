import { describe, expect, it } from 'vitest'
import { z } from 'zod'
import {
  accessControlSchema,
  auditPageSchema,
  clusterCapacitySchema,
  clusterEventsSchema,
  clusterNetworkSchema,
  clusterResourcesSchema,
  clusterStorageSchema,
  clustersSchema,
  clusterUsageSchema,
  inventorySchema,
  jobsSchema,
  kubernetesOverviewSchema,
  machineSchema,
  machinesSchema,
  nodeDetailSchema,
  noticesSchema,
  schematicSchema,
  sweepPlanSchema,
  usersSchema,
  wallSchema,
  workloadsSchema,
} from '@/api'
import demo from '../fixtures/demo.json'

/**
 * The layout guard's fixtures, parsed with the product's own schemas.
 *
 * This exists because of how the guard fails when nobody is watching. It renders
 * the screens against these fixtures; if one of them stops matching what the app
 * expects, the screen renders an error or an empty list, every table has no rows,
 * and the guard reports a clean run — which is exactly the state ledger 149
 * records: a guard measuring the empty state passed the screen the operator
 * photographed as broken.
 *
 * So the fixtures are held to the same contract as the server's answers. Not a
 * hand-written copy of it: the schemas below are the ones api.ts parses real
 * responses with, imported rather than restated, so a field that changes shape
 * in the product breaks this on the same day.
 */

const fixtures = demo as Record<string, unknown>

describe('the layout guard’s fixtures', () => {
  it.each([
    ['/api/v1/machines', machinesSchema],
    ['/api/v1/clusters', clustersSchema],
    ['/api/v1/users', usersSchema],
    ['/api/v1/jobs', jobsSchema],
    ['/api/v1/audit', auditPageSchema],
    ['/api/v1/clusters/c-homelab/kubernetes', kubernetesOverviewSchema],
    ['/api/v1/schematics', z.array(schematicSchema)],
    ['/api/v1/provision/notices', noticesSchema],
    ['/api/v1/machines/m-cp-1', machineSchema],
    ['/api/v1/clusters/c-homelab/kubernetes/events', clusterEventsSchema],
    ['/api/v1/clusters/c-homelab/kubernetes/workloads', workloadsSchema],
    ['/api/v1/clusters/c-homelab/kubernetes/resources', clusterResourcesSchema],
    ['/api/v1/clusters/c-homelab/kubernetes/nodes/srv-node-01.homelab.example', nodeDetailSchema],
    ['/api/v1/clusters/c-homelab/kubernetes/usage', clusterUsageSchema],
    ['/api/v1/clusters/c-homelab/kubernetes/capacity', clusterCapacitySchema],
    ['/api/v1/clusters/c-homelab/kubernetes/sweep', sweepPlanSchema],
    ['/api/v1/clusters/c-homelab/kubernetes/storage', clusterStorageSchema],
    ['/api/v1/clusters/c-homelab/kubernetes/network', clusterNetworkSchema],
    ['/api/v1/clusters/c-homelab/kubernetes/access', accessControlSchema],
    ['/api/v1/clusters/c-homelab/kubernetes/inventory', inventorySchema],
    ['/api/v1/clusters/c-homelab/wall', wallSchema],
  ])('%s is something the product would accept', (path, schema) => {
    const parsed = schema.safeParse(fixtures[path])
    // The error is printed in full rather than as "expected true": a fixture
    // that drifts is fixed by reading which field drifted.
    expect(parsed.success ? null : JSON.stringify(parsed.error.issues, null, 2)).toBeNull()
  })

  it('carries enough rows to make a layout measurable', () => {
    // A single row cannot show a table overflowing, and an empty one measures
    // nothing at all. These counts are what the guard's route list assumes.
    const machines = machinesSchema.parse(fixtures['/api/v1/machines'])
    const overview = kubernetesOverviewSchema.parse(
      fixtures['/api/v1/clusters/c-homelab/kubernetes'],
    )

    expect(machines.machines.length).toBeGreaterThanOrEqual(3)
    expect(overview.pods.length).toBeGreaterThanOrEqual(3)
    expect(overview.deployments.length).toBeGreaterThanOrEqual(2)
    expect(overview.services.length).toBeGreaterThanOrEqual(2)
  })

  it('uses values long enough to break a phone layout', () => {
    // Fixtures made of "foo" measure a layout nobody has. The screen the
    // operator photographed broke on a pod name and an image reference, so the
    // fixtures have to carry the real lengths.
    const overview = kubernetesOverviewSchema.parse(
      fixtures['/api/v1/clusters/c-homelab/kubernetes'],
    )

    const longestPod = Math.max(...overview.pods.map((p) => p.name.length))
    const longestImage = Math.max(...overview.deployments.map((d) => d.image.length))

    expect(longestPod).toBeGreaterThanOrEqual(24)
    expect(longestImage).toBeGreaterThanOrEqual(40)
  })
})
