# Settings Panel Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Clean up the settings panel by removing dead settings inherited from the vendored editor, porting the full icon theme system (6 themes), replacing the dual theme picker with a single Color Theme + Theme Mode design, fixing the modal shell bugs, and wiring sidebar position to the layout.

**Architecture:** Work flows from foundational (types, icon registry) → data layer (initializer, store wiring) → UI layer (tab cleanup, appearance tab rewrite, modal fixes). Each task is independently committable and leaves the app in a working state.

**Tech Stack:** React, TypeScript, Zustand, Tailwind CSS, Shadcn/ui, @phosphor-icons/react, material-file-icons (already installed), Monaco Editor.

---

## File Map

**Modified:**
- `web/src/extensions/icon-themes/types.ts` — add `IconThemeSource` interface
- `web/src/extensions/icon-themes/icon-theme-registry.ts` — replace stub with full implementation
- `web/src/features/settings/types/settings.ts` — add `themeMode` field
- `web/src/features/settings/config/default-settings.ts` — add `themeMode` default
- `web/src/features/settings/lib/settings-effects.ts` — handle `themeMode`, remove `syncSystemTheme` logic
- `web/src/features/settings/lib/settings-normalization.ts` — handle missing `themeMode` on old saved data
- `web/src/features/window/stores/ui-state-store.ts` — remove `"general"` + `"keyboard"` from `SettingsTab`, default tab to `"appearance"`
- `web/src/features/settings/components/settings-vertical-tabs.tsx` — remove General + Keybindings from `SETTINGS_TAB_ITEMS`
- `web/src/features/settings/components/settings-dialog.tsx` — remove General/Keyboard cases from switch
- `web/src/features/settings/components/tabs/appearance-settings.tsx` — rewrite theme section, remove dead layout settings
- `web/src/features/settings/components/tabs/editor-settings.tsx` — remove Editor Engine `SettingRow`
- `web/src/features/file-explorer/components/file-explorer-icon.tsx` — wire to iconThemeRegistry
- `web/src/components/layout/IDEShell.tsx` — read `sidebarPosition` setting, flip layout
- `web/src/main.tsx` — call `initializeIconThemes()` on startup

**Created:**
- `web/src/extensions/icon-themes/builtin/classic-theme.tsx`
- `web/src/extensions/icon-themes/builtin/material-theme.tsx`
- `web/src/extensions/icon-themes/builtin/colorful-material-theme.tsx`
- `web/src/extensions/icon-themes/builtin/compact-theme.tsx`
- `web/src/extensions/icon-themes/builtin/minimal-theme.tsx`
- `web/src/extensions/icon-themes/builtin/none-theme.tsx`
- `web/src/extensions/icon-themes/icon-theme-initializer.ts`

---

### Task 1: Icon theme types — add `IconThemeSource`

**Files:**
- Modify: `web/src/extensions/icon-themes/types.ts`

- [ ] **Step 1: Replace the file content**

```typescript
// web/src/extensions/icon-themes/types.ts
import type React from "react"

export interface FileIconResult {
  svg?: string | null
  component?: React.ReactNode | null
}

export interface IconThemeDefinition {
  id: string
  name: string
  description?: string
  category?: string
  getFileIcon(
    fileName: string,
    isDir: boolean,
    isExpanded?: boolean,
    isSymlink?: boolean,
  ): FileIconResult
}

export interface IconThemeSource {
  extensionId: string
  isBundled?: boolean
}
```

- [ ] **Step 2: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

Expected: no errors related to icon-themes/types.ts

- [ ] **Step 3: Commit**

```bash
git add web/src/extensions/icon-themes/types.ts
git commit -m "feat(icon-themes): add IconThemeSource type"
```

---

### Task 2: Icon theme registry — replace stub with full implementation

**Files:**
- Modify: `web/src/extensions/icon-themes/icon-theme-registry.ts`

- [ ] **Step 1: Replace the stub with the real implementation**

```typescript
// web/src/extensions/icon-themes/icon-theme-registry.ts
import type { IconThemeDefinition, IconThemeSource } from "./types"

class IconThemeRegistry {
  private themes: Map<string, IconThemeDefinition> = new Map()
  private themeSources: Map<string, IconThemeSource> = new Map()
  private listeners: Set<() => void> = new Set()

  registerTheme(theme: IconThemeDefinition, source?: IconThemeSource) {
    this.themes.set(theme.id, theme)
    if (source) {
      this.themeSources.set(theme.id, source)
    } else {
      this.themeSources.delete(theme.id)
    }
    this.notifyListeners()
  }

  unregisterTheme(id: string) {
    this.themes.delete(id)
    this.themeSources.delete(id)
    this.notifyListeners()
  }

  unregisterThemesByExtension(extensionId: string) {
    const themeIds = Array.from(this.themeSources.entries())
      .filter(([, source]) => source.extensionId === extensionId)
      .map(([themeId]) => themeId)

    for (const themeId of themeIds) {
      this.themes.delete(themeId)
      this.themeSources.delete(themeId)
    }

    if (themeIds.length > 0) {
      this.notifyListeners()
    }
  }

  getThemeSource(id: string): IconThemeSource | undefined {
    return this.themeSources.get(id)
  }

  getThemesByExtension(extensionId: string): IconThemeDefinition[] {
    return Array.from(this.themeSources.entries())
      .filter(([, source]) => source.extensionId === extensionId)
      .map(([themeId]) => this.themes.get(themeId))
      .filter((theme): theme is IconThemeDefinition => Boolean(theme))
  }

  clearExtension(extensionId: string) {
    this.unregisterThemesByExtension(extensionId)
  }

  markBundledTheme(id: string) {
    const theme = this.themes.get(id)
    if (!theme) return
    this.themeSources.set(id, { extensionId: "builtin", isBundled: true })
    this.notifyListeners()
  }

  getTheme(id: string): IconThemeDefinition | undefined {
    return this.themes.get(id)
  }

  getAllThemes(): IconThemeDefinition[] {
    return Array.from(this.themes.values())
  }

  // Alias kept for backward compat with existing callers
  getIconTheme(id: string): IconThemeDefinition | undefined {
    return this.getTheme(id)
  }

  // Alias kept for backward compat
  getAllIconThemes(): IconThemeDefinition[] {
    return this.getAllThemes()
  }

  onRegistryChange(callback: () => void): () => void {
    this.listeners.add(callback)
    return () => this.listeners.delete(callback)
  }

  private notifyListeners() {
    for (const listener of this.listeners) {
      listener()
    }
  }
}

export const iconThemeRegistry = new IconThemeRegistry()
```

