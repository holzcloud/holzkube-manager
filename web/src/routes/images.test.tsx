import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { FactoryExtension, FactoryVersions, Schematic } from '@/api'
import { onSudoRequired, type SudoChallenge } from '@/api'
import { ARCH_STORAGE_KEY, ImagesView } from './images'

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
    probe_reason: '',
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
  /** What GET /api/v1/schematics answers with. */
  saved?: Schematic[]
  /** Requests whose path matches are answered with this problem instead. */
  fail?: { path: string; status: number; body: unknown }
  /**
   * DELETE answers 428 until the sudo window is granted, which is what the
   * server does for a Destructive route. Nothing in this file grants it.
   */
  sudoRequired?: boolean
}

/**
 * The asset references, derived from the query the way the server derives them.
 * Deriving rather than hardcoding is the point of the architecture and
 * secure-boot cases: an assertion against a fixed string would pass just as
 * happily if the screen never sent the parameter at all.
 */
function assetsFor(id: string, arch: string, version: string, secureboot: boolean) {
  const segment = `metal-${arch}${secureboot ? '-secureboot' : ''}`
  return {
    iso: `https://factory.talos.dev/image/${id}/${version}/${segment}.iso`,
    pxe: `https://factory.talos.dev/pxe/${id}/${version}/${segment}`,
    disk_image: `https://factory.talos.dev/image/${id}/${version}/${segment}.raw.zst`,
    cmdline: `https://factory.talos.dev/image/${id}/${version}/cmdline-${segment}`,
    installer: `factory.talos.dev/metal-installer/${id}:${version}`,
  }
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
      return json(options.saved ?? [])
    }

    const assetMatch = url.pathname.match(/^\/api\/v1\/schematics\/([^/]+)\/assets$/)
    if (assetMatch !== null && assetMatch[1] !== undefined) {
      const id = assetMatch[1]
      const record = (options.saved ?? []).find((each) => each.id === id)
      return json(
        assetsFor(
          id,
          url.searchParams.get('arch') ?? '',
          url.searchParams.get('version') ?? record?.talos_version ?? 'v1.13.9',
          url.searchParams.get('secureboot') === 'true',
        ),
      )
    }

    const idMatch = url.pathname.match(/^\/api\/v1\/schematics\/([^/]+)$/)
    if (idMatch !== null && idMatch[1] !== undefined) {
      if (method === 'DELETE') {
        if (options.sudoRequired === true) {
          return new Response(
            JSON.stringify({
              type: 'https://holzkube.dev/problems/sudo',
              title: 'Re-authentication required',
              status: 428,
              detail: 'This action is destructive.',
              code: 'sudo.required',
            }),
            { status: 428, headers: { 'Content-Type': 'application/problem+json' } },
          )
        }
        return new Response(null, { status: 204 })
      }
      const record = (options.saved ?? []).find((each) => each.id === idMatch[1])
      return record === undefined
        ? json({ type: 'about:blank', title: 'no such schematic' }, 404)
        : json(record)
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

/** How many times the saved-schematics list itself was fetched. */
function listFetchCount(fetchMock: { mock: { calls: unknown[][] } }): number {
  return fetchMock.mock.calls.filter((call) => {
    const init = call[1] as RequestInit | undefined
    return String(call[0]) === '/api/v1/schematics' && (init?.method ?? 'GET') === 'GET'
  }).length
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

/**
 * The three usability states, one fixture each. They exist as three because
 * "the Factory refused" and "nobody asked" carry different repairs, and a
 * screen that showed two would send an operator to fix a schematic nothing has
 * found fault with.
 */
const USABLE = schematicFixture({
  id: 'a'.repeat(64),
  name: 'probed-good',
  usable: true,
  probed_at: '2026-08-29T10:00:00Z',
  probe_reason: '',
  extensions: ['siderolabs/intel-ucode'],
})

const REFUSED = schematicFixture({
  id: 'b'.repeat(64),
  name: 'probed-bad',
  usable: false,
  probed_at: '2026-08-29T10:05:00Z',
  probe_reason: `${'b'.repeat(64)} at v1.13.9/amd64 answered HTTP 400`,
})

const UNPROBED = schematicFixture({
  id: 'c'.repeat(64),
  name: 'never-probed',
  usable: false,
  // The zero time. Not the same statement as "probed and refused".
  probed_at: '0001-01-01T00:00:00Z',
  probe_reason: '',
})

/** Opens the detail dialog for one saved schematic. */
async function openDetail(user: ReturnType<typeof userEvent.setup>, record: Schematic) {
  await user.click(await screen.findByRole('button', { name: `Schematic ${record.name}` }))
  return within(await screen.findByRole('dialog'))
}

describe('ImagesView — the saved schematics', () => {
  beforeEach(() => {
    vi.stubGlobal('scrollTo', vi.fn())
  })

  it('renders three distinguishable usability states and the reason for a refusal', async () => {
    stubFactory({ saved: [USABLE, REFUSED, UNPROBED] })

    renderImages()

    const table = within(await screen.findByRole('table'))
    const good = within(table.getByRole('button', { name: 'Schematic probed-good' }))
    const bad = within(table.getByRole('button', { name: 'Schematic probed-bad' }))
    const unprobed = within(table.getByRole('button', { name: 'Schematic never-probed' }))

    expect(good.getByText(/Usable — the build probe confirmed it/)).toBeInTheDocument()
    expect(bad.getByText(/Not usable — the Factory refused to build it/)).toBeInTheDocument()
    expect(unprobed.getByText(/Not verified — the build probe did not run/)).toBeInTheDocument()

    // The verdict alone is not actionable; the reason is what makes it one.
    expect(bad.getByText(/answered HTTP 400/)).toBeInTheDocument()
    // And a schematic nobody probed is not accused of anything.
    expect(unprobed.queryByText(/answered HTTP/)).not.toBeInTheDocument()
  })

  it('shows the schematic id in full — it is what a machine config refers to', async () => {
    stubFactory({ saved: [USABLE] })
    const user = userEvent.setup()

    renderImages()
    const detail = await openDetail(user, USABLE)

    expect(detail.getByText(USABLE.id)).toBeInTheDocument()
    expect(detail.getByText(USABLE.canonical.trim())).toBeInTheDocument()
  })

  it('renders the installer reference on the same panel as the ISO URL', async () => {
    stubFactory({ saved: [USABLE] })
    const user = userEvent.setup()

    renderImages()
    const detail = await openDetail(user, USABLE)

    const iso = await detail.findByLabelText('ISO reference')
    const installer = detail.getByLabelText('Installer reference')

    expect(iso).toHaveTextContent(`/image/${USABLE.id}/v1.13.9/metal-amd64.iso`)
    expect(installer).toHaveTextContent(`metal-installer/${USABLE.id}:v1.13.9`)
    // PITFALLS P9(b): an ISO from one schematic and an installer from another
    // is the documented drift, so the sentence saying so is part of the panel.
    expect(detail.getByText(/must share this schematic/)).toBeInTheDocument()
  })

  it('changes every asset URL when the architecture control changes', async () => {
    stubFactory({ saved: [USABLE] })
    const user = userEvent.setup()

    renderImages()
    const detail = await openDetail(user, USABLE)

    const labels = ['ISO', 'PXE', 'Disk image', 'Kernel cmdline']
    for (const label of labels) {
      expect(await detail.findByLabelText(`${label} reference`)).toHaveTextContent('metal-amd64')
    }

    await user.click(detail.getByRole('combobox', { name: 'Architecture' }))
    await user.click(await screen.findByRole('option', { name: 'arm64' }))

    for (const label of labels) {
      await waitFor(() =>
        expect(detail.getByLabelText(`${label} reference`)).toHaveTextContent('metal-arm64'),
      )
      expect(detail.getByLabelText(`${label} reference`)).not.toHaveTextContent('metal-amd64')
    }
  })

  it('remembers the architecture rather than defaulting to the developer machine', async () => {
    localStorage.setItem(ARCH_STORAGE_KEY, 'arm64')
    stubFactory({ saved: [USABLE] })
    const user = userEvent.setup()

    renderImages()
    const detail = await openDetail(user, USABLE)

    expect(await detail.findByLabelText('ISO reference')).toHaveTextContent('metal-arm64')
  })

  /**
   * The G-02-8 regression, and the reason the two architectures were split.
   *
   * Reading somebody else's saved schematic at another architecture is a
   * question. What the operator builds is a preference. This asserts that the
   * answer to the question never rewrites the preference -- neither the
   * control the next schematic is created from, nor the value that outlives
   * the session.
   */
  it('does not let the asset panel rewrite the remembered architecture', async () => {
    localStorage.setItem(ARCH_STORAGE_KEY, 'amd64')
    stubFactory({ saved: [USABLE] })
    const user = userEvent.setup()

    renderImages()
    const detail = await openDetail(user, USABLE)
    expect(await detail.findByLabelText('ISO reference')).toHaveTextContent('metal-amd64')

    await user.click(detail.getByRole('combobox', { name: 'Architecture' }))
    await user.click(await screen.findByRole('option', { name: 'arm64' }))
    await waitFor(() =>
      expect(detail.getByLabelText('ISO reference')).toHaveTextContent('metal-arm64'),
    )

    // Close first. While the modal is open Radix marks the rest of the app
    // aria-hidden, so a role query for the form's control finds nothing and
    // the assertion below would pass for the wrong reason.
    await user.click(detail.getByRole('button', { name: 'Close' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())

    expect(screen.getByRole('combobox', { name: 'Architecture' })).toHaveTextContent('amd64')
    expect(localStorage.getItem(ARCH_STORAGE_KEY)).toBe('amd64')

    // And reopening starts the panel from the remembered value again rather
    // than from the last thing the panel happened to be asked.
    const reopened = await openDetail(user, USABLE)
    expect(await reopened.findByLabelText('ISO reference')).toHaveTextContent('metal-amd64')
  })

  it('creates the next schematic for the architecture the form shows, not the one just inspected', async () => {
    localStorage.setItem(ARCH_STORAGE_KEY, 'amd64')
    const fetchMock = stubFactory({ saved: [USABLE] })
    const user = userEvent.setup()

    renderImages()
    const detail = await openDetail(user, USABLE)
    await detail.findByLabelText('ISO reference')

    await user.click(detail.getByRole('combobox', { name: 'Architecture' }))
    await user.click(await screen.findByRole('option', { name: 'arm64' }))
    await waitFor(() =>
      expect(detail.getByLabelText('ISO reference')).toHaveTextContent('metal-arm64'),
    )

    await user.click(detail.getByRole('button', { name: 'Close' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())

    await user.type(screen.getByLabelText('Name'), 'workers')
    await user.click(screen.getByRole('button', { name: 'Create schematic' }))

    await waitFor(() => expect(bodiesPostedTo(fetchMock, '/api/v1/schematics')).toHaveLength(1))
    const posted = bodiesPostedTo(fetchMock, '/api/v1/schematics')[0] as { arch: string }
    expect(posted.arch).toBe('amd64')
  })

  it('adds the secure-boot suffix to the rendered ISO URL when the toggle is on', async () => {
    stubFactory({ saved: [USABLE] })
    const user = userEvent.setup()

    renderImages()
    const detail = await openDetail(user, USABLE)

    expect(await detail.findByLabelText('ISO reference')).toHaveTextContent('metal-amd64.iso')

    await user.click(detail.getByRole('checkbox', { name: 'SecureBoot' }))

    await waitFor(() =>
      expect(detail.getByLabelText('ISO reference')).toHaveTextContent(
        'metal-amd64-secureboot.iso',
      ),
    )
  })

  it('copies a reference to the clipboard', async () => {
    stubFactory({ saved: [USABLE] })
    const user = userEvent.setup()

    renderImages()
    const detail = await openDetail(user, USABLE)

    await detail.findByLabelText('ISO reference')
    await user.click(detail.getByRole('button', { name: 'Copy ISO' }))

    await waitFor(async () =>
      expect(await navigator.clipboard.readText()).toBe(
        `https://factory.talos.dev/image/${USABLE.id}/v1.13.9/metal-amd64.iso`,
      ),
    )

    await user.click(detail.getByRole('button', { name: 'Copy Schematic ID' }))
    await waitFor(async () => expect(await navigator.clipboard.readText()).toBe(USABLE.id))
  })

  it('deletes through the existing sudo dialog and adds no confirmation of its own', async () => {
    const fetchMock = stubFactory({ saved: [USABLE], sudoRequired: true })
    const challenges: SudoChallenge[] = []
    onSudoRequired((challenge) => {
      challenges.push(challenge)
      // Declined, so the request is not replayed. What is under test is that
      // the ask reached the existing dialog at all.
      challenge.settle(false)
    })

    const user = userEvent.setup()

    renderImages()
    const detail = await openDetail(user, USABLE)

    await user.click(detail.getByRole('button', { name: 'Delete schematic' }))

    await waitFor(() => expect(challenges).toHaveLength(1))
    // Named, so the prompt says what it is asking about.
    expect(challenges[0]?.action).toBe('Delete this schematic')

    // Exactly one DELETE was attempted, and no second confirmation of this
    // screen's own stood in front of it -- a second "are you sure?" trains the
    // operator to click past the first.
    const deletes = fetchMock.mock.calls.filter(
      (call) => (call[1] as RequestInit | undefined)?.method === 'DELETE',
    )
    expect(deletes).toHaveLength(1)
    expect(String(deletes[0]?.[0])).toBe(`/api/v1/schematics/${USABLE.id}`)

    onSudoRequired(null)
  })

  /**
   * G-02-7. The measured failure was a modal whose entire text content was the
   * word "Close" -- visibleTextLength: 5. These two assert the sentence rather
   * than a length, and assert that the two reasons are told apart: a record
   * that is gone and a fetch that failed carry different repairs.
   *
   * `fail` matches on a path prefix, so scoping it to `/api/v1/schematics/`
   * also catches the assets request for the same id. That is deliberate and
   * not a leak in the fixture: the error branch renders no asset panel, so
   * nothing asks for them.
   */
  const NOT_FOUND_PROBLEM = {
    type: 'https://holzkube.dev/problems/not-found',
    title: 'Not found',
    status: 404,
    detail: 'No such schematic.',
    code: 'notfound.schematic',
  }

  const STORE_FAILURE_PROBLEM = {
    type: 'https://holzkube.dev/problems/internal',
    title: 'Internal error',
    status: 500,
    code: 'internal.unexpected',
  }

  it('says a schematic is no longer stored instead of opening an empty dialog', async () => {
    const fetchMock = stubFactory({
      saved: [USABLE],
      fail: { path: '/api/v1/schematics/', status: 404, body: NOT_FOUND_PROBLEM },
    })
    const user = userEvent.setup()

    renderImages()
    await screen.findByRole('button', { name: `Schematic ${USABLE.name}` })
    const before = listFetchCount(fetchMock)

    const detail = await openDetail(user, USABLE)

    expect(await detail.findByRole('alert')).toHaveTextContent(/no longer stored/i)
    // The id is only recoverable from a stored record; the Factory will not
    // enumerate schematics. The dialog has to say so.
    expect(detail.getByText(/will not list schematics back/i)).toBeInTheDocument()
    expect(detail.getByRole('button', { name: 'Close' })).toBeInTheDocument()

    // Nothing that acts on a record that is not there.
    expect(detail.queryByRole('button', { name: 'Delete schematic' })).not.toBeInTheDocument()
    expect(detail.queryByText(/Factory-canonical document/)).not.toBeInTheDocument()
    expect(detail.queryByText('Assets')).not.toBeInTheDocument()

    // And the stale row is asked about again, so it leaves the list.
    await waitFor(() => expect(listFetchCount(fetchMock)).toBeGreaterThan(before))
  })

  it('distinguishes a failed fetch from a deleted schematic and keeps the row', async () => {
    const fetchMock = stubFactory({
      saved: [USABLE],
      fail: { path: '/api/v1/schematics/', status: 500, body: STORE_FAILURE_PROBLEM },
    })
    const user = userEvent.setup()

    renderImages()
    await screen.findByRole('button', { name: `Schematic ${USABLE.name}` })
    const before = listFetchCount(fetchMock)

    const detail = await openDetail(user, USABLE)

    expect(await detail.findByRole('alert')).toHaveTextContent(/could not be loaded/i)
    // Explicitly not the deletion sentence: a transport or store failure says
    // nothing about whether the record still exists.
    expect(detail.queryByText(/no longer stored/i)).not.toBeInTheDocument()
    expect(detail.queryByText(/deleted/i)).not.toBeInTheDocument()
    expect(detail.queryByRole('button', { name: 'Delete schematic' })).not.toBeInTheDocument()

    // T-02-51: dropping the row on a 500 would remove the operator's only
    // recoverable reference to the id at the moment the server is unwell.
    //
    // Closed first, for the same reason as the architecture regression above:
    // an open modal marks the rest of the app aria-hidden, so a role query for
    // the table row finds nothing while it is up.
    await user.click(detail.getByRole('button', { name: 'Close' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())

    expect(listFetchCount(fetchMock)).toBe(before)
    expect(screen.getByRole('button', { name: `Schematic ${USABLE.name}` })).toBeInTheDocument()
  })
})
