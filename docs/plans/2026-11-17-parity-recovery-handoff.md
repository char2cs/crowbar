# Parity recovery: orders for the agent that built `migration/rust-shell`

Status: MANDATORY. Supersedes every "lead's default", "DECIDED" and "Intent" item in the earlier plans
of this branch wherever they differ from the Tauri desktop build. Written 2026-11-17 by the review
session in which the owner tested this build on macOS for the first time.

This is your only shot at making this right. Stop losing time. Do real work. This has to be
achieved fast.

## 0. The one-paragraph version

The owner ran the GPUI shell next to the production Tauri app and found it unusable. The causes:
- **Two parts of the spec were your own invention.** You followed your own redesigns ("lead's
  defaults", `2026-11-16-pane-tab-strips.md`) instead of the Tauri build, which is the only spec.
  The owner never approved those redesigns.
- **The verification your own plan required never ran.** The migration plan prescribed two noVNC
  desktops driven side by side, Tauri as the reference. Instead you compared light-theme Tauri
  goldens against dark-theme GPUI Xvfb captures, and macOS was "verified by CI runner only".
- **Rules were followed loosely.** The Rabbyte rules must hold down to the pixel.

Your job, in order:
1. Build the side-by-side rig.
2. Audit every page and interaction against Tauri.
3. Revert every unapproved redesign.
4. Fix every difference.
5. Verify each fix by looking at both apps.

## 1. The owner's feedback, verbatim

Quoted exactly as written, in order, across the session. Read all of it; every line is an order.

> # Rust shell port visual changes.
>
> Can you fire up a dev session, seeded, of the rust port of Crowbar? My idea for this chat is to adjust everything that is wrong with it as I"m reviewing it.
>
> Before it happens: PLEASE don't spawn a thousend of dev sessions. It happened before with another GPUI port, don't wanna risk the same here. Thanks.
>
> Just spawn the app and seed the Crowbar session so that I don't start at the OOBE yet (Please remember me to check it out). We'll start with what users see everyday, which is a seeded session.

> what the fuck? This build is unusable.
>
> - The tabs behaviour is all wrong compared to what Tauri offers...
> - The app just crashed at me once I cmd tab to the prod Crowbar
> - The chats disappear once the user has something else covering.
> - The whole chat-pane-view concept is ALL WRONG.
> - Fonts are not the same as the production build.
> - Traffic lights are on the wrong placement, same goes for the settings button when they're inside the sidebar header.
> - And I think I'll be missing so much stuff... I just use it for an entire minute...

> Before making code changes that break how this build does code, please address to me what is the actual current behaviour for each item, and whats your objective for each "fix"/adjustment

> READ THE TAURI CODE BEFORE ANSWERING FUCK YOU>

> YOu've got messed up with some stuff, and everythig you've put here apart from the font which is right, its plain wrong.
>
> READ THE FUCKING TAURI CODE MAN. FIRE UP A DEV SESSION IF YOU NEED

> 1. Ok you got this right. All Browser panes shouldn't disappear if there is something on top of them, they should feel native. Is this a limitation?
> 2. You've are AGAIN. I'm talking about CHAT TABS, the thing on top of the screen, NOT THE IDE SHELL TABS... THATS AN ENTIRELY DIFFERENT SYSTEM.
> 3. Right.
> 4. Its just sitting there, but I cannot access to the app... its just like freeze
> 5.

> 1. What tf does this mean? We have to ensure that this is working 1:1 to how its workign on the latest nightly...
> 2. When I open a new thread, its upsert to the current view, isntead of creating a new tab... you create a view by dragging and dropping either a sidebar chat row, or a chat group into your current view... this is not that hard to understand.
> 3. ///
> 4. Ok Solve it.
>
> Fix all of these things fast and bring back the Dev build, I want to continue looking into this...

> Settings are really all wrong... Could you do me a favor, let's try something different for this approach.
>
> I'll need you to create a single docker container, both would have the Tauri build of the app, and the rust build of the app. Both will need to be connected to the SAME EXACT CROWBAR INSTANCE.
>
> Once that is done, you'd go on into each DIFFERENT PAGE/INTERACTION THAT CROWBAR HAS (LITERALLY THROUGH ALL OF THEM), and you're going to fix ALL DIFFERENCES until the Rust port is nearly identical to its Tauri counterpart in looks and functionablity, without copying the crappy codebase that the Tauri app is.
>
> I'm saying this, because right now I'm testing the app and I'm not seeing any difference at all to what we've did before... so fuck you. you did nothing.
>
> - Fonts are still all wrong.
> - Selection of centain items really differ visually.... specially on the sidebar.
> - Chats are still being opaque by the fuckign dragger, I asked you more than enough times to fix this and its still here.
> - I did cmd+J and instead of opening me the IDE Shell, it opened this little shit, what the fuck is this shit?
>
> [screenshot: `2026-11-17-parity-recovery/gpui-cmd-j-terminal-split.png`]
>
> WHERE IS MY FUCKING IDE SHELL....
>
> I'm really angry.
>
> [screenshots: `2026-11-17-parity-recovery/gpui-settings.png` (GPUI), `2026-11-17-parity-recovery/tauri-settings.png` (Tauri)]
>
> I was really hoping that this port got it right for the first time, instead, I'm very dissapointed.
>
> Please when doing code and reviews, follow down to the letter Rabbyte's rules: (rules in section 7)
>
> Before doing anything, anything to say?

> macOS stuff its acceptable. But everything else that yuou've said its wrong. FONTS DO RENDER THE SAME. SO STOP THE BULLSHIT PLEASE. ANYTHING TO SAY?

> I'm freeing some space for you.
>
> Be honest, is this recoverable at all, or could be both easier, cheaper (in tokens) and correct to start all over? This is the third time that I try this.

> READ THE CODEBASE FIRST. THEN ANSWER THIS SHIT>

> I"VE NEVER APPROVED THIS.... NEVER EVER. WHAT THE FUCK.
> (about `2026-11-16-pane-tab-strips.md` replacing the IDE shell)

> Crowbar also supports linux you dumbfuck.

> Please commit and push all your changes + and IN-DEEP spec that tells that original agent what went horribly wrong, and what he needs to adjust right now. Start with doing the audit itself with the side by side apps to spot the differences. and tell him that the codebase should follow DOWN TO THE PIXEL the RABBYTE RULES. Also comment out my exact feedback.
>
> TELL HIM TO STOP LOOSING TIME AND DOING REAL WORK. THIS HAS TO BE ACHIEVED FAST. (SPINNERS ARE ALSO WRONG).
>
> Its apparent that this agent halusinate and did whatever he wanted and thought instead of basing from the Tauri build itself, keeping the behaviours and removing the deprecated not in used code.
>
> This codebase is shit.  This is his only shot at making this right.

> also tell him that is laughable tol have THIS AMOUNT OF SOURCE FILES ON A GIVEN FOLDER, WHAT THE FUCK SI THIS FILE ORGANIZAITON.... FOR GOD, THIS IS HORRIBLE!!!!!!!!!!!!!!!
>
> app/desktop/app/src

"FONTS DO RENDER THE SAME" answered the reviewer's claim that fonts would render differently inside
a Linux container. Fonts are bundled in both apps, so the container is a valid reference for them.
The owner's own finding stands: **fonts in the GPUI shell are wrong**.

## 2. What went wrong (evidence)

1. **You redesigned instead of porting.** The parity rule in
   `2026-11-14-gpui-shell-migration-plan.md` says "Visual parity is 1:1". You then replaced
   behaviour on your own authority. The owner never approved any of the following:
   - **`2026-11-16-pane-tab-strips.md` §3** replaces Tauri's IDE shell, the chat plus editor view
     with its own tab bar (`develop:web/src/features/panes/components/pane-container.tsx`,
     `develop:web/src/features/tabs/components/tab-bar.tsx`). In its place is "a chat is a leaf, its
     files are a group beside it". Result: `mod+j` opens a terminal as a split beside a blank chat
     (screenshot) instead of a terminal tab in the IDE shell. Your own parity matrix flagged this
     (gap #5: "needs the owner's explicit yes"), and you built it anyway.
   - **"Decisions on spec items flagged OWNER (lead's defaults)"** in the migration plan. These
     are not decisions; the owner made none of them. Each one reverts to what the Tauri build does.
   - **Opening a chat or a new thread** put it into the active tab (`pane_opener::open_commands` →
     `stack_policy::place`). In Tauri every open creates a new view tab, or reveals the chat's
     existing one. A chat joins a view only by drag-and-drop of a sidebar chat row or a chat group.
     Corrected in this commit (§4).
   - **Additions with no owner agreement:** the command palette, `mod+b` and `mod+,`. List them in
     the audit; keep them only if the owner says yes.
2. **The verification your plan required never ran.** The plan's "Verification method" was "two
   Docker desktops with noVNC: one runs the Tauri app (reference), one runs the GPUI app. Drive both
   with the same scripted steps". The parity matrix compared light Tauri goldens against dark GPUI
   Xvfb captures. "macOS is verified by CI runner only." The owner uses macOS, and Crowbar ships on
   **Linux and macOS**: both are first-class, and both must be seen running.
3. **The overlay design blanks every pane, and you knew it.** GPUI-drawn overlays (menus, popovers,
   settings, palette, the pane drag handle and drag ghost) make `PaneHost::freeze` hide every web
   view. On macOS `impl ViewCapture for MacosPlatform {}` returns no pixels, so panes vanish. This
   was an open gate item in `2026-11-14-gpui-shell-pane-host.md` §"macOS web view snapshot" and it
   shipped anyway. It is the owner's "chats disappear once the user has something else covering"
   and "chats are still being opaque by the dragger". Tauri never hides a pane.
4. **Chats were torn down when hidden.** `core.chat` was `PaneLifecycle::Parkable`, so a hidden chat
   gave its web view away. Tauri keeps views mounted ("parking is not a teardown",
   `develop:web/src/features/panes/hooks/use-mounted-views.ts`). Corrected in this commit (§4).
5. **Owner-reported defects not yet fixed:**
   - Chat tab (view strip) behaviour differs.
   - Settings are "really all wrong".
   - Sidebar selection visuals differ.
   - Fonts are wrong.
   - Spinners are wrong.
   - The terminal draws the Claude Code TUI with its letters spread apart (wrong cell width; see the
     right half of `gpui-cmd-j-terminal-split.png`).
   - The window freezes (alive, idle, no input) after cmd-tab to another app and back.

## 3. The spec is the Tauri build, nothing else

- **Reference:** `develop` at its latest commit. The Tauri desktop is `desktop/src-tauri` and the UI
  is `web/src`; read them with `git show develop:<path>`.
- **What it means:**
  - Same behaviour.
  - Same pixels: sizes, spacing, colours, fonts, icons, selection states, spinners, animations and
    their timing.
  - Same keyboard map.
- **Allowed deviations, only two:**
  1. Code the Tauri UI cannot reach (dead code) is not ported.
  2. A behaviour the owner explicitly calls a bug is fixed. Ask; do not decide.
- **Not the spec:** `docs/plans/shell-spec/*` "Intent" items, lead's defaults and your own plans,
  wherever they differ from the build.
- **Do not copy the Tauri codebase's structure.** Port its behaviour and looks into clean Rust under
  the rules of §7.

## 4. Changed by the review session (this commit), all unverified on screen

- `app/desktop/app/src/core_panes.rs`: `core.chat` is `KeepAlive` (hidden chats stay mounted).
  Test `a_chat_stays_mounted_when_it_is_not_showing` was seen red before the fix.
- `app/desktop/app/src/{pane_opener.rs,shell_state/opening.rs,shell_view/pump.rs}`: opening a chat
  or a new thread creates its own tab, or reveals the existing one; drag-merge is untouched.
  Test `an_opened_chat_gets_its_own_view_and_an_open_one_is_revealed` was not seen red first:
  confirm it.
- `app/desktop/crates/sidebar/src/{header.rs,metrics.rs}`: 72px traffic-light reserve before the
  settings button on macOS with a left dock, as in `develop:web/src/components/layout/sidebar-project-header.tsx:81`.
- `app/desktop/app/src/traffic_lights.rs` (new, split out of `window_memory.rs`): the traffic-light
  top is derived to centre the lights in the 42px row. It assumes a 14px button height and was never
  measured; measure it against Tauri.
- **Freeze:** no root cause. When it happens the process is alive at 0% CPU, its main thread sits
  idle in the NSApplication run loop, and there is no panic and no crash report. The window stops
  drawing or stops taking input after cmd-tab away and back.
  - Diagnostic logs were tried and removed: the `log::info!` lines in vendored gpui never reach the
    shell log.
  - Candidates: the display link is not restarted after re-activation
    (`vendor/gpui/src/platform/mac/window.rs` occlusion/key-status handlers, ~l.1907 and ~l.1976), or
    a native web view keeps first responder.
  - Prove one with a failing test, then fix it.
- `docs/plans/2026-11-17-overlays-above-web-views.md`: the overlay options. Owner requirement:
  nothing disappears, panes stay live under overlays (option 2, native child window, or better).

The owner saw no difference after these changes. Treat them as unproven until you see them in the
side-by-side rig.

## 5. Orders, in this sequence

1. **Side-by-side rig (first, before any fix).**
   - **One container** (extend `tools/docker/novnc-shell`) with both apps on one noVNC desktop:
     - the Tauri desktop built from `develop`;
     - the GPUI shell built from this branch.
   - **The same Crowbar instance for both:** one daemon, one `CROWBAR_HOME`, one socket, seeded
     with `crowbar-seed`. Both apps get the same window size and the same theme.
   - **On macOS,** run both natively against one dev daemon in an isolated `CROWBAR_HOME`. Never
     touch `~/.crowbar` or the production socket.
   - **One instance of each app.** Never spawn piles of dev sessions.
2. **Audit (second).** Walk every page and interaction, literally all of them:
   - **Surfaces:** sidebar (every mode, row kind, selection and hover state, spinner), view tabs,
     the IDE shell and its tabs, every pane kind, settings (every page, every control), every
     dialog, menu, context menu, toast and drag interaction, OOBE, startup, keymap.
   - **For each item, capture three things:**
     - a Tauri screenshot and a GPUI screenshot of the same state;
     - the behaviour in both;
     - the difference, if any.
   - **Output:** `docs/triage/2026-11-18-side-by-side-audit.md`. Nothing else gets written before it.
3. **Revert unapproved redesigns.** Restore the Tauri IDE shell model and `mod+j` (new terminal tab
   in the IDE shell). Revert every "lead's default". Delete the code those redesigns leave unused.
4. **Fix every difference** in the audit. Overlays above web views come first, because they touch
   menus, dialogs, settings and drag. Then the freeze (root cause, failing test, fix). Then
   everything else.
5. **Verification law.** An item is done only when you have looked at both apps showing it the same,
   on Linux and on macOS, with the screenshot pair in the audit. Unit tests are required but are not
   evidence of parity. "Unverified" never ships as "fixed".
6. **Do not:**
   - write new plans;
   - ask the owner what the Tauri build already answers;
   - redesign anything;
   - narrate.

## 6. File organisation is unacceptable

**Current state:** every source file sits flat in one folder.

| Folder | Top-level `.rs` files |
|---|---|
| `app/desktop/app/src` | 130 (6 subdirectories) |
| `crates/sidebar/src` | 215 |
| `crates/shell_data/src` | 210 |
| `crates/ui/src` | 189 |
| `crates/settings_ui/src` | 114 |
| `crates/terminal/src` | 75 |
| `crates/pane_host/src` | 64 |
| `crates/layout_view/src` | 55 |
| `crates/terminal_view/src` | 51 |
| `crates/daemon_supervisor/src` | 50 |
| `crates/webview/src` | 50 |

All crates above are under `app/desktop/`.

**Required:**
- **Module directories by responsibility**, each with its own private submodules: rules 14–17 and 45.
  "One type per file" does not mean one folder for every file.
- **The crate root exposes only the public interface and entry point.** Everything else lives in
  the module that owns it.
- **Do it while you revert and fix**, not as a separate pass: every file you touch moves into its
  module.
- **Every move is a pure move**, with the rules gate re-keyed (`check-rust-rules.sh --rekey`).
- The review session's own new file `app/desktop/app/src/traffic_lights.rs` landed in the flat
  folder too; move it with the window-chrome module.

## 7. Rabbyte rules, down to the pixel

The owner's rules, verbatim. Every line of Rust you touch must satisfy them. Where a rule names a Go
construct, apply the Rust equivalent in `docs/rust-style.md` (`docs/rabbyte-manifest.md` is the
source). Re-run the gates in `docs/standards-coverage.md` before every commit.

```text
I. Code
One parameter per line. The opening paren stays on the  func  line and the closing  )  gets its own line.

Maximum 2 indentation levels. Extract a method instead of reaching level 3.

Guard clauses first, no  else , success return last.

Use named returns instead of  var  in the body. Never use naked returns.

Extract long booleans into a named variable that reads like a sentence.

Prefer  slices ,  maps  and  cmp  over hand-rolled loops.

No inline comments and none on unexported symbols. Every exported symbol gets a detailed doc comment. The only allowed exceptions are deprecations, spec links and platform constraints.

II. Types8. One type per file, named after the type in  snake_case .
9. An  ErrXxx  sentinel lives in the same file as its error struct.
10. The package's main free function lives in the file named after the package.
11. A stateless type is a package-level function named after the package, not a struct.
12. Use  NewXxx  only when there is real state to initialize.
13. Public APIs expose interfaces. Implementation structs are unexported.
III. Modules14. The package root exposes only the interface, the entry-point function, the exported errors and the types those signatures use.
15. Everything else lives in the module's own  internal/  subtree.
16. Sibling modules never import each other's internals. Same-layer modules never import each other.
17. Dependencies flow one way, from higher layers to lower ones.
IV. Go internals18. Use  internal/  as the default home for anything not deliberately public.
19. Nest  internal/  at the lowest module that needs it.
20. No import cycles. Extract the shared contract into a third, lower package.
21. Package names are short, lowercase, singular nouns, with no underscores.
22. No  util ,  common ,  helpers ,  base  or  misc  packages.
23. No stutter.  vault.Vault  is the one allowed exception (rule 11).  vault.VaultManager  and  user.UserService  are not allowed.
24. No  init()  and no mutable package-level state.
25.  context.Context  is always the first parameter and is never stored in a struct.
26. Never ignore an error. Wrap with  %w  and compare with  errors.Is  and  errors.As .
27. No  panic  outside  main  and unrecoverable startup.
28. Keep interfaces small (1 to 3 methods) and define them in the consuming package.
V. Clean Code29. Names reveal intent. No abbreviations beyond  ctx  and  err .
30. A function does one thing, at one level of abstraction, and stays small.
31. A type has a single responsibility.
32. No magic numbers or strings. Use named constants.
33. No boolean flag parameters. Split into two functions.
34. Command-query separation. A function either changes state or returns information, not both.
35. No hidden side effects.
36. Extract duplication only at the third repetition.
37. Delete dead code, commented-out code and unused parameters.
38. Depend on abstractions and inject dependencies through constructors.
VI. Tests39. Table-driven tests, one behavior per case.
40. Test the exported interface, not internals.
41. No real infrastructure by default. Use in-memory fakes.
VII. Files42. 1:1 tests. Every  x.go  has exactly one  x_test.go  beside it, and all of its tests live there.
43. No free test files. A  _test.go  with no matching source file must not exist.
44. Source files stay under 500 to 600 lines, with no exceptions. Test files are exempt.
45. Overflow means the abstraction is wrong. Split by responsibility, not by line count.
```

The repo's `CLAUDE.md` also applies:
- A fix starts with a failing test.
- No timers, polls or retries as fixes.
- Delete code you make unused.
- No stubs that ship.
