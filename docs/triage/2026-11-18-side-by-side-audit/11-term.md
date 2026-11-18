# 11. Terminal (TERM)

### Open and attach

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| TERM-001 | New terminal tab via mod+j: tab "Terminal N" opens in active pane, shell spawned in workspace worktree | web/src/features/panes/hooks/use-pane-keyboard.ts:67 | Workspace `main` open, ctrl+j | — | | | | | | | |
| TERM-002 | New terminal from New Tab page "Terminal" action (replaces the new-tab page in place) | web/src/features/tabs/hooks/use-pane-new-tab-actions.ts:32 | ctrl+t > Terminal (page itself: see VT/IDE) | — | | | | | | | |
| TERM-003 | "Open in Terminal" from file-tree folder: tab named after folder, shell cwd = folder | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx:217 | Files > right-click `src` > Open in Terminal (item: see MENU); `pwd` | — | | | | | | | |
| TERM-004 | "Open in Terminal" from an editor tab: cwd = file's directory, tab named after it | web/src/features/tabs/components/tab-context-menu.tsx:139 | Open src/pricing.ts > right-click its tab > Open in Terminal; `pwd` | — | | | | | | | |
| TERM-005 | Default label "Terminal N" (N = open terminal count + 1) | web/src/features/panes/stores/slices/new-buffer.ts:68 | ctrl+j twice; read both tab labels | — | | | | | | | |
| TERM-006 | First terminal ever: xterm chunk lazy-loads, pane blank (no spinner) until loaded | web/src/features/terminal/components/lazy-terminal.tsx:14 | rig reset, ctrl+j, shoot immediately | — | | | | | | | |
| TERM-007 | Never-shown terminal tab builds/attaches nothing until first shown | web/src/features/terminal/components/terminal.tsx:90 | ctrl+j, switch to another tab, restart app, select terminal tab | — | | | | | | | |
| TERM-008 | First frame after attach: daemon snapshot drawn, scrolled to bottom, full repaint | web/src/features/terminal/hooks/use-terminal-connection.ts:116 | ctrl+j; shoot prompt | — | | | | | | | |
| TERM-009 | Prompt layout: 16px left inset, transparent background over pane glass, app foreground text | web/src/features/terminal/components/terminal.tsx:213 | ctrl+j; shoot | — | | | | | | | |
| TERM-010 | Terminal tabs reconnect to the same PTY after app restart (screen and cwd preserved) | web/src/features/terminal/hooks/use-terminal-attachment.ts:124 | ctrl+j, `cd src`, `echo hi`, restart app, select tab | — | | | | | | | |
| TERM-011 | Closing a terminal tab kills its PTY (not in reopen-closed history) | web/src/features/terminal/lib/kill-terminal-session.ts:6 | ctrl+j, ctrl+w, ctrl+shift+t (tab strip: see VT/IDE) | — | | | | | | | |

