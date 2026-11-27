import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { DocumentRef } from '@/lib/api/documents'
import { ErrorBoundary } from '@/components/error-boundary'
import { useBufferById } from '@/features/workspace/stores/hooks/use-buffer-store'
import { useWorkspaceReadOnly } from '@/features/editor/hooks/use-workspace-read-only'
import { isEditorContent } from '@/features/panes/types/pane-content'
import { documentRefOf, documentUri, refFromUri } from '@/features/editor/documents/document-ref'
import {
  documentText,
  onExternalText,
  registerPendingWriter,
  replaceDocumentText,
} from '@/features/editor/documents/document-text'
import { useDocumentText } from '@/features/editor/documents/use-document-text'
import { toast } from '@/features/window/stores/toast-store'
import { useMarkdownViewStore } from './markdown-view-store'
import { MarkdownViewToggle } from './markdown-view-toggle'
import type { Value } from 'platejs'
import { Plate, PlateContent, usePlateEditor } from 'platejs/react'
import { usePreservedScroll } from '@/features/editor/hooks/use-preserved-scroll'
import { LinkWorkspaceContext } from '@/lib/markdown-link'
import { MarkdownAssetContext, type MarkdownAssetInfo } from './markdown-asset'
import { joinFrontmatter, splitFrontmatter } from './markdown-frontmatter'
import { markdownPlugins } from './markdown-plugins'
import type { MarkdownSource } from './markdown-source'
import { parseMarkdown, plateValueToMarkdown } from './markdown-serialization'
import './markdown-editor.css'
const SINK_DELAY_MS = 150

export interface RichMarkdownDocument {
  scrollKey: string
  content: string | null
  write: ((text: string) => void) | null
  holdPending: (flush: () => void) => () => void
  followExternal?: (adopt: (text: string) => void) => () => void
  asset: MarkdownAssetInfo | null
  linkWorkspace: string | null
  readOnly: boolean
}

