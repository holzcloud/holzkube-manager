import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { FactoryExtension, FactoryVersions, Schematic } from '@/api'
import { ImagesView } from './images'

/**
 * FACT-01, FACT-04 and FACT-05 as executed tests. The client is faked at the
 * global fetch; no server runs.
 *
 * Two of the cases here are the whole reason this screen was planned rather
 * than left to a placeholder. The version fixture ends in a release candidate,
 * because "newest" and "last" are different answers about the upstream list and
 * a picker that takes the last element preselects a prerelease. And the extension
 * control is checked for what it *cannot* do -- accept a name that did not come
 * out of the catalog -- since that is what "kein Freitextfeld" means.
 */

vi.mock('@/routes/__root', () => ({
  authenticatedRoute: { addChildren: () => undefined },
}))

/**
 * Ascending, ending in a release candidate, exactly as the Factory serves it.
 * `newest_stable` is the server's own comparison and is deliberately not the
 * last element of anything here.
 */
function versionsFixture(overrides: Partial<FactoryVersions> = {}): FactoryVersions {
  return {
    stable: ['v1.12.0', 'v1.13.8', 'v1.13.9'],
    prerelease: ['v1.14.0-rc.1', 'v1.14.0-rc.2'],
    newest_stable: 'v1.13.9',
    broken: {},
    ...overrides,
  }
}

function extension(name: string, description = ''): FactoryExtension {
  return { name, ref: `${name}:v1`, digest: `sha256:${name}`, author: 'siderolabs', description }
}

/** The extension catalog per version. A version with no entry answers empty. */
type Catalog = Record<string, FactoryExtension[]>

const DEFAULT_CATALOG: Catalog = {
  'v1.13.9': [
    extension('siderolabs/intel-ucode', 'Intel CPU microcode'),
    extension('siderolabs/gvisor', 'gVisor container runtime'),
  ],
  'v1.13.8': [extension('siderolabs/intel-ucode', 'Intel CPU microcode')],
  'v1.12.0': [extension('siderolabs/intel-ucode', 'Intel CPU microcode')],
}

function schematicFixture(overrides: Partial<Schematic> = {}): Schematic {
  return {
    id: 'a'.repeat(64),
    cluster: '',
    name: 'workers',
    talos_version: 'v1.13.9',
    canonical: 'customization: {}\n',
    extensions: [],
    kernel_args: [],
    meta: [],
    usable: true,
    probed_at: '2026-08-29T10:00:00Z',
    created_at: '2026-08-29T10:00:00Z',
    rev: 1,
    ...overrides,
  }
}

interface StubOptions {
  versions?: FactoryVersions
  catalog?: Catalog
  /** The 201 body for POST /api/v1/schematics, minus the warnings. */
  created?: Schematic
  /** Requests whose path matches are answered with this problem instead. */
  fail?: { path: string; status: number; body: unknown }
}

/**
 * Routes by URL rather than by call order: this screen fires the version and
 * catalog queries concurrently, so a queue keyed on order would assert the
 * scheduler rather than the code.
 */
