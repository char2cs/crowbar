# 12. Browser pane (BRW)

### Open and toolbar

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| BRW-001 | New browser tab from New Tab page (homepage empty by default → blank tab "Browser") | web/src/features/tabs/hooks/use-pane-new-tab-actions.ts:38 | ctrl+t > Browser (page: see VT/IDE) | — | | | | | | | |
| BRW-002 | Blank tab: address bar auto-focused, placeholder "Search or enter address", empty content area, no webview | web/src/features/browser/components/browser-pane.tsx:69 | after BRW-001; shoot | — | | | | | | | |
| BRW-003 | Blank tab: Back, Forward, Reload, Pick element all disabled | web/src/features/browser/components/browser-pane.tsx:100 | after BRW-001; hover each | — | | | | | | | |
| BRW-004 | New tab with a homepage set opens straight on that page | web/src/features/browser/lib/homepage.ts:6 | Settings > Browser homepage `localhost:8000` (see SET), ctrl+t > Browser. Seed: local HTTP server | — | | | | | | | |
| BRW-005 | Open from terminal URL menu "Open in Crowbar Browser": tab in the terminal's pane | web/src/features/terminal/lib/terminal-url-menu.ts:37 | ctrl+j, `echo http://localhost:8000`, ctrl+click > Open in Crowbar Browser (see TERM-060) | — | | | | | | | |
| BRW-006 | Toolbar layout: ← → ⟳ ⌖ [Agent badge] address input, bottom border | web/src/features/browser/components/browser-pane.tsx:91 | any browser tab; shoot toolbar | — | | | | | | | |
| BRW-007 | Pick element button tooltip "Pick element (copy pointer)" | web/src/features/browser/components/browser-pane.tsx:131 | page loaded; hover ⌖ | tooltip (see A11Y) | | | | | | | |
| BRW-008 | Tab label: page title, else host, else "Browser"; globe icon | web/src/features/browser/lib/browser-tab-name.ts:2 | load page with/without `<title>`; read tab. Seed: local HTTP server | — | | | | | | | |
| BRW-009 | No stop button, no loading indicator anywhere in the pane (loading state stored only) | web/src/features/panes/stores/slices/buffer-slice.ts:231 | load a slow page; shoot toolbar mid-load. Seed: slow local HTTP endpoint | — | | | | | | | |
| BRW-010 | No devtools / inspect entry in the pane | web/src/features/browser/components/browser-pane.tsx:91 | right-click page; look for Inspect. Seed: local HTTP server | — | | | | | | | |

### Address bar

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| BRW-011 | Typing edits a draft; tab URL unchanged until Enter | web/src/features/browser/components/browser-pane.tsx:145 | blank tab, type `localhost:8000` | — | | | | | | | |
| BRW-012 | Blur discards the draft, reverts to current URL | web/src/features/browser/components/browser-pane.tsx:146 | type text, click elsewhere | — | | | | | | | |
| BRW-013 | Enter on empty/whitespace: nothing happens | web/src/features/browser/lib/normalize-url.ts:10 | blank tab, type spaces, Enter | — | | | | | | | |
| BRW-014 | Enter on blank tab first creates the native webview at the content area | web/src/features/browser/hooks/use-browser-webview.ts:39 | blank tab, `localhost:8000`, Enter. Seed: local HTTP server | — | | | | | | | |
| BRW-015 | `localhost:PORT` / `127.0.0.1` → `http://` prefix | web/src/features/browser/lib/normalize-url.ts:12 | type `localhost:8000`, Enter; read address | — | | | | | | | |
| BRW-016 | Dotted host → `https://` prefix | web/src/features/browser/lib/normalize-url.ts:13 | type `example.com`, Enter; read address (no network: error page) | — | | | | | | | |
| BRW-017 | Explicit http(s) URL kept as typed | web/src/features/browser/lib/normalize-url.ts:11 | type `http://localhost:8000/a`, Enter | — | | | | | | | |
| BRW-018 | Words / phrase → search URL of chosen engine (DuckDuckGo default) | web/src/features/browser/lib/search-engines.ts:24 | type `pricing rounding`, Enter; read address | — | | | | | | | |
| BRW-019 | Non-web scheme typed (`file:///…`, `about:blank`) becomes a search, never loads | web/src/features/browser/lib/normalize-url.ts:14 | type `file:///etc/hostname`, Enter | — | | | | | | | |
| BRW-020 | Custom search engine template used; invalid template falls back to DuckDuckGo | web/src/features/browser/lib/search-engines.ts:25 | Settings > Browser > Custom (see SET); type `foo`, Enter | — | | | | | | | |
| BRW-021 | Address updates on in-page navigation (link click, redirect) | web/src/features/browser/components/browser-events-listener.tsx:26 | click a link on loaded page; read address. Seed: local page with links | — | | | | | | | |
| BRW-022 | Rejected URL from native side: error toast "Go to the page" | web/src/features/browser/components/browser-pane.tsx:84 | address bar never produces one (toast: see TST). Seed: navigation to a non-http(s) URL via agent | — | | | | | | | |

