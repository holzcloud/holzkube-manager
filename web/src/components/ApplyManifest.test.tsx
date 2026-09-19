import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/api'
import { ApplyManifest } from '@/components/ApplyManifest'

/**
 * Applying a manifest, from the screen.
 *
 * The behaviour these tests pin is the one an operator's safety rests on: the
 * plan belongs to the text it was made for. A screen that kept a plan across an
 * edit would offer an apply described by a document that is no longer in the
 * box, which is worse than offering no description at all.
 */

function wrap(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

const settings = {
  api_version: 'v1',
  kind: 'ConfigMap',
  namespace: 'default',
  name: 'settings',
  action: 'update',
  namespaced: true,
}

const onePlan = { objects: [settings], warnings: [] }

afterEach(() => {
  vi.restoreAllMocks()
})

describe('applying a manifest', () => {
  it('will not apply before a plan, and says why', async () => {
    const apply = vi.spyOn(api.kubernetes, 'applyManifest')

    wrap(<ApplyManifest clusterID="c-1" />)
    await userEvent.type(screen.getByLabelText('Manifest'), 'kind: ConfigMap')

    expect(screen.getByRole('button', { name: 'Apply' })).toBeDisabled()
    expect(screen.getByText(/plan it first/i)).toBeInTheDocument()
    expect(apply).not.toHaveBeenCalled()
  })

  it('shows what the plan says the cluster would do', async () => {
    vi.spyOn(api.kubernetes, 'planManifest').mockResolvedValue(onePlan)

    wrap(<ApplyManifest clusterID="c-1" />)
    await userEvent.type(screen.getByLabelText('Manifest'), 'kind: ConfigMap')
    await userEvent.click(screen.getByRole('button', { name: 'Plan' }))

    // "Update" rather than "create", and the difference came from the cluster:
    // the manifest text is identical either way, which is the whole reason the
    // plan is a request and not a parse.
    await waitFor(() => expect(screen.getByText(/Update/)).toBeInTheDocument())
    expect(screen.getByText('settings')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Apply' })).toBeEnabled()
  })

  it('throws the plan away when the manifest is edited', async () => {
    const plan = vi.spyOn(api.kubernetes, 'planManifest').mockResolvedValue(onePlan)
    const apply = vi.spyOn(api.kubernetes, 'applyManifest')

    wrap(<ApplyManifest clusterID="c-1" />)
    const box = screen.getByLabelText('Manifest')
    await userEvent.type(box, 'kind: ConfigMap')
    await userEvent.click(screen.getByRole('button', { name: 'Plan' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Apply' })).toBeEnabled())

    // Any edit at all: the plan described the text as it was, not as it is.
    await userEvent.type(box, 'Map2')

    expect(screen.getByRole('button', { name: 'Apply' })).toBeDisabled()
    expect(apply).not.toHaveBeenCalled()
    expect(plan).toHaveBeenCalledTimes(1)
  })

  it('applies the text that was planned', async () => {
    vi.spyOn(api.kubernetes, 'planManifest').mockResolvedValue(onePlan)
    const apply = vi.spyOn(api.kubernetes, 'applyManifest').mockResolvedValue({
      applied: onePlan.objects,
      failed: [],
      fully_applied: true,
    })

    wrap(<ApplyManifest clusterID="c-1" />)
    await userEvent.type(screen.getByLabelText('Manifest'), 'kind: ConfigMap')
    await userEvent.click(screen.getByRole('button', { name: 'Plan' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Apply' })).toBeEnabled())
    await userEvent.click(screen.getByRole('button', { name: 'Apply' }))

    await waitFor(() => expect(apply).toHaveBeenCalledWith('c-1', 'kind: ConfigMap'))
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent(/applied 1 object/i))
  })

  it('shows a conflict per object instead of calling the apply a success', async () => {
    vi.spyOn(api.kubernetes, 'planManifest').mockResolvedValue(onePlan)
    vi.spyOn(api.kubernetes, 'applyManifest').mockResolvedValue({
      applied: [{ ...settings, name: 'uncontested', action: 'applied' }],
      failed: [
        {
          object: { ...settings, name: 'contested' },
          reason: 'another field manager owns a field this object sets',
        },
      ],
      fully_applied: false,
    })

    wrap(<ApplyManifest clusterID="c-1" />)
    await userEvent.type(screen.getByLabelText('Manifest'), 'kind: ConfigMap')
    await userEvent.click(screen.getByRole('button', { name: 'Plan' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Apply' })).toBeEnabled())
    await userEvent.click(screen.getByRole('button', { name: 'Apply' }))

    // The count that failed, and the reason, per object. Five of ten applied is
    // not "applied" and it is not "failed" either.
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent(/1 did not apply/i))
    expect(screen.getByText(/another field manager/i)).toBeInTheDocument()
  })

  it('shows the plan’s warnings, which are the part worth reading', async () => {
    vi.spyOn(api.kubernetes, 'planManifest').mockResolvedValue({
      objects: onePlan.objects,
      warnings: ['ConfigMap "settings" names no namespace, so it would be applied to "default"'],
    })

    wrap(<ApplyManifest clusterID="c-1" />)
    await userEvent.type(screen.getByLabelText('Manifest'), 'kind: ConfigMap')
    await userEvent.click(screen.getByRole('button', { name: 'Plan' }))

    await waitFor(() =>
      expect(screen.getByRole('list', { name: 'Warnings' })).toHaveTextContent(/no namespace/i),
    )
  })
})
