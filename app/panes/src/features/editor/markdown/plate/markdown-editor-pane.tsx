import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
// `Value` resolves from `platejs` (re-exported from `@platejs/slate`), not
// from `platejs/react` — the react entrypoint's barrel does not re-export it
// (see markdown-serialization.ts for the same note).
import type { Value } from 'platejs'
import { Plate, PlateContent, usePlateEditor } from 'platejs/react'
import type { DocumentRef } from '@/lib/api/documents'
import { ErrorBoundary } from '@/components/error-boundary'
import { useBufferById } from '@/features/workspace/stores/hooks/use-buffer-store'
import { usePreservedScroll } from '@/features/editor/hooks/use-preserved-scroll'
import { useWorkspaceReadOnly } from '@/features/editor/hooks/use-workspace-read-only'
import { MarkdownAssetContext, type MarkdownAssetInfo } from './markdown-asset'
import { isEditorContent } from '@/features/panes/types/pane-content'
import { documentRefOf, documentUri, refFromUri } from '@/features/editor/documents/document-ref'
import {
  documentText,
  onExternalText,
  registerPendingWriter,
  replaceDocumentText,
} from '@/features/editor/documents/document-text'
import { useDocumentText } from '@/features/editor/documents/use-document-text'
import { LinkWorkspaceContext } from '@/lib/markdown-link'
import { toast } from '@/features/window/stores/toast-store'
import { useMarkdownViewStore } from './markdown-view-store'
import { joinFrontmatter, splitFrontmatter } from './markdown-frontmatter'
import { markdownPlugins } from './markdown-plugins'
import type { MarkdownSource } from './markdown-source'
import { parseMarkdown, plateValueToMarkdown } from './markdown-serialization'
import { MarkdownViewToggle } from './markdown-view-toggle'
// Reading typography for the Plate/Slate DOM, scoped under `.crowbar-markdown-editor`. Not
// `../styles.css`: that one targets the HTML preview's DOM.
import './markdown-editor.css'

// Serializing the whole document per keystroke would stall typing.
const SINK_DELAY_MS = 150

interface MarkdownEditorPaneProps {
  paneId: string
  bufferId: string
}

