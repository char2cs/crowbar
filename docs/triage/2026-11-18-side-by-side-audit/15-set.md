# 15. Settings (SET)

### Open and close

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| SET-001 | Open from sidebar header gear (tooltip "Settings", below) → dialog on Appearance | web/src/components/layout/sidebar-project-header.tsx:54 | Hover gear in sidebar header (shoot tooltip), click | backdrop + popup fade 200ms ease-in-out | | | | | | | |
| SET-002 | Gear always reopens on Appearance, not the last tab (`openSettingsDialog()` defaults the tab) | web/src/features/window/stores/ui-state-store.ts:112 | Open, pick Terminal, Esc, open again via gear | — | | | | | | | |
| SET-003 | Open from a remote project's host line under the project name ("Host settings: <name>, <state>") → Hosts tab | web/src/components/layout/sidebar-project-switcher.tsx:31 | Sidebar project switcher > host line under name. Seed: project on a paired remote host | — | | | | | | | |
| SET-004 | Already-open dialog follows a deep link (tab syncs to `settingsInitialTab`) | web/src/features/settings/components/settings-dialog.tsx:31 | Not reachable from desktop UI with dialog open (modal) — see Skipped | — | | | | | | | |
| SET-005 | Enter animation: backdrop black/32 + backdrop-blur fades in; popup opacity 0→1 | web/src/components/ui/dialog.tsx:23 | Open via gear; capture frames | 200ms ease-in-out (opacity) | | | | | | | |
| SET-006 | Close with Esc (focus anywhere in dialog, not mid-capture); no exit fade — dialog unmounts at once | web/src/components/layout/ide-shell.tsx:154 | Open, `rig key tauri Escape` | none on exit | | | | | | | |
| SET-007 | Close with header X (ghost icon button, far right, sr "Close") | web/src/components/ui/dialog.tsx:256 | Open, click X | none on exit | | | | | | | |
| SET-008 | Close by clicking the backdrop outside the popup | web/src/components/ui/dialog.tsx:214 | Open, click outside dialog area | none on exit | | | | | | | |
| SET-009 | Search query cleared on close; reopen shows empty search and all tabs | web/src/features/settings/components/settings-dialog.tsx:44 | Type "font", Esc, reopen | — | | | | | | | |

### Frame

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| SET-010 | Dialog frame at 1280x800: 90vw (max 1120) × 74vh (max 820), no header border; header = "Settings" title, search field (w-64) , X | web/src/features/settings/components/settings-dialog.tsx:52 | `rig size tauri standard`, open Settings | — | | | | | | | |
| SET-011 | Rail (208px): Appearance, Editor, Files, Git, Terminal, Browser, Agents, Active sessions, Keybindings, Hosts, Developer — duotone icon 18px + heading-font 19px label | web/src/features/settings/components/settings-tab-items.ts:30 | Open Settings; shoot rail | — | | | | | | | |
| SET-012 | Rail item states: active = primary/12 pill, foreground text; hover = accent bg; others muted | web/src/features/settings/components/settings-vertical-tabs.tsx:118 | Hover Editor while Appearance active | — | | | | | | | |
| SET-013 | Rail keyboard: ArrowDown/Right next, ArrowUp/Left previous (wrap), Home first, End last; roving tabindex | web/src/features/settings/components/settings-vertical-tabs.tsx:89 | Click Appearance, `rig key tauri Down Down End Home Up` | — | | | | | | | |
| SET-014 | Rail scrolls by wheel when tabs overflow its height | web/src/features/settings/components/settings-vertical-tabs.tsx:40 | `rig size tauri 760x480`, open, scroll over rail | — | | | | | | | |
| SET-015 | Content panel: scrolls vertically, p-3; first section's heading hidden on every page | web/src/features/settings/components/settings-section.tsx:74 | Open Appearance; "Theme" heading absent, "Icons" visible | — | | | | | | | |
| SET-016 | Section heading: heading font 32px + optional muted description | web/src/features/settings/components/settings-section.tsx:84 | Appearance > "Typography" heading | — | | | | | | | |
| SET-017 | Setting row: label + muted description left, control right; hover / focus-within bg muted/50 | web/src/features/settings/components/settings-section.tsx:256 | Hover a row | bg colors 150ms (Tailwind default) | | | | | | | |
| SET-018 | Row click forwards to its control (switch toggles, select opens, number field focuses+selects) | web/src/features/settings/components/settings-section.tsx:183 | Click description text of Editor > Word Wrap; of Theme Mode | — | | | | | | | |
| SET-019 | Reset button (↺ ghost, tooltip "Reset <label>") beside label, invisible at default, appears once changed; click restores default | web/src/features/settings/components/settings-section.tsx:273 | Editor > Tab Size set 4; hover ↺; click | — | | | | | | | |
| SET-020 | Narrow (<720px): rail hidden; title becomes dropdown (active icon + label + caret); search w-44; modal 86vh × (100vw−32px) | web/src/features/settings/components/settings-layout.tsx:31 | `rig size tauri min`, open Settings | — | | | | | | | |
| SET-021 | Narrow tab dropdown: lists visible tabs with icons, active highlighted; pick switches page | web/src/features/settings/components/settings-layout.tsx:48 | At min size click title dropdown, pick Terminal | menu fade + zoom-in-95 100ms | | | | | | | |
| SET-022 | Very narrow (<640px): rows stack, label above full-width control; search w-32 below 520px | web/src/features/settings/components/settings-section.tsx:305 | `rig size tauri 600x480`, open Editor; then `rig size tauri 500x480` | — | | | | | | | |

