import { act, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { SidebarModeBar } from '@/components/layout/sidebar-mode-bar'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'
import {
  EMPTY_FOCUSED_WORKSPACE_CONTEXT,
  publishFocusedWorkspaceContext,
  type FocusedWorkspaceContext,
} from '@/features/window/stores/focused-workspace-context-store'

const REPO_CTX: FocusedWorkspaceContext = {
  projectId: 'p1',
  workspaceId: 'ws-repo',
  isProjectHome: false,
  repoId: 'r1',
  repoPath: '/repo',
  rootPath: '/repo',
}
const HOME_CTX: FocusedWorkspaceContext = {
  projectId: 'p1',
  workspaceId: 'ws-home',
  isProjectHome: true,
  repoId: null,
  repoPath: null,
  rootPath: '/projects/p1',
}

function segment(name: string): HTMLElement {
  return screen.getByRole('tab', { name })
}

describe('SidebarModeBar', () => {
  beforeEach(() => {
    useSidebarStore.setState(getInitialState())
    publishFocusedWorkspaceContext(REPO_CTX)
  })

  it('holds Chats, Files and Git, in that order', () => {
    render(<SidebarModeBar />)
    expect(screen.getAllByRole('tab').map((t) => t.getAttribute('aria-label'))).toEqual([
      'Chats',
      'Files',
      'Git',
    ])
  })

  it('selects Chats by default', () => {
    render(<SidebarModeBar />)
    expect(segment('Chats')).toHaveAttribute('aria-selected', 'true')
    expect(segment('Files')).toHaveAttribute('aria-selected', 'false')
    expect(segment('Git')).toHaveAttribute('aria-selected', 'false')
  })

  it('selecting a segment writes the mode to the store', () => {
    render(<SidebarModeBar />)
    fireEvent.click(segment('Git'))
    expect(useSidebarStore.getState().activeTab).toBe('git')
    expect(segment('Git')).toHaveAttribute('aria-selected', 'true')
    expect(segment('Chats')).toHaveAttribute('aria-selected', 'false')
  })

  describe('availability follows the focused workspace', () => {
    it('Git is disabled for a workspace with no repo, and cannot be selected', () => {
      publishFocusedWorkspaceContext(HOME_CTX)
      render(<SidebarModeBar />)
      expect(segment('Git')).toBeDisabled()
      expect(segment('Files')).toBeEnabled()
      fireEvent.click(segment('Git'))
      expect(useSidebarStore.getState().activeTab).toBe('chats')
    })

    it('with no focused workspace, Files and Git are disabled and Chats stays usable', () => {
      publishFocusedWorkspaceContext(EMPTY_FOCUSED_WORKSPACE_CONTEXT)
      render(<SidebarModeBar />)
      expect(segment('Chats')).toBeEnabled()
      expect(segment('Files')).toBeDisabled()
      expect(segment('Git')).toBeDisabled()
    })

    it('shows Chats as selected once the chosen mode becomes unavailable', () => {
      useSidebarStore.setState({ activeTab: 'git' })
      render(<SidebarModeBar />)
      expect(segment('Git')).toHaveAttribute('aria-selected', 'true')

      act(() => publishFocusedWorkspaceContext(HOME_CTX))

      expect(segment('Git')).toHaveAttribute('aria-selected', 'false')
      expect(segment('Chats')).toHaveAttribute('aria-selected', 'true')
    })
  })
})