- [ ] **Step 2: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add web/src/extensions/icon-themes/icon-theme-registry.ts
git commit -m "feat(icon-themes): replace stub registry with full implementation"
```

---

### Task 3: Port the 6 built-in icon themes

**Files:**
- Create: `web/src/extensions/icon-themes/builtin/classic-theme.tsx`
- Create: `web/src/extensions/icon-themes/builtin/material-theme.tsx`
- Create: `web/src/extensions/icon-themes/builtin/colorful-material-theme.tsx`
- Create: `web/src/extensions/icon-themes/builtin/compact-theme.tsx`
- Create: `web/src/extensions/icon-themes/builtin/minimal-theme.tsx`
- Create: `web/src/extensions/icon-themes/builtin/none-theme.tsx`

- [ ] **Step 1: Create classic-theme.tsx**

```tsx
// web/src/extensions/icon-themes/builtin/classic-theme.tsx
import { FileText, Folder, FolderOpen } from "@phosphor-icons/react"
import type { IconThemeDefinition } from "../types"

export const classicIconTheme: IconThemeDefinition = {
  id: "classic",
  name: "Classic",
  description: "Traditional file manager style icons",
  getFileIcon: (_fileName: string, isDir: boolean, isExpanded = false, _isSymlink = false) => {
    if (isDir) {
      const Icon = isExpanded ? FolderOpen : Folder
      return { component: <Icon strokeWidth={1.5} /> }
    }
    return { component: <FileText strokeWidth={1.5} /> }
  },
}
```

- [ ] **Step 2: Create material-theme.tsx**

```tsx
// web/src/extensions/icon-themes/builtin/material-theme.tsx
import { Folder, FolderOpen } from "@phosphor-icons/react"
import { getIcon } from "material-file-icons"
import type { IconThemeDefinition } from "../types"

export const materialIconTheme: IconThemeDefinition = {
  id: "material",
  name: "Material Icons",
  description: "Material Design file icons",
  getFileIcon: (fileName: string, isDir: boolean, isExpanded = false, _isSymlink = false) => {
    if (isDir) {
      const Icon = isExpanded ? FolderOpen : Folder
      return { component: <Icon /> }
    }
    const icon = getIcon(fileName)
    const svgContent = icon.svg
      .replace(/fill="[^"]*"/g, 'fill="currentColor"')
      .replace(/stroke="[^"]*"/g, 'stroke="currentColor"')
    return { svg: svgContent }
  },
}
```

- [ ] **Step 3: Create colorful-material-theme.tsx**

```tsx
// web/src/extensions/icon-themes/builtin/colorful-material-theme.tsx
import { Folder, FolderOpen } from "@phosphor-icons/react"
import { getIcon } from "material-file-icons"
import type { IconThemeDefinition } from "../types"

export const colorfulMaterialIconTheme: IconThemeDefinition = {
  id: "colorful-material",
  name: "Colorful Material Icons",
  description: "Material Design file icons with original colors",
  getFileIcon: (fileName: string, isDir: boolean, isExpanded = false, _isSymlink = false) => {
    if (isDir) {
      const Icon = isExpanded ? FolderOpen : Folder
      return { component: <Icon /> }
    }
    const icon = getIcon(fileName)
    // Keep original colors — do not replace fill/stroke
    return { svg: icon.svg }
  },
}
```

- [ ] **Step 4: Create compact-theme.tsx**

```tsx
// web/src/extensions/icon-themes/builtin/compact-theme.tsx
import { File, Folder, FolderOpen } from "@phosphor-icons/react"
import type { IconThemeDefinition } from "../types"

export const compactIconTheme: IconThemeDefinition = {
  id: "compact",
  name: "Compact",
  description: "Smaller, space-efficient icons",
  getFileIcon: (_fileName: string, isDir: boolean, isExpanded = false, _isSymlink = false) => {
    if (isDir) {
      const Icon = isExpanded ? FolderOpen : Folder
      return { component: <Icon strokeWidth={2} /> }
    }
    return { component: <File strokeWidth={2} /> }
  },
}
```

- [ ] **Step 5: Create minimal-theme.tsx**

```tsx
// web/src/extensions/icon-themes/builtin/minimal-theme.tsx
import { File, Folder, FolderOpen } from "@phosphor-icons/react"
import type { IconThemeDefinition } from "../types"

