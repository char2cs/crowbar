# Athas UI Integration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Integrate Athas's IDE feature modules (Monaco editor, file explorer, terminal, git panel, split panes, settings) into Crowbar's web frontend, producing a 100% production-ready IDE shell wired to mock data.

**Architecture:** Athas feature modules are copied wholesale into `web/src/features/`. All Tauri `invoke()` calls are replaced with Crowbar-native patterns — reads via TanStack Query with mock `queryFn`, writes/side-effects via `crowbar-bridge.ts` no-op stubs. Four new glue components compose the final layout. Two routes are minimally updated.

**Tech Stack:** React 19, TypeScript strict, Vite, TanStack Router/Query, Zustand 5 + immer, Monaco Editor, @xterm/xterm, @dnd-kit, shadcn/ui, Tailwind CSS v4, Vitest

---

## File Map

### New directories (copied from Athas, path `/Users/char2cs/Projects/Cloned/athas/src/features/`)
- `web/src/features/panes/` — split pane tree + pane store
- `web/src/features/tabs/` — tab bar + buffer store
- `web/src/features/editor/` — Monaco editor + editor store (LSP stubbed)
- `web/src/features/athas-editor/` — custom editor rendering layers
- `web/src/features/file-explorer/` — virtualized file tree + explorer store
- `web/src/features/terminal/` — xterm.js terminal + terminal store
- `web/src/features/git/` — git panel + git store
- `web/src/features/settings/` — settings modal (trimmed to 7 panels)
- `web/src/features/command-palette/` — quick-open palette
- `web/src/features/layout/` — MainSidebar, BottomPane, Footer (sidebar/ + bottom-pane/ + footer/ subdirs)

### New Athas utils (copied from Athas `src/utils/`)
- `web/src/utils/zustand-selectors.ts`
- `web/src/utils/cn.ts`
- `web/src/utils/path-helpers.ts`
- `web/src/utils/platform.ts`
- `web/src/utils/date.ts`
- `web/src/utils/search-match.ts`
- `web/src/utils/fuzzy-matcher.tsx`
- `web/src/utils/stream-utils.ts`
- `web/src/utils/storage.ts`
- `web/src/utils/keybinding-display.ts`
- `web/src/utils/keyboard/` (directory)
- `web/src/utils/frecency.ts`

### New custom UI components (copied from Athas `src/ui/`)
- `web/src/components/ui/pane.tsx`
- `web/src/components/ui/sidebar-tree.tsx`
- `web/src/components/ui/item.tsx`
- `web/src/components/ui/primitive-dialog-service.tsx`

### New Crowbar files (created from scratch)
- `web/src/lib/crowbar-bridge.ts` — system operation stubs
- `web/src/lib/mock/files.ts` — mock file tree data
- `web/src/lib/mock/git-data.ts` — mock git status, branches, commits
- `web/src/lib/mock/terminal.ts` — mock terminal session seed
- `web/src/components/layout/IDEShell.tsx` — root layout
- `web/src/components/layout/SidebarTabs.tsx` — sidebar tab strip
- `web/src/components/layout/WorkspacesSidebarPanel.tsx` — extracted Sidebar internals
- `web/src/components/layout/FlowTab.tsx` — locked flow tab wrapper

### Modified files
- `web/package.json` — add 15 npm packages
- `web/src/lib/queries.ts` — add 4 query options
- `web/src/routes/__root.tsx` — swap AppShell → IDEShell
- `web/src/routes/workspaces/$wsId.tsx` — remove WorkspaceStepTabs wrapper
- `web/src/components/layout/SidebarHeader.tsx` — add settings trigger button
- `web/src/features/file-explorer/stores/file-explorer-clipboard-store.ts` — replace invoke() with crowbar-bridge
- `web/src/features/terminal/hooks/use-terminal-connection.ts` — replace invoke() + listen() with crowbar-bridge
- `web/src/features/terminal/components/terminal-container.tsx` — replace invoke() with crowbar-bridge
- `web/src/features/editor/stores/buffer-store.ts` — replace close_terminal invoke() with crowbar-bridge
- `web/src/features/settings/lib/settings-effects.ts` — replace window invoke() with crowbar-bridge
- `web/src/features/settings/components/tabs/appearance-settings.tsx` — replace toggle_menu_bar invoke()
- `web/src/features/command-palette/constants/view-actions.tsx` — replace toggle_menu_bar invoke()

### Deleted files
- `web/src/components/layout/AppShell.tsx` — replaced by IDEShell
- `web/src/components/layout/Sidebar.tsx` — replaced by WorkspacesSidebarPanel
- `web/src/hooks/useSidebarWidth.ts` — replaced by Athas sidebar resize

### Tests (new)
- `web/src/__tests__/components/layout/IDEShell.test.tsx`
- `web/src/__tests__/components/layout/SidebarTabs.test.tsx`
- `web/src/__tests__/components/layout/WorkspacesSidebarPanel.test.tsx`
- `web/src/__tests__/components/layout/FlowTab.test.tsx`
- `web/src/__tests__/lib/mock/files.test.ts`
- `web/src/__tests__/lib/mock/git-data.test.ts`
- `web/src/__tests__/lib/mock/terminal.test.ts`
- `web/src/__tests__/lib/crowbar-bridge.test.ts`

---

## Wave 1 — Foundation

### Task 1: Add npm dependencies

**Files:**
- Modify: `web/package.json`

- [ ] **Step 1: Add packages**

In `web/`, run:
```bash
cd web && npm install \
  monaco-editor@^0.55.1 \
  @xterm/xterm@^5.5.0 \
  "@xterm/addon-fit@^0.10.0" \
  "@xterm/addon-web-links@^0.11.0" \
  "@xterm/addon-search@^0.15.0" \
  "@xterm/addon-clipboard@^0.1.0" \
  "@xterm/addon-webgl@^0.18.0" \
  "@xterm/addon-unicode11@^0.8.0" \
  "@tanstack/react-virtual@^3.13.12" \
  "@dnd-kit/core@^6.3.1" \
  "@dnd-kit/sortable@^10.0.0" \
  "@dnd-kit/modifiers@^9.0.0" \
  "@dnd-kit/utilities@^3.2.2" \
  "material-file-icons@^2.4.0" \
  "immer@^10.2.0" \
  "@phosphor-icons/react@^2.1.10" \
  "framer-motion@^12.34.3"
```

- [ ] **Step 2: Verify install succeeded**
```bash
cd web && node -e "require('./node_modules/monaco-editor/package.json'); require('./node_modules/@xterm/xterm/package.json'); console.log('OK')"
```
Expected: `OK`

- [ ] **Step 3: Commit**
```bash
git add web/package.json web/package-lock.json
git commit -m "chore(web): add Athas IDE dependencies"
```

---

### Task 2: Add missing shadcn/ui components

**Files:**
- Modify: `web/src/components/ui/` (new files generated)

- [ ] **Step 1: Add components**
```bash
cd web && npx shadcn@latest add context-menu resizable sonner switch checkbox label slider --yes
```

- [ ] **Step 2: Verify generated**
```bash
ls web/src/components/ui/context-menu.tsx web/src/components/ui/resizable.tsx web/src/components/ui/sonner.tsx web/src/components/ui/switch.tsx web/src/components/ui/checkbox.tsx web/src/components/ui/label.tsx web/src/components/ui/slider.tsx
```
Expected: all 7 files listed without error.

- [ ] **Step 3: Commit**
```bash
git add web/src/components/ui/
git commit -m "feat(web/ui): add missing shadcn components for Athas integration"
```

---

### Task 3: Copy Athas shared utilities

**Files:**
- Create: `web/src/utils/` (new directory with files listed in File Map)

- [ ] **Step 1: Copy utils directory**
```bash
mkdir -p web/src/utils
cp /Users/char2cs/Projects/Cloned/athas/src/utils/zustand-selectors.ts web/src/utils/
cp /Users/char2cs/Projects/Cloned/athas/src/utils/cn.ts web/src/utils/
cp /Users/char2cs/Projects/Cloned/athas/src/utils/path-helpers.ts web/src/utils/
cp /Users/char2cs/Projects/Cloned/athas/src/utils/platform.ts web/src/utils/
cp /Users/char2cs/Projects/Cloned/athas/src/utils/date.ts web/src/utils/
cp /Users/char2cs/Projects/Cloned/athas/src/utils/search-match.ts web/src/utils/
cp /Users/char2cs/Projects/Cloned/athas/src/utils/fuzzy-matcher.tsx web/src/utils/
cp /Users/char2cs/Projects/Cloned/athas/src/utils/stream-utils.ts web/src/utils/
cp /Users/char2cs/Projects/Cloned/athas/src/utils/storage.ts web/src/utils/
cp /Users/char2cs/Projects/Cloned/athas/src/utils/keybinding-display.ts web/src/utils/
cp /Users/char2cs/Projects/Cloned/athas/src/utils/frecency.ts web/src/utils/
cp -r /Users/char2cs/Projects/Cloned/athas/src/utils/keyboard web/src/utils/
```

- [ ] **Step 2: Fix cn.ts — Crowbar uses `@/lib/utils` not `@/utils/cn`**

Check if `web/src/utils/cn.ts` re-exports the same `cn` function already in `web/src/lib/utils.ts`. If yes, replace `web/src/utils/cn.ts` with a re-export:
```ts
// web/src/utils/cn.ts
export { cn } from '@/lib/utils'
```

- [ ] **Step 3: TypeScript check**
```bash
cd web && npx tsc --noEmit --project tsconfig.json 2>&1 | grep "src/utils" | head -20
```
Fix any errors in the copied utils before proceeding.

- [ ] **Step 4: Commit**
```bash
git add web/src/utils/
git commit -m "feat(web): copy Athas shared utilities"
```

---

### Task 4: Copy custom Athas UI components

**Files:**
- Create: `web/src/components/ui/pane.tsx`
- Create: `web/src/components/ui/sidebar-tree.tsx`
- Create: `web/src/components/ui/item.tsx`
- Create: `web/src/components/ui/primitive-dialog-service.tsx`

- [ ] **Step 1: Copy the four custom components**
```bash
cp /Users/char2cs/Projects/Cloned/athas/src/ui/pane.tsx web/src/components/ui/pane.tsx
cp /Users/char2cs/Projects/Cloned/athas/src/ui/sidebar-tree.tsx web/src/components/ui/sidebar-tree.tsx
cp /Users/char2cs/Projects/Cloned/athas/src/ui/item.tsx web/src/components/ui/item.tsx
cp /Users/char2cs/Projects/Cloned/athas/src/ui/primitive-dialog-service.tsx web/src/components/ui/primitive-dialog-service.tsx
```

- [ ] **Step 2: Add header comment to each file**

Add `// copied from Athas — no shadcn/ui equivalent` as the first line of each of the four files.

- [ ] **Step 3: Fix import paths in each file**

Replace `@/ui/` with `@/components/ui/` and `@/utils/` with `@/utils/` (same path) in all four files:
```bash
sed -i '' "s|from '@/ui/|from '@/components/ui/|g" web/src/components/ui/pane.tsx web/src/components/ui/sidebar-tree.tsx web/src/components/ui/item.tsx web/src/components/ui/primitive-dialog-service.tsx
```

