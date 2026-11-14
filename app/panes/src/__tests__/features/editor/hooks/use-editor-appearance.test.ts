import { renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type * as Monaco from 'monaco-editor'
import { useSettingsStore } from '@/features/settings/store'

vi.mock('monaco-editor/esm/vs/editor/editor.api.js', () => ({ editor: { setTheme: vi.fn() } }))
const platform = vi.hoisted(() => ({ mobile: false }))
vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  get IS_MOBILE() {
    return platform.mobile
  },
}))

const { useEditorOptions } = await import('@/features/editor/hooks/use-editor-appearance')

const fakeEditor = () => {
  const updateOptions = vi.fn()
  return {
    editor: { updateOptions } as unknown as Monaco.editor.IStandaloneCodeEditor,
    updateOptions,
  }
}

afterEach(() => {
  platform.mobile = false
})

describe('useEditorOptions', () => {
  it('uses the settings font size', () => {
    useSettingsStore.setState((s) => {
      s.settings.fontSize = 13
    })
    const { editor, updateOptions } = fakeEditor()
    renderHook(() => useEditorOptions(editor, { readOnly: false, scrollable: true }))
    expect(updateOptions).toHaveBeenCalledWith(expect.objectContaining({ fontSize: 13 }))
  })

  // Under 16px iOS zooms the page into any field that takes focus, Monaco's input included.
  it('never goes under 16px on the phone', () => {
    platform.mobile = true
    useSettingsStore.setState((s) => {
      s.settings.fontSize = 13
    })
    const { editor, updateOptions } = fakeEditor()
    renderHook(() => useEditorOptions(editor, { readOnly: false, scrollable: true }))
    expect(updateOptions).toHaveBeenCalledWith(expect.objectContaining({ fontSize: 16 }))
  })
})
