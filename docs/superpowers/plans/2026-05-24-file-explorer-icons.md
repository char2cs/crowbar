# File Explorer Icons Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port Athas's 6 builtin icon themes and real icon registry into Crowbar so the file explorer shows proper file-type icons instead of generic Phosphor fallbacks.

**Architecture:** The rendering infrastructure (`FileExplorerIcon`, icon theme store, settings UI) already works correctly. The `IconThemeRegistry` is a stub — replace it with the real Athas implementation, port the 6 builtin theme files, and wire the initializer at app startup.

**Tech Stack:** React 18, TypeScript, `material-file-icons` npm package, Phosphor Icons (already installed), DOMPurify (already installed)

**Source:** `/Users/char2cs/Projects/Cloned/athas/src/extensions/icon-themes/`

---

## File Map

### Created
```
web/src/extensions/icon-themes/icon-theme-initializer.ts
web/src/extensions/icon-themes/builtin/manifest.json
web/src/extensions/icon-themes/builtin/material-theme.tsx
web/src/extensions/icon-themes/builtin/colorful-material-theme.tsx
web/src/extensions/icon-themes/builtin/minimal-theme.tsx
web/src/extensions/icon-themes/builtin/classic-theme.tsx
web/src/extensions/icon-themes/builtin/compact-theme.tsx
web/src/extensions/icon-themes/builtin/none-theme.tsx
web/src/extensions/bundled/icon-themes/minimal/icons/file.svg
web/src/extensions/bundled/icon-themes/minimal/icons/folder.svg
web/src/extensions/bundled/icon-themes/minimal/icons/folder-open.svg
```

### Replaced (stub → real)
```
web/src/extensions/icon-themes/icon-theme-registry.ts
```

### Modified
```
web/src/main.tsx
```

### Deleted
```
web/src/features/file-explorer/components/file-explorer-icon.tsx   (stub with "will be replaced" comment)
```

---

### Task 1: Install Dependency

- [ ] **Step 1: Install material-file-icons**

```bash
cd web && npm install material-file-icons
```

- [ ] **Step 2: Verify installation**

```bash
node -e "const m = require('material-file-icons'); console.log(typeof m.getIcon)"
```

Expected: `function`

- [ ] **Step 3: Commit**

```bash
git add web/package.json web/package-lock.json
git commit -m "feat(icons): install material-file-icons"
```

---

### Task 2: Port Icon Theme Registry

Replace the stub at `web/src/extensions/icon-themes/icon-theme-registry.ts` with the real implementation from Athas.

**Files:**
- Replace: `web/src/extensions/icon-themes/icon-theme-registry.ts`

- [ ] **Step 1: Read the current stub**

Open `web/src/extensions/icon-themes/icon-theme-registry.ts` and note what the `IconThemeDefinition` interface expects. Keep the same interface — only the implementation changes.

- [ ] **Step 2: Read the Athas source**