- [ ] **Step 4: TypeScript check**
```bash
cd web && npx tsc --noEmit 2>&1 | grep "components/ui/pane\|components/ui/sidebar-tree\|components/ui/item\|components/ui/primitive-dialog" | head -20
```
Fix any errors.

- [ ] **Step 5: Commit**
```bash
git add web/src/components/ui/pane.tsx web/src/components/ui/sidebar-tree.tsx web/src/components/ui/item.tsx web/src/components/ui/primitive-dialog-service.tsx
git commit -m "feat(web/ui): copy Athas custom UI primitives (pane, sidebar-tree, item, dialog-service)"
```

---

### Task 5: Create crowbar-bridge.ts

**Files:**
- Create: `web/src/lib/crowbar-bridge.ts`
- Create: `web/src/__tests__/lib/crowbar-bridge.test.ts`

- [ ] **Step 1: Write the test first**

```ts
// web/src/__tests__/lib/crowbar-bridge.test.ts
import { describe, it, expect, vi } from 'vitest'
import {
  terminalWrite,
  terminalResize,
  terminalClose,
  terminalListen,
  clipboardSet,
  clipboardPaste,
  clipboardGet,
  clipboardClear,
  setWindowTransparency,
  setMacOSWindowAppearance,
  toggleMenuBar,
} from '@/lib/crowbar-bridge'

describe('crowbar-bridge', () => {
  it('terminalWrite resolves without error', async () => {
    await expect(terminalWrite('id-1', 'hello')).resolves.toBeUndefined()
  })

  it('terminalResize resolves without error', async () => {
    await expect(terminalResize('id-1', 24, 80)).resolves.toBeUndefined()
  })

  it('terminalClose resolves without error', async () => {
    await expect(terminalClose('id-1')).resolves.toBeUndefined()
  })

  it('terminalListen returns an unlisten function', () => {
    const unlisten = terminalListen('id-1', vi.fn())
    expect(typeof unlisten).toBe('function')
    expect(() => unlisten()).not.toThrow()
  })

  it('clipboardSet stores entries in memory', async () => {
    await clipboardSet([{ path: '/foo', is_dir: false }], 'copy')
    const state = await clipboardGet()
    expect(state).toEqual({ entries: [{ path: '/foo', is_dir: false }], operation: 'copy' })
  })

  it('clipboardPaste returns empty array', async () => {
    const result = await clipboardPaste('/target')
    expect(result).toEqual([])
  })

  it('clipboardClear nulls the clipboard', async () => {
    await clipboardSet([{ path: '/foo', is_dir: false }], 'copy')
    await clipboardClear()
    const state = await clipboardGet()
    expect(state).toBeNull()
  })

  it('setWindowTransparency resolves without error', async () => {
    await expect(setWindowTransparency(true)).resolves.toBeUndefined()
  })

  it('setMacOSWindowAppearance resolves without error', async () => {
    await expect(setMacOSWindowAppearance('dark', false)).resolves.toBeUndefined()
  })

  it('toggleMenuBar resolves without error', async () => {
    await expect(toggleMenuBar(true)).resolves.toBeUndefined()
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**
```bash
cd web && npx vitest run src/__tests__/lib/crowbar-bridge.test.ts 2>&1 | tail -5
```
Expected: FAIL — module not found.

- [ ] **Step 3: Implement crowbar-bridge.ts**

```ts
// web/src/lib/crowbar-bridge.ts
// Crowbar system operations — mock implementations for this session.
// FUTURE: replace each function body with the real Crowbar Go API or Tauri plugin call.

// ── Terminal PTY ──────────────────────────────────────────────────────────────
// FUTURE: WebSocket to Go PTY handler at ws://localhost/api/terminal/:id

export async function terminalWrite(
  _id: string,
  _data: string,
): Promise<void> {
  // FUTURE: ws.send(JSON.stringify({ type: 'write', id: _id, data: _data }))
}

export async function terminalResize(
  _id: string,
  _rows: number,
  _cols: number,
): Promise<void> {
  // FUTURE: ws.send(JSON.stringify({ type: 'resize', id: _id, rows: _rows, cols: _cols }))
}

export async function terminalClose(_id: string): Promise<void> {
  // FUTURE: ws.send(JSON.stringify({ type: 'close', id: _id }))
}

export function terminalListen(
  _id: string,
  _onData: (data: string) => void,
): () => void {
  // FUTURE: subscribe to WebSocket messages for terminal output
  return () => {} // no-op unlisten
}

// ── File Clipboard ────────────────────────────────────────────────────────────
// FUTURE: Go API file operations at /api/fs/clipboard

export interface ClipboardEntry {
  path: string
  is_dir: boolean
}

export interface FileClipboardState {
  entries: ClipboardEntry[]
  operation: 'copy' | 'cut'
}

export interface PastedEntry {
  path: string
  success: boolean
}

let _clipboard: FileClipboardState | null = null

export async function clipboardSet(
  entries: ClipboardEntry[],
  operation: 'copy' | 'cut',
): Promise<void> {
  _clipboard = { entries, operation }
  // FUTURE: POST /api/fs/clipboard/set
}

export async function clipboardPaste(
  _targetDirectory: string,
): Promise<PastedEntry[]> {
  // FUTURE: POST /api/fs/clipboard/paste
  return []
}

export async function clipboardGet(): Promise<FileClipboardState | null> {
  // FUTURE: GET /api/fs/clipboard
  return _clipboard
}

export async function clipboardClear(): Promise<void> {
  _clipboard = null
  // FUTURE: DELETE /api/fs/clipboard
}

// ── Window Management ─────────────────────────────────────────────────────────
// FUTURE: Tauri plugin calls when Crowbar's desktop wrapper exposes them

export async function setWindowTransparency(_enabled: boolean): Promise<void> {
  // FUTURE: invoke Tauri window transparency plugin
}

export async function setMacOSWindowAppearance(
  _themeType: string,
  _transparencyEnabled: boolean,
): Promise<void> {
  // FUTURE: invoke Tauri macOS appearance plugin
}

export async function toggleMenuBar(_toggle: boolean): Promise<void> {
  // FUTURE: invoke Tauri menu bar plugin
}
```

- [ ] **Step 4: Run tests — expect PASS**
```bash
cd web && npx vitest run src/__tests__/lib/crowbar-bridge.test.ts
```
Expected: all 10 tests pass.

- [ ] **Step 5: Commit**
```bash
git add web/src/lib/crowbar-bridge.ts web/src/__tests__/lib/crowbar-bridge.test.ts
git commit -m "feat(web): add crowbar-bridge system operation stubs"
```

---

## Wave 2 — Feature Module Copy

> All copy tasks in this wave follow the same pattern:
> 1. `cp -r` the Athas feature folder into `web/src/features/`
> 2. Replace `from "@/ui/` → `from "@/components/ui/` (double quotes, in-place)
> 3. Replace `from '@/ui/` → `from '@/components/ui/` (single quotes, in-place)
> 4. Run `tsc --noEmit` scoped to the new folder, fix errors
> 5. Commit

The sed command template for steps 2–3:
```bash
find web/src/features/FEATURE_NAME -name "*.ts" -o -name "*.tsx" | xargs sed -i '' \
  -e 's|from "@/ui/|from "@/components/ui/|g' \
  -e "s|from '@/ui/|from '@/components/ui/|g"
```

---

### Task 6: Copy features/panes/

**Files:**
- Create: `web/src/features/panes/` (full directory)

- [ ] **Step 1: Copy**
```bash
cp -r /Users/char2cs/Projects/Cloned/athas/src/features/panes web/src/features/panes
```

- [ ] **Step 2: Fix UI import paths**
```bash
find web/src/features/panes -name "*.ts" -o -name "*.tsx" | xargs sed -i '' \
  -e 's|from "@/ui/|from "@/components/ui/|g' \
  -e "s|from '@/ui/|from '@/components/ui/|g"
```

- [ ] **Step 3: TypeScript check**
```bash
cd web && npx tsc --noEmit 2>&1 | grep "features/panes" | head -20
```
Fix any errors. Common issues: missing type imports from dropped features. If a pane type references a removed feature, replace with `unknown` or the appropriate type from the local pane types.

- [ ] **Step 4: Commit**
```bash
git add web/src/features/panes/
git commit -m "feat(web): copy Athas panes feature module"
```

---

### Task 7: Copy features/tabs/

**Files:**
- Create: `web/src/features/tabs/` (full directory)

- [ ] **Step 1: Copy**
```bash
cp -r /Users/char2cs/Projects/Cloned/athas/src/features/tabs web/src/features/tabs
```

- [ ] **Step 2: Fix UI import paths**
```bash
find web/src/features/tabs -name "*.ts" -o -name "*.tsx" | xargs sed -i '' \
  -e 's|from "@/ui/|from "@/components/ui/|g' \
  -e "s|from '@/ui/|from '@/components/ui/|g"
```

- [ ] **Step 3: TypeScript check — fix errors**
```bash
cd web && npx tsc --noEmit 2>&1 | grep "features/tabs" | head -20
```

- [ ] **Step 4: Commit**
```bash
git add web/src/features/tabs/
git commit -m "feat(web): copy Athas tabs feature module"
```

---

### Task 8: Copy features/editor/ and stub LSP

**Files:**
- Create: `web/src/features/editor/` (full directory)

The editor feature imports from `@/features/lsp/` which we are not copying. We stub the LSP client by replacing the import with a no-op module.

- [ ] **Step 1: Copy**
```bash
cp -r /Users/char2cs/Projects/Cloned/athas/src/features/editor web/src/features/editor
```

- [ ] **Step 2: Fix UI import paths**
```bash
find web/src/features/editor -name "*.ts" -o -name "*.tsx" | xargs sed -i '' \
  -e 's|from "@/ui/|from "@/components/ui/|g' \
  -e "s|from '@/ui/|from '@/components/ui/|g"
```

- [ ] **Step 3: Check for LSP imports**
```bash
grep -r "from.*features/lsp\|from.*lsp-client" web/src/features/editor/ --include="*.ts" --include="*.tsx" -l
```

- [ ] **Step 4: Create LSP stub**

Create `web/src/features/lsp/lsp-client.ts` — a no-op stub so the editor's LSP imports resolve:
```ts
// web/src/features/lsp/lsp-client.ts
// LSP is out of scope for this session. All methods are no-ops.
// FUTURE: wire to a real language server via the Go backend.

export class LspClient {
  isEnabled(): boolean { return false }
  async start(): Promise<void> {}
  async stop(): Promise<void> {}
  async getCompletions(): Promise<[]> { return [] }
  async getHover(): Promise<null> { return null }
  async getDefinition(): Promise<null> { return null }
  async formatDocument(): Promise<[]> { return [] }
  async getReferences(): Promise<null> { return null }
  async getDocumentSymbols(): Promise<[]> { return [] }
  async documentOpen(): Promise<void> {}
  async documentChange(): Promise<void> {}
  async documentSave(): Promise<void> {}
  async documentClose(): Promise<void> {}
}