### Search

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| SET-023 | Search field: magnifier icon, placeholder "Search settings..." | web/src/features/settings/components/settings-layout.tsx:69 | Open Settings; shoot header | — | | | | | | | |
| SET-024 | Typing filters rail to tabs with index hits and jumps to the first visible tab; rows filter by label/description substring; empty sections hide | web/src/features/settings/components/settings-vertical-tabs.tsx:31 | Click search, `rig type tauri font` | — | | | | | | | |
| SET-025 | Row filter is substring on row text, not index: "terminal" on Terminal page keeps Font Family, Font Size, Scrollback; Cursor section hidden | web/src/features/settings/lib/settings-row-search.ts:4 | Type "terminal"; open Terminal in rail | — | | | | | | | |
| SET-026 | Keybindings, Active sessions, Hosts, Developer have no index entries: vanish from rail on any query | web/src/features/settings/config/search-index.ts:3 | Type "a" | — | | | | | | | |
| SET-027 | No results: rail shows "No matching settings"; content blank (all sections hidden) | web/src/features/settings/components/settings-vertical-tabs.tsx:133 | Type "zzzz" | — | | | | | | | |
| SET-028 | Clear search: all tabs return, current tab kept, all rows visible | web/src/features/settings/store.ts:104 | After SET-024 `rig key tauri ctrl+a BackSpace` | — | | | | | | | |
| SET-029 | Search at narrow width: title dropdown lists only matching tabs | web/src/features/settings/components/settings-layout.tsx:17 | `rig size tauri min`, type "cursor", open title dropdown | — | | | | | | | |