export const minimalIconTheme: IconThemeDefinition = {
  id: "minimal",
  name: "Minimal Icons",
  description: "Simple monochrome file icons",
  getFileIcon: (_fileName: string, isDir: boolean, isExpanded = false, _isSymlink = false) => {
    if (isDir) {
      const Icon = isExpanded ? FolderOpen : Folder
      return { component: <Icon /> }
    }
    return { component: <File /> }
  },
}
```

- [ ] **Step 6: Create none-theme.tsx**

```tsx
// web/src/extensions/icon-themes/builtin/none-theme.tsx
import { File, Folder, FolderOpen } from "@phosphor-icons/react"
import type { IconThemeDefinition } from "../types"

export const noneIconTheme: IconThemeDefinition = {
  id: "none",
  name: "None",
  description: "No file type icons — basic file and folder icons only",
  getFileIcon: (_fileName: string, isDir: boolean, isExpanded = false, _isSymlink = false) => {
    if (isDir) {
      const Icon = isExpanded ? FolderOpen : Folder
      return { component: <Icon /> }
    }
    return { component: <File /> }
  },
}
```

- [ ] **Step 7: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

Expected: no errors

- [ ] **Step 8: Commit**

```bash
git add web/src/extensions/icon-themes/builtin/
git commit -m "feat(icon-themes): add 6 built-in icon themes (classic, material, colorful-material, compact, minimal, none)"
```

---

### Task 4: Icon theme initializer + bootstrap wiring

**Files:**
- Create: `web/src/extensions/icon-themes/icon-theme-initializer.ts`
- Modify: `web/src/main.tsx`

- [ ] **Step 1: Create the initializer**

```typescript
// web/src/extensions/icon-themes/icon-theme-initializer.ts
import { iconThemeRegistry } from "./icon-theme-registry"
import { classicIconTheme } from "./builtin/classic-theme"
import { materialIconTheme } from "./builtin/material-theme"
import { colorfulMaterialIconTheme } from "./builtin/colorful-material-theme"
import { compactIconTheme } from "./builtin/compact-theme"
import { minimalIconTheme } from "./builtin/minimal-theme"
import { noneIconTheme } from "./builtin/none-theme"

export function initializeIconThemes() {
  iconThemeRegistry.registerTheme(classicIconTheme)
  iconThemeRegistry.registerTheme(materialIconTheme)
  iconThemeRegistry.registerTheme(colorfulMaterialIconTheme)
  iconThemeRegistry.registerTheme(compactIconTheme)
  iconThemeRegistry.registerTheme(minimalIconTheme)
  iconThemeRegistry.registerTheme(noneIconTheme)
}
```

- [ ] **Step 2: Call it from main.tsx**

In `web/src/main.tsx`, add the import and call right after the existing `ensureStartupAppearanceApplied()` call:

```typescript
// Add this import at the top of main.tsx with the other imports:
import { initializeIconThemes } from "@/extensions/icon-themes/icon-theme-initializer"

// Add this call right after ensureStartupAppearanceApplied():
initializeIconThemes()
```

The relevant section of main.tsx should look like:

```typescript
// Apply the cached theme immediately (synchronous) so the correct dark/light
// class is set before React renders anything — prevents a flash of light mode.
ensureStartupAppearanceApplied()

// Register built-in icon themes synchronously so the registry is populated
// before any component reads from it.
initializeIconThemes()

// Kick off settings load.
void initializeSettingsStore()
```

- [ ] **Step 3: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add web/src/extensions/icon-themes/icon-theme-initializer.ts web/src/main.tsx
git commit -m "feat(icon-themes): register all built-in themes on app startup"
```

---

### Task 5: Wire FileExplorerIcon to the icon theme registry

**Files:**
- Modify: `web/src/features/file-explorer/components/file-explorer-icon.tsx`

The current stub always renders a plain `<FileText />`. Replace it with a real implementation that reads the active theme from settings and looks it up in the registry.

- [ ] **Step 1: Replace the stub**

```tsx
// web/src/features/file-explorer/components/file-explorer-icon.tsx
import { FileText } from "@phosphor-icons/react"
import { useMemo } from "react"
import { iconThemeRegistry } from "@/extensions/icon-themes/icon-theme-registry"
import { useSettingsStore } from "@/features/settings/store"

export interface FileExplorerIconProps {
  fileName?: string
  filePath?: string
  isDirectory?: boolean
  /** Alias for isDirectory. */
  isDir?: boolean
  isExpanded?: boolean
  className?: string
  size?: number
}

export function FileExplorerIcon({
  fileName = "",
  isDirectory,
  isDir,
  isExpanded = false,
  className,
  size = 16,
}: FileExplorerIconProps) {
  const iconThemeId = useSettingsStore((state) => state.settings.iconTheme)

  const iconResult = useMemo(() => {
    const theme = iconThemeRegistry.getTheme(iconThemeId) ?? iconThemeRegistry.getAllThemes()[0]
    if (!theme) return null
    try {
      return theme.getFileIcon(fileName, isDirectory ?? isDir ?? false, isExpanded)
    } catch {
      return null
    }
  }, [iconThemeId, fileName, isDirectory, isDir, isExpanded])

  if (!iconResult) {
    return <FileText className={className} size={size} />
  }

  if (iconResult.component) {
    // Wrap the component to apply className and size via a span container
    return (
      <span
        className={className}
        style={{ display: "inline-flex", alignItems: "center", width: size, height: size, flexShrink: 0 }}
      >
        {iconResult.component}
      </span>
    )
  }

  if (iconResult.svg) {
    return (
      <span
        className={className}
        style={{ display: "inline-flex", alignItems: "center", width: size, height: size, flexShrink: 0 }}
        // eslint-disable-next-line react/no-danger
        dangerouslySetInnerHTML={{ __html: iconResult.svg }}
      />
    )
  }

  return <FileText className={className} size={size} />
}
```

