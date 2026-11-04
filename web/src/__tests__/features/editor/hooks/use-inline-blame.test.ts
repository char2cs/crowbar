import { act, renderHook, waitFor } from '@testing-library/react'
import type * as Monaco from 'monaco-editor'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { apiFetch } = vi.hoisted(() => ({ apiFetch: vi.fn() }))
vi.mock('@/lib/api/fetch', () => ({ apiFetch }))

import { useInlineBlame } from '@/features/editor/hooks/use-inline-blame'
import { blameKey, useGitBlameStore } from '@/features/git/stores/git-blame-store'
import { useSettingsStore } from '@/features/settings/store'
import { __resetWorkspaceScopesForTest, recordWorkspaceScope } from '@/lib/workspace-scope'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

// The load effect only needs a bound editor and model; nothing is rendered
// because every load here answers with no entries.
const editor = {} as Monaco.editor.IStandaloneCodeEditor
const model = {} as Monaco.editor.ITextModel

beforeEach(() => {
  apiFetch.mockReset()
  apiFetch.mockResolvedValue([])
  useGitBlameStore.setState({ blame: {} })
  useSettingsStore.setState((s) => ({ settings: { ...s.settings, inlineBlame: true } }))
  resetHostWorld()
  __resetWorkspaceScopesForTest()
  recordWorkspaceScope({ projectId: 'p-local', repoId: 'r', wsId: 'ws-local', owningChatId: 'c-l' })
})
afterEach(() => resetHostWorld())

describe('useInlineBlame', () => {
  it('loads a file’s blame once the editor and model exist', async () => {
    renderHook(() => useInlineBlame(editor, model, 'ws-local', 'a.ts'))

    await waitFor(() => expect(apiFetch).toHaveBeenCalledTimes(1))
    expect(apiFetch.mock.calls[0][0]).toBe('/v0/chats/c-l/blame?path=a.ts')
  })

  it('never asks the daemon to blame an untitled buffer, which has no file', () => {
    renderHook(() => useInlineBlame(editor, model, 'ws-local', 'untitled:Untitled-1'))

    expect(apiFetch).not.toHaveBeenCalled()
    expect(useGitBlameStore.getState().blame[blameKey('ws-local', 'untitled:Untitled-1')]).toBe(
      undefined,
    )
  })

  describe('on an install that has paired a remote host', () => {
    beforeEach(() => {
      pairRemoteHost('host-1')
      recordWorkspaceScope({
        projectId: 'p-remote',
        repoId: 'r',
        wsId: 'ws-remote',
        owningChatId: 'c-r',
      })
    })

    // A failed load is recorded and never retried until the file is saved or closed.
    it('asks nothing, and records no failure, until the project list names the host', async () => {
      renderHook(() => useInlineBlame(editor, model, 'ws-remote', 'a.ts'))
      await act(async () => {})
      expect(apiFetch).not.toHaveBeenCalled()
      expect(useGitBlameStore.getState().blame[blameKey('ws-remote', 'a.ts')]).toBeUndefined()

      act(() => loadProjects(projectOn('p-remote', 'host-1')))

      await waitFor(() => expect(apiFetch).toHaveBeenCalledTimes(1))
      expect(apiFetch.mock.calls[0][0]).toBe('crowbar://host-1/v0/chats/c-r/blame?path=a.ts')
    })
  })
})