export function MarkdownEditorPane({ paneId, bufferId }: MarkdownEditorPaneProps) {
  // `paneId` isn't consumed yet — it's the Task-6 integration point (pane-scoped
  // rich/source toggle affordance keyed off the owning pane rather than just the
  // buffer). Accepted now so the call site doesn't need to change later.
  void paneId

  // `null` (not `''`) when the document is not open: the child reads it as "nothing to sync"
  // rather than adopting an empty document, so a vanishing buffer never blanks the screen.
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
    // The toggle lives OUTSIDE the ErrorBoundary, in this outer component, so
    // it stays reachable (and lets the user bail to source view) even if the
    // inner rich editor is mid-crash — it must not depend on the child's state.
    <div className="relative h-full">
      {/* In the reading column's right gutter; z-[60] keeps it above the floating selection
          toolbar (z-40), which comes later in the tree. */}
      <div className="absolute right-2 top-2 z-[60] rounded-md bg-background/80 backdrop-blur-sm">
        <MarkdownViewToggle bufferId={bufferId} />
      </div>
      {/* Parsing runs in the child, since a boundary never catches its own render. Keyed by
          buffer so a `.md` switch cannot write A's text into B, and a crash in one file's
          tab does not poison the next. */}
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
  // Leading YAML frontmatter never reaches Plate (a `---` block parses as a thematic break and
  // setext heading); it is carried verbatim and re-attached in `flush()`. Deserialized once, so a
  // re-parse never fights the cursor: replaced text goes through adopt, a new bufferId remounts.
  const initial = useMemo<{ frontmatter: string; initialValue: Value; source: MarkdownSource }>(
    () => {
      const { frontmatter, body } = splitFrontmatter(content ?? '')
      const { value, source } = parseMarkdown(body)
      return { frontmatter, initialValue: value, source }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps -- initial value only, deliberately not re-derived on content change
    [],
  )

  // `frontmatterRef` is the source of truth for `flush()` (read at flush time,
  // so the callback identity stays stable); `frontmatterText` mirrors it purely
  // so the read-only banner re-renders when a resync brings a new block in.
  const frontmatterRef = useRef(initial.frontmatter)
  const [frontmatterText, setFrontmatterText] = useState(initial.frontmatter)
  // The body text the editor was loaded from: a save copies every block the user did not touch
  // back from it byte for byte, and re-serializes only the edited ones.
  const sourceRef = useRef(initial.source)

  const editor = usePlateEditor({ plugins: markdownPlugins, value: initial.initialValue })

  // The document this buffer holds, so the `html` node can resolve local
  // `<img>` srcs (README logos etc.) beside it. `null` for an unsaved buffer —
  // the node then leaves images at their raw src instead of guessing.
  const assetBuffer = useBufferById(bufferId)
  const readOnly = useWorkspaceReadOnly(
    assetBuffer && isEditorContent(assetBuffer) ? assetBuffer.workspaceId : null,
  )
  const assetUri =
    assetBuffer && isEditorContent(assetBuffer) && !assetBuffer.isVirtual
      ? documentUri(documentRefOf(assetBuffer))
      : null
  const assetInfo = useMemo<MarkdownAssetInfo | null>(() => {
    const doc = assetUri ? refFromUri(assetUri) : null
    return doc ? { doc } : null
  }, [assetUri])
  // A host document's links belong to no workspace; an unsaved buffer's to its own.
  const linkWorkspace =
    assetBuffer && !(isEditorContent(assetBuffer) && assetBuffer.authority === 'host')
      ? assetBuffer.workspaceId
      : null

  // Baseline: what a no-edit flush would write, so a pristine flush (or a selection-only onChange)
  // is a no-op. Not `initialValue`: Plate's init rewrites children (an empty doc gains a U+200B
  // paragraph), which would otherwise count as an edit.
  const baselineRef = useRef<string | undefined>(undefined)
  if (baselineRef.current === undefined) {
    baselineRef.current = joinFrontmatter(
      frontmatterRef.current,
      plateValueToMarkdown(editor.children as Value, sourceRef.current),
    )
  }

  // Trailing-debounce serialize -> document text. `flush` reads the editor's
  // CURRENT value, so a flush run by a reader of the text or by unmount
  // persists whatever is on screen even if no debounce is in flight.
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  // The document text this editor believes it is in sync with: what it last
  // adopted AND what it last wrote. Used by the resync to recognise its own
  // write echoing back (see below).
  const syncedContentRef = useRef<string | null>(content)

  const flush = useCallback(() => {
    if (timerRef.current) {
      clearTimeout(timerRef.current)
      timerRef.current = null
    }
    if (!doc) return // the buffer is gone: nothing to write into
    const md = joinFrontmatter(
      frontmatterRef.current,
      plateValueToMarkdown(editor.children as Value, sourceRef.current),
    )
    if (md === baselineRef.current) {
      // No real edit since mount or the last write: writing would dirty (and with autoSave
      // rewrite) a file the user never touched.
      return
    }
    // Pin the write to THIS pane's document rather than the active buffer —
    // this pane may not be active when a flush lands (tab-switch race).
    replaceDocumentText(doc, md)
    baselineRef.current = md
    // The document is about to hand this exact text back as `content`; record
    // it so the resync recognises the echo instead of re-parsing it (which
    // would reset the caret mid-typing).
    syncedContentRef.current = md
  }, [doc, editor])

  const onChange = useCallback(() => {
    // A selection-only change (click, arrow key) cannot change the serialization, so it arms no
    // debounce. An empty op list says nothing about what happened, so it still arms one.
    const ops = editor.operations
    if (ops.length > 0 && ops.every((op) => op.type === 'set_selection')) return
    if (timerRef.current) clearTimeout(timerRef.current)
    timerRef.current = setTimeout(flush, SINK_DELAY_MS)
  }, [editor, flush])

  // Text replaced under a mounted editor (a split pane on the file, a reload after checkout) is
  // adopted only while this editor is pristine; a dirty one keeps its work. No loop: echoes of our
  // writes match syncedContentRef, and adoption re-derives the baseline so its flush writes nothing.
  const adopt = useCallback(
    (text: string) => {
      if (text === syncedContentRef.current) return
      const current = joinFrontmatter(
        frontmatterRef.current,
        plateValueToMarkdown(editor.children as Value, sourceRef.current),
      )
      if (current !== baselineRef.current) return // unsaved local edits — never clobber

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

  // External text lands at once, before a keystroke can dirty the editor; a
  // second pane's edits arrive through `content`, at most every 150 ms.
  useEffect(() => {
    if (!doc) return
    return onExternalText(doc, () => {
      const text = documentText(doc)
      if (text !== null) adopt(text)
    })
  }, [doc, adopt])

  useEffect(() => {
    if (content !== null) adopt(content)
  }, [content, adopt])

  // A tab switch unmounts this pane, so the reading position lives outside it, as for the markdown
  // preview (Monaco's retained widget restores its own). Ready once `content !== null`: only a
  // deserialized document is tall enough to scroll, and restoring into a short box clamps to 0.
  const scrollRef = useRef<HTMLDivElement>(null)
  usePreservedScroll(scrollRef, bufferId, content !== null)

  // Every reader of the document's text (a save, a view toggle) runs the
  // pending write first, and unmount flushes, so a Cmd+S or a tab switch never
  // races a stale in-memory value.
  useEffect(() => {
    const stop = doc ? registerPendingWriter(doc, flush) : null
    return () => {
      stop?.()
      flush()
    }
  }, [doc, flush])

  return (
    <MarkdownAssetContext.Provider value={assetInfo}>
      <LinkWorkspaceContext value={linkWorkspace}>
        <Plate editor={editor} onChange={onChange}>
          {/* The scope of markdown-editor.css and the scroll host: Plate wraps the editable in
            a non-flex div, so the editable itself can never be bounded to scroll. */}
          <div
            ref={scrollRef}
            className="crowbar-markdown-editor flex h-full flex-col overflow-y-auto"
          >
            {frontmatterText && <MarkdownFrontmatterBanner frontmatter={frontmatterText} />}
            {/* BlockPlaceholderKit hints an empty line in a non-empty document; this prop
              covers the wholly empty file it deliberately skips. */}
            <PlateContent className="outline-none" placeholder="Write…" readOnly={readOnly} />
          </div>
        </Plate>
      </LinkWorkspaceContext>
    </MarkdownAssetContext.Provider>
  )
}

// Frontmatter never enters the Plate document (see markdown-frontmatter.ts) —
// it's rendered here as a plain, non-editable block so the user can still see
// it's present rather than having it silently vanish from the rich view.
function MarkdownFrontmatterBanner({ frontmatter }: { frontmatter: string }) {
  // Trim exactly one trailing newline for tidy display; `frontmatter` itself
  // (used by flush()) is untouched by this — display-only.
  const display = frontmatter.replace(/\r?\n$/, '')
  return (
    // The horizontal padding comes from the stylesheet, not a utility: it is
    // the same centring expression the editable uses, so the block lines up
    // with the reading column instead of running full-bleed.
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

// Rendered by the ErrorBoundary when Plate throws: switches this buffer to the
// Monaco source view and surfaces a toast (spec: fall back to Monaco).
function FallbackToSource({ onMount }: { onMount: () => void }) {
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