export const lspClient = new LspClient()
export default lspClient
```

Also create `web/src/features/lsp/index.ts`:
```ts
export * from './lsp-client'
```

- [ ] **Step 5: Fix editor/stores/buffer-store.ts — replace close_terminal invoke() calls**

Find and replace the three `invoke("close_terminal", ...)` and `invoke("close_remote_terminal", ...)` calls in `web/src/features/editor/stores/buffer-store.ts`:

Replace the import line:
```ts
// BEFORE (near top of file)
import { invoke } from "@tauri-apps/api/core";
```
```ts
// AFTER
import { terminalClose } from '@/lib/crowbar-bridge'
```

Replace each invoke call:
```ts
// BEFORE
await invoke("close_terminal", { id: existingExternalEditor.terminalConnectionId })
// AFTER
await terminalClose(existingExternalEditor.terminalConnectionId)
```
```ts
// BEFORE
await invoke("close_terminal", { id: closedBuffer.terminalConnectionId })
// AFTER
await terminalClose(closedBuffer.terminalConnectionId)
```
```ts
// BEFORE
await invoke(closeCommand, { id: session.connectionId })
// AFTER (closeCommand was either "close_terminal" or "close_remote_terminal" — both map to terminalClose)
await terminalClose(session.connectionId)
```

Also fix `editor-app-store.ts` — remove the SSH write invoke (remote feature is dropped):
```bash
grep -n "ssh_write_file\|invoke" web/src/features/editor/stores/editor-app-store.ts | head -5
```
Remove the `invoke("ssh_write_file", ...)` block and the `import { invoke }` line from that file.

- [ ] **Step 6: TypeScript check**
```bash
cd web && npx tsc --noEmit 2>&1 | grep "features/editor\|features/lsp" | head -30
```
Fix remaining errors.

- [ ] **Step 7: Commit**
```bash
git add web/src/features/editor/ web/src/features/lsp/
git commit -m "feat(web): copy Athas editor feature module, stub LSP"
```

---

### Task 9: Copy features/athas-editor/

**Files:**
- Create: `web/src/features/athas-editor/` (full directory)

- [ ] **Step 1: Copy**
```bash
cp -r /Users/char2cs/Projects/Cloned/athas/src/features/athas-editor web/src/features/athas-editor
```

- [ ] **Step 2: Fix UI import paths**
```bash
find web/src/features/athas-editor -name "*.ts" -o -name "*.tsx" | xargs sed -i '' \
  -e 's|from "@/ui/|from "@/components/ui/|g' \
  -e "s|from '@/ui/|from '@/components/ui/|g"
```

- [ ] **Step 3: Fix external-editor-terminal.tsx — replace terminal invoke() calls**

In `web/src/features/athas-editor/components/external-editor-terminal.tsx` (if it exists):
Replace `import { invoke } from "@tauri-apps/api/core"` with:
```ts
import { terminalWrite, terminalResize } from '@/lib/crowbar-bridge'
```
Replace `invoke("terminal_write", { id: terminalConnectionId, data })` with `terminalWrite(terminalConnectionId, data)`.
Replace `invoke("terminal_resize", { id: terminalConnectionId, rows, cols })` with `terminalResize(terminalConnectionId, rows, cols)`.

- [ ] **Step 4: TypeScript check**
```bash
cd web && npx tsc --noEmit 2>&1 | grep "features/athas-editor" | head -20
```

- [ ] **Step 5: Commit**
```bash
git add web/src/features/athas-editor/
git commit -m "feat(web): copy Athas athas-editor feature module"
```

---

### Task 10: Copy features/file-explorer/ and replace clipboard invoke() calls

**Files:**
- Create: `web/src/features/file-explorer/` (full directory)

- [ ] **Step 1: Copy**
```bash
cp -r /Users/char2cs/Projects/Cloned/athas/src/features/file-explorer web/src/features/file-explorer
```

- [ ] **Step 2: Fix UI import paths**
```bash
find web/src/features/file-explorer -name "*.ts" -o -name "*.tsx" | xargs sed -i '' \
  -e 's|from "@/ui/|from "@/components/ui/|g' \
  -e "s|from '@/ui/|from '@/components/ui/|g"
```

- [ ] **Step 3: Replace invoke() in file-explorer-clipboard-store.ts**

In `web/src/features/file-explorer/stores/file-explorer-clipboard-store.ts`:

Replace:
```ts
import { invoke } from "@tauri-apps/api/core";
```
With:
```ts
import {
  clipboardSet,
  clipboardPaste,
  clipboardGet,
  clipboardClear,
  type ClipboardEntry,
  type FileClipboardState,
  type PastedEntry,
} from '@/lib/crowbar-bridge'
```

Remove the local `ClipboardEntry`, `FileClipboardState`, `PastedEntry` type definitions (they now come from crowbar-bridge).

Replace each invoke call:
```ts
// BEFORE
await invoke("clipboard_set", { entries, operation: "copy" });
// AFTER
await clipboardSet(entries, "copy");
```
```ts
// BEFORE
await invoke("clipboard_set", { entries, operation: "cut" });
// AFTER
await clipboardSet(entries, "cut");
```
```ts
// BEFORE
const result = await invoke<PastedEntry[]>("clipboard_paste", { targetDirectory });
// AFTER
const result = await clipboardPaste(targetDirectory);
```
```ts
// BEFORE
const clipboard = await invoke<FileClipboardState | null>("clipboard_get");
// AFTER
const clipboard = await clipboardGet();
```
```ts
// BEFORE
await invoke("clipboard_clear");
// AFTER
await clipboardClear();
```

- [ ] **Step 4: TypeScript check**
```bash
cd web && npx tsc --noEmit 2>&1 | grep "features/file-explorer" | head -20
```

- [ ] **Step 5: Commit**
```bash
git add web/src/features/file-explorer/
git commit -m "feat(web): copy Athas file-explorer feature module, replace clipboard invoke calls"
```

---

### Task 11: Copy features/terminal/ and replace PTY invoke() calls

**Files:**
- Create: `web/src/features/terminal/` (full directory)

- [ ] **Step 1: Copy**
```bash
cp -r /Users/char2cs/Projects/Cloned/athas/src/features/terminal web/src/features/terminal
```

- [ ] **Step 2: Fix UI import paths**
```bash
find web/src/features/terminal -name "*.ts" -o -name "*.tsx" | xargs sed -i '' \
  -e 's|from "@/ui/|from "@/components/ui/|g' \
  -e "s|from '@/ui/|from '@/components/ui/|g"
```

- [ ] **Step 3: Replace invoke() and listen() in use-terminal-connection.ts**

In `web/src/features/terminal/hooks/use-terminal-connection.ts`:

Replace:
```ts
import { invoke } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";
```
With:
```ts
import { terminalWrite, terminalResize, terminalClose, terminalListen } from '@/lib/crowbar-bridge'
```

Replace every `invoke("terminal_write", ...)` / `invoke("remote_terminal_write", ...)` with `terminalWrite(id, data)`.
Replace every `invoke("terminal_resize", ...)` / `invoke("remote_terminal_resize", ...)` with `terminalResize(id, rows, cols)`.
Replace every `invoke("close_terminal", ...)` / `invoke("close_remote_terminal", ...)` with `terminalClose(id)`.
Replace `listen('terminal-output', ...)` usage pattern with `terminalListen(id, onData)` — the returned unlisten function replaces the Tauri event unlisten.

- [ ] **Step 4: Replace invoke() in terminal-container.tsx**

In `web/src/features/terminal/components/terminal-container.tsx`:

Replace `import { invoke } from "@tauri-apps/api/core"` with:
```ts
import { terminalWrite, terminalClose } from '@/lib/crowbar-bridge'
```
Replace `invoke(closeCommand, { id: session.connectionId })` with `terminalClose(session.connectionId)`.
Replace `invoke("terminal_write", { id: connectionId, data: pendingCommand })` with `terminalWrite(connectionId, pendingCommand)`.

- [ ] **Step 5: TypeScript check**
```bash
cd web && npx tsc --noEmit 2>&1 | grep "features/terminal" | head -20
```

- [ ] **Step 6: Commit**
```bash
git add web/src/features/terminal/
git commit -m "feat(web): copy Athas terminal feature module, replace PTY invoke calls"
```

---

### Task 12: Copy features/git/

**Files:**
- Create: `web/src/features/git/` (full directory)

The git feature has no `invoke()` calls (git operations are handled by Athas's Rust backend via a different mechanism — likely event listeners set up at app init, not inline invoke calls in the feature module itself).

- [ ] **Step 1: Copy**
```bash
cp -r /Users/char2cs/Projects/Cloned/athas/src/features/git web/src/features/git
```

- [ ] **Step 2: Fix UI import paths**
```bash
find web/src/features/git -name "*.ts" -o -name "*.tsx" | xargs sed -i '' \
  -e 's|from "@/ui/|from "@/components/ui/|g' \
  -e "s|from '@/ui/|from '@/components/ui/|g"
```

- [ ] **Step 3: Check for any Tauri imports**
```bash
grep -r "tauri-apps\|invoke\|listen" web/src/features/git/ --include="*.ts" --include="*.tsx" | head -10
```
If any are found, replace with no-ops following the same pattern as previous tasks.

- [ ] **Step 4: TypeScript check**
```bash
cd web && npx tsc --noEmit 2>&1 | grep "features/git" | head -20
```

- [ ] **Step 5: Commit**
```bash
git add web/src/features/git/
git commit -m "feat(web): copy Athas git feature module"
```

---

### Task 13: Copy features/settings/ — trimmed to 7 panels

**Files:**
- Create: `web/src/features/settings/` (full directory, then delete 8 panels)

- [ ] **Step 1: Copy**
```bash
cp -r /Users/char2cs/Projects/Cloned/athas/src/features/settings web/src/features/settings
```

- [ ] **Step 2: Delete the 8 non-applicable settings panels**
```bash
rm web/src/features/settings/components/tabs/account-settings.tsx
rm web/src/features/settings/components/tabs/advanced-settings.tsx
rm web/src/features/settings/components/tabs/ai-settings.tsx
rm web/src/features/settings/components/tabs/collaboration-settings.tsx
rm web/src/features/settings/components/tabs/database-settings.tsx
rm web/src/features/settings/components/tabs/enterprise-settings.tsx
rm web/src/features/settings/components/tabs/extensions-settings.tsx
rm web/src/features/settings/components/tabs/features-settings.tsx
```

- [ ] **Step 3: Fix UI import paths**
```bash
find web/src/features/settings -name "*.ts" -o -name "*.tsx" | xargs sed -i '' \
  -e 's|from "@/ui/|from "@/components/ui/|g' \
  -e "s|from '@/ui/|from '@/components/ui/|g"
