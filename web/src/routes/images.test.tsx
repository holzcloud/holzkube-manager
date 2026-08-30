import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { FactoryExtension, FactoryVersions, Schematic, SchematicWarning } from '@/api'
import {
  onSudoRequired,
  type SudoChallenge,
  WARNING_INSTALLER_REPO_FALLBACK_UNVERIFIED,
} from '@/api'
import { CODE_UPSTREAM_FACTORY_REJECTED, CODE_UPSTREAM_FACTORY_UNAVAILABLE } from '@/lib/problem'
import { problemType } from '@/test/problem-fixtures'
import { ARCH_STORAGE_KEY, hasControlCharacter, ImagesView } from './images'

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
    arch: 'amd64',
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
  /**
   * Warnings the assets route answers with. They are about *this resolution* —
   * how the installer repository name was obtained — rather than about the
   * schematic, so they are a property of the answer and not of the record.
   */
  assetWarnings?: SchematicWarning[]
  /**
   * The unresolved-installer outcome: a `200` whose four registry-free
   * references are present and whose `installer` is null, carrying the code and
   * detail of the problem the route would otherwise have answered with.
   *
   * A separate option rather than an override of `assetWarnings`, because the
   * two are different statements. A warning is about a reference that exists;
   * this is about one that does not.
   */
  installerError?: { code: string; detail: string }
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
function assetsFor(
  id: string,
  arch: string,
  version: string,
  secureboot: boolean,
  warnings: SchematicWarning[] = [],
  installerError?: { code: string; detail: string },
) {
  const segment = `metal-${arch}${secureboot ? '-secureboot' : ''}`
  // The installer repository name carries the SecureBoot selection too, because
  // that is the only thing that selects it — plan 02-09 made the server
  // SecureBoot-aware here, and a stub that kept handing back the ordinary name
  // would be telling a different story than the server does.
  const installerRepo = `metal-installer${secureboot ? '-secureboot' : ''}`
  const references = {
    iso: `https://factory.talos.dev/image/${id}/${version}/${segment}.iso`,
    pxe: `https://factory.talos.dev/pxe/${id}/${version}/${segment}`,
    disk_image: `https://factory.talos.dev/image/${id}/${version}/${segment}.raw.zst`,
    cmdline: `https://factory.talos.dev/image/${id}/${version}/cmdline-${segment}`,
  }
  // The four references are unchanged in the unresolved case, and that is the
  // point of the outcome rather than an incidental property of the stub: they
  // are functions of the request and never touched the registry, so a stub that
  // dropped them here would be reproducing the bug instead of the server.
  if (installerError !== undefined) {
    return { ...references, installer: null, installer_error: installerError, warnings }
  }
  return {
    ...references,
    installer: `factory.talos.dev/${installerRepo}/${id}:${version}`,
    warnings,
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
          options.assetWarnings ?? [],
          options.installerError,
        ),
      )
    }

    const idMatch = url.pathname.match(/^\/api\/v1\/schematics\/([^/]+)$/)
    if (idMatch !== null && idMatch[1] !== undefined) {
      if (method === 'DELETE') {
        if (options.sudoRequired === true) {
          return new Response(
            JSON.stringify({
              type: problemType('sudo'),
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

  it('stays quiet about a row that has just been added and holds nothing', async () => {
    // The live panel predicts on what submit would send, which drops blank
    // rows. predictWarnings itself asks the server's question -- is there an
    // entry -- so the filtering has to happen here or adding a row would warn
    // about a value nobody has typed yet.
    stubFactory()
    const user = userEvent.setup()

    renderImages()
    await catalogLoaded()

    await user.click(screen.getByRole('button', { name: 'Add kernel argument' }))
    await user.click(screen.getByRole('button', { name: 'Add META value' }))

    expect(await screen.findByLabelText('Kernel argument 1')).toHaveValue('')
    expect(screen.queryByRole('alert', { name: 'Schematic warnings' })).not.toBeInTheDocument()
  })

  /**
   * G-02-6, the client half. The server refuses these values in its own
   * canonical serialiser before anything crosses the network, so a value that
   * reaches the request can only ever come back as an error. Refusing it here
   * means the operator is told which row is wrong while they are still looking
   * at it, and the server's 400 is a backstop rather than the first line of
   * defence.
   *
   * userEvent.type cannot produce a control character -- it models keystrokes,
   * and there is no key for U+0007. fireEvent.change sets the value the way a
   * paste does, which is how such a character actually arrives in a form.
   */
  it('refuses a kernel argument carrying a control character before any request', async () => {
    const fetchMock = stubFactory()
    const user = userEvent.setup()

    renderImages()
    await catalogLoaded()

    await user.type(screen.getByLabelText('Name'), 'bell')
    await user.click(screen.getByRole('button', { name: 'Add kernel argument' }))
    fireEvent.change(screen.getByLabelText('Kernel argument 1'), {
      target: { value: 'console=ttyS0\u0007' },
    })

    const alert = await screen.findByRole('alert', { name: 'Kernel argument 1 is not usable' })
    expect(alert).toHaveTextContent(/control character/i)
    expect(screen.getByRole('button', { name: 'Create schematic' })).toBeDisabled()

    // The button being disabled is not the claim; the claim is that nothing was
    // sent. A submit path that bypassed the button would still be caught here.
    await user.click(screen.getByRole('button', { name: 'Create schematic' }))
    expect(bodiesPostedTo(fetchMock, '/api/v1/schematics')).toHaveLength(0)
  })

  it('re-enables Create once the control character is removed', async () => {
    const fetchMock = stubFactory()
    const user = userEvent.setup()

    renderImages()
    await catalogLoaded()

    await user.type(screen.getByLabelText('Name'), 'bell')
    await user.click(screen.getByRole('button', { name: 'Add kernel argument' }))
    const row = screen.getByLabelText('Kernel argument 1')
    fireEvent.change(row, { target: { value: 'console=ttyS0\u0007' } })

    await screen.findByRole('alert', { name: 'Kernel argument 1 is not usable' })

    fireEvent.change(row, { target: { value: 'console=ttyS0' } })

    await waitFor(() =>
      expect(
        screen.queryByRole('alert', { name: 'Kernel argument 1 is not usable' }),
      ).not.toBeInTheDocument(),
    )
    expect(screen.getByRole('button', { name: 'Create schematic' })).toBeEnabled()

    await user.click(screen.getByRole('button', { name: 'Create schematic' }))
    await waitFor(() => expect(bodiesPostedTo(fetchMock, '/api/v1/schematics')).toHaveLength(1))
  })

  it('refuses a META value carrying a control character before any request', async () => {
    const fetchMock = stubFactory()
    const user = userEvent.setup()

    renderImages()
    await catalogLoaded()

    await user.type(screen.getByLabelText('Name'), 'bell')
    await user.click(screen.getByRole('button', { name: 'Add META value' }))
    fireEvent.change(screen.getByLabelText('META value 1'), {
      target: { value: 'something\u0007' },
    })

    const alert = await screen.findByRole('alert', { name: 'META value 1 is not usable' })
    expect(alert).toHaveTextContent(/control character/i)
    expect(screen.getByRole('button', { name: 'Create schematic' })).toBeDisabled()
    expect(bodiesPostedTo(fetchMock, '/api/v1/schematics')).toHaveLength(0)
  })

  /**
   * The second half of G-02-6's artifacts note: the create error stayed on
   * screen through subsequent unrelated interactions, so an operator editing
   * the form was reading a sentence about a request they had already replaced.
   */
  it('clears a create error as soon as the form it belongs to changes', async () => {
    stubFactory({
      fail: {
        path: '/api/v1/schematics',
        status: 502,
        body: {
          type: problemType('upstream'),
          title: 'The Image Factory did not answer usably.',
          status: 502,
          detail: 'The Image Factory did not answer usably: creating the schematic.',
          code: 'upstream.factory-unavailable',
        },
      },
    })
    const user = userEvent.setup()

    renderImages()

    await user.type(screen.getByLabelText('Name'), 'workers')
    await user.click(screen.getByRole('button', { name: 'Create schematic' }))

    const failure = await screen.findByRole('alert', { name: 'Create failed' })
    expect(failure).toBeInTheDocument()

    await user.type(screen.getByLabelText('Name'), '-2')

    await waitFor(() =>
      expect(screen.queryByRole('alert', { name: 'Create failed' })).not.toBeInTheDocument(),
    )
  })

  it('hasControlCharacter transcribes the server rule and nothing wider', () => {
    // The server's representable() refuses a string that is not valid UTF-8,
    // any rune below U+0020, and U+007F. All three, and nothing else.
    expect(hasControlCharacter('console=ttyS0')).toBe(false)
    expect(hasControlCharacter('')).toBe(false)
    expect(hasControlCharacter('ümlaut — dash')).toBe(false)
    expect(hasControlCharacter('a\u0000b')).toBe(true)
    expect(hasControlCharacter('a\tb')).toBe(true)
    expect(hasControlCharacter('a\nb')).toBe(true)
    expect(hasControlCharacter('a\u001fb')).toBe(true)
    expect(hasControlCharacter('a\u007fb')).toBe(true)
    // U+0080 is not in the server's set, and a client that refused it would
    // reject input the server accepts.
    expect(hasControlCharacter('a\u0080b')).toBe(false)

    // The other half of representable: not valid UTF-8. A lone surrogate is
    // the only way a browser produces one, and it is the half the server
    // cannot enforce -- JSON.stringify emits it as a well-formed \udXXX
    // escape and Go decodes an unpaired escape to U+FFFD, so representable is
    // handed a clean string and the id is computed over a character the
    // operator never typed.
    expect(hasControlCharacter('a\ud800b')).toBe(true)
    expect(hasControlCharacter('a\udfffb')).toBe(true)
    // A well-formed pair is an ordinary character and must stay accepted, or
    // this would refuse input the server takes.
    expect(hasControlCharacter('a\ud83d\ude00b')).toBe(false)
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
          type: problemType('upstream'),
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
  arch: 'amd64',
})

const REFUSED = schematicFixture({
  id: 'b'.repeat(64),
  name: 'probed-bad',
  usable: false,
  probed_at: '2026-08-29T10:05:00Z',
  probe_reason: `${'b'.repeat(64)} at v1.13.9/amd64 answered HTTP 400`,
  // arm64, so the row assertions below cannot pass against a hardcoded amd64
  // or against a qualifier read from anything but this record.
  arch: 'arm64',
})

const UNPROBED = schematicFixture({
  id: 'c'.repeat(64),
  name: 'never-probed',
  usable: false,
  // The zero time. Not the same statement as "probed and refused".
  probed_at: '0001-01-01T00:00:00Z',
  probe_reason: '',
  // A record with no verdict still has an architecture it was asked about.
  arch: 'arm64',
})

/**
 * A record written before model.Schematic.Arch existed. Its architecture is
 * empty forever -- there is nothing on the record to recover it from -- and
 * those are precisely the records the G-02-8 leak produced.
 */
const PRE_ARCH = schematicFixture({
  id: 'd'.repeat(64),
  name: 'pre-arch',
  usable: true,
  probed_at: '2026-08-29T10:10:00Z',
  probe_reason: '',
  arch: '',
})

/**
 * G-02-14. A record that claims usability while carrying no probe timestamp.
 *
 * No request path produces one today -- the POST handler writes `usable` and
 * `probed_at` together -- which is exactly why the two of these are here. A
 * migration, an import or a hand-edited store file is not bound by what the
 * handler happens to do, and the badge is what an operator reads either way.
 *
 * Two fixtures rather than one parameterised fixture, because the two zero
 * forms arrive from different places: the empty string is what a hand-written
 * or partially-migrated JSON record carries, the `0001-01-01` timestamp is what
 * a decoded zero `time.Time` serialises to. `isProbed` accepts both, and a test
 * that covered one while claiming both is how it would quietly lose half its
 * job.
 */
const CLAIMED_UNPROBED_EMPTY = schematicFixture({
  id: 'e'.repeat(64),
  name: 'claims-no-timestamp',
  usable: true,
  probed_at: '',
  probe_reason: '',
  arch: 'arm64',
})

const CLAIMED_UNPROBED_ZERO_TIME = schematicFixture({
  id: 'f'.repeat(64),
  name: 'claims-zero-time',
  usable: true,
  probed_at: '0001-01-01T00:00:00Z',
  probe_reason: '',
  arch: 'amd64',
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
    stubFactory({ saved: [USABLE, REFUSED, UNPROBED, PRE_ARCH] })

    renderImages()

    const table = within(await screen.findByRole('table'))
    const good = within(table.getByRole('button', { name: 'Schematic probed-good' }))
    const bad = within(table.getByRole('button', { name: 'Schematic probed-bad' }))
    const unprobed = within(table.getByRole('button', { name: 'Schematic never-probed' }))
    const preArch = within(table.getByRole('button', { name: 'Schematic pre-arch' }))

    expect(good.getByText(/Usable — the build probe confirmed it/)).toBeInTheDocument()
    expect(bad.getByText(/Not usable — the Factory refused to build it/)).toBeInTheDocument()
    // G-02-1: the old copy asserted the probe "did not run". On the measured
    // common case it ran for a full thirty seconds and gave up, so the badge was
    // stating as fact something the record cannot support. The new wording
    // asserts neither, and the muted line below it states the disjunction.
    expect(unprobed.getByText(/Not verified — the build probe has no verdict/)).toBeInTheDocument()
    // The old copy asserted it as fact. The disjunction below still contains the
    // words "did not run", and must: it is one of the two possibilities. What
    // must be gone is the badge stating it as the answer.
    expect(unprobed.queryByText(/the build probe did not run/)).not.toBeInTheDocument()
    expect(unprobed.getByText(/either did not run or did not answer in time/)).toBeInTheDocument()
    expect(unprobed.getByText(/may still be buildable/)).toBeInTheDocument()

    // The verdict alone is not actionable; the reason is what makes it one.
    expect(bad.getByText(/answered HTTP 400/)).toBeInTheDocument()
    // And a schematic nobody probed is not accused of anything.
    expect(unprobed.queryByText(/answered HTTP/)).not.toBeInTheDocument()

    // G-02-8: each row's verdict names the architecture it is about, read from
    // that row's own record. The three fixtures do not share an architecture, so
    // a qualifier taken from anywhere else fails here.
    expect(good.getByText('architecture: amd64')).toBeInTheDocument()
    expect(bad.getByText('architecture: arm64')).toBeInTheDocument()
    expect(unprobed.getByText('architecture: arm64')).toBeInTheDocument()

    // The regression guard for every record that exists today: written before
    // the field did, unqualified forever, and not dressed up with a guess.
    expect(preArch.getByText(/Usable — the build probe confirmed it/)).toBeInTheDocument()
    expect(preArch.queryByText(/^architecture:/)).not.toBeInTheDocument()
  })

  it('names the architecture the new schematic was created at on the result panel', async () => {
    // The creation result is where an operator sees the verdict first, and it is
    // the one place the architecture was chosen rather than read back.
    stubFactory({ saved: [], created: schematicFixture({ name: 'fresh', arch: 'arm64' }) })
    const user = userEvent.setup()

    renderImages()
    await catalogLoaded()

    await user.type(screen.getByLabelText('Name'), 'fresh')
    await user.click(screen.getByRole('button', { name: 'Create schematic' }))

    const panel = await screen.findByText('Schematic created.')
    const row = within(panel.parentElement as HTMLElement)
    expect(row.getByText(/Usable — the build probe confirmed it/)).toBeInTheDocument()
    expect(row.getByText('architecture: arm64')).toBeInTheDocument()
  })

  it('names the architecture the verdict is about, beside the verdict', async () => {
    // G-02-8's second half. "Usable — the build probe confirmed it" is a claim
    // whose subject is missing until the record says which architecture was
    // probed. arm64 and not amd64 on purpose: the fixture default is amd64, so
    // an amd64 assertion would pass against a hardcoded qualifier.
    const arm = schematicFixture({ name: 'arm-workers', arch: 'arm64' })
    stubFactory({ saved: [arm] })
    const user = userEvent.setup()

    renderImages()
    const detail = await openDetail(user, arm)

    expect(detail.getByText(/Usable — the build probe confirmed it/)).toBeInTheDocument()
    expect(detail.getByText('architecture: arm64')).toBeInTheDocument()
  })

  it('refuses a usable claim from a record with an empty probe timestamp', async () => {
    // G-02-14, first zero form. `usable: true` with `probed_at: ''` is a record
    // asserting a confirmation nothing gave it, and the badge used to repeat the
    // assertion verbatim: `UsabilityVerdict` tested `usable` before it tested
    // whether the record held a verdict at all, so `isProbed` was never reached
    // on the one branch where an overclaim does damage -- it sends an operator
    // to install from an image no probe ever built.
    //
    // T-02-62: no state may make a stronger claim than the record supports.
    stubFactory({ saved: [CLAIMED_UNPROBED_EMPTY] })
    const user = userEvent.setup()

    renderImages()

    const table = within(await screen.findByRole('table'))
    const row = within(table.getByRole('button', { name: 'Schematic claims-no-timestamp' }))
    expect(row.getByText(/Not verified — the build probe has no verdict/)).toBeInTheDocument()
    expect(row.queryByText(/Usable — the build probe confirmed it/)).not.toBeInTheDocument()
    expect(row.getByText(/either did not run or did not answer in time/)).toBeInTheDocument()
    // The architecture qualifier is a property of the record, not of the
    // verdict, and reordering the verdict must not have taken it with it.
    expect(row.getByText('architecture: arm64')).toBeInTheDocument()

    // And the same record in the dialog: the badge is rendered in two places,
    // and a fix in one of them is not a fix.
    const detail = await openDetail(user, CLAIMED_UNPROBED_EMPTY)
    expect(detail.getByText(/Not verified — the build probe has no verdict/)).toBeInTheDocument()
    expect(detail.queryByText(/Usable — the build probe confirmed it/)).not.toBeInTheDocument()
    expect(detail.getByText('architecture: arm64')).toBeInTheDocument()
  })

  it('refuses a usable claim from a record carrying the zero timestamp', async () => {
    // G-02-14, second zero form, and its own test rather than a parameter of
    // the one above. `0001-01-01T00:00:00Z` is a decoded zero `time.Time`; the
    // empty string is a hand-written or partially-migrated record. `isProbed`
    // answers for both, and only a case each keeps it answering for both.
    stubFactory({ saved: [CLAIMED_UNPROBED_ZERO_TIME] })
    const user = userEvent.setup()

    renderImages()

    const table = within(await screen.findByRole('table'))
    const row = within(table.getByRole('button', { name: 'Schematic claims-zero-time' }))
    expect(row.getByText(/Not verified — the build probe has no verdict/)).toBeInTheDocument()
    expect(row.queryByText(/Usable — the build probe confirmed it/)).not.toBeInTheDocument()
    expect(row.getByText('architecture: amd64')).toBeInTheDocument()

    const detail = await openDetail(user, CLAIMED_UNPROBED_ZERO_TIME)
    expect(detail.getByText(/Not verified — the build probe has no verdict/)).toBeInTheDocument()
    expect(detail.queryByText(/Usable — the build probe confirmed it/)).not.toBeInTheDocument()
    expect(detail.getByText('architecture: amd64')).toBeInTheDocument()
  })

  it("keeps the detail dialog's width class through the class merge", async () => {
    // G-02-19, and named for the merge rather than for the width because the
    // merge is what this can actually check. jsdom lays nothing out: it has no
    // viewport, applies no stylesheet and resolves no breakpoint, so a width
    // assertion here would be a sentence about a number nothing computed. What
    // *is* checkable is the defect itself, which was never a rendering
    // outcome -- `DialogContent` sets `sm:max-w-sm`, the caller set an
    // unprefixed `max-w-3xl`, tailwind-merge keys by variant as well as by
    // property, so both classes survived onto the element and the narrower one
    // won the cascade. The class attribute below is the exact artifact of that
    // merge, and it holds the wide rule at the same breakpoint as the default
    // and no longer holds the default.
    //
    // The pixel half of this claim is plan 02-17's: it adds the browser runner,
    // and its first assertion is that this dialog measures wider than the old
    // 384px at a wide viewport. Neither test is sufficient alone -- this one
    // says the right classes reach the element, that one says the browser then
    // does something with them.
    stubFactory({ saved: [USABLE] })
    const user = userEvent.setup()

    renderImages()
    await openDetail(user, USABLE)

    const dialog = await screen.findByRole('dialog')
    const classes = (dialog.getAttribute('class') ?? '').split(/\s+/)

    expect(classes).toContain('sm:max-w-3xl')
    expect(classes).not.toContain('sm:max-w-sm')
    // The unprefixed form is the trap, not a second acceptable answer: it would
    // reintroduce the same silent loss and would also clobber the base
    // small-screen rule asserted below.
    expect(classes).not.toContain('max-w-3xl')
    // Below the breakpoint the component's own rule is still the one that
    // applies; widening the dialog must not have taken the narrow-viewport
    // margin with it.
    expect(classes).toContain('max-w-[calc(100%-2rem)]')
  })

  it('leaves a record written before the architecture existed unqualified', async () => {
    // Every schematic stored before model.Schematic.Arch existed decodes with an
    // empty architecture -- and those are precisely the records the G-02-8 leak
    // produced. Dressing one up with a qualifier nobody measured would be the
    // same lie in a new place, so the badge is exactly what plan 02-12 leaves.
    const old = schematicFixture({ name: 'pre-arch', arch: '' })
    stubFactory({ saved: [old] })
    const user = userEvent.setup()

    renderImages()
    const detail = await openDetail(user, old)

    expect(detail.getByText(/Usable — the build probe confirmed it/)).toBeInTheDocument()
    expect(detail.queryByText(/^architecture:/)).not.toBeInTheDocument()
  })

  it("warns about a stored record on the server's terms rather than the form's", async () => {
    // WR-03. The server warns on presence -- len(ExtraKernelArgs) > 0 and
    // len(Meta) > 0 -- while this panel used to warn only on a non-blank entry.
    // The two agree only for records this form created, because submit drops
    // blank rows before sending; a record written through the API keeps them,
    // and the detail panel then showed nothing for a schematic the server's own
    // Warnings() warns about. The same schematic warning on the create panel
    // and not when it is reopened is worse than either alone -- it teaches the
    // operator that the panel cannot be trusted.
    const authoredElsewhere = schematicFixture({
      name: 'api-authored',
      kernel_args: [''],
      meta: [{ key: 10, value: '' }],
    })
    stubFactory({ saved: [authoredElsewhere] })
    const user = userEvent.setup()

    renderImages()
    const detail = await openDetail(user, authoredElsewhere)

    const warning = await detail.findByRole('alert', { name: 'Schematic warnings' })
    expect(warning).toHaveTextContent('schematic.installer-ignores-kernel-args')
    expect(warning).toHaveTextContent('schematic.installer-ignores-meta')
  })

  it('shows the schematic id in full — it is what a machine config refers to', async () => {
    stubFactory({ saved: [USABLE] })
    const user = userEvent.setup()

    renderImages()
    const detail = await openDetail(user, USABLE)

    // Scoped to the Schematic ID heading's own paragraph rather than the whole
    // dialog. Since ReferenceValue wraps each path segment in its own element,
    // the asset references below also contain an element whose entire text is
    // the id -- `/image/<id>/<version>/...` has the id as one segment -- so an
    // unscoped exact-text query now matches three elements. All three are the id
    // shown in full; this test is about the one under the heading.
    const idHeading = detail.getByRole('heading', { name: 'Schematic ID' })
    const idPanel = idHeading.parentElement
    expect(idPanel).not.toBeNull()
    expect(within(idPanel as HTMLElement).getByText(USABLE.id)).toBeInTheDocument()
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

  it('shows the operator what was not proven about the installer reference', async () => {
    // G-02-3. The reference is usable, so it is still rendered; what was missing
    // was any statement that the preferred repository name had never actually
    // been ruled out. A warning that only reaches a log is not a warning to the
    // person reading the reference.
    const detailText =
      'This installer reference names the repository "installer", which answered for v1.13.9. ' +
      'The preferred repository metal-installer did not answer at all, so it was never ruled out.'
    stubFactory({
      saved: [USABLE],
      assetWarnings: [{ code: WARNING_INSTALLER_REPO_FALLBACK_UNVERIFIED, detail: detailText }],
    })
    const user = userEvent.setup()

    renderImages()
    const detail = await openDetail(user, USABLE)

    await detail.findByLabelText('Installer reference')

    const box = await detail.findByRole('alert', { name: 'Installer reference warnings' })
    expect(within(box).getByText(WARNING_INSTALLER_REPO_FALLBACK_UNVERIFIED)).toBeInTheDocument()
    expect(within(box).getByText(detailText)).toBeInTheDocument()

    // The heading must describe the reference, not the schematic: this warning
    // says nothing about an ISO diverging from an installed system.
    expect(within(box).queryByText(/produce an ISO and an installed system that differ/)).toBeNull()
  })

  it('renders no warning box when the installer repository name was proven', async () => {
    stubFactory({ saved: [USABLE] })
    const user = userEvent.setup()

    renderImages()
    const detail = await openDetail(user, USABLE)

    await detail.findByLabelText('Installer reference')
    expect(detail.queryByRole('alert', { name: 'Installer reference warnings' })).toBeNull()
  })

  /*
   * G-02-15's client half. The server used to answer `502` with nothing at all
   * when the installer could not be resolved, and this panel rendered one
   * hardcoded sentence for it — a sentence that never read `assets.error`, never
   * named SecureBoot, and never said which of the two opposite remedies applied.
   * No test covered that branch at all, which is how it stayed that way.
   */

  const UNAVAILABLE_DETAIL =
    'The Image Factory did not answer usably: resolving the installer image reference for v1.13.9.'
  const REJECTED_DETAIL =
    'The Image Factory refused: resolving the installer image reference for v1.13.9.'

  it('renders the four references it did receive when the installer could not be resolved', async () => {
    stubFactory({
      saved: [USABLE],
      installerError: { code: CODE_UPSTREAM_FACTORY_UNAVAILABLE, detail: UNAVAILABLE_DETAIL },
    })
    const user = userEvent.setup()

    renderImages()
    const detail = await openDetail(user, USABLE)

    // All four, with their exact values. They never touched the registry, so a
    // registry that would not answer cannot have made any of them wrong.
    expect(await detail.findByLabelText('ISO reference')).toHaveTextContent(
      `https://factory.talos.dev/image/${USABLE.id}/v1.13.9/metal-amd64.iso`,
    )
    expect(detail.getByLabelText('PXE reference')).toHaveTextContent(
      `https://factory.talos.dev/pxe/${USABLE.id}/v1.13.9/metal-amd64`,
    )
    expect(detail.getByLabelText('Disk image reference')).toHaveTextContent(
      `https://factory.talos.dev/image/${USABLE.id}/v1.13.9/metal-amd64.raw.zst`,
    )
    expect(detail.getByLabelText('Kernel cmdline reference')).toHaveTextContent(
      `https://factory.talos.dev/image/${USABLE.id}/v1.13.9/cmdline-metal-amd64`,
    )

    // The installer row is present and says why it is empty. Omitting it would
    // leave a grid of four rows that reads as complete.
    const installer = detail.getByLabelText('Installer reference')
    const reason = within(installer).getByRole('alert')

    // The sentence is the server's, not a third one written here.
    expect(reason).toHaveTextContent(UNAVAILABLE_DETAIL)
    // ... and the remedy is the retryable one.
    expect(reason).toHaveTextContent(/again/i)
    expect(reason).not.toHaveTextContent(/no installer/i)
  })

  it('separates a refused installer from a registry that did not answer', async () => {
    stubFactory({
      saved: [USABLE],
      installerError: { code: CODE_UPSTREAM_FACTORY_REJECTED, detail: REJECTED_DETAIL },
    })
    const user = userEvent.setup()

    renderImages()
    const detail = await openDetail(user, USABLE)

    await detail.findByLabelText('ISO reference')
    const reason = within(detail.getByLabelText('Installer reference')).getByRole('alert')

    expect(reason).toHaveTextContent(REJECTED_DETAIL)
    // The two remedies are opposite, so the two sentences must be too. A refusal
    // is a verdict about this version: asking again reproduces it exactly.
    expect(reason).toHaveTextContent(/no installer/i)
    expect(reason).not.toHaveTextContent(/asking again may/i)
  })

  it('names SecureBoot, and what unticking it would ask instead, only when it was ticked', async () => {
    stubFactory({
      saved: [USABLE],
      installerError: { code: CODE_UPSTREAM_FACTORY_REJECTED, detail: REJECTED_DETAIL },
    })
    const user = userEvent.setup()

    renderImages()
    const detail = await openDetail(user, USABLE)

    // The flag is a query parameter and never reaches the stored record, so the
    // client is the only place that knows this request asked for it. Before the
    // box is ticked, nothing may claim it did.
    const before = within(detail.getByLabelText('Installer reference'))
    expect(before.queryByText(/SecureBoot/)).toBeNull()

    await user.click(detail.getByRole('checkbox', { name: 'SecureBoot' }))

    await waitFor(() => {
      const row = within(detail.getByLabelText('Installer reference'))
      expect(row.getByText(/SecureBoot/)).toBeInTheDocument()
      // It must say what unticking asks — a different image — and must not read
      // as an offer to use the ordinary installer for a SecureBoot ISO, which is
      // the drift the whole resolution exists to prevent.
      expect(row.getByText(/does not produce a SecureBoot node/)).toBeInTheDocument()
    })
  })

  it('shows one error and no reference grid when the assets request itself fails', async () => {
    stubFactory({
      saved: [USABLE],
      fail: {
        path: `/api/v1/schematics/${USABLE.id}/assets`,
        status: 400,
        body: {
          type: problemType('validation'),
          title: 'Request is not valid',
          status: 400,
          detail: '"riscv64" is not an architecture the Factory builds for.',
          code: 'validation.failed',
        },
      },
    })
    const user = userEvent.setup()

    renderImages()
    const detail = await openDetail(user, USABLE)

    // The server's own sentence, rather than the sentence this panel used to
    // hardcode for every failure it could have.
    expect(await detail.findByRole('alert', { name: 'Asset references failed' })).toHaveTextContent(
      '"riscv64" is not an architecture the Factory builds for.',
    )
    // Nothing was computed, so nothing is shown. A request that fails before the
    // URL builders is not a partial answer.
    expect(detail.queryByLabelText('ISO reference')).toBeNull()
    expect(detail.queryByLabelText('Installer reference')).toBeNull()
  })

  it('renders each path segment as its own element and the reference as one string', async () => {
    // This replaces a test named `cannot split a repository name across a line
    // break`, which asserted that the class string lacked two utilities and that
    // the text content was intact. Both stayed true while the name split, so it
    // passed throughout: UAX #14 offers a break after U+002D HYPHEN-MINUS
    // regardless of `overflow-wrap` and `word-break`, and every installer name is
    // hyphenated. It was then cited as evidence that the gap was refuted, which
    // is what a test named for a property it cannot check is for (G-02-10).
    //
    // THE LINE-BREAK PROPERTY IS NOT ASSERTED HERE AND CANNOT BE. jsdom performs
    // no layout, so nothing in this file can tell a name that splits from one
    // that does not. It is measured in `images.browser.test.tsx`, which counts
    // line boxes for all four installer repository names at every width in a
    // swept range, in an engine that lays out.
    //
    // What this checks is the structure that fix is made of, which jsdom can see
    // honestly: the segments are separate elements each carrying the rule that
    // suppresses soft wrapping inside it, `<wbr />` sits between them as the only
    // offered break point, and the text content is still exactly the reference.
    stubFactory({ saved: [USABLE] })
    const user = userEvent.setup()

    renderImages()
    const detail = await openDetail(user, USABLE)

    const installer = await detail.findByLabelText('Installer reference')
    const reference = `factory.talos.dev/metal-installer/${USABLE.id}:v1.13.9`

    // The whole reference is still one string: the Copy control writes this
    // value and a mouse selection yields the rendered text. A mitigation that
    // swapped the hyphen for U+2011 would satisfy a line-box measurement and
    // hand out something that is not a valid OCI reference, so this assertion is
    // what forbids that class of fix.
    expect(installer).toHaveTextContent(reference)

    const value = installer.querySelector('[data-reference]')
    expect(value).not.toBeNull()
    expect(value?.textContent).toBe(reference)

    const segments = Array.from(value?.querySelectorAll('span') ?? [])
    expect(segments.map((each) => each.textContent)).toEqual([
      'factory.talos.dev',
      'metal-installer',
      `${USABLE.id}:v1.13.9`,
    ])
    for (const segment of segments) {
      expect(segment.className).toContain('whitespace-nowrap')
    }

    // The `<wbr />` elements are OUTSIDE the segment elements. Inside one they
    // would be suppressed along with every other break opportunity there, and
    // the reference could then not wrap anywhere at all.
    const breaks = Array.from(value?.querySelectorAll('wbr') ?? [])
    expect(breaks).toHaveLength(segments.length - 1)
    for (const each of breaks) {
      expect(each.closest('span[class*="whitespace-nowrap"]')).toBeNull()
    }
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
    type: problemType('not-found'),
    title: 'Not found',
    status: 404,
    detail: 'No such schematic.',
    code: 'notfound.schematic',
  }

  const STORE_FAILURE_PROBLEM = {
    type: problemType('internal'),
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