- [ ] **Step 2: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add web/src/features/file-explorer/components/file-explorer-icon.tsx
git commit -m "feat(icon-themes): wire FileExplorerIcon to registry — renders active icon theme"
```

---

### Task 6: Settings types — add `themeMode`

**Files:**
- Modify: `web/src/features/settings/types/settings.ts`
- Modify: `web/src/features/settings/config/default-settings.ts`

- [ ] **Step 1: Add `themeMode` to the Settings interface**

In `web/src/features/settings/types/settings.ts`, add a new export and field in the `// Theme` section:

```typescript
// Add this near the top with the other type exports:
export type ThemeMode = "light" | "dark" | "system"
```

In the `Settings` interface, inside `// Theme`, add after the `iconTheme` line:

```typescript
  themeMode: ThemeMode;
```

The Theme section of the interface should look like:

```typescript
  // Theme
  theme: Theme;
  iconTheme: string;
  themeMode: ThemeMode;
  syncSystemTheme: boolean;     // deprecated — kept for migration only
  autoThemeLight: Theme;        // deprecated — kept for migration only
  autoThemeDark: Theme;         // deprecated — kept for migration only
  nativeMenuBar: boolean;
  compactMenuBar: boolean;
  windowTransparency: boolean;
  sidebarTabsPosition: "top" | "left";
  titleBarProjectMode: "tabs" | "window";
```

- [ ] **Step 2: Add the default value**

In `web/src/features/settings/config/default-settings.ts`, add after `iconTheme: "material"`:

```typescript
  themeMode: "system",
```

- [ ] **Step 3: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

Expected: no errors (the new field has a default and the deprecated fields are still present)

- [ ] **Step 4: Commit**

```bash
git add web/src/features/settings/types/settings.ts web/src/features/settings/config/default-settings.ts
git commit -m "feat(settings): add themeMode field (light | dark | system) replacing syncSystemTheme toggle"
```

---

### Task 7: Settings normalization — handle missing `themeMode` in saved data

When users have old settings saved to localStorage, `themeMode` won't be present. Normalization adds it.

**Files:**
- Modify: `web/src/features/settings/lib/settings-normalization.ts`

- [ ] **Step 1: Check current normalization file**

```bash
cat web/src/features/settings/lib/settings-normalization.ts
```

- [ ] **Step 2: Add themeMode migration**

Find the function that fills in missing settings (likely merging with `defaultSettings`). Add the following migration — if `themeMode` is absent but `syncSystemTheme` is present, derive the initial value:

```typescript
// Inside the normalization/migration function, after the spread with defaultSettings:
if (!normalized.themeMode) {
  normalized.themeMode = normalized.syncSystemTheme ? "system" : "light";
}
```

If the normalization file just does a spread like `{ ...defaultSettings, ...savedSettings }`, the `themeMode` default will already be applied automatically. In that case no change is needed — verify by checking the file.

- [ ] **Step 3: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

- [ ] **Step 4: Commit**

```bash
git add web/src/features/settings/lib/settings-normalization.ts
git commit -m "feat(settings): migrate themeMode from legacy syncSystemTheme on first load"
```

---

### Task 8: Settings effects — replace `syncSystemTheme` logic with `themeMode`

**Files:**
- Modify: `web/src/features/settings/lib/settings-effects.ts`

- [ ] **Step 1: Update `getEffectiveTheme`**

Replace the current function:

```typescript
// OLD:
function getEffectiveTheme(
  settings: Pick<Settings, "theme" | "syncSystemTheme" | "autoThemeLight" | "autoThemeDark">,
): Theme {
  if (!settings.syncSystemTheme) {
    return settings.theme;
  }
  return getSystemThemePreference() === "dark" ? settings.autoThemeDark : settings.autoThemeLight;
}
```

With:

```typescript
// NEW:
function getEffectiveTheme(
  settings: Pick<Settings, "theme" | "themeMode">,
): Theme {
  if (settings.themeMode === "system") {
    // The theme itself supports light/dark; just return it and let the
    // themeRegistry apply the right variant based on system preference.
    return settings.theme;
  }
  return settings.theme;
}
```

> Note: `themeMode` controls whether the `.dark` class is toggled on `<html>`. The registry applies the theme CSS; Tailwind's `dark:` variants follow the `.dark` class. `getEffectiveTheme` now always returns `settings.theme` — the mode is applied separately in `applyTheme` + `syncModeWithSystem`.

- [ ] **Step 2: Add a `applyThemeMode` function**

After `stopSystemThemeSync()`, add:

```typescript
function applyThemeMode(themeMode: "light" | "dark" | "system") {
  if (themeMode === "system") {
    const systemIsDark = getSystemThemePreference() === "dark";
    document.documentElement.classList.toggle("dark", systemIsDark);
  } else {
    document.documentElement.classList.toggle("dark", themeMode === "dark");
  }
}
```

- [ ] **Step 3: Update `syncThemeWithSystem` to use themeMode**

Replace the current `syncThemeWithSystem` signature and body:

```typescript
function syncThemeWithSystem(themeMode: "light" | "dark" | "system") {
  if (typeof window === "undefined" || !window.matchMedia) return;
  if (themeMode !== "system") {
    stopSystemThemeSync();
    return;
  }

  const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");
  stopSystemThemeSync();

  const handleChange = () => {
    applyThemeMode("system");
  };

  if ("addEventListener" in mediaQuery) {
    mediaQuery.addEventListener("change", handleChange);
    removeThemeSyncListener = () => mediaQuery.removeEventListener("change", handleChange);
  } else {
    const legacyMediaQuery = mediaQuery as LegacyMediaQueryList;
    legacyMediaQuery.addListener(handleChange);
    removeThemeSyncListener = () => legacyMediaQuery.removeListener(handleChange);
  }
}
```

