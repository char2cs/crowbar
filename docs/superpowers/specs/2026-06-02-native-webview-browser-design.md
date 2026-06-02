# Native Webview Browser Pane — Design Spec

**Date:** 2026-06-02  
**Branch:** enhancement/design-language  
**Status:** Approved

## Problem

The built-in browser uses an `<iframe>` inside the Tauri webview. Sites that set
`X-Frame-Options: SAMEORIGIN` (e.g. Google) refuse to load. This is a fundamental
limitation of the iframe approach — the Tauri webview enforces the header just like
a real browser.

## Solution

Replace the iframe with a native Tauri child webview per browser buffer. Native
webviews are not subject to `X-Frame-Options` because they are not embedded frames —
they are independent OS-level webview instances positioned to overlay the React pane.

## Architecture

### Layer map

| Layer | File | Role |
|---|---|---|
| Bridge | `web/src/lib/crowbar-bridge.ts` | Platform-agnostic interface; Tauri `invoke` calls live here |
| Hook | `web/src/features/web-viewer/hooks/use-browser-pane-anchor.ts` | ResizeObserver → bridge sync |
| Component | `web/src/features/web-viewer/components/web-viewer.tsx` | Nav bar + anchor div; no Tauri imports |
| Store | `web/src/features/web-viewer/stores/web-viewer-navigation-store.ts` | Per-buffer URL + canGoBack/canGoForward |
| Event listener | `web/src/features/web-viewer/components/browser-pane-event-listener.tsx` | Mounted once at app root; bridges Tauri events → store |
| Rust | `desktop/src-tauri/src/browser_pane.rs` | Child webview lifecycle + Tauri commands |
| Rust | `desktop/src-tauri/src/lib.rs` | Module registration + command registration |

**Layering rule:** `web-viewer.tsx` and the hook never import `@tauri-apps/*`. All
Tauri coupling lives in `crowbar-bridge.ts` and the Rust module.

## Bridge Interface

Six functions added to `crowbar-bridge.ts`:

```ts
// Create or reposition a child webview for this buffer.
// visible=false hides without destroying (preserves session state).
browserPaneSync(
  bufferId: string,
  rect: { x: number; y: number; width: number; height: number },
  visible: boolean,
): Promise<void>

browserPaneNavigate(bufferId: string, url: string): Promise<void>
browserPaneGoBack(bufferId: string): Promise<void>
browserPaneGoForward(bufferId: string): Promise<void>
browserPaneReload(bufferId: string): Promise<void>
browserPaneClose(bufferId: string): Promise<void>
```

Non-Tauri stubs: all return `Promise.resolve()`.

Detection: `isTauri()` helper checks `window.__TAURI_INTERNALS__` (set by Tauri at
runtime). The hook exposes an `isTauri` boolean read once at mount; the component
uses this to render "requires desktop app" instead of the anchor div.

## React Changes

### `use-browser-pane-anchor.ts`

- Accepts `bufferId`, `isVisible`, and `anchorRef: RefObject<HTMLDivElement>`.
- Attaches a `ResizeObserver` to the anchor div and also listens to `window` resize.
- On any change: reads `anchorRef.current.getBoundingClientRect()`, calls
  `browserPaneSync(bufferId, rect, isVisible)`.
- Debounces to ~16ms (one animation frame) to avoid flooding Rust during pane resize drags.
- On unmount: calls `browserPaneClose(bufferId)`.
- Returns `isTauri: boolean` (checked once at mount via `window.__TAURI_INTERNALS__`)
  so the component can branch on non-Tauri.

### `web-viewer.tsx`

- Drops the `<iframe>` entirely.
- Renders nav bar (reload, address bar) — unchanged in appearance.
- If `isTauri === false`: renders a centered "This feature requires the desktop app" fallback.
- If `isTauri === true`: renders `<div ref={anchorRef} className="min-h-0 flex-1" />`.
- Address bar value reads from the navigation store (`url` for this `bufferId`) so
  that link clicks inside the native webview update the bar.
