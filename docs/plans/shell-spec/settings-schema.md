# Settings modal spec (3/3): proposed schema, port list, ownership, live capture checklist

Companion to `settings.md` (chrome, controls) and `settings-pages.md` (every field). Everything here is derived from `app/panes/src/features/settings/*` unless cited otherwise.

## 1. Proposed schema (what the native modal renders)

Goal: the shell renders the modal from data so extensions can add pages/rows later, while today's modal is reproduced exactly. The schema is data only; chrome (section 2 of `settings.md`), control anatomy (section 7) and typography are fixed by the shell's theme and are not schema fields.

```jsonc
{
  "schema": 1,
  "pages": [ Page ]                       // order = rail order
}
Page {
  "id": "appearance",                     // stable persisted id (deep link target); today's ids are kept
  "label": "Appearance",
  "icon": "PaintBrush",                   // phosphor name, rendered duotone 18 px
  "when": Condition?,                     // page visible iff true (platform gating, e.g. hosts)
  "searchable": true,                     // false for keybindings/hosts/active-sessions (see settings.md 5)
  "blocks": [ Section | CustomBlock ]
}
Section {
  "type": "section",
  "title": "Typography",                  // header hidden by the renderer when it is the page's first block (parity rule)
  "description": "...",
  "rows": [ Row ]
}
Row {
  "key": "uiFontSize",                    // setting key (see storage)
  "label": "UI Font Size",
  "description": "Adjust UI text and icon scale in 0.5px steps",
  "control": Control,
  "resettable": true,                     // shows reset icon when value != default
  "default": 15,                          // required for resettable
  "when": Condition?,                     // row visible iff true (Custom Search URL)
  "searchKeywords": ["font","size"]       // feeds the search index (today: config/search-index.ts)
}
Control =
  { "type": "switch", "invert": false }                                   // `invert` for "Respect .gitignore"
| { "type": "select", "options": [ {"value","label"} ] | {"source":"themes|iconThemes|fonts|monoFonts|terminalFonts"},
    "width": "compact|default|wide|xwide", "placeholder": "...", "extraTrailing": {"icon":"Info","tooltip":"..."} }
| { "type": "number", "min": 8, "max": 32, "step": 1, "width": "numberCompact|number|default", "ariaLabel": "..." }
| { "type": "text", "placeholder": "...", "width": "textWide", "validate": "searchTemplate" }
| { "type": "textList", "separator": ",", "placeholder": "...", "rows": 2, "commit": "blurOrEnter" }
| { "type": "button", "label": "Clear data", "variant": "outline", "action": "browser.clearData",
    "enabledWhen": Condition, "disabledDescription": "...", "confirm": { "title","message","confirmLabel" } }
CustomBlock {
  "type": "custom",
  "widget": "keybindings" | "hosts" | "agents" | "activeSessions",
  "heading": {"title": "Agents", "description": "..."}   // widgets that draw their own 32 px h3 (agents, hosts, activeSessions)
}
Condition = { "setting": "browserSearchEngine", "eq": "custom" }
          | { "platform": "desktop" }          // replaces isTauri()
          | { "capability": "browser.clearData" }
          | { "all": [Condition] } | { "any": [Condition] }
Storage (per row, default "local"):
  "store": "local"                              // shell settings file, key = row.key
  | { "daemon": { "get": "/v0/settings/chat/permission-level", "put": "/v0/settings/chat/permission-level", "optimistic": true } }
Normalise: "normalize": "uiFontSize|markdownFontSize|lineHeight|renderWhitespace|indentSize|zoom|searchEngine|theme|fontFamily" // named, shell-implemented (settings-normalization.ts)
```

Vocabulary notes tied to source:
- Control kinds are exactly the five used: switch (`Switch`), select (`SettingSelect`/`FontSelector`/the permission `Select`), number (`NumberInput`), text (`Input`), textList (the two `textarea`s), button (Clear data), and the one-off Agents upload (a `fileButton` control: `{"type":"filePicker","accept":[".yaml",".yml"],"action":"agents.uploadDescriptor"}`; treat as a custom action row inside the `agents` widget).
- Dynamic option sources: `themes` (theme registry), `iconThemes`, `fonts` (4 bundled), `monoFonts` (Geist Mono Variable), `terminalFonts` (installed Nerd Fonts first with the suffix " (Nerd Font)", then monospace list; unknown current value prepended as "(Custom)"). The shell implements these providers; extensions can register more.
- `when` covers: Hosts page (`isTauri()` -> `platform:desktop`), Custom Search URL (`browserSearchEngine == custom`), Clear data enablement (capability from `browserApi.capabilities().clearData`), and the DEV-only Developer page (omit; do not model).
- First-section-header suppression is a renderer rule, not schema, but the schema must keep Section order faithful (see `settings-pages.md` for which section is first on each page).
- Search: the schema must carry `label`, `description`, `section`, `searchKeywords` per row so the index can be generated (replaces hand-kept `config/search-index.ts`, 51 records, which today drifts from the page: it lacks Keybindings/Hosts/Active sessions and uses section names that are not headings).
- Reset: `default` per row replaces `getDefaultSetting(key)`; equality is strict, arrays compared via `join(',')` (`file-tree-settings.tsx:148-149`).

