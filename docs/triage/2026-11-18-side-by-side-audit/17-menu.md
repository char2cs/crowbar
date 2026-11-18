# 17. Menus and context menus (MENU)

### Native app menu (macOS only; OS-drawn)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| MENU-001 | No application menu bar on Linux: the menu is built only under `cfg(target_os = "macos")` | desktop/src-tauri/src/lib.rs:89 | Linux: `rig reset`; observe the window top edge (no File/Edit/Window bar) | — | | | | | | | |
| MENU-002 | Menu bar layout on macOS: Crowbar, Edit, Window (no File, View or Help) | desktop/src-tauri/src/app_menu.rs:57 | macOS: observe the menu bar | — (OS-drawn) | | | | | | | |
| MENU-003 | Crowbar > About Crowbar: native About panel from default metadata (name, version, icon) | desktop/src-tauri/src/app_menu.rs:19 | macOS: Crowbar > About Crowbar | — (OS-drawn) | | | | | | | |
| MENU-004 | Crowbar > Services submenu (system services), after a separator | desktop/src-tauri/src/app_menu.rs:21 | macOS: Crowbar > Services | — (OS-drawn) | | | | | | | |
| MENU-005 | Crowbar > Hide Crowbar, Cmd+H | desktop/src-tauri/src/app_menu.rs:23 | macOS: Crowbar > Hide Crowbar, or Cmd+H | — (OS-drawn) | | | | | | | |
| MENU-006 | Crowbar > Hide Others, Cmd+Opt+H | desktop/src-tauri/src/app_menu.rs:24 | macOS: Crowbar > Hide Others | — (OS-drawn) | | | | | | | |
| MENU-007 | Crowbar > Show All | desktop/src-tauri/src/app_menu.rs:25 | macOS: Crowbar > Show All | — (OS-drawn) | | | | | | | |
| MENU-008 | Crowbar > Quit Crowbar, Cmd+Q, after a separator; no quit confirmation | desktop/src-tauri/src/app_menu.rs:27 | macOS: Cmd+Q | — (OS-drawn) | | | | | | | |
| MENU-009 | Edit > Cut, Cmd+X (fired as a DOM event Monaco intercepts) | desktop/src-tauri/src/app_menu.rs:32 | macOS: select text in src/pricing.ts > Edit > Cut | — (OS-drawn) | | | | | | | |
| MENU-010 | Edit > Copy, Cmd+C | desktop/src-tauri/src/app_menu.rs:33 | macOS: select text > Edit > Copy | — (OS-drawn) | | | | | | | |
| MENU-011 | Edit > Paste, Cmd+V | desktop/src-tauri/src/app_menu.rs:34 | macOS: Edit > Paste into the composer | — (OS-drawn) | | | | | | | |
| MENU-012 | Edit has no Undo, Redo or Select All: Cmd+Z / Cmd+Shift+Z / Cmd+A reach the webview (Monaco's model) | desktop/src-tauri/src/app_menu.rs:1 | macOS: open Edit menu; then type in src/pricing.ts and press Cmd+Z | — | | | | | | | |
| MENU-013 | Window > New Window, Cmd+Opt+N: opens a new window at `/` | desktop/src-tauri/src/app_menu.rs:39 | macOS: Window > New Window | — (OS-drawn) | | | | | | | |
| MENU-014 | Window > Toggle Console: accelerator mirrors the keymap chord (default Cmd+`), cleared if the chord is not expressible; toggles the focused window's console | desktop/src-tauri/src/console_menu.rs:75 | macOS: Window > Toggle Console; rebind the chord in Settings > Keybindings and reopen the menu | — (OS-drawn) | | | | | | | |
| MENU-015 | Window > Close Window, Cmd+Shift+W: closes the focused window (same path as the traffic light); stays enabled, no-op when none is focused | desktop/src-tauri/src/app_menu.rs:43 | macOS: Window > Close Window | — (OS-drawn) | | | | | | | |
| MENU-016 | Window > separator + Minimize, Cmd+M | desktop/src-tauri/src/app_menu.rs:54 | macOS: Window > Minimize | — (OS-drawn) | | | | | | | |

### Native context menu mechanism

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| MENU-017 | Every `ContextMenu` in the desktop app is OS-drawn (GTK on Linux, NSMenu on macOS) at the pointer; nothing is drawn in the DOM | web/src/components/ui/context-menu.tsx:218 | Sidebar > right-click Perf fixture chat row | — (OS-drawn) | | | | | | | |
| MENU-018 | Native items show no icons, no destructive red, no keybinding hint (no item sets an accelerator); disabled items greyed; submenus cascade natively | web/src/lib/crowbar-bridge.ts:308 | Sidebar > right-click checkout (main) row; compare Delete Repo to other items | — (OS-drawn) | | | | | | | |
| MENU-019 | Keyboard in native menus is OS-handled: Up/Down, Right/Left into/out of submenus, Enter, Esc; no type-ahead | web/src/lib/crowbar-bridge.ts:355 | right-click Perf fixture chat row; `rig key <app> Down Down Return`, then reopen and `Escape` | — | | | | | | | |
| MENU-020 | Opening a second native menu closes the first; choosing an item closes the menu after its action runs | web/src/lib/crowbar-bridge.ts:347 | right-click one sidebar row, then right-click another without choosing | — | | | | | | | |
| MENU-021 | Rendered fallback popup when the native popup throws: base-ui menu, min-w 180px, z 10040, icons, kbd hints, destructive red | web/src/components/ui/context-menu.tsx:241 | not drivable. Seed: native menu failure | — (no data-starting/ending style) | | | | | | | |
| MENU-022 | Unhandled right-click (transcript text, composer, Monaco, terminal) opens no app menu; Monaco's own menu disabled; only the webview's default menu, if any | web/src/features/editor/lib/monaco-adapters.ts:37 | right-click chat transcript text; right-click inside src/pricing.ts editor; right-click a terminal | — | | | | | | | |

### Sidebar row menu (native)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| MENU-023 | Open: right-click any sidebar row (tree or view-tab strip); one row only, no multiselect | web/src/components/sidebar/row-context-menu.tsx:76 | Sidebar > right-click Perf fixture chat row | — (OS-drawn) | | | | | | | |
| MENU-024 | Open from the project-home row's "..." button: same menu, placed under the button (left, bottom+4px) | web/src/components/sidebar/row-context-menu.tsx:89 | Sidebar > hover checkout (main) row > click "..." | — (OS-drawn) | | | | | | | |
| MENU-025 | Rename: every row except a locked branch; starts the inline rename editor | web/src/components/sidebar/lib/row-menu.ts:172 | right-click Perf fixture chat row > Rename | — | | | | | | | |
| MENU-026 | Lock: branch rows with a worktree (not project home, not placeholder), while unlocked | web/src/components/sidebar/lib/row-menu.ts:174 | right-click feature/pricing-rounding row > Lock | — | | | | | | | |
| MENU-027 | Unlock: same rows while locked; Rename is then absent | web/src/components/sidebar/lib/row-menu.ts:174 | after MENU-026, right-click feature/pricing-rounding row > Unlock | — | | | | | | | |
| MENU-028 | Import branches: project-home row only; opens the repo import dialog | web/src/components/sidebar/lib/row-menu.ts:175 | right-click checkout (main) row > Import branches | — | | | | | | | |
| MENU-029 | New folder: branch, folder and chat rows; a chat's folder roots at the space top, then opens on its name | web/src/components/sidebar/lib/row-menu.ts:177 | right-click feature/pricing-rounding row > New folder | — | | | | | | | |
| MENU-030 | New thread in Terminal: same rows, present only if the first enabled provider can start on a terminal (absent, never disabled) | web/src/components/sidebar/lib/row-menu.ts:178 | right-click feature/pricing-rounding row; look for the item. Seed: enabled provider with terminal support | — | | | | | | | |
| MENU-031 | Move to… submenu: each folder, repo or branch a drag could drop the row into (branch rows by branch name); absent with no target | web/src/components/sidebar/row-context-menu.tsx:162 | right-click Perf fixture chat row > Move to… | — (OS submenu) | | | | | | | |
| MENU-032 | Move to… > target: commits as that drop | web/src/components/sidebar/row-context-menu.tsx:170 | right-click Perf fixture chat row > Move to… > first target | — | | | | | | | |
| MENU-033 | Make a branch: chat that owns no worktree and is not moving; starts inline branch naming | web/src/components/sidebar/row-context-menu.tsx:188 | right-click a chat under the project home. Seed: chat in the main (home) workspace | — | | | | | | | |
| MENU-034 | Cancel move: chat whose move waits for its turn to end | web/src/components/sidebar/row-context-menu.tsx:178 | right-click the moving chat > Cancel move. Seed: chat move pending behind a running turn | — | | | | | | | |
| MENU-035 | Separator + Kill agent: chat with an active session; opens the kill confirmation | web/src/features/agent/hooks/use-kill-agent-menu-items.tsx:19 | right-click a working chat row > Kill agent. Seed: running agent | — | | | | | | | |
| MENU-036 | Separator + Delete Repo: project-home row; holds the repo for removal and asks to confirm; toast "Can't delete <repo> yet" if nothing held | web/src/components/sidebar/lib/row-menu.ts:180 | right-click checkout (main) row > Delete Repo | — | | | | | | | |
| MENU-037 | Item order: tab verbs, Rename, Lock/Unlock, Import branches, New folder, New thread in Terminal, Move to…, Make a branch, Cancel move, Kill agent, separator, Delete Repo | web/src/components/sidebar/row-context-menu.tsx:143 | right-click checkout (main) row and feature/pricing-rounding row; compare order | — | | | | | | | |
| MENU-038 | View-tab strip chat tab: Pin Tab (Unpin Tab when pinned) leads the menu | web/src/components/sidebar/hooks/use-view-tab-menu-items.tsx:14 | right-click a chat tab in the top view-tab strip > Pin Tab; reopen > Unpin Tab | — | | | | | | | |
| MENU-039 | View-tab strip chat tab: Close, then a separator before the row's own items | web/src/components/sidebar/hooks/use-view-tab-menu-items.tsx:19 | right-click a chat tab in the view-tab strip > Close | — | | | | | | | |

### Tree foot "More…" menu (native)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| MENU-040 | Open: More… under the tree foot hairline; menu at the button's bottom-left +4px | web/src/components/sidebar/tree-foot.tsx:87 | Sidebar > scroll to tree end > More… | — (OS-drawn) | | | | | | | |
| MENU-041 | Import a repo: opens the add-repository flow | web/src/components/sidebar/tree-foot.tsx:56 | More… > Import a repo | — | | | | | | | |
| MENU-042 | Create a new folder | web/src/components/sidebar/tree-foot.tsx:58 | More… > Create a new folder | — | | | | | | | |
| MENU-043 | Delete space (red only in the fallback popup): holds the space for removal | web/src/components/sidebar/tree-foot.tsx:64 | More… > Delete space (then Cancel in the confirm) | — | | | | | | | |

### Editor/terminal tab menu (native)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| MENU-044 | Open: right-click a tab in a pane's tab strip | web/src/features/tabs/components/tab-bar-item.tsx:127 | Files sidebar > open README.md; right-click its tab | — (OS-drawn) | | | | | | | |
| MENU-045 | Pin Tab / Unpin Tab | web/src/features/tabs/components/tab-context-menu.tsx:65 | right-click README.md tab > Pin Tab; reopen > Unpin Tab | — | | | | | | | |
| MENU-046 | Rename (terminal tabs only): opens the Rename Terminal prompt | web/src/features/tabs/components/tab-context-menu.tsx:73 | `rig key <app> ctrl+j`; right-click the terminal tab > Rename | — | | | | | | | |
| MENU-047 | Copy Path: absolute path (terminal: its synthetic path) | web/src/features/tabs/components/tab-context-menu.tsx:104 | right-click README.md tab > Copy Path; paste in composer | — | | | | | | | |
| MENU-048 | Copy Relative Path: relative to the workspace | web/src/features/tabs/components/tab-context-menu.tsx:121 | right-click README.md tab > Copy Relative Path; paste in composer | — | | | | | | | |
| MENU-049 | Reveal in Finder (same label on Linux): only for a tab on a local-host workspace; opens the OS file manager | web/src/features/tabs/components/tab-context-menu.tsx:129 | right-click README.md tab > Reveal in Finder | — | | | | | | | |
| MENU-050 | Open in Terminal: non-virtual tab whose path has no `://`; opens a terminal in the file's folder | web/src/features/tabs/components/tab-context-menu.tsx:139 | right-click README.md tab > Open in Terminal | — | | | | | | | |
| MENU-051 | Reload: non-virtual editor tab; a dirty tab asks "Discard unsaved changes…" first | web/src/features/tabs/components/tab-context-menu.tsx:158 | right-click README.md tab > Reload | — | | | | | | | |
| MENU-052 | Separator + Close (fallback shows Ctrl W hint; native shows none) | web/src/features/tabs/components/tab-context-menu.tsx:167 | right-click README.md tab > Close | — | | | | | | | |
| MENU-053 | Close Others: this pane's other tabs, skipping pinned and uncloseable | web/src/features/tabs/components/tab-context-menu.tsx:175 | open README.md, src/pricing.ts, src/inventory.ts; pin one; right-click another > Close Others | — | | | | | | | |
| MENU-054 | Close to Right | web/src/features/tabs/components/tab-context-menu.tsx:180 | three tabs open; right-click the first > Close to Right | — | | | | | | | |
| MENU-055 | Close All (pinned and uncloseable stay) | web/src/features/tabs/components/tab-context-menu.tsx:184 | right-click any tab > Close All | — | | | | | | | |

### Tab overflow "More tabs" (base-ui, web-drawn)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| MENU-056 | Caret "More tabs" trigger appears only while the strip overflows; re-checked on tab and size change | web/src/features/tabs/components/tab-overflow-menu.tsx:45 | `rig size <app> min`; open README.md, src/pricing.ts, src/inventory.ts | — | | | | | | | |
| MENU-057 | Items: every open tab by name, active one semibold; click selects; menu aligned end, max-w 80 | web/src/features/tabs/components/tab-overflow-menu.tsx:54 | click More tabs > an inactive tab | — | | | | | | | |
| MENU-058 | Keyboard (base-ui Menu): Up/Down/Home/End with loop, Enter/Space activates, Esc closes and refocuses trigger, type-ahead by label | web/src/components/ui/dropdown-menu.tsx:7 | open More tabs; `rig key <app> Down Down Return`; reopen, type a tab's first letter, `Escape` | — | | | | | | | |
| MENU-059 | Open/close animation of every base-ui DropdownMenu | web/src/components/ui/dropdown-menu.tsx:40 | open and close More tabs | 100ms ease: fade 0→1, zoom 95→100%, slide 0.5rem from the anchor side; close fades/zooms out | | | | | | | |

### File explorer menu (native)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| MENU-060 | Open: right-click a file or folder in the Files sidebar; right-click empty tree space = the root folder's menu; position clamped to keep 250×400 on screen | web/src/features/file-explorer/file-explorer/hooks/use-tree-container-events.ts:153 | Files sidebar > right-click README.md; right-click src; right-click empty space below the tree | — (OS-drawn) | | | | | | | |
| MENU-061 | Folder: New File (hidden when locked): inline name input | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx:168 | right-click src > New File | — | | | | | | | |
| MENU-062 | Folder: New Folder (hidden when locked) | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx:175 | right-click src > New Folder | — | | | | | | | |
| MENU-063 | Folder: Upload File (workspace set, not locked): OS file chooser, uploads into the folder | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx:183 | right-click src > Upload File; cancel the chooser | — | | | | | | | |
| MENU-064 | Folder: Refresh | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx:191 | right-click src > Refresh | — | | | | | | | |
| MENU-065 | Folder: Open All Files: opens every file in tabs; over 100 asks first | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx:199 | right-click src > Open All Files | — | | | | | | | |
| MENU-066 | Folder: Collapse All (root collapses everything, a folder its subtree) | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx:205 | expand src; right-click empty space > Collapse All | — | | | | | | | |
| MENU-067 | Folder: Open in Terminal (terminal named after the folder) | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx:218 | right-click src > Open in Terminal | — | | | | | | | |
| MENU-068 | File: Open | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx:264 | right-click README.md > Open | — | | | | | | | |
| MENU-069 | File: Copy Content (file text to clipboard; error toast on failure) | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx:272 | right-click README.md > Copy Content; paste in composer | — | | | | | | | |
| MENU-070 | Env file: separator + Create .env.example / Create .env.local / Create .env.development (not locked) | web/src/features/file-explorer/file-explorer/lib/env-template.ts:7 | right-click .env > Create .env.example. Seed: .env file in the workspace | — | | | | | | | |
| MENU-071 | File: Properties: opens the Properties dialog | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx:291 | right-click README.md > Properties | — | | | | | | | |
| MENU-072 | Copy Path (absolute); row flashes feedback 1.5s | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx:305 | right-click README.md > Copy Path | — | | | | | | | |
| MENU-073 | Copy Relative Path (`.` for the root) | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx:321 | right-click src/pricing.ts > Copy Relative Path | — | | | | | | | |
| MENU-074 | Reveal in Finder (same label on Linux): local-host workspace only | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx:337 | right-click README.md > Reveal in Finder | — | | | | | | | |
| MENU-075 | Copy (stages the file for paste) | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx:354 | right-click README.md > Copy; select src; `rig key <app> ctrl+v` | — | | | | | | | |
| MENU-076 | Cut (hidden when locked) | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx:365 | right-click README.md > Cut | — | | | | | | | |
| MENU-077 | Duplicate (not the root, not locked) | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx:390 | right-click README.md > Duplicate | — | | | | | | | |
| MENU-078 | Rename (not the root, not locked): inline rename | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx:398 | right-click README.md > Rename; `Escape` | — | | | | | | | |
| MENU-079 | Separator + Delete (red in fallback): opens Delete File / Delete Folder dialog | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx:405 | right-click README.md > Delete; Cancel | — | | | | | | | |
| MENU-080 | Locked workspace hides New File/Folder, Upload, env items, Cut, Duplicate, Rename, Delete; read-only items stay | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx:164 | Lock feature/pricing-rounding (MENU-026); Files > right-click README.md and src | — | | | | | | | |
| MENU-081 | Root menu (empty space): folder items address the root; no Rename, Duplicate or Delete | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx:157 | Files > right-click empty space below the tree | — | | | | | | | |

### Terminal link menu (native)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| MENU-082 | Open: Ctrl+click (Cmd on macOS) a URL in a terminal, plain or OSC 8; menu at the pointer | web/src/features/terminal/lib/terminal-links.ts:187 | `rig key <app> ctrl+j`; `rig type <app> echo https://example.com`; Return; ctrl+click the URL | — (OS-drawn) | | | | | | | |
| MENU-083 | Open in Default Browser (OS default handler) | web/src/features/terminal/lib/terminal-url-menu.ts:32 | link menu > Open in Default Browser | — | | | | | | | |
| MENU-084 | Open in Crowbar Browser: disabled when the terminal has no pane; opens a browser tab in the terminal's own pane | web/src/features/terminal/lib/terminal-url-menu.ts:37 | link menu > Open in Crowbar Browser | — | | | | | | | |
| MENU-085 | Copy Link (toast "Could not copy link" on failure) | web/src/features/terminal/lib/terminal-url-menu.ts:48 | link menu > Copy Link; paste in composer | — | | | | | | | |

### Markdown block menu (native)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| MENU-086 | Open: right-click a block in the rich markdown editor (not the editor root, not read-only); selects the block | web/src/components/ui/block-context-menu.tsx:114 | Files > open README.md (rich view); right-click a paragraph | — (OS-drawn) | | | | | | | |
| MENU-087 | Delete: removes the selected blocks | web/src/components/ui/block-context-menu.tsx:60 | block menu > Delete | — | | | | | | | |
| MENU-088 | Duplicate | web/src/components/ui/block-context-menu.tsx:68 | block menu > Duplicate | — | | | | | | | |
| MENU-089 | Turn into submenu: Paragraph, Heading 1, Heading 2, Heading 3, Blockquote | web/src/components/ui/block-context-menu.tsx:73 | block menu > Turn into > Heading 2 | — (OS submenu) | | | | | | | |
| MENU-090 | Separator + Indent | web/src/components/ui/block-context-menu.tsx:90 | block menu > Indent | — | | | | | | | |
| MENU-091 | Outdent | web/src/components/ui/block-context-menu.tsx:95 | after MENU-090, block menu > Outdent | — | | | | | | | |
| MENU-092 | Align submenu: Left, Center, Right | web/src/components/ui/block-context-menu.tsx:100 | block menu > Align > Center | — (OS submenu) | | | | | | | |
| MENU-093 | Dismiss refocuses the block selection | web/src/components/ui/block-context-menu.tsx:48 | open block menu; `rig key <app> Escape`; press Down | — | | | | | | | |

### Composer "+" menu (base-ui, web-drawn)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| MENU-094 | Open: composer "+" (Add to this message); menu above, aligned end, 8px gap | web/src/features/agent/composer/composer-plus-button.tsx:190 | open Perf fixture chat; click "+" in the composer | 100ms ease fade/zoom 95%, slide from bottom | | | | | | | |
| MENU-095 | Attach files… with hint Ctrl+U: opens the attach modal | web/src/features/agent/composer/composer-plus-button.tsx:84 | "+" > Attach files… | — | | | | | | | |
| MENU-096 | Paste image with hint Ctrl+V: attaches clipboard images; empty clipboard toasts "No image on the clipboard" | web/src/features/agent/composer/composer-plus-button.tsx:120 | "+" > Paste image | — | | | | | | | |
| MENU-097 | New drawing: opens the drawing takeover | web/src/features/agent/composer/composer-plus-button.tsx:92 | "+" > New drawing | — | | | | | | | |
| MENU-098 | Separator + Mention a file with hint @: types @ (opens the file picker) | web/src/features/agent/composer/composer-plus-button.tsx:131 | "+" > Mention a file | — | | | | | | | |
| MENU-099 | Run a command with hint /: types / (opens the command picker) | web/src/features/agent/composer/composer-plus-button.tsx:139 | "+" > Run a command | — | | | | | | | |
| MENU-100 | Keyboard: base-ui Menu nav, Esc closes and refocuses "+" | web/src/components/ui/dropdown-menu.tsx:7 | open "+"; `rig key <app> Down Down Escape` | — | | | | | | | |

### Review comment actions (base-ui, web-drawn)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| MENU-101 | Open: "..." Comment actions on a review message; shown only when editable or deletable and not editing; aligned end, min-w 40 | web/src/features/git/components/review-message-row.tsx:88 | open src/pricing.ts in feature workspace; on a review thread > "..." | 100ms ease fade/zoom 95%, slide 0.5rem | | | | | | | |
| MENU-102 | Edit: message by the current git login, not an agent | web/src/features/git/components/review-message-row.tsx:96 | "..." > Edit. Seed: git identity matching the thread author | — | | | | | | | |
| MENU-103 | Copy as Markdown: toast "Copied to clipboard" | web/src/features/git/components/review-message-row.tsx:101 | "..." > Copy as Markdown | — | | | | | | | |
| MENU-104 | Separator + Delete (destructive red): opens the delete confirmation | web/src/features/git/components/review-message-row.tsx:108 | "..." > Delete; Cancel | — | | | | | | | |

### Sidebar host picker (base-ui, web-drawn)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| MENU-105 | Open: sidebar header host pill (status dot + name, title "Local · <state> · Switch host"); menu centred below, min-w 56 | web/src/components/layout/sidebar-host-picker.tsx:56 | Sidebar header > click the host pill | 100ms ease fade/zoom 95%, slide 0.5rem | | | | | | | |
| MENU-106 | Host row: status icon, name, "This machine" or endpoint, "Current" tag; choosing switches the listed host | web/src/components/layout/sidebar-host-picker.tsx:36 | host pill > Local. Seed: second paired host for a second row | — | | | | | | | |

### Settings title tab dropdown (base-ui, web-drawn)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| MENU-107 | Under 720px the settings title becomes a tab dropdown: each visible (search-filtered) tab with icon, active one muted | web/src/features/settings/components/settings-layout.tsx:52 | `rig size <app> min`; Sidebar header > settings gear; click the title dropdown > a tab | 100ms ease fade/zoom 95%, slide 0.5rem | | | | | | | |

### Markdown table toolbar menus (base-ui, non-modal)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| MENU-108 | Background color: "Colors" 10-column swatch grid (swatch tooltips 700ms); click paints the selected cells | web/src/components/ui/table-cell-color-menu.tsx:83 | rich markdown table > select a cell > Background color. Seed: markdown file with a table | 100ms ease fade/zoom 95%, slide 0.5rem | | | | | | | |
| MENU-109 | Background color > Clear (eraser) | web/src/components/ui/table-cell-color-menu.tsx:96 | Background color > Clear | — | | | | | | | |
| MENU-110 | Cell borders: checkbox items Top Border, Right Border, Bottom Border, Left Border | web/src/components/ui/table-floating-toolbar.tsx:337 | table cell > Cell borders > Top Border. Seed: markdown file with a table | 100ms ease fade/zoom 95%, slide 0.5rem | | | | | | | |
| MENU-111 | Cell borders: No Border, Outside Borders | web/src/components/ui/table-floating-toolbar.tsx:368 | Cell borders > No Border | — | | | | | | | |

### Custom dropdowns (framer-motion, web-drawn)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| MENU-112 | Language mode chip: 220px list, autofocused "Search languages...", current marked ✓, "No languages found" when empty; Esc in the field closes | web/src/features/editor/components/toolbar/editor-status-actions.tsx:227 | open src/pricing.ts; click the language chip; type `zzz` | 120ms easeOut | | | | | | | |
| MENU-113 | Language mode pick: sets the tab's language override and closes | web/src/features/editor/components/toolbar/editor-status-actions.tsx:258 | language chip > Plain Text | — | | | | | | | |
| MENU-114 | Editor preferences (sliders): Breadcrumbs, Minimap (shortcut hint), separator, Line Numbers, Word Wrap, Parameter Hints, Auto Completion; ✓ when on; stays open on toggle | web/src/features/editor/components/toolbar/editor-status-actions.tsx:312 | open src/pricing.ts; click Editor preferences > Word Wrap | 120ms easeOut | | | | | | | |
| MENU-115 | Language server chip menu: state title (running / not running / not installed / no server / unavailable) + detail; Restart when running, Start when stopped, busy labels Restarting… / Starting… | web/src/features/editor/components/toolbar/lsp-status-menu.tsx:110 | open src/pricing.ts; click the lightning chip | 120ms easeOut | | | | | | | |
| MENU-116 | Breadcrumb segment dropdown: folder listing at the segment; folder drills in, file opens and closes; "Go back" after drilling | web/src/features/editor/components/toolbar/file-path-breadcrumb.tsx:144 | open src/pricing.ts; click the `src` breadcrumb > inventory.ts | 120ms easeOut | | | | | | | |
| MENU-117 | Files filter menu: Hidden Files, Gitignored Files, separator, Git Status; ✓ toggles; stays open; funnel tinted when any is off | web/src/features/file-explorer/file-explorer/components/file-explorer-search-header.tsx:119 | Files sidebar; `rig key <app> ctrl+f`; click the funnel > Hidden Files | 120ms easeOut | | | | | | | |
| MENU-118 | Context gauge menu: Compact conversation (opens above, aligned end) | web/src/features/agent/controls/context-gauge.tsx:82 | chat composer > context pill. Seed: chat with context telemetry and compaction available | 120ms easeOut | | | | | | | |
| MENU-119 | Keyboard: Esc closes and refocuses the trigger; outside mousedown closes; Up/Down/Home/End/Enter only for item lists (filter, gauge) with focus inside; no type-ahead | web/src/components/ui/dropdown.tsx:161 | open Files filter menu; `rig key <app> Down Return Escape` | — | | | | | | | |
| MENU-120 | Open/close animation of every custom Dropdown | web/src/components/ui/menu-popover.tsx:80 | open and close Editor preferences | 120ms easeOut: opacity 0→1, scale 0.98→1, y −4px (below) / +4px (above); exit reverses | | | | | | | |

### Selects (base-ui, web-drawn)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| MENU-121 | Merge strategy select: Create a merge commit, Squash and merge, Rebase and merge; merge button label follows | web/src/features/git/components/review-merge-box.tsx:89 | feature workspace > Git sidebar > Merge box > strategy select | — (no transition) | | | | | | | |
| MENU-122 | New space daemon select: Local; each saved host (disabled while unreachable, with status words); separator + "+ Add a host…" | web/src/components/layout/create-space-daemon-row.tsx:81 | open the new space panel > Daemon select. Seed: saved remote host | — (no transition) | | | | | | | |
| MENU-123 | Select keyboard: base-ui (arrows, Enter, Esc, type-ahead, item aligned with trigger) | web/src/components/ui/select.tsx:92 | open merge strategy select; `rig key <app> Down Return` | — | | | | | | | |

### Popover menus (base-ui, web-drawn)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| MENU-124 | Code block language combobox (cmdk): "Search language...", ✓ current, "No language found."; pick sets the block language | web/src/components/ui/code-block-node.tsx:305 | rich markdown > code block > language button. Seed: markdown file with a fenced code block | 150ms cubic-bezier(0.4,0,0.2,1) scale 0.98 + fade in; close instant | | | | | | | |
| MENU-125 | Icon popover (project/repo icon, pencil on hover): Upload, Emoji (inline input, Enter applies), GitHub (repo only), Reset to default | web/src/components/layout/icon-popover.tsx:195 | Sidebar project switcher > click the project icon; Sidebar > click checkout repo icon | 150ms cubic-bezier(0.4,0,0.2,1) scale 0.98 + fade in; close instant | | | | | | | |

Skipped:
- `web/src/components/ui/context-menu.tsx:103` ImperativeContextMenu as the primary menu: only in browser dev and on the phone (desktop pops the native menu).
- `web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx:236` Generate Image: `onGenerateImage` is never passed by SidebarFilesPanel (dead on desktop).
- `web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx:247` Add Folder to Workspace: `onAddFolderToWorkspace` never passed (dead).
- `web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx:374` Remove From Workspace: `onRemoveFolderFromWorkspace` never passed (dead).
- `web/src/components/sidebar/lib/row-menu.ts:186` rowVerbs: the phone's single long-press menu.
- `web/src/features/agent/composer/native-plus-menu.tsx` NativePlusMenuSheet and the phone-only "+" items (Photo library, Take a photo): mobile only.
- `web/src/features/agent/components/provider-switch-dropdown.tsx`, `web/src/features/agent/controls/agent-selection-picker.tsx`, `web/src/features/agent/composer/permission-level-switcher.tsx`: chat pickers, owned by CHAT.
- `web/src/features/settings/components/font-selector.tsx`, `setting-select.tsx`, `tabs/default-permission-level-setting.tsx`, `tabs/developer-settings.tsx` selects: settings controls, owned by SET.
- `web/src/components/ui/dropdown-menu.tsx:97` DropdownMenuSub/SubTrigger/SubContent, RadioGroup/RadioItem, Shortcut: no call site.
- `web/src/features/terminal/lib/terminal-url-menu.ts:26` non-desktop path (link opens without a menu): browser/phone only.
- `desktop/src-tauri/src/console_menu.rs:111` `set_console_menu_chord` on Linux: no-op, no menu exists.
