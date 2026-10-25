import { act, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { SidebarBody } from '@/components/layout/sidebar-body'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'
import {
  EMPTY_FOCUSED_WORKSPACE_CONTEXT,
  publishFocusedWorkspaceContext,
  type FocusedWorkspaceContext,
} from '@/features/window/stores/focused-workspace-context-store'

const { treeProps } = vi.hoisted(() => ({ treeProps: vi.fn() }))
vi.mock('@/features/file-explorer/components/file-explorer-tree', () => ({
  FileExplorerTree: (props: { workspaceId: string | null; rootFolderPath?: string }) => {
    treeProps(props)
    return <div data-testid="panel-files" />
  },
}))
vi.mock('@/features/git/components/git-panel', () => ({
  GitPanel: () => <div data-testid="panel-git" />,
}))

const REPO_CTX: FocusedWorkspaceContext = {
  projectId: 'p1',
  workspaceId: 'ws-repo',
  isProjectHome: false,
  repoId: 'r1',
  repoPath: '/repo',
  rootPath: '/repo',
}
const BRANCH_CTX: FocusedWorkspaceContext = {
  projectId: 'p1',
  workspaceId: 'ws-branch',
  isProjectHome: false,
  repoId: 'r1',
  repoPath: '/worktrees/branch',
  rootPath: '/worktrees/branch',
}
const HOME_CTX: FocusedWorkspaceContext = {
  projectId: 'p1',
  workspaceId: 'ws-home',
  isProjectHome: true,
  repoId: null,
  repoPath: null,
  rootPath: '/projects/p1',
}

function renderBody() {
  return render(
    <SidebarBody
      chatsPanel={<div data-testid="panel-chats" />}
      projectSwitcher={<div data-testid="project-switcher" />}
    />,
  )
}

describe('SidebarBody', () => {
  beforeEach(() => {
    useSidebarStore.setState(getInitialState())
    publishFocusedWorkspaceContext(REPO_CTX)
    treeProps.mockClear()
  })

  it('shows the Chats panel by default', () => {
    renderBody()
    expect(screen.getByTestId('panel-chats')).toBeVisible()
    expect(screen.getByTestId('panel-files')).not.toBeVisible()
    expect(screen.getByTestId('panel-git')).not.toBeVisible()
  })

  it('shows only the selected panel while all three stay mounted', () => {
    renderBody()
    const chats = screen.getByTestId('panel-chats')
    const files = screen.getByTestId('panel-files')
    const git = screen.getByTestId('panel-git')

    fireEvent.click(screen.getByRole('tab', { name: 'Changes' }))
    expect(git).toBeVisible()
    expect(chats).not.toBeVisible()
    expect(files).not.toBeVisible()

    fireEvent.click(screen.getByRole('tab', { name: 'Files' }))
    expect(files).toBeVisible()
    expect(git).not.toBeVisible()

    fireEvent.click(screen.getByRole('tab', { name: 'Chats' }))
    expect(chats).toBeVisible()
    // Same nodes throughout: a switch hides, it never remounts.
    expect(screen.getByTestId('panel-chats')).toBe(chats)
    expect(screen.getByTestId('panel-files')).toBe(files)
    expect(screen.getByTestId('panel-git')).toBe(git)
  })

  it('restores a persisted mode on mount', () => {
    useSidebarStore.setState({ activeTab: 'files' })
    renderBody()
    expect(screen.getByTestId('panel-files')).toBeVisible()
    expect(screen.getByTestId('panel-chats')).not.toBeVisible()
  })

  it('falls back to Chats when focus moves to a workspace without a repo while Git shows', () => {
    publishFocusedWorkspaceContext(BRANCH_CTX)
    useSidebarStore.setState({ activeTab: 'git' })
    renderBody()
    expect(screen.getByTestId('panel-git')).toBeVisible()

    act(() => publishFocusedWorkspaceContext(HOME_CTX))

    expect(screen.getByTestId('panel-chats')).toBeVisible()
    expect(screen.getByTestId('panel-git')).not.toBeVisible()
  })

  it('a persisted Git mode with no repo focused shows Chats', () => {
    publishFocusedWorkspaceContext(HOME_CTX)
    useSidebarStore.setState({ activeTab: 'git' })
    renderBody()
    expect(screen.getByTestId('panel-chats')).toBeVisible()
  })

  // The Files panel follows the focused workspace context, never the route.
  it('scopes Files to the focused workspace and follows focus', () => {
    publishFocusedWorkspaceContext(BRANCH_CTX)
    renderBody()
    expect(treeProps).toHaveBeenLastCalledWith(
      expect.objectContaining({ workspaceId: 'ws-branch', rootFolderPath: '/worktrees/branch' }),
    )

    act(() => publishFocusedWorkspaceContext(HOME_CTX))

    expect(treeProps).toHaveBeenLastCalledWith(
      expect.objectContaining({ workspaceId: 'ws-home', rootFolderPath: '/projects/p1' }),
    )
  })

  it('renders the mode bar on an empty stage, with Chats showing', () => {
    publishFocusedWorkspaceContext(EMPTY_FOCUSED_WORKSPACE_CONTEXT)
    useSidebarStore.setState({ activeTab: 'files' })
    renderBody()
    expect(screen.getByRole('tablist', { name: 'Sidebar mode' })).toBeInTheDocument()
    expect(screen.getByTestId('panel-chats')).toBeVisible()
    expect(screen.getByRole('tab', { name: 'Files' })).toHaveAttribute('aria-disabled', 'true')
  })

  it('puts the project switcher on the first line, above the mode bar', () => {
    renderBody()
    const switcher = screen.getByTestId('project-switcher')
    const modeBar = screen.getByRole('tablist', { name: 'Sidebar mode' })
    expect(switcher.compareDocumentPosition(modeBar)).toBe(Node.DOCUMENT_POSITION_FOLLOWING)
    expect(screen.queryByRole('button', { name: 'Settings' })).toBeNull()
  })
})