```

- [ ] **Step 4: Replace window invoke() calls in settings-effects.ts**

In `web/src/features/settings/lib/settings-effects.ts`:

Replace:
```ts
import { invoke } from "@tauri-apps/api/core";
```
With:
```ts
import { setWindowTransparency, setMacOSWindowAppearance } from '@/lib/crowbar-bridge'
```

Replace:
```ts
void invoke("set_window_transparency_enabled", { enabled }).catch(...)
```
With:
```ts
void setWindowTransparency(enabled).catch(() => {})
```

Replace:
```ts
void invoke("set_macos_window_appearance", { themeType, transparencyEnabled })
```
With:
```ts
void setMacOSWindowAppearance(themeType, transparencyEnabled)
```

- [ ] **Step 5: Replace toggle_menu_bar invoke() in appearance-settings.tsx**

In `web/src/features/settings/components/tabs/appearance-settings.tsx`:

Replace `import { invoke } from "@tauri-apps/api/core"` → add to imports: `import { toggleMenuBar } from '@/lib/crowbar-bridge'`
Replace `invoke("toggle_menu_bar", { toggle: checked })` → `void toggleMenuBar(checked)`

- [ ] **Step 6: Update the settings dialog to only show 7 tabs**

In `web/src/features/settings/components/settings-dialog.tsx` (or wherever tabs are declared), remove references to the 8 deleted panels. The tab list should only contain: `general`, `appearance`, `editor`, `terminal`, `keyboard` (or `keybindings`), `git`, `file-tree`.

Remove their imports and entries from the tab config array/object.

- [ ] **Step 7: TypeScript check**
```bash
cd web && npx tsc --noEmit 2>&1 | grep "features/settings" | head -30
```
Fix any errors from removed panels.

- [ ] **Step 8: Commit**
```bash
git add web/src/features/settings/
git commit -m "feat(web): copy Athas settings feature module, trim to 7 panels, replace window invoke calls"
```

---

### Task 14: Copy features/command-palette/ and replace toggle_menu_bar

**Files:**
- Create: `web/src/features/command-palette/` (full directory)

- [ ] **Step 1: Copy**
```bash
cp -r /Users/char2cs/Projects/Cloned/athas/src/features/command-palette web/src/features/command-palette
```

- [ ] **Step 2: Fix UI import paths**
```bash
find web/src/features/command-palette -name "*.ts" -o -name "*.tsx" | xargs sed -i '' \
  -e 's|from "@/ui/|from "@/components/ui/|g' \
  -e "s|from '@/ui/|from '@/components/ui/|g"
```

- [ ] **Step 3: Replace toggle_menu_bar in view-actions.tsx**

In `web/src/features/command-palette/constants/view-actions.tsx`:
Add import: `import { toggleMenuBar } from '@/lib/crowbar-bridge'`
Replace `invoke("toggle_menu_bar", { toggle: newValue })` → `void toggleMenuBar(newValue)`
Remove the `import { invoke }` line.

- [ ] **Step 4: Remove action entries that reference dropped features**

In the command palette actions list, remove any actions that reference LSP, extensions, remote, vim, AI, or collaboration features that we didn't copy. If an action's handler calls into a missing feature, delete that action entry.

- [ ] **Step 5: TypeScript check**
```bash
cd web && npx tsc --noEmit 2>&1 | grep "features/command-palette" | head -20
```

- [ ] **Step 6: Commit**
```bash
git add web/src/features/command-palette/
git commit -m "feat(web): copy Athas command-palette feature module"
```

---

### Task 15: Copy features/layout/ (MainSidebar, BottomPane, Footer)

**Files:**
- Create: `web/src/features/layout/` (subdirs: `sidebar/`, `bottom-pane/`, `footer/`)

We only need `sidebar/`, `bottom-pane/`, and `footer/` subdirectories. We do NOT copy `main-layout.tsx` (Crowbar has its own layout composition via IDEShell).

- [ ] **Step 1: Copy the three subdirectories**
```bash
mkdir -p web/src/features/layout
cp -r /Users/char2cs/Projects/Cloned/athas/src/features/layout/components/sidebar web/src/features/layout/
cp -r /Users/char2cs/Projects/Cloned/athas/src/features/layout/components/bottom-pane web/src/features/layout/
cp -r /Users/char2cs/Projects/Cloned/athas/src/features/layout/components/footer web/src/features/layout/
```

- [ ] **Step 2: Fix UI import paths**
```bash
find web/src/features/layout -name "*.ts" -o -name "*.tsx" | xargs sed -i '' \
  -e 's|from "@/ui/|from "@/components/ui/|g' \
  -e "s|from '@/ui/|from '@/components/ui/|g"
```

- [ ] **Step 3: Check for references to dropped features**
```bash
grep -r "features/vim\|features/lsp\|features/extensions\|features/remote\|features/ai\|features/collaboration\|features/window" web/src/features/layout/ --include="*.ts" --include="*.tsx" | head -20
```
For each reference found, remove the import and the component usage from the layout files. The sidebar, bottom pane, and footer should render without those features.

- [ ] **Step 4: TypeScript check**
```bash
cd web && npx tsc --noEmit 2>&1 | grep "features/layout" | head -20
```

- [ ] **Step 5: Commit**
```bash
git add web/src/features/layout/
git commit -m "feat(web): copy Athas layout feature module (sidebar, bottom-pane, footer)"
```

---

## Wave 3 — Mock Data Layer

### Task 16: Create lib/mock/files.ts

**Files:**
- Create: `web/src/lib/mock/files.ts`
- Create: `web/src/__tests__/lib/mock/files.test.ts`

- [ ] **Step 1: Write the test first**

```ts
// web/src/__tests__/lib/mock/files.test.ts
import { describe, it, expect } from 'vitest'
import { getMockFileTree } from '@/lib/mock/files'
import type { FileNode } from '@/lib/mock/files'

describe('getMockFileTree', () => {
  it('returns a non-empty array for any rootPath', () => {
    const tree = getMockFileTree('/any/path')
    expect(tree.length).toBeGreaterThan(0)
  })

  it('every node has required fields', () => {
    const tree = getMockFileTree('/workspace')
    function checkNode(node: FileNode) {
      expect(typeof node.name).toBe('string')
      expect(typeof node.path).toBe('string')
      expect(typeof node.isDirectory).toBe('boolean')
      if (node.children) {
        node.children.forEach(checkNode)
      }
    }
    tree.forEach(checkNode)
  })

  it('returns at least one directory node', () => {
    const tree = getMockFileTree('/workspace')
    const hasDir = tree.some(n => n.isDirectory)
    expect(hasDir).toBe(true)
  })

  it('returns at least one file node', () => {
    const tree = getMockFileTree('/workspace')
    function hasFile(nodes: FileNode[]): boolean {
      return nodes.some(n => !n.isDirectory || (n.children ? hasFile(n.children) : false))
    }
    expect(hasFile(tree)).toBe(true)
  })

  it('git status field is optional', () => {
    const tree = getMockFileTree('/workspace')
    const withStatus = tree.some(n => n.gitStatus !== undefined)
    // At least some nodes should have git status in the mock
    expect(withStatus).toBe(true)
  })
})
```

- [ ] **Step 2: Run test — expect FAIL**
```bash
cd web && npx vitest run src/__tests__/lib/mock/files.test.ts 2>&1 | tail -5
```

- [ ] **Step 3: Implement files.ts**

```ts
// web/src/lib/mock/files.ts

export type GitStatus = 'modified' | 'added' | 'deleted' | 'renamed' | 'untracked'

export interface FileNode {
  name: string
  path: string
  isDirectory: boolean
  gitStatus?: GitStatus
  children?: FileNode[]
}

export function getMockFileTree(_rootPath: string): FileNode[] {
  return [
    {
      name: 'src',
      path: 'src',
      isDirectory: true,
      children: [
        {
          name: 'payment',
          path: 'src/payment',
          isDirectory: true,
          children: [
            { name: 'PaymentService.ts', path: 'src/payment/PaymentService.ts', isDirectory: false, gitStatus: 'modified' },
            { name: 'PaymentError.ts', path: 'src/payment/PaymentError.ts', isDirectory: false, gitStatus: 'added' },
            { name: 'webhook.ts', path: 'src/payment/webhook.ts', isDirectory: false },
            { name: 'payment.test.ts', path: 'src/payment/payment.test.ts', isDirectory: false, gitStatus: 'modified' },
          ],
        },
        {
          name: 'auth',
          path: 'src/auth',
          isDirectory: true,
          children: [
            { name: 'AuthService.ts', path: 'src/auth/AuthService.ts', isDirectory: false },
            { name: 'middleware.ts', path: 'src/auth/middleware.ts', isDirectory: false },
          ],
        },
        {
          name: 'db',
          path: 'src/db',
          isDirectory: true,
          children: [
            { name: 'schema.ts', path: 'src/db/schema.ts', isDirectory: false },
            { name: 'migrations.ts', path: 'src/db/migrations.ts', isDirectory: false },
          ],
        },
        { name: 'index.ts', path: 'src/index.ts', isDirectory: false },
        { name: 'config.ts', path: 'src/config.ts', isDirectory: false },
      ],
    },
    { name: 'package.json', path: 'package.json', isDirectory: false },
    { name: 'tsconfig.json', path: 'tsconfig.json', isDirectory: false },
    { name: 'README.md', path: 'README.md', isDirectory: false },
    { name: '.env.example', path: '.env.example', isDirectory: false, gitStatus: 'untracked' },
  ]
}
```

- [ ] **Step 4: Run tests — expect PASS**
```bash
cd web && npx vitest run src/__tests__/lib/mock/files.test.ts
```

- [ ] **Step 5: Commit**
```bash
git add web/src/lib/mock/files.ts web/src/__tests__/lib/mock/files.test.ts
git commit -m "feat(web): add mock file tree data"
```

---

### Task 17: Create lib/mock/git-data.ts and lib/mock/terminal.ts

**Files:**
- Create: `web/src/lib/mock/git-data.ts`
- Create: `web/src/lib/mock/terminal.ts`
- Create: `web/src/__tests__/lib/mock/git-data.test.ts`
- Create: `web/src/__tests__/lib/mock/terminal.test.ts`

- [ ] **Step 1: Write git-data tests**

```ts
// web/src/__tests__/lib/mock/git-data.test.ts
import { describe, it, expect } from 'vitest'
import { getMockGitStatus, getMockCommitHistory, getMockBranches } from '@/lib/mock/git-data'

describe('getMockGitStatus', () => {
  it('returns staged and unstaged files', () => {
    const status = getMockGitStatus('/repo')
    expect(Array.isArray(status.staged)).toBe(true)
    expect(Array.isArray(status.unstaged)).toBe(true)
    expect(status.staged.length + status.unstaged.length).toBeGreaterThan(0)
  })
  it('each file has path and status', () => {
    const status = getMockGitStatus('/repo')
    ;[...status.staged, ...status.unstaged].forEach(f => {
      expect(typeof f.path).toBe('string')
      expect(typeof f.status).toBe('string')
    })
  })
})

describe('getMockCommitHistory', () => {
  it('returns at least 3 commits', () => {
    const commits = getMockCommitHistory('/repo')
    expect(commits.length).toBeGreaterThanOrEqual(3)
  })
  it('each commit has required fields', () => {
    getMockCommitHistory('/repo').forEach(c => {
      expect(typeof c.hash).toBe('string')
      expect(typeof c.message).toBe('string')
      expect(typeof c.author).toBe('string')
      expect(typeof c.date).toBe('string')
    })
  })
})