- [ ] **Step 4: Update `applySettingsSideEffects`**

Replace the current implementation:

```typescript
export function applySettingsSideEffects(settings: Settings) {
  cacheFontSettings(settings);
  applyWindowTransparency(settings.windowTransparency);
  applyThemeMode(settings.themeMode);
  void applyTheme(settings.theme);
  syncThemeWithSystem(settings.themeMode);
  syncOllamaBaseUrl(settings.ollamaBaseUrl);
  syncCustomProviderBaseUrl(settings.aiCustomBaseUrl);
  void syncOllamaApiKey();
}
```

- [ ] **Step 5: Update `applySettingSideEffect`**

Replace the theme-related cases:

```typescript
export function applySettingSideEffect<K extends keyof Settings>(
  key: K,
  value: Settings[K],
  getSettings: () => Settings,
) {
  if (key === "theme") {
    void applyTheme(getSettings().theme);
  }

  if (key === "themeMode") {
    const settings = getSettings();
    applyThemeMode(settings.themeMode);
    syncThemeWithSystem(settings.themeMode);
  }

  if (key === "ollamaBaseUrl") {
    syncOllamaBaseUrl(value as string);
  }

  if (key === "aiCustomBaseUrl") {
    syncCustomProviderBaseUrl(value as string);
  }

  if (key === "fontFamily" || key === "uiFontFamily" || key === "uiFontSize") {
    cacheFontSettings(getSettings());
  }

  if (key === "windowTransparency") {
    applyWindowTransparency(value as boolean);
  }
}
```

> Remove the `syncSystemTheme`, `autoThemeLight`, `autoThemeDark` cases — they are no longer wired to the UI.

- [ ] **Step 6: Update the `Pick<Settings>` type for `getEffectiveTheme` callers**

The `applyTheme` function signature in `settings-effects.ts` receives a `Theme` string directly — no changes needed there. Just make sure no remaining code calls `getEffectiveTheme` with the old fields. Search:

```bash
grep -n "getEffectiveTheme\|syncSystemTheme\|autoThemeLight\|autoThemeDark" web/src/features/settings/lib/settings-effects.ts
```

Expected: `getEffectiveTheme` appears 0 times (we removed it), and the deprecated fields are not referenced.

- [ ] **Step 7: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit 2>&1 | head -30
```

- [ ] **Step 8: Commit**

```bash
git add web/src/features/settings/lib/settings-effects.ts
git commit -m "feat(settings): replace syncSystemTheme toggle with themeMode (light|dark|system)"
```

---

### Task 9: Tab structure cleanup — remove General and Keybindings

**Files:**
- Modify: `web/src/features/settings/components/settings-vertical-tabs.tsx`
- Modify: `web/src/features/settings/components/settings-dialog.tsx`
- Modify: `web/src/features/window/stores/ui-state-store.ts`

- [ ] **Step 1: Remove General and Keybindings from SETTINGS_TAB_ITEMS**

In `web/src/features/settings/components/settings-vertical-tabs.tsx`, replace `SETTINGS_TAB_ITEMS`:

```typescript
export const SETTINGS_TAB_ITEMS: SettingsTabItem[] = [
  { id: "appearance",    label: "Appearance",  icon: PaintBrush },
  { id: "editor",        label: "Editor",      icon: CodeBlock },
  { id: "file-explorer", label: "Files",       icon: TreeStructure },
  { id: "git",           label: "Git",         icon: GitBranch },
  { id: "terminal",      label: "Terminal",    icon: TerminalWindow },
];
```

Remove the unused imports `GearSix` and `Keyboard` from the import statement at the top.

- [ ] **Step 2: Remove General and Keyboard cases from the dialog switch**

In `web/src/features/settings/components/settings-dialog.tsx`:

Remove these import lines:
```typescript
import { GeneralSettings } from "./tabs/general-settings";
import { KeyboardSettings } from "./tabs/keyboard-settings";
```

Replace `renderTabContent`:

```typescript
const renderTabContent = () => {
  switch (activeTab) {
    case "appearance":    return <AppearanceSettings />;
    case "editor":        return <EditorSettings />;
    case "file-explorer": return <FileTreeSettings />;
    case "git":           return <GitSettings />;
    case "terminal":      return <TerminalSettings />;
    default:              return <AppearanceSettings />;
  }
};
```

Also update the initial `settingsInitialTab` fallback in the `useEffect`:

```typescript
useEffect(() => {
  if (isOpen) {
    if (settingsInitialTab === "language") {
      setActiveTab("editor");
    } else if (
      (!hasEnterpriseAccess && settingsInitialTab === "enterprise") ||
      (!hasTeamsAccess && settingsInitialTab === "collaboration") ||
      settingsInitialTab === "general" ||
      settingsInitialTab === "keyboard"
    ) {
      setActiveTab("appearance");
    } else {
      setActiveTab(settingsInitialTab);
    }
  }
}, [settingsInitialTab, isOpen, hasEnterpriseAccess, hasTeamsAccess]);
```

- [ ] **Step 3: Update the default tab in ui-state-store**

In `web/src/features/window/stores/ui-state-store.ts`, change the default:

```typescript
// OLD:
settingsInitialTab: "general" as SettingsTab,

// NEW:
settingsInitialTab: "appearance" as SettingsTab,
```

- [ ] **Step 4: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

- [ ] **Step 5: Commit**

```bash
git add web/src/features/settings/components/settings-vertical-tabs.tsx \
        web/src/features/settings/components/settings-dialog.tsx \
        web/src/features/window/stores/ui-state-store.ts