### Appearance

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| SET-030 | Color Theme: select (wide) Crowbar / Frosted; default Crowbar | web/src/features/settings/components/tabs/appearance-settings.tsx:86 | Settings > Appearance > Color Theme: open list | — | | | | | | | |
| SET-031 | Color Theme Frosted applied app-wide live (sidebar, tabs, dialog) | web/src/features/settings/lib/settings-effects.ts:152 | Color Theme: Frosted; Esc; shoot window | — | | | | | | | |
| SET-032 | Theme Mode: select Sync with System / Light / Dark; default Sync with System | web/src/features/settings/components/tabs/appearance-settings.tsx:102 | Appearance > Theme Mode: open list | — | | | | | | | |
| SET-033 | Theme Mode Light / Dark applied app-wide live (incl. Monaco, xterm) | web/src/features/settings/lib/settings-effects.ts:158 | Theme Mode: Light; shoot; Dark; shoot | — | | | | | | | |
| SET-034 | Sync with System follows the OS scheme live | web/src/features/settings/lib/settings-effects.ts:62 | Theme Mode: Sync with System; `rig theme light`, `rig theme dark` | — | | | | | | | |
| SET-035 | Icon Theme (section Icons): select Classic / Material Icons / Colorful Material Icons / Compact / Minimal Icons / None; default Material Icons | web/src/features/settings/components/tabs/appearance-settings.tsx:120 | Appearance > Icon Theme: open list | — | | | | | | | |
| SET-036 | Icon Theme change re-renders file tree and tab icons live | web/src/extensions/icon-themes/icon-theme-initializer.ts:13 | Icon Theme: Classic, then None; Esc; ctrl+2; shoot tree | — | | | | | | | |
| SET-037 | UI Font Family (section Typography): font select, bundled first "CalSansUI (bundled)", "Geist Variable (bundled)", "Instrument Serif (bundled)", "Geist Mono Variable (bundled)", then system fonts | web/src/features/settings/components/font-selector.tsx:93 | Appearance > UI Font Family: open list | — | | | | | | | |
| SET-038 | UI Font Family change applied app-wide live | web/src/features/settings/components/tabs/appearance-settings.tsx:138 | UI Font Family: Geist Variable; shoot | — | | | | | | | |
| SET-039 | Heading Font Family: same picker, default Instrument Serif; changes section/dialog headings live | web/src/features/settings/components/tabs/appearance-settings.tsx:153 | Heading Font Family: CalSansUI; shoot "Typography" heading | — | | | | | | | |
| SET-040 | UI Font Size: number field with −/+ buttons, 10–24, step 0.5, default 15; ArrowUp/Down step | web/src/features/settings/components/tabs/appearance-settings.tsx:168 | Appearance > UI Font Size: click + twice | — | | | | | | | |
| SET-041 | UI Font Size applied live (text + icon scale, whole app incl. dialog) | web/src/features/settings/components/font-style-injector.tsx:79 | UI Font Size: set 18; shoot; set 12; shoot | — | | | | | | | |
| SET-042 | Number field out of range clamps on blur (40 → 24; empty → min) | web/src/components/ui/number-input.tsx:97 | UI Font Size: select all, type 40, Tab | — | | | | | | | |
| SET-043 | Sidebar Position (section Layout): select Left / Right (compact); moves sidebar live | web/src/features/settings/components/tabs/appearance-settings.tsx:188 | Appearance > Sidebar Position: Right; Esc; shoot | — | | | | | | | |
| SET-044 | Tab list: select Current project / Everything on this client; default Current project | web/src/features/settings/components/tabs/appearance-settings.tsx:203 | Appearance > Tab list: open list | — | | | | | | | |
| SET-045 | Sidebar follows focus: switch, default off | web/src/features/settings/components/tabs/appearance-settings.tsx:218 | Appearance > Sidebar follows focus: toggle | thumb translate 150ms, track bg 200ms | | | | | | | |
| SET-046 | Build readout: switch, default on in dev builds / off in release; toggles readout at sidebar foot live | web/src/features/settings/components/tabs/appearance-settings.tsx:232 | Appearance > Build readout: toggle; Esc; shoot sidebar foot | thumb translate 150ms, track bg 200ms | | | | | | | |

