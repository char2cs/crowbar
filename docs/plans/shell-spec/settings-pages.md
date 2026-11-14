# Settings modal spec (2/3): pages, field by field

Conventions: **Store** `L` = shell-local setting, persisted one key per setting in `localStorage` as `crowbar:settings:<key>` as JSON (`src/features/settings/lib/settings-persistence.ts:9-43`), written through a 300 ms trailing debounce that coalesces keys (`:54-67`); **D** = daemon HTTP (not localStorage). Every `L` write also runs `normalizeSettingValue` (`settings-normalization.ts:165-246`) before state and storage, so the stored value is the normalised one. Change applies **live** (no Save button anywhere). `Reset` = the row's reset icon appears iff value != default; it writes the default (`SettingRow onReset`). Row heights are those of section 6 in `settings.md` (about 8 + max(label+description, control) + 8 px). Defaults from `src/features/settings/config/default-settings.ts`; keys/types from `src/features/settings/types/settings.ts`.

Page root everywhere: `div.space-y-4`; first section header hidden (see `settings.md` 6).

## 1. Appearance (`tabs/appearance-settings.tsx`)

| Section (header shown?) | Row label | Description | Control | Options / range | Default | Key | Notes |
|---|---|---|---|---|---|---|---|
| Theme (hidden, first) | Color Theme | Choose your active color theme | select width 176 | from theme registry: `crowbar` "Crowbar", `frosted` "Frosted" (`src/extensions/themes/theme-registry.ts:12-24`); current value is prepended if unknown (`:56-61`) | `crowbar` | `theme` | applies by setting `data-theme` + notifying editors; unknown id normalises to default (`settings-normalization.ts:59-65`); reset `:89` |
| Theme | Theme Mode | Use light, dark, or follow system preference | select 176 | `system` "Sync with System", `light` "Light", `dark` "Dark" | `system` | `themeMode` | toggles `.dark` on `<html>`; `system` follows `prefers-color-scheme` and listens for changes (`settings-effects.ts:47-79`); invalid -> `system` |
| Icons | Icon Theme | Icons displayed in the file tree and tabs | select 176 | registry: `material` "Material Icons" (default), `colorful-material` "Colorful Material Icons", `classic` "Classic", `compact` "Compact", `minimal` "Minimal Icons", `none` "None" (`src/extensions/icon-themes/builtin/*`) | `material` | `iconTheme` | file/tab icon sets; the native tab bar and sidebar must resolve them |
| Typography | UI Font Family | Font family for UI elements (file tree, markdown, etc.) | FontSelector 192 | bundled fonts: CalSansUI, Geist Variable, Instrument Serif, Geist Mono Variable (`font-selector.tsx:21-50`, `font-store.ts:20-50`; OS fonts are not enumerable, so this is the full list) | `CalSansUI` | `uiFontFamily` | a value not in the list is silently reset to the default by an effect (`font-selector.tsx:107-111`); labels carry no "(bundled)" suffix because system list = bundled list |
| Typography | Heading Font Family | Font for headings - Markdown titles, dialog titles, settings section titles | FontSelector 192 | same list | `Instrument Serif` | `headingFontFamily` | |
| Typography | UI Font Size | Adjust UI text and icon scale in 0.5px steps | NumberInput 112, `size=xs` | min 10, max 24, step 0.5 | 15 | `uiFontSize` | snaps to 0.5 and clamps (`ui-font-size.ts:10-18`); sets `--app-ui-scale`; aria-label `UI font size: 15.00 pixels` |
| Layout | Sidebar Position | Which side the file sidebar appears on | select 112 | `left` "Left", `right` "Right" | `left` | `sidebarPosition` | |
| Layout | Tab list | Which tabs the strip lists: those of the current project, or every tab open on this client across all projects and hosts | select 112 | `project` "Current project", `all` "Everything on this client" | `project` | `tabListScope` | |
| Layout | Sidebar follows focus | Switch the sidebar between Chats, Files and Git as focus moves between chats, editors and diffs. | switch | bool | false | `sidebarFollowsFocus` | |
| Layout | Build readout | Show the build, daemon connection, frame rate and open terminal, agent and workspace counts at the foot of the sidebar, or in the tab strip while it is hidden | switch | bool | `import.meta.env.DEV` (false in release) | `buildReadout` | **Present in release builds although the inventory marks the readout as dev tooling to cut** (`docs/triage/shell-surface-inventory.md`). Needs an owner decision: port the row with the readout or drop both. |