### Field map (key -> row) for the generator

Keys with a UI row (see `settings-pages.md` for labels/ranges): theme, themeMode, iconTheme, uiFontFamily, headingFontFamily, uiFontSize, sidebarPosition, tabListScope, sidebarFollowsFocus, buildReadout (decision pending), fontFamily, fontSize, editorLineHeight, tabSize, wordWrap, lineNumbers, renderWhitespace, renderIndentGuides, semanticHighlighting, highlightOccurrences, showMinimap, maxOpenTabs, autoSave, formatOnSave, inlineBlame, autoCompletion, parameterHints, markdownFontSize, fileTreeIndentSize, fileTreeDensity, compactFoldersInFileTree, showHiddenFilesInFileTree, showGitignoredFilesInFileTree (inverted), hiddenFilePatterns, hiddenDirectoryPatterns, compactGitStatusBadges, showGitStatusInFileTree, terminalFontFamily, terminalFontSize, terminalLineHeight, terminalLetterSpacing, terminalScrollback, terminalCursorStyle, terminalCursorBlink, terminalCursorWidth, browserHomepage, browserSearchEngine, browserCustomSearchUrl, browserZoom, chatIsDefaultPresentation; keybindingPreset and keybindingUserOverrides (keybindings widget); daemon: default permission level, provider preferences (order, disabled, mcpDisabled), descriptors.

Keys with no row (keep in the settings model, no UI): windowTransparency, sidebarWidth, coreFeatures, buildBadgeOverride, syncSystemTheme/autoThemeLight/autoThemeDark (deprecated, migrate then drop), diffViewMode (written by the review pane toggle), chatSplitPresentationEnabled (dev tab only).

## 2. Port list (settings surface)

### Reachable in release: port to the native modal
| React unit | Role | Source |
|---|---|---|
| `SettingsDialog` (+ `SettingsTabContent` switch minus `developer`) | modal host, tab sync | `settings-dialog.tsx` |
| `AppDialog` chrome as used here (title, headerActions, close) | window chrome | `components/ui/dialog.tsx:201-279` |
| `SettingsPanels`, `SettingsTitle` (<720 px dropdown), `SettingsSearchInput` | layout, search | `settings-layout.tsx` |
| `SettingsVerticalTabs`, `SETTINGS_TAB_ITEMS` | rail, keyboard nav | `settings-vertical-tabs.tsx`, `settings-tab-items.ts` |
| `Section`, `SettingRow`, `SETTINGS_CONTROL_WIDTHS`, row search | section/row anatomy | `settings-section.tsx` (desktop branches only), `settings-control-widths.ts`, `lib/settings-row-search.ts`, `lib/settings-tab-visibility.ts` |
| `SettingSelect`, `FontSelector`, `NumberInput`, `Switch`, `Input`, textarea, `Button`, `Tooltip`, `Badge`, `Select` popup | controls | `setting-select.tsx`, `font-selector.tsx`, `components/ui/*` |
| Tabs: Appearance, Editor, FileTree, Git, Terminal, Browser, Providers (+ `ProvidersSettingsView`, `SortableProviderRow`, `ProviderColumnHeader`, `DefaultPermissionLevelSetting`, `ChatPresentationSetting`, `DescriptorStatus`, `provider-preferences`, `use-providers-settings`, `agent-providers-store`), ActiveSessions, Keybindings, Hosts | pages | `tabs/*` |
| Hosts: `AddHostDialog`, `HostStatusIcon`/`HostStatusDot`, `host-tone`, `use-pairing-flow`, `pairing`, `endpoint`, `host-revoke`, `host-config-store`, status prober | Hosts page logic | `src/features/hosts/*` (Agent C moves the logic to Rust crates; the shell only draws) |
| Keymaps: `registry`, `types`, `chord`, `effective-keymaps`, `keybinding-presets`, `keymaps/stores/store` | keybinding model | `src/features/keymaps/*` |
| Settings model: `default-settings`, `settings-normalization`, `settings-persistence`, `settings-effects`, `ui-font-size`, `markdown-font-size`, `font-family-resolution`, `typography-defaults`, `settings-import-export` (see unused list), theme registry, icon theme registry | state | `src/features/settings/{config,lib}` |
| Confirm dialog (used by revoke, clear data, kill) | `primitiveConfirm` | `src/components/ui/primitive-dialog-*.tsx` |
| Toasts for the flows above | feedback | `features/window/stores/toast-store` |