### Editor

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| SET-047 | Editor Font Family: monospace-only font select, "Geist Mono Variable (bundled)" + system mono fonts | web/src/features/settings/components/tabs/editor-settings.tsx:28 | Settings > Editor > Editor Font Family: open list | — | | | | | | | |
| SET-048 | Font Size: number 8–32, default 14 | web/src/features/settings/components/tabs/editor-settings.tsx:42 | Editor > Font Size: set 18; open a file to see | — | | | | | | | |
| SET-049 | Line Height: number 1–2, step 0.1, default 1.4 | web/src/features/settings/components/tabs/editor-settings.tsx:58 | Editor > Line Height: + | — | | | | | | | |
| SET-050 | Tab Size: number 1–8, default 2 | web/src/features/settings/components/tabs/editor-settings.tsx:75 | Editor > Tab Size: set 4 | — | | | | | | | |
| SET-051 | Word Wrap: switch, default off | web/src/features/settings/components/tabs/editor-settings.tsx:90 | Editor > Word Wrap: toggle | thumb translate 150ms, track bg 200ms | | | | | | | |
| SET-052 | Line Numbers: switch, default on | web/src/features/settings/components/tabs/editor-settings.tsx:103 | Editor > Line Numbers: toggle | thumb translate 150ms, track bg 200ms | | | | | | | |
| SET-053 | Render Whitespace: select None / Boundary / Trailing / All; default None | web/src/features/settings/components/tabs/editor-settings.tsx:116 | Editor > Render Whitespace: open list | — | | | | | | | |
| SET-054 | Indent Guides: switch, default on | web/src/features/settings/components/tabs/editor-settings.tsx:132 | Editor > Indent Guides: toggle | thumb translate 150ms, track bg 200ms | | | | | | | |
| SET-055 | Semantic Highlighting: switch, default on | web/src/features/settings/components/tabs/editor-settings.tsx:147 | Editor > Semantic Highlighting: toggle | thumb translate 150ms, track bg 200ms | | | | | | | |
| SET-056 | Highlight Occurrences: switch, default off | web/src/features/settings/components/tabs/editor-settings.tsx:162 | Editor > Highlight Occurrences: toggle | thumb translate 150ms, track bg 200ms | | | | | | | |
| SET-057 | Show Minimap: switch, default off | web/src/features/settings/components/tabs/editor-settings.tsx:177 | Editor > Show Minimap: toggle | thumb translate 150ms, track bg 200ms | | | | | | | |
| SET-058 | Max Open Tabs: number 1–100, default 100 | web/src/features/settings/components/tabs/editor-settings.tsx:189 | Editor > Max Open Tabs: set 5 | — | | | | | | | |
| SET-059 | Auto Save: switch, default off | web/src/features/settings/components/tabs/editor-settings.tsx:205 | Editor > Auto Save: toggle | thumb translate 150ms, track bg 200ms | | | | | | | |
| SET-060 | Format on Save: switch, default off | web/src/features/settings/components/tabs/editor-settings.tsx:217 | Editor > Format on Save: toggle | thumb translate 150ms, track bg 200ms | | | | | | | |
| SET-061 | Inline Blame: switch, default on | web/src/features/settings/components/tabs/editor-settings.tsx:230 | Editor > Inline Blame: toggle | thumb translate 150ms, track bg 200ms | | | | | | | |
| SET-062 | Auto Completion: switch, default on | web/src/features/settings/components/tabs/editor-settings.tsx:243 | Editor > Auto Completion: toggle | thumb translate 150ms, track bg 200ms | | | | | | | |
| SET-063 | Parameter Hints: switch, default on | web/src/features/settings/components/tabs/editor-settings.tsx:256 | Editor > Parameter Hints: toggle | thumb translate 150ms, track bg 200ms | | | | | | | |
| SET-064 | Markdown Font Size (section Markdown): number 12–24, step 1, default 16 | web/src/features/settings/components/tabs/editor-settings.tsx:271 | Editor > scroll down > Markdown Font Size: + | — | | | | | | | |

### Files

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| SET-065 | Indent Size: number 8–32, default 16; tree indent live | web/src/features/settings/components/tabs/file-tree-settings.tsx:50 | Settings > Files > Indent Size: set 28; Esc; ctrl+2 | — | | | | | | | |
| SET-066 | Density: select Compact / Default / Comfortable (rows 20 / default / 30px) | web/src/features/settings/components/tabs/file-tree-settings.tsx:68 | Files > Density: Comfortable; Esc; ctrl+2; shoot | — | | | | | | | |
| SET-067 | Compact Folders: switch, default off | web/src/features/settings/components/tabs/file-tree-settings.tsx:81 | Files > Compact Folders: toggle | thumb translate 150ms, track bg 200ms | | | | | | | |
| SET-068 | Show Hidden Files: switch, default on | web/src/features/settings/components/tabs/file-tree-settings.tsx:98 | Files > Show Hidden Files: toggle | thumb translate 150ms, track bg 200ms | | | | | | | |
| SET-069 | Respect .gitignore: switch (inverse of show-gitignored), default off | web/src/features/settings/components/tabs/file-tree-settings.tsx:118 | Files > Respect .gitignore: toggle | thumb translate 150ms, track bg 200ms | | | | | | | |
| SET-070 | Hidden Files (section Filters): 2-row textarea, placeholder "*.log, *.tmp, **/*.bak"; commits on blur or Enter (Shift+Enter newline) | web/src/features/settings/components/tabs/file-tree-settings.tsx:141 | Files > Hidden Files: type `*.md`, Enter | — | | | | | | | |
| SET-071 | Hidden Directories: textarea, placeholder "node_modules, .git, build/"; same commit rules | web/src/features/settings/components/tabs/file-tree-settings.tsx:171 | Files > Hidden Directories: type `src`, Tab | — | | | | | | | |
| SET-072 | Patterns re-normalise after commit ("a,,b " → "a, b"); reset ↺ appears | web/src/features/settings/components/tabs/file-tree-settings.tsx:14 | Hidden Files: type `a,,b `, Enter | — | | | | | | | |