git commit -m "feat(settings): remove General and Keybindings tabs — 5-tab layout (Appearance, Editor, Files, Git, Terminal)"
```

---

### Task 10: Rewrite the Appearance settings tab

**Files:**
- Modify: `web/src/features/settings/components/tabs/appearance-settings.tsx`

This is the biggest UI change. The new tab has:
- **Theme section**: Color Theme dropdown (all themes) + Theme Mode dropdown (Light / Dark / Sync with System)
- **Icons section**: Icon Theme dropdown (reads from registry — now populated)
- **Typography section**: UI Font Family (keep FontSelector) + UI Font Size
- **Layout section**: Sidebar Position only (remove Sidebar Tabs, Window Transparency, Title Bar Project Mode, Open Projects in New Window)

- [ ] **Step 1: Replace the entire file**

```tsx
// web/src/features/settings/components/tabs/appearance-settings.tsx
import { Upload } from "@phosphor-icons/react";
import { useEffect, useMemo, useState } from "react";
import { iconThemeRegistry } from "@/extensions/icon-themes/icon-theme-registry";
import type { IconThemeDefinition } from "@/extensions/icon-themes/types";
import { themeRegistry } from "@/extensions/themes/theme-registry";
import type { ThemeDefinition } from "@/extensions/themes/types";
import {
  formatUiFontSize,
  UI_FONT_SIZE_MAX,
  UI_FONT_SIZE_MIN,
  UI_FONT_SIZE_STEP,
} from "@/features/settings/lib/ui-font-size";
import { getDefaultSetting, useSettingsStore } from "@/features/settings/store";
import { Button } from "@/components/ui/button";
import NumberInput from "@/components/ui/number-input";
import Section, { SETTINGS_CONTROL_WIDTHS, SettingRow } from "../settings-section";
import Select from "@/components/ui/select";
import { cn } from "@/utils/cn";
import { FontSelector } from "../font-selector";
import type { ThemeMode } from "@/features/settings/types/settings";

const THEME_MODE_OPTIONS: { value: ThemeMode; label: string }[] = [
  { value: "system", label: "Sync with System" },
  { value: "light",  label: "Light" },
  { value: "dark",   label: "Dark" },
];

const SIDEBAR_POSITION_OPTIONS = [
  { value: "left",  label: "Left" },
  { value: "right", label: "Right" },
];

