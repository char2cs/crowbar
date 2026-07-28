import { useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
// `Value` resolves from `platejs` (re-exported from `@platejs/slate`), not
// from `platejs/react` — the react entrypoint's barrel does not re-export it
// (see markdown-serialization.ts for the same note).
import type { Value } from 'platejs'
import { Plate, PlateContent, usePlateEditor } from 'platejs/react'
import { ErrorBoundary } from '@/components/error-boundary'
import { useBufferById } from '@/features/workspace/stores/hooks/use-buffer-store'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { useEditorAppStore } from '@/features/editor/stores/editor-app-store'
import { usePreservedScroll } from '@/features/editor/hooks/use-preserved-scroll'
import { MarkdownAssetContext, type MarkdownAssetInfo } from './markdown-asset'
import { hasTextContent } from '@/features/panes/types/pane-content'
import { toast } from '@/features/window/stores/toast-store'
import { useMarkdownViewStore } from './markdown-view-store'
import { joinFrontmatter, splitFrontmatter } from './markdown-frontmatter'
import { markdownPlugins } from './markdown-plugins'
import { markdownToPlateValue, plateValueToMarkdown } from './markdown-serialization'
import { MarkdownViewToggle } from './markdown-view-toggle'
// Reading typography for the Plate/Slate DOM, scoped under
// `.crowbar-markdown-editor` (applied below). Deliberately NOT `../styles.css`
// — that one targets the HTML preview's DOM and still belongs to it; see this
// file's header for why the two cannot be shared.
import './markdown-editor.css'

// Matches Monaco's ContentSink debounce cadence (see code-editor.tsx).
const SINK_DELAY_MS = 150

interface MarkdownEditorPaneProps {
  paneId: string
  bufferId: string
  /** True while this tab is a single-click preview (italic, evictable). */
  isPreview?: boolean
  /** Promote the preview tab to a permanent one. Called on the first real edit. */
  onPromote?: () => void
}

export function MarkdownEditorPane({
  paneId,
  bufferId,
  isPreview,
  onPromote,
}: MarkdownEditorPaneProps) {
  // `paneId` isn't consumed yet — it's the Task-6 integration point (pane-scoped
  // rich/source toggle affordance keyed off the owning pane rather than just the
  // buffer). Accepted now so the call site doesn't need to change later.
  void paneId

  // `null` (rather than `''`) when the buffer is gone or holds no text: the
  // child treats that as "nothing to sync" instead of adopting an empty
  // document, so a buffer disappearing out from under a mounted editor can
  // never blank what's on screen.
  const buffer = useBufferById(bufferId)
  const content = buffer && hasTextContent(buffer) ? buffer.content : null

  const fallbackToSource = useCallback(() => {
    useMarkdownViewStore.getState().setView(bufferId, 'source')
    toast.error('Rich markdown editor failed', 'Switched to source view for this file.')
  }, [bufferId])

  return (
    // The toggle lives OUTSIDE the ErrorBoundary, in this outer component, so
    // it stays reachable (and lets the user bail to source view) even if the
    // inner rich editor is mid-crash — it must not depend on the child's state.
    <div className="relative h-full">
      {/* Sits in the reading column's right gutter. It clears the text
          vertically too — the surface's 3rem top padding starts the first
          line below it — and the wash keeps it legible over content that
          scrolls underneath. z-[60]: strictly above the floating selection
          toolbar (z-40, floating-toolbar.tsx), which comes after this div in
          the tree, so it needs real headroom, not just a matching z-index. */}
      <div className="absolute right-2 top-2 z-[60] rounded-md bg-background/80 backdrop-blur-sm">
        <MarkdownViewToggle bufferId={bufferId} />
      </div>
      {/* Deserialization and editor construction happen in the CHILD, not here —
          a React error boundary never catches errors thrown by the component that
          renders it, so `markdownToPlateValue`/`usePlateEditor` must run below this
          boundary (inside `MarkdownRichEditor`) for a deserialize/construction
          crash to actually hit the fallback instead of bypassing it.

          Keyed by `bufferId` (C1): the pane tree renders the active buffer
          WITHOUT a key, so switching between two `.md` tabs arrives here as a
          prop update. The Plate editor and its parsed document are memoized for
          the lifetime of the mount, so without this key the previous file's
          document would stay on screen while `bufferId` — and therefore the
          flush target — moved on, writing file A's text into file B. Keying the
          boundary itself also clears a crash from the previous buffer, so a
          file that broke Plate doesn't poison its neighbour's tab. */}
      <ErrorBoundary key={bufferId} fallback={<FallbackToSource onMount={fallbackToSource} />}>
        <MarkdownRichEditor
          bufferId={bufferId}
          content={content}
          isPreview={isPreview}
          onPromote={onPromote}
        />
      </ErrorBoundary>
    </div>
  )
}

function MarkdownRichEditor({
  bufferId,
  content,
  isPreview,
  onPromote,
}: {
  bufferId: string
  content: string | null
  isPreview?: boolean
  onPromote?: () => void
}) {
  const { handleContentChange } = useEditorAppStore.use.actions()

  // Split off any leading YAML frontmatter BEFORE it ever reaches Plate.
  // Markdown parses a leading `---` block as a thematic break + setext
  // heading, so feeding it whole into Plate and serializing back out
  // destroys the frontmatter (see the Task 8 spec). Only `body` is
  // deserialized; `frontmatter` is carried through untouched and re-attached
  // verbatim in `flush()`.
  //
  // Deserialize ONCE for the initial value: re-parsing on every render would
  // fight the user's cursor. A buffer whose content is REPLACED underneath a
  // mounted editor is handled by the resync effect below, and a change of
  // `bufferId` remounts this component (see the key in the parent).
  const initial = useMemo<{ frontmatter: string; initialValue: Value }>(
    () => {
      const { frontmatter, body } = splitFrontmatter(content ?? '')
      return { frontmatter, initialValue: markdownToPlateValue(body) }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps -- initial value only, deliberately not re-derived on content change
    [],
  )

  // `frontmatterRef` is the source of truth for `flush()` (read at flush time,
  // so the callback identity stays stable); `frontmatterText` mirrors it purely
  // so the read-only banner re-renders when a resync brings a new block in.
  const frontmatterRef = useRef(initial.frontmatter)
  const [frontmatterText, setFrontmatterText] = useState(initial.frontmatter)

  const editor = usePlateEditor({ plugins: markdownPlugins, value: initial.initialValue })

  // Where this file lives, so the `html` node can resolve local `<img>` srcs
  // (README logos etc.) against the file's folder. `null` when unknown — the
  // node then leaves images at their raw src instead of guessing.
  const assetBuffer = useBufferById(bufferId)
  // Raw context (not `useWorkspaceStoreContext`, which throws without a
  // provider): a standalone render — e.g. a unit test of this component — has
  // no workspace, and that should just mean "no local-image resolution", not a
  // crash. `workspaceId` is stable for the store's life, so reading it off the
  // store snapshot (non-reactively) is fine.
  const workspaceStore = useContext(WorkspaceStoreContext)
  const assetInfo = useMemo<MarkdownAssetInfo | null>(() => {
    const wsId = workspaceStore?.getState().workspaceId
    const path = assetBuffer && 'path' in assetBuffer ? assetBuffer.path : undefined
    if (!wsId || !path) return null
    const fileDir = path.includes('/') ? path.slice(0, path.lastIndexOf('/')) : ''
    return { wsId, fileDir }
  }, [workspaceStore, assetBuffer])

  // Baseline = the canonical FULL-DOCUMENT serialization of the document the
  // editor ACTUALLY holds (frontmatter + body) — i.e. exactly what a no-edit
  // flush would produce. Deliberately NOT the buffer's raw `content`:
  // `plateValueToMarkdown(markdownToPlateValue(md))` is not byte-stable for
  // every construct (GFM tables re-canonicalize `| --- |` to `| - |`), so
  // comparing against the original bytes would report a "change" on every
  // pristine open. Comparing against this baseline instead makes a pristine
  // flush a true no-op, and is immune to the selection/cursor-only `onChange`
  // events Slate/Plate fire on click or arrow-key movement (those don't touch
  // `editor.children`, so they can never make `md` differ from the baseline).
  //
  // Read from `editor.children`, NOT from the parsed `initialValue` (C2):
  // `withSlate` initializes children synchronously and Plate's `init` rewrites
  // them — an empty document becomes a synthesized paragraph, and
  // `transformInitialValue` hooks may rewrite others. A baseline measured from
  // the pre-init value would therefore differ from what the editor holds, and
  // the first flush would write that difference into a file the user only
  // opened (for an empty file: an invisible U+200B).
  const baselineRef = useRef<string | undefined>(undefined)
  if (baselineRef.current === undefined) {
    baselineRef.current = joinFrontmatter(
      frontmatterRef.current,
      plateValueToMarkdown(editor.children as Value),
    )
  }

  // Trailing-debounce serialize -> buffer store, mirroring Monaco's ContentSink.
  // `flush` always reads the editor's CURRENT value (not just a value captured
  // at the last onChange) so a flush triggered by the window event or unmount
  // persists whatever is on screen even if no debounce is in flight — the same
  // contract Cmd+S relies on for Monaco.
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  // The buffer content this editor believes it is in sync with: what it last
  // adopted from the store AND what it last wrote. Used by the resync effect to
  // recognise its own write echoing back (see below).
  const syncedContentRef = useRef<string | null>(content)

  // Preview promotion mirrors Monaco (editor-surface's `writeContent`): the
  // first edit that actually persists promotes the tab. Held in refs so
  // `flush`'s identity — and therefore the debounce and the window listener —
  // doesn't churn on every preview/promote prop change.
  const isPreviewRef = useRef(isPreview)
  isPreviewRef.current = isPreview
  const onPromoteRef = useRef(onPromote)
  onPromoteRef.current = onPromote

  const flush = useCallback(() => {
    if (timerRef.current) {
      clearTimeout(timerRef.current)
      timerRef.current = null
    }
    const md = joinFrontmatter(
      frontmatterRef.current,
      plateValueToMarkdown(editor.children as Value),
    )
    if (md === baselineRef.current) {
      // No real edit since mount (or since the last write): writing here would
      // silently dirty — and with autoSave on, rewrite to disk — a file the
      // user never touched. See the baseline comment above for why we can't
      // compare against the buffer's raw content instead.
      return
    }
    if (isPreviewRef.current) onPromoteRef.current?.()
    // Pin the write to THIS pane's buffer rather than the active buffer — this
    // pane may not be active when a flush lands (tab-switch race). Skip Plate's
    // own undo grouping so it doesn't double-write Monaco's undo stack; Plate
    // holds the text (not the buffer), so handleContentChange must apply it.
    void handleContentChange(md, undefined, undefined, undefined, {
      targetBufferId: bufferId,
      skipUndoGrouping: true,
    })
    baselineRef.current = md
    // The store is about to hand this exact text back as `content`; record it
    // so the resync effect recognises the echo instead of re-parsing it (which
    // would reset the caret mid-typing).
    syncedContentRef.current = md
  }, [bufferId, editor, handleContentChange])

  const onChange = useCallback(() => {
    // Selection-only events (a click, an arrow key) can never change the
    // serialized document, so arming the debounce for them is pure waste — it
    // re-serializes the whole document 150 ms later just to compare equal.
    // Guarded on a NON-EMPTY operation list: an empty list means we can't tell
    // what happened, and skipping a real edit would lose data.
    const ops = editor.operations
    if (ops.length > 0 && ops.every((op) => op.type === 'set_selection')) return
    if (timerRef.current) clearTimeout(timerRef.current)
    timerRef.current = setTimeout(flush, SINK_DELAY_MS)
  }, [editor, flush])

  // I1/I2: the buffer's content can be replaced underneath a MOUNTED editor —
  // a second pane split on the SAME file flushing its own edit, or
  // external-buffer-sync reloading the file after a git checkout. Plate holds a
  // parsed copy, so without this the editor keeps showing (and, on the next
  // keystroke, writes back) the stale document over the new content.
  //
  // Adoption is gated on THIS editor being pristine — its current serialization
  // still equals its baseline, i.e. it holds no unsaved local edits. A dirty
  // editor is left completely alone: in-progress work is never destroyed, and
  // the user is already warned about the divergence by external-buffer-sync's
  // hasExternalChange toast.
  //
  // It cannot loop. `syncedContentRef` holds both what was last adopted and
  // what was last written, so a write echoing back is skipped outright. And the
  // adoption itself re-derives the baseline from the resulting `editor.children`,
  // so the `onChange` that `setValue` fires ends in a flush that compares equal
  // and writes nothing.
  useEffect(() => {
    if (content === null || content === syncedContentRef.current) return
    const current = joinFrontmatter(
      frontmatterRef.current,
      plateValueToMarkdown(editor.children as Value),
    )
    if (current !== baselineRef.current) return // unsaved local edits — never clobber

    syncedContentRef.current = content
    const { frontmatter, body } = splitFrontmatter(content)
    frontmatterRef.current = frontmatter
    setFrontmatterText(frontmatter)
    editor.tf.setValue(markdownToPlateValue(body))
    baselineRef.current = joinFrontmatter(
      frontmatter,
      plateValueToMarkdown(editor.children as Value),
    )
  }, [content, editor])

  // A tab switch UNMOUNTS this pane — PaneContainer renders only the active
  // buffer, and EditorPane keys the rich editor by buffer id — so the reading
  // position has to live outside the component or it dies with the DOM node and
  // the file reopens at the top. Same retention the markdown PREVIEW uses;
  // Monaco needs none of it (its retained per-pane widget restores a per-model
  // view state).
  //
  // Readiness is `content !== null`, because the document is only tall enough to
  // scroll once it has been deserialized, and restoring into a short box clamps
  // to 0. `initial` is derived from `content` at FIRST render, so on the way
  // back to an already-open tab the text is in the DOM on the first commit and
  // the restore lands immediately. The one case where content arrives later is
  // a file being opened for the first time — which has no stored offset anyway.
  const scrollRef = useRef<HTMLDivElement>(null)
  usePreservedScroll(scrollRef, bufferId, content !== null)

  // Flush on the app-wide flush event (save / switch-away) and on unmount, so a
  // Cmd+S or a tab switch never races a stale in-memory value.
  useEffect(() => {
    const onFlush = () => flush()
    window.addEventListener('flush-editor-content', onFlush)
    return () => {
      window.removeEventListener('flush-editor-content', onFlush)
      flush()
    }
  }, [flush])

  return (
    <MarkdownAssetContext.Provider value={assetInfo}>
      <Plate editor={editor} onChange={onChange}>
        {/* `crowbar-markdown-editor` is the scope for markdown-editor.css: the
            reading column, the block rhythm and every node's typography hang
            off it. It is ALSO the scroll container. The editable can't be:
            Plate wraps `PlateContent` in a plain (non-flex) `w-full` div, so an
            `overflow-auto`/`flex-1` on the editable never bounds it — with no
            flex parent it grows to full content height and there's nothing to
            scroll. This bounded `h-full` ancestor is the reliable scroll host;
            the editable keeps its own padding, so the reading column still
            centres and the frontmatter banner scrolls with the content. */}
        <div
          ref={scrollRef}
          className="crowbar-markdown-editor flex h-full flex-col overflow-y-auto"
        >
          {frontmatterText && <MarkdownFrontmatterBanner frontmatter={frontmatterText} />}
          {/* Two placeholder mechanisms, deliberately non-overlapping:
              BlockPlaceholderKit (markdown-plugins.ts) hints the FOCUSED empty
              line once the document already has other content — but its own
              `isPristineEmptyEditor` check suppresses it on a brand-new,
              wholly-empty file, which is exactly what Slate's native
              `placeholder` prop here covers instead. */}
          <PlateContent className="outline-none" placeholder="Write…" />
        </div>
      </Plate>
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