### Git

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| SET-073 | Compact Git Status Badges: switch, default off (first section "Git View", heading hidden) | web/src/features/settings/components/tabs/git-settings.tsx:12 | Settings > Git > Compact Git Status Badges: toggle | thumb translate 150ms, track bg 200ms | | | | | | | |
| SET-074 | Show Git Status In File Tree (section File Tree): switch, default on | web/src/features/settings/components/tabs/git-settings.tsx:29 | Git > Show Git Status In File Tree: toggle; Esc; ctrl+2 | thumb translate 150ms, track bg 200ms | | | | | | | |

### Terminal

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| SET-075 | Font Family: select (xwide), installed Nerd Fonts "(Nerd Font)" first, then system mono; current not installed shown "(Custom)"; default Geist Mono Variable | web/src/features/settings/components/tabs/terminal-settings.tsx:63 | Settings > Terminal > Font Family: open list | — | | | | | | | |
| SET-076 | Font Family info icon tooltip "Note: Selected font must be installed…" (left) | web/src/features/settings/components/tabs/terminal-settings.tsx:82 | Hover ⓘ next to the select | — | | | | | | | |
| SET-077 | Font Size: number 8–32, default 14 | web/src/features/settings/components/tabs/terminal-settings.tsx:88 | Terminal > Font Size: set 18; ctrl+j to see | — | | | | | | | |
| SET-078 | Line Height: number 1–2, step 0.1, default 1 | web/src/features/settings/components/tabs/terminal-settings.tsx:104 | Terminal > Line Height: + | — | | | | | | | |
| SET-079 | Letter Spacing: number −5–5, step 0.1, default 0 (accepts negative) | web/src/features/settings/components/tabs/terminal-settings.tsx:123 | Terminal > Letter Spacing: − twice | — | | | | | | | |
| SET-080 | Scrollback: number 1000–100000, step 1000, default 10000 | web/src/features/settings/components/tabs/terminal-settings.tsx:142 | Terminal > Scrollback: + | — | | | | | | | |
| SET-081 | Cursor Style (section Cursor): select Block / Underline / Bar; default Block | web/src/features/settings/components/tabs/terminal-settings.tsx:163 | Terminal > Cursor Style: open list | — | | | | | | | |
| SET-082 | Blinking Cursor: switch, default on | web/src/features/settings/components/tabs/terminal-settings.tsx:180 | Terminal > Blinking Cursor: toggle | thumb translate 150ms, track bg 200ms | | | | | | | |
| SET-083 | Cursor Width: number 1–6, default 2 | web/src/features/settings/components/tabs/terminal-settings.tsx:195 | Terminal > Cursor Width: set 6 | — | | | | | | | |

### Browser

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| SET-084 | Homepage: text field (textWide), placeholder "https://example.com", saves per keystroke, default empty | web/src/features/settings/components/tabs/browser-settings.tsx:63 | Settings > Browser > Homepage: type `localhost:8000` | — | | | | | | | |
| SET-085 | Search Engine: select DuckDuckGo / Google / Bing / Brave Search / Custom; default DuckDuckGo | web/src/features/settings/components/tabs/browser-settings.tsx:78 | Browser > Search Engine: open list | — | | | | | | | |
| SET-086 | Custom Search URL row appears only when engine = Custom; placeholder "https://kagi.com/search?q=%s" | web/src/features/settings/components/tabs/browser-settings.tsx:95 | Search Engine: Custom | — | | | | | | | |
| SET-087 | Custom URL validation: no http(s) or no %s → description "Must be an http(s) address containing %s; DuckDuckGo is used until it is", field aria-invalid | web/src/features/settings/components/tabs/browser-settings.tsx:99 | Custom Search URL: type `kagi.com` | — | | | | | | | |
| SET-088 | Page Zoom (section Display): number 50–300, step 10, default 100 | web/src/features/settings/components/tabs/browser-settings.tsx:121 | Browser > Page Zoom: + | — | | | | | | | |
| SET-089 | Clear Browsing Data (section Privacy): "Clear data" outline button disabled on Linux, description "Not available on this system: …" | desktop/src-tauri/src/browser/data.rs:22 | Browser > scroll to Privacy; shoot | — | | | | | | | |
| SET-090 | Clear data enabled (macOS 14+): confirm "Clear browsing data?" then toast "Browsing data cleared" | web/src/features/settings/components/tabs/browser-settings.tsx:23 | macOS only: click Clear data > Clear data | dialog fade 200ms ease-in-out | | | | | | | |