export const AppearanceSettings = () => {
  const { settings, updateSetting } = useSettingsStore();
  const [themeOptions, setThemeOptions] = useState<{ value: string; label: string }[]>([]);
  const [iconThemeOptions, setIconThemeOptions] = useState<{ value: string; label: string }[]>([]);

  // Load color themes from registry
  useEffect(() => {
    const load = () => {
      const options = themeRegistry.getAllThemes().map((theme: ThemeDefinition) => ({
        value: theme.id,
        label: theme.name,
      }));
      setThemeOptions(options);
    };
    load();
    return themeRegistry.onRegistryChange(load);
  }, []);

  // Ensure the current theme appears even if it's not in the list yet
  const normalizedThemeOptions = useMemo(() => {
    if (themeOptions.some((o) => o.value === settings.theme)) return themeOptions;
    const fallback = themeRegistry.getTheme(settings.theme);
    if (!fallback) return themeOptions;
    return [{ value: fallback.id, label: fallback.name }, ...themeOptions];
  }, [themeOptions, settings.theme]);

  // Load icon themes from registry
  useEffect(() => {
    const load = () => {
      const options = iconThemeRegistry.getAllThemes().map((theme: IconThemeDefinition) => ({
        value: theme.id,
        label: theme.name,
      }));
      setIconThemeOptions(options);
    };
    load();
    return iconThemeRegistry.onRegistryChange(load);
  }, []);

  const normalizedIconThemeOptions = useMemo(() => {
    if (iconThemeOptions.some((o) => o.value === settings.iconTheme)) return iconThemeOptions;
    const fallback = iconThemeRegistry.getTheme(settings.iconTheme);
    if (!fallback) return iconThemeOptions;
    return [{ value: fallback.id, label: fallback.name }, ...iconThemeOptions];
  }, [iconThemeOptions, settings.iconTheme]);

  const handleUploadTheme = async () => {
    const input = document.createElement("input");
    input.type = "file";
    input.accept = ".json";
    input.onchange = async (e) => {
      const file = (e.target as HTMLInputElement).files?.[0];
      if (file) {
        const { uploadTheme } = await import("@/features/settings/utils/theme-upload");
        const result = await uploadTheme(file);
        if (!result.success) {
          console.error("Theme upload failed:", result.error);
        }
      }
    };
    input.click();
  };

  return (
    <div className="space-y-4">
      <Section title="Theme">
        <SettingRow
          label="Color Theme"
          description="Choose your active color theme"
          onReset={() => updateSetting("theme", getDefaultSetting("theme"))}
          canReset={settings.theme !== getDefaultSetting("theme")}
        >
          <div className="flex items-center gap-2">
            <Select
              value={settings.theme}
              options={normalizedThemeOptions}
              onChange={(value) => updateSetting("theme", value)}
              className={SETTINGS_CONTROL_WIDTHS.wide}
              size="xs"
              variant="default"
              searchable
              searchableTrigger="input"
            />
            <Button
              type="button"
              onClick={handleUploadTheme}
              variant="default"
              tooltip="Upload theme"
              aria-label="Upload theme"
              compact
            >
              <Upload />
            </Button>
          </div>
        </SettingRow>

        <SettingRow
          label="Theme Mode"
          description="Use light, dark, or follow system preference"
          onReset={() => updateSetting("themeMode", getDefaultSetting("themeMode"))}
          canReset={settings.themeMode !== getDefaultSetting("themeMode")}
        >
          <Select
            value={settings.themeMode}
            options={THEME_MODE_OPTIONS}
            onChange={(value) => updateSetting("themeMode", value as ThemeMode)}
            className={SETTINGS_CONTROL_WIDTHS.wide}
            size="xs"
            variant="default"
          />
        </SettingRow>
      </Section>

      <Section title="Icons">
        <SettingRow
          label="Icon Theme"
          description="Icons displayed in the file tree and tabs"
          onReset={() => updateSetting("iconTheme", getDefaultSetting("iconTheme"))}
          canReset={settings.iconTheme !== getDefaultSetting("iconTheme")}
        >
          <Select
            value={settings.iconTheme}
            options={normalizedIconThemeOptions}
            onChange={(value) => updateSetting("iconTheme", value)}
            className={SETTINGS_CONTROL_WIDTHS.wide}
            size="xs"
            variant="default"
          />
        </SettingRow>
      </Section>

      <Section title="Typography">
        <SettingRow
          label="UI Font Family"
          description="Font family for UI elements (file tree, markdown, etc.)"
          onReset={() => updateSetting("uiFontFamily", getDefaultSetting("uiFontFamily"))}
          canReset={settings.uiFontFamily !== getDefaultSetting("uiFontFamily")}
        >
          <FontSelector
            value={settings.uiFontFamily}
            onChange={(fontFamily) => updateSetting("uiFontFamily", fontFamily)}
            className={SETTINGS_CONTROL_WIDTHS.text}
            monospaceOnly={false}
          />
        </SettingRow>

        <SettingRow
          label="UI Font Size"
          description="Adjust UI text and icon scale in 0.5px steps"
          onReset={() => updateSetting("uiFontSize", getDefaultSetting("uiFontSize"))}
          canReset={settings.uiFontSize !== getDefaultSetting("uiFontSize")}
        >
          <NumberInput
            min={String(UI_FONT_SIZE_MIN)}
            max={String(UI_FONT_SIZE_MAX)}
            step={String(UI_FONT_SIZE_STEP)}
            value={settings.uiFontSize}
            onChange={(value) => updateSetting("uiFontSize", value)}
            className={cn(SETTINGS_CONTROL_WIDTHS.number, "tabular-nums")}
            size="xs"
            aria-label={`UI font size: ${formatUiFontSize(settings.uiFontSize)} pixels`}
          />
        </SettingRow>
      </Section>

      <Section title="Layout">
        <SettingRow
          label="Sidebar Position"
          description="Which side the file sidebar appears on"
          onReset={() => updateSetting("sidebarPosition", getDefaultSetting("sidebarPosition"))}
          canReset={settings.sidebarPosition !== getDefaultSetting("sidebarPosition")}
        >
          <Select
            value={settings.sidebarPosition}
            options={SIDEBAR_POSITION_OPTIONS}
            onChange={(value) => updateSetting("sidebarPosition", value as "left" | "right")}
            className={SETTINGS_CONTROL_WIDTHS.compact}
            size="xs"
            variant="default"
          />
        </SettingRow>
      </Section>
    </div>
  );
};
```

- [ ] **Step 2: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

- [ ] **Step 3: Commit**

```bash
git add web/src/features/settings/components/tabs/appearance-settings.tsx
git commit -m "feat(settings): rewrite Appearance tab — Color Theme + Theme Mode, all 6 icon themes, clean layout section"
```

---

### Task 11: Editor tab — remove Editor Engine control

**Files:**
- Modify: `web/src/features/settings/components/tabs/editor-settings.tsx`

- [ ] **Step 1: Remove the Editor Engine SettingRow**

Search for the `editorEngine` SettingRow in `web/src/features/settings/components/tabs/editor-settings.tsx`. It looks like this:

```tsx
<SettingRow
  label="Editor Engine"
  ...
>
  <Select
    value={settings.editorEngine}
    ...
  />
</SettingRow>
```

Delete the entire `<SettingRow>` block for Editor Engine.

Also remove the `editorEngineOptions` array definition and any unused imports that only served that row (like `EditorEngine` type if it's no longer used in the file).

- [ ] **Step 2: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

- [ ] **Step 3: Commit**

```bash
git add web/src/features/settings/components/tabs/editor-settings.tsx
git commit -m "feat(settings): remove Editor Engine selector — Monaco only"
```

---

### Task 12: Wire Sidebar Position to the IDE layout

**Files:**
- Modify: `web/src/components/layout/IDEShell.tsx`

Currently the sidebar is always on the left — `sidebarPosition` is stored but never applied to the layout. The fix is to read the setting and flip the flex direction of the root row.

- [ ] **Step 1: Add the settings import and read the setting**

At the top of `web/src/components/layout/IDEShell.tsx`, add:

```typescript
import { useSettingsStore } from "@/features/settings/store"
```

Inside `IDEShell()`, add:

```typescript
const sidebarPosition = useSettingsStore((state) => state.settings.sidebarPosition)
```

- [ ] **Step 2: Apply direction to the ResizablePanelGroup**

The root layout is a `<ResizablePanelGroup orientation="horizontal">`. Wrap the ResizablePanel (sidebar) and the main panel order using `order` classes based on `sidebarPosition`:

```tsx
<ResizablePanelGroup orientation="horizontal" className="h-full">
  <ResizablePanel
    defaultSize="20%"
    minSize="12%"
    maxSize="45%"
    className="flex flex-col overflow-hidden"
    style={{ order: sidebarPosition === "right" ? 2 : 0 }}
  >
    {/* sidebar content */}
  </ResizablePanel>

  <ResizableHandle style={{ order: sidebarPosition === "right" ? 1 : 1 }} />

  <ResizablePanel
    className="flex min-w-0 flex-1 flex-col overflow-hidden"
    style={{ order: sidebarPosition === "right" ? 0 : 2 }}
  >
    {/* main content */}
  </ResizablePanel>
