import { cleanup, render } from '@testing-library/react'
import { afterEach, beforeAll, describe, expect, it } from 'vitest'
// The stylesheet is imported for its effect: the body's tracking, the font and
// every utility measured here come from it. Without it the spans below are laid
// out in the browser's default font with no tracking at all, and the space
// advance agrees with the reference because nothing was applied -- a pass that
// measures nothing.
import '@/index.css'

/**
 * The space between two words in a 12-px line (Phase 12, UI-SPEC checker
 * resolution 4; 11-UI-REVIEW Pillar 4).
 *
 * The Phase 11 picture showed 12-px muted lines whose words ran together
 * ("412MiB·measured23sago"). What is measured here is the advance of the space
 * itself: the width of "a b" minus the width of "ab", in a `text-xs` span that
 * sits in the product's body and inherits whatever tracking the body sets. The
 * reference is the same two strings in the same font and size with
 * `letter-spacing: normal; word-spacing: normal` -- the space the font means.
 *
 * Headings are the other half: their tighter tracking is a design decision, and
 * a fix for 12-px text must not reach them.
 */

const width = (el: Element) => el.getBoundingClientRect().width

function Line({
  testid,
  text,
  style,
}: {
  testid: string
  text: string
  style?: React.CSSProperties
}) {
  return (
    <span
      data-testid={testid}
      className="text-xs text-muted-foreground"
      style={{ whiteSpace: 'pre', ...style }}
    >
      {text}
    </span>
  )
}

const REFERENCE: React.CSSProperties = { letterSpacing: 'normal', wordSpacing: 'normal' }

function measure() {
  const { getByTestId } = render(
    <div className="font-sans">
      <Line testid="product-spaced" text="a b" />
      <br />
      <Line testid="product-joined" text="ab" />
      <br />
      <Line testid="reference-spaced" text="a b" style={REFERENCE} />
      <br />
      <Line testid="reference-joined" text="ab" style={REFERENCE} />
    </div>,
  )
  const widths = {
    productSpaced: width(getByTestId('product-spaced')),
    productJoined: width(getByTestId('product-joined')),
    referenceSpaced: width(getByTestId('reference-spaced')),
    referenceJoined: width(getByTestId('reference-joined')),
  }
  const productSpace = widths.productSpaced - widths.productJoined
  const referenceSpace = widths.referenceSpaced - widths.referenceJoined
  return { widths, productSpace, referenceSpace }
}

beforeAll(async () => {
  // The variable font is a web font: measured before it has loaded, every
  // width is the fallback's, and the comparison is of a font nobody sees.
  await document.fonts.load('12px "Manrope Variable"')
  await document.fonts.ready
})

afterEach(() => {
  cleanup()
})

describe('12-px lines', () => {
  it('are laid out in the product font, at 12 px', () => {
    const { getByTestId } = render(<Line testid="line" text="a b" />)
    const style = getComputedStyle(getByTestId('line'))
    expect(style.fontSize).toBe('12px')
    expect(style.fontFamily).toContain('Manrope Variable')
    expect(document.fonts.check('12px "Manrope Variable"')).toBe(true)
  })

  it('separate their words by the space the font means', () => {
    const { widths, productSpace, referenceSpace } = measure()
    // The four widths go into the plan's summary; they are what decided the fix.
    console.log(
      `12-px widths (DPR ${window.devicePixelRatio}): ${JSON.stringify(widths)} ` +
        `space product=${productSpace.toFixed(3)} reference=${referenceSpace.toFixed(3)}`,
    )
    // The reference is a real space, not a measurement of nothing.
    expect(referenceSpace).toBeGreaterThan(1.5)
    expect(Math.abs(productSpace - referenceSpace)).toBeLessThanOrEqual(0.1)
  })

  it('leave the tracking of headings and of larger text alone', () => {
    const { getByRole, getByTestId } = render(
      <div>
        <h1 className="font-heading text-xl font-semibold tracking-tight">Host</h1>
        <p data-testid="body" className="text-sm">
          Body text
        </p>
      </div>,
    )
    // tracking-tight at 20 px, and the body's snug tracking (-0.015em of the
    // body's 16 px, inherited as the computed -0.24px) at 14 px: what
    // these read before the 12-px fix, and what they must still read after.
    console.log(
      `h1 letter-spacing ${getComputedStyle(getByRole('heading')).letterSpacing}, ` +
        `text-sm ${getComputedStyle(getByTestId('body')).letterSpacing}`,
    )
    expect(getComputedStyle(getByRole('heading')).letterSpacing).toBe('-0.5px')
    expect(getComputedStyle(getByTestId('body')).letterSpacing).toBe('-0.24px')
  })
})