### Navigation and page states

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| BRW-023 | Back = history.back(); always enabled once a URL exists (even with no history) | desktop/src-tauri/src/browser/commands.rs:193 | load page A, then B; click ←; click ← again at start | — | | | | | | | |
| BRW-024 | Forward = history.forward() | desktop/src-tauri/src/browser/commands.rs:198 | after BRW-023, click → | — | | | | | | | |
| BRW-025 | Reload re-loads the page | desktop/src-tauri/src/browser/commands.rs:203 | click ⟳ | — | | | | | | | |
| BRW-026 | Title change updates tab label live | desktop/src-tauri/src/browser/commands.rs:91 | page that sets `document.title` on a timer. Seed: local page | — | | | | | | | |
| BRW-027 | Page load error: WebKit's own error page inside the webview (no app error UI) | desktop/src-tauri/src/browser/commands.rs:86 | type `localhost:9`, Enter | — | | | | | | | |
| BRW-028 | No network: dotted host / search shows WebKit can't-connect page | desktop/src-tauri/src/browser/commands.rs:81 | type `example.com`, Enter | — | | | | | | | |
| BRW-029 | Page zoom = Settings browserZoom (100% default), applied only at creation | web/src/features/browser/lib/ensure-webview.ts:21 | Settings > Browser zoom 150 (see SET), open new browser tab, load page; old tab unchanged | — | | | | | | | |
| BRW-030 | ctrl+=/ctrl+- inside page: WebKit page zoom keys not app-handled | desktop/src-tauri/src/browser/commands.rs:114 | click into page, ctrl+equal. Seed: local page | — | | | | | | | |
| BRW-031 | target=_blank / window.open: native window denied, URL opens as new browser tab in same pane | desktop/src-tauri/src/browser/commands.rs:94 | click a `target=_blank` link. Seed: local page with _blank link | — | | | | | | | |
| BRW-032 | Download link: no app handling (webview default) | desktop/src-tauri/src/browser/commands.rs:84 | click a download link. Seed: local page serving a file download | — | | | | | | | |
| BRW-033 | Page content is interactive (scroll, click, type in forms) | desktop/src-tauri/src/browser/gtk_layer.rs:56 | scroll page, type in an input. Seed: local page with form | — | | | | | | | |
| BRW-034 | Keyboard focus moves into page on click; app shortcuts while page focused go to the page | desktop/src-tauri/src/browser/commands.rs:104 | click page, press ctrl+j | — | | | | | | | |

### Element picker

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| BRW-035 | Pick toggle by button: ⌖ pressed/active state, crosshair cursor over page | web/src/features/browser/components/browser-pane.tsx:129 | page loaded, click ⌖. Seed: local page | — | | | | | | | |
| BRW-036 | Pick toggle by mod+shift+e (pane focused, URL loaded, visible) | web/src/features/browser/components/browser-pane.tsx:58 | focus pane toolbar, ctrl+shift+e | — | | | | | | | |
| BRW-037 | Hover outline: 2px #3b82f6 box around element under pointer | web/src/features/browser/lib/pointer-agent.js:201 | in pick mode, move over elements | — | | | | | | | |
| BRW-038 | Click element: pick ends, pointer copied, toast "Pointer copied" | web/src/features/browser/hooks/use-pointer-pick.ts:26 | click a button (toast: see TST); paste into chat composer | — | | | | | | | |
| BRW-039 | Drag region: filled blue box (15% wash), release picks elements inside, toast "Pointer copied" / "N items" | web/src/features/browser/lib/pointer-agent.js:203 | press-drag across a list | — | | | | | | | |
| BRW-040 | Shift+click: multi-select marks persist; plain click or Enter finishes "multi" | web/src/features/browser/lib/pointer-agent.js:301 | shift+click 3 items, Enter | — | | | | | | | |
| BRW-041 | Alt-drag / double-click: native text selection, release picks quoted text | web/src/features/browser/lib/pointer-agent.js:258 | alt+drag over a paragraph | — | | | | | | | |
| BRW-042 | Escape cancels pick: overlay removed, button un-pressed, no toast | web/src/features/browser/lib/pointer-agent.js:311 | pick mode, click page, Escape | — | | | | | | | |
| BRW-043 | Toggle again (button/chord) cancels pick | web/src/features/browser/hooks/use-pointer-pick.ts:37 | pick mode, click ⌖ | — | | | | | | | |
| BRW-044 | Tab hidden/occluded during pick cancels it | web/src/features/browser/hooks/use-pointer-pick.ts:41 | pick mode, switch tab, back | — | | | | | | | |
| BRW-045 | Page navigates away during pick: pick ends null | web/src/features/browser/lib/pointer-agent.js:327 | pick mode, page auto-redirects. Seed: local page redirecting on timer | — | | | | | | | |
| BRW-046 | Pick failure/timeout (60s): error toast "Could not pick" | web/src/features/browser/hooks/use-pointer-pick.ts:29 | pick mode, wait 60s (toast: see TST) | — | | | | | | | |
| BRW-047 | Copied pointer text format "⌖ <kind> "<label>" · url · tab id" + ⌖data JSON line | web/src/features/browser/lib/pointer.ts:55 | after BRW-038, paste into editor | — | | | | | | | |

