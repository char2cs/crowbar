import { act, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { useSidebarFocusMode } from '@/components/layout/use-sidebar-focus-mode'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'
import {
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

function Shell() {
  useSidebarFocusMode()
  return (
    <>
      <div data-sidebar-surface="chats">
        <textarea aria-label="chat composer" />
      </div>
      <div data-sidebar-surface="files">
        <textarea aria-label="editor" />
        <div data-sidebar-surface="chats">
          <textarea aria-label="terminal" />
        </div>
      </div>
      <div data-sidebar-surface="files">
        <textarea aria-label="second editor" />
      </div>
      <div data-sidebar-surface="git">
        <button type="button">diff</button>
      </div>
      <div role="dialog">
        <input aria-label="settings field" />
      </div>
    </>
  )
}

function focus(name: string) {
  act(() => screen.getByLabelText(name).focus())
}

function focusDiff() {
  act(() => screen.getByRole('button', { name: 'diff' }).focus())
}

const mode = () => useSidebarStore.getState().activeTab

describe('useSidebarFocusMode', () => {
  beforeEach(() => {
    useSidebarStore.setState(getInitialState())
    publishFocusedWorkspaceContext(REPO_CTX)
  })

  it('focusing an editor surface shows Files', () => {
    render(<Shell />)
    focus('editor')
    expect(mode()).toBe('files')
  })

  it('focusing a diff surface shows Git when the workspace has a repo', () => {
    render(<Shell />)
    focusDiff()
    expect(mode()).toBe('git')
  })

  it('a diff surface leaves the mode alone when the workspace has no repo', () => {
    publishFocusedWorkspaceContext(HOME_CTX)
    render(<Shell />)
    focus('editor')
    focusDiff()
    expect(mode()).toBe('files')
  })

  it('the chat column and a terminal inside the editor view both show Chats', () => {
    render(<Shell />)
    focus('editor')
    focus('chat composer')
    expect(mode()).toBe('chats')
    focus('editor')
    focus('terminal')
    expect(mode()).toBe('chats')
  })

  it('with panes of different kinds on screen, the focused one decides', () => {
    render(<Shell />)
    focus('editor')
    expect(mode()).toBe('files')
    focusDiff()
    expect(mode()).toBe('git')
    focus('chat composer')
    expect(mode()).toBe('chats')
    focus('second editor')
    expect(mode()).toBe('files')
  })

  it('a manual choice holds while focus stays in the same kind, until it crosses kinds', () => {
    render(<Shell />)
    focus('editor')
    act(() => useSidebarStore.getState().setActiveTab('git'))
    focus('second editor')
    focus('editor')
    expect(mode()).toBe('git')
    focus('chat composer')
    expect(mode()).toBe('chats')
  })

  it('focus entering an unmarked element, like a dialog, changes nothing', () => {
    render(<Shell />)
    focus('editor')
    act(() => useSidebarStore.getState().setActiveTab('git'))
    focus('settings field')
    expect(mode()).toBe('git')
    // Closing the dialog hands focus back to the same editor: not a crossing.
    focus('editor')
    expect(mode()).toBe('git')
  })

  it('stops listening once unmounted', () => {
    const { unmount } = render(<Shell />)
    const editor = screen.getByLabelText('editor')
    const surface = editor.parentElement!
    unmount()
    document.body.appendChild(surface)
    act(() => editor.focus())
    expect(mode()).toBe('chats')
    surface.remove()
  })
})