function stubFactory(options: StubOptions = {}) {
  const versions = options.versions ?? versionsFixture()
  const catalog = options.catalog ?? DEFAULT_CATALOG

  const fetchMock = vi.fn(async (input: string, init?: RequestInit) => {
    const url = new URL(String(input), 'https://127.0.0.1:8443')
    const method = init?.method ?? 'GET'

    const json = (body: unknown, status = 200) =>
      new Response(JSON.stringify(body), {
        status,
        headers: { 'Content-Type': 'application/json' },
      })

    if (options.fail !== undefined && url.pathname.startsWith(options.fail.path)) {
      return new Response(JSON.stringify(options.fail.body), {
        status: options.fail.status,
        headers: { 'Content-Type': 'application/problem+json' },
      })
    }

    if (url.pathname === '/api/v1/factory/versions') {
      return json(versions)
    }
    if (url.pathname === '/api/v1/factory/extensions') {
      const version = url.searchParams.get('version') ?? ''
      return json({ version, extensions: catalog[version] ?? [] })
    }
    if (url.pathname === '/api/v1/schematics' && method === 'POST') {
      const body = JSON.parse(String(init?.body)) as { name: string; talos_version: string }
      const record = options.created ?? schematicFixture()
      return json(
        { ...record, name: body.name, talos_version: body.talos_version, warnings: [] },
        201,
      )
    }
    if (url.pathname === '/api/v1/schematics' && method === 'GET') {
      return json([])
    }

    return json({ type: 'about:blank', title: 'unrouted in this test' }, 500)
  })

  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function bodiesPostedTo(fetchMock: { mock: { calls: unknown[][] } }, path: string): unknown[] {
  return fetchMock.mock.calls
    .filter((call) => {
      const init = call[1] as RequestInit | undefined
      return String(call[0]) === path && (init?.method ?? 'GET') === 'POST'
    })
    .map((call) => JSON.parse(String((call[1] as RequestInit).body)))
}

function renderImages() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ImagesView />
    </QueryClientProvider>,
  )
}

/** Waits for the extension catalog for the default version to be on screen. */
async function catalogLoaded() {
  return within(await screen.findByRole('group', { name: 'System extensions' }))
}