describe('getMockBranches', () => {
  it('returns at least one branch', () => {
    expect(getMockBranches('/repo').length).toBeGreaterThan(0)
  })
  it('exactly one branch is current', () => {
    const current = getMockBranches('/repo').filter(b => b.isCurrent)
    expect(current.length).toBe(1)
  })
})
```

- [ ] **Step 2: Write terminal tests**

```ts
// web/src/__tests__/lib/mock/terminal.test.ts
import { describe, it, expect } from 'vitest'
import { getMockTerminalSeed } from '@/lib/mock/terminal'

describe('getMockTerminalSeed', () => {
  it('returns a prompt string', () => {
    const seed = getMockTerminalSeed()
    expect(typeof seed.prompt).toBe('string')
    expect(seed.prompt.length).toBeGreaterThan(0)
  })
  it('returns prior output lines', () => {
    const seed = getMockTerminalSeed()
    expect(Array.isArray(seed.priorOutput)).toBe(true)
    expect(seed.priorOutput.length).toBeGreaterThan(0)
  })
})
```

- [ ] **Step 3: Run tests — expect FAIL**
```bash
cd web && npx vitest run src/__tests__/lib/mock/git-data.test.ts src/__tests__/lib/mock/terminal.test.ts 2>&1 | tail -5
```

- [ ] **Step 4: Implement git-data.ts**

```ts
// web/src/lib/mock/git-data.ts

export type FileStatus = 'modified' | 'added' | 'deleted' | 'renamed'

export interface GitFile {
  path: string
  status: FileStatus
}

export interface GitStatus {
  staged: GitFile[]
  unstaged: GitFile[]
  branch: string
}

export interface Commit {
  hash: string
  shortHash: string
  message: string
  author: string
  date: string
}

export interface Branch {
  name: string
  isCurrent: boolean
  isRemote: boolean
  lastCommit?: string
}

export function getMockGitStatus(_repoPath: string): GitStatus {
  return {
    branch: 'feat/payment-flow',
    staged: [
      { path: 'src/payment/PaymentError.ts', status: 'added' },
    ],
    unstaged: [
      { path: 'src/payment/PaymentService.ts', status: 'modified' },
      { path: 'src/payment/payment.test.ts', status: 'modified' },
    ],
  }
}

export function getMockCommitHistory(_repoPath: string): Commit[] {
  return [
    {
      hash: 'a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2',
      shortHash: 'a1b2c3d',
      message: 'feat: add PaymentError with typed codes',
      author: 'Mateo Urrutia',
      date: '2 hours ago',
    },
    {
      hash: 'b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3',
      shortHash: 'b2c3d4e',
      message: 'feat: scaffold PaymentService class',
      author: 'Mateo Urrutia',
      date: '4 hours ago',
    },
    {
      hash: 'c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4',
      shortHash: 'c3d4e5f',
      message: 'chore: add payment module structure',
      author: 'Mateo Urrutia',
      date: '1 day ago',
    },
    {
      hash: 'd4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5',
      shortHash: 'd4e5f6a',
      message: 'feat: initial project scaffold',
      author: 'Mateo Urrutia',
      date: '3 days ago',
    },
    {
      hash: 'e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6',
      shortHash: 'e5f6a1b',
      message: 'chore: add README and license',
      author: 'Mateo Urrutia',
      date: '5 days ago',
    },
  ]
}

export function getMockBranches(_repoPath: string): Branch[] {
  return [
    { name: 'feat/payment-flow', isCurrent: true, isRemote: false, lastCommit: 'feat: add PaymentError with typed codes' },
    { name: 'main', isCurrent: false, isRemote: false, lastCommit: 'chore: add README and license' },
    { name: 'fix/auth-bug', isCurrent: false, isRemote: false, lastCommit: 'fix: resolve token expiry edge case' },
    { name: 'origin/main', isCurrent: false, isRemote: true, lastCommit: 'chore: add README and license' },
  ]
}
```

- [ ] **Step 5: Implement terminal.ts**

```ts
// web/src/lib/mock/terminal.ts

export interface TerminalSeed {
  prompt: string
  priorOutput: string[]
}

export function getMockTerminalSeed(): TerminalSeed {
  return {
    prompt: '$ ',
    priorOutput: [
      '\x1b[1;32m~/projects/payment-api\x1b[0m on \x1b[1;35mfeat/payment-flow\x1b[0m',
      '❯ bun test src/payment/',
      '\x1b[32m✓\x1b[0m PaymentService › handleWebhook › validates payload \x1b[90m(12ms)\x1b[0m',
      '\x1b[32m✓\x1b[0m PaymentService › handleWebhook › throws PaymentError on invalid data \x1b[90m(8ms)\x1b[0m',
      '\x1b[32m✓\x1b[0m PaymentError › has typed error code \x1b[90m(3ms)\x1b[0m',
      '',
      '\x1b[32m3 tests passed\x1b[0m in 45ms',
      '',
      '\x1b[1;32m~/projects/payment-api\x1b[0m on \x1b[1;35mfeat/payment-flow\x1b[0m',
    ],
  }
}
```

- [ ] **Step 6: Run tests — expect PASS**
```bash
cd web && npx vitest run src/__tests__/lib/mock/git-data.test.ts src/__tests__/lib/mock/terminal.test.ts
```

- [ ] **Step 7: Commit**
```bash
git add web/src/lib/mock/git-data.ts web/src/lib/mock/terminal.ts web/src/__tests__/lib/mock/git-data.test.ts web/src/__tests__/lib/mock/terminal.test.ts
git commit -m "feat(web): add mock git and terminal data"
```

---

### Task 18: Extend lib/queries.ts with IDE query options

**Files:**
- Modify: `web/src/lib/queries.ts`

- [ ] **Step 1: Add tests for new query options**

Add to `web/src/__tests__/lib/mock/projects.test.ts` (extend existing test file) or create a new file `web/src/__tests__/lib/queries-ide.test.ts`:

```ts
// web/src/__tests__/lib/queries-ide.test.ts
import { describe, it, expect } from 'vitest'
import { fileTreeQueryOptions, gitStatusQueryOptions, gitHistoryQueryOptions, gitBranchesQueryOptions } from '@/lib/queries'

describe('IDE query options', () => {
  it('fileTreeQueryOptions has correct queryKey', () => {
    const opts = fileTreeQueryOptions('/workspace')
    expect(opts.queryKey).toEqual(['file-tree', '/workspace'])
  })

  it('fileTreeQueryOptions queryFn returns non-empty array', async () => {
    const opts = fileTreeQueryOptions('/workspace')
    const result = await opts.queryFn()
    expect(Array.isArray(result)).toBe(true)
    expect((result as unknown[]).length).toBeGreaterThan(0)
  })

  it('gitStatusQueryOptions has correct queryKey', () => {
    const opts = gitStatusQueryOptions('/repo')
    expect(opts.queryKey).toEqual(['git-status', '/repo'])
  })

  it('gitStatusQueryOptions queryFn returns status object', async () => {
    const opts = gitStatusQueryOptions('/repo')
    const result = await opts.queryFn()
    expect(result).toHaveProperty('staged')
    expect(result).toHaveProperty('unstaged')
    expect(result).toHaveProperty('branch')
  })

  it('gitHistoryQueryOptions queryFn returns commits array', async () => {
    const opts = gitHistoryQueryOptions('/repo')
    const result = await opts.queryFn()
    expect(Array.isArray(result)).toBe(true)
    expect((result as unknown[]).length).toBeGreaterThanOrEqual(3)
  })

  it('gitBranchesQueryOptions queryFn returns branches array', async () => {
    const opts = gitBranchesQueryOptions('/repo')
    const result = await opts.queryFn()
    expect(Array.isArray(result)).toBe(true)
    expect((result as unknown[]).some((b: unknown) => (b as { isCurrent: boolean }).isCurrent)).toBe(true)
  })
})
```

- [ ] **Step 2: Run test — expect FAIL**
```bash
cd web && npx vitest run src/__tests__/lib/queries-ide.test.ts 2>&1 | tail -5
```

- [ ] **Step 3: Add query options to lib/queries.ts**

Append to the existing `web/src/lib/queries.ts`:

```ts
import { getMockFileTree } from '@/lib/mock/files'
import { getMockGitStatus, getMockCommitHistory, getMockBranches } from '@/lib/mock/git-data'

export const fileTreeQueryOptions = (rootPath: string) =>
  queryOptions({
    queryKey: ['file-tree', rootPath] as const,
    queryFn: () => getMockFileTree(rootPath),
    // FUTURE: queryFn: () => api.get(`/fs/tree?root=${encodeURIComponent(rootPath)}`)
  })

export const gitStatusQueryOptions = (repoPath: string) =>
  queryOptions({
    queryKey: ['git-status', repoPath] as const,
    queryFn: () => getMockGitStatus(repoPath),
    // FUTURE: queryFn: () => api.get(`/git/status?repo=${encodeURIComponent(repoPath)}`)
  })

export const gitHistoryQueryOptions = (repoPath: string) =>
  queryOptions({
    queryKey: ['git-history', repoPath] as const,
    queryFn: () => getMockCommitHistory(repoPath),
    // FUTURE: queryFn: () => api.get(`/git/log?repo=${encodeURIComponent(repoPath)}`)
  })

export const gitBranchesQueryOptions = (repoPath: string) =>
  queryOptions({
    queryKey: ['git-branches', repoPath] as const,
    queryFn: () => getMockBranches(repoPath),
    // FUTURE: queryFn: () => api.get(`/git/branches?repo=${encodeURIComponent(repoPath)}`)
  })
```

Note: `queryOptions` is already imported in the file from `@tanstack/react-query`.

- [ ] **Step 4: Run tests — expect PASS**
```bash
cd web && npx vitest run src/__tests__/lib/queries-ide.test.ts
```

- [ ] **Step 5: Commit**
```bash
git add web/src/lib/queries.ts web/src/__tests__/lib/queries-ide.test.ts
git commit -m "feat(web): add IDE query options for file tree, git status, and branches"
```

---

## Wave 4 — Glue Components

### Task 19: Create WorkspacesSidebarPanel.tsx

This is `Sidebar.tsx` extracted from its outer container. Internals are **identical** — only the filename changes and the file gets the outer container removed (that's now handled by MainSidebar from Athas).

**Files:**
- Create: `web/src/components/layout/WorkspacesSidebarPanel.tsx`
- Create: `web/src/__tests__/components/layout/WorkspacesSidebarPanel.test.tsx`

- [ ] **Step 1: Write the test first**

```tsx
// web/src/__tests__/components/layout/WorkspacesSidebarPanel.test.tsx
import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { WorkspacesSidebarPanel } from '@/components/layout/WorkspacesSidebarPanel'

const baseProps = {
  userInitials: 'MU',
  chats: [{ id: 'c1', title: 'Test chat', age: '1h' }],
  repos: [],
}