### Agents

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| SET-091 | Page heading "Agents" (32px) + intro paragraph in body | web/src/features/settings/components/tabs/providers-settings-view.tsx:88 | Settings > Agents | — | | | | | | | |
| SET-092 | Default permission level: select Guarded / Trusted / Full Auto (daemon-owned, no reset button) | web/src/features/settings/components/tabs/default-permission-level-setting.tsx:85 | Agents > Default permission level: open list, pick Trusted | — | | | | | | | |
| SET-093 | Permission level load/save failure: reverts and toast "Could not save permission level" | web/src/features/settings/components/tabs/default-permission-level-setting.tsx:62 | Pick a level while daemon down. Seed: daemon stopped | — | | | | | | | |
| SET-094 | "Upload agent descriptor" file button + help text; shows "Adding agent…" while uploading; toast "Agent added" | web/src/features/settings/components/tabs/providers-settings.tsx:57 | Agents > Upload agent descriptor > pick file. Seed: valid YAML descriptor on disk | — | | | | | | | |
| SET-095 | List states: "Loading agents…" / "Crowbar could not load the agent list…" + Retry / "No agents available." | web/src/features/settings/components/tabs/providers-settings-view.tsx:104 | Agents with daemon slow/down. Seed: daemon stopped | — | | | | | | | |
| SET-096 | List intro ("Drag to set which agent…") + column header Installed / Tools / Enabled | web/src/features/settings/components/tabs/provider-columns.tsx:17 | Agents; shoot list header | — | | | | | | | |
| SET-097 | Provider row: drag handle ⋮⋮, provider icon, name, Installed dot (hollow = "Not installed", green = installed), Tools switch, Enabled switch | web/src/features/settings/components/tabs/sortable-provider-row.tsx:33 | Agents; hover Installed dot (title). Seed: one provider CLI installed for filled dot | — | | | | | | | |
| SET-098 | Drag reorder by handle (6px activation); dragged row 40% opacity, others shift | web/src/features/settings/components/tabs/providers-settings-view.tsx:58 | Drag first row handle below second | dnd-kit sortable transform 250ms ease | | | | | | | |
| SET-099 | Keyboard reorder: focus handle, Space, ArrowDown, Space | web/src/features/settings/components/tabs/providers-settings-view.tsx:60 | Tab to "Reorder <name>", `rig key tauri space Down space` | dnd-kit sortable transform 250ms ease | | | | | | | |
| SET-100 | Tools switch: "Let <name> use Crowbar's tools"; save failure → toast "Could not save providers" | web/src/features/settings/components/tabs/sortable-provider-row.tsx:78 | Toggle a row's Tools switch | thumb translate 150ms, track bg 200ms | | | | | | | |
| SET-101 | Enabled switch: hides provider from New chat surfaces | web/src/features/settings/components/tabs/sortable-provider-row.tsx:88 | Toggle Enabled; Esc; ctrl+t to see New chat options | thumb translate 150ms, track bg 200ms | | | | | | | |
| SET-102 | "Descriptor checks" footer: "Checking descriptors…", failure line, or per descriptor id + source + badge OK / N warnings / Blocked / Override refused, with findings list | web/src/features/agent/components/descriptor-status.tsx:29 | Agents > scroll to bottom | — | | | | | | | |
| SET-103 | Chat By Default (section Chat): switch, default on | web/src/features/settings/components/tabs/chat-presentation-setting.tsx:13 | Agents > scroll > Chat By Default: toggle | thumb translate 150ms, track bg 200ms | | | | | | | |