Not in the UI but in the store: `windowTransparency` (true; sets `data-window-transparency`), `syncSystemTheme`, `autoThemeLight`, `autoThemeDark` (deprecated migration), `sidebarWidth` (220; layout owns the real width), `coreFeatures.breadcrumbs` (true), `buildBadgeOverride` ('auto').

## 2. Editor (`tabs/editor-settings.tsx`, sections "Editor" hidden-first and "Markdown")

| Row | Description | Control (width) | Range / options | Default | Key | Notes |
|---|---|---|---|---|---|---|
| Editor Font Family | Font family for code editor | FontSelector monospace-only (192) | `Geist Mono Variable` only | Geist Mono Variable | `fontFamily` | static "Geist Mono" is rewritten to "Geist Mono Variable" (`settings-normalization.ts:30-37`) |
| Font Size | Editor font size in pixels | number (96) | 8-32, step 1 | 14 | `fontSize` | |
| Line Height | Editor line height multiplier | number (96) | 1-2, step 0.1 | 1.4 | `editorLineHeight` | snapped to 0.1, clamped (`:39-46`); non-finite -> 1.4 |
| Tab Size | Number of spaces per tab | number (96) | 1-8, step 1 | 2 | `tabSize` | |
| Word Wrap | Wrap lines that exceed viewport width | switch | | false | `wordWrap` | |
| Line Numbers | Show line numbers in the editor | switch | | true | `lineNumbers` | |
| Render Whitespace | Show visible markers for spaces and tabs | select 144 | none "None", boundary "Boundary", trailing "Trailing", all "All" | none | `renderWhitespace` | invalid -> none |
| Indent Guides | Show vertical guides for indentation levels | switch | | true | `renderIndentGuides` | |
| Semantic Highlighting | Color functions, types, and properties using language servers and grammars | switch | | true | `semanticHighlighting` | |
| Highlight Occurrences | Highlight visible matches for the word under the cursor | switch | | false | `highlightOccurrences` | |
| Show Minimap | Show a minimap overview on the right side of the editor | switch | | false | `showMinimap` | |
| Max Open Tabs | Maximum number of tabs before oldest closes | number (96) | 1-100, step 1 | 100 | `maxOpenTabs` | consumed by tab layer (`features/tabs`) |
| Auto Save | Automatically save files when editing | switch | | false | `autoSave` | |
| Format on Save | Format with the language server when saving (manual saves only) | switch | | false | `formatOnSave` | |
| Inline Blame | Show who last changed the cursor's line, and when | switch | | true | `inlineBlame` | |
| Auto Completion | Show completion suggestions while typing | switch | | true | `autoCompletion` | |
| Parameter Hints | Show function parameter hints | switch | | true | `parameterHints` | |
| (section **Markdown**, header visible, 32 px) Markdown Font Size | Base text size for the rich markdown editor, in pixels | number (96), aria `Markdown font size: N pixels` | 12-24, step 1 | 16 | `markdownFontSize` | sets `--md-base-font-size` |

All rows have a reset icon. Search index lists Editor sections as Saving/Typography/Display/Markdown, which do not exist as headings in the page (the index sections are only search metadata).

## 3. Files (`tabs/file-tree-settings.tsx`)

| Section | Row | Description | Control | Range / options | Default | Key | Notes |
|---|---|---|---|---|---|---|---|
| Display (hidden-first) | Indent Size | Pixels per nesting level | number (96) | 8-32 | 16 | `fileTreeIndentSize` | rounds, clamps; non-finite -> 20 (`:48-55`) |
| Display | Density | Choose file tree row spacing | select 144 | compact "Compact", default "Default", comfortable "Comfortable" (`src/features/file-explorer/file-explorer/lib/file-tree-density.ts:5-9`) | default | `fileTreeDensity` | |
| Display | Compact Folders | Collapse single-child folder chains | switch | | false | `compactFoldersInFileTree` | |
| Display | Show Hidden Files | Show dotfiles and hidden directories | switch | | true | `showHiddenFilesInFileTree` | |
| Display | Respect .gitignore | Hide files matched by root and nested .gitignore files | switch, **inverted** (`checked = !showGitignoredFilesInFileTree`) | | key true -> switch off | `showGitignoredFilesInFileTree` | UI polarity is the opposite of the key |
| Filters (header visible) | Hidden Files | Comma-separated glob patterns | textarea 192, rows 2, placeholder `*.log, *.tmp, **/*.bak` | text <-> string[] | `[]` | `hiddenFilePatterns` | commit on blur / Enter |
| Filters | Hidden Directories | Comma-separated glob patterns | textarea, placeholder `node_modules, .git, build/` | same | `[]` | `hiddenDirectoryPatterns` | |