### Rendering

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| TERM-012 | Typing echoes; keys forwarded verbatim to PTY | web/src/features/terminal/hooks/use-terminal-connection.ts:126 | ctrl+j, type `echo hello`, Enter | — | | | | | | | |
| TERM-013 | ANSI 16 colours from theme palette (normal + bright) | web/src/features/terminal/hooks/use-terminal-theme.ts:62 | type `for i in $(seq 30 37) $(seq 90 97); do printf "\e[${i}m$i \e[0m"; done; echo`, Enter | — | | | | | | | |
| TERM-014 | Background colours, bold, italic, underline, inverse SGR | web/src/features/terminal/hooks/use-xterm-instance.ts:101 | type `printf '\e[41m bg \e[0m \e[1mbold\e[0m \e[3mital\e[0m \e[4munder\e[0m \e[7minv\e[0m\n'`, Enter | — | | | | | | | |
| TERM-015 | 256-colour and truecolor output | web/src/features/terminal/hooks/use-xterm-instance.ts:101 | type `printf '\e[38;5;208m256\e[0m \e[38;2;0;200;120mrgb\e[0m\n'`, Enter | — | | | | | | | |
| TERM-016 | Unicode 11 widths: emoji and CJK occupy 2 cells | web/src/features/terminal/lib/mount-xterm.ts:57 | type `echo "😀中文\|ab"`, Enter | — | | | | | | | |
| TERM-017 | Font: terminal font family with Nerd-font fallbacks, Linux fallback Liberation Mono, 14px, line height 1 | web/src/features/terminal/utils/resolve-font.ts:5 | ctrl+j; shoot glyph metrics | — | | | | | | | |
| TERM-018 | Theme switch live: palette, foreground, cursor re-read; daemon told new bg/fg/dark | web/src/features/terminal/hooks/use-terminal-connection.ts:148 | ctrl+j, run TERM-013 colours, rig theme light, then dark | — | | | | | | | |
| TERM-019 | Cursor: block, blinking, foreground-coloured (defaults) | web/src/features/settings/config/default-settings.ts:41 | ctrl+j; shoot twice ~600ms apart | xterm blink ~600ms | | | | | | | |
| TERM-020 | Cursor unfocused: outline (hollow) block, no blink | web/src/features/terminal/hooks/use-xterm-instance.ts:108 | ctrl+j, click sidebar; shoot | — | | | | | | | |
| TERM-021 | Cursor style/blink/width setting applies live (bar width 2 × zoom) | web/src/features/terminal/hooks/use-xterm-instance.ts:144 | Settings > Terminal > cursor style bar (see SET), return to terminal | — | | | | | | | |
| TERM-022 | Font size/family/line height/letter spacing/scrollback settings apply live and refit | web/src/features/terminal/hooks/use-xterm-instance.ts:147 | Settings > Terminal (see SET) change font size 18; return | — | | | | | | | |
| TERM-023 | Bell (BEL) has no visual or audio feedback | web/src/features/terminal/lib/mount-xterm.ts:33 | type `printf '\a'`, Enter | — | | | | | | | |
| TERM-024 | No app-level clear chord (mod+k does nothing); `clear` / ctrl+l clear via the shell | web/src/features/terminal/lib/mount-xterm.ts:50 | `seq 1 50`, Enter; ctrl+k; then ctrl+l | — | | | | | | | |

