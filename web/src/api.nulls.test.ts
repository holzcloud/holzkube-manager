import { describe, expect, it } from 'vitest'
import {
  accessControlSchema,
  clusterNetworkSchema,
  clusterSchema,
  clusterStorageSchema,
  configDriftDetailSchema,
  hostSchema,
  inventorySchema,
} from '@/api'
import demo from '../fixtures/demo.json'

/**
 * A null list is an empty list (2026-09-20).
 *
 * The operator opened /kubernetes and got a screenful of zod errors: the answer
 * carried `"quotas": null`, because a nil slice in Go marshals to null and every
 * namespace without a ResourceQuota had one. `.default([])` fills a MISSING field
 * and does nothing for an explicit null, so the parse failed and the page was an
 * error message.
 *
 * The server is fixed and guarded. These are about the second half: a null must
 * never again be able to turn a page into an error.
 */
describe('a null list', () => {
  it('parses as an empty list, which is what the operator hit', () => {
    const parsed = inventorySchema.safeParse({
      namespaces: [{ name: 'default', phase: 'Active', pods_running: 0, quotas: null }],
      custom_kinds: null,
      notice: '',
    })
    expect(parsed.success ? null : JSON.stringify(parsed.error.issues)).toBeNull()
    expect(parsed.success && parsed.data.namespaces[0]?.quotas).toEqual([])
    expect(parsed.success && parsed.data.custom_kinds).toEqual([])
  })

  it('is tolerated on every other answer built in this milestone', () => {
    for (const schema of [clusterStorageSchema, clusterNetworkSchema, accessControlSchema]) {
      const parsed = schema.safeParse({
        volumes: null,
        claims: null,
        classes: null,
        services: null,
        policies: null,
        unprotected: null,
        ingress_classes: null,
        bindings: null,
        roles: null,
        accounts: null,
        administrators: null,
        notice: '',
      })
      expect(parsed.success ? null : JSON.stringify(parsed.error.issues)).toBeNull()
    }
  })

  it('still fills a missing list, which is what it always did', () => {
    const parsed = inventorySchema.safeParse({ notice: '' })
    expect(parsed.success && parsed.data.namespaces).toEqual([])
  })

  it('reads a null or absent actions.outdated as nothing outdated (13-14)', () => {
    const host = structuredClone((demo as Record<string, unknown>)['/api/v1/host']) as Record<
      string,
      unknown
    >
    const actions = host.actions as Record<string, unknown>
    for (const outdated of [null, undefined]) {
      const parsed = hostSchema.safeParse({ ...host, actions: { ...actions, outdated } })
      expect(parsed.success ? null : JSON.stringify(parsed.error.issues)).toBeNull()
      expect(parsed.success && parsed.data.actions.outdated).toEqual([])
    }
  })

  it('reads a drift detail with null or missing diff lists as empty lists (15-09)', () => {
    const detail = {
      machine: 'm-1',
      state: 'drift',
      sentence: 'This node differs from its baseline.',
      digest: 'ab'.repeat(32),
      notice: 'A rotated secret is not visible here.',
    }
    for (const diff of [{ changes: null, paths: null }, {}]) {
      const parsed = configDriftDetailSchema.safeParse({ ...detail, diff })
      expect(parsed.success ? null : JSON.stringify(parsed.error.issues)).toBeNull()
      expect(parsed.success && parsed.data.diff?.changes).toEqual([])
      expect(parsed.success && parsed.data.diff?.paths).toEqual([])
    }
    const none = configDriftDetailSchema.safeParse({ ...detail, diff: null })
    expect(none.success ? null : JSON.stringify(none.error.issues)).toBeNull()
    expect(none.success && none.data.diff).toBeFalsy()
  })

  it('reads a cluster config_drift with null or missing items as an empty list (15-09)', () => {
    const cluster = (demo as unknown as { '/api/v1/clusters': { clusters: unknown[] } })[
      '/api/v1/clusters'
    ].clusters[0] as Record<string, unknown>
    const drift = { checked_at: '2026-10-12T09:05:00Z', nodes: 0, read_nodes: 0, drifted: 0 }
    for (const items of [null, undefined, []]) {
      const parsed = clusterSchema.safeParse({ ...cluster, config_drift: { ...drift, items } })
      expect(parsed.success ? null : JSON.stringify(parsed.error.issues)).toBeNull()
      expect(parsed.success && parsed.data.config_drift?.items).toEqual([])
    }
  })
})
