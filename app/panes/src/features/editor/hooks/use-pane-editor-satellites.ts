import { useEffect, useState } from 'react'
import { KeyCode, KeyMod } from 'monaco-editor/esm/vs/editor/editor.api.js'
import type * as Monaco from 'monaco-editor'
import { useStore } from 'zustand'
import type {
  ActiveEditorContext,
  ActiveEditorRegistry,
} from '@/features/editor/lib/active-editor-context'
import { documentRefOf, documentUri, refFromUri } from '@/features/editor/documents/document-ref'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { isEditorContent } from '@/features/panes/types/pane-content'
import { useEditorOptions, useEditorTheme, useModelOptions } from './use-editor-appearance'
import { useInlineBlame } from './use-inline-blame'

type StandaloneEditor = Monaco.editor.IStandaloneCodeEditor

export interface PaneEditorSatelliteDeps {
  registry: ActiveEditorRegistry
  readOnly?: boolean
  scrollable?: boolean
  isActiveSurface?: boolean
}

interface BoundEditor {
  editor: StandaloneEditor | null
  model: Monaco.editor.ITextModel | null
  uri: string
  filePath: string
}

const UNBOUND: BoundEditor = { editor: null, model: null, uri: '', filePath: '' }

function toBound(ctx: ActiveEditorContext | undefined): BoundEditor {
  if (!ctx) return UNBOUND
  return {
    editor: (ctx.editor as StandaloneEditor | undefined) ?? null,
    model: (ctx.model as Monaco.editor.ITextModel | undefined) ?? null,
    uri: ctx.uri,
    filePath: ctx.filePath,
  }
}

export function usePaneEditorSatellites(paneId: string, deps: PaneEditorSatelliteDeps): void {
  const { registry, readOnly = false, scrollable = true, isActiveSurface = true } = deps

  const [bound, setBound] = useState<BoundEditor>(() => toBound(registry.get(paneId)))
  useEffect(() => registry.subscribe(paneId, (ctx) => setBound(toBound(ctx))), [paneId, registry])
  const { editor, model, uri, filePath } = bound
  const doc = refFromUri(uri)
  const workspaceId = doc?.authority === 'workspace' ? doc.workspaceId : null

  const languageOverride = useStore(windowPaneStore, (state) => {
    const own = state.buffers.find(
      (b) => isEditorContent(b) && documentUri(documentRefOf(b)) === uri,
    )
    return own && isEditorContent(own) ? own.languageOverride : undefined
  })

  useEditorTheme(editor)
  useEditorOptions(editor, { readOnly, scrollable })
  useModelOptions(model, filePath, languageOverride)
  useInlineBlame(editor, model, workspaceId, filePath)

  useEffect(() => {
    if (!editor) return
    editor.addCommand(KeyMod.CtrlCmd | KeyCode.KeyA, () => {
      const m = editor.getModel()
      if (m) editor.setSelection(m.getFullModelRange())
    })
  }, [editor])

  useEffect(() => {
    if (!editor || !model || !isActiveSurface || readOnly) return
    editor.focus()
  }, [editor, model, isActiveSurface, readOnly])
}