## 4. Git (`tabs/git-settings.tsx`)

| Section | Row | Description | Control | Default | Key |
|---|---|---|---|---|---|
| Git View (hidden-first) | Compact Git Status Badges | Use a denser layout for diff stats and staged labels in the Git panel | switch | false | `compactGitStatusBadges` |
| File Tree (visible) | Show Git Status In File Tree | Display Git color decorations in Files | switch | true | `showGitStatusInFileTree` |

`diffViewMode` ('split' default) has no row here; it is set from the review pane's own toggle but stored in the same settings (`types/settings.ts`).

## 5. Terminal (`tabs/terminal-settings.tsx`)

| Section | Row | Description | Control | Range / options | Default | Key |
|---|---|---|---|---|---|---|
| Typography (hidden-first) | Font Family | Font family for the integrated terminal. Select a Nerd Font for best icon support. | select 224 + info icon | options: installed Nerd Fonts from `COMMON_TERMINAL_NERD_FONTS` (`src/features/terminal/utils/terminal-fonts.ts:3`) labelled `<font> (Nerd Font)`, then the monospace list (only Geist Mono Variable, since OS fonts are not enumerable), plus `<value> (Custom)` for an unknown value; placeholder "Select font..." | Geist Mono Variable | `terminalFontFamily` |
| Typography | (info icon) | tooltip (left): "Note: Selected font must be installed on your system to work correctly. If icons are missing, try installing a Nerd Font." | phosphor `Info` 16 px, muted, hover foreground, `cursor-help` | | | |
| Typography | Font Size | Terminal font size in pixels | number (112) | 8-32 | 14 | `terminalFontSize` |
| Typography | Line Height | Line height multiplier | number 112 | 1-2 step 0.1 | 1 | `terminalLineHeight` (legacy 1.2 -> 1) |
| Typography | Letter Spacing | Additional spacing between characters | number 112 | -5 to 5 step 0.1 | 0 | `terminalLetterSpacing` |
| Typography | Scrollback | How many lines of terminal history to keep in memory | number 144 | 1000-100000 step 1000 | 10000 | `terminalScrollback` |
| Cursor (visible) | Cursor Style | Shape of the cursor | select 144 | block "Block", underline "Underline", bar "Bar" | block | `terminalCursorStyle` |
| Cursor | Blinking Cursor | Whether the cursor should blink | switch | | true | `terminalCursorBlink` |
| Cursor | Cursor Width | Thickness of the bar or block cursor | number 112 | 1-6 | 2 | `terminalCursorWidth` |

## 6. Browser (`tabs/browser-settings.tsx`)

| Section | Row | Description | Control | Range / options | Default | Key / source |
|---|---|---|---|---|---|---|
| Navigation (hidden-first) | Homepage | Page a new browser tab opens on; leave empty for a blank address bar | text 224, placeholder `https://example.com` | free text | "" | `browserHomepage` |
| Navigation | Search Engine | Used when the address bar text is not a web address | select 144 | duckduckgo "DuckDuckGo", google "Google", bing "Bing", brave "Brave Search", custom "Custom" (`src/features/browser/lib/search-engines.ts:3-9`) | duckduckgo | `browserSearchEngine` |
| Navigation | **Custom Search URL** (only when engine = custom) | valid: "Search address for the custom engine, with %s where the query goes"; invalid: "Must be an http(s) address containing %s; DuckDuckGo is used until it is" | text 224, placeholder `https://kagi.com/search?q=%s`, `aria-invalid` = text non-empty and not `^https?://\S+$` containing `%s` | | "" | `browserCustomSearchUrl` (`isUsableSearchTemplate`, `search-engines.ts:19-21`) |
| Display (visible) | Page Zoom | Default zoom for pages, in percent; applies to tabs opened afterwards | number 112 | 50-300 step 10 | 100 | `browserZoom` (rounded, clamped `:85-88`) |
| Privacy (visible) | Clear Browsing Data | enabled: "Delete cookies, cache and site storage of every browser tab"; disabled: "Not available on this system: browser tabs share their storage with the app itself, so clearing it would erase app data too." | outline button "Clear data" (aria "Clear browsing data"), disabled unless `browserApi.capabilities().clearData` | | | action: confirm dialog (title "Clear browsing data?", message "This signs you out of every site and deletes cached files and site storage of all browser tabs. Open pages keep what they already loaded until they reload.", confirm "Clear data", cancel default), then `browserApi.clearData()`; success toast "Browsing data cleared" / "Reload open pages to see the change."; failure reported "Could not clear browsing data" |