### Keyboard, selection, clipboard

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| TERM-025 | ctrl+c sends ^C (interrupts program) | web/src/features/terminal/lib/mount-xterm.ts:50 | type `sleep 100`, Enter, ctrl+c | — | | | | | | | |
| TERM-026 | Other ctrl chords reach the program (ctrl+u kill line, ctrl+a/e, ctrl+r) | web/src/features/terminal/lib/mount-xterm.ts:50 | type `abc def`, ctrl+u; ctrl+r | — | | | | | | | |
| TERM-027 | Shift+Enter sends CSI 13;2u, Alt+Enter CSI 13;3u (once, no CR) | web/src/features/terminal/utils/terminal-key-overrides.ts:9 | type `cat -v`, Enter, shift+Return, alt+Return, ctrl+c | — | | | | | | | |
| TERM-028 | Mouse drag selection: wash = foreground @25% | web/src/features/terminal/hooks/use-terminal-theme.ts:71 | `seq 1 20`, Enter; drag across lines | — | | | | | | | |
| TERM-029 | Double-click selects word, triple-click selects line | web/src/features/terminal/lib/mount-xterm.ts:33 | `echo alpha beta`, Enter; double-click `beta`; triple-click | — | | | | | | | |
| TERM-030 | Right-click does not select word; no app context menu (webview default only) | web/src/features/terminal/hooks/use-xterm-instance.ts:116 | right-click on a word | — | | | | | | | |
| TERM-031 | Linux copy: ctrl+shift+c copies selection, never sends to PTY; no selection = no-op | web/src/features/terminal/lib/mount-xterm.ts:65 | select text, ctrl+shift+c; paste elsewhere (chat composer) | — | | | | | | | |
| TERM-032 | Linux paste: ctrl+shift+v pastes clipboard (bracketed paste when app enables it) | web/src/features/terminal/lib/mount-xterm.ts:67 | copy `echo pasted`, ctrl+shift+v | — | | | | | | | |
| TERM-033 | macOS: cmd+c / cmd+v / cmd+a are app/OS shortcuts, never reach PTY; Option = Meta (macOS only) | web/src/features/terminal/lib/mount-xterm.ts:52 | macOS only: select, cmd+c, cmd+v; alt+b moves word | — | | | | | | | |
| TERM-034 | Pane-switch focus sequences (ESC[I/ESC[O) suppressed: no garbage at prompt | web/src/features/terminal/hooks/use-terminal-connection.ts:130 | ctrl+j, click sidebar, click terminal, repeat | — | | | | | | | |
| TERM-035 | Window focus/blur reported to apps that enabled DECSET 1004 | web/src/features/terminal/hooks/use-terminal-connection.ts:169 | type `printf '\e[?1004h'; cat -v`, Enter; focus other app window, return | — | | | | | | | |
| TERM-036 | Text drag-drop / replacement text inserted as input | web/src/features/terminal/lib/mount-xterm.ts:80 | select `README.md` text in editor, drag onto terminal | — | | | | | | | |
| TERM-037 | OS file drop types shell-quoted absolute paths + trailing space, focuses terminal | web/src/features/terminal/utils/terminal-file-drop.ts:19 | Drag a file from OS file manager onto terminal (see DND). Seed: OS file manager in rig | — | | | | | | | |

### Scrollback

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| TERM-038 | Wheel scrolls scrollback (10000 lines default) | web/src/features/settings/config/default-settings.ts:40 | `seq 1 500`, Enter; scroll up 10 | — | | | | | | | |
| TERM-039 | Scrollbar: xterm slider painted with app scrollbar tokens, transparent track, inset rounded thumb | web/src/features/terminal/styles/terminal.css:56 | after TERM-038; shoot right edge | — | | | | | | | |
| TERM-040 | Scrollbar slider hover/active colour | web/src/features/terminal/hooks/use-terminal-theme.ts:74 | hover, then drag slider | — | | | | | | | |
| TERM-041 | Typing while scrolled up jumps back to bottom | web/src/features/terminal/hooks/use-xterm-instance.ts:101 | scroll up, type `x` | — | | | | | | | |
| TERM-042 | Mouse-tracking app owns wheel; shift+wheel scrolls scrollback 3× lines on primary buffer | web/src/features/terminal/utils/wheel-routing.ts:5 | `seq 1 200; printf '\e[?1000h'`, Enter; scroll up; shift+scroll up; `printf '\e[?1000l'` | — | | | | | | | |
| TERM-043 | Alternate screen app (less): no scrollback, wheel per xterm default | web/src/features/terminal/hooks/use-terminal-connection.ts:156 | `less README.md`, Enter; scroll; q | — | | | | | | | |

### Find bar

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| TERM-044 | ctrl+f with terminal focused opens find bar top-right, input focused and selected | web/src/features/terminal/hooks/use-terminal-shortcuts.ts:28 | ctrl+j, `seq 1 30`, Enter, ctrl+f | — | | | | | | | |
| TERM-045 | Find bar empty: placeholder "Find in terminal...", no count, prev/next disabled, 3 toggles | web/src/features/terminal/components/terminal-search.tsx:92 | after TERM-044; shoot | — | | | | | | | |
| TERM-046 | Incremental search: matches highlighted (--primary), active match (--color-accent), count "n/m" | web/src/features/terminal/hooks/use-terminal-search.ts:58 | type `2` | — | | | | | | | |
| TERM-047 | No match: count "0/0", nav disabled | web/src/features/terminal/components/terminal-search.tsx:96 | type `zzz` | — | | | | | | | |
| TERM-048 | Enter = next match, Shift+Enter = previous; counter updates | web/src/features/terminal/components/terminal-search.tsx:68 | type `1`, Enter ×2, shift+Return | — | | | | | | | |
| TERM-049 | Previous/Next chevron buttons | web/src/components/ui/search.tsx:175 | click ▲ / ▼ | — | | | | | | | |
| TERM-050 | Match case toggle (pressed state, tooltip "Match case") re-runs search | web/src/features/terminal/components/terminal-search.tsx:103 | `echo Foo foo`, ctrl+f, `foo`, click Aa | — | | | | | | | |
| TERM-051 | Match whole word toggle | web/src/features/terminal/components/terminal-search.tsx:110 | `echo foo foobar`, ctrl+f, `foo`, click whole-word | — | | | | | | | |
| TERM-052 | Regex toggle | web/src/features/terminal/components/terminal-search.tsx:117 | ctrl+f, `1[0-9]`, click .* | — | | | | | | | |
| TERM-053 | Clear (×) inside input empties term and decorations | web/src/components/ui/search.tsx:112 | type term, click inner × | — | | | | | | | |
| TERM-054 | Close via Esc or "Close search" ×: bar hides, decorations + selection cleared, terminal refocused | web/src/features/terminal/hooks/use-terminal-search.ts:52 | Escape; reopen, click outer × | — | | | | | | | |
| TERM-055 | ctrl+f while bar open re-focuses/selects input | web/src/features/terminal/hooks/use-terminal-shortcuts.ts:31 | bar open, click terminal, ctrl+f | — | | | | | | | |
| TERM-056 | ctrl+f with focus outside terminal not taken by terminal | web/src/features/terminal/hooks/use-terminal-shortcuts.ts:24 | click sidebar, ctrl+f | — | | | | | | | |

### Links

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| TERM-057 | URL hover without modifier: no underline, text cursor | web/src/features/terminal/lib/terminal-links.ts:214 | `echo https://example.com/a`, Enter; hover URL | — | | | | | | | |
| TERM-058 | URL hover with ctrl (cmd on macOS) held: underline + pointer; follows key press/release | web/src/features/terminal/lib/terminal-links.ts:199 | hover URL, hold ctrl, release | — | | | | | | | |
| TERM-059 | Plain click on URL does not open | web/src/features/terminal/lib/terminal-links.ts:188 | click URL | — | | | | | | | |
| TERM-060 | ctrl+click URL: native menu Open in Default Browser / Open in Crowbar Browser / Copy Link | web/src/features/terminal/lib/terminal-url-menu.ts:30 | ctrl+click URL (items: see MENU) | — | | | | | | | |
| TERM-061 | URL punctuation trimmed: "(https://x.dev/a)." links only the URL | web/src/features/terminal/lib/terminal-links.ts:22 | `echo "(https://x.dev/a)."`; ctrl+hover | — | | | | | | | |
| TERM-062 | Soft-wrapped URL is one link across rows | web/src/features/terminal/lib/terminal-links.ts:107 | rig size min; echo a 200-char URL; ctrl+hover 2nd row | — | | | | | | | |
| TERM-063 | OSC 8 hyperlink: ctrl+click opens link menu; plain click ignored | web/src/features/terminal/lib/terminal-links.ts:237 | `printf '\e]8;;https://example.com\e\\link\e]8;;\e\\\n'`; click, ctrl+click | — | | | | | | | |
| TERM-064 | File link `src/pricing.ts:3` ctrl+click opens editor at line 3 (relative to cwd) | web/src/features/terminal/lib/terminal-links.ts:304 | `echo src/pricing.ts:3`; ctrl+click | — | | | | | | | |
| TERM-065 | Absolute path outside worktree on local host opens as plain file | web/src/features/terminal/lib/terminal-links.ts:288 | `echo /etc/hostname`; ctrl+click | — | | | | | | | |
| TERM-066 | `~/` path: toast "Cannot open file" / "Could not resolve ~/x to a path." | web/src/features/terminal/lib/terminal-links.ts:317 | `echo ~/foo/bar`; ctrl+click (toast: see TST) | — | | | | | | | |
| TERM-067 | Bare filename and trailing period not linked ("README.md", "see a/b.md.") | web/src/features/terminal/lib/terminal-links.ts:26 | `echo README.md see src/pricing.ts.`; ctrl+hover each | — | | | | | | | |
| TERM-068 | Path inside a URL is not a file link | web/src/features/terminal/lib/terminal-links.ts:50 | `echo https://x.dev/src/pricing.ts`; ctrl+hover | — | | | | | | | |
| TERM-069 | Remote workspace: path outside worktree toasts "... is outside the current workspace." | web/src/features/terminal/lib/terminal-links.ts:307 | ctrl+click `/etc/hostname`. Seed: remote host workspace | — | | | | | | | |

### Size, zoom, focus, splits

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| TERM-070 | Window resize refits cols/rows and resizes PTY (one fit per frame) | web/src/features/terminal/hooks/use-pty-size-sync.ts:62 | `tput cols; tput lines`; rig size min; repeat; rig size standard | — | | | | | | | |
| TERM-071 | Output reflows on width change | web/src/features/terminal/hooks/use-pty-size-sync.ts:67 | `seq -s ' ' 1 200`; rig size min / standard | — | | | | | | | |
| TERM-072 | Split/sidebar drag: fit deferred until drag ends, then one refit | web/src/features/terminal/hooks/use-pty-size-sync.ts:72 | split right (see SPL), drag divider, release; `tput cols` | — | | | | | | | |
| TERM-073 | Sidebar toggle refits | web/src/features/terminal/hooks/use-pty-size-sync.ts:92 | rig sidebar left; `tput cols` | — | | | | | | | |
| TERM-074 | mod+= / mod+- : terminal font size ±2 (clamped 8–32), persisted setting, all terminals | web/src/features/terminal/hooks/use-terminal-shortcuts.ts:43 | terminal focused, ctrl+equal ×2, ctrl+minus | — | | | | | | | |
| TERM-075 | mod+0 resets terminal font size to 14 | web/src/features/terminal/hooks/use-terminal-shortcuts.ts:50 | after TERM-074, ctrl+0 | — | | | | | | | |
| TERM-076 | Zoom chords also drive the global chat-zoom handler (chat zoom changes too) | web/src/features/keymaps/hooks/use-zoom-keyboard.ts:21 | split terminal + chat; terminal focused ctrl+equal; inspect chat | — | | | | | | | |
| TERM-077 | Zoom chords ignored when terminal not focused | web/src/features/terminal/hooks/use-terminal-shortcuts.ts:43 | click sidebar, ctrl+equal | — | | | | | | | |
| TERM-078 | Focus follows active pane/tab: terminal focused on activation (one call) | web/src/features/terminal/components/terminal.tsx:182 | ctrl+j, open README.md tab, back to terminal tab, type | — | | | | | | | |
| TERM-079 | Mousedown anywhere in terminal (incl. left inset) focuses it and activates its tab + pane | web/src/features/terminal/components/terminal-tab.tsx:50 | split, focus other pane, click terminal's left padding, type | — | | | | | | | |
| TERM-080 | Hidden tab keeps xterm + transport mounted; returning shows same screen/scrollback | web/src/features/terminal/components/terminal-tab.tsx:21 | `seq 1 300`, switch tab, switch back, scroll up | — | | | | | | | |
| TERM-081 | Terminal in split: each pane its own terminal; keyboard only to focused one; find/zoom only for active | web/src/features/terminal/hooks/use-terminal-shortcuts.ts:57 | ctrl+j, split right (see SPL), ctrl+j; ctrl+alt+Left/Right, type | — | | | | | | | |
| TERM-082 | Same terminal shown in two panes: only visible view owns PTY size; each has own transport | web/src/features/terminal/hooks/use-pty-size-sync.ts:12 | move/duplicate terminal tab into split (see SPL); resize one | — | | | | | | | |

### Title, exit, reconnect, errors

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| TERM-083 | OSC title sets tab label | web/src/features/terminal/hooks/use-terminal-connection.ts:137 | `printf '\e]0;build\a'`, Enter; read tab | — | | | | | | | |
| TERM-084 | Rejected title (>28 chars, contains @ or /, ESC, CSI body) falls back to directory label | web/src/features/tabs/hooks/use-buffer-display-name.ts:64 | `printf '\e]0;user@host:~/x\a'`; read tab | — | | | | | | | |
| TERM-085 | Empty title clears prior title (back to cwd/folder label) | web/src/features/terminal/utils/terminal-title.ts:6 | after TERM-083, `printf '\e]0;\a'` | — | | | | | | | |
| TERM-086 | `exit` / ctrl+d: exit frame closes the tab in every pane | web/src/features/terminal/components/terminal-tab.tsx:35 | ctrl+j, type `exit`, Enter; repeat with ctrl+d | — | | | | | | | |
| TERM-087 | Bound session gone on (re)attach (daemon reaped it): tab closes | web/src/features/terminal/hooks/use-terminal-attachment.ts:141 | ctrl+j, kill shell from another terminal (`kill -9 <pid>`), restart app | — | | | | | | | |
| TERM-088 | Transport drop: silent re-attach, full redraw; input typed meanwhile flushed in order | web/src/features/terminal/hooks/use-terminal-connection.ts:52 | restart daemon while typing. Seed: rig daemon restart command | — | | | | | | | |
| TERM-089 | Daemon unreachable (could not list): stays detached, re-attaches when host up | web/src/features/terminal/hooks/use-terminal-attachment.ts:137 | stop daemon, wait, start. Seed: rig daemon stop/start | — | | | | | | | |
| TERM-090 | Attach/create failure: toast "Terminal unavailable" / "Could not start the terminal. Try reopening the tab." | web/src/features/terminal/hooks/use-terminal-attachment.ts:114 | ctrl+j with daemon failing PTY create (toast: see TST). Seed: daemon PTY create failure | — | | | | | | | |
| TERM-091 | Host notice overlay "Waiting for this terminal’s host…" while host unresolved | web/src/features/terminal/components/terminal-host-notice.tsx:21 | restore app with project list not yet read. Seed: slow/paired host resolution | — | | | | | | | |
| TERM-092 | Host notice "The host this terminal runs on is not available right now" | web/src/features/terminal/components/terminal-host-notice.tsx:22 | Seed: workspace on removed host | — | | | | | | | |
| TERM-093 | Host notice "Its project is no longer in your list" | web/src/features/terminal/components/terminal-host-notice.tsx:23 | Seed: project removed while terminal tab open | — | | | | | | | |
| TERM-094 | Remote host unreachable notice "<name> is not available right now" + status detail | web/src/features/terminal/components/terminal-host-notice.tsx:34 | Seed: remote host, then make unreachable | — | | | | | | | |
| TERM-095 | Remote host "no live stream — this terminal can’t attach" notice | web/src/features/terminal/components/terminal-host-notice.tsx:38 | Seed: remote host refusing streams | — | | | | | | | |
| TERM-096 | Remote host probing: "Waiting for <name>…" | web/src/features/terminal/components/terminal-host-notice.tsx:42 | Seed: remote host in probing state | — | | | | | | | |
| TERM-097 | Local terminal never shows a host notice while re-attaching | web/src/features/terminal/components/terminal-host-notice.tsx:50 | restart daemon with terminal visible | — | | | | | | | |

Skipped:
- `web/src/features/terminal/components/terminal-key-bar.tsx`: mobile-only (`IS_MOBILE`) key bar.
- `web/src/features/agent/terminal/agent-terminal-surface.tsx`: chat's CLI terminal view (attach-only, agent frame), owned by CHAT.
- `initialCommand` path (`use-terminal-connection.ts:85`): no desktop caller opens a terminal with a command.
- `terminal-store.ts` widthMode/tabLayout/tabSidebar*: no UI reads them.
- `watchDevicePixelRatio` refit (`refit.ts:44`): DPR never changes in Xvfb rig.
- `utils/input-tape.ts`: diagnostic recorder, no visible output.
- Terminal file drop in a plain browser (non-Tauri): web build only.
