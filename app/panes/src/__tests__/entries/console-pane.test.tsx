import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { ConsolePane } from '@/entries/console-pane'

describe('ConsolePane', () => {
  it('is the console region of the window', () => {
    render(<ConsolePane />)
    expect(screen.getByRole('region', { name: 'Console' })).toBeTruthy()
  })
})
