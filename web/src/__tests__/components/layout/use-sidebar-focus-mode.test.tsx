import { act, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useSidebarFocusMode } from '@/components/layout/use-sidebar-focus-mode'
import { getDefaultSettingsSnapshot } from '@/features/settings/config/default-settings'
import { useSettingsStore } from '@/features/settings/store'
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

// The user's first press: until one lands, focus is startup's, not the user's.
function press() {
  fireEvent.pointerDown(document.body)
}

function renderShell() {
  const view = render(<Shell />)
  press()
  return view
}

function setFollowsFocus(sidebarFollowsFocus: boolean) {
  act(() => {
    useSettingsStore.setState({
      settings: { ...getDefaultSettingsSnapshot(), sidebarFollowsFocus },
    })
  })
}

describe('useSidebarFocusMode', () => {
  beforeEach(() => {
    useSidebarStore.setState(getInitialState())
    setFollowsFocus(true)
    publishFocusedWorkspaceContext(REPO_CTX)
  })

  it('startup focus across surfaces, before the user presses anything, keeps the persisted mode', () => {
    act(() => useSidebarStore.getState().setActiveTab('git'))
    render(<Shell />)
    focus('editor')
    focus('chat composer')
    expect(mode()).toBe('git')
    press()
    focus('editor')
    expect(mode()).toBe('files')
  })

  it('a key press also hands focus to the user', () => {
    act(() => useSidebarStore.getState().setActiveTab('git'))
    render(<Shell />)
    focus('chat composer')
    fireEvent.keyDown(document.body, { key: 'Tab' })
    focus('editor')
    expect(mode()).toBe('files')
  })

  it('focusing an editor surface shows Files', () => {
    renderShell()
    focus('editor')
    expect(mode()).toBe('files')
  })

  it('focusing a diff surface shows Git when the workspace has a repo', () => {
    renderShell()
    focusDiff()
    expect(mode()).toBe('git')
  })

  it('a diff surface leaves the mode alone when the workspace has no repo', () => {
    publishFocusedWorkspaceContext(HOME_CTX)
    renderShell()
    focus('editor')
    focusDiff()
    expect(mode()).toBe('files')
  })

  it('the chat column and a terminal inside the editor view both show Chats', () => {
    renderShell()
    focus('editor')
    focus('chat composer')
    expect(mode()).toBe('chats')
    focus('editor')
    focus('terminal')
    expect(mode()).toBe('chats')
  })

  it('with panes of different kinds on screen, the focused one decides', () => {
    renderShell()
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
    renderShell()
    focus('editor')
    act(() => useSidebarStore.getState().setActiveTab('git'))
    focus('second editor')
    focus('editor')
    expect(mode()).toBe('git')
    focus('chat composer')
    expect(mode()).toBe('chats')
  })

  it('focus entering an unmarked element, like a dialog, changes nothing', () => {
    renderShell()
    focus('editor')
    act(() => useSidebarStore.getState().setActiveTab('git'))
    focus('settings field')
    expect(mode()).toBe('git')
    // Closing the dialog hands focus back to the same editor: not a crossing.
    focus('editor')
    expect(mode()).toBe('git')
  })

  it('removes its focus and interaction listeners once unmounted', () => {
    const removed = vi.spyOn(document, 'removeEventListener')
    const { unmount } = renderShell()
    const editor = screen.getByLabelText('editor')
    const surface = editor.parentElement!
    unmount()
    const types = removed.mock.calls.map(([type]) => type)
    expect(types).toEqual(expect.arrayContaining(['focusin', 'pointerdown', 'keydown']))
    removed.mockRestore()
    document.body.appendChild(surface)
    act(() => editor.focus())
    expect(mode()).toBe('chats')
    surface.remove()
  })

  it('with the setting off, focus crossing surfaces never changes the mode', () => {
    setFollowsFocus(false)
    act(() => useSidebarStore.getState().setActiveTab('git'))
    renderShell()
    focus('editor')
    focus('chat composer')
    focusDiff()
    expect(mode()).toBe('git')
    act(() => useSidebarStore.getState().setActiveTab('files'))
    focus('terminal')
    expect(mode()).toBe('files')
  })

  it('turning the setting on makes the next focus crossing act, and off stops it', () => {
    setFollowsFocus(false)
    renderShell()
    focus('editor')
    expect(mode()).toBe('chats')
    setFollowsFocus(true)
    focus('chat composer')
    focus('editor')
    expect(mode()).toBe('files')
    setFollowsFocus(false)
    focus('chat composer')
    expect(mode()).toBe('files')
  })
})
