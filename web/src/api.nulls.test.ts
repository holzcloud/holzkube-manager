import { describe, expect, it } from 'vitest'
import {
  accessControlSchema,
  clusterNetworkSchema,
  clusterStorageSchema,
  inventorySchema,
} from '@/api'

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
})
