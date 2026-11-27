import { useCallback, useEffect, useMemo, useRef, type RefObject } from 'react'
import { useStore } from 'zustand'
import { ErrorBoundary } from '@/components/error-boundary'
import { documentUri } from '@/features/editor/documents/document-ref'
import { useWorkspaceReadOnly } from '@/features/editor/hooks/use-workspace-read-only'
import type { MarkdownAssetInfo } from '@/features/editor/markdown/plate/markdown-asset'
import {
  FallbackToSource,
  RichMarkdownEditor,
} from '@/features/editor/markdown/plate/markdown-editor-pane'
import { MarkdownViewButton } from '@/features/editor/markdown/plate/markdown-view-toggle'
import { toast } from '@/features/window/stores/toast-store'
import type { DocumentRef } from '@/lib/api/documents'
import type { PaneClient } from '@/sdk/pane-client'
import { APPLE_KEYBOARD } from '@/utils/platform'
import { documentSession } from './document-session'

type Shell = Pick<PaneClient, 'editDocument' | 'showSource' | 'saveDocument'>

interface ShellRichEditorProps {
  client: Shell
  doc: DocumentRef
  wsId: string
}

function isSaveChord(event: KeyboardEvent): boolean {
  const mod = APPLE_KEYBOARD ? event.metaKey : event.ctrlKey
  return mod && !event.shiftKey && !event.altKey && event.key.toLowerCase() === 's'
}

type Pending = RefObject<(() => void) | null>

function release(pending: Pending, flush: () => void) {
  if (pending.current === flush) pending.current = null
}

function saveOnChord(save: () => void) {
  return (event: KeyboardEvent) => {
    if (event.repeat || !isSaveChord(event)) return
    event.preventDefault()
    save()
  }
}

function usePendingEdit() {
  const pending: Pending = useRef<(() => void) | null>(null)
  const hold = useCallback((flush: () => void) => {
    pending.current = flush
    return () => release(pending, flush)
  }, [])
  const flush = useCallback(() => pending.current?.(), [])
  return { hold, flush }
}

function useSaveChord(save: () => void) {
  useEffect(() => {
    const onKeyDown = saveOnChord(save)
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [save])
}

export function ShellRichEditor({ client, doc, wsId }: ShellRichEditorProps) {
  const content = useStore(documentSession, (s) => s.text)
  const readOnly = useWorkspaceReadOnly(doc.authority === 'workspace' ? wsId : null)
  const { hold, flush } = usePendingEdit()
  const write = useCallback((text: string) => client.editDocument(text), [client])
  const asset = useMemo<MarkdownAssetInfo>(() => ({ doc }), [doc])
  const showSource = useCallback(() => {
    flush()
    client.showSource()
  }, [client, flush])
  const save = useCallback(() => {
    flush()
    client.saveDocument()
  }, [client, flush])
  const fallBack = useCallback(() => {
    client.showSource()
    toast.error('Rich markdown editor failed', 'Switched to source view for this file.')
  }, [client])
  useSaveChord(save)

  return (
    <div className="relative h-full">
      <div className="absolute right-2 top-2 z-[60] rounded-md bg-background/80 backdrop-blur-sm">
        <MarkdownViewButton view="rich" onClick={showSource} />
      </div>
      <ErrorBoundary fallback={<FallbackToSource onMount={fallBack} />}>
        <RichMarkdownEditor
          scrollKey={documentUri(doc)}
          content={content}
          write={write}
          holdPending={hold}
          asset={asset}
          linkWorkspace={doc.authority === 'host' ? null : wsId}
          readOnly={readOnly}
        />
      </ErrorBoundary>
    </div>
  )
}