### Dead or not to be ported
| Unit | Why |
|---|---|
| `tabs/developer-settings.tsx`, `tabs/chat-split-view-setting.tsx` (only caller is the dev tab), Developer tab id, `buildBadgeOverride`, `BuildBadgeSection` | `import.meta.env.DEV` only (`settings-tab-items.ts:48`); inventory marks CUT |
| `components/mobile-settings-page.tsx`, `mobile-agents-settings.tsx`, `native-agents-settings-feed.tsx`, `native-hosts-settings-feed.tsx`, `lib/phone-settings-pages.ts`, `SettingsCard` mobile branch, every `IS_MOBILE` branch in `settings-section.tsx` and `providers-settings-view.tsx` | phone shell / iOS only (stays with Tauri mobile) |
| `lib/settings-import-export.ts`, `settings-download.ts`, `store.updateSettingsFromJSON`, `store.resetToDefaults` | no UI path (only the dev tab calls export/import/reset; verify with knip) |
| `lib/settings-storage-sync.ts` | cross-webview sync for the phone settings page |
| `config/search-index.ts` as a hand-kept file | superseded by schema-generated index |
| Unused CSS/vars: `--app-ui-button-height`, `--app-ui-badge-height`, `--app-ui-button-compact-*`, `--settings-section-title` | defined/set but consumed nowhere (grep) |
| Sidebar `BuildReadout` + Appearance "Build readout" row | decision: inventory says CUT; the row exists in release today, so removing it changes visible behaviour. Owner call. |
| `Keybinding` display-only Editor commands | keep rows (they are visible) but they stay non-editable |

## 3. Non-visual behaviour the Rust side must keep