describe('WorkspacesSidebarPanel', () => {
  it('renders chat titles', () => {
    render(<WorkspacesSidebarPanel {...baseProps} />)
    expect(screen.getByText('Test chat')).toBeInTheDocument()
  })

  it('renders New chat button', () => {
    render(<WorkspacesSidebarPanel {...baseProps} />)
    expect(screen.getByText('New chat')).toBeInTheDocument()
  })

  it('renders repo names', () => {
    const repos = [{ id: 'r1', name: 'payment-api', avatarLabel: 'P', avatarColor: '#6366f1', workspaces: [] }]
    render(<WorkspacesSidebarPanel {...baseProps} repos={repos} />)
    expect(screen.getByText('payment-api')).toBeInTheDocument()
  })

  it('calls onChatClick when a chat is clicked', () => {
    const onChatClick = vi.fn()
    render(<WorkspacesSidebarPanel {...baseProps} onChatClick={onChatClick} />)
    screen.getByText('Test chat').click()
    expect(onChatClick).toHaveBeenCalledWith('c1')
  })
})
```

- [ ] **Step 2: Run test — expect FAIL**
```bash
cd web && npx vitest run src/__tests__/components/layout/WorkspacesSidebarPanel.test.tsx 2>&1 | tail -5
```

- [ ] **Step 3: Create WorkspacesSidebarPanel.tsx**

Copy `web/src/components/layout/Sidebar.tsx` to `web/src/components/layout/WorkspacesSidebarPanel.tsx`, then:

1. Rename the exported function from `Sidebar` to `WorkspacesSidebarPanel`
2. Remove the outer `<div className="flex h-full flex-col overflow-hidden bg-card">` wrapper — this is now provided by Athas's `MainSidebar`. The component should return `<>` fragment or a simple `<div className="flex flex-1 flex-col overflow-hidden">` that fits inside the sidebar panel.

The file should look like:
```tsx
// web/src/components/layout/WorkspacesSidebarPanel.tsx
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import { SidebarHeader } from './SidebarHeader'
import { ChatRow, RepoRow, WorkspaceRow, NewRow } from './SidebarRow'
import type { ProjectChat, Repo } from '@/lib/store/sidebar'

export type { ProjectChat, Repo }

interface WorkspacesSidebarPanelProps {
  userInitials: string
  chats: ProjectChat[]
  repos: Repo[]
  collapsedRepos?: Set<string>
  activeChatId?: string
  activeWorkspaceId?: string
  onChatClick?: (id: string) => void
  onWorkspaceClick?: (repoId: string, workspaceId: string) => void
  onNewChat?: () => void
  onNewWorkspace?: () => void
  onDeleteChat?: (id: string) => void
  onDeleteWorkspace?: (wsId: string) => void
  onRepoToggle?: (repoId: string) => void
  onProjectsClick?: () => void
  onProjectSelect?: (id: string) => void
  onSettingsClick?: () => void
}

const EMPTY_SET = new Set<string>()

export function WorkspacesSidebarPanel({
  userInitials, chats, repos, collapsedRepos = EMPTY_SET,
  activeChatId, activeWorkspaceId,
  onChatClick, onWorkspaceClick, onNewChat, onNewWorkspace,
  onDeleteChat, onDeleteWorkspace, onRepoToggle, onProjectsClick,
  onProjectSelect, onSettingsClick,
}: WorkspacesSidebarPanelProps) {
  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      <SidebarHeader
        userInitials={userInitials}
        onProjectsClick={onProjectsClick}
        onProjectSelect={onProjectSelect}
        onSettingsClick={onSettingsClick}
      />
      <ScrollArea className="flex-1">
        <div className="py-1">
          {chats.map(chat => (
            <ChatRow
              key={chat.id}
              title={chat.title}
              age={chat.age}
              active={chat.id === activeChatId}
              onClick={() => onChatClick?.(chat.id)}
              onDelete={onDeleteChat ? () => onDeleteChat(chat.id) : undefined}
            />
          ))}
          <NewRow label="New chat" onClick={onNewChat} />
          <Separator className="my-1 mx-3" />
          {repos.map(repo => {
            const collapsed = collapsedRepos.has(repo.id)
            return (
              <div key={repo.id}>
                <RepoRow
                  name={repo.name}
                  avatarLabel={repo.avatarLabel}
                  avatarColor={repo.avatarColor}
                  collapsed={collapsed}
                  onClick={() => onRepoToggle?.(repo.id)}
                />
                {!collapsed && (
                  <>
                    {repo.workspaces.map(ws => (
                      <WorkspaceRow
                        key={ws.id}
                        num={ws.num}
                        branch={ws.branch}
                        added={ws.added}
                        deleted={ws.deleted}
                        age={ws.age}
                        active={ws.id === activeWorkspaceId}
                        onClick={() => onWorkspaceClick?.(repo.id, ws.id)}
                        onDelete={onDeleteWorkspace ? () => onDeleteWorkspace(ws.id) : undefined}
                      />
                    ))}
                    <NewRow label="New workspace" onClick={onNewWorkspace} />
                  </>
                )}
              </div>
            )
          })}
        </div>
      </ScrollArea>
    </div>
  )
}
```

Note: `onSettingsClick` prop is added — it will be wired in SidebarTabs to open the settings dialog.

- [ ] **Step 4: Run tests — expect PASS**
```bash
cd web && npx vitest run src/__tests__/components/layout/WorkspacesSidebarPanel.test.tsx
```

- [ ] **Step 5: Commit**
```bash
git add web/src/components/layout/WorkspacesSidebarPanel.tsx web/src/__tests__/components/layout/WorkspacesSidebarPanel.test.tsx
git commit -m "feat(web): add WorkspacesSidebarPanel (extracted from Sidebar)"
```

---

### Task 20: Create SidebarTabs.tsx

**Files:**
- Create: `web/src/components/layout/SidebarTabs.tsx`
- Create: `web/src/__tests__/components/layout/SidebarTabs.test.tsx`

- [ ] **Step 1: Write the test**

```tsx
// web/src/__tests__/components/layout/SidebarTabs.test.tsx
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { SidebarTabs } from '@/components/layout/SidebarTabs'

vi.mock('@/features/file-explorer/components/file-explorer-tree', () => ({
  FileExplorerTree: () => <div data-testid="file-explorer" />,
}))
vi.mock('@/features/git/components/git-view', () => ({
  GitView: () => <div data-testid="git-view" />,
}))
vi.mock('@/components/layout/WorkspacesSidebarPanel', () => ({
  WorkspacesSidebarPanel: () => <div data-testid="workspaces-panel" />,
}))

const baseProps = {
  userInitials: 'MU',
  chats: [],
  repos: [],
  activeWorkspaceRepoPath: '/repo',
}

describe('SidebarTabs', () => {
  it('renders Workspaces tab by default', () => {
    render(<SidebarTabs {...baseProps} />)
    expect(screen.getByTestId('workspaces-panel')).toBeInTheDocument()
  })

  it('renders Files tab when clicked', () => {
    render(<SidebarTabs {...baseProps} />)
    fireEvent.click(screen.getByRole('tab', { name: /files/i }))
    expect(screen.getByTestId('file-explorer')).toBeInTheDocument()
  })

  it('renders Git tab when clicked', () => {
    render(<SidebarTabs {...baseProps} />)
    fireEvent.click(screen.getByRole('tab', { name: /git/i }))
    expect(screen.getByTestId('git-view')).toBeInTheDocument()
  })

  it('shows three tab triggers', () => {
    render(<SidebarTabs {...baseProps} />)
    expect(screen.getAllByRole('tab').length).toBe(3)
  })
})
```

- [ ] **Step 2: Run test — expect FAIL**
```bash
cd web && npx vitest run src/__tests__/components/layout/SidebarTabs.test.tsx 2>&1 | tail -5
```

- [ ] **Step 3: Implement SidebarTabs.tsx**

```tsx
// web/src/components/layout/SidebarTabs.tsx
import { useState } from 'react'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { WorkspacesSidebarPanel } from './WorkspacesSidebarPanel'
import { FileExplorerTree } from '@/features/file-explorer/components/file-explorer-tree'
import { GitView } from '@/features/git/components/git-view'
import type { ProjectChat, Repo } from '@/lib/store/sidebar'

type SidebarTab = 'workspaces' | 'files' | 'git'

interface SidebarTabsProps {
  userInitials: string
  chats: ProjectChat[]
  repos: Repo[]
  collapsedRepos?: Set<string>
  activeChatId?: string
  activeWorkspaceId?: string
  activeWorkspaceRepoPath: string
  onChatClick?: (id: string) => void
  onWorkspaceClick?: (repoId: string, workspaceId: string) => void
  onNewChat?: () => void
  onNewWorkspace?: () => void
  onDeleteChat?: (id: string) => void
  onDeleteWorkspace?: (wsId: string) => void
  onRepoToggle?: (repoId: string) => void
  onProjectsClick?: () => void
  onProjectSelect?: (id: string) => void
  onSettingsOpen?: () => void
}

export function SidebarTabs({
  userInitials,
  chats,
  repos,
  collapsedRepos,
  activeChatId,
  activeWorkspaceId,
  activeWorkspaceRepoPath,
  onChatClick,
  onWorkspaceClick,
  onNewChat,
  onNewWorkspace,
  onDeleteChat,
  onDeleteWorkspace,
  onRepoToggle,
  onProjectsClick,
  onProjectSelect,
  onSettingsOpen,
}: SidebarTabsProps) {
  const [activeTab, setActiveTab] = useState<SidebarTab>('workspaces')

  return (
    <Tabs
      value={activeTab}
      onValueChange={(v) => setActiveTab(v as SidebarTab)}
      className="flex flex-1 flex-col overflow-hidden"
    >
      <TabsList className="h-9 w-full justify-start gap-0 rounded-none border-b border-border bg-card px-0">
        <TabsTrigger
          value="workspaces"
          className="flex-1 rounded-none border-b-2 border-transparent py-2 text-[11px] tracking-wide uppercase data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:shadow-none"
        >
          Workspaces
        </TabsTrigger>
        <TabsTrigger
          value="files"
          className="flex-1 rounded-none border-b-2 border-transparent py-2 text-[11px] tracking-wide uppercase data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:shadow-none"
        >
          Files
        </TabsTrigger>
        <TabsTrigger
          value="git"
          className="flex-1 rounded-none border-b-2 border-transparent py-2 text-[11px] tracking-wide uppercase data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:shadow-none"
        >
          Git
        </TabsTrigger>
      </TabsList>

      <TabsContent value="workspaces" className="flex flex-1 flex-col overflow-hidden mt-0">
        <WorkspacesSidebarPanel
          userInitials={userInitials}
          chats={chats}
          repos={repos}
          collapsedRepos={collapsedRepos}
          activeChatId={activeChatId}
          activeWorkspaceId={activeWorkspaceId}
          onChatClick={onChatClick}
          onWorkspaceClick={onWorkspaceClick}
          onNewChat={onNewChat}
          onNewWorkspace={onNewWorkspace}
          onDeleteChat={onDeleteChat}
          onDeleteWorkspace={onDeleteWorkspace}
          onRepoToggle={onRepoToggle}
          onProjectsClick={onProjectsClick}
          onProjectSelect={onProjectSelect}
          onSettingsClick={onSettingsOpen}
        />
      </TabsContent>

      <TabsContent value="files" className="flex flex-1 flex-col overflow-hidden mt-0">
        <FileExplorerTree rootPath={activeWorkspaceRepoPath} />
      </TabsContent>

      <TabsContent value="git" className="flex flex-1 flex-col overflow-hidden mt-0">
        <GitView repoPath={activeWorkspaceRepoPath} />
      </TabsContent>
    </Tabs>
  )
}
```

- [ ] **Step 4: Run tests — expect PASS**
```bash
cd web && npx vitest run src/__tests__/components/layout/SidebarTabs.test.tsx
```

- [ ] **Step 5: Commit**
```bash
git add web/src/components/layout/SidebarTabs.tsx web/src/__tests__/components/layout/SidebarTabs.test.tsx
git commit -m "feat(web): add SidebarTabs (Workspaces/Files/Git)"
```

---

### Task 21: Create FlowTab.tsx

**Files:**
- Create: `web/src/components/layout/FlowTab.tsx`
- Create: `web/src/__tests__/components/layout/FlowTab.test.tsx`

- [ ] **Step 1: Write the test**

```tsx
// web/src/__tests__/components/layout/FlowTab.test.tsx
import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { FlowTab } from '@/components/layout/FlowTab'