describe('ImagesView — the authoring half', () => {
  beforeEach(() => {
    vi.stubGlobal('scrollTo', vi.fn())
  })

  it('defaults to the newest stable version, not the last element of the upstream list', async () => {
    stubFactory()

    renderImages()

    const picker = await screen.findByRole('combobox', { name: 'Talos version' })
    await waitFor(() => expect(picker).toHaveTextContent('v1.13.9'))
    // The upstream list ends here; preselecting it is the documented trap.
    expect(picker).not.toHaveTextContent('v1.14.0-rc.2')
  })

  it('hides prereleases until they are explicitly asked for', async () => {
    stubFactory()
    const user = userEvent.setup()

    renderImages()
    await catalogLoaded()

    await user.click(screen.getByRole('combobox', { name: 'Talos version' }))
    expect(screen.queryByRole('option', { name: /v1\.14\.0-rc\.1/ })).not.toBeInTheDocument()
    await user.keyboard('{Escape}')

    await user.click(screen.getByRole('checkbox', { name: /Show pre-release versions/ }))
    await user.click(screen.getByRole('combobox', { name: 'Talos version' }))
    expect(await screen.findByRole('option', { name: /v1\.14\.0-rc\.1/ })).toBeInTheDocument()
  })

  it('renders a broken version disabled and says why it is listed', async () => {
    stubFactory({
      versions: versionsFixture({
        broken: { 'v1.12.0': 'metal-installer does not resolve for this version' },
      }),
    })
    const user = userEvent.setup()

    renderImages()
    await catalogLoaded()

    await user.click(screen.getByRole('combobox', { name: 'Talos version' }))

    const broken = await screen.findByRole('option', { name: /v1\.12\.0/ })
    expect(broken).toHaveAttribute('aria-disabled', 'true')
    expect(broken).toHaveTextContent('metal-installer does not resolve for this version')
  })

  it('offers extensions only from the catalog and has no field to type one into', async () => {
    stubFactory()

    renderImages()
    const picker = await catalogLoaded()

    const offered = picker.getAllByRole('checkbox').map((box) => box.closest('label')?.textContent)
    expect(offered).toHaveLength(2)
    expect(offered?.[0]).toContain('siderolabs/intel-ucode')
    expect(offered?.[1]).toContain('siderolabs/gvisor')

    // FACT-01: the catalog is the only source of an extension name, so the
    // picker contains no textbox an unvalidated name could be typed into.
    expect(picker.queryAllByRole('textbox')).toHaveLength(0)
    expect(picker.queryAllByRole('combobox')).toHaveLength(0)
  })

  it('sends only names that came out of the fetched catalog', async () => {
    const fetchMock = stubFactory()
    const user = userEvent.setup()

    renderImages()
    const picker = await catalogLoaded()

    await user.type(screen.getByLabelText('Name'), 'workers')
    await user.click(picker.getByRole('checkbox', { name: /siderolabs\/gvisor/ }))
    await user.click(screen.getByRole('button', { name: 'Create schematic' }))

    await waitFor(() => expect(bodiesPostedTo(fetchMock, '/api/v1/schematics')).toHaveLength(1))

    const [body] = bodiesPostedTo(fetchMock, '/api/v1/schematics') as [
      { extensions: string[]; talos_version: string },
    ]
    const catalogNames = DEFAULT_CATALOG['v1.13.9']?.map((each) => each.name) ?? []
    expect(body.extensions).toEqual(['siderolabs/gvisor'])
    for (const sent of body.extensions) {
      expect(catalogNames).toContain(sent)
    }
    expect(body.talos_version).toBe('v1.13.9')
  })

  it('reports by name an extension the new version does not have, rather than carrying it over', async () => {
    stubFactory()
    const user = userEvent.setup()

    renderImages()
    const picker = await catalogLoaded()

    await user.click(picker.getByRole('checkbox', { name: /siderolabs\/gvisor/ }))

    await user.click(screen.getByRole('combobox', { name: 'Talos version' }))
    await user.click(await screen.findByRole('option', { name: /v1\.13\.8/ }))

    const report = await screen.findByText(/Removed from the selection/)
    expect(report).toHaveTextContent('siderolabs/gvisor')

    const after = await catalogLoaded()
    expect(after.getByRole('checkbox', { name: /siderolabs\/intel-ucode/ })).not.toBeChecked()
  })

  it('warns about kernel arguments while they are being typed, before any create request', async () => {
    const fetchMock = stubFactory()
    const user = userEvent.setup()

    renderImages()
    await catalogLoaded()

    expect(screen.queryByRole('alert', { name: 'Schematic warnings' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Add kernel argument' }))
    await user.type(screen.getByLabelText('Kernel argument 1'), 'console=ttyS0')

    const warning = await screen.findByRole('alert', { name: 'Schematic warnings' })
    expect(warning).toHaveTextContent('schematic.installer-ignores-kernel-args')
    expect(warning).toHaveTextContent(/installer and initramfs images honour system extensions/)

    // FACT-04 says beim Autoren: the warning is on screen and nothing has been
    // created. A warning that only arrives with the 201 arrives too late.
    expect(bodiesPostedTo(fetchMock, '/api/v1/schematics')).toHaveLength(0)
  })

  it('warns about META values while they are being typed', async () => {
    stubFactory()
    const user = userEvent.setup()

    renderImages()
    await catalogLoaded()

    await user.click(screen.getByRole('button', { name: 'Add META value' }))
    await user.type(screen.getByLabelText('META value 1'), 'something')

    const warning = await screen.findByRole('alert', { name: 'Schematic warnings' })
    expect(warning).toHaveTextContent('schematic.installer-ignores-meta')
  })

  it('keeps "created" and "usable" apart on the create result', async () => {
    stubFactory({
      created: schematicFixture({ usable: false, probed_at: '2026-08-29T10:00:00Z' }),
    })
    const user = userEvent.setup()

    renderImages()
    await catalogLoaded()

    await user.type(screen.getByLabelText('Name'), 'workers')
    await user.click(screen.getByRole('button', { name: 'Create schematic' }))

    expect(await screen.findByText('Schematic created.')).toBeInTheDocument()
    expect(screen.getByText(/Not usable/)).toBeInTheDocument()
  })

  it('offers nothing rather than a stale catalog when the catalog cannot be fetched', async () => {
    stubFactory({
      fail: {
        path: '/api/v1/factory/extensions',
        status: 502,
        body: {
          type: 'https://holzkube.dev/problems/upstream',
          title: 'The Image Factory did not answer usably.',
          status: 502,
          code: 'upstream.factory-unavailable',
        },
      },
    })

    renderImages()

    expect(
      await screen.findByText(/extension catalog for v1\.13\.9 could not be fetched/),
    ).toBeInTheDocument()
    expect(screen.queryByRole('group', { name: 'System extensions' })).not.toBeInTheDocument()
  })
})
