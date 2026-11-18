# 10. Chat view (CHAT)

### Pane-level states and the blank chat

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| CHAT-001 | Chat pane before its host resolves: centred "Opening this chat…" | web/src/features/agent/components/agent-chat-unavailable.tsx:4 | Open a chat tab right after `rig reset` (first frames). Seed: host resolution delayed | — | | | | | | | |
| CHAT-002 | "Chat unavailable" + "The host this chat runs on is not available right now." | web/src/features/agent/components/agent-chat-unavailable.tsx:5 | Seed: chat whose host is unreachable | — | | | | | | | |
| CHAT-003 | "Chat unavailable" + "Its project is no longer in your list. You can close this tab." | web/src/features/agent/components/agent-chat-unavailable.tsx:9 | Seed: open chat tab whose project was removed | — | | | | | | | |
| CHAT-004 | Blank chat = empty document: full-pane writing surface, serif placeholder "Describe the change…" as first line | web/src/features/agent/composer/agent-composer.tsx:221 | main tab (new chat), nothing typed | — | | | | | | | |
| CHAT-005 | Ghost prompts under the first line: GOAL / WHERE / KEEP with hints | web/src/features/agent/chat/agent-empty-document.tsx:14 | main tab, box empty | — | | | | | | | |
| CHAT-006 | Ghost prompts disappear on first character | web/src/features/agent/chat/agent-empty-document.tsx:122 | main tab > type `a` | — | | | | | | | |
| CHAT-007 | Document handle (capsule: selection chips left, + and send right) rides 4px under the LAST written line, not the caret | web/src/features/agent/chat/agent-empty-document.tsx:95 | main tab > type 3 lines (Ctrl+Enter avoided; plain Enter) > click into line 1 | instant (no transition) | | | | | | | |
| CHAT-008 | Document first block becomes a serif h1 title once it holds text; emptied → plain line again | web/src/features/agent/composer/plate/chat-composer-structure.ts:118 | main tab > type `Title`; then delete all | — | | | | | | | |
| CHAT-009 | Enter at end of title jumps caret to body line (no new block) | web/src/features/agent/composer/plate/chat-composer-structure.ts:122 | main tab > type `Title` > Enter | — | | | | | | | |
| CHAT-010 | Leading `/` in document is not a title (stays a command line) | web/src/features/agent/composer/plate/chat-composer-structure.ts:143 | main tab > type `/` | — | | | | | | | |
| CHAT-011 | Plain Enter in document = new line; Ctrl+Enter (mod+Enter) sends | web/src/features/agent/chat/lib/composer-keys.ts:71 | main tab > type text > Enter (newline) > `rig key ctrl+Return` | — | | | | | | | |
| CHAT-012 | Note line "No agent is installed. Add one in Settings → Agents to start this chat." | web/src/features/agent/chat/agent-chat-view.tsx:232 | main tab (no provider installed) | — | | | | | | | |
| CHAT-013 | Note line "This chat has no agent yet. Pick one, then send a message to start it." | web/src/features/agent/chat/agent-chat-view.tsx:231 | Seed: a connected+enabled provider, chat with no provider | — | | | | | | | |
| CHAT-014 | Sending with no agent: the note repeated as the error line under the box | web/src/features/agent/chat/agent-chat-view.tsx:237 | main tab > type `hi` > Ctrl+Enter | — | | | | | | | |
| CHAT-015 | Draft chat (no chat behind it yet): blank document without + attach button | web/src/features/agent/components/draft-chat-pane.tsx:116 | New chat from sidebar/tab "+" (draft id) | — | | | | | | | |
| CHAT-016 | Draft chat send: box clears, send circle shows spinner while chat is created; refused create puts prompt back above any newer text | web/src/features/agent/components/draft-chat-pane.tsx:47 | Draft chat > type > Enter. Seed: provider able to create chat | FlickerSpinner (stepped strip, duration per chosen spinner SVG, infinite) | | | | | | | |
| CHAT-017 | Draft chat error line (refusal e.g. too large) under document | web/src/features/agent/components/draft-chat-pane.tsx:153 | Draft chat > paste >64KB text (Shift+paste) > Enter | — | | | | | | | |
| CHAT-018 | Draft chat: Enter sends (draft uses send-key, not document Enter rule) | web/src/features/agent/components/draft-chat-pane.tsx:84 | Draft chat > type > Enter | — | | | | | | | |
| CHAT-019 | Blank chat with terminal wait: warning capsule in handle slot "X is asking whether you trust this folder. Crowbar can't answer this one." + answer buttons + "Open Terminal" | web/src/features/agent/components/agent-terminal-wait-banner.tsx:36 | Seed: blank chat whose CLI is in terminal_wait kind=trust (with declared answers) | — | | | | | | | |
| CHAT-020 | Same banner, other kind: "X is waiting for input in its terminal." | web/src/features/agent/components/agent-terminal-wait-banner.tsx:33 | Seed: blank chat, terminal_wait of unknown kind | — | | | | | | | |
| CHAT-021 | Blank chat reviving: spinner signpost with revival message in handle slot | web/src/features/agent/components/agent-chat-pane.tsx:279 | Seed: blank chat whose runner is being revived | FlickerSpinner | | | | | | | |
| CHAT-022 | Pickers in blank chat render in `.blank-picker` slot (above document) | web/src/features/agent/chat/agent-chat-view.tsx:410 | main tab > type `/` (Seed: slash catalogue) or `@` | — | | | | | | | |
| CHAT-023 | First send from blank: dock slides from the handle's position to rest; underbar hidden then fades in | web/src/features/agent/chat/lib/arrival-animation.ts:19 | main tab > type > Ctrl+Enter. Seed: a provider that accepts the send | transform 320ms cubic-bezier(0,0,0.2,1); underbar opacity+translateY(-4px) 200ms ease-out (composer-dock.css:79) | | | | | | | |
| CHAT-024 | Transcript fades in when the chat mounts or is shown (tab switch, first send) | web/src/features/agent/styles/transcript-layout.css:39 | Switch from main tab to Perf fixture tab | opacity 0→1 320ms cubic-bezier(0,0,0.2,1) via @starting-style (transcript-stream.css:140); none under reduced motion | | | | | | | |
| CHAT-025 | Type-to-focus: 4 quick printable keys while focus is outside any field redirect into the composer (≤1s gaps) | web/src/features/agent/chat/lib/type-to-focus.ts:12 | Perf fixture tab > click transcript blank area > `rig type Perf fixture hello` | — | | | | | | | |
| CHAT-026 | Type-to-focus ignores keys while another input is focused / modifier held | web/src/features/agent/chat/lib/type-to-focus.ts:52 | Perf fixture tab > open model search (Seed) or ctrl+key | — | | | | | | | |

### Transcript layout, scrolling and loading

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| CHAT-027 | "Loading messages…" with spinner while feed connects and nothing is loaded | web/src/features/agent/transcript/agent-transcript.tsx:429 | Open Perf fixture tab right after `rig reset` (first frames) | FlickerSpinner | | | | | | | |
| CHAT-028 | Feed error, empty: "Couldn't reach this chat. Crowbar keeps trying." + ghost "Terminal" button | web/src/features/agent/transcript/agent-transcript.tsx:434 | Seed: chat whose feed fails | — | | | | | | | |
| CHAT-029 | Feed error with rows: rows stay, composer becomes "unfollowed" signpost with the error | web/src/features/agent/chat/agent-chat-view.tsx:141 | Seed: feed failure after load | — | | | | | | | |
| CHAT-030 | Conversation lands at the bottom on open (bottom-anchored) | web/src/features/agent/transcript/agent-transcript.tsx:265 | Open Perf fixture tab | — | | | | | | | |
| CHAT-031 | Centred column (max 720px measure), 18px row gap; tool/marker rows flush; notice cards 10px | web/src/features/agent/transcript/agent-transcript.tsx:448 | Perf fixture tab at standard size | — | | | | | | | |
| CHAT-032 | Text scrolling under the dock dissolves (7 progressive backdrop-blur layers + 24px mask fade) | web/src/features/agent/chat/dock-stack.tsx:52 | Perf fixture tab > scroll up 3 so text sits behind composer | — | | | | | | | |
| CHAT-033 | Text under the overlay header fades (mask to header clearance) | web/src/features/agent/styles/transcript-layout.css:48 | Perf fixture tab > scroll up 3; look at top edge | — | | | | | | | |
| CHAT-034 | Wheel scroll up releases bottom follow (reader takes over) | web/src/features/agent/hooks/use-transcript-anchor.ts:62 | Perf fixture tab > scroll transcript up 1 | — | | | | | | | |
| CHAT-035 | Keyboard read-up keys (ArrowUp/PageUp/Home/Shift+Space) in transcript release follow | web/src/features/agent/hooks/use-transcript-anchor.ts:62 | Perf fixture tab > click transcript > `rig key Prior` | — | | | | | | | |
| CHAT-036 | "Jump to latest" round button (arrow down) appears when > 1/3 viewport from end; sticky 12px above dock | web/src/features/agent/transcript/jump-to-latest.tsx:37 | Perf fixture tab > scroll transcript up 10 | — | | | | | | | |
| CHAT-037 | Jump to latest hidden within 1/3 viewport of the end / when stuck | web/src/features/agent/hooks/use-transcript-anchor.ts:8 | Perf fixture tab > scroll up 1 (small) | — | | | | | | | |
| CHAT-038 | Click Jump to latest: eases back to bottom, button disappears | web/src/features/agent/transcript/agent-transcript.tsx:409 | Perf fixture tab > scroll up 10 > click Jump to latest | exponential follow τ=100ms per frame, max 32ms/frame (follow-scroll.ts:12) | | | | | | | |
| CHAT-039 | Auto-scroll follows growing reply while at bottom (within 96px) | web/src/features/agent/hooks/lib/follow-scroll.ts:24 | Seed: a streaming assistant reply | exponential ease τ=100ms | | | | | | | |
| CHAT-040 | Reader scrolled up while reply streams: view stays put, rows above don't shift | web/src/features/agent/transcript/agent-transcript.tsx:158 | Seed: streaming reply; scroll up 5 while it grows | — | | | | | | | |
| CHAT-041 | On send, the new prompt is pinned to the top with blank room below for the reply (desktop) | web/src/features/agent/transcript/agent-transcript.tsx:328 | Perf fixture tab > send a prompt. Seed: provider accepting it | — | | | | | | | |
| CHAT-042 | Pin released when turn ends: reserved room slides away | web/src/features/agent/styles/transcript-layout.css:113 | After CHAT-041 turn ends while at bottom | min-height 560ms cubic-bezier(0.4,0,0.2,1); none under reduced motion | | | | | | | |
| CHAT-043 | Scroll position kept per chat across tab switches (stuck or distance from bottom) | web/src/features/agent/chat/agent-chat-view.tsx:148 | Perf fixture tab > scroll up 10 > switch to main tab > back | — | | | | | | | |
| CHAT-044 | Older turns page in when resting within 2 viewports of the first loaded row, place kept | web/src/features/agent/hooks/use-older-turns.ts:7 | Perf fixture tab > scroll up 30 to the top. Seed: chat longer than one feed window | — | | | | | | | |
| CHAT-045 | Copy a selection from the transcript drops invisible caret anchors | web/src/features/agent/transcript/lib/plain-copy.ts:8 | Perf fixture tab > drag-select across replies > ctrl+c > paste in composer | — | | | | | | | |
| CHAT-046 | Background tab: transcript stays mounted, hidden; Jump button not drawn while hidden | web/src/features/agent/transcript/agent-transcript.tsx:480 | Perf fixture scrolled up > switch tab > back | — | | | | | | | |
| CHAT-047 | Narrow pane: column and dock padding at min window size | web/src/features/agent/styles/composer-dock.css:22 | `rig size min` on Perf fixture tab | — | | | | | | | |

