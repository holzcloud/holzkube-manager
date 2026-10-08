import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Problem } from '@/components/Problem'

describe('Problem', () => {
  it('is announced to assistive technology', () => {
    render(<Problem error={new Error('The node refused.')} />)
    expect(screen.getByRole('alert')).toHaveTextContent('The node refused.')
  })

  it('renders a thrown non-Error as text', () => {
    render(<Problem error="plain string" />)
    expect(screen.getByRole('alert')).toHaveTextContent('plain string')
  })
})