- Reload / back / forward buttons call bridge functions directly.
- `isVisible` prop drives the `visible` flag passed to the hook.

### `web-viewer-navigation-store.ts`

Per-buffer state shape:

```ts
interface NavEntry {
  url: string
  canGoBack: boolean
  canGoForward: boolean
}
// store: Map<bufferId, NavEntry>
```

Updated by `BrowserPaneEventListener` when events arrive from Rust.

### `browser-pane-event-listener.tsx`

- Mounted once at the app root (alongside other global listeners).
- In a `useEffect`, calls `tauri.event.listen('browser-pane-navigated', handler)`.
- `handler` writes `{ url, canGoBack, canGoForward }` into the navigation store
  keyed by `bufferId`.
- Returns the unlisten function as the `useEffect` cleanup.

## Rust Side

### `BrowserPaneManager` (Tauri state)

```rust
struct BrowserPaneManager {
    panes: Mutex<HashMap<String, Webview>>,
}
```

Registered via `.manage(BrowserPaneManager::new())` in `lib.rs`.

### Commands

| Command | Behaviour |
|---|---|
| `browser_pane_sync(buffer_id, x, y, width, height, visible)` | If no entry for `buffer_id`: create a child `Webview` (`decorations: false`, `transparent: true`) at the given rect. If exists: move + resize. Then show or hide. |
| `browser_pane_navigate(buffer_id, url)` | Navigate the child webview to the URL |
| `browser_pane_go_back(buffer_id)` | `webview.eval("history.back()")` |
| `browser_pane_go_forward(buffer_id)` | `webview.eval("history.forward()")` |
| `browser_pane_reload(buffer_id)` | `webview.eval("location.reload()")` |
| `browser_pane_close(buffer_id)` | Close the webview and remove from map |

### Navigation events → React

Each child webview has a small JS snippet injected on creation that:
1. Listens for `popstate` + `hashchange` events.
2. Uses a `MutationObserver` on `document.title` as a proxy for page navigation
   completing (URL bar update timing).
3. On any navigation: calls back to Rust (via a registered IPC handler on the child
   webview) with `{ bufferId, url, canGoBack, canGoForward }`.
4. Rust re-emits this as a `browser-pane-navigated` event to the `main` window.

`canGoBack` / `canGoForward` are read from `window.history.length` and a
`navigationIndex` counter maintained by the injected script.

## Event Flow Summary

```
User types URL → address bar submit
  → browserPaneNavigate(bufferId, url) [bridge]
  → invoke('browser_pane_navigate') [Tauri IPC]
  → child webview navigates

User clicks link in child webview
  → injected JS fires
  → Rust receives navigation callback
  → emit_to('main', 'browser-pane-navigated', { bufferId, url, canGoBack, canGoForward })
  → BrowserPaneEventListener receives event
  → navigation store updated
  → address bar re-renders with new URL
  → tab bar back/forward buttons enable/disable

Pane resized (splitter drag)
  → ResizeObserver fires on anchor div
  → debounced ~16ms
  → browserPaneSync(bufferId, newRect, visible) [bridge]
  → invoke('browser_pane_sync') [Tauri IPC]
  → Rust moves/resizes child webview

Tab switched to different buffer
  → isVisible=false passed to WebViewerPane
  → hook calls browserPaneSync(..., visible=false)
  → Rust hides the child webview (session preserved)

Buffer closed
  → WebViewerPane unmounts
  → hook cleanup calls browserPaneClose(bufferId)
  → Rust closes and removes child webview
```

## Non-Tauri Fallback

When `window.__TAURI_INTERNALS__` is absent (browser dev mode):
- All bridge functions return stubs immediately.
- `browserPaneSync` returns `{ tauri: false }`.
- `WebViewer` renders: "This feature requires the desktop app."
- No iframe fallback — the design decision is to be explicit rather than silently
  degraded.

## Out of Scope

- Zoom level (`zoomLevel` field exists in buffer state but is not wired up here)
- Per-profile cookies/storage isolation (`profileKey` field — deferred)
- Download handling in the child webview
- Find-in-page