### User messages

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| CHAT-048 | First turn "frozen": first user prompt full-width plain text, no bubble | web/src/features/agent/transcript/rows/user-row.tsx:207 | Perf fixture tab > scroll to top ("Synthetic prompt #1") | — | | | | | | | |
| CHAT-049 | First-turn divider (wavy rule) under the first prompt | web/src/features/agent/transcript/first-turn-divider.tsx:3 | Perf fixture tab > scroll to top | — | | | | | | | |
| CHAT-050 | First assistant reply also frozen (full width) | web/src/features/agent/transcript/rows/text-row.tsx:61 | Perf fixture tab > top, "Synthetic reply #1" | — | | | | | | | |
| CHAT-051 | Later user prompt: right-aligned stadium bubble, muted fill, max 88% | web/src/features/agent/transcript/rows/user-row.tsx:207 | Perf fixture tab, "Synthetic prompt #30" | — | | | | | | | |
| CHAT-052 | Multi-line bubble (>60 chars or newline): 18px radius rounded rectangle | web/src/features/agent/transcript/rows/user-row.tsx:175 | Seed: user message >60 chars or with newline | — | | | | | | | |
| CHAT-053 | User bubble renders markdown (bold, code, lists) and diagrams | web/src/features/agent/transcript/rows/user-row.tsx:209 | Seed: user message with markdown | — | | | | | | | |
| CHAT-054 | Sending prompt (outbox, no row yet): bubble with solid border `.queued.sent` | web/src/features/agent/transcript/rows/user-row.tsx:191 | Perf fixture tab > send. Seed: provider accepting the send | — | | | | | | | |
| CHAT-055 | Pending prompt (row landed, provider not yet taken it): dashed-border bubble | web/src/features/agent/transcript/rows/user-row.tsx:46 | Seed: crowbar-origin prompt fact state pending | — | | | | | | | |
| CHAT-056 | Refused prompt: warning-striped bubble + "The provider never took this prompt. Retry sends it again." + Retry and Edit icon buttons | web/src/features/agent/transcript/rows/user-row.tsx:49 | Seed: crowbar-origin prompt fact state refused | — | | | | | | | |
| CHAT-057 | Refused prompt, session lost text "The session was lost. Start a new conversation to send this." | web/src/features/agent/transcript/rows/user-row.tsx:50 | Seed: refused prompt reason session_lost | — | | | | | | | |
| CHAT-058 | Refused prompt Retry hidden once a retry is queued | web/src/features/agent/transcript/rows/user-row.tsx:123 | Seed as CHAT-056 > click Retry | — | | | | | | | |
| CHAT-059 | Refused prompt Edit: text back in composer, row discarded | web/src/features/agent/chat/use-chat-send.ts:29 | Seed as CHAT-056 > click Edit | — | | | | | | | |
| CHAT-060 | Refused prompt as first turn: frozen `firstq` variant with error and acts | web/src/features/agent/transcript/rows/user-row.tsx:189 | Seed: refused first prompt | — | | | | | | | |
| CHAT-061 | Retry/Edit icon tooltips ("Retry", "Edit") | web/src/features/agent/transcript/rows/user-row.tsx:127 | Seed as CHAT-056 > hover icons | tooltip (→ MENU/DLG/TST) | | | | | | | |
| CHAT-062 | Slash-command prompt drawn as marker: mono moss `/cmd` chip + muted args, no bubble | web/src/features/agent/transcript/rows/user-row.tsx:97 | Seed: settled user message `/review fast` | — | | | | | | | |
| CHAT-063 | Built-in slash run in CLI: "Ran in the terminal" after the command | web/src/features/agent/transcript/rows/user-row.tsx:102 | Seed: prompt fact settled with builtin=true | — | | | | | | | |
| CHAT-064 | Harness message: "Sent to the agent by X — not by you" + inline code | web/src/features/agent/transcript/rows/user-row.tsx:62 | Seed: user item origin=harness, short text | — | | | | | | | |
| CHAT-065 | Harness message long (>160 chars or >3 lines): `<details>` "Show payload" collapsed, opens to pre | web/src/features/agent/transcript/rows/user-row.tsx:70 | Seed: long harness message > click Show payload | — | | | | | | | |
| CHAT-066 | Provider note message: notice alert span, no bubble | web/src/features/agent/transcript/rows/user-row.tsx:68 | Seed: user item origin=note | — | | | | | | | |
| CHAT-067 | Message render failure: "This message could not be displayed. The rest of the chat is not affected." + "Show as plain text" + "Try again" | web/src/features/agent/transcript/rows/message-boundary.tsx:4 | Seed: a message whose markdown throws in the renderer | — | | | | | | | |
| CHAT-068 | Render failure "Show as plain text" reveals raw text, button hides | web/src/features/agent/transcript/rows/message-boundary.tsx:13 | As CHAT-067 > click Show as plain text | — | | | | | | | |

