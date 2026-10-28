import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import type * as Monaco from 'monaco-editor'

const { getBlame } = vi.hoisted(() => ({ getBlame: vi.fn() }))
vi.mock('@/features/git/api/git-blame-api', () => ({ getBlame }))

import { useInlineBlame } from '@/features/editor/hooks/use-inline-blame'
import { blameKey, useGitBlameStore } from '@/features/git/stores/git-blame-store'
import { useSettingsStore } from '@/features/settings/store'

// The load effect only needs a bound editor and model; nothing is rendered
// because every load here answers with no entries.
const editor = {} as Monaco.editor.IStandaloneCodeEditor
const model = {} as Monaco.editor.ITextModel

beforeEach(() => {
  getBlame.mockReset()
  getBlame.mockResolvedValue([])
  useGitBlameStore.setState({ blame: {} })
  useSettingsStore.setState((s) => ({ settings: { ...s.settings, inlineBlame: true } }))
})

describe('useInlineBlame', () => {
  it("loads a saved file's blame", async () => {
    renderHook(() => useInlineBlame(editor, model, 'ws-1', 'src/a.ts'))

    await waitFor(() => expect(getBlame).toHaveBeenCalledWith('ws-1', 'src/a.ts'))
  })

  it('never asks the daemon to blame an untitled buffer, which has no file', () => {
    renderHook(() => useInlineBlame(editor, model, 'ws-1', 'untitled:Untitled-1'))

    expect(getBlame).not.toHaveBeenCalled()
    expect(useGitBlameStore.getState().blame[blameKey('ws-1', 'untitled:Untitled-1')]).toBe(
      undefined,
    )
  })
})
