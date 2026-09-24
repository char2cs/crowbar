/**
 * Settings → retained Monaco widget: theme and editor options. Bound to the
 * pane's editor instance, re-applied only when a setting (or the instance)
 * changes — never on a tab switch.
 */
import { useEffect, useSyncExternalStore } from 'react'
import { editor as monacoEditor } from 'monaco-editor/esm/vs/editor/editor.api.js'
import type * as Monaco from 'monaco-editor'
import { themeRegistry } from '@/extensions/themes/theme-registry'
import { useSettingsStore } from '@/features/settings/store'
import { useZoomStore } from '@/features/window/stores/zoom-store'
import { defineMonacoTheme } from '../monaco/define-theme'
import { toMonacoLanguageId } from '../monaco/language'
import { getLanguageIdFromPath } from '../utils/language-id'
import { calculateLineHeight } from '../utils/lines'

type StandaloneEditor = Monaco.editor.IStandaloneCodeEditor

// The app flips light/dark by toggling a class on <html>, not through a store,
// so a MutationObserver is the only way an effect learns a mode change
// happened (defineMonacoTheme reads the class itself; this only re-runs it).
let darkModeVersion = 0
const darkModeListeners = new Set<() => void>()
let darkModeObserver: MutationObserver | null = null

function subscribeDarkMode(listener: () => void): () => void {
  if (!darkModeObserver && typeof MutationObserver !== 'undefined') {
    darkModeObserver = new MutationObserver(() => {
      darkModeVersion++
      darkModeListeners.forEach((l) => l())
    })
    darkModeObserver.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class'],
    })
  }
  darkModeListeners.add(listener)
  return () => darkModeListeners.delete(listener)
}

function useDarkModeVersion(): number {
  return useSyncExternalStore(
    subscribeDarkMode,
    () => darkModeVersion,
    () => 0,
  )
}

/** Apply the color theme to the editor (and follow theme-registry changes). */
export function useEditorTheme(editor: StandaloneEditor | null): void {
  const theme = useSettingsStore((s) => s.settings.theme)
  const darkMode = useDarkModeVersion()
  useEffect(() => {
    if (!editor) return
    const apply = () => monacoEditor.setTheme(defineMonacoTheme(theme || 'crowbar-dark'))
    apply()
    const offRegistry = themeRegistry.onRegistryChange(apply)
    const offTheme = themeRegistry.onThemeChange(apply)
    return () => {
      offRegistry()
      offTheme()
    }
  }, [editor, theme, darkMode])
}

export interface EditorOptionInputs {
  readOnly: boolean
  scrollable: boolean
}

/** Widget-level options from settings. */
export function useEditorOptions(
  editor: StandaloneEditor | null,
  { readOnly, scrollable }: EditorOptionInputs,
): void {
  const baseFontSize = useSettingsStore((s) => s.settings.fontSize)
  const fontFamily = useSettingsStore((s) => s.settings.fontFamily)
  const lineHeightSetting = useSettingsStore((s) => s.settings.editorLineHeight)
  const tabSize = useSettingsStore((s) => s.settings.tabSize)
  const wordWrap = useSettingsStore((s) => s.settings.wordWrap)
  const lineNumbers = useSettingsStore((s) => s.settings.lineNumbers)
  const renderWhitespace = useSettingsStore((s) => s.settings.renderWhitespace)
  const renderIndentGuides = useSettingsStore((s) => s.settings.renderIndentGuides)
  const semanticHighlighting = useSettingsStore((s) => s.settings.semanticHighlighting)
  const highlightOccurrences = useSettingsStore((s) => s.settings.highlightOccurrences)
  const minimap = useSettingsStore((s) => s.settings.showMinimap)
  const autoCompletion = useSettingsStore((s) => s.settings.autoCompletion)
  const parameterHints = useSettingsStore((s) => s.settings.parameterHints)
  const zoomLevel = useZoomStore.use.editorZoomLevel()

  const fontSize = baseFontSize * zoomLevel
  const lineHeight = calculateLineHeight(fontSize, lineHeightSetting)

  useEffect(() => {
    if (!editor) return
    editor.updateOptions({
      fontFamily,
      fontSize,
      lineHeight,
      tabSize,
      readOnly,
      domReadOnly: readOnly,
      lineNumbers: lineNumbers ? 'on' : 'off',
      minimap: { enabled: minimap },
      renderWhitespace,
      wordWrap: wordWrap ? 'on' : 'off',
      guides: { indentation: renderIndentGuides, highlightActiveIndentation: renderIndentGuides },
      'semanticHighlighting.enabled': semanticHighlighting,
      occurrencesHighlight: highlightOccurrences ? 'singleFile' : 'off',
      quickSuggestions: autoCompletion,
      suggestOnTriggerCharacters: autoCompletion,
      parameterHints: { enabled: parameterHints },
      scrollbar: {
        vertical: scrollable ? 'auto' : 'hidden',
        horizontal: scrollable ? 'auto' : 'hidden',
      },
    })
  }, [
    editor,
    autoCompletion,
    fontFamily,
    fontSize,
    highlightOccurrences,
    lineHeight,
    lineNumbers,
    minimap,
    parameterHints,
    readOnly,
    renderIndentGuides,
    renderWhitespace,
    scrollable,
    semanticHighlighting,
    tabSize,
    wordWrap,
  ])
}

/**
 * Model-level options that a fresh model does not carry: language (the
 * buffer's override wins over the path's) and indentation. Re-applied on
 * each swap to a new model.
 */
export function useModelOptions(
  model: Monaco.editor.ITextModel | null,
  filePath: string,
  languageOverride: string | undefined,
): void {
  const tabSize = useSettingsStore((s) => s.settings.tabSize)
  const monacoLanguageId = toMonacoLanguageId(languageOverride ?? getLanguageIdFromPath(filePath))
  useEffect(() => {
    if (!model || model.isDisposed()) return
    monacoEditor.setModelLanguage(model, monacoLanguageId)
    model.updateOptions({ tabSize, insertSpaces: true })
  }, [model, monacoLanguageId, tabSize])
}