### Active sessions

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| SET-104 | Heading "Active sessions" + "Closing a chat only detaches from it…"; empty "No agents are running." | web/src/features/settings/components/tabs/active-sessions-settings.tsx:49 | Settings > Active sessions | — | | | | | | | |
| SET-105 | Session row: title, place path (mono), viewers label, ghost "Kill" (skull) red on hover → kill confirm | web/src/features/settings/components/tabs/active-sessions-settings.tsx:19 | Active sessions > hover Kill > click. Seed: running agent session | — | | | | | | | |

### Keybindings

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| SET-106 | Preset section (heading hidden): "Active preset" select Default / Compact (alternate) + note "the standard IDE presets … are not yet provided" | web/src/features/settings/components/tabs/keybindings-settings.tsx:137 | Settings > Keybindings; open preset list | — | | | | | | | |
| SET-107 | Preset Compact: Reopen last closed tab → Ctrl+Shift+R with "preset" tag; live | web/src/features/keymaps/defaults/keybinding-presets.ts:18 | Active preset: Compact (alternate) | — | | | | | | | |
| SET-108 | Sections Navigation, Panes, Tabs, Chats, Editor; row = keyboard icon + label + mono chord button (Ctrl+T style on Linux, ⌘T on mac) | web/src/features/settings/components/tabs/keybindings-settings.tsx:158 | Keybindings; scroll through | — | | | | | | | |
| SET-109 | Unbound command shows "Unbound" (Cycle chat provider) | web/src/features/settings/components/tabs/keybindings-settings.tsx:89 | Keybindings > Chats | — | | | | | | | |
| SET-110 | Display-only rows (Save, Save all): "display-only" chip, chord button disabled | web/src/features/settings/components/tabs/keybindings-settings.tsx:176 | Keybindings > Editor; click Save chord | — | | | | | | | |
| SET-111 | Capture: click chord → button turns primary "Press keys…"; bare modifiers ignored; next chord saved, row gets "custom" tag + "Reset" | web/src/features/settings/components/tabs/keybindings-settings.tsx:48 | Tabs > New tab chord; `rig key tauri ctrl+shift+k` | — | | | | | | | |
| SET-112 | Capture cancel with Esc: binding unchanged, dialog stays open | web/src/features/settings/components/tabs/keybindings-settings.tsx:53 | Click a chord, `rig key tauri Escape` | — | | | | | | | |
| SET-113 | Capture cancel by clicking the button again | web/src/features/settings/components/tabs/keybindings-settings.tsx:83 | Click a chord twice | — | | | | | | | |
| SET-114 | Rebind live: after SET-111 ctrl+shift+k opens New Tab page, ctrl+t does nothing | web/src/features/panes/hooks/use-pane-keyboard.ts:58 | Esc out; `rig key tauri ctrl+shift+k`, then ctrl+t | — | | | | | | | |
| SET-115 | Conflict: two commands on one chord → warning icon (title "Also bound to: …") + red border on both rows | web/src/features/settings/components/tabs/keybindings-settings.tsx:70 | New tab chord: `rig key tauri ctrl+w`; hover ⚠ | — | | | | | | | |
| SET-116 | Reset one: ghost "Reset" beside an overridden row clears it | web/src/features/settings/components/tabs/keybindings-settings.tsx:195 | After SET-111 click Reset | — | | | | | | | |
| SET-117 | "Reset all custom keybindings" outline button at page foot, only while overrides exist | web/src/features/settings/components/tabs/keybindings-settings.tsx:215 | Override two rows; scroll to bottom; click | — | | | | | | | |
| SET-118 | Preset + overrides persist across restart | web/src/features/keymaps/stores/store.ts:65 | Override New tab; restart app; Keybindings | — | | | | | | | |

