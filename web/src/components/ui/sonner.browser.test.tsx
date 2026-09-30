import { act, cleanup, render } from '@testing-library/react'
import { toast } from 'sonner'
import { afterEach, describe, expect, it } from 'vitest'
import { page } from 'vitest/browser'
// Imported for its effect: the dismiss chip's size comes from Tailwind classes
// generated from this entry point, and without it the chip keeps sonner's own
// 20 px and the measurement measures nothing of ours.
import '@/index.css'
import { notify, Toaster } from '@/components/Toaster'

/**
 * A toast's dismiss button is a thumb target below md (MOB-01, UI-SPEC "Toasts").
 *
 * This is a component test and not a layout-audit opener because nothing in the
 * running product raises a toast without doing something: every toast follows
 * an action that changes state, and the audit only opens and measures. So the
 * application's own Toaster is rendered here, in a real browser at the phone
 * width, and a toast is raised the way the product raises one.
 *
 * Above md the chip stays the 24 px bordered circle it has always been.
 */

async function dismissButton(): Promise<HTMLElement> {
  render(<Toaster />)
  act(() => {
    notify.info('Node m-cp-1 is rebooting', 'It will be back in a minute.')
  })
  // At least one: a selector that finds nothing would pass every size check
  // below by measuring no element at all.
  await expect
    .poll(() => document.querySelectorAll('[data-close-button]').length)
    .toBeGreaterThanOrEqual(1)
  const toastEl = document.querySelector<HTMLElement>('[data-sonner-toast]')
  expect(toastEl).not.toBeNull()
  // Sonner mounts the toast off-screen and slides it in; measure it at rest.
  await expect.poll(() => toastEl?.dataset.mounted).toBe('true')
  const button = document.querySelector<HTMLElement>('[data-close-button]') as HTMLElement
  await Promise.all(
    (toastEl as HTMLElement).getAnimations({ subtree: true }).map((a) => a.finished),
  )
  return button
}

afterEach(async () => {
  act(() => {
    toast.dismiss()
  })
  cleanup()
  await page.viewport(1200, 900)
})

describe('the toast dismiss button', () => {
  it('is at least 44 x 44 px at 390 px', async () => {
    await page.viewport(390, 844)
    const box = (await dismissButton()).getBoundingClientRect()
    expect(box.width).toBeGreaterThanOrEqual(44)
    expect(box.height).toBeGreaterThanOrEqual(44)
  })

  it('stays the 24 px chip at 1280 px', async () => {
    await page.viewport(1280, 900)
    const box = (await dismissButton()).getBoundingClientRect()
    expect(box.width).toBe(24)
    expect(box.height).toBe(24)
  })
})
