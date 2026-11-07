import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ComposerSlashPicker } from '@/features/agent/composer/composer-slash-picker'

const help = { id: 'b-help', insertText: '/help ', description: 'Show help', terminal: true }
const compact = { id: 'b-compact', insertText: '/compact ', description: 'Summarise' }

describe('ComposerSlashPicker', () => {
  it('shows nothing at all when nothing matches', () => {
    render(<ComposerSlashPicker items={[]} selected={0} onSelect={() => {}} />)

    expect(screen.queryByRole('listbox')).not.toBeInTheDocument()
  })

  it('says which commands run in the terminal, and only those', () => {
    render(<ComposerSlashPicker items={[compact, help]} selected={0} onSelect={() => {}} />)

    const [first, second] = screen.getAllByRole('option')
    expect(second).toHaveTextContent('/help')
    expect(second).toHaveTextContent('Runs in the terminal')
    expect(first).not.toHaveTextContent('Runs in the terminal')
  })

  it('picks the clicked command', () => {
    const onSelect = vi.fn()
    render(<ComposerSlashPicker items={[compact, help]} selected={0} onSelect={onSelect} />)

    fireEvent.click(screen.getByRole('option', { name: /\/help/ }))

    expect(onSelect).toHaveBeenCalledWith(help)
  })
})