Open `/Users/char2cs/Projects/Cloned/athas/src/extensions/icon-themes/icon-theme-registry.ts` and copy its implementation verbatim into the Crowbar file. Adjust import paths as needed (Athas uses `@/` aliases that map to its own src — verify they match Crowbar's tsconfig paths).

Common import path differences to fix:
- Athas `@/extensions/...` → Crowbar `@/extensions/...` (likely identical)
- Athas `@/utils/...` → verify the util exists in Crowbar at the same path

- [ ] **Step 3: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit 2>&1 | grep icon-theme-registry
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add web/src/extensions/icon-themes/icon-theme-registry.ts
git commit -m "feat(icons): replace icon theme registry stub with real implementation"
```

---

### Task 3: Port Builtin Theme Files

Copy all 6 builtin theme files from Athas. These implement `IconThemeDefinition` and call `getIcon()` from `material-file-icons` or return Phosphor icon components.

**Files:**
- Create: `web/src/extensions/icon-themes/builtin/material-theme.tsx`
- Create: `web/src/extensions/icon-themes/builtin/colorful-material-theme.tsx`
- Create: `web/src/extensions/icon-themes/builtin/minimal-theme.tsx`
- Create: `web/src/extensions/icon-themes/builtin/classic-theme.tsx`
- Create: `web/src/extensions/icon-themes/builtin/compact-theme.tsx`
- Create: `web/src/extensions/icon-themes/builtin/none-theme.tsx`
- Create: `web/src/extensions/icon-themes/builtin/manifest.json`

- [ ] **Step 1: Copy all builtin theme files from Athas**

```bash
cp /Users/char2cs/Projects/Cloned/athas/src/extensions/icon-themes/builtin/material-theme.tsx \
   web/src/extensions/icon-themes/builtin/

cp /Users/char2cs/Projects/Cloned/athas/src/extensions/icon-themes/builtin/colorful-material-theme.tsx \
   web/src/extensions/icon-themes/builtin/

cp /Users/char2cs/Projects/Cloned/athas/src/extensions/icon-themes/builtin/minimal-theme.tsx \
   web/src/extensions/icon-themes/builtin/

cp /Users/char2cs/Projects/Cloned/athas/src/extensions/icon-themes/builtin/classic-theme.tsx \
   web/src/extensions/icon-themes/builtin/

cp /Users/char2cs/Projects/Cloned/athas/src/extensions/icon-themes/builtin/compact-theme.tsx \
   web/src/extensions/icon-themes/builtin/

cp /Users/char2cs/Projects/Cloned/athas/src/extensions/icon-themes/builtin/none-theme.tsx \
   web/src/extensions/icon-themes/builtin/

cp /Users/char2cs/Projects/Cloned/athas/src/extensions/icon-themes/builtin/manifest.json \
   web/src/extensions/icon-themes/builtin/
```

- [ ] **Step 2: Copy minimal theme SVG assets**

```bash
mkdir -p web/src/extensions/bundled/icon-themes/minimal/icons

cp /Users/char2cs/Projects/Cloned/athas/src/extensions/bundled/icon-themes/minimal/icons/file.svg \
   web/src/extensions/bundled/icon-themes/minimal/icons/

cp /Users/char2cs/Projects/Cloned/athas/src/extensions/bundled/icon-themes/minimal/icons/folder.svg \
   web/src/extensions/bundled/icon-themes/minimal/icons/

cp /Users/char2cs/Projects/Cloned/athas/src/extensions/bundled/icon-themes/minimal/icons/folder-open.svg \
   web/src/extensions/bundled/icon-themes/minimal/icons/
```

- [ ] **Step 3: Fix import paths in copied files**

Each copied file may reference Athas-specific paths. Scan for imports that don't resolve:

```bash
cd web && npx tsc --noEmit 2>&1 | grep "icon-themes/builtin"
```

For each error, fix the import path. Most common issues:
- `import { getIcon } from 'material-file-icons'` — correct, no change needed
- References to Athas internal utils — swap for the equivalent Crowbar path or inline the helper

- [ ] **Step 4: Verify all 6 theme files compile**

```bash
cd web && npx tsc --noEmit 2>&1 | grep -E "(material-theme|colorful|minimal|classic|compact|none-theme)"
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/extensions/icon-themes/builtin/ \
        web/src/extensions/bundled/
git commit -m "feat(icons): port 6 builtin icon themes from Athas"
```

---

### Task 4: Port Icon Theme Initializer

**Files:**
- Create: `web/src/extensions/icon-themes/icon-theme-initializer.ts`

- [ ] **Step 1: Copy from Athas and fix paths**

```bash
cp /Users/char2cs/Projects/Cloned/athas/src/extensions/icon-themes/icon-theme-initializer.ts \
   web/src/extensions/icon-themes/
```

Open the file and verify:
1. It imports each of the 6 builtin theme files
2. It calls `iconThemeRegistry.registerTheme(...)` for each
3. All import paths resolve in Crowbar's directory structure

Fix any broken paths. The file should look similar to:

```typescript
// web/src/extensions/icon-themes/icon-theme-initializer.ts
import { iconThemeRegistry } from './icon-theme-registry'
import { materialTheme } from './builtin/material-theme'
import { colorfulMaterialTheme } from './builtin/colorful-material-theme'
import { minimalTheme } from './builtin/minimal-theme'
import { classicTheme } from './builtin/classic-theme'
import { compactTheme } from './builtin/compact-theme'
import { noneTheme } from './builtin/none-theme'

export function initializeIconThemes(): void {
  iconThemeRegistry.registerTheme(colorfulMaterialTheme)
  iconThemeRegistry.registerTheme(materialTheme)
  iconThemeRegistry.registerTheme(minimalTheme)
  iconThemeRegistry.registerTheme(classicTheme)
  iconThemeRegistry.registerTheme(compactTheme)
  iconThemeRegistry.registerTheme(noneTheme)
}
```

(Exact export names and function signature may differ — match what the Athas file actually exports.)

- [ ] **Step 2: Verify it compiles**

```bash
cd web && npx tsc --noEmit 2>&1 | grep icon-theme-initializer
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/extensions/icon-themes/icon-theme-initializer.ts
git commit -m "feat(icons): port icon theme initializer"
```

---

### Task 5: Wire Initializer at App Startup

**Files:**
- Modify: `web/src/main.tsx`

- [ ] **Step 1: Call initializeIconThemes before React mounts**

Open `web/src/main.tsx`. Add the import and call it before `ReactDOM.createRoot`:

```typescript
import { initializeIconThemes } from '@/extensions/icon-themes/icon-theme-initializer'

// Called before React mounts — themes are available synchronously
initializeIconThemes()

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
)
```

- [ ] **Step 2: Delete the stub file-explorer-icon.tsx**

```bash
rm web/src/features/file-explorer/components/file-explorer-icon.tsx
```

Verify no other file imports from this path:
```bash
grep -rn "file-explorer/components/file-explorer-icon" web/src --include="*.ts" --include="*.tsx"
```

Expected: zero matches.

- [ ] **Step 3: Verify TypeScript clean**

```bash
cd web && npx tsc --noEmit 2>&1
```

Expected: zero errors.

- [ ] **Step 4: Manual smoke test**

```bash
cd web && npm run dev
```

1. Open the app and navigate to a workspace
2. Open the file explorer in the sidebar
3. File icons should now be Material Design icons (colourful by default), not generic grey file/folder icons
4. Open Settings → Appearance → confirm icon theme selector shows the 6 theme options
5. Switch to "Material (Monochrome)" — icons should update to monochrome style
6. Switch to "None" — falls back to generic Phosphor icons

- [ ] **Step 5: Commit**

```bash
git add web/src/main.tsx
git add -A  # picks up deleted stub
git commit -m "feat(icons): wire icon theme initializer at app startup"
```

---

### Task 6: Run All Tests

- [ ] **Step 1: Full test suite**

```bash
cd web && npx vitest run
```

Expected: all tests pass. The icon theme system has no unit tests (it's integration-level rendering), so this confirms nothing was broken.

- [ ] **Step 2: Final type-check**

```bash
cd web && npx tsc --noEmit
```

Expected: zero errors.

- [ ] **Step 3: Final commit**

```bash
git add -A
git commit -m "feat(icons): file explorer icon themes complete"
```