The Custom Search URL row has no reset-visible rule beyond the default-compare. The clear-data row has no reset.

## 7. Agents (id `providers`, `tabs/providers-settings.tsx`, `providers-settings-view.tsx`)

Layout top to bottom inside one `Section title="Agents" plain` (first section, so its header is hidden; the body re-draws it):

1. Heading block `mb-2 px-1 py-1.5`: `h3` "Agents" (heading font 32 px) + paragraph (12 px muted): "Agents are the coding CLIs Crowbar runs your chats on - Claude Code, Codex, and any others you add. Crowbar starts one for you when you open a chat, and each keeps its own session and login." (em dash in source, `providers-settings-view.tsx:91-95`).
2. **Default permission level** row (standard SettingRow in `SettingsCard`): label "Default permission level", description "How much of a new chat's tool-call approval is answered automatically.", select 144, options from `PERMISSION_LEVEL_OPTIONS` (`src/features/agent/api/agent-providers-api.ts:144-157`): `guarded` "Guarded" (Asks before edits, commands and tools.), `trusted` "Trusted" (Edits files freely. Asks before commands.), `full-auto` "Full Auto" (Never asks. Runs every command and tool.) -- the item descriptions are NOT shown in this select (labels only). Default `guarded`. **D**: `GET/PUT /v0/settings/chat/permission-level`; optimistic with generation fence; on failure revert and toast "Could not save permission level" / "Crowbar could not reach the daemon - try again."; load failure toast "Could not load permission level". (`default-permission-level-setting.tsx:156-212`). No reset icon.
3. **Upload agent descriptor**: a bordered label-button (`mx-1 inline-flex gap-2 rounded-md border px-3 py-2 text-foreground`, 12 px) "Upload agent descriptor" / "Adding agent..." while uploading, hidden `input[type=file] accept=".yaml,.yml,application/yaml,text/yaml"`; below it 11 px (`ui-text-xs`) note "Upload a YAML descriptor with a unique provider ID. Its CLI must be installed separately." **D**: `POST /v0/settings/chat/descriptors`; success toast "Agent added" / "`<id>` is now available as a provider."; then re-list; errors via `report(... "Could not add agent")`; refresh failure toast "Could not refresh agents" / "Reopen this tab to load the new provider."
4. Explanatory line (12 px muted): "Drag to set which agent a new chat opens first, and turn one off to hide it from every New chat surface. Tools lets that agent use Crowbar itself - reading your workspaces and leaving review comments; turn it off and the agent still runs, with no way in."
5. Column header row (aria-hidden): `flex gap-2 pb-1 px-1`, flexible spacer then three labels "Installed", "Tools", "Enabled", each in a 56 px (`w-14`) centred cell, 11 px muted nowrap.
6. List (`space-y-1`, 4 px) of `SortableProviderRow` inside dnd-kit `DndContext` (`PointerSensor` activation distance 6 px, `KeyboardSensor` with sortable coordinates, `closestCenter`, vertical list strategy).
   Row: `flex items-center gap-2 rounded-lg px-1 py-2 hover:bg-muted/50 transition-colors`; dragging: `relative z-10 opacity-40`.
   - Drag handle: 24 px square button, `DotsSixVertical` 16 px bold, muted, hover foreground, `cursor-grab` (`active:cursor-grabbing`), focus ring 2 px, aria "Reorder `<name>`". Only the handle starts a drag.
   - Provider icon (`ProviderIcon`, svg string from the daemon, `text-foreground`).
   - Name label 12 px, flex-1, truncate.
   - Installed cell: 8 px dot (`size-2`); connected = `bg-success`, else 1 px `border-muted-foreground/50` transparent; title/aria "Installed" / "Not installed".
   - Tools cell: Switch (`mcpEnabled`), aria "Let `<name>` use Crowbar's tools".
   - Enabled cell: Switch (`enabled`), aria "Enable `<name>`", title "Offer `<name>` when starting a chat".
