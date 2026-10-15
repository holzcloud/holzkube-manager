import { createMemoryHistory, createRouter, RouterProvider } from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  api,
  type ConfigDriftDetail,
  clustersSchema,
  configDriftDetailSchema,
  machineSchema,
} from '@/api'
import { queryClient } from '@/routes/__root'
import { routeTree } from '@/routeTree'
import demo from '../../fixtures/demo.json'

/**
 * Where the configuration drift is wired in (15-11, verification warning W1).
 *
 * The verification removed `<ConfigDriftBanner>` from AppShell.tsx and then
 * `<NodeConfigDrift>` from node-detail.tsx, and the suite stayed green at 927 of
 * 927 each time: both components have tests of their own, their use had none.
 * This renders the real route tree at a node page, so the shell that is the
 * authenticated layout and the page that is the node route are the ones
 * production builds, with the server's answers mirrored and nothing else
 * reachable.
 *
 * What it holds is that the banner stands in the shell and the section stands on
 * the node page with the machine of that page (D-06, D-05). What it does not
 * hold is how either looks, their grid, or their touch targets: that is the
 * browser project and the layout guard, which do not run here.
 */

const MACHINE = 'm-cp-1'
const BANNER_SENTENCE =
  'cp-1 has differed from its baseline for more than a day. Look at it, or confirm it as intended.'
const SECTION_SENTENCE =
  'This node differs from its baseline and nothing this installation did explains it.'
const DIGEST = 'cd'.repeat(32)

const fixtures = demo as Record<string, unknown>

class QuietSource {
  static readonly CLOSED = 2
  readyState = 0
  onopen = null
  onmessage = null
  onerror = null
  addEventListener() {}
  close() {}
}

function drift(): ConfigDriftDetail {
  return configDriftDetailSchema.parse({
    machine: MACHINE,
    state: 'drift',
    sentence: SECTION_SENTENCE,
    notice: 'A change of nothing but a secret value is not visible here.',
    checked_at: '2026-10-12T09:00:00Z',
    read_at: '2026-10-12T09:00:00Z',
    baseline_taken_at: '2026-10-01T08:00:00Z',
    baseline_source: 'first-sight',
    drift_since: '2026-10-10T07:30:00Z',
    digest: DIGEST,
    diff: {
      changes: [
        {
          path: 'KubeletConfig.image',
          kind: 'modified',
          before: 'registry.example.com/kubelet:v1.34.0',
          after: 'registry.example.com/kubelet:v1.35.0',
        },
      ],
      paths: ['KubeletConfig.image'],
    },
  })
}

/** Every answer the shell and the node page ask for, and nothing else. */
function mirror() {
  vi.spyOn(api, 'status').mockResolvedValue({
    setup_required: false,
    audit_chain: { ok: true, broken_at_line: 0, file: '' },
    oidc_enabled: false,
    password_login: true,
    local_sign_in_url: '',
    talos_range: '',
    allow_prerelease: false,
  } as Awaited<ReturnType<typeof api.status>>)
  vi.spyOn(api, 'me').mockResolvedValue({
    id: 'u-1',
    username: 'operator',
    dry_run: false,
    role: 'admin',
    sso: false,
  })

  // One cluster of the fixture, so the banner's sentence stands once.
  const clusters = clustersSchema
    .parse(fixtures['/api/v1/clusters'])
    .clusters.slice(0, 1)
    .map((c) => ({
      ...c,
      config_drift: {
        checked_at: '2026-10-12T09:05:00Z',
        nodes: 1,
        read_nodes: 1,
        drifted: 1,
        banner: true,
        warning: BANNER_SENTENCE,
        items: [
          {
            machine: MACHINE,
            node: 'cp-1',
            state: 'drift' as const,
            since: '2026-10-10T07:30:00Z',
            sentence: 'differs',
          },
        ],
      },
    }))
  vi.spyOn(api.clusters, 'list').mockResolvedValue(clusters)
  vi.spyOn(api.machines, 'get').mockResolvedValue(
    machineSchema.parse(fixtures[`/api/v1/machines/${MACHINE}`]),
  )
  return vi.spyOn(api.configDrift, 'get').mockResolvedValue(drift())
}

function renderNodePage() {
  const router = createRouter({
    routeTree,
    history: createMemoryHistory({ initialEntries: [`/nodes/${MACHINE}`] }),
  })
  return render(<RouterProvider router={router} />)
}

beforeEach(() => {
  // The client belongs to the root module and outlives a test.
  queryClient.clear()
  vi.stubGlobal('EventSource', QuietSource)
  // Whatever is not mirrored fails loudly with its address, never reaches a network.
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL) => {
      throw new Error(`drift-wiring: an unmirrored request to ${String(input)}`)
    }),
  )
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('where the configuration drift is used', () => {
  it('the shell shows the configuration banner on a node page', async () => {
    mirror()
    renderNodePage()

    const sentence = await screen.findByText(BANNER_SENTENCE)
    expect(sentence.closest('[role="status"]')).not.toBeNull()
  })

  it('the node page asks for the configuration of its own machine and shows it', async () => {
    const get = mirror()
    renderNodePage()

    expect(await screen.findByText(SECTION_SENTENCE)).toBeInTheDocument()
    expect(get).toHaveBeenCalledWith(MACHINE)
    expect(screen.getByRole('button', { name: 'Confirm as intended' })).toBeInTheDocument()
  })
})
