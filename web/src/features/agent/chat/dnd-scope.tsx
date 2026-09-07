import type { ReactNode } from 'react'
import { DndProvider } from 'react-dnd'
import { TouchBackend } from 'react-dnd-touch-backend'
import { createDragDropManager, type DragDropManager } from 'dnd-core'

/**
 * `TouchBackend` (mouse events enabled), not `HTML5Backend` — reordering an
 * attachment did NOTHING on a real desktop drag, confirmed live to be TWO
 * stacked causes, both because this drag lives inside Tauri's WKWebView:
 *
 * 1. The handle sits inside the composer's own `contenteditable="true"`
 *    Slate region (fixed separately, see attachment-drag-handle.tsx's
 *    `contentEditable={false}` wrapper) — WebKit arbitrates a real
 *    mousedown+move there as text selection before a native `dragstart`
 *    can fire at all.
 * 2. Even past that, Tauri's own OS-file-drop interception
 *    (`dragDropEnabled`, on by default — see tauri-file-drop.ts's doc
 *    comment, and desktop/src-tauri/tauri.conf.json, which does not
 *    override it) intercepts a real native drag gesture at the OS/webview
 *    boundary before it ever becomes a DOM `dragstart`/`dragover`/`drop`
 *    event — needed as-is so dragging a real file in from Finder keeps
 *    working (`useTauriFileDrop`). `HTML5Backend` has no way to work around
 *    that: it IS the native Drag and Drop API.
 *
 * `TouchBackend` sidesteps both: it drives dragging entirely off ordinary
 * `mousedown`/`mousemove`/`mouseup` (with `enableMouseEvents: true`), never
 * touching the native Drag and Drop API Tauri's own file-drop handling
 * competes with. The two systems are otherwise unrelated — external OS file
 * drops still go through Tauri's `onDragDropEvent` (tauri-file-drop.ts),
 * completely separate from this in-page reordering.
 *
 * ONE manager, created at most once for the whole page's lifetime — not
 * react-dnd's own implicit global-singleton mechanism (`DndProvider` given
 * just `backend`, no explicit `manager`). That mechanism ref-counts through
 * a `useEffect`, whose cleanup nulls its singleton reference the instant the
 * count dips to 0 — which happens transiently, on every component's FIRST
 * mount, under React 18 StrictMode's dev-only double-invoke (simulated
 * unmount, then remount, no real DOM change). Nothing ever restores that
 * reference afterward (only render-time code repopulates it, and no
 * re-render follows), so the component itself keeps working fine off its
 * own already-closed-over manager while the GLOBAL reference to it is
 * quietly gone. The next `DndProvider` to mount anywhere then finds no
 * singleton and builds a brand new manager + backend — which, for
 * `HTML5Backend`, threw "Cannot have two HTML5 backends at the same time"
 * (confirmed live, and reproduced in isolation by instrumenting
 * `HTML5BackendImpl.setup` — see dnd-scope.test.tsx). A manager built once,
 * outside any component's render/effect lifecycle, sidesteps that
 * bookkeeping entirely regardless of which backend it wraps.
 */
let manager: DragDropManager | undefined
function agentDndManager(): DragDropManager {
  manager ??= createDragDropManager(TouchBackend, undefined, { enableMouseEvents: true })
  return manager
}

/**
 * The one `<DndProvider>` this feature needs, scoped to a single chat view
 * rather than the app root.
 *
 * `@platejs/dnd`'s `useDraggable`/`useDropLine` (attachment-drag-handle.tsx,
 * wired into `ChatCodeBlockElement`/`ChatAttachmentFileCard`) are built on
 * `react-dnd`'s `useDrag`/`useDrop`, which THROW without an ancestor
 * `DndProvider` — and nothing else in this app renders one (the file
 * editor's own `table-node.tsx` `RowDragHandle` has the identical latent
 * gap, left alone; fixing it is out of this feature's scope). `AgentChatView`
 * is the real common ancestor of every Plate tree that can render an
 * attachment node live: the transcript's streaming `MarkdownMessage` and the
 * composer's `ChatMarkdownEditor` (via `AgentComposer` and
 * `AgentEmptyDocument`) — confirmed by reading that file rather than
 * assumed. One provider here covers both, instead of two separate ones
 * duplicated at each leaf.
 *
 * Safe with several chat tabs kept mounted at once (see AgentChatPane's
 * keep-alive `hidden` tabs): every `DndScope` shares the SAME manager
 * (`agentDndManager()` above), so many mounts never means many competing
 * HTML5 backends.
 */
export function DndScope({ children }: { children: ReactNode }) {
  return <DndProvider manager={agentDndManager()}>{children}</DndProvider>
}