### Native webview placement

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| BRW-048 | Webview laid exactly over content area below toolbar; follows resize | web/src/features/browser/hooks/use-browser-webview.ts:42 | page loaded; rig size min, standard | — | | | | | | | |
| BRW-049 | Linux: webview on a pass-through GTK Fixed layer over the app webview (not splitting the window) | desktop/src-tauri/src/browser/gtk_layer.rs:40 | first browser tab load; shoot whole window | — | | | | | | | |
| BRW-050 | Inactive tab: webview parked off-window left, page keeps running; returning un-parks same page state | desktop/src-tauri/src/browser/park.rs:16 | load page, scroll, open README.md tab, back to browser tab | — | | | | | | | |
| BRW-051 | Parking hands keyboard focus back to the app webview | desktop/src-tauri/src/browser/commands.rs:122 | click page, ctrl+tab to other tab, type | — | | | | | | | |
| BRW-052 | Any body-level popup (menu, dialog, dropdown, command palette) parks the webview; tooltips/toasts do not | web/src/features/browser/hooks/use-native-overlay-active.ts:5 | page loaded; right-click a tab (menu: see MENU); hover ⌖ tooltip | — | | | | | | | |
| BRW-053 | Sidebar peek card over pane parks the webview | web/src/features/browser/hooks/use-browser-occluded.ts:10 | rig sidebar left (collapsed), hover edge to peek | — | | | | | | | |
| BRW-054 | Console open in overlay mode parks the webview (push mode does not) | web/src/features/browser/hooks/use-browser-occluded.ts:11 | page loaded; ctrl+grave (console, overlay dock) | — | | | | | | | |
| BRW-055 | Another pane fullscreen parks this pane's webview | web/src/features/browser/hooks/use-browser-occluded.ts:17 | split (see SPL), fullscreen other pane | — | | | | | | | |
| BRW-056 | Tab drag over the pane (drop target) parks the webview | web/src/features/panes/components/pane-editor-view-content.tsx:152 | drag another tab over browser pane (see DND) | — | | | | | | | |
| BRW-057 | Browser tab stays mounted across tab switch/split (page not rebuilt) | web/src/features/panes/components/pane-editor-view-content.tsx:135 | load page, type in input, switch tabs, back | — | | | | | | | |
| BRW-058 | Browser in split: two webviews side by side, each placed in its own pane | web/src/features/browser/hooks/use-browser-webview.ts:28 | two browser tabs, split right (see SPL) | — | | | | | | | |
| BRW-059 | Closing the tab closes the webview | web/src/features/panes/lib/buffer-release.ts:81 | ctrl+w on browser tab | — | | | | | | | |
| BRW-060 | Reopened app restores tab at last URL; webview made only when tab first shown | web/src/features/browser/hooks/use-browser-webview.ts:29 | load page, restart app, select browser tab | — | | | | | | | |

### Agent-driven

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| BRW-061 | "Agent" info badge in toolbar while an agent command runs on the tab | web/src/features/browser/components/browser-pane.tsx:138 | agent calls browser_* tool on tab. Seed: agent/MCP client driving browser tools | — | | | | | | | |
| BRW-062 | Agent-opened tab appears in agent's pane; never-shown tab's webview created parked | web/src/features/browser/lib/agent-ops.ts:130 | agent browser_open. Seed: agent/MCP client | — | | | | | | | |

Skipped:
- `desktop/src-tauri/src/browser/webkit.rs`: macOS WKWebView eval/snapshot, used only by agent tools (no visible UI).
- `desktop/src-tauri/src/browser/webkitgtk.rs`: agent eval/screenshot plumbing, no visible UI.
- `desktop/src-tauri/src/browser/data.rs` clear-data: surfaced in Settings > Browser (see SET); unsupported on Linux.
- `desktop/src-tauri/src/navigation.rs`: app-webview guard; browser tabs exempt (`exempt_from_app_guard`), so no browser-pane behaviour.
- `web/src/features/browser/lib/page-agent.js`, `dom-agent.ts`, `agent-peer.ts`: agent snapshot/click/type ops, no pane UI beyond BRW-060/061.
- `browser-bridge.ts` non-desktop rejection: web build only.
