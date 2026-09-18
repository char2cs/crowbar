import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { SidebarRow } from '@/components/sidebar/sidebar-row'
import { rowsFromRepo } from '@/components/sidebar/lib/rows-from-repo'
import type { Repo } from '@/lib/store/sidebar'

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: vi.fn(), success: vi.fn(), info: vi.fn() },
}))

/**
 * A repo header row is identified by the row itself (`repoIcon`, the same
 * signal `sidebar-drop-policy.ts`/`drop-actions.ts` already key on), never by
 * `parentId === null`: a repo's entry can be filed into a project-home folder
 * (`repo.folderId`), and it is still the repo.
 */
function makeRepo(folderId: string): Repo {
  return {
    id: 'repo-alpha',
    projectId: 'proj-1',
    folderId,
    name: 'repo-alpha',
    avatarLabel: 'A',
    avatarColor: 'bg-indigo-700',
    defaultWorkspaceId: 'ws-home',
    defaultBranch: 'main',
    defaultWorkspaceStatus: 'new',
    defaultOwningChatId: 'home-chat',
    workspaces: [],
    folders: [],
    chats: [{ id: 'home-chat', repoId: 'repo-alpha', title: '', order: 0, workspaceId: 'ws-home' }],
  }
}

function renderHeader(folderId: string) {
  const [header] = rowsFromRepo(makeRepo(folderId))
  render(
    <SidebarRow row={header} depth={1} onOpen={vi.fn()} onTrash={vi.fn()} onCreate={vi.fn()} />,
  )
  return header
}

describe('SidebarRow: a repo header filed into a home folder', () => {
  it('is drawn by rowsFromRepo with the folder as its parent', () => {
    expect(rowsFromRepo(makeRepo('home-folder-1'))[0]?.parentId).toBe('home-folder-1')
  })

  it('keeps its overflow menu and icon picker, and never gains the Remove control', () => {
    renderHeader('home-folder-1')
    expect(screen.getByRole('button', { name: 'More actions for repo-alpha' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /edit repo-alpha icon/i })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Remove repo-alpha' })).not.toBeInTheDocument()
  })

  it('renders identically to the same header at the project-home root', () => {
    renderHeader('')
    expect(screen.getByRole('button', { name: 'More actions for repo-alpha' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Remove repo-alpha' })).not.toBeInTheDocument()
  })
})