### Assistant replies and markdown

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| CHAT-069 | Assistant reply prose, left-aligned full width (14px/1.7) | web/src/features/agent/transcript/rows/text-row.tsx:54 | Perf fixture tab, any "Synthetic reply #i" | — | | | | | | | |
| CHAT-070 | Bold text (`**markdown**`) | web/src/features/agent/styles/transcript-prose.css:80 | Perf fixture tab, reply text | — | | | | | | | |
| CHAT-071 | Inline code span (`` `code` ``) | web/src/features/agent/styles/transcript-prose.css:60 | Perf fixture tab, reply text | — | | | | | | | |
| CHAT-072 | Headings h1/h2/h3 (serif) | web/src/features/agent/styles/transcript-prose.css:12 | Seed: reply with `#`/`##`/`###` | — | | | | | | | |
| CHAT-073 | Bulleted / numbered lists (incl. nested, 16px indent) | web/src/features/agent/styles/transcript-prose.css:38 | Seed: reply with lists | — | | | | | | | |
| CHAT-074 | Fenced code block, shiki-highlighted, muted panel; NO copy button | web/src/features/agent/composer/plate/attachments/chat-code-block-node.tsx:72 | Seed: reply with ```go fenced block | — | | | | | | | |
| CHAT-075 | Code block re-highlights when its grammar loads late | web/src/features/agent/transcript/plate/markdown-message-static.tsx:54 | Seed: reply with fence in a not-yet-loaded language | — | | | | | | | |
| CHAT-076 | Link in reply: primary colour, underlined; click opens external URL | web/src/features/agent/styles/transcript-prose.css:98 | Seed: reply with `[x](https://…)` > click | — | | | | | | | |
| CHAT-077 | Table (GFM) with header row | web/src/features/agent/styles/transcript-prose.css:103 | Seed: reply with markdown table | — | | | | | | | |
| CHAT-078 | Blockquote and horizontal rule | web/src/features/agent/styles/transcript-prose.css:87 | Seed: reply with `>` and `---` | — | | | | | | | |
| CHAT-079 | Callout block | web/src/features/agent/composer/plate/chat-composer-plugins.ts:87 | Seed: reply with callout markdown | — | | | | | | | |
| CHAT-080 | Image in message: max 320px tall, hover darkens, cursor zoom-in, title "Click to expand" | web/src/features/agent/composer/plate/attachments/chat-markdown-image-node.tsx:39 | Seed: message with an image attachment | overlay bg transition-colors 150ms cubic-bezier(0.4,0,0.2,1) | | | | | | | |
| CHAT-081 | Image lightbox: borderless image ≤88vw/88vh over backdrop, round X close | web/src/features/agent/composer/plate/attachments/chat-image-lightbox.tsx:27 | As CHAT-080 > click image; Esc or X to close | scale .98→1 + opacity 200ms ease-in-out; backdrop 200ms (dialog.tsx:23) | | | | | | | |
| CHAT-082 | Attachment file link in message renders as file card (icon, name, size) | web/src/features/agent/composer/plate/attachments/chat-attachment-file-card.tsx:141 | Seed: sent message with an attachment link | — | | | | | | | |
| CHAT-083 | Pasted-text attachment in message: "Pasted text" pill with line count; click opens raw-text dialog | web/src/features/agent/composer/plate/attachments/text-attachment-pill.tsx:24 | Seed: sent message with text-attachment fence > click | dialog 200ms (→ MENU/DLG/TST) | | | | | | | |
| CHAT-084 | Mermaid diagram fence renders as diagram | web/src/features/agent/composer/plate/attachments/diagram-block.tsx:70 | Seed: reply with ```mermaid | — | | | | | | | |
| CHAT-085 | Diagram skeleton (160px) while mermaid renders / fence streaming, then grows to height | web/src/features/agent/composer/plate/attachments/diagram-block.tsx:42 | Seed: streaming reply with open mermaid fence | height 160px→actual 220ms ease-out; skeleton pulse 2s linear infinite; none under reduced motion | | | | | | | |
| CHAT-086 | Reply settled with fence left open: diagram shows as plain code | web/src/features/agent/composer/plate/attachments/diagram-block.tsx:80 | Seed: settled reply with unclosed mermaid fence | — | | | | | | | |
| CHAT-087 | Excalidraw fence in message: preview image (dark-mode filter) + "Edit diagram" pencil (opens takeover) | web/src/features/agent/composer/plate/attachments/excalidraw-preview.tsx:132 | Seed: message with excalidraw fence + PNG > click pencil | — | | | | | | | |
| CHAT-088 | Excalidraw preview placeholder (element count) while PNG/SVG loads or fails | web/src/features/agent/composer/plate/attachments/excalidraw-preview.tsx:152 | Seed: excalidraw fence with missing PNG | — | | | | | | | |
| CHAT-089 | Turn bar under the reply that hands control back: model, effort, Copy icon, time "Aug 30, 6:36 PM"; hidden until hover | web/src/features/agent/transcript/rows/text-row.tsx:71 | Perf fixture tab > hover "Synthetic reply #30" | opacity 0.15s (transcript-tools.css:9) | | | | | | | |
| CHAT-090 | Turn bar shows only copy+time when turn facts carry no model/effort | web/src/features/agent/transcript/rows/text-row.tsx:73 | Perf fixture tab > hover a reply (seed has no model) | — | | | | | | | |
| CHAT-091 | Turn time tooltip: full date "medium/short" | web/src/features/agent/lib/turn-time.ts:27 | Perf fixture tab > hover reply > hover time | native title | | | | | | | |
| CHAT-092 | Copy reply → toast "Copied to clipboard" (failure: "Could not copy to clipboard") | web/src/features/agent/transcript/rows/text-row.tsx:14 | Perf fixture tab > hover reply > click Copy (toast → MENU/DLG/TST) | — | | | | | | | |
| CHAT-093 | No turn bar on a reply followed by tool calls in the same run, nor while working | web/src/features/agent/transcript/agent-transcript.tsx:196 | Seed: turn with text, then tools, then text | — | | | | | | | |
| CHAT-094 | Streaming reply: words fade in (per-word cascade), no cursor/caret glyph | web/src/features/agent/transcript/plate/chat-fresh-text-plugin.tsx:17 | Seed: a streaming assistant reply | opacity 0→1 260ms ease-out, per-word animation-delay, fill both (transcript-stream.css:121); none under reduced motion | | | | | | | |
| CHAT-095 | Streaming reply markdown restructures in place (list/heading forms as tokens arrive) | web/src/features/agent/transcript/plate/markdown-message.tsx:42 | Seed: streaming reply with markdown | — | | | | | | | |
| CHAT-096 | Phase-tagged text row (data-phase, e.g. commentary) | web/src/features/agent/transcript/rows/text-row.tsx:58 | Seed: text item with phase | — | | | | | | | |

### Tool rows

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| CHAT-097 | Folded run of consecutive read calls: one line "Read 5 files" + caret toggle | web/src/features/agent/transcript/rows/tool-run-row.tsx:18 | Perf fixture tab, any turn. Seed: read_file calls with kind=read (the seed sets no kind, so it shows 5 separate rows) | — | | | | | | | |
| CHAT-098 | Expand folded run: each call's own row listed | web/src/features/agent/transcript/rows/tool-run-row.tsx:50 | Perf fixture tab > click run caret. Seed: as CHAT-097 | caret rotate 90° 120ms ease (transcript-tools.css:86) | | | | | | | |
| CHAT-099 | Collapse run again | web/src/features/agent/transcript/rows/tool-run-row.tsx:45 | Perf fixture tab > click caret twice. Seed: as CHAT-097 | 120ms ease | | | | | | | |
| CHAT-100 | Run sentence mixes families: "Read 2 files, ran 1 search, fetched 1 page" | web/src/features/agent/transcript/lib/tool-runs.ts:61 | Seed: turn with read+search+fetch calls | — | | | | | | | |
| CHAT-101 | Live run: spinner + "Reading 2 files…" shimmer + ticker of newest call | web/src/features/agent/transcript/rows/tool-run-row.tsx:30 | Seed: running read calls | shimmer 1.6s linear infinite (transcript-tools.css:205); FlickerSpinner | | | | | | | |
| CHAT-102 | Run cut around a call that failed/asked: that call stands alone, the rest fold either side | web/src/features/agent/transcript/rows/tool-run-row.tsx:65 | Seed: read run with one error call in the middle | — | | | | | | | |
| CHAT-103 | Read row ok: check glyph, file icon, "Read" + path subject (L-line, +N more) | web/src/features/agent/transcript/rows/tool-row.tsx:141 | Perf fixture tab > expand run | — | | | | | | | |
| CHAT-104 | Tool row hover tooltip = cost: duration "50ms" (and +N −M for edits) | web/src/features/agent/transcript/rows/tool-row.tsx:127 | Perf fixture tab > expand run > hover a call | native title | | | | | | | |
| CHAT-105 | Tool subject hover shows full path; subject not clickable | web/src/features/agent/transcript/rows/tool-row.tsx:109 | Perf fixture tab > expand run > hover file_i_j.go | — | | | | | | | |
| CHAT-106 | Tool row details caret only when body has input/result ref, output, error or diff | web/src/features/agent/transcript/rows/tool-row.tsx:123 | Perf fixture tab > expand run (caret present?) | — | | | | | | | |
| CHAT-107 | Expanded tool row: short output pre (mono, muted, max 7.5em) | web/src/features/agent/transcript/rows/tool-row.tsx:197 | Seed: tool call with output > expand | 120ms caret | | | | | | | |
| CHAT-108 | Expanded payload: "Request" / "Result" sides, "Loading…" | web/src/features/agent/transcript/rows/tool-payload.tsx:39 | Seed: tool call with inputRef/resultRef > expand | — | | | | | | | |
| CHAT-109 | Payload load failed: "Couldn't load this. Try again" | web/src/features/agent/transcript/rows/tool-payload.tsx:43 | Seed: payload blob read failing | — | | | | | | | |
| CHAT-110 | Payload swept: "No longer available" | web/src/features/agent/transcript/rows/tool-payload.tsx:50 | Seed: payload ref whose blob is gone | — | | | | | | | |
| CHAT-111 | Running tool: spinner, live verb shimmer ("Reading", "Editing", "Running", "Searching for", "Fetching") | web/src/features/agent/transcript/rows/tool-row.tsx:84 | Seed: tool call status running | shimmer 1.6s linear infinite | | | | | | | |
| CHAT-112 | Failed tool: X glyph + "Failed" outcome pill | web/src/features/agent/transcript/rows/tool-row.tsx:52 | Seed: tool call status error | — | | | | | | | |
| CHAT-113 | Expanded failed tool shows error text in output pre | web/src/features/agent/transcript/rows/tool-row.tsx:199 | Seed: errored tool with error body > expand | — | | | | | | | |
| CHAT-114 | Declined tool: prohibit glyph + "Denied" | web/src/features/agent/transcript/rows/tool-row.tsx:53 | Seed: tool call status declined | — | | | | | | | |
| CHAT-115 | Interrupted tool: filled stop glyph + "Stopped" | web/src/features/agent/transcript/rows/tool-row.tsx:54 | Seed: tool call status interrupted | — | | | | | | | |
| CHAT-116 | Abandoned tool: dashed circle + "Result unavailable"; verb in plain form ("Read") | web/src/features/agent/transcript/rows/tool-row.tsx:55 | Seed: tool call status abandoned | — | | | | | | | |
| CHAT-117 | Tool awaiting an ask: dashed circle + "Needs your answer", live verb, scrolled into view | web/src/features/agent/transcript/rows/tool-row.tsx:66 | Seed: tool call with pending askId | — | | | | | | | |
| CHAT-118 | Tool ask note pill: "Allowed" / "Allowed · <grant>" / "Allowed in terminal" | web/src/features/agent/transcript/rows/tool-row.tsx:149 | Seed: tool call with resolved allow ask | — | | | | | | | |
| CHAT-119 | Tool ask note pill denied tone: "Denied" | web/src/features/agent/transcript/rows/tool-row.tsx:61 | Seed: tool call with resolved deny ask | — | | | | | | | |
| CHAT-120 | Tool ask note pill dashed: "Timed out" / "Not answered" | web/src/features/agent/transcript/rows/tool-row.tsx:62 | Seed: tool call whose ask was refused/abandoned | — | | | | | | | |
| CHAT-121 | Edit tool: pencil icon, "Edited <path>" | web/src/features/agent/transcript/lib/tool-label.ts:7 | Seed: edit tool call with diff | — | | | | | | | |
| CHAT-122 | Edit tool creating a file: "Created <path>" / live "Creating" | web/src/features/agent/transcript/lib/tool-label.ts:14 | Seed: edit with `--- /dev/null` diff | — | | | | | | | |
| CHAT-123 | Expanded edit: inline unified diff (sticky file header, simple hunk separators, highlighted) | web/src/features/agent/activity/turn-diff-preview.tsx:40 | Seed: edit tool with diff > expand | 120ms caret | | | | | | | |
| CHAT-124 | Edit diff too large: "Diff preview is too large to render live." | web/src/features/agent/activity/turn-diff-preview.tsx:33 | Seed: edit with very large diff > expand | — | | | | | | | |
| CHAT-125 | Edit diff unparseable: raw diff pre | web/src/features/agent/activity/turn-diff-preview.tsx:37 | Seed: edit with non-unified diff text > expand | — | | | | | | | |
| CHAT-126 | Live edit diff streams into an expanded running edit | web/src/features/agent/transcript/rows/tool-row.tsx:120 | Seed: running edit with live diff stream | — | | | | | | | |
| CHAT-127 | Execute tool: terminal icon, "Ran <program args>" (sudo/env/sh -c peeled, first line) | web/src/features/agent/transcript/lib/tool-label.ts:45 | Seed: execute tool `bash -c "go test ./..."` | — | | | | | | | |
| CHAT-128 | Search tool: magnifier, `Searched "pattern"` | web/src/features/agent/transcript/lib/tool-label.ts:92 | Seed: search tool with target | — | | | | | | | |
| CHAT-129 | Fetch tool: globe icon, "Fetched host/…" (host only while live) | web/src/features/agent/transcript/lib/tool-label.ts:93 | Seed: fetch tool with URL target | — | | | | | | | |
| CHAT-130 | Subagent-family tool: "Ran subagent <title>" | web/src/features/agent/transcript/lib/tool-label.ts:11 | Seed: tool kind subagent with title | — | | | | | | | |
| CHAT-131 | MCP tool: coloured server badge (5 hues by name) + " · tool name" | web/src/features/agent/transcript/rows/mcp-server-badge.tsx:13 | Seed: tool call with mcpServer | — | | | | | | | |
| CHAT-132 | Unknown-kind tool: generic tool icon + humanised name ("Read file") | web/src/features/agent/transcript/lib/tool-label.ts:115 | Seed: tool with no known kind | — | | | | | | | |
| CHAT-133 | Crowbar's own MCP tools and plan/meta calls hidden from transcript (unless asked about) | web/src/features/agent/transcript/lib/tool-runs.ts:34 | Seed: mcp__crowbar__ call in a turn | — | | | | | | | |
| CHAT-134 | Resolved ask record row: "<tool> · Allow" / "· answered at the terminal" / "· refused, no answer in time" / "· left unanswered" | web/src/features/agent/transcript/rows/tool-row.tsx:217 | Seed: resolved ask items of each resolution | — | | | | | | | |

### Reasoning, plans, subagents, markers and notices

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| CHAT-135 | Reasoning row collapsed: "Thought for Ns" (or "Thought") + caret | web/src/features/agent/transcript/rows/reasoning-row.tsx:10 | Seed: reasoning item with start/end | — | | | | | | | |
| CHAT-136 | Reasoning expanded: markdown text, caret rotated 90° | web/src/features/agent/transcript/rows/reasoning-row.tsx:23 | Seed as CHAT-135 > click toggle | caret rotate instant (activity-plan.css:2) | | | | | | | |
| CHAT-137 | Plan snapshot row: "Tasks · d/n done" + caret | web/src/features/agent/transcript/rows/plan-row.tsx:18 | Seed: plan snapshot item | — | | | | | | | |
| CHAT-138 | Plan snapshot expanded: ordered steps with status styling | web/src/features/agent/transcript/rows/plan-row.tsx:21 | Seed as CHAT-137 > click | instant caret | | | | | | | |
| CHAT-139 | Subagent card running: spinner, robot icon, title, type, live clock, "▸ <action>" line | web/src/features/agent/transcript/rows/subagent-card.tsx:92 | Seed: running subagent | FlickerSpinner; clock 1s tick | | | | | | | |
| CHAT-140 | Subagent card "Parent waiting…" | web/src/features/agent/transcript/rows/subagent-card.tsx:147 | Seed: running subagent with parentWaiting | — | | | | | | | |
| CHAT-141 | Subagent done: check, took clock, "· N tool uses · Nk tokens" | web/src/features/agent/transcript/rows/subagent-card.tsx:31 | Seed: finished subagent with usage | — | | | | | | | |
| CHAT-142 | Subagent failed / stopped / unavailable outcome pill; failed with nothing: "Failed · could not start" | web/src/features/agent/transcript/rows/subagent-card.tsx:110 | Seed: subagents in failed, interrupted, abandoned | — | | | | | | | |
| CHAT-143 | Subagent card expanded: its own rows (tools, text, asks, nested subagents) + result markdown | web/src/features/agent/transcript/rows/subagent-card.tsx:148 | Seed: subagent with scope and result > click header | 120ms caret | | | | | | | |
| CHAT-144 | Subagent without scope: header not a button | web/src/features/agent/transcript/rows/subagent-card.tsx:140 | Seed: subagent with no scope | — | | | | | | | |
| CHAT-145 | Divider marker "Compacted" (wavy rule, pill tag) | web/src/features/agent/transcript/rows/marker-row.tsx:51 | Seed: compaction_ended marker | — | | | | | | | |
| CHAT-146 | Divider "Interrupted · you stopped this turn" | web/src/features/agent/transcript/rows/marker-row.tsx:54 | Seed: turn_interrupted reason=stopped | — | | | | | | | |
| CHAT-147 | Divider "Interrupted" / "Interrupted unexpectedly" (provider idle) | web/src/features/agent/transcript/lib/marker-failure.ts:105 | Seed: turn_interrupted reason other / provider_idle | — | | | | | | | |
| CHAT-148 | Divider "Session lost · continued from Crowbar's record" | web/src/features/agent/transcript/rows/marker-row.tsx:58 | Seed: from_record marker | — | | | | | | | |
| CHAT-149 | Divider "Switched to <provider>" | web/src/features/agent/transcript/rows/marker-row.tsx:62 | Seed: provider_switched marker | — | | | | | | | |
| CHAT-150 | Divider "Model: x" / "Effort: x" | web/src/features/agent/transcript/rows/marker-row.tsx:68 | Seed: model_changed / effort_changed markers | — | | | | | | | |
| CHAT-151 | Divider one launch's switches joined: "Switched to X · model · high effort" | web/src/features/agent/transcript/rows/marker-row.tsx:16 | Seed: marker with parts | — | | | | | | | |
| CHAT-152 | Divider for stopped turn: "Stopped · signed out of X" / "Stopped · usage limit reached" / "X was busy · this turn did not finish" / "Stopped · X had a problem on its side" | web/src/features/agent/transcript/lib/turn-failure.ts:64 | Seed: turn_failed markers kinds auth/limit/overloaded/other | — | | | | | | | |
| CHAT-153 | Divider "Stopped · session lost" | web/src/features/agent/transcript/lib/marker-failure.ts:84 | Seed: session_lost marker | — | | | | | | | |
| CHAT-154 | Divider "Stopped · X could not start: detail" | web/src/features/agent/transcript/lib/marker-failure.ts:94 | Seed: runner_exited reason spawn_failed | — | | | | | | | |
| CHAT-155 | Divider with slash command chip before result | web/src/features/agent/transcript/rows/marker-row.tsx:196 | Seed: marker with command | — | | | | | | | |
| CHAT-156 | Move dividers: "Moving to X after this turn" / "Moved to X — now working in dir" / "Move to X failed: r" / "Move cancelled" | web/src/features/agent/transcript/rows/move-marker.tsx:9 | Seed: move markers each phase | — | | | | | | | |
| CHAT-157 | Branch-out dividers: "Becoming a branch after this turn" / "Now on branch b — working in dir" / "Making a branch failed" / "Branch cancelled" | web/src/features/agent/transcript/rows/move-marker.tsx:25 | Seed: branchOut move markers | — | | | | | | | |
| CHAT-158 | Failed move "Retry" button under divider; disabled once sent | web/src/features/agent/transcript/rows/move-marker.tsx:64 | Seed: failed move marker with target > click Retry | — | | | | | | | |
| CHAT-159 | Generic marker: detail or marker name in pill | web/src/features/agent/transcript/rows/marker-row.tsx:74 | Seed: unknown marker | — | | | | | | | |
| CHAT-160 | Warning notice card: bold first sentence + rest | web/src/features/agent/transcript/rows/marker-row.tsx:138 | Seed: warning marker | — | | | | | | | |
| CHAT-161 | Context warning notice with "Compact now" outline button | web/src/features/agent/transcript/rows/marker-row.tsx:111 | Seed: warning mentioning context, idle chat with compaction provider | — | | | | | | | |
| CHAT-162 | Session-conflict notice "This session moved to another chat." + "Open that chat" | web/src/features/agent/transcript/rows/marker-row.tsx:84 | Seed: session_conflict marker with chat > click | — | | | | | | | |
| CHAT-163 | Legacy-unreadable notice "N older records could not be converted." + Show them / Hide them list | web/src/features/agent/transcript/rows/marker-row.tsx:89 | Seed: legacy_unreadable marker with records > toggle | — | | | | | | | |

### Working line and activity

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| CHAT-164 | Working line: spinner + rotating verb ("Brewing…") + " · 12s" clock | web/src/features/agent/activity/working-line.tsx:66 | Seed: running turn with no tool/stream/ask/todo | verb changes every 3400ms (verbs.ts:192); clock 1s; FlickerSpinner | | | | | | | |
| CHAT-165 | Working line headline from live reasoning + subtitle body | web/src/features/agent/activity/working-line.tsx:84 | Seed: running turn with live reasoning text | — | | | | | | | |
| CHAT-166 | Working line "Stopping…" after Stop pressed | web/src/features/agent/activity/working-line.tsx:34 | Seed: running turn > Esc | — | | | | | | | |
| CHAT-167 | Working line shows connecting text while switching CLI | web/src/features/agent/activity/working-line.tsx:35 | Seed: provider switch in flight | — | | | | | | | |
| CHAT-168 | Working line hidden when a tool runs, text streams, ask, active todo or compaction shows it | web/src/features/agent/activity/working-line.tsx:41 | Seed: running tool call | — | | | | | | | |
| CHAT-169 | Working line hidden when only subagents run | web/src/features/agent/activity/working-line.tsx:51 | Seed: running subagent, idle main | — | | | | | | | |
| CHAT-170 | Working line clock starts from Enter while sending, before the turn starts | web/src/features/agent/chat/use-composer-facts.ts:133 | Perf fixture tab > send. Seed: provider accepting | — | | | | | | | |

### Composer: input, keys and states

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| CHAT-171 | Dock pill empty: stadium, placeholder "Ask for a change, or type / for commands", + and grey send circle | web/src/features/agent/composer/agent-composer.tsx:227 | Perf fixture tab | — | | | | | | | |
| CHAT-172 | Placeholder while working/sending: "Queue a follow-up…" | web/src/features/agent/composer/agent-composer.tsx:224 | Seed: running turn | — | | | | | | | |
| CHAT-173 | Pill focus ring (ring-colour border + 3px halo) | web/src/features/agent/styles/composer-pill.css:43 | Perf fixture tab > click composer | box-shadow 0.18s ease | | | | | | | |
| CHAT-174 | Composer autofocuses on chat open | web/src/features/agent/composer/composer-field.tsx:62 | Open Perf fixture tab > type | — | | | | | | | |
| CHAT-175 | Send circle idle disabled (muted, title "Send — Enter") | web/src/features/agent/composer/composer-action-button.tsx:45 | Perf fixture tab, empty box > hover send | — | | | | | | | |
| CHAT-176 | Send circle active (primary, up arrow) once text typed | web/src/features/agent/composer/composer-action-button.tsx:66 | Perf fixture tab > type `x` | bg/border 0.12s ease | | | | | | | |
| CHAT-177 | Send press: circle scales to .94 | web/src/features/agent/styles/composer-pill.css:221 | Perf fixture tab > type > mouse-down on send | transform 0.12s ease | | | | | | | |
| CHAT-178 | Enter sends; Shift+Enter inserts newline (dock) | web/src/features/agent/chat/lib/send-key.ts:6 | Perf fixture tab > type > shift+Return > Return | — | | | | | | | |
| CHAT-179 | Enter during IME composition does not send | web/src/features/agent/chat/lib/send-key.ts:6 | IME (see IME) | — | | | | | | | |
| CHAT-180 | Multi-line growth: pill turns 18px-radius rectangle, padding 12px, send pinned bottom-right, + 4px above | web/src/features/agent/styles/composer-pill.css:24 | Perf fixture tab > type, shift+Return ×2 | instant (no easing) | | | | | | | |
| CHAT-181 | Single wrapped line: handle rides the last line | web/src/features/agent/composer/composer-handle.tsx:31 | Perf fixture tab > type a long sentence that wraps | instant | | | | | | | |
| CHAT-182 | Max height 50vh, then the field scrolls inside (stable gutter) | web/src/features/agent/styles/composer-pill.css:72 | Perf fixture tab > shift+Return ×30 | — | | | | | | | |
| CHAT-183 | Rich markdown in box: `**bold**` renders bold as typed | web/src/features/agent/composer/composer-field.tsx:36 | Perf fixture tab > type `**bold** ` | — | | | | | | | |
| CHAT-184 | Typing ``` fence creates a code block in the box; mod+alt+8 toggles code block | web/src/features/agent/composer/plate/chat-composer-plugins.ts:60 | Perf fixture tab > type ```` ``` ```` + space / `rig key ctrl+alt+8` | — | | | | | | | |
| CHAT-185 | URL typed/pasted becomes link (not inside code, no trailing punctuation) + link floating toolbar | web/src/features/agent/composer/plate/attachments/chat-link-kit.tsx:17 | Perf fixture tab > type `https://example.com ` > click link | — | | | | | | | |
| CHAT-186 | Selecting text in box shows mark floating toolbar | web/src/features/agent/composer/plate/chat-composer-plugins.ts:99 | Perf fixture tab > type words > shift+Home | — | | | | | | | |
| CHAT-187 | `/command` and `@path` tokens highlighted in the draft (chips) | web/src/features/agent/composer/plate/chat-composer-chips.tsx:8 | Perf fixture tab > type `/review @src/main.go` | — | | | | | | | |
| CHAT-188 | Ctrl+A selects the whole draft | web/src/features/agent/composer/plate/chat-markdown-editor.tsx:179 | Perf fixture tab > type 2 lines > ctrl+a | — | | | | | | | |
| CHAT-189 | Tab moves focus out of box (indents only in list/code) | web/src/features/agent/composer/plate/chat-composer-structure.ts:67 | Perf fixture tab > type > Tab | — | | | | | | | |
| CHAT-190 | Up arrow at caret start recalls previous own prompt; repeated walks older | web/src/features/agent/chat/lib/composer-keys.ts:90 | Perf fixture tab > empty box > Up (expect "Synthetic prompt #30") > Up | — | | | | | | | |
| CHAT-191 | Down arrow at caret end walks newer; past newest restores the live draft | web/src/features/agent/hooks/use-prompt-history.ts:34 | After CHAT-190 > Down ×2 | — | | | | | | | |
| CHAT-192 | Up inside a multi-line draft moves caret, no recall | web/src/features/agent/chat/lib/composer-keys.ts:87 | Perf fixture tab > type 2 lines > Up | — | | | | | | | |
| CHAT-193 | Ctrl+R reverse-search older prompts containing typed text | web/src/features/agent/chat/lib/composer-keys.ts:81 | Perf fixture tab > type `#1` > ctrl+r > ctrl+r | — | | | | | | | |
| CHAT-194 | Draft persists across tab switch / reload, restored without stealing focus | web/src/features/agent/composer/plate/chat-markdown-editor.tsx:376 | Perf fixture tab > type `draft` > switch to main tab > back | — | | | | | | | |
| CHAT-195 | Draft cleared on send | web/src/features/agent/composer/plate/chat-markdown-editor.tsx:366 | Perf fixture tab > type > Enter. Seed: provider accepting | — | | | | | | | |
| CHAT-196 | Send error alert line under dock (e.g. "Wait for the attached photo to finish uploading.", "This prompt is too large to submit.") | web/src/features/agent/stores/lib/outbox.ts:55 | Perf fixture tab > Shift+paste >64KB text > Enter | — | | | | | | | |
| CHAT-197 | Error line clears on next edit | web/src/features/agent/chat/use-composer-draft.ts:65 | After CHAT-196 > type a char | — | | | | | | | |
| CHAT-198 | Stop button (halt) when turn runs and box empty; title "Stop this turn — Esc" | web/src/features/agent/composer/composer-action-button.tsx:27 | Seed: running turn | — | | | | | | | |
| CHAT-199 | Queue button (queue icon) when text typed during a turn; title "Queue — sends when this turn ends" | web/src/features/agent/composer/composer-action-button.tsx:28 | Seed: running turn > type | — | | | | | | | |
| CHAT-200 | Sending visual: spinner in disabled circle, title "Sending…" | web/src/features/agent/composer/composer-action-button.tsx:29 | Perf fixture tab > send. Seed: slow delivery | FlickerSpinner | | | | | | | |
| CHAT-201 | Esc while turn runs and box empty: stops turn | web/src/features/agent/chat/lib/composer-keys.ts:51 | Seed: running turn > Esc | — | | | | | | | |
| CHAT-202 | Esc in empty box with unsent prompt: takes it back into the box | web/src/features/agent/chat/use-prompt-take-back.ts:54 | Seed: queued/sending prompt > Esc | — | | | | | | | |
| CHAT-203 | Take-back toasts: "Taking the prompt back…" loading; "Its provider already has this prompt — Stop ends its turn." | web/src/features/agent/stores/chat-outbox.ts:221 | As CHAT-202 (toast → MENU/DLG/TST) | — | | | | | | | |
| CHAT-204 | Stop before turn starts: prompt withdrawn back to box (or turn stopped if provider has it) | web/src/features/agent/chat/use-chat-send.ts:48 | Seed: prompt sending > click Stop | — | | | | | | | |
| CHAT-205 | Taken-back prompt with composer gone: stashed; failure toast "Couldn't keep the prompt you took back" | web/src/features/agent/chat/use-prompt-take-back.ts:41 | Seed: take-back after closing tab | — | | | | | | | |
| CHAT-206 | Focus returns to box when a request card leaves the slot (only if focus was on body) | web/src/features/agent/composer/agent-composer.tsx:212 | Seed: answer a pending ask | — | | | | | | | |
| CHAT-207 | Compaction: dot-matrix rain behind pill text; input stays usable | web/src/features/agent/composer/compaction-rain.tsx:70 | Seed: chat compacting | translateY 0→84px steps(14) infinite, 2.8s and 4.2s layers (composer-pill.css:380); paused under reduced motion | | | | | | | |
| CHAT-208 | Footer "Compacting…" label (primary, bold) left of gauge | web/src/features/agent/controls/provider-bar.tsx:65 | Seed: chat compacting | — | | | | | | | |
| CHAT-209 | Tray tuck: top tray tucked behind pill top edge, bottom tray behind bottom edge | web/src/features/agent/chat/dock-stack.tsx:23 | Seed: a queued prompt and a running shell | — | | | | | | | |

### Composer occupants (signposts, requests)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| CHAT-210 | Signpost "Offline — read-only" replaces the pill | web/src/features/agent/composer/lib/composer-state.ts:59 | Seed: chat host offline | — | | | | | | | |
| CHAT-211 | Signpost "Loading this chat…" (dormant/unknown) | web/src/features/agent/composer/lib/composer-state.ts:61 | Open Perf fixture tab right after `rig reset`, or no provider/runner | — | | | | | | | |
| CHAT-212 | Signpost "This provider cannot accept a prompt typed here." + "Terminal" button | web/src/features/agent/composer/composer-signpost.tsx:41 | Seed: outbox item code prompt_submit_unsupported | — | | | | | | | |
| CHAT-213 | Signpost terminal wait: "This provider is waiting for you to trust the workspace" / "…an answer only its terminal can give" + answers + Terminal | web/src/features/agent/composer/lib/composer-state.ts:46 | Seed: non-blank chat in terminal_wait (trust / other) | — | | | | | | | |
| CHAT-214 | Signpost reviving: spinner + message | web/src/features/agent/composer/composer-signpost.tsx:31 | Seed: reviving runner | FlickerSpinner | | | | | | | |
| CHAT-215 | Signpost Stop button while a turn runs or prompt held | web/src/features/agent/composer/composer-signpost.tsx:46 | Seed: running turn + terminal wait | — | | | | | | | |
| CHAT-216 | Permission request card: title by family ("Run a command", "Edit a file", "Read a file", "Fetch a web page", "Search", "Approve this plan", "Use a tool"), alert role | web/src/features/agent/composer/lib/ask-labels.ts:28 | Seed: pending tool_permission ask per family | — | | | | | | | |
| CHAT-217 | Permission title fallback "Run <toolName>" / "The agent is asking for permission" + target subtitle | web/src/features/agent/composer/lib/ask-labels.ts:44 | Seed: permission ask with unknown family | — | | | | | | | |
| CHAT-218 | Permission body: command code with shell prompt, `&&`/`\|\|` dimmed | web/src/features/agent/composer/composer-choice-request.tsx:62 | Seed: execute permission with command input | — | | | | | | | |
| CHAT-219 | Permission body: edit diff lines with numbers, −/+ signs, changed span marked; head path + "+N −M" | web/src/features/agent/composer/composer-choice-request.tsx:115 | Seed: edit permission with diff input | — | | | | | | | |
| CHAT-220 | Permission body: MCP args as labelled rows; head "server · tool" | web/src/features/agent/composer/composer-choice-request.tsx:100 | Seed: MCP permission with args | — | | | | | | | |
| CHAT-221 | Permission body: markdown prose (e.g. plan) scrolled in its bound | web/src/features/agent/composer/composer-choice-request.tsx:179 | Seed: plan permission with markdown | — | | | | | | | |
| CHAT-222 | Folded body (>12 lines or >240 chars/line): cut "…", warn cue "N more lines not shown", "Show all N lines" button | web/src/features/agent/composer/lib/ask-request.ts:40 | Seed: permission with long command > click Show all | — | | | | | | | |
| CHAT-223 | Body note line (agent's why / truncation note) | web/src/features/agent/composer/composer-choice-request.tsx:190 | Seed: permission with detail and no input | — | | | | | | | |
| CHAT-224 | Answer buttons: refusals left (outline), grants middle, primary Allow right; "Allow once" beside broader grant; digit kbd hints | web/src/features/agent/composer/composer-choice-controls.tsx:93 | Seed: permission with deny, allow, allow-for-session options | — | | | | | | | |
| CHAT-225 | Digit key answers a flat card; Enter only from the card itself | web/src/features/agent/composer/use-choice-card.ts:151 | Seed as CHAT-224 > press `1` | — | | | | | | | |
| CHAT-226 | "Answer sent. Waiting for X to confirm it." hint, buttons busy | web/src/features/agent/composer/composer-choice.tsx:197 | Seed as CHAT-224 > click Allow (daemon slow to resolve) | — | | | | | | | |
| CHAT-227 | Answer error line in card | web/src/features/agent/composer/composer-choice.tsx:202 | Seed: answer endpoint failing | — | | | | | | | |
| CHAT-228 | Card with answers only in terminal: "Terminal" button | web/src/features/agent/composer/composer-choice.tsx:210 | Seed: terminal-only ask | — | | | | | | | |
| CHAT-229 | Unanswerable here: "Answer this in the terminal. It offers:" + read-only option list | web/src/features/agent/composer/composer-choice-controls.tsx:159 | Seed: pending ask with no relay | — | | | | | | | |
| CHAT-230 | Question card single: radio rows with label, description, digit kbd | web/src/features/agent/composer/composer-choice-controls.tsx:56 | Seed: pending question ask (single) | — | | | | | | | |
| CHAT-231 | Question multi: checkboxes + "pick any" tag | web/src/features/agent/composer/composer-choice-controls.tsx:30 | Seed: multi question ask | — | | | | | | | |
| CHAT-232 | Question "Send answer" disabled until picked; Enter sends | web/src/features/agent/composer/composer-choice.tsx:236 | Seed as CHAT-230 > press `2` > Enter | — | | | | | | | |
| CHAT-233 | Several questions in one prompt: stepper, "Next ↵", "Skip", "Back", final "Send answers" | web/src/features/agent/composer/composer-choice.tsx:212 | Seed: question ask with 3 questions | — | | | | | | | |
| CHAT-234 | Question headline fallbacks "The agent has a question" / "The agent has N questions" | web/src/features/agent/composer/lib/ask-labels.ts:49 | Seed: question ask without text | — | | | | | | | |
| CHAT-235 | Several pending requests: carousel, dots, prev/next arrows (disabled at ends), "1 of 3", peeking edges | web/src/features/agent/composer/composer-choice-stepper.tsx:15 | Seed: 3 pending asks > click next | — | | | | | | | |
| CHAT-236 | Elicitation form: text/number inputs, boolean switch, choice segmented, required `*`, help text | web/src/features/agent/composer/composer-choice-form.tsx:27 | Seed: pending elicitation with schema | — | | | | | | | |
| CHAT-237 | Elicitation field error after leaving it / submit; form error beside Submit | web/src/features/agent/composer/composer-choice-form.tsx:46 | Seed as CHAT-236 > leave required empty > Tab | — | | | | | | | |
| CHAT-238 | Elicitation "Decline esc" / "Submit ↵"; Esc declines, Enter in field submits | web/src/features/agent/composer/use-choice-card.ts:166 | Seed as CHAT-236 > Esc / Enter | — | | | | | | | |
| CHAT-239 | Elicitation "This form can only be filled in the terminal." | web/src/features/agent/composer/composer-choice.tsx:186 | Seed: elicitation needing terminal | — | | | | | | | |
| CHAT-240 | Elicitation headline "A tool is asking for some details" | web/src/features/agent/composer/lib/ask-labels.ts:52 | Seed: elicitation without question | — | | | | | | | |

### Composer pickers and menus

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| CHAT-241 | Slash picker opens on leading `/` (above pill, max 360px) | web/src/features/agent/hooks/use-slash-catalog.ts:48 | Perf fixture tab > composer > type `/`. Seed: slash catalogue for the chat's provider | — | | | | | | | |
| CHAT-242 | Slash picker absent when catalogue empty/unavailable or no match (never an empty list) | web/src/features/agent/composer/composer-slash-picker.tsx:105 | Perf fixture tab > type `/` (no provider) | — | | | | | | | |
| CHAT-243 | Groups "Commands", "Runs in the terminal", "Skills" headers | web/src/features/agent/composer/composer-slash-picker.tsx:34 | Seed: catalogue with all three kinds > type `/` | — | | | | | | | |
| CHAT-244 | Command row: name with match underlined, description, "↵" on highlighted row | web/src/features/agent/composer/composer-slash-picker.tsx:58 | Seed > type `/re` | — | | | | | | | |
| CHAT-245 | Skill row: namespace (`ns:`) dimmed, wide name column | web/src/features/agent/composer/composer-slash-picker.tsx:60 | Seed: skill `plugin:skill` > type `/` | — | | | | | | | |
| CHAT-246 | Filter: name-prefix matches first, then label/description/source matches | web/src/features/agent/hooks/use-slash-catalog.ts:128 | Seed > type `/rev` | — | | | | | | | |
| CHAT-247 | Footer "N of M commands", ↑↓ move, ↵ run, esc close | web/src/features/agent/composer/composer-slash-picker.tsx:148 | Seed > type `/` | — | | | | | | | |
| CHAT-248 | Up/Down moves highlight (wraps), highlighted row scrolls into view | web/src/features/agent/hooks/use-slash-catalog.ts:151 | Seed > type `/` > Down ×20 | — | | | | | | | |
| CHAT-249 | Enter / Tab accepts: first line replaced with insert text, picker closes | web/src/features/agent/chat/lib/composer-keys.ts:66 | Seed > `/` > Down > Return | — | | | | | | | |
| CHAT-250 | Click a row accepts (mousedown keeps focus) | web/src/features/agent/composer/composer-slash-picker.tsx:135 | Seed > `/` > click a row | — | | | | | | | |
| CHAT-251 | Esc closes picker, text kept | web/src/features/agent/chat/lib/composer-keys.ts:52 | Seed > `/` > Esc | — | | | | | | | |
| CHAT-252 | Enter with open picker but no highlighted item sends the text (e.g. `/compact`) | web/src/features/agent/chat/lib/composer-keys.ts:74 | Seed > type `/zzz` > Return | — | | | | | | | |
| CHAT-253 | Each command's effect = its text sent as prompt; transcript shows slash row | web/src/features/agent/transcript/rows/user-row.tsx:173 | Seed: provider + catalogue > run a command | — | | | | | | | |
| CHAT-254 | @ file picker: "Files in <repo>" header, rows icon + name (match underlined) + dir + "Folder" tag | web/src/features/agent/composer/composer-slash-picker.tsx:72 | Perf fixture tab > type `@` | — | | | | | | | |
| CHAT-255 | @ picker filters by name substring (max 50) | web/src/features/agent/hooks/use-file-mention.ts:74 | Perf fixture tab > type `@ma` | — | | | | | | | |
| CHAT-256 | @ picker footer "N matches", ↑↓ move, ↵ mention, tab open folder | web/src/features/agent/composer/composer-slash-picker.tsx:150 | Perf fixture tab > type `@` | — | | | | | | | |
| CHAT-257 | Accept a folder: `@dir/` inserted, picker stays open listing it | web/src/features/agent/hooks/use-file-mention.ts:91 | Perf fixture tab > `@` > highlight a folder > Tab | — | | | | | | | |
| CHAT-258 | Accept a file: `@path ` inserted, picker closes, chip-highlighted | web/src/features/agent/hooks/use-file-mention.ts:29 | Perf fixture tab > `@` > Return on a file | — | | | | | | | |
| CHAT-259 | @ picker takes keys before slash picker | web/src/features/agent/chat/agent-chat-view.tsx:270 | Seed catalogue > type `/x @` | — | | | | | | | |
| CHAT-260 | + button (neutral circle, title "Add to this message"), data-open while menu open | web/src/features/agent/composer/composer-plus-button.tsx:161 | Perf fixture tab > hover/click + | — | | | | | | | |
| CHAT-261 | + menu above (end-aligned): "Attach files… Ctrl+U", "Paste image Ctrl+V", "New drawing", divider, "Mention a file @", "Run a command /" | web/src/features/agent/composer/composer-plus-button.tsx:117 | Perf fixture tab > click + | fade+zoom-95+slide-from-bottom-2 100ms (dropdown-menu.tsx:40) | | | | | | | |
| CHAT-262 | + > Mention a file: inserts `@` (space-prefixed) and opens @ picker | web/src/features/agent/composer/plate/chat-markdown-editor.tsx:342 | Perf fixture tab > type `hi` > + > Mention a file | — | | | | | | | |
| CHAT-263 | + > Run a command: `/` line inserted above existing text, picker opens | web/src/features/agent/composer/plate/chat-markdown-editor.tsx:347 | Perf fixture tab > type `hi` > + > Run a command | — | | | | | | | |
| CHAT-264 | + > Paste image with no image on clipboard: info toast "No image on the clipboard" | web/src/features/agent/composer/composer-plus-button.tsx:71 | Perf fixture tab > + > Paste image (toast → MENU/DLG/TST) | — | | | | | | | |
| CHAT-265 | Stash: Ctrl+S with text stashes it and clears the box | web/src/features/agent/chat/use-composer-draft.ts:91 | Perf fixture tab > type `stash me` > ctrl+s | — | | | | | | | |
| CHAT-266 | Ctrl+S on empty box with one stash restores it | web/src/features/agent/chat/use-composer-draft.ts:111 | After CHAT-265 > ctrl+s | — | | | | | | | |
| CHAT-267 | Ctrl+S with several stashes: "Restore a stashed prompt" picker (summary + saved time), first item focused | web/src/features/agent/composer/composer-stash-picker.tsx:33 | Stash 2 drafts > empty box > ctrl+s | — | | | | | | | |
| CHAT-268 | Stash picker: click restores; Esc closes | web/src/features/agent/composer/composer-stash-picker.tsx:38 | As CHAT-267 > Return / Esc | — | | | | | | | |
| CHAT-269 | Stash errors: "This draft is too large to stash.", "Stash or clear the current draft before restoring another one." | web/src/features/agent/chat/use-composer-draft.ts:75 | Open stash picker > type in box > pick an item | — | | | | | | | |

### Attachments

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| CHAT-270 | Attach dialog "Attach files": close X, dropzone "Drop files here or choose from your computer", "Images, PDFs, text and code, up to 25 MB each" | web/src/features/agent/composer/attach-file-modal.tsx:191 | Perf fixture tab > + > Attach files… | dialog scale .98 + opacity 200ms ease-in-out; backdrop 200ms (dialog.tsx:72) | | | | | | | |
| CHAT-271 | Ctrl+U in composer opens attach dialog | web/src/features/agent/composer/agent-composer.tsx:185 | Perf fixture tab > composer > ctrl+u | 200ms | | | | | | | |
| CHAT-272 | "choose from your computer" opens system file picker | web/src/features/agent/composer/attach-file-modal.tsx:265 | As CHAT-270 > click link | — | | | | | | | |
| CHAT-273 | Dropzone drop-target highlight while dragging files over | web/src/features/agent/composer/attach-file-modal.tsx:255 | Drag a file over dropzone (see DND) | — | | | | | | | |
| CHAT-274 | Staged row uploading: name, "12 KB of 1.2 MB · Uploading…", determinate bar | web/src/features/agent/composer/attach-file-list.tsx:28 | As CHAT-270 > choose a file | width 0.15s linear (composer-pickers.css:469) | | | | | | | |
| CHAT-275 | Staged row uploading by host path: indeterminate bar, "Uploading…" | web/src/features/agent/composer/attach-file-list.tsx:34 | Tauri drop onto dropzone | bar translateX 150% 1.1s ease-in-out infinite alternate (composer-pickers.css:464) | | | | | | | |
| CHAT-276 | Staged row done: "1.2 MB · Uploaded" | web/src/features/agent/composer/lib/attach-staged.ts:34 | As CHAT-274, wait | — | | | | | | | |
| CHAT-277 | Too-large file: "30 MB · too large, the limit is 25 MB" (bad status) | web/src/features/agent/composer/lib/use-staged-uploads.ts:26 | As CHAT-270 > choose a >25 MB file | — | | | | | | | |
| CHAT-278 | Upload failed: problem text + "Retry" button | web/src/features/agent/composer/attach-file-list.tsx:22 | Seed: upload endpoint failing | — | | | | | | | |
| CHAT-279 | Remove staged row (X "Remove <name>") aborts its upload | web/src/features/agent/composer/attach-file-list.tsx:40 | As CHAT-274 > click X | — | | | | | | | |
| CHAT-280 | Footer hint "Uploading 1 of 2…"; "Attach N files ↵" disabled until done; "Cancel esc" | web/src/features/agent/composer/attach-file-modal.tsx:99 | As CHAT-270 > choose 2 files | — | | | | | | | |
| CHAT-281 | Attach (click or Enter) inserts the finished files into the box and closes | web/src/features/agent/composer/attach-file-modal.tsx:77 | After upload > Return | — | | | | | | | |
| CHAT-282 | Cancel/Esc/backdrop aborts uploads and closes | web/src/features/agent/composer/attach-file-modal.tsx:91 | As CHAT-280 > Esc | 200ms out | | | | | | | |
| CHAT-283 | File card in box: file icon, name (2-line clamp), size | web/src/features/agent/composer/plate/attachments/chat-attachment-file-card.tsx:82 | After CHAT-281 (non-image file) | — | | | | | | | |
| CHAT-284 | Attachment hover controls: reorder handle (⋮⋮) + trash "Remove this attachment" | web/src/features/agent/composer/plate/attachment-drag-handle.tsx:218 | After CHAT-281 > hover card | opacity 100ms | | | | | | | |
| CHAT-285 | Drag attachment to reorder with drop line (dnd-kit) | web/src/features/agent/composer/plate/attachment-drag-handle.tsx:124 | 2 attachments > drag handle (see DND) | — | | | | | | | |
| CHAT-286 | Backspace at start of the line after an attachment removes it | web/src/features/agent/composer/plate/chat-composer-structure.ts:73 | After CHAT-281 > caret on next line start > BackSpace | — | | | | | | | |
| CHAT-287 | Image attachment in box: zoomable preview (max 320px), hover controls | web/src/features/agent/composer/plate/attachments/chat-markdown-image-node.tsx:65 | Attach a PNG via dialog | 150ms overlay | | | | | | | |
| CHAT-288 | Paste image: local preview at caret immediately, swapped to uploaded image | web/src/features/agent/composer/plate/chat-paste-plugin.ts:62 | Copy a PNG to clipboard > composer > ctrl+v | — | | | | | | | |
| CHAT-289 | Paste image upload fails: preview removed + error toast "Could not attach that image" | web/src/features/agent/composer/plate/chat-paste-plugin.ts:75 | Seed: upload failing > paste image | — | | | | | | | |
| CHAT-290 | Sending while photo still uploading: "Wait for the attached photo to finish uploading." | web/src/features/agent/stores/lib/outbox.ts:55 | Paste a large image > Enter at once | — | | | | | | | |
| CHAT-291 | Paste long text (>400 chars or >6 lines) → "Pasted text" pill with "N lines" | web/src/features/agent/composer/plate/chat-paste-plugin.ts:100 | Copy 10 lines > composer > ctrl+v | — | | | | | | | |
| CHAT-292 | Pasted-text pill click → raw text dialog | web/src/features/agent/composer/plate/attachments/text-attachment-pill.tsx:33 | After CHAT-291 > click pill | dialog 200ms | | | | | | | |
| CHAT-293 | Paste >32 KB text → uploaded as pasted.txt file card | web/src/features/agent/composer/plate/chat-paste-plugin.ts:105 | Copy 40 KB text > ctrl+v | — | | | | | | | |
| CHAT-294 | Shift+paste or paste inside code block stays plain text | web/src/features/agent/composer/plate/chat-paste-plugin.ts:53 | Copy 10 lines > ctrl+shift+v | — | | | | | | | |
| CHAT-295 | Paste file copied in file manager → file card | web/src/features/agent/composer/plate/chat-paste-plugin.ts:86 | Copy a file in a file manager > ctrl+v | — | | | | | | | |
| CHAT-296 | Drag files over pill: drop-target ring | web/src/features/agent/composer/agent-composer.tsx:157 | Drag file over composer (see DND) | — | | | | | | | |
| CHAT-297 | Drop files on pill/document: each uploads and inserts | web/src/features/agent/composer/agent-composer.tsx:146 | Drop file on composer (see DND) | — | | | | | | | |
| CHAT-298 | Document drop-target as inset ring | web/src/features/agent/styles/composer-document.css:19 | main tab > drag file over document | — | | | | | | | |
| CHAT-299 | + > New drawing: Excalidraw takeover covering the chat pane, header "Excalidraw", "Attach" (disabled until canvas ready), close X | web/src/features/agent/composer/excalidraw-takeover.tsx:86 | Perf fixture tab > + > New drawing | — | | | | | | | |
| CHAT-300 | Excalidraw Attach → "Attaching…", inserts preview + PNG, closes; failure toast "Couldn't save this drawing" | web/src/features/agent/composer/excalidraw-takeover.tsx:93 | Draw a box > Attach | — | | | | | | | |
| CHAT-301 | New drawing reopens the chat's last saved design | web/src/features/agent/composer/agent-composer.tsx:191 | After CHAT-300 > + > New drawing | — | | | | | | | |
| CHAT-302 | Excalidraw preview in box with "Edit diagram" pencil → takeover with that scene | web/src/features/agent/composer/plate/attachments/excalidraw-preview.tsx:133 | After CHAT-300 > click pencil | — | | | | | | | |

### Trays

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| CHAT-303 | Top tray absent when nothing to show | web/src/features/agent/trays/top-tray.tsx:26 | Perf fixture tab | — | | | | | | | |
| CHAT-304 | Turn failure notice auth (warn): "Signed out of X" / "Sign in again to continue." + "Sign in" (opens terminal) | web/src/features/agent/trays/turn-failure-tray.tsx:112 | Seed: newest marker turn_failed kind=auth | — | | | | | | | |
| CHAT-305 | Limit (plain tone): "Usage limit reached" / "Resets at 7:00 PM, in 2h 14m. Another model can keep going now." + "Switch model" | web/src/features/agent/trays/turn-failure-tray.tsx:102 | Seed: turn_failed kind=limit + rate-limit resetsAt + model catalogue | — | | | | | | | |
| CHAT-306 | Switch model opens footer model picker; model chip lit (data-hot) | web/src/features/agent/trays/use-model-hot.ts:6 | As CHAT-305 > click Switch model | dropdown 120ms | | | | | | | |
| CHAT-307 | Overloaded (warn): "X is busy right now" / "Try again in a moment. Nothing was lost." + Retry | web/src/features/agent/transcript/lib/turn-failure.ts:75 | Seed: turn_failed kind=overloaded | — | | | | | | | |
| CHAT-308 | Other (danger): "X had a problem on its side" / "Try again. Nothing was lost." + Retry | web/src/features/agent/transcript/lib/turn-failure.ts:82 | Seed: turn_failed no kind | — | | | | | | | |
| CHAT-309 | Retry disables after one click | web/src/features/agent/trays/turn-failure-tray.tsx:54 | As CHAT-308 > click Retry | — | | | | | | | |
| CHAT-310 | Session lost (danger): "The session was lost" / "Start a new conversation to keep going." + "New conversation" | web/src/features/agent/transcript/lib/marker-failure.ts:79 | Seed: session_lost marker newest | — | | | | | | | |
| CHAT-311 | New conversation failure toast (spawn failure) | web/src/features/agent/chat/agent-chat-view.tsx:218 | As CHAT-310 > click (no provider) (toast → MENU/DLG/TST) | — | | | | | | | |
| CHAT-312 | Could not start (danger): "X could not start" + detail, no button | web/src/features/agent/transcript/lib/marker-failure.ts:87 | Seed: runner_exited spawn_failed newest | — | | | | | | | |
| CHAT-313 | Cut short (warn): "The turn was cut short" / "What was said is kept. Try again to carry on." + Retry | web/src/features/agent/transcript/lib/marker-failure.ts:98 | Seed: turn_interrupted not by user | — | | | | | | | |
| CHAT-314 | Failure notice gone once a newer prompt is sent/queued | web/src/features/agent/trays/turn-failure-tray.tsx:20 | As CHAT-308 > send a prompt | — | | | | | | | |
| CHAT-315 | Send failure (danger): "Your message was not sent" / "Check the connection and try again." + Edit, Discard, Retry | web/src/features/agent/trays/send-failure-tray.tsx:21 | Seed: outbox item failed (network) | — | | | | | | | |
| CHAT-316 | Final send failures (Edit + Discard only): "The session was lost", "This agent can't take typed messages here", "Your message is too large to send" | web/src/features/agent/trays/lib/send-failure.ts:13 | Seed: failed outbox items codes session_lost / prompt_submit_unsupported / too_large | — | | | | | | | |
| CHAT-317 | Send failure Edit puts text back; Discard drops it | web/src/features/agent/trays/send-failure-tray.tsx:32 | As CHAT-315 > click Edit / Discard | — | | | | | | | |
| CHAT-318 | To-do tray collapsed: pips (≤8, scaled), "2 of 5 done", current step text, caret down | web/src/features/agent/trays/plan-tray.tsx:21 | Seed: plan with unfinished steps | — | | | | | | | |
| CHAT-319 | To-do tray open: every step, spinner on active, caret up | web/src/features/agent/trays/plan-tray.tsx:38 | As CHAT-318 > click | FlickerSpinner | | | | | | | |
| CHAT-320 | To-do tray disappears when all steps done | web/src/features/agent/trays/plan-tray.tsx:15 | Seed: plan all done | — | | | | | | | |
| CHAT-321 | Queued tray single: "Queued" label + text + pencil Edit + X Remove | web/src/features/agent/trays/queued-tray.tsx:28 | Seed: running turn > type > Enter | — | | | | | | | |
| CHAT-322 | Queued tray several: "N queued" header, rows | web/src/features/agent/trays/queued-tray.tsx:20 | Seed: running turn > queue 3 | — | | | | | | | |
| CHAT-323 | Queued Edit: removed from queue, text into composer; Remove: dropped | web/src/features/agent/trays/queued-tray.tsx:35 | As CHAT-321 > click pencil / X | — | | | | | | | |
| CHAT-324 | Resolved tray: "N requests answered" (or "closed") + "2 allowed · 1 denied" + Show/Hide caret | web/src/features/agent/trays/resolved-tray.tsx:58 | Seed: decided asks after newest message | — | | | | | | | |
| CHAT-325 | Resolved tray open: rows verdict (Allowed/Denied/Timed out/Not answered/Allowed in terminal), what, HH:MM | web/src/features/agent/trays/resolved-tray.tsx:76 | As CHAT-324 > click Show | — | | | | | | | |
| CHAT-326 | Resolved tray hidden once a new prompt is in the outbox | web/src/features/agent/trays/use-top-tray.ts:34 | As CHAT-324 > type and send | — | | | | | | | |
| CHAT-327 | Bottom tray subagent row: spinner, name, action, live clock | web/src/features/agent/trays/tray-row.tsx:28 | Seed: running subagent | FlickerSpinner; clock 1s | | | | | | | |
| CHAT-328 | Bottom tray shell row running: command/label, last output line, clock, X "End this shell" | web/src/features/agent/trays/tray-row.tsx:36 | Seed: running background shell, provider with shellStop | — | | | | | | | |
| CHAT-329 | Shell row stopping: "Stopping…", no X | web/src/features/agent/trays/lib/shell-rows.ts:32 | As CHAT-328 > click X | — | | | | | | | |
| CHAT-330 | Ended shell row: "Exit 0 · summary" / "Stopped" / "Failed" / "Finished" + ran-for clock | web/src/features/agent/trays/lib/shell-rows.ts:6 | Seed: ended shells each way | — | | | | | | | |
| CHAT-331 | Monitor row ("Monitor", "End this monitor") | web/src/features/agent/trays/tray-row.tsx:22 | Seed: running monitor | — | | | | | | | |
| CHAT-332 | Unlabelled shell detail "opened by the agent, still managed" | web/src/features/agent/trays/lib/shell-rows.ts:30 | Seed: shell with no label/tail | — | | | | | | | |
| CHAT-333 | Row tooltip: command, cwd, process, timeout/"Keeps watching", summary, tail | web/src/features/agent/trays/lib/shell-rows.ts:12 | As CHAT-328 > hover row | native title | | | | | | | |
| CHAT-334 | Mixed kinds: "Subagents"/"Shells"/"Monitors" group labels | web/src/features/agent/trays/lib/tray-rows.ts:50 | Seed: subagent + shell running | — | | | | | | | |
| CHAT-335 | >4 rows: 3 shown + "N more · K running" row; expand; "Show fewer" | web/src/features/agent/trays/bottom-tray.tsx:24 | Seed: 6 shells > click more / Show fewer | — | | | | | | | |

### Footer controls

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| CHAT-336 | Footer underbar: selection cluster left, context pill right | web/src/features/agent/controls/provider-bar.tsx:49 | Perf fixture tab | — | | | | | | | |
| CHAT-337 | Context pill unknown: muted empty ring + "context", title "Context usage not reported yet" (span) | web/src/features/agent/controls/context-gauge.tsx:35 | Perf fixture tab > hover pill | — | | | | | | | |
| CHAT-338 | Context pill known: conic ring in primary to N% + bold "N%" + "context" | web/src/features/agent/controls/context-gauge.tsx:38 | Seed: telemetry context usedPercent 40 | — | | | | | | | |
| CHAT-339 | Context pill ≥85%: ring warning colour | web/src/features/agent/controls/context-gauge.tsx:45 | Seed: usedPercent 90 | — | | | | | | | |
| CHAT-340 | Context pill while compacting: warning ring | web/src/features/agent/controls/context-gauge.tsx:45 | Seed: compacting | — | | | | | | | |
| CHAT-341 | Context tooltip: "N of M tokens · 5h: 40% · $0.0123" (tokens, rate limits, cost) | web/src/features/agent/controls/context-gauge.tsx:95 | Seed: telemetry with tokens, rateLimits, cost > hover | native title | | | | | | | |
| CHAT-342 | Compact offered (button, hover accent): unknown usage → click compacts at once, title "Compact the conversation" | web/src/features/agent/controls/context-gauge.tsx:58 | Seed: idle live chat on provider with compaction | — | | | | | | | |
| CHAT-343 | Compact offered with usage: click opens menu "Compact conversation" | web/src/features/agent/controls/context-gauge.tsx:82 | Seed as CHAT-342 + usage > click pill | opacity/scale .98/y 4px 120ms easeOut (menu-popover.tsx:80) | | | | | | | |
| CHAT-344 | Compact refused: report toast "Couldn't compact the chat" | web/src/features/agent/chat/lib/offered-compact.ts:9 | Seed: compact endpoint refusing (toast → MENU/DLG/TST) | — | | | | | | | |
| CHAT-345 | Model chip absent when no connected provider has a model catalogue | web/src/features/agent/controls/agent-selection-picker.tsx:405 | Perf fixture tab (no provider) | — | | | | | | | |
| CHAT-346 | Model chip: provider icon, bold model (or reported model, or "Model"), effort label, up-down icon; title "X — provider, model and effort." | web/src/features/agent/controls/agent-selection-picker.tsx:411 | Seed: connected provider with models | — | | | | | | | |
| CHAT-347 | Model chip hover accent; disabled during turn/switch | web/src/features/agent/styles/composer-footer.css:73 | Seed as CHAT-346; running turn | — | | | | | | | |
| CHAT-348 | Model menu (280px, above chip): search "Search models…" focused | web/src/features/agent/controls/agent-selection-picker.tsx:441 | Seed > click model chip | 120ms easeOut popover | | | | | | | |
| CHAT-349 | Model menu sections per provider (icon + name), rows with check on picked | web/src/features/agent/controls/agent-selection-picker.tsx:91 | Seed: 2 providers with models > open | row bg transition-colors 150ms | | | | | | | |
| CHAT-350 | Model search fuzzy filter; provider name match shows all its models | web/src/features/agent/controls/agent-selection-picker.tsx:331 | Seed > open > type `hgh` | — | | | | | | | |
| CHAT-351 | Model search no match: "No matches for "q"" | web/src/features/agent/controls/agent-selection-picker.tsx:80 | Seed > open > type `zzzz` | — | | | | | | | |
| CHAT-352 | Model menu keys: Down/Up highlight, Enter picks, Esc closes; hover highlights | web/src/features/agent/controls/agent-selection-picker.tsx:385 | Seed > open > Down > Return | — | | | | | | | |
| CHAT-353 | Picking a model stages it (chip updates, nothing sent); provider of section staged too | web/src/features/agent/controls/agent-selection-picker.tsx:371 | Seed > open > pick other model | — | | | | | | | |
| CHAT-354 | Effort slider: "Effort" + value ("High" or "Medium (default)"), track with fill + thumb, level buttons | web/src/features/agent/controls/agent-selection-picker.tsx:194 | Seed: model with effort levels > open | fill width / thumb left transition 150ms (Tailwind default) | | | | | | | |
| CHAT-355 | Effort slider drag / click track snaps to level; arrow keys step | web/src/features/agent/controls/agent-selection-picker.tsx:168 | As CHAT-354 > drag thumb / Right | — | | | | | | | |
| CHAT-356 | Effort unknown: no thumb/fill drawn | web/src/features/agent/controls/agent-selection-picker.tsx:223 | Seed: effort not in model's levels | — | | | | | | | |
| CHAT-357 | Effort section absent for model without levels | web/src/features/agent/controls/agent-selection-picker.tsx:192 | Seed: model with no efforts | — | | | | | | | |
| CHAT-358 | Permission chip: shield + level label + up-down; title "What the agent may do without asking"; absent without levels | web/src/features/agent/composer/permission-level-switcher.tsx:106 | Seed: provider declaring permission levels | — | | | | | | | |
| CHAT-359 | Permission menu (332px): Guarded / Trusted / Full Auto rows with description, check on current, risk style on Full Auto | web/src/features/agent/composer/permission-level-switcher.tsx:121 | As CHAT-358 > click chip | 120ms easeOut popover | | | | | | | |
| CHAT-360 | Pick level: shown at once; write failure rolls back + toast "Could not set permission level" | web/src/features/agent/composer/permission-level-switcher.tsx:69 | As CHAT-359 > pick Trusted (daemon refusing) | — | | | | | | | |
| CHAT-361 | Inherited level (host default, nearest offered) shown when chat pins none | web/src/features/agent/composer/permission-level-switcher.tsx:91 | Seed: chat without pinned level | — | | | | | | | |
| CHAT-362 | Blank document carries the same selection cluster in its handle | web/src/features/agent/chat/agent-chat-view.tsx:453 | main tab. Seed: provider with models | — | | | | | | | |

### View switch and terminal-chat variant

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| CHAT-363 | Chat/Terminal segmented switch in pane header slot (chat icon, terminal icon) | web/src/features/agent/controls/chat-view-switch.tsx:36 | Perf fixture tab, header right (header → VT/IDE/PANE) | — | | | | | | | |
| CHAT-364 | Switch fallback position top-right of chat when no header slot | web/src/features/agent/controls/chat-view-switch.tsx:37 | Chat pane without overlay header | — | | | | | | | |
| CHAT-365 | Switch hidden when provider has no terminal | web/src/features/agent/controls/chat-view-switch.tsx:27 | Seed: provider hasTerminal=false | — | | | | | | | |
| CHAT-366 | Click Terminal segment: indicator slides, terminal surface shown | web/src/features/agent/controls/view-switch.tsx:44 | Perf fixture tab > click terminal segment | indicator width/translate 200ms ease-in-out (segmented-control.tsx:33) | | | | | | | |
| CHAT-367 | mod+/ toggles chat ↔ terminal | web/src/features/agent/components/agent-chat-pane.tsx:266 | Perf fixture tab > `rig key ctrl+slash` ×2 | 200ms indicator | | | | | | | |
| CHAT-368 | Arrow keys move between segments (activate on focus) | web/src/features/agent/controls/view-switch.tsx:36 | Focus switch > Right | — | | | | | | | |
| CHAT-369 | Segment disabled while non-hotswap provider has a live turn | web/src/features/agent/controls/view-switch.tsx:26 | Seed: non-hotswap provider, running turn | — | | | | | | | |
| CHAT-370 | Split segment (columns icon) when split enabled + hotswap | web/src/features/agent/controls/view-switch.tsx:56 | Seed: split setting on + hotswap provider | — | | | | | | | |
| CHAT-371 | Split view: chat and terminal side-by-side (45/55) with sash; stacked below 780px wide | web/src/features/agent/components/agent-chat-pane.tsx:387 | Seed as CHAT-370 > Split; `rig size 900x800` | — | | | | | | | |
| CHAT-372 | Terminal surface idle: "Start session" button | web/src/features/agent/terminal/agent-terminal-surface.tsx:173 | Perf fixture tab > terminal segment (no runner) | — | | | | | | | |
| CHAT-373 | Terminal surface offline: "Offline — a session can start once the host is back." + disabled Start session | web/src/features/agent/terminal/agent-terminal-surface.tsx:175 | Seed: host offline > terminal | — | | | | | | | |
| CHAT-374 | Terminal surface reviving: large spinner + message | web/src/features/agent/terminal/agent-terminal-surface.tsx:166 | Seed: reviving runner > terminal | FlickerSpinner | | | | | | | |
| CHAT-375 | Terminal surface attached with no PTY: "This agent has no terminal view attached right now." | web/src/features/agent/terminal/agent-terminal-surface.tsx:157 | Seed: live runner without session | — | | | | | | | |
| CHAT-376 | Terminal surface attached: xterm grid of the CLI (attach-only) | web/src/features/agent/terminal/agent-terminal-surface.tsx:134 | Seed: live runner with PTY > terminal | — | | | | | | | |
| CHAT-377 | Start session click (no provider installed): spawn failure toast | web/src/features/agent/terminal/agent-terminal-surface.tsx:186 | As CHAT-372 > click Start session (toast → MENU/DLG/TST) | — | | | | | | | |
| CHAT-378 | Terminal header: view switch + provider switch dropdown (provider icon, name, caret) | web/src/features/agent/terminal/agent-terminal-surface.tsx:93 | Perf fixture tab > terminal | — | | | | | | | |
| CHAT-379 | Provider switch menu: other enabled providers with icons | web/src/features/agent/components/provider-switch-dropdown.tsx:77 | Seed: 2 enabled providers > terminal > click dropdown | 120ms easeOut popover | | | | | | | |
| CHAT-380 | Provider switch menu empty: "No other provider is enabled. Turn one on in Settings to switch mid-chat — this conversation's context comes with you." | web/src/features/agent/components/provider-switch-dropdown.tsx:88 | Perf fixture tab > terminal > click dropdown | 120ms | | | | | | | |
| CHAT-381 | Provider switch disabled: title "Wait for prompt delivery before switching providers" | web/src/features/agent/components/provider-switch-dropdown.tsx:62 | Seed: running turn > terminal > hover | — | | | | | | | |
| CHAT-382 | Terminal: "N prompts pending in Chat" bar + "Return to Chat" + trash "Cancel unsent prompts" | web/src/features/agent/components/agent-chat-pane.tsx:434 | Seed: queued prompts > terminal | — | | | | | | | |
| CHAT-383 | Return-to-chat notice "The agent is no longer waiting on you here." + Return to Chat + Dismiss | web/src/features/agent/components/agent-terminal-wait-banner.tsx:66 | Seed: follow wait banner to terminal, wait clears | — | | | | | | | |
| CHAT-384 | Terminal-only view caps width (max-w-4xl, 16px gutters); chat drops the cap | web/src/features/agent/components/agent-chat-pane.tsx:321 | Perf fixture tab > terminal at standard size | — | | | | | | | |
| CHAT-385 | Click empty space around terminal focuses it | web/src/features/agent/components/agent-chat-pane.tsx:286 | Seed: attached terminal > click margin | — | | | | | | | |
| CHAT-386 | Chat zoom applies to chat surface only | web/src/features/agent/components/agent-chat-pane.tsx:344 | Perf fixture tab > ctrl+plus | — | | | | | | | |

Skipped:
- `web/src/features/agent/controls/native-model-picker.tsx` — `hasNativeChrome()` is `IS_MOBILE && isTauri()`: mobile only.
- `web/src/features/agent/composer/native-plus-menu.tsx`, `native-attach-sheet.tsx` — native mobile sheets only.
- `web/src/features/agent/transcript/jump-to-latest.tsx:18` NativeJumpToLatest — mobile native chrome only.
- `web/src/features/agent/composer/composer-plus-button.tsx:99` "Photo library" / "Take a photo" items and hidden inputs — IS_MOBILE only.
- `web/src/features/agent/composer/attach-file-modal.tsx:199` attach sheet with Photos/Camera/Files buttons — IS_MOBILE only.
- `web/src/features/agent/controls/agent-selection-picker.tsx:257` PermissionRadios inside model menu — IS_MOBILE only (desktop uses chip).
- `web/src/features/agent/chat/agent-chat-view.tsx:376` footer/handle ViewSwitch — IS_MOBILE only.
- `web/src/features/agent/trays/bottom-tray.tsx:21` grouped summary row ("3 subagents · 2 shells") — IS_MOBILE only.
- `web/src/features/agent/terminal/agent-terminal-surface.tsx:150` TerminalKeyBar — IS_MOBILE only.
- `web/src/features/agent/transcript/agent-transcript.tsx:330` no prompt pin on phone — mobile branch.
- `web/src/features/agent/components/descriptor-status.tsx` — rendered in Settings (see SET).
- `web/src/features/agent/hooks/use-kill-agent-menu-items.tsx` — sidebar row context menu items (see MENU).
- `web/src/features/agent/components/agent-chat-pane.tsx:257` AGENT_CYCLE_PROVIDER chord — ships unbound (registry.ts defaultChord '').
- `web/src/features/agent/chat/use-chat-open-mark.ts`, `markStart/markEnd` in markdown-message/agent-transcript — perf marks, nothing visible.