7. States: `providers.length==0 && loading`: "Loading agents..." (12 px muted, py-2, px-1); `failed`: "Crowbar could not load the agent list - it could not reach the daemon." + `Button xs outline` "Retry"; empty & ready: "No agents available." Column header and the explanatory line only render over a real list.
8. Footer `DescriptorStatus` (`src/features/agent/components/descriptor-status.tsx`): `space-y-2 px-1 pt-3`; `h4` "Descriptor checks" (12 px weight 500 foreground); loading "Checking descriptors..."; failed "Crowbar could not check the descriptors - it could not reach the daemon."; per report: id (12 px foreground) + source (`shipped` if null, 11 px muted truncate) + Badge: fellBack -> warning "Override refused - using shipped"; errors -> error "Blocked"; warnings -> warning "N warning(s)"; else success "OK"; findings list `pl-3 space-y-1` 11 px muted: `<severity> <rule> at <path|(document)> line N: <message>` and optional "Fix: <hint>", error severity in destructive colour. Data `GET /v0/settings/chat/descriptors`.
9. Section **Chat** (visible header, 32 px) with row "Chat By Default": description "Open a chat on Chat rather than Terminal. Both surfaces stay in every chat either way - this only picks the one you land on."; switch; default true; key `chatIsDefaultPresentation` (**L**); reset icon.

**D** write path for providers: any toggle/reorder PUTs `/v0/settings/chat/providers` with the complete ordered list `[{id, disabled, mcpDisabled}]` (negative polarity), optimistic update, generation-fenced; failure restores previous list and toasts "Could not save providers" / "Crowbar could not reach the daemon - try again." (`use-providers-settings.ts:64-121`, `provider-preferences.ts`). Reads `GET /v0/settings/chat/providers`. List item fields (`AgentProvider`): id, displayName, icon (svg), connected, enabled, mcpEnabled, modelSelect, effortSelect, compaction, etc.

## 8. Active sessions (`tabs/active-sessions-settings.tsx`)

Body heading (own `h3` "Active sessions", 32 px) + "Closing a chat only detaches from it; its agent keeps running. Kill one here to end it." Empty: "No agents are running." (12 px muted, px-1). List `space-y-1`; row `flex items-center gap-3 rounded-lg px-1 py-2 hover:bg-muted/50`: text column (title 12 px foreground truncate; route path 12 px mono muted truncate, falls back to first 8 chars of chat id; viewers label "No viewers" / "1 viewer" / "N viewers" 12 px muted), then ghost small `Button` with Skull icon + "Kill" (aria "Kill the agent on `<title>`"; muted, hover `text-destructive`). Click -> `confirmAndKillAgent(session)` (`src/features/agent/lib/kill-agent.ts`). Data: live entity store `useActiveSessionList()` (`src/core/entities/active-sessions.ts`), keyed `hostId+chatId`, derived from chat + presence entities pushed by the daemon stream; no polling.

## 9. Keybindings (`tabs/keybindings-settings.tsx`)

Store: `useKeymapStore` (`src/features/keymaps/stores/store.ts`) persists `crowbar:settings:keybindingPreset` (default `default`) and `crowbar:settings:keybindingUserOverrides` (commandId -> normalised chord) immediately (no debounce). Resolution: override > preset > default (`utils/effective-keymaps.ts:20-30`).

Chord grammar (`utils/chord.ts`): lowercase, `+`-separated, order `mod`, `shift`, `alt`, key. `mod` = Cmd on Apple keyboards, Ctrl elsewhere. Capture normalises `event.key.toLowerCase()`; bare modifiers ignored. Label: Apple `⌘⇧⌥K` joined without separator; others `Ctrl+Shift+Alt+K`; arrows as arrows; other keys upper-cased.

Sections in order (`CATEGORY_ORDER`): **Preset** (header hidden, first!), Navigation, Panes, Tabs, Chats, Editor.