vi.mock('@tanstack/react-router', () => ({
  Outlet: () => <div data-testid="outlet-content">Chat content</div>,
  useRouterState: () => ({ location: { pathname: '/workspaces/ws1/brainstorming' } }),
}))
vi.mock('@/lib/queries', () => ({
  workspaceQueryOptions: (id: string) => ({ queryKey: ['workspace', id], queryFn: () => null }),
}))
vi.mock('@tanstack/react-query', () => ({
  useQuery: () => ({ data: { flow: { states: [{ name: 'brainstorming', label: 'Brainstorm', ui: 'chat' }] }, currentState: 'brainstorming' } }),
}))
vi.mock('@/components/layout/WorkspaceStepTabs', () => ({
  WorkspaceStepTabs: ({ states }: { states: unknown[] }) => (
    <div data-testid="step-tabs">{states.length} steps</div>
  ),
}))

describe('FlowTab', () => {
  it('renders outlet content', () => {
    render(<FlowTab workspaceId="ws1" />)
    expect(screen.getByTestId('outlet-content')).toBeInTheDocument()
  })

  it('renders WorkspaceStepTabs below outlet content', () => {
    render(<FlowTab workspaceId="ws1" />)
    expect(screen.getByTestId('step-tabs')).toBeInTheDocument()
  })

  it('WorkspaceStepTabs comes after outlet in the DOM', () => {
    const { container } = render(<FlowTab workspaceId="ws1" />)
    const outlet = container.querySelector('[data-testid="outlet-content"]')
    const tabs = container.querySelector('[data-testid="step-tabs"]')
    expect(outlet!.compareDocumentPosition(tabs!)).toBe(Node.DOCUMENT_POSITION_FOLLOWING)
  })
})
```

- [ ] **Step 2: Run test — expect FAIL**
```bash
cd web && npx vitest run src/__tests__/components/layout/FlowTab.test.tsx 2>&1 | tail -5
```

- [ ] **Step 3: Implement FlowTab.tsx**

```tsx
// web/src/components/layout/FlowTab.tsx
import { Outlet, useNavigate, useRouterState } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { workspaceQueryOptions } from '@/lib/queries'
import { WorkspaceStepTabs } from './WorkspaceStepTabs'

interface FlowTabProps {
  workspaceId: string
}

export function FlowTab({ workspaceId }: FlowTabProps) {
  const navigate = useNavigate()
  const pathname = useRouterState({ select: s => s.location.pathname })
  const { data: workspace } = useQuery(workspaceQueryOptions(workspaceId))

  const currentStep = pathname.split('/').pop() ?? workspace?.currentState ?? ''

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      {/* Route content — ChatView or DiffView depending on flow phase */}
      <div className="flex flex-1 flex-col overflow-hidden">
        <Outlet />
      </div>

      {/* Pinned bottom — ChatInput is inside Outlet (ChatView), WorkspaceStepTabs below */}
      {workspace && (
        <div className="border-t border-border">
          <WorkspaceStepTabs
            states={workspace.flow.states}
            currentStep={currentStep}
            onStepChange={(step) =>
              navigate({ to: '/workspaces/$wsId/$step', params: { wsId: workspaceId, step } })
            }
          />
        </div>
      )}
    </div>
  )
}
```

Wait — the ChatInput is _inside_ ChatView (which is inside Outlet). The WorkspaceStepTabs needs to be below ChatInput, meaning it should be _below_ the Outlet. That's exactly what the code above does — `<Outlet />` renders ChatView (which contains ChatInput), and WorkspaceStepTabs is rendered after it.

- [ ] **Step 4: Run tests — expect PASS**
```bash
cd web && npx vitest run src/__tests__/components/layout/FlowTab.test.tsx
```

- [ ] **Step 5: Commit**
```bash
git add web/src/components/layout/FlowTab.tsx web/src/__tests__/components/layout/FlowTab.test.tsx
git commit -m "feat(web): add FlowTab — locked flow tab with WorkspaceStepTabs at bottom"
```

---

### Task 22: Create IDEShell.tsx

**Files:**
- Create: `web/src/components/layout/IDEShell.tsx`
- Create: `web/src/__tests__/components/layout/IDEShell.test.tsx`

- [ ] **Step 1: Write the test**

```tsx
// web/src/__tests__/components/layout/IDEShell.test.tsx
import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { IDEShell } from '@/components/layout/IDEShell'

vi.mock('@/features/layout/sidebar/main-sidebar', () => ({
  MainSidebar: ({ children }: { children: React.ReactNode }) => (
    <aside data-testid="main-sidebar">{children}</aside>
  ),
}))
vi.mock('@/features/panes/components/split-view-root', () => ({
  SplitViewRoot: () => <div data-testid="split-view-root" />,
}))
vi.mock('@/components/layout/SidebarTabs', () => ({
  SidebarTabs: () => <div data-testid="sidebar-tabs" />,
}))
vi.mock('@/lib/store/sidebar', () => ({
  useSidebarStore: () => ({
    chats: [], repos: [], collapsedRepos: new Set(),
    addChat: vi.fn(), deleteChat: vi.fn(), deleteWorkspace: vi.fn(), toggleRepo: vi.fn(),
  }),
}))

describe('IDEShell', () => {
  it('renders MainSidebar', () => {
    render(<IDEShell />)
    expect(screen.getByTestId('main-sidebar')).toBeInTheDocument()
  })

  it('renders SplitViewRoot', () => {
    render(<IDEShell />)
    expect(screen.getByTestId('split-view-root')).toBeInTheDocument()
  })

  it('renders SidebarTabs inside MainSidebar', () => {
    render(<IDEShell />)
    const sidebar = screen.getByTestId('main-sidebar')
    expect(sidebar.querySelector('[data-testid="sidebar-tabs"]')).toBeTruthy()
  })
})
```

- [ ] **Step 2: Run test — expect FAIL**
```bash
cd web && npx vitest run src/__tests__/components/layout/IDEShell.test.tsx 2>&1 | tail -5
```

- [ ] **Step 3: Implement IDEShell.tsx**

```tsx
// web/src/components/layout/IDEShell.tsx
import { useNavigate, useRouterState } from '@tanstack/react-router'
import { MainSidebar } from '@/features/layout/sidebar/main-sidebar'
import { SplitViewRoot } from '@/features/panes/components/split-view-root'
import { SidebarTabs } from './SidebarTabs'
import { useSidebarStore } from '@/lib/store/sidebar'
import { createMockChat, deleteMockChat } from '@/lib/mock/chats'
import { deleteMockWorkspace } from '@/lib/mock/workspaces'

export function IDEShell() {
  const navigate = useNavigate()
  const pathname = useRouterState({ select: s => s.location.pathname })
  const { chats, repos, collapsedRepos, addChat, deleteChat, deleteWorkspace, toggleRepo } =
    useSidebarStore()

  const activeWorkspaceId = pathname.match(/\/workspaces\/([^/]+)/)?.[1]
  const activeChatId = pathname.match(/\/chat\/([^/]+)/)?.[1]

  // Derive the repo path for the active workspace — used to sync Files + Git tabs
  const activeRepo = repos.find(r => r.workspaces.some(ws => ws.id === activeWorkspaceId))
  const activeWorkspaceRepoPath = activeRepo ? `/repos/${activeRepo.id}` : '/repos/default'

  const handleNewChat = () => {
    const chat = createMockChat()
    addChat({ id: chat.id, title: chat.title, age: chat.age })
    navigate({ to: '/chat/$chatId', params: { chatId: chat.id } })
  }

  const handleDeleteChat = (id: string) => {
    deleteMockChat(id)
    deleteChat(id)
    if (activeChatId === id) {
      const remaining = chats.filter(c => c.id !== id)
      remaining.length > 0
        ? navigate({ to: '/chat/$chatId', params: { chatId: remaining[0].id } })
        : navigate({ to: '/' })
    }
  }

  const handleDeleteWorkspace = (wsId: string) => {
    deleteMockWorkspace(wsId)
    deleteWorkspace(wsId)
    if (activeWorkspaceId === wsId) navigate({ to: '/' })
  }

  return (
    <div className="flex h-screen overflow-hidden bg-background text-foreground">
      <MainSidebar>
        <SidebarTabs
          userInitials="MU"
          chats={chats}
          repos={repos}
          collapsedRepos={collapsedRepos}
          activeChatId={activeChatId}
          activeWorkspaceId={activeWorkspaceId}
          activeWorkspaceRepoPath={activeWorkspaceRepoPath}
          onChatClick={(id) => navigate({ to: '/chat/$chatId', params: { chatId: id } })}
          onWorkspaceClick={(_, wsId) => navigate({ to: '/workspaces/$wsId', params: { wsId } })}
          onNewChat={handleNewChat}
          onNewWorkspace={() => navigate({ to: '/workspaces/new' })}
          onDeleteChat={handleDeleteChat}
          onDeleteWorkspace={handleDeleteWorkspace}
          onRepoToggle={toggleRepo}
          onProjectsClick={() => navigate({ to: '/projects' })}
          onProjectSelect={() => navigate({ to: '/' })}
        />
      </MainSidebar>
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden border-l border-border">
        <SplitViewRoot />
      </div>
    </div>
  )
}
```

- [ ] **Step 4: Run tests — expect PASS**
```bash
cd web && npx vitest run src/__tests__/components/layout/IDEShell.test.tsx
```

- [ ] **Step 5: Commit**
```bash
git add web/src/components/layout/IDEShell.tsx web/src/__tests__/components/layout/IDEShell.test.tsx
git commit -m "feat(web): add IDEShell root layout"
```

---

## Wave 5 — Route Wiring

### Task 23: Update routes and delete replaced files

**Files:**
- Modify: `web/src/routes/__root.tsx`
- Modify: `web/src/routes/workspaces/$wsId.tsx`
- Delete: `web/src/components/layout/AppShell.tsx`
- Delete: `web/src/components/layout/Sidebar.tsx`
- Delete: `web/src/hooks/useSidebarWidth.ts`

- [ ] **Step 1: Update __root.tsx**

Replace the entire content of `web/src/routes/__root.tsx`:

```tsx
// web/src/routes/__root.tsx
import { createRootRoute, Outlet } from '@tanstack/react-router'
import { IDEShell } from '@/components/layout/IDEShell'
import { ErrorBoundary } from '@/components/ErrorBoundary'

