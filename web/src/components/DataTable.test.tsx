import { render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { type Column, DataTable } from '@/components/DataTable'

/**
 * The two presentations of a set of rows.
 *
 * The test that matters is the last one. The operator photographed /kubernetes
 * on a phone on 2026-09-19: a "Scale" field and a "Roll pods" button, with the
 * deployment's namespace and name scrolled off the left edge. Reachable buttons,
 * unknowable subject. So the claim this file pins is not "it looks nicer" — it
 * is that on a phone an action and the name it acts on are in the same box.
 */

interface Row {
  id: string
  name: string
  namespace: string
  ready: string
  image: string
}

const rows: Row[] = [
  {
    id: '1',
    name: 'coredns',
    namespace: 'kube-system',
    ready: '1/1',
    image: 'registry.k8s.io/coredns/coredns:v1.11.3',
  },
  {
    id: '2',
    name: 'holzcloud-website',
    namespace: 'default',
    ready: '1/2',
    image: 'ghcr.io/holzcloud/website:2026.09.17',
  },
]

const columns: Column<Row>[] = [
  { key: 'namespace', label: 'Namespace', render: (r) => r.namespace },
  { key: 'name', label: 'Deployment', role: 'identity', render: (r) => r.name },
  { key: 'ready', label: 'Ready', render: (r) => r.ready },
  { key: 'image', label: 'Image', role: 'detail', render: (r) => r.image },
  {
    key: 'actions',
    label: 'Actions',
    role: 'actions',
    render: (r) => <button type="button">Roll pods {r.name}</button>,
  },
]

/** The width this renders at. The default stub answers false to everything. */
function atPhoneWidth(isPhone: boolean) {
  vi.stubGlobal(
    'matchMedia',
    vi.fn((q: string) => ({
      matches: q.includes('max-width: 767px') ? isPhone : false,
      media: q,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('on a desk', () => {
  it('is a table, with every column as a header', () => {
    atPhoneWidth(false)
    render(
      <DataTable
        label="Deployments"
        rows={rows}
        keyOf={(r) => r.id}
        columns={columns}
        empty="none"
      />,
    )

    const table = screen.getByRole('table', { name: 'Deployments' })
    for (const label of ['Namespace', 'Deployment', 'Ready', 'Image', 'Actions']) {
      expect(within(table).getByRole('columnheader', { name: label })).toBeInTheDocument()
    }
    expect(within(table).getAllByRole('row')).toHaveLength(rows.length + 1)
  })
})

describe('on a phone', () => {
  it('is not a table at all', () => {
    atPhoneWidth(true)
    render(
      <DataTable
        label="Deployments"
        rows={rows}
        keyOf={(r) => r.id}
        columns={columns}
        empty="none"
      />,
    )

    // A table here is the defect: at 390px these were measured 522 to 1027px
    // wide, so reading one means swiping its identity off the left edge.
    expect(screen.queryByRole('table')).toBeNull()
    expect(screen.getAllByRole('listitem')).toHaveLength(rows.length)
  })

  it('keeps every action in the same box as the name it acts on', () => {
    atPhoneWidth(true)
    render(
      <DataTable
        label="Deployments"
        rows={rows}
        keyOf={(r) => r.id}
        columns={columns}
        empty="none"
      />,
    )

    for (const row of rows) {
      const button = screen.getByRole('button', { name: `Roll pods ${row.name}` })
      const box = button.closest('[data-row]')
      expect(box).not.toBeNull()
      // The name is inside the same box. This is the whole point: the
      // photographed screen had the button without it.
      expect(within(box as HTMLElement).getByText(row.name)).toBeInTheDocument()
      expect(within(box as HTMLElement).getByText(row.namespace)).toBeInTheDocument()
    }
  })

  it('labels every value, because a number with no label is not information', () => {
    atPhoneWidth(true)
    render(
      <DataTable
        label="Deployments"
        rows={rows}
        keyOf={(r) => r.id}
        columns={columns}
        empty="none"
      />,
    )

    const first = screen.getAllByRole('listitem')[0] as HTMLElement
    // "1/1" alone says nothing; the header that gave it meaning is a column
    // heading on the desk and has to become a label here.
    expect(within(first).getByText('Ready')).toBeInTheDocument()
    expect(within(first).getByText('1/1')).toBeInTheDocument()
  })

  it('folds the detail away in the scanning shape, and keeps the identity out', () => {
    atPhoneWidth(true)
    render(
      <DataTable
        label="Archive"
        rows={rows}
        keyOf={(r) => r.id}
        columns={columns}
        phone="rows"
        empty="none"
      />,
    )

    // priority+: the identity and the summary are visible; the detail is behind
    // a disclosure the browser implements.
    expect(screen.getByText('coredns')).toBeInTheDocument()
    const disclosures = screen.getAllByRole('group')
    expect(disclosures).toHaveLength(rows.length)
    expect(disclosures[0]?.hasAttribute('open')).toBe(false)
  })
})

describe('with nothing to show', () => {
  it('says whose claim the emptiness is, rather than “no data”', () => {
    atPhoneWidth(true)
    render(
      <DataTable
        label="Deployments"
        rows={[]}
        keyOf={(r: Row) => r.id}
        columns={columns}
        empty="The API server answered, and this cluster has no deployments."
      />,
    )

    expect(screen.getByText(/the API server answered/i)).toBeInTheDocument()
    expect(screen.queryByRole('table')).toBeNull()
  })
})