### Hosts

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| SET-119 | Heading "Hosts" + "Crowbar always runs on this computer. Add another machine…" | web/src/features/settings/components/tabs/hosts-settings.tsx:94 | Settings > Hosts | — | | | | | | | |
| SET-120 | Local row: status icon, "Local", "This computer" (state appended when not up), no revoke | web/src/features/settings/components/tabs/hosts-settings.tsx:100 | Hosts; shoot first row | — | | | | | | | |
| SET-121 | Remote row: status icon, name, endpoint (mono, tinted by state, " · <state>" if not up), trash "Revoke <name>" (tooltip) | web/src/features/settings/components/tabs/hosts-settings.tsx:30 | Hosts; hover trash. Seed: paired remote host (online and offline) | — | | | | | | | |
| SET-122 | Revoke: confirm, row removed, focus moves to Add host; trash aria-disabled while revoking | web/src/features/settings/components/tabs/hosts-settings.tsx:134 | Click trash > confirm. Seed: paired remote host | — | | | | | | | |
| SET-123 | "Add host" outline button (+) opens Add host dialog (flow: see HOST) | web/src/features/settings/components/tabs/hosts-settings.tsx:113 | Hosts > Add host | dialog fade 200ms ease-in-out | | | | | | | |

### Developer

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| SET-124 | Developer tab present only in dev builds | web/src/features/settings/components/settings-tab-items.ts:48 | Rail bottom. Seed: dev build of Tauri app | — | | | | | | | |
| SET-125 | Build Badge Mode: select Auto (detected: <channel>) / Force: Development / Nightly / Beta / Release / Off + ↻ cycle button (tooltip "Cycle to the next mode") + reset | web/src/features/settings/components/tabs/developer-settings.tsx:87 | Settings > Developer > Mode: open list; click ↻ | — | | | | | | | |
| SET-126 | Build badge preview in sidebar header follows Mode live | web/src/components/layout/use-readout-lines.ts:30 | Mode: Force: Nightly; Esc; shoot sidebar header | — | | | | | | | |
| SET-127 | Agent chats > Split view: switch, default off; on adds Split to chat surface switcher | web/src/features/settings/components/tabs/chat-split-view-setting.tsx:14 | Developer > Split view: on; open seeded chat | thumb translate 150ms, track bg 200ms | | | | | | | |
| SET-128 | Diagnostics > Export diagnostics: button → "Exporting…" disabled → toast "Diagnostics exported" + path | web/src/features/settings/components/tabs/developer-settings.tsx:151 | Developer > Export diagnostics | — | | | | | | | |
| SET-129 | Backup & Restore > Export settings → crowbar-settings.json download + toast "Settings exported" | web/src/features/settings/components/tabs/developer-settings.tsx:177 | Developer > Export settings | — | | | | | | | |
| SET-130 | Import settings → JSON file picker; toast "Settings imported" or "Import failed" | web/src/features/settings/components/tabs/developer-settings.tsx:186 | Developer > Import settings > pick file. Seed: exported settings JSON + an invalid JSON | — | | | | | | | |
| SET-131 | Reset all settings (destructive) → confirm "Reset settings" (Reset all / Cancel) → toast "Settings reset" | web/src/features/settings/components/tabs/developer-settings.tsx:207 | Change a few settings; Developer > Reset all > Reset all | dialog fade 200ms ease-in-out | | | | | | | |

### Persistence

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| SET-132 | Changed settings persist across restart (debounced write) | web/src/features/settings/lib/settings-persistence.ts:58 | Appearance > Theme Mode: Light; UI Font Size 18; restart app | — | | | | | | | |

Skipped:
- `web/src/features/settings/components/mobile-settings-page.tsx`, `mobile-agents-settings.tsx`, `native-agents-settings-feed.tsx`, `native-hosts-settings-feed.tsx`, `lib/phone-settings-pages.ts` — phone only.
- `web/src/features/window/hooks/use-native-sidebar.ts:446` (openSettings intent) — phone native chrome only.
- `SettingsCard` grouped-card styling and `layout="stacked"` (settings-section.tsx:18, :248) — IS_MOBILE only.
- Settings keys with no desktop control: `syncSystemTheme`, `autoThemeLight`, `autoThemeDark`, `windowTransparency`, `sidebarWidth`, `coreFeatures.breadcrumbs`, `diffViewMode` (default-settings.ts:60-93).
- settings-dialog.tsx:31 deep-link while open — only caller with a tab (host line) is behind the modal.
- No About page and no extensions/themes page in Settings; themes surface only as Color Theme / Icon Theme options. About is the macOS app-menu item (KEY).
- `web/src/features/keymaps/hooks/use-command-shortcut.ts:2` — stub, no shortcut hints rendered.
- Split view chat surface (chat-presentation.ts:27) — dev builds only, covered by SET-127.
