import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import {
  ComposerSlashPicker,
  type PickerItem,
} from '@/features/agent/composer/composer-slash-picker'

const compact: PickerItem = {
  id: 'b-compact',
  insertText: '/compact ',
  description: 'Summarise',
  kind: 'builtin',
}
const help: PickerItem = {
  id: 'b-help',
  insertText: '/help ',
  description: 'Show help',
  kind: 'builtin',
  terminal: true,
}
const review: PickerItem = {
  id: 's-review',
  insertText: '/code-review:code-review ',
  description: 'Review a pull request',
  kind: 'skill',
}

describe('ComposerSlashPicker commands', () => {
  it('shows nothing at all when nothing matches', () => {
    render(<ComposerSlashPicker items={[]} selected={0} onSelect={() => {}} />)

    expect(screen.queryByRole('listbox')).not.toBeInTheDocument()
  })

  it('groups commands, terminal-only commands and skills under their labels', () => {
    render(
      <ComposerSlashPicker
        items={[compact, help, review]}
        selected={0}
        total={31}
        onSelect={() => {}}
      />,
    )

    const listbox = screen.getByRole('listbox')
    const labels = [...listbox.querySelectorAll('.gh')].map((el) => el.textContent)
    expect(labels).toEqual(['Commands', 'Runs in the terminal', 'Skills'])
    expect(listbox).toHaveTextContent('3 of 31 commands')
  })

  it('underlines what was typed and marks the selected row with its key', () => {
    render(
      <ComposerSlashPicker items={[compact, help]} selected={0} query="co" onSelect={() => {}} />,
    )

    const [first, second] = screen.getAllByRole('option')
    expect(first.querySelector('u')).toHaveTextContent('co')
    expect(first).toHaveTextContent('↵')
    expect(second).not.toHaveTextContent('↵')
  })

  it('picks the clicked command', () => {
    const onSelect = vi.fn()
    render(<ComposerSlashPicker items={[compact, help]} selected={0} onSelect={onSelect} />)

    fireEvent.click(screen.getByRole('option', { name: /\/help/ }))

    expect(onSelect).toHaveBeenCalledWith(help)
  })
})

describe('ComposerSlashPicker files', () => {
  const file: PickerItem = {
    id: 'pricing/total.go',
    insertText: '@pricing/total.go ',
    description: '',
    name: 'total.go',
    dir: 'pricing/',
    isDir: false,
  }
  const folder: PickerItem = {
    id: 'web/totals',
    insertText: '@web/totals/',
    description: 'Folder',
    name: 'totals',
    dir: 'web/',
    isDir: true,
  }

  it('lists each match with its folder, tags a folder, and counts the matches', () => {
    render(
      <ComposerSlashPicker
        variant="files"
        items={[file, folder]}
        selected={0}
        query="tot"
        onSelect={() => {}}
      />,
    )

    const [first, second] = screen.getAllByRole('option')
    expect(first).toHaveTextContent('total.gopricing/')
    expect(first.querySelector('u')).toHaveTextContent('tot')
    expect(second).toHaveTextContent('totals/web/Folder')
    expect(screen.getByRole('listbox')).toHaveTextContent('2 matches')
  })

  it('names what the files are of', () => {
    render(
      <ComposerSlashPicker
        variant="files"
        scope="checkout"
        items={[file]}
        selected={0}
        onSelect={() => {}}
      />,
    )

    expect(screen.getByText('Files in checkout')).toBeInTheDocument()
  })
})
