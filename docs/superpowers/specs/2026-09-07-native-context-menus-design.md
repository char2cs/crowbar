# Native context menus — design

## Problem

Every right-click menu in the web UI is a React-rendered popup (`ImperativeContextMenu` /
`useContextMenu` in `web/src/components/ui/context-menu.tsx`, plus a Plate-specific one for the
editor). That's the workaround browser apps need because a browser has no OS menu affordance to
call into. This app ships inside Tauri, which does have that affordance — so the fake menus are a
maintenance liability with no remaining justification. Replace every live context menu with the
OS's real menu via Tauri's menu API, and remove the React-rendered ones.

## Scope

Five live menu surfaces get migrated:

1. `web/src/features/tabs/components/tab-context-menu.tsx` — tab bar (pin, rename terminal, split,
   copy/reveal path, reload, close variants)
2. `web/src/features/agent/tree/agent-chat-context-menu.tsx` — chats tree (new thread, group into
   folder, remove)
3. `web/src/components/layout/row-context-menu.tsx` — workspace/sidebar tree (group, lock/unlock,
   remove)
4. `web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx` — file
   explorer (new file/folder, upload, refresh, open-all, collapse-all, open-in-terminal,
   generate-image, workspace-folder add/remove, open, copy content, env-template group, properties,
   copy/copy-relative path, reveal, copy/cut, duplicate, rename, delete)
5. `web/src/components/ui/block-context-menu.tsx` — Plate editor per-block menu (delete, duplicate,
   turn-into submenu, indent/outdent, align submenu)

Explicitly out of scope, because there is nothing live to migrate:

- The declarative `ContextMenuRoot` / `ContextMenuTrigger` / `ContextMenuContent` / ... compound
  export in `context-menu.tsx`. Zero real consumers exist today (the only hit outside the file
  itself is a code comment in `block-context-menu.tsx` explaining why it deliberately does *not*
  use it). Delete it rather than port it.
- `GitFileItem`'s `onContextMenu` prop (`web/src/features/git/components/status/git-status-file-item.tsx`).
  Its only consumer, `changed-files-tree.tsx`, never wires it up — no menu exists there today, so
  there's nothing to migrate. Not building a new one as part of this work.

## Architecture

Build native menus entirely in JS via `@tauri-apps/api/menu` (already a dependency,
`^2.11.1`). No new Rust/Tauri command: `Menu.new()` / `Submenu.new()` / `MenuItem.new()` /
`CheckMenuItem.new()` / `PredefinedMenuItem.new({ item: 'Separator' })`, then
`menu.popup({ x, y })`. Each item's `action: (id) => void` callback fires on click — that's the
same shape as today's `ContextMenuItem.onClick`, so the item-model builder functions in each
feature file (`chatMenuFor`, `rowMenuFor`, the file-explorer `contextMenuItems` memo, the tab
menu's inline array, the Plate menu's per-item handlers) don't need to change — only the last step,
building `Menu` nodes and calling `.popup()` instead of rendering `<ContextMenu items={...} />`,
moves into a new helper.

Desktop-side change: add the `core:menu:default` permission to
`desktop/src-tauri/capabilities/default.json`. No other Rust changes.

### Item model → native mapping