export function RichMarkdownEditor({
  scrollKey,
  content,
  write,
  holdPending,
  followExternal,
  asset: assetInfo,
  linkWorkspace,
  readOnly,
}: RichMarkdownDocument) {
  const initial = useMemo<{ frontmatter: string; initialValue: Value; source: MarkdownSource }>(
    () => {
      const { frontmatter, body } = splitFrontmatter(content ?? '')
      const { value, source } = parseMarkdown(body)
      return { frontmatter, initialValue: value, source }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps -- initial value only, deliberately not re-derived on content change
    [],
  )

  const frontmatterRef = useRef(initial.frontmatter)
  const [frontmatterText, setFrontmatterText] = useState(initial.frontmatter)
  const sourceRef = useRef(initial.source)

  const editor = usePlateEditor({ plugins: markdownPlugins, value: initial.initialValue })

  const baselineRef = useRef<string | undefined>(undefined)
  if (baselineRef.current === undefined) {
    baselineRef.current = joinFrontmatter(
      frontmatterRef.current,
      plateValueToMarkdown(editor.children as Value, sourceRef.current),
    )
  }

  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const syncedContentRef = useRef<string | null>(content)

  const flush = useCallback(() => {
    if (timerRef.current) {
      clearTimeout(timerRef.current)
      timerRef.current = null
    }
    if (!write) return
    const md = joinFrontmatter(
      frontmatterRef.current,
      plateValueToMarkdown(editor.children as Value, sourceRef.current),
    )
    if (md === baselineRef.current) {
      return
    }
    write(md)
    baselineRef.current = md
    syncedContentRef.current = md
  }, [write, editor])

  const onChange = useCallback(() => {
    const ops = editor.operations
    if (ops.length > 0 && ops.every((op) => op.type === 'set_selection')) return
    if (timerRef.current) clearTimeout(timerRef.current)
    timerRef.current = setTimeout(flush, SINK_DELAY_MS)
  }, [editor, flush])

  const adopt = useCallback(
    (text: string) => {
      if (text === syncedContentRef.current) return
      const current = joinFrontmatter(
        frontmatterRef.current,
        plateValueToMarkdown(editor.children as Value, sourceRef.current),
      )
      if (current !== baselineRef.current) return

      syncedContentRef.current = text
      const { frontmatter, body } = splitFrontmatter(text)
      frontmatterRef.current = frontmatter
      setFrontmatterText(frontmatter)
      const parsed = parseMarkdown(body)
      sourceRef.current = parsed.source
      editor.tf.setValue(parsed.value)
      baselineRef.current = joinFrontmatter(
        frontmatter,
        plateValueToMarkdown(editor.children as Value, sourceRef.current),
      )
    },
    [editor],
  )

  useEffect(() => followExternal?.(adopt), [followExternal, adopt])

  useEffect(() => {
    if (content !== null) adopt(content)
  }, [content, adopt])

  const scrollRef = useRef<HTMLDivElement>(null)
  usePreservedScroll(scrollRef, scrollKey, content !== null)

  useEffect(() => {
    const stop = holdPending(flush)
    return () => {
      stop()
      flush()
    }
  }, [holdPending, flush])

  return (
    <MarkdownAssetContext.Provider value={assetInfo}>
      <LinkWorkspaceContext value={linkWorkspace}>
        <Plate editor={editor} onChange={onChange}>
          <div
            ref={scrollRef}
            className="crowbar-markdown-editor flex h-full flex-col overflow-y-auto"
          >
            {frontmatterText && <MarkdownFrontmatterBanner frontmatter={frontmatterText} />}
            <PlateContent className="outline-none" placeholder="Write…" readOnly={readOnly} />
          </div>
        </Plate>
      </LinkWorkspaceContext>
    </MarkdownAssetContext.Provider>
  )
}

function MarkdownFrontmatterBanner({ frontmatter }: { frontmatter: string }) {
  const display = frontmatter.replace(/\r?\n$/, '')
  return (
    <div className="crowbar-markdown-editor__frontmatter shrink-0 border-b border-border bg-muted/40 py-3">
      <div className="mb-1 flex items-center gap-1.5 text-[0.6875rem] text-muted-foreground/80">
        <span className="font-medium uppercase tracking-wider">Frontmatter</span>
        <span aria-hidden>·</span>
        <span>Read-only here — edit in Source view</span>
      </div>
      <pre className="overflow-x-auto whitespace-pre-wrap font-mono text-xs leading-relaxed text-muted-foreground">
        {display}
      </pre>
    </div>
  )
}

interface MarkdownEditorPaneProps {
  paneId: string
  bufferId: string
}

export function MarkdownEditorPane({ paneId, bufferId }: MarkdownEditorPaneProps) {
  void paneId

  const buffer = useBufferById(bufferId)
  const uri =
    buffer && isEditorContent(buffer) && buffer.path ? documentUri(documentRefOf(buffer)) : null
  const doc = useMemo(() => (uri ? refFromUri(uri) : null), [uri])
  const content = useDocumentText(doc)

  const fallbackToSource = useCallback(() => {
    useMarkdownViewStore.getState().setView(bufferId, 'source')
    toast.error('Rich markdown editor failed', 'Switched to source view for this file.')
  }, [bufferId])

  return (
    <div className="relative h-full">
      <div className="absolute right-2 top-2 z-[60] rounded-md bg-background/80 backdrop-blur-sm">
        <MarkdownViewToggle bufferId={bufferId} />
      </div>
      <ErrorBoundary key={bufferId} fallback={<FallbackToSource onMount={fallbackToSource} />}>
        <MarkdownRichEditor bufferId={bufferId} doc={doc} content={content} />
      </ErrorBoundary>
    </div>
  )
}

function MarkdownRichEditor({
  bufferId,
  doc,
  content,
}: {
  bufferId: string
  doc: DocumentRef | null
  content: string | null
}) {
  const assetBuffer = useBufferById(bufferId)
  const readOnly = useWorkspaceReadOnly(
    assetBuffer && isEditorContent(assetBuffer) ? assetBuffer.workspaceId : null,
  )
  const assetUri =
    assetBuffer && isEditorContent(assetBuffer) && !assetBuffer.isVirtual
      ? documentUri(documentRefOf(assetBuffer))
      : null
  const asset = useMemo<MarkdownAssetInfo | null>(() => {
    const ref = assetUri ? refFromUri(assetUri) : null
    return ref ? { doc: ref } : null
  }, [assetUri])
  const linkWorkspace =
    assetBuffer && !(isEditorContent(assetBuffer) && assetBuffer.authority === 'host')
      ? assetBuffer.workspaceId
      : null
  const write = useMemo(
    () => (doc ? (text: string) => replaceDocumentText(doc, text) : null),
    [doc],
  )
  const holdPending = useCallback(
    (flush: () => void) => (doc ? registerPendingWriter(doc, flush) : () => {}),
    [doc],
  )
  const followExternal = useCallback(
    (adopt: (text: string) => void) => {
      if (!doc) return () => {}
      return onExternalText(doc, () => {
        const text = documentText(doc)
        if (text !== null) adopt(text)
      })
    },
    [doc],
  )
  return (
    <RichMarkdownEditor
      scrollKey={bufferId}
      content={content}
      write={write}
      holdPending={holdPending}
      followExternal={followExternal}
      asset={asset}
      linkWorkspace={linkWorkspace}
      readOnly={readOnly}
    />
  )
}

export function FallbackToSource({ onMount }: { onMount: () => void }) {
  useEffect(() => {
    onMount()
    // eslint-disable-next-line react-hooks/exhaustive-deps -- run once on mount
  }, [])
  return (
    <div className="flex h-full items-center justify-center p-8 text-sm text-muted-foreground">
      Rich markdown failed to load — switching to source view.
    </div>
  )
}