1. **Persistence format**: one JSON value per key under the `crowbar:settings:` namespace; absent key = default; object values merge over defaults; load normalises, then rewrites only already-stored keys (never writes defaults, so a later build's new default still applies) `settings-persistence.ts:45-52`, `settings-bootstrap.ts:22-53`. 300 ms trailing debounce write coalescing. The native shell needs an equivalent file; React panes also read these (`theme`, fonts, editor options, file tree, browser, terminal) so the shell must push the effective settings to every pane (pane contract `state`/`extension.event`) instead of localStorage.
2. **Normalisation rules** (`settings-normalization.ts`): uiFontSize snap 0.5 / clamp 10..24; markdownFontSize round / clamp 12..24; editorLineHeight snap 0.1 / clamp 1..2 / non-finite 1.4; fileTreeIndentSize round / clamp 8..32 / non-finite 20; browserZoom round / clamp 50..300; legacy terminal line height 1.2 -> 1; "Geist Mono" -> "Geist Mono Variable"; unknown theme / themeMode / searchEngine / density / renderWhitespace -> default; bad booleans -> default.
3. **Effects**: `theme`, `themeMode` (toggle dark class; follow OS appearance live when `system`), fonts and sizes (CSS vars; scale = size/15), `windowTransparency` attribute; macOS window appearance sync (`setMacOSWindowAppearance`, `settings-effects.ts:118-122`).
4. **Font picker rule**: a stored family not in the available list is reset to the default on mount.
5. **Search**: clears on close; first visible tab auto-selected; hidden sections when all rows filtered.
6. **Keybinding semantics**: preset < override resolution, normalised chords, conflict detection by normalised equality, capture mode swallows all keys at the window level, Esc cancels, `Unbound` is valid, `liveEditable=false` rows are inert. The hooks that consume the registry (pane navigate, tabs, chats, zoom, console, sidebar tabs) must read the same effective map. Hardcoded: save, save-all, project switch `mod+shift+digit`.
7. **Agents**: optimistic writes with a generation fence (a stale GET or failed write must not overwrite a newer write); full-table PUT (never partial); error toasts as listed.
8. **Hosts**: pairing sequence (probe fingerprint -> user confirm -> code -> redeem), pinned fingerprint is the confirmed one, dialog non-dismissable while a request is pending, revoke order (remote DELETE first, then local cleanup), remote-ever-paired flag, removal propagates to every consumer (`onHostRemoved`).
9. **Deep link**: opening with a tab id re-targets an already-open dialog; unknown tab id falls back to Appearance (`settings-dialog.tsx:97-98`).
10. **Accessibility**: tablist/tab/tabpanel roles, roving tabindex, row label/description wired to the control (`aria-labelledby`/`aria-describedby`), switches and number steppers labelled, status dots labelled in words.

## 4. Who owns what

| Concern | Daemon | Shell | Pane webviews |
|---|---|---|---|
| Default permission level, provider order/enabled/tools flags, descriptors and their reports, active sessions (chats + presence), host client revoke (`DELETE /v0/clients/:id`) | owns (HTTP/stream) | reads/writes, optimistic, error toasts | none |
| Appearance/editor/files/git/terminal/browser settings, keybinding preset + overrides, last settings tab, search query | none | owns (settings file, model, normalisation), draws the modal | consume via pushed state (theme, fonts, editor options...) |
| Hosts list, host tokens, fingerprints, status prober | remote daemons answer probes/pairing | owns (Rust: Agent C) and draws | none |
| Modal visibility, deep link target | none | owns (view state) | request through the pane host if a pane needs to open settings (none does today) |

## 5. Edge cases (behaviour to replicate)

- Opening Settings always starts at Appearance unless a tab id is given.
- Search hides tabs without index entries (Keybindings, Hosts, Active sessions).
- Reset icons occupy space when invisible (rows do not shift).
- Textareas apply only on blur/Enter; number fields apply per keystroke, clamp on blur; typing `-` or empty then blur gives 0.
- "Respect .gitignore" switch is inverted against its key.
- Build readout row exists in release but defaults off.
- Terminal font list is effectively a single entry because OS fonts cannot be enumerated in the web view (the native shell could enumerate system fonts; keep parity first, decide separately).
- Agents list states: loading, failed (+Retry), empty, list; a descriptor override that fails validation falls back to the shipped one (badge "Override refused - using shipped").
- Revoke has several confirm branches; "Remove anyway" only after a failed remote revoke.
- Capturing a chord that already belongs to another command is allowed (warning only).

## 6. States to capture live (checklist for the lead; how to reach each)

Reach the dialog by clicking the sidebar header gear (16 px icon, top of sidebar). Hosts page: gear -> Hosts tab (dev build shows Developer; ignore it).

| # | State | How |
|---|---|---|
| 1 | Dialog closed to open transition (frames at fixed fps) | gear click; record 400 ms |
| 2 | Open on each page (10 pages) idle | click each rail tab; Appearance, Editor, Files, Git, Terminal, Browser, Agents, Active sessions, Keybindings, Hosts |
| 3 | Rail: hover, active, keyboard focus (Arrow keys), after Home/End | mouse, keyboard |
| 4 | Row hover wash + reset icon visible vs hidden | hover a row; change a value to show reset |
| 5 | Each control: switch off/on/pressed/focus, select closed/open/hover item/selected check, number (focus, min, max, typed invalid), text focus/invalid (Custom Search URL), textarea focus | Appearance, Editor, Browser pages |
| 6 | Section headers (visible 32 px ones: Layout, Typography, Markdown, Filters, File Tree, Cursor, Display, Privacy, Chat, per-category keybindings) vs hidden first header | scroll each page |
| 7 | Search: empty, matching one tab, matching rows, no match ("No matching settings"), keyword-only hit on blank page | type "font", "git", "zzz", "zoom" |
| 8 | Agents: list (with providers installed), loading, failed + Retry (stop daemon), empty, dragging a row, descriptor status badges | needs real provider CLIs (allowed per memory note) or mock daemon |
| 9 | Active sessions: empty; with a running agent; kill confirm dialog | start a chat |
| 10 | Keybindings: idle row, display-only chip, custom/preset chip, capturing ("Press keys..."), conflict (warning + red border), reset, "Reset all", preset select | click a chord button, press an existing chord |
| 11 | Hosts: local only; remote row up/down/needs-repair/probing; revoke confirm; Add host stages 1-3, error, pending | needs a second daemon (`crowbar pair`) in another container |
| 12 | Browser: Clear data enabled/disabled and its confirm dialog | Browser page |
| 13 | Theme light vs dark for every page (Theme Mode select) and Frosted theme | Appearance |
| 14 | UI font size 10, 15, 24 (scale effects) | Appearance |
| 15 | Narrow window < 720 px (dropdown title), < 640 px (stacked rows) | resize window |
| 16 | Close via X, Esc, backdrop click; confirm exit animation presence | |

Computed-style probes to run on the live DOM (`getComputedStyle`, via the bridge): `[data-slot=dialog-content]` rect/background/radius/shadow; `[data-slot=dialog-backdrop]` background + backdrop-filter; header height and title font; `[role=tablist]` padding/gap; one `[role=tab]` height/gap/radius/line-height/font-size for the label; `[data-settings-section]` header `Label` font-size/line-height/weight; a `SettingRow` rect and padding; reset button actual rect; `[data-slot=number-input]` children widths at each width class; switch track/thumb sizes at the app breakpoint; select trigger height; keybinding chip min-width and mono font; Agents row heights; scale values with `--app-ui-scale`.