</ResizablePanelGroup>
```

> If `ResizablePanelGroup` doesn't support `style` on children through `order`, use a CSS class approach: add `flex-row-reverse` to the group's `className` when `sidebarPosition === "right"`:

```tsx
<ResizablePanelGroup
  orientation="horizontal"
  className={cn("h-full", sidebarPosition === "right" && "flex-row-reverse")}
>
```

Use whichever approach applies — check what props `ResizablePanelGroup` accepts by reading `web/src/components/ui/resizable.tsx`.

- [ ] **Step 3: Verify TypeScript compiles and the setting flips the sidebar**

```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

Then open the dev server, change Sidebar Position in Settings → Appearance, and confirm the sidebar moves.

- [ ] **Step 4: Commit**

```bash
git add web/src/components/layout/IDEShell.tsx
git commit -m "feat(settings): wire sidebarPosition to IDEShell layout — sidebar flips left/right"
```

---

### Task 13: Fix modal — phantom scrollbar and scaling

**Files:**
- Modify: `web/src/features/settings/components/settings-dialog.tsx`
- Possibly: `web/src/components/ui/dialog.tsx`

- [ ] **Step 1: Inspect the dialog in the browser**

Open the settings dialog and use DevTools → Elements. Look for:
1. Any element that has both `overflow-y: auto` (or `scroll`) AND `overflow-y: auto` on a child — the phantom scrollbar comes from double-scroll containers
2. Any `transform: scale()` or non-1 `zoom` on the modal or its ancestors — this causes the scaling issue

- [ ] **Step 2: Fix phantom scrollbar**

In `settings-dialog.tsx`, the content pane already has `overflow-y-auto` (the `div` with `ref={contentRef}`). Check that the `<Dialog>` wrapper (`classNames.modal`) does NOT also have `overflow-y-auto`. If it does, remove it.

The `classNames.modal` value is:
```
"h-[74vh] max-h-[820px] w-[90vw] max-w-[1120px] min-w-0 border-0 max-[720px]:h-[86vh] max-[720px]:w-[calc(100vw-32px)] [&>div:first-child]:border-b-0"
```

Check `web/src/components/ui/dialog.tsx` for the base modal class — if it includes `overflow-y-auto`, remove it (the content div handles scrolling, not the modal itself). The modal shell should be `overflow-hidden`.

- [ ] **Step 3: Fix scaling**

Use DevTools → Computed styles on the modal element. If `zoom` or `transform` is applied, trace back to where it comes from. Common causes:
- A parent element with `font-size` that differs from the root, combined with `em`-based sizing
- A `[--app-ui-control-font-size]` CSS variable being applied incorrectly

The content div has `[--app-ui-control-font-size:var(--ui-text-sm)]` — this sets a CSS variable, it doesn't scale. If scaling is visible, look for any `zoom` property on ancestors in DevTools → Computed tab.

Remove or fix the offending CSS.

- [ ] **Step 4: Verify in browser**

Open settings dialog. Scroll the left nav and the content pane — each should have exactly one scrollbar only in its own scroll zone, with no phantom scrollbar track visible on the modal frame.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/settings/components/settings-dialog.tsx web/src/components/ui/dialog.tsx
git commit -m "fix(settings): remove phantom scrollbar and fix scaling in settings modal"
```

---

## Self-Review

### Spec coverage check

| Spec requirement | Task |
|---|---|
| Remove General tab | Task 9 |
| Remove Keybindings tab | Task 9 |
| Single Color Theme picker | Task 10 |
| Theme Mode dropdown (Light/Dark/Sync) | Tasks 6, 8, 10 |
| Icon themes: full registry implementation | Task 2 |
| Icon themes: 6 built-ins | Task 3 |
| Icon themes: bootstrap wiring | Task 4 |
| Icon themes: applied in file explorer | Task 5 |
| Icon Theme dropdown in UI | Task 10 |
| UI Font Family: FontSelector (Shadcn Select already used) | Task 10 (kept FontSelector) |
| Remove Sidebar Tabs Position | Task 10 |
| Remove Title Bar Project Mode | Task 10 |
| Remove Window Transparency | Task 10 |
| Remove Open Projects in New Window | Task 10 |
| Sidebar Position wired to layout | Task 12 |
| Remove Editor Engine setting | Task 11 |
| Editor settings already wired to Monaco | Verified — no fix needed |
| Files/Git/Terminal tabs: kept, no changes | ✓ (omitted from plan intentionally) |
| Modal phantom scrollbar fix | Task 13 |
| Modal scaling fix | Task 13 |
| themeMode migration for old saved data | Task 7 |

### Placeholder scan

No TBD, TODO, or "similar to Task N" placeholders. All code blocks are complete.

### Type consistency

- `ThemeMode` exported from `types/settings.ts` — used by name in `appearance-settings.tsx` and `settings-effects.ts`
- `IconThemeSource` added to `types.ts` — used in `icon-theme-registry.ts`
- `colorfulMaterialIconTheme.id` is `"colorful-material"` (not `"material"` as in the upstream source, which collided with `materialIconTheme.id` — fixed here)
- `getDefaultSetting("themeMode")` returns `"system"` — matches `defaultSettings.themeMode: "system"`
