/**
 * usePaneEditorSatellites — everything bound to a pane's retained Monaco
 * widget besides the model swap itself (usePaneEditorController owns that):
 *
 *  - theme + settings-driven options (use-editor-appearance.ts),
 *  - the model's language/indentation after each swap,
 *  - store → model sync for genuine external changes (disk reload,
 *    format-on-save),
 *  - the LSP document lifecycle and diagnostics markers
 *    (use-lsp-document-sync.ts),
 *  - current-line git blame (use-inline-blame.ts),
 *  - focus on swap.
 *
 * The retained editor + current model come from the pane's active-editor
 * registry, which the controller publishes on every swap.
 */
import type React from 'react'
import { useEffect, useState } from 'react'
import { KeyCode, KeyMod } from 'monaco-editor/esm/vs/editor/editor.api.js'
import type * as Monaco from 'monaco-editor'
import { useStore } from 'zustand'
import type {
  ActiveEditorContext,
  ActiveEditorRegistry,
} from '@/features/editor/lib/active-editor-context'
import type { EditorManager } from '@/features/editor/lib/editor-manager'
import { fileUri } from '@/features/editor/lib/editor-uri'
import { shouldReconcileModelFromStore } from '@/features/editor/lib/pane-editor-controller'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { isEditorContent, type PaneContent } from '@/features/panes/types/pane-content'
import { getLanguageIdFromPath } from '../utils/language-id'
import { useEditorOptions, useEditorTheme, useModelOptions } from './use-editor-appearance'
import { useInlineBlame } from './use-inline-blame'
import { useLspDocumentSync } from './use-lsp-document-sync'

type StandaloneEditor = Monaco.editor.IStandaloneCodeEditor

export interface PaneEditorSatelliteDeps {
  /**
   * The active-editor registry and Monaco manager for the BUFFER'S OWN
   * workspace — the values EditorSurface resolved from its `workspaceId`
   * prop, not the ambient (pane chat's) workspace: a pane can hold a chat
   * from one workspace and a file from another.
   */
  registry: ActiveEditorRegistry
  editorManager: EditorManager
  /** Same workspace as `editorManager` — half of the model uri. */
  workspaceId: string
  readOnly?: boolean
  scrollable?: boolean
  isActiveSurface?: boolean
  /**
   * Latch the surface's content-write seam reads to ignore the model-change
   * event a GENUINE external edit (applied here) re-fires, so it does not
   * bounce back to the buffer store. Set to the applied text just before.
   */
  externalApplyRef?: React.MutableRefObject<string | null>
}

interface BoundEditor {
  editor: StandaloneEditor | null
  model: Monaco.editor.ITextModel | null
  filePath: string
}

const UNBOUND: BoundEditor = { editor: null, model: null, filePath: '' }

function toBound(ctx: ActiveEditorContext | undefined): BoundEditor {
  if (!ctx) return UNBOUND
  return {
    editor: (ctx.editor as StandaloneEditor | undefined) ?? null,
    model: (ctx.model as Monaco.editor.ITextModel | undefined) ?? null,
    filePath: ctx.filePath,
  }
}

function activeEditorBuffer(
  state: { panes: Record<string, { activeEditorTabId?: string | null }>; buffers: PaneContent[] },
  paneId: string,
) {
  const id = state.panes[paneId]?.activeEditorTabId ?? null
  const buffer = id ? state.buffers.find((b) => b.id === id) : null
  return buffer && isEditorContent(buffer) ? buffer : null
}

export function usePaneEditorSatellites(paneId: string, deps: PaneEditorSatelliteDeps): void {
  const {
    registry,
    editorManager,
    workspaceId,
    readOnly = false,
    scrollable = true,
    isActiveSurface = true,
    externalApplyRef,
  } = deps

  const [bound, setBound] = useState<BoundEditor>(() => toBound(registry.get(paneId)))
  useEffect(() => registry.subscribe(paneId, (ctx) => setBound(toBound(ctx))), [paneId, registry])
  const { editor, model, filePath } = bound

  const languageOverride = useStore(
    windowPaneStore,
    (state) => activeEditorBuffer(state, paneId)?.languageOverride,
  )
  const languageId = languageOverride ?? getLanguageIdFromPath(filePath) ?? 'plaintext'

  useEditorTheme(editor)
  useEditorOptions(editor, { readOnly, scrollable })
  useModelOptions(model, filePath, languageOverride)
  useLspDocumentSync(model, filePath, languageId, workspaceId)
  useInlineBlame(editor, model, workspaceId, filePath)

  // Cmd/Ctrl+A selects the whole model even when an app-level shortcut would
  // otherwise claim the keystroke first.
  useEffect(() => {
    if (!editor) return
    editor.addCommand(KeyMod.CtrlCmd | KeyCode.KeyA, () => {
      const m = editor.getModel()
      if (m) editor.setSelection(m.getFullModelRange())
    })
  }, [editor])

  // ── Store → model: genuine external changes only ──────────────────────────
  // Local typing flows model → sink → store, so by the time the store fires
  // the model already holds that text and nothing is applied.
  useEffect(() => {
    if (!editor || !model || !filePath) return
    const apply = (content: string) => {
      if (model.isDisposed() || model.getValue() === content) return
      const selection = editor.getSelection()
      if (externalApplyRef) externalApplyRef.current = content
      editorManager.applyExternalEdit(paneId, fileUri(workspaceId, filePath), content)
      if (selection) editor.setSelection(selection)
    }
    // On (re)bind the store is authoritative only for a CLEAN buffer: a dirty
    // one's model is ahead of the store with un-flushed keystrokes (I2).
    const initial = activeEditorBuffer(windowPaneStore.getState(), paneId)
    if (initial && shouldReconcileModelFromStore(initial)) apply(initial.content)
    let previous = initial?.content
    return windowPaneStore.subscribe((state) => {
      const buffer = activeEditorBuffer(state, paneId)
      if (!buffer || buffer.path !== filePath || buffer.content === previous) return
      previous = buffer.content
      apply(buffer.content)
    })
  }, [editor, model, filePath, editorManager, externalApplyRef, paneId, workspaceId])

  // ── Focus the editor when it shows a new model on the active surface ──────
  useEffect(() => {
    if (!editor || !model || !isActiveSurface || readOnly) return
    editor.focus()
  }, [editor, model, isActiveSurface, readOnly])
}