function RootLayout() {
  return (
    <ErrorBoundary>
      <IDEShell />
    </ErrorBoundary>
  )
}

export const Route = createRootRoute({ component: RootLayout })
```

Wait — `IDEShell` contains `SplitViewRoot` from Athas's panes system. But the existing routes render their content into `<Outlet />`. With the Athas pane system, route content doesn't render into a simple `<Outlet />` — it renders into the active pane via the buffer/pane store.

This requires a bridge: the Flow tab (`FlowTab`) needs to render the current workspace route's `<Outlet />` inside it. Let me think about how routing works with the panes system.

In Athas, the pane system manages which "buffer" (file or special view) is active in each pane. File content is loaded into Monaco via the buffer store, not via router. Special views (like our Flow tab) are a special buffer type.

The correct approach is: the FlowTab is a special buffer registered in the pane store. When a workspace is navigated to, the pane store shows the FlowTab buffer. When files are opened from the file explorer, they add file buffers to the active pane.

To make this work cleanly with TanStack Router, we keep `<Outlet />` rendering in the FlowTab buffer, and have the pane store's initial state include a FlowTab buffer as the first pane.

For the root route, the `<Outlet />` needs to be rendered somewhere. We put it in FlowTab. `IDEShell` replaces `AppShell` as the root layout but does NOT render `<Outlet />` directly — `FlowTab` does that.

Update `__root.tsx`:
```tsx
// web/src/routes/__root.tsx
import { createRootRoute } from '@tanstack/react-router'
import { IDEShell } from '@/components/layout/IDEShell'
import { ErrorBoundary } from '@/components/ErrorBoundary'

export const Route = createRootRoute({
  component: () => (
    <ErrorBoundary>
      <IDEShell />
    </ErrorBoundary>
  ),
})
```

- [ ] **Step 2: Update WorkspaceLayout route ($wsId.tsx)**

The `WorkspaceLayout` no longer wraps content in `WorkspaceStepTabs` — that lives in `FlowTab`. Replace the entire file content:

```tsx
// web/src/routes/workspaces/$wsId.tsx
import { createFileRoute, Outlet } from '@tanstack/react-router'

export const Route = createFileRoute('/workspaces/$wsId')({
  component: () => <Outlet />,
})
```

`WorkspaceStepTabs` is now rendered by `FlowTab` instead.

- [ ] **Step 3: Delete replaced files**
```bash
rm web/src/components/layout/AppShell.tsx
rm web/src/components/layout/Sidebar.tsx
rm web/src/hooks/useSidebarWidth.ts
```

- [ ] **Step 4: TypeScript check — full project**
```bash
cd web && npx tsc --noEmit 2>&1 | head -40
```
Fix any remaining errors. Common issues: leftover imports of `AppShell`, `Sidebar`, or `useSidebarWidth`. Search for them:
```bash
grep -r "AppShell\|from.*Sidebar'\|useSidebarWidth" web/src/ --include="*.ts" --include="*.tsx" | grep -v "WorkspacesSidebarPanel\|SidebarTabs\|SidebarHeader\|SidebarRow\|SidebarSkeleton"
```

- [ ] **Step 5: Commit**
```bash
git add -A
git commit -m "feat(web): wire IDEShell into root route, simplify workspace layout"
```

---

## Wave 6 — Settings Trigger

### Task 24: Add settings trigger to SidebarHeader

The settings dialog trigger needs a home since we dropped `features/window/` (Athas's title bar). It goes in `SidebarHeader`.

**Files:**
- Modify: `web/src/components/layout/SidebarHeader.tsx`
- Modify: `web/src/components/layout/IDEShell.tsx` (wire settings open state)

- [ ] **Step 1: Update test for SidebarHeader**

The existing test at `web/src/__tests__/components/layout/SidebarHeader.test.tsx` should be updated to verify the settings button renders. Read the existing test and add:

```tsx
it('renders a settings button', () => {
  render(
    <SidebarHeader
      userInitials="MU"
      onSettingsClick={vi.fn()}
    />
  )
  expect(screen.getByRole('button', { name: /settings/i })).toBeInTheDocument()
})

it('calls onSettingsClick when settings button is clicked', () => {
  const onSettingsClick = vi.fn()
  render(<SidebarHeader userInitials="MU" onSettingsClick={onSettingsClick} />)
  fireEvent.click(screen.getByRole('button', { name: /settings/i }))
  expect(onSettingsClick).toHaveBeenCalled()
})
```

- [ ] **Step 2: Run test — expect FAIL (settings button not yet there)**
```bash
cd web && npx vitest run src/__tests__/components/layout/SidebarHeader.test.tsx 2>&1 | tail -10
```

- [ ] **Step 3: Update SidebarHeader.tsx**

Read the current `web/src/components/layout/SidebarHeader.tsx` and add an `onSettingsClick` prop and a settings icon button. Import the gear icon from `@phosphor-icons/react`:

Add to props interface:
```ts
onSettingsClick?: () => void
```

Add a settings button to the header JSX (place it near the user avatar, right side of the header):
```tsx
import { GearSix } from '@phosphor-icons/react'

// In the JSX, add beside or near the user avatar:
<button
  onClick={onSettingsClick}
  aria-label="Settings"
  className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
>
  <GearSix size={16} />
</button>
```

- [ ] **Step 4: Wire settings open state in IDEShell.tsx**

Add settings dialog state to `IDEShell.tsx`:

```tsx
import { useState } from 'react'
import { SettingsDialog } from '@/features/settings/components/settings-dialog'

// Inside IDEShell:
const [settingsOpen, setSettingsOpen] = useState(false)

// Pass to SidebarTabs:
onSettingsOpen={() => setSettingsOpen(true)}

// Add at the bottom of the return:
<SettingsDialog isOpen={settingsOpen} onClose={() => setSettingsOpen(false)} />
```

- [ ] **Step 5: Run tests — expect PASS**
```bash
cd web && npx vitest run src/__tests__/components/layout/SidebarHeader.test.tsx
```

- [ ] **Step 6: TypeScript check**
```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

- [ ] **Step 7: Commit**
```bash
git add web/src/components/layout/SidebarHeader.tsx web/src/components/layout/IDEShell.tsx
git commit -m "feat(web): add settings trigger to SidebarHeader, wire SettingsDialog in IDEShell"
```

---

## Wave 7 — Error Boundaries and Loading States

### Task 25: Add error boundaries and loading states to IDE features

**Files:**
- Modify: `web/src/components/layout/SidebarTabs.tsx` (loading + error states for Files and Git tabs)
- Modify: `web/src/components/layout/IDEShell.tsx` (error boundary around SplitViewRoot)

- [ ] **Step 1: Add Suspense + ErrorBoundary to SidebarTabs.tsx**

Wrap `FileExplorerTree` and `GitView` in `<Suspense>` with skeleton fallbacks and `<ErrorBoundary>` wrappers in `SidebarTabs.tsx`:

```tsx
import { Suspense } from 'react'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { SidebarSkeleton } from './SidebarSkeleton'

// In the Files tab content:
<TabsContent value="files" className="flex flex-1 flex-col overflow-hidden mt-0">
  <ErrorBoundary>
    <Suspense fallback={<SidebarSkeleton />}>
      <FileExplorerTree rootPath={activeWorkspaceRepoPath} />
    </Suspense>
  </ErrorBoundary>
</TabsContent>

// In the Git tab content:
<TabsContent value="git" className="flex flex-1 flex-col overflow-hidden mt-0">
  <ErrorBoundary>
    <Suspense fallback={<SidebarSkeleton />}>
      <GitView repoPath={activeWorkspaceRepoPath} />
    </Suspense>
  </ErrorBoundary>
</TabsContent>
```

- [ ] **Step 2: Add ErrorBoundary around SplitViewRoot in IDEShell.tsx**

```tsx
// In IDEShell.tsx, wrap the main area:
<div className="flex min-w-0 flex-1 flex-col overflow-hidden border-l border-border">
  <ErrorBoundary>
    <SplitViewRoot />
  </ErrorBoundary>
</div>
```

- [ ] **Step 3: TypeScript check**
```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

- [ ] **Step 4: Commit**
```bash
git add web/src/components/layout/SidebarTabs.tsx web/src/components/layout/IDEShell.tsx
git commit -m "feat(web): add error boundaries and loading states to IDE shell"
```

---

## Wave 8 — Final Verification

### Task 26: Full test suite pass and coverage check

**Files:**
- No new files — run and fix only

- [ ] **Step 1: Run full test suite**
```bash
cd web && npx vitest run --reporter=verbose 2>&1 | tail -30
```
Expected: all tests pass.

- [ ] **Step 2: Check coverage**
```bash
cd web && npx vitest run --coverage 2>&1 | tail -20
```
Expected: overall coverage ≥ 95%. If below 95%, identify which new files are uncovered and add tests for them.

- [ ] **Step 3: TypeScript strict check — full project**
```bash
cd web && npx tsc --noEmit 2>&1
```
Expected: no errors, no output.

- [ ] **Step 4: Check for console.log artifacts**
```bash
grep -r "console\.log\|console\.error\|console\.warn" web/src/features/ --include="*.ts" --include="*.tsx" | grep -v "ErrorBoundary\|console\.warn.*matchMedia" | head -20
```
Remove any found (except legitimate error boundary catches).

- [ ] **Step 5: Check for any remaining Tauri imports**
```bash
grep -r "tauri-apps" web/src/ --include="*.ts" --include="*.tsx"
```
Expected: no output. If any found, replace with crowbar-bridge equivalents.

- [ ] **Step 6: Lint**
```bash
cd web && npx eslint src/ --max-warnings=0 2>&1 | tail -10
```
Fix any lint errors.

- [ ] **Step 7: Final commit**
```bash
git add -A
git commit -m "feat(web): Athas UI integration — production-ready IDE shell with mock data

- Copied Athas feature modules: panes, tabs, editor, file-explorer, terminal, git, settings, command-palette, layout
- Replaced all Tauri invoke() calls with crowbar-bridge stubs (FUTURE: real Go API)
- Mock data layer for file tree, git status, branches, commit history, terminal
- New glue components: IDEShell, SidebarTabs, WorkspacesSidebarPanel, FlowTab
- WorkspaceStepTabs moved to bottom of Flow tab, below ChatInput
- Settings modal trimmed to 7 relevant panels
- All tests pass, TypeScript strict, ≥95% coverage"
```
