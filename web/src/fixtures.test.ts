import { describe, expect, it } from 'vitest'
import { z } from 'zod'
import {
  accessControlSchema,
  appDetailSchema,
  appsSchema,
  auditPageSchema,
  clusterCapacitySchema,
  clusterEventsSchema,
  clusterNetworkSchema,
  clusterResourcesSchema,
  clusterStorageSchema,
  clustersSchema,
  clusterUsageSchema,
  hardwareSchema,
  hostSchema,
  inventorySchema,
  jobsSchema,
  kubernetesOverviewSchema,
  machineSchema,
  machinesSchema,
  nodeDetailSchema,
  noticesSchema,
  powerSchema,
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
    ['/api/v1/machines/m-cp-1/hardware', hardwareSchema],
    ['/api/v1/clusters/c-homelab/kubernetes/apps', appsSchema],
    ['/api/v1/clusters/c-homelab/kubernetes/apps/media/Deployment/jellyfin', appDetailSchema],
    ['/api/v1/machines/m-cp-1/power', powerSchema],
    ['/api/v1/clusters/c-homelab/power', powerSchema],
    ['/api/v1/clusters/c-homelab/kubernetes/apps/media/Deployment/jellyfin/power', powerSchema],
    ['/api/v1/host', hostSchema],
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

  it("describes a wall whose roll-up accounts for all of the wall's workloads", () => {
    // wallSchema takes `namespaces` as nullish, so a fixture WITHOUT the
    // roll-up parses cleanly -- and the layout audit would then photograph a
    // wall missing a third of itself and report the page as fine. That is the
    // same unperformed measurement as every other one in this file's history:
    // the schema says the shape is possible, not that the fixture is a cluster.
    const wall = wallSchema.parse(fixtures['/api/v1/clusters/c-homelab/wall'])

    expect(wall.namespaces.length).toBeGreaterThanOrEqual(4)
    expect(wall.namespaces.reduce((sum, tile) => sum + tile.total, 0)).toBe(wall.workloads.length)
    // And at least one tile that is NOT green, with the offender named: a
    // photograph of an all-green wall cannot show whether the colour or the
    // sentence ever renders.
    const troubled = wall.namespaces.filter((tile) => tile.state !== 'ok')
    expect(troubled.length).toBeGreaterThanOrEqual(1)
    for (const tile of troubled) {
      if (tile.state !== 'stopped') expect(tile.worst).not.toBe('')
    }
  })

  it('puts the host on the wall and on /host in warning, the same host both times', () => {
    // wallSchema takes `host` as nullish, so a fixture without it parses and
    // the wall is drawn without the host tile -- the layout audit and the
    // README picture would then miss the tile this phase adds, and pass.
    const wall = wallSchema.parse(fixtures['/api/v1/clusters/c-homelab/wall'])
    const host = hostSchema.parse(fixtures['/api/v1/host'])

    expect(wall.host).not.toBeNull()
    expect(wall.host?.state).toBe('warn')
    expect(host.health.state).toBe('warn')
    expect(host.device.hostname.readable && host.device.hostname.value).toBe(wall.host?.name)
    // The ▲ and the notice agree: the sensor the warning names is past its line.
    const cpu = host.live.sensors.readable
      ? host.live.sensors.value.temperatures.find((t) => t.chip === 'cpu_thermal')
      : undefined
    expect(cpu).toBeDefined()
    expect(cpu?.celsius ?? 0).toBeGreaterThanOrEqual(cpu?.warn_c ?? Number.POSITIVE_INFINITY)
    expect(host.health.warnings[0]).toMatch(/^cpu_thermal /)
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
