import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { RemoteFolderTree } from '@/features/hosts/components/remote-folder-tree'
import type { BrowseEntry } from '@/features/hosts/lib/browse'

const dir = (path: string): BrowseEntry => ({ name: path.slice(1), path, isDir: true })

// Everything but the choice stays the same object, as it does while the picker is open.
const fixed = {
  treeRef: () => {},
  hostName: 'homelab-prox',
  entries: [dir('/home'), dir('/srv')],
  listings: {},
  expanded: new Set<string>(),
  activeKey: '/home',
  onActiveKeyChange: vi.fn(),
  onExpandedChange: vi.fn(),
  onPickFolder: vi.fn(),
}

const selection = () =>
  screen.getAllByRole('treeitem').map((item) => item.getAttribute('aria-selected'))

describe('RemoteFolderTree', () => {
  // Enter on the active row chooses it without moving it: only `selected` changes.
  it('marks the chosen folder selected, and only it, as the choice moves', () => {
    const { rerender } = render(<RemoteFolderTree {...fixed} selectedPath={null} />)
    expect(selection()).toEqual(['false', 'false'])

    rerender(<RemoteFolderTree {...fixed} selectedPath="/home" />)
    expect(selection()).toEqual(['true', 'false'])

    rerender(<RemoteFolderTree {...fixed} selectedPath="/srv" />)
    expect(selection()).toEqual(['false', 'true'])
  })
})