- Preset block: row `flex items-center justify-between px-1 py-1`: label "Active preset" (12 px muted) + select 224 with options default "Default", compact "Compact (alternate)" (`defaults/keybinding-presets.ts:23-26`); note below `px-1` 12 px muted: "Note: the standard IDE presets (VS Code, JetBrains, Sublime, Xcode, Atom, Emacs, Zed) are not yet provided - only Default and one alternate (Compact) ship real bindings." The Section's own description ("Choose a base keybinding set. ...") never shows (hidden header). `compact` only rebinds `tabs.reopenClosed` to `mod+shift+r`.
- Each category section: visible 32 px title; rows in `div.divide-y.divide-border/60`; row `flex items-center justify-between gap-3 px-1 py-2`: left = 14 px `Keyboard` icon (muted) + label (12 px, truncate) + optional chips: "display-only" (11 px, `rounded bg-muted px-1.5 py-0.5` muted) when not liveEditable; "custom" or "preset" (11 px `text-accent-foreground`) when `source != default`. Right = rebind control + optional "Reset" ghost (11 px muted) when overridden.
- Rebind control (`RebindControl :28-93`): `Button compact`, variant outline when idle / default (primary fill) while capturing, `min-w-[88px]` mono 12 px centred, text = formatted chord or "Unbound"; capturing text "Press keys..."; disabled when `!liveEditable`. Conflict (another command has the same normalised effective chord): 14 px duotone `Warning` icon (destructive) with title "Also bound to: <labels>" before the button and the button gets `border-destructive text-destructive`. Conflicts only warn; the override is still saved.
- Capture mode: window-level `keydown` listener in capture phase, always `preventDefault` + `stopPropagation` (global shortcuts cannot fire); Esc cancels; a real key saves immediately `setOverride(commandId, chord)` and exits capture; clicking the button again toggles capture off. Only one capture per control (no cross-row exclusivity logic).
- Footer: when any override exists, outline button "Reset all custom keybindings" (`px-1` wrapper).

Registry (`src/features/keymaps/registry.ts:39-225`), 25 commands; display `mod` as per platform:

| Category | Command id | Label | Default chord | Live editable |
|---|---|---|---|---|
| Navigation | navigation.toggleConsole | Toggle console | mod+` | yes |
| Navigation | navigation.sidebarChats | Sidebar: Chats | mod+1 | yes |
| Navigation | navigation.sidebarFiles | Sidebar: Files | mod+2 | yes |
| Navigation | navigation.sidebarGit | Sidebar: Git | mod+3 | yes |
| Panes | panes.navigateLeft/Right/Up/Down | Focus pane left/right/up/down | mod+alt+Arrow* | yes |
| Tabs | tabs.new | New tab | mod+t | yes |
| Tabs | tabs.newTerminal | New terminal tab | mod+j | yes |
| Tabs | agent.newChat | New chat | mod+n | yes |
| Tabs | agent.newChatTerminal | New chat (Terminal) | mod+alt+n | yes |
| Tabs | tabs.newFile | New file | mod+shift+n | yes |
| Tabs | tabs.openFile | Open file... | mod+o | yes |
| Tabs | tabs.reopenClosed | Reopen last closed tab | mod+shift+t | yes |
| Tabs | tabs.closeActive | Close active tab | mod+w | yes |
| Tabs | browser.togglePick | Pick element in browser tab | mod+shift+e | yes |
| Chats | agent.toggleViewMode | Toggle chat / terminal view | mod+/ | yes |
| Chats | agent.cycleProvider | Cycle chat provider | (unbound) | yes |
| Chats | agent.zoomIn/zoomOut/zoomReset | Zoom chat in / out / Reset chat zoom | mod+= / mod+- / mod+0 | yes |
| Editor | editor.save | Save | mod+s | no (display-only, hardcoded in `use-save-keyboard.ts`) |
| Editor | editor.saveAll | Save all | mod+shift+s | no |

Hardcoded, not in the registry and not editable: project switching `mod+shift+<digit>` (`use-space-switcher-keyboard.ts`). Within the order of render, the settings render command order of the `COMMANDS` array inside each category (Tabs: new, newTerminal, newChat, newChatTerminal, newFile, openFile, reopenClosed, closeActive, browser pick).

## 10. Hosts (`tabs/hosts-settings.tsx`, only in the desktop app)

Body heading `h3` "Hosts" (32 px) + description "Crowbar always runs on this computer. Add another machine running its own daemon to reach it from here too." Then `space-y-1` rows, then an `px-1` outline button "Add host" (`Plus` icon, `size=sm`).

Host row: `flex items-center gap-3 rounded-lg px-1 py-2 hover:bg-muted/50`: status icon (`HostStatusIcon`: 16 px `Desktop` for Local / `Cloud` for remote, regular weight, muted; with an 5 px (`size-1.25`) status dot at bottom-right, offset -1 px, the icon masked by a radial cut-out `transparent 3.75px, black 4.25px` centred 1.5 px inside the corner) then text: name (12 px foreground truncate) and a second line (12 px): Local -> "This computer" (never tinted); remote -> endpoint in mono, tinted by tone, plus ` - <state word>` when state != up. Remote rows end with a ghost `icon-sm` (28 px) trash button (`Trash` icon, muted, hover destructive, tooltip "Revoke `<name>`", aria-disabled + 64% opacity while revoking).

Status dot colours (`host-status-icon.tsx:10-16`): online -> `--success`; offline -> `--destructive`; needs-attention -> `--warning-foreground` in light / `--warning` in dark; unsettled -> 1 px ring `muted-foreground/65`, transparent. Text tone (`host-tone.ts:44-56`): light uses the `*-foreground` token, dark the base token.

State words (`host-tone.ts:5-13`): unknown "Status unknown", probing "Checking...", up "Online", down "Offline", needs-repair "Needs pairing again", no-live-stream "Online - no live stream", not-available "Host not available". Tone map: up/no-live-stream -> online; down/not-available -> offline; needs-repair -> needs-attention; unknown/probing -> unsettled.

Status source: local = connection store (`connected`->up, `disconnected`/`migrating`->down, `idle`->unknown); remote = prober + stream failures (`src/features/hosts/lib/host-status.ts`, `host-prober.ts`).

Revoke flow (`host-revoke.ts`): confirm dialog "Revoke `<name>`?" / "To use it again you'll need to run crowbar pair on it." (confirm "Revoke" destructive, cancel "Keep it"); DELETE `/v0/clients/<clientId>` on that host; on failure further confirms depending on failure kind (unreachable / identity-changed / refused / other) with "Remove anyway"; local cleanup `unregisterRemoteHost` + `forgetHostToken` (Tauri commands), toast on partial failure, then `removeHost`. After success focus moves to "Add host".

Add host dialog (`src/features/hosts/components/add-host-dialog.tsx`): standard `DialogPopup` (`max-w-md` = 448 px, `rounded-2xl`, header `p-6`, title `font-heading text-xl` "Add remote host", footer `px-6 py-4 border-t bg-muted/72`, backdrop as section 2 of `settings.md`, scale 98%->100% + opacity). Three stages:
1. Address: paragraph "Run crowbar pair on the host first: it shows the fingerprint and the code you'll need."; Name input (placeholder `homelab`, maxLength 64); Address input (placeholder `homelab.example.com or 1.2.3.4`, mono 12 px) with hint "Uses port 3737 unless you add one, like :4000."; error line (12 px destructive); footer Cancel (ghost) / Continue (primary; "Connecting..." while probing). Address normalisation `normalizeEndpoint` (`lib/endpoint.ts`): strips `http(s)://`/`tcp://`, adds `:3737`, validates DNS label / IPv4 / IPv6, port 1-65535; messages "Enter a host name or IP address, optionally with a port, like homelab.example.com:3737." and "The port must be a number from 1 to 65535."
2. Fingerprint: "Confirm this matches the fingerprint printed by that host's CLI:" + mono 12 px break-all box (`rounded-lg border bg-muted/50 px-3 py-2`), focused on arrival; footer Back / Confirm fingerprint.
3. Code: label "Pairing code", 6-slot OTP field with a separator after slot 3, numeric only (NFKC digits), hint "The 6-digit code crowbar pair printed on the host.", live notice "A pairing code is exactly 6 digits."; footer Back / Pair ("Pairing..."; aria-disabled until 6 digits). Dialog cannot be dismissed while probe/pair is pending. Pairing calls `probeRemoteFingerprint` (Tauri) then `redeemPairingCode`, host id = slug + 8 random hex chars; hosts persisted in `localStorage` key `crowbar.hosts` (`stores/host-config-store.ts`).