| Today (`ContextMenuItem`) | Native |
|---|---|
| `label` | `MenuItem`/`Submenu` `text` |
| `onClick` | `action` callback |
| `separator: true` | `PredefinedMenuItem.new({ item: 'Separator' })` |
| `disabled` | `enabled: !disabled` |
| `shortcut` / `keybinding` | `accelerator` |
| `items` (submenu) | `Submenu.new({ text, items })` |
| `icon` | **dropped** — native `MenuItem` carries no icon; only `IconMenuItem` does, and it wants an image asset, not a React node. Converting ~20 inline Phosphor/Lucide glyphs to asset icons is not worth it for parity most native context menus (Finder, VS Code's OS-native menus) don't bother with either. Menus become label + accelerator only. |
| `closeOnClick` | dropped — native menus always close on activation; nothing in the current call sites relies on staying open after a click. |
| `className` (e.g. `text-destructive` on Delete/Remove) | dropped — no native per-item styling hook. Destructive items keep their label text as the only signal, same as a native app's own destructive menu items. |

### New helper

Replace `useContextMenu` / `ImperativeContextMenu` (still exists — see Fallback) with a function in
`web/src/components/ui/context-menu.tsx`:

```ts
async function showNativeContextMenu(items: ContextMenuItem[], position: { x: number; y: number }): Promise<void>
```

It walks the `ContextMenuItem[]` tree, builds the native `Menu`, and pops it at `position`. Call
sites that currently do `openAt(pos, data)` then render `<ContextMenu items={...} />` instead do
`openAt(pos, data)` then call `showNativeContextMenu(items, pos)` — the `useContextMenu` hook's
`isOpen`/`position`/`data`/`open`/`openAt`/`close` surface is unchanged, since it's still needed to
track "what row is this menu for" between the native call firing and its `action` callbacks running.

### Fallback for non-Tauri dev

`web/package.json`'s `dev` script runs plain `vite` — no Tauri shell, so `@tauri-apps/api/menu`
calls have nothing to talk to. The codebase already has a live pattern for this
(`isTauri()` from `@/lib/crowbar-bridge`, used by `icon-popover.tsx`, `tauri-file-drop.ts`,
`daemon-health-listener.tsx`, etc.): branch on it and take a genuinely different path, not a shim.

`showNativeContextMenu` becomes:

```ts
async function showContextMenu(items: ContextMenuItem[], position, onClose: () => void): Promise<void> {
  if (!isTauri()) {
    // fall back to today's rendered popup — ImperativeContextMenu / useContextMenu are kept
    // for exactly this path, not deleted.
    return
  }
  // native path
}
```

Concretely: `ImperativeContextMenu` stays in `context-menu.tsx` as-is. Each call site keeps
rendering it conditionally (`{!isTauri() && menu.isOpen && <ContextMenu ... />}`) alongside calling
the native path when `isTauri()` is true. `useContextMenu`'s `open`/`openAt` become the single entry
point that decides which of the two happens.

## Per-surface notes

**Tabs, agent-chats, workspace rows, file-explorer** — mechanical. Each already builds a flat
`ContextMenuItem[]` (with separators and, for the file explorer, no submenus currently) purely from
state; only the render-vs-popup call changes.

**Plate block menu** (`block-context-menu.tsx`) is structurally different and needs its own path:

- Trigger is Plate's own `BlockMenuPlugin.api.blockMenu.show(id, {x, y})`, invoked from a
  `ContextMenuPrimitive.Trigger`'s `onContextMenu`, not a raw DOM listener like the other four.
- `isTouch` bypass stays unchanged — no menu (native or otherwise) on touch devices.
- "Turn into" and "Align" become native `Submenu`s built from the same `handleTurnInto` /
  `handleAlign` callbacks that exist today.
- Plate tracks its own open/closed state via `openId` / `usePluginOption`. Today, `onOpenChange`
  from the Radix root fires `api.blockMenu.hide()` when the popup closes. With a native menu there's
  no Radix root to fire that — call `api.blockMenu.hide()` explicitly after `popup()`'s promise
  resolves (it resolves when the native menu closes, whether an item was clicked or it was
  dismissed), so Plate's `openId` state doesn't stay stuck open.
- `onCloseAutoFocus` currently refocuses the block selection after the Radix content unmounts;
  with a native menu, do the equivalent focus restore after `popup()` resolves.

## Testing

RTL tests that assert on the rendered popup DOM (`context-menu.test.tsx`, `tab-bar-item.test.tsx`,
`agent-chat-context-menu.test.tsx`, `agent-chats-panel.test.tsx`, `agent-chats-panel-perf.test.tsx`,
`use-file-explorer-context-menu.test.tsx`) can't survive as-is — a native menu isn't in the element
tree to query. New coverage:

- Item-model builder functions (`chatMenuFor`, `rowMenuFor`, the file-explorer `contextMenuItems`
  memo, the tab menu's item array, the Plate menu's `handleTurnInto`/`handleAlign`) get tested
  directly for the right items/labels/disabled-state given input state — most of this already
  works this way and doesn't change.
- A thin integration layer mocks `@tauri-apps/api/menu` and asserts `showNativeContextMenu` builds
  the right item tree (right labels, right nesting for submenus, separators in the right spots) and
  calls `.popup()` at the right position, and that invoking a constructed item's `action` runs the
  right handler.
- The `!isTauri()` fallback path keeps using the existing RTL-against-`ImperativeContextMenu` style
  of test, since that code path is unchanged.

Manual verification of the native path happens in `make dev-desktop` (Tauri desktop build) per
existing project practice — headless/browser testing exercises the fallback path, not the native
one, so it proves nothing about the real menu.

## Non-goals

- No Rust-side menu construction or new IPC command.
- No per-item icons on native menus.
- No attempt to keep `closeOnClick: false` or per-item `className` styling working natively.
- No new context menu on git status rows — that prop is unwired today and stays unwired.
