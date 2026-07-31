// refdrive — real pointer input against the reference app, for the one state
// that cannot be driven any other way.
//
// WHY THIS EXISTS
//
// Every other state in the §8.3 matrix is reachable from JavaScript: `selected`
// is a click, `focus` is a focus() call, content length and theme are props.
// `hover` is not. CSS `:hover` is user-agent pointer state, maintained by
// WebKit from the real cursor — a dispatched `new MouseEvent('mouseover')` runs
// listeners and changes nothing about which rules match. Chrome's escape hatch,
// `CSS.forcePseudoState`, is a DevTools-protocol method and WKWebView does not
// speak that protocol.
//
// So the only honest way to observe what the reference paints on hover is to
// move the actual cursor over the actual element and look. That is what this
// does: CGWarpMouseCursorPosition to place it, then a synthesized
// kCGEventMouseMoved so WebKit gets an event to update hover state from. The
// warp alone is not enough — it relocates the cursor without telling anyone.
//
// CALIBRATION, RATHER THAN ARITHMETIC
//
// Converting a client coordinate to a screen coordinate needs the webview's
// origin inside the window, which depends on the titlebar style, and on whether
// the window is on a Retina display. Crowbar draws its own titlebar, so the
// offset is not a constant anyone can look up, and getting it wrong by a few
// points silently hovers the neighbouring row — which looks like a converged
// run against the wrong element.
//
// `calibrate` was to measure it instead of computing it: the caller installs a
// mousemove listener, this tool warps to a known screen point, and the page
// reports the clientX/clientY it observed. One sample would solve the offset
// exactly, because the mapping is a translation — the scale is 1, as client
// coordinates are CSS pixels and so are the AppKit points CGEvent uses, and
// Retina changes the backing store, not this mapping.
//
// **That design does not work on this machine, and it was never built.** It
// needs a mousemove to reach the page, and none does — see the section below on
// denied events. A listener installed on `window` in the capture phase counted
// zero real moves across an entire session while `:hover` sat there looking
// healthy.
//
// What replaces it is cheaper and does not need an event: `windows` reports the
// window's frame, and with `titleBarStyle: Overlay` the webview covers the whole
// window, so client (0,0) is the frame origin. That is an assumption, so do not
// trust it — it costs nothing to check, because the thing you actually care
// about is verifiable directly. Aim at a row, then ask the page whether *that
// row* matches `:hover`. If the offset were wrong you would be told, by the only
// question whose answer matters.
//
// PLACEMENT IS NOT OURS TO DO
//
// A pointer tool is useless against a window that is not on the screen, and for
// months of session time every Crowbar window on this machine sat at
// (1723, 1137) with a 77×32 corner showing — too little to hover a row. The
// natural readings of that were all wrong. It is not the app misplacing itself,
// it is not `core:window:allow-set-position` missing from the capability (it is
// missing, and that is irrelevant), and it is not a synthetic titlebar drag
// grabbing a dead point.
//
// It is **AeroSpace**, a tiling window manager, doing its job. AeroSpace hides
// the windows of a non-visible workspace by parking them just off the
// bottom-right of the display — which is exactly where they were, to the pixel,
// for four unrelated processes at once. It holds Accessibility permission and
// re-asserts each window's frame through the AX API, so it wins every argument
// about where a window goes. `place` therefore does not fight it. It asks.
//
// See `wm` for the diagnosis and `place` for the fix; both say more about their
// own limits at their implementation below.
//
// SYNTHETIC EVENTS ARE DENIED HERE, AND THE DENIAL IS SILENT
//
// The second surprise, and the one that invalidates the obvious reading of a
// `:hover` check. Posting a CGEvent — `post(tap:)` or `postToPid` — requires
// the post-events privilege. This process does not have it
// (`CGPreflightPostEventAccess()` is false), and macOS drops the events without
// an error: the call returns void, nothing throws, nothing arrives. The same
// goes for synthetic keystrokes.
//
// Meanwhile `document.querySelectorAll(':hover')` keeps answering with twenty
// or so elements, because WebKit's hover chain persists from wherever a human
// last left the mouse. Its *length* is not evidence of anything. Warping the
// cursor onto a different element does not change it. The only check worth
// making is whether the element you aimed at matches `:hover`.
//
// `point` is the way in: warp the cursor (ungated), then change the window's
// shape and put it back, which makes AppKit deliver a real mouse-moved at the
// cursor. See it for the detail and its limits.
//
// USAGE
//   swift refdrive.swift windows                 list on-screen Crowbar windows
//   swift refdrive.swift wm                      who owns window placement, and
//                                                where each Crowbar window is
//                                                parked — run this FIRST when a
//                                                window is not where you left it
//   swift refdrive.swift place <pid> [workspace] put that app's window alone on
//                                                a visible workspace, and prove
//                                                it landed on the screen
//   swift refdrive.swift point <pid> <sx> <sy>   move the pointer there AND make
//                                                the page recompute `:hover`
//                                                from it — what `hover` cannot
//                                                do without Accessibility
//   swift refdrive.swift activate <pid>          make that app frontmost first
//   swift refdrive.swift hover <screenX> <screenY>
//                                                warp only; the mouseMoved it
//                                                posts is dropped on a machine
//                                                without the post-events
//                                                privilege — prefer `point`
//   swift refdrive.swift park                    move the cursor far away, so
//                                                nothing is hovered (the
//                                                `resting` capture needs this —
//                                                a cursor left over the row
//                                                from a previous step makes
//                                                resting and hover identical)

import AppKit
import CoreGraphics
import Foundation

func fail(_ message: String) -> Never {
    FileHandle.standardError.write("refdrive: \(message)\n".data(using: .utf8)!)
    exit(2)
}

/// Place the cursor and *try* to tell the system it moved.
///
/// The order matters. `CGWarpMouseCursorPosition` updates the cursor location
/// but posts no event, so a webview that has not seen a mouse event since the
/// last one still believes the pointer is wherever it was. Posting the move
/// afterwards, at the same location, is meant to give WebKit the event it needs
/// to recompute `:hover`.
///
/// Only the warp is reliable. The post needs the post-events privilege, and
/// where that is missing macOS discards the event with no error of any kind —
/// `post` returns void whether or not anything was delivered, so there is
/// nothing here to check and nothing to report. `point` is the command that
/// closes this gap; this function is the half of it that always works, and is
/// left alone so `hover` keeps behaving as it always has on a machine that does
/// have the privilege.
func movePointer(to point: CGPoint) {
    CGWarpMouseCursorPosition(point)
    // Re-associate immediately: the warp suppresses hardware cursor updates for
    // a moment, and without this a physical mouse nudge during a run would be
    // ignored rather than merely racing us.
    CGAssociateMouseAndMouseCursorPosition(1)
    guard let move = CGEvent(
        mouseEventSource: nil,
        mouseType: .mouseMoved,
        mouseCursorPosition: point,
        mouseButton: .left
    ) else {
        fail("could not synthesize a mouseMoved event")
    }
    move.post(tap: .cghidEventTap)
}

func listWindows() {
    let opts: CGWindowListOption = [.optionOnScreenOnly, .excludeDesktopElements]
    guard let infos = CGWindowListCopyWindowInfo(opts, kCGNullWindowID) as? [[String: Any]] else {
        fail("CGWindowListCopyWindowInfo returned nothing")
    }
    var found = false
    for info in infos {
        let owner = info[kCGWindowOwnerName as String] as? String ?? "?"
        guard owner.lowercased().contains("crowbar") else { continue }
        found = true
        let number = info[kCGWindowNumber as String] as? Int ?? -1
        let pid = info[kCGWindowOwnerPID as String] as? Int ?? -1
        let layer = info[kCGWindowLayer as String] as? Int ?? -1
        let title = info[kCGWindowName as String] as? String ?? ""
        let b = info[kCGWindowBounds as String] as? [String: Any] ?? [:]
        let x = b["X"] as? Double ?? -1
        let y = b["Y"] as? Double ?? -1
        let w = b["Width"] as? Double ?? -1
        let h = b["Height"] as? Double ?? -1
        print("id=\(number) pid=\(pid) layer=\(layer) owner=\(owner) title=\(title) x=\(x) y=\(y) w=\(w) h=\(h)")
    }
    if !found {
        // Not an error: the caller may legitimately be checking whether the app
        // has finished launching yet.
        print("(no on-screen window owned by a process matching 'crowbar')")
    }
}

// ─── window placement ────────────────────────────────────────────────────────

/// Every window CoreGraphics knows about for one process, in display
/// coordinates (top-left origin, y growing downwards — the same space
/// `CGDisplayBounds` and `movePointer` use, so frames and cursor points are
/// directly comparable without a flip).
func windowFrames(pid: Int32) -> [(id: Int, frame: CGRect)] {
    let opts: CGWindowListOption = [.optionOnScreenOnly, .excludeDesktopElements]
    guard let infos = CGWindowListCopyWindowInfo(opts, kCGNullWindowID) as? [[String: Any]] else {
        return []
    }
    return infos.compactMap { info in
        guard (info[kCGWindowOwnerPID as String] as? Int).map({ Int32($0) }) == pid,
              (info[kCGWindowLayer as String] as? Int) == 0,
              let b = info[kCGWindowBounds as String] as? [String: Any],
              let x = b["X"] as? Double, let y = b["Y"] as? Double,
              let w = b["Width"] as? Double, let h = b["Height"] as? Double,
              // The 1800×39 strips every Tauri app publishes are the menu-bar
              // overlay, not a real window; a 0-area entry is not placeable
              // either. Neither is something a caller can hover.
              w > 200, h > 200,
              let id = info[kCGWindowNumber as String] as? Int
        else { return nil }
        return (id, CGRect(x: x, y: y, width: w, height: h))
    }
}

/// The union of every active display, so "on the screen" is a question with an
/// answer on a multi-monitor machine too.
func displayUnion() -> CGRect {
    var count: UInt32 = 0
    CGGetActiveDisplayList(0, nil, &count)
    var ids = [CGDirectDisplayID](repeating: 0, count: Int(count))
    CGGetActiveDisplayList(count, &ids, &count)
    return ids.reduce(CGRect.null) { $0.union(CGDisplayBounds($1)) }
}

struct ToolResult {
    let status: Int32
    let out: String
}

/// Shell out and wait. `Process` rather than `system()` so the output can be
/// parsed and a non-zero status reported with the tool's own words rather than
/// as a bare exit code.
@discardableResult
func runTool(_ path: String, _ arguments: [String]) -> ToolResult {
    let task = Process()
    task.executableURL = URL(fileURLWithPath: path)
    task.arguments = arguments
    let pipe = Pipe()
    task.standardOutput = pipe
    task.standardError = pipe
    do {
        try task.run()
    } catch {
        return ToolResult(status: 127, out: "could not run \(path): \(error)")
    }
    let data = pipe.fileHandleForReading.readDataToEndOfFile()
    task.waitUntilExit()
    let out = String(data: data, encoding: .utf8) ?? ""
    return ToolResult(status: task.terminationStatus, out: out.trimmingCharacters(in: .whitespacesAndNewlines))
}

/// Where AeroSpace's CLI lives, if it is installed at all.
///
/// Not `/usr/bin/env aerospace`: this tool is run from `swift refdrive.swift`
/// by whatever shell an agent happens to have, and `$PATH` in that shell has
/// repeatedly not included Homebrew. The bundled binary inside the .app is the
/// last candidate because it is the one that is certainly the same build as the
/// running server.
func aerospaceBinary() -> String? {
    let candidates = [
        "/opt/homebrew/bin/aerospace",
        "/usr/local/bin/aerospace",
        "/Applications/AeroSpace.app/Contents/MacOS/aerospace",
    ]
    return candidates.first { FileManager.default.isExecutableFile(atPath: $0) }
}

/// AeroSpace's own window ids for a process.
///
/// These are CoreGraphics window numbers — the same integers `windows` prints
/// as `id=`. That is not documented, it is observed, and it is worth stating
/// because it is what lets `place` address a window by pid and then verify the
/// result against a completely independent source (CGWindowList) rather than
/// against AeroSpace's own account of what it did.
func aerospaceWindows(binary: String, pid: Int32) -> [(id: Int, workspace: String)] {
    let result = runTool(binary, ["list-windows", "--all", "--format", "%{window-id}|%{app-pid}|%{workspace}"])
    guard result.status == 0 else { return [] }
    return result.out.split(separator: "\n").compactMap { line in
        let parts = line.split(separator: "|", omittingEmptySubsequences: false)
        guard parts.count == 3, let id = Int(parts[0]), Int32(parts[1]) == pid else { return nil }
        return (id, String(parts[2]))
    }
}

/// Make the app frontmost *and* its window key.
///
/// `NSRunningApplication.activate` is not enough, and its return value says so
/// only by lying: it returned `true` while `document.hasFocus()` in the target
/// webview stayed `false`. A window that is not key defeats far more than it
/// looks like it should — `:focus` does not match there even when
/// `document.activeElement` is the element you focused, so a `focus` capture
/// taken in that state silently measures the wrong thing.
///
/// Under a tiling window manager the WM owns focus as well as placement, so ask
/// it first and keep `activate` as the fallback for a machine without one. The
/// caller should still confirm with `document.hasFocus()`: neither route
/// reports failure.
func focusWindow(pid: Int32, windowID: Int) {
    if let binary = aerospaceBinary() {
        runTool(binary, ["focus", "--window-id", String(windowID)])
    }
    NSRunningApplication(processIdentifier: pid)?.activate(options: [])
}

/// Wait until the window is really where it was asked to go.
///
/// AeroSpace moves windows through the Accessibility API, which is asynchronous
/// with respect to its CLI returning: `move-node-to-workspace` exits 0 as soon
/// as the request is queued. Returning at that point would hand the caller a
/// window that is still parked, and the hover that follows would land on
/// nothing — the same silent failure `activate` exists to prevent.
///
/// So this blocks on the observable itself, the frame CoreGraphics reports,
/// rather than on a fixed sleep. The deadline is a failure report, not a
/// timing assumption: if it expires the window genuinely did not move.
func waitUntilOnScreen(pid: Int32, deadline: TimeInterval = 5) -> (id: Int, frame: CGRect)? {
    let screen = displayUnion()
    let until = Date().addingTimeInterval(deadline)
    repeat {
        // `contains` and not `intersects`: a window with one visible corner
        // intersects the display, and that corner is exactly the state this
        // whole command exists to escape.
        if let hit = windowFrames(pid: pid).first(where: { screen.contains($0.frame) }) {
            return hit
        }
        usleep(50_000)
    } while Date() < until
    return nil
}

let args = CommandLine.arguments
guard args.count >= 2 else {
    fail("usage: refdrive.swift windows | wm | place <pid> [workspace] | point <pid> <x> <y> | activate <pid> | hover <x> <y> | hoverpid <pid> <x> <y> | park")
}

switch args[1] {
case "windows":
    listWindows()

case "hover":
    guard args.count == 4, let x = Double(args[2]), let y = Double(args[3]) else {
        fail("usage: refdrive.swift hover <screenX> <screenY>")
    }
    movePointer(to: CGPoint(x: x, y: y))
    print("moved to \(x),\(y)")

case "activate":
    // macOS routes mouseMoved only to the active application, so a warp into an
    // inactive window changes nothing observable: the page sees no event and
    // `:hover` never matches. That failure is silent — the cursor really is over
    // the row, and the row really is not hovered — which is why this exists as
    // an explicit step rather than something the caller is trusted to remember.
    //
    // NSRunningApplication.activate is deliberately chosen over System Events:
    // the AppleScript route needs Accessibility permission, which this machine
    // does not grant to osascript, and activation does not require it.
    //
    // It also disambiguates overlapping windows. Sibling agent sessions run
    // their own Crowbar from a different worktree, and macOS cascades both to
    // the same origin, so "the window at (262,122)" is not a unique thing to
    // point at. Activating by pid makes the intended app frontmost, and the
    // caller confirms it landed by checking that its own page recorded the move.
    // ITS `true` IS NOT A PROMISE THE WINDOW IS KEY.
    //
    // Observed directly: this printed `-> true` while `document.hasFocus()` in
    // the target webview was `false`, and stayed false across repeated calls.
    // Activation makes the *application* frontmost; under a tiling window
    // manager the WM decides which window is key, and it had focused something
    // else. Only `aerospace focus --window-id` fixed it.
    //
    // That gap is worth more than it looks. A window that is not key does not
    // match `:focus` at all — `document.activeElement` is still the element you
    // focused, `:focus-visible` is quietly false, and a `focus` capture taken
    // there measures a resting row while claiming to be a focused one. `place`
    // and `point` therefore go through `focusWindow`, which asks the WM first.
    // This command is left as the thin wrapper it always was, for callers that
    // want exactly `NSRunningApplication.activate` and nothing else.
    //
    // Whichever you use, confirm on the page with `document.hasFocus()`.
    guard args.count == 3, let pid = Int32(args[2]) else {
        fail("usage: refdrive.swift activate <pid>")
    }
    guard let app = NSRunningApplication(processIdentifier: pid) else {
        fail("no running application with pid \(pid)")
    }
    let ok = app.activate(options: [])
    print("activate pid=\(pid) -> \(ok) (bundle=\(app.bundleIdentifier ?? "?")) — `true` does not mean the window is key; check document.hasFocus()")

case "hoverpid":
    // Last resort for a machine whose screen is locked.
    //
    // `hover` posts to the HID tap, which macOS routes to the *active*
    // application — and while the login window owns the session there is none,
    // so the event is synthesized correctly and delivered to nobody.
    //
    // `CGEventPostToPid` addresses a process directly instead of going through
    // that routing. Whether WebKit updates `:hover` from an event that arrived
    // this way is the open question this command exists to answer: the event
    // still has to reach the WKWebView's NSWindow and be dispatched as a real
    // mouseMoved, and an app that is not active may drop it earlier than that.
    //
    // The caller decides. This command only reports that the post was made —
    // it CANNOT tell you the hover landed. Confirm on the page side
    // (`document.querySelectorAll(':hover')`, or a mousemove listener) and
    // treat "posted" as meaning nothing on its own.
    guard args.count == 5, let pid = Int32(args[2]),
          let x = Double(args[3]), let y = Double(args[4])
    else {
        fail("usage: refdrive.swift hoverpid <pid> <screenX> <screenY>")
    }
    guard let move = CGEvent(
        mouseEventSource: nil,
        mouseType: .mouseMoved,
        mouseCursorPosition: CGPoint(x: x, y: y),
        mouseButton: .left
    ) else {
        fail("could not synthesize a mouseMoved event")
    }
    move.postToPid(pid)
    print("posted mouseMoved to pid=\(pid) at \(x),\(y) — verify on the page side")

case "park":
    // Top-left corner of the main display. Far from any row, and on screen, so
    // the cursor is somewhere real rather than clamped from an off-screen point.
    movePointer(to: CGPoint(x: 2, y: 2))
    print("parked at 2,2")

case "point":
    // Put the pointer somewhere AND make the page believe it — which `hover`,
    // on this machine, does not.
    //
    // WHY `hover` IS NOT ENOUGH, AND WHY THAT WAS INVISIBLE
    //
    // `hover` does two things: warp the cursor, then post a mouseMoved so
    // WebKit recomputes `:hover`. Only the first half works here. Posting a
    // synthetic CGEvent needs the post-events privilege, and this process does
    // not have it — `CGPreflightPostEventAccess()` is false, as is
    // `AXIsProcessTrusted()`. macOS drops such events **silently**: `post` and
    // `postToPid` both return void, both "succeed", and nothing arrives. Every
    // keystroke synthesised the same way is dropped too.
    //
    // The failure is disguised because `document.querySelectorAll(':hover')`
    // keeps returning a plausible 20-odd elements — the chain under wherever a
    // human last left the mouse. It is stale, and reading it as proof that
    // hover works is the trap: the chain does not change when the cursor is
    // warped onto a different element. The check that does not lie is whether
    // the *intended* element matches `:hover`, which is what the caller must
    // assert after calling this.
    //
    // WHAT DOES WORK
    //
    // `CGWarpMouseCursorPosition` is not gated, so the cursor really does move
    // (the app agrees: Tauri's `cursorPosition()` reports the warped point). It
    // is documented not to generate an event, which is the whole problem —
    // WebKit only re-hit-tests when an event arrives.
    //
    // An event does arrive when the *window* changes shape under a stationary
    // cursor: AppKit rebuilds the view's tracking areas and delivers a real
    // mouseMoved at the cursor's true position. Nobody needs permission for
    // that, because nobody is synthesising anything — the window server is
    // reporting a fact. So this warps, then asks AeroSpace to toggle the window
    // fullscreen and back, which changes the frame twice and puts it back
    // exactly where it was.
    //
    // LIMITS
    //
    //   - It needs AeroSpace. Without a window manager there is no ungated way
    //     to change the frame from outside the app, and the honest answer
    //     becomes "grant Accessibility" — see the message this prints.
    //   - The frame excursion is real. A resize relays out the page, and any
    //     scroll position not owned by application state snaps back: a
    //     carousel scrolled by assigning `scrollLeft` returns to panel 0, while
    //     one switched with the app's own tab control survives. Drive the app,
    //     not its scroll offsets, before calling this.
    //   - It reports where it put the cursor. It CANNOT tell you what ended up
    //     hovered — the window may have moved, the row may have scrolled.
    //     Assert on the page that the element you meant matches `:hover`.
    guard args.count == 5, let pid = Int32(args[2]),
          let x = Double(args[3]), let y = Double(args[4])
    else {
        fail("usage: refdrive.swift point <pid> <screenX> <screenY>")
    }
    guard let before = windowFrames(pid: pid).first(where: { displayUnion().contains($0.frame) }) else {
        fail("pid \(pid) has no window on the screen — run `place \(pid)` first")
    }
    // Frontmost first: an inactive app is not sent mouse-moved at all, so the
    // tracking-area event would be generated and discarded.
    focusWindow(pid: pid, windowID: before.id)
    movePointer(to: CGPoint(x: x, y: y))
    guard let binary = aerospaceBinary() else {
        fail("""
            cursor warped to \(x),\(y) but nothing can make the app read it.
            Synthetic events are denied to this process \
            (CGPreflightPostEventAccess=\(CGPreflightPostEventAccess())), and without \
            AeroSpace there is no way to change the window frame from outside \
            the app either. To drive real pointer input directly, the user must \
            grant Accessibility (System Settings -> Privacy & Security -> \
            Accessibility) to the application responsible for this process — for \
            an agent session that is the GUI app at the root of the process \
            tree, not `swift` and not this script.
            """)
    }
    // `on` then `off` rather than a single move: fullscreen is the one frame
    // change AeroSpace will make and then perfectly undo, so the window ends up
    // byte-identical to where it started and the caller's screen coordinates
    // stay valid. `--no-outer-gaps` makes the excursion bigger than the 5px
    // gaps, so the frame genuinely changes even when the window already fills
    // its workspace.
    let windowID = String(before.id)
    runTool(binary, ["fullscreen", "on", "--window-id", windowID, "--no-outer-gaps"])
    runTool(binary, ["fullscreen", "off", "--window-id", windowID])
    // Block on the frame coming back, for the same reason `place` does: the CLI
    // returns before the AX write lands, and a caller that measured geometry
    // against a half-restored window would compute the wrong screen point next
    // time.
    let until = Date().addingTimeInterval(5)
    var restored = false
    repeat {
        if windowFrames(pid: pid).contains(where: { $0.id == before.id && $0.frame == before.frame }) {
            restored = true
            break
        }
        usleep(50_000)
    } while Date() < until
    guard restored else {
        let now = windowFrames(pid: pid).first(where: { $0.id == before.id })?.frame
        fail("window \(before.id) did not return to \(before.frame) after the nudge (now \(now.map(String.init(describing:)) ?? "gone"))")
    }
    print("point pid=\(pid) at \(x),\(y) — frame \(before.frame) restored; verify :hover on the page")

case "wm":
    // Diagnosis, not action — run this before believing anything about where a
    // window is.
    //
    // A Crowbar window sitting off the screen looks like an app bug and is
    // almost never one. On this machine a tiling window manager parks the
    // windows of every non-visible workspace in the same off-screen corner, so
    // the symptom is identical no matter which app you look at — and the tell
    // that it is the WM, not the app, is precisely that: four unrelated
    // processes, one shared corner. Printing every Crowbar window's frame
    // beside its workspace makes that visible in one line each.
    //
    // Limit: this only knows about AeroSpace. Another WM, or a hand-placed
    // window, will show as "unmanaged" and this command will have nothing to
    // say about why the frame is where it is.
    let screen = displayUnion()
    print("displays: \(screen)")
    guard let binary = aerospaceBinary() else {
        print("window manager: none found (looked for AeroSpace)")
        print("  → placement is whatever the app and macOS decided; refdrive cannot change it")
        break
    }
    let focused = runTool(binary, ["list-workspaces", "--focused"])
    guard focused.status == 0 else {
        print("window manager: AeroSpace installed at \(binary) but not answering (server not running?)")
        print("  → \(focused.out)")
        break
    }
    print("window manager: AeroSpace at \(binary), focused workspace = \(focused.out)")
    let listing = runTool(binary, ["list-windows", "--all", "--format", "%{window-id}|%{app-pid}|%{workspace}|%{app-name}"])
    for line in listing.out.split(separator: "\n") {
        let parts = line.split(separator: "|", omittingEmptySubsequences: false)
        guard parts.count == 4, parts[3].lowercased().contains("crowbar") else { continue }
        let id = Int(parts[0]) ?? -1
        let pid = Int32(parts[1]) ?? -1
        let frame = windowFrames(pid: pid).first(where: { $0.id == id })?.frame
        let where_: String
        if let f = frame {
            where_ = screen.contains(f) ? "on screen \(f)" : "PARKED OFF SCREEN \(f)"
        } else {
            where_ = "no CoreGraphics frame (minimised, or not a real window)"
        }
        print("  id=\(id) pid=\(pid) app=\(parts[3]) workspace=\(parts[2]) — \(where_)")
    }

case "place":
    // Put one app's window on the screen, and refuse to claim success unless
    // CoreGraphics agrees that it is there.
    //
    // WHY IT DELEGATES TO AeroSpace RATHER THAN MOVING THE WINDOW ITSELF
    //
    // Nothing available to an unprivileged process can move another process's
    // window on macOS. The three routes all close:
    //
    //   - The page cannot ask Tauri to move itself: the app's capability grants
    //     `core:default` plus `core:window:allow-start-dragging`, and
    //     `core:window:default` is all getters — `set_position` is not in it.
    //   - AXUIElement (and System Events, which is a wrapper over it) needs
    //     Accessibility permission, which this machine does not grant to
    //     osascript: `-1719, not allowed assistive access`.
    //   - A synthetic titlebar drag is not the answer either, and the reason is
    //     worth recording because the obvious explanation is wrong. Crowbar
    //     sets `titleBarStyle: Overlay` with `hiddenTitle: true`, so the
    //     draggable area is whatever the webview marks with
    //     `data-tauri-drag-region` — and it was natural to assume a failed drag
    //     had grabbed a dead point. It had not.
    //     `document.elementFromPoint` over the top-left 90×40 of the window
    //     returns the drag-region div itself at 940 of 966 sampled points,
    //     including the (40, 6) that was tried. The grab was valid. The drag
    //     lost because AeroSpace re-asserts the frame of a window belonging to
    //     a non-visible workspace, and it holds the Accessibility permission
    //     that would be needed to argue.
    //
    // AeroSpace already has that permission and exposes a CLI, so the honest
    // move is to ask it. That is not a workaround; on this machine AeroSpace
    // *is* the placement authority, and a tool that fought it would be racing
    // something that always gets the last write.
    //
    // WHAT IT DOES
    //
    // Moves the app's window to a workspace of its own and makes that workspace
    // visible. A dedicated workspace, rather than the currently focused one,
    // because AeroSpace tiles: dropping a third window onto a workspace that
    // already holds two gives all three a third of the display, and a
    // 570-point-wide Crowbar hides the git panel the captures need. Alone on a
    // workspace, the window is tiled to the whole display every time — which
    // also makes the geometry reproducible run to run, not just visible.
    //
    // LIMITS — read these before trusting a capture taken minutes later
    //
    //   - It is not sticky. Focusing an app on another workspace switches
    //     AeroSpace away and re-parks this window. Sibling agent sessions run
    //     their own Crowbar and activating one does exactly that. Treat
    //     placement as something to re-establish immediately before each
    //     pointer step, not once per session; the command is idempotent and
    //     cheap, so re-running it costs nothing.
    //   - It changes what the human at the keyboard is looking at. There is no
    //     way around that: a window has to be visible to be hovered.
    //   - Without AeroSpace it does nothing but tell you so, and say what would
    //     have to be granted instead.
    guard args.count == 3 || args.count == 4, let pid = Int32(args[2]) else {
        fail("usage: refdrive.swift place <pid> [workspace]")
    }
    let workspace = args.count == 4 ? args[3] : "refdrive"

    // Already there? Say so and stop. Re-running `place` before every step is
    // the documented protocol, so the common case must be quiet and must not
    // switch workspaces for nothing.
    if let already = windowFrames(pid: pid).first(where: { displayUnion().contains($0.frame) }) {
        print("place pid=\(pid) — already on screen: id=\(already.id) \(already.frame)")
        break
    }

    guard let binary = aerospaceBinary() else {
        fail("""
            no window manager found, and this window is off the screen.
            refdrive cannot move another process's window on its own: Tauri's
            capability does not grant core:window:allow-set-position, and the
            AXUIElement route needs Accessibility permission. To place it
            without AeroSpace, the user must grant Accessibility (System
            Settings → Privacy & Security → Accessibility) to the program that
            will do the moving — osascript for a System Events one-liner, or
            whichever terminal runs this tool — and then it can be moved with
            AXPosition. Nothing here can grant that for you.
            """)
    }
    let found = aerospaceWindows(binary: binary, pid: pid)
    guard !found.isEmpty else {
        fail("AeroSpace knows no window for pid \(pid) — is the app finished launching?")
    }
    for window in found {
        let moved = runTool(binary, ["move-node-to-workspace", workspace, "--window-id", String(window.id)])
        guard moved.status == 0 else {
            fail("move-node-to-workspace \(workspace) for id \(window.id) failed: \(moved.out)")
        }
    }
    let switched = runTool(binary, ["workspace", workspace])
    guard switched.status == 0 else {
        fail("could not switch to workspace \(workspace): \(switched.out)")
    }
    guard let landed = waitUntilOnScreen(pid: pid) else {
        let frames = windowFrames(pid: pid).map { "id=\($0.id) \($0.frame)" }.joined(separator: ", ")
        fail("""
            moved pid \(pid) to workspace \(workspace) but no window of that \
            process is fully inside the displays \(displayUnion()); frames now: \
            \(frames.isEmpty ? "(none)" : frames)
            """)
    }
    // Frontmost as well as visible. `hover` needs the app active — see
    // `activate` — and having placed the window it is a footgun to leave that
    // to a separate step the caller may forget.
    focusWindow(pid: pid, windowID: landed.id)
    print("place pid=\(pid) workspace=\(workspace) -> id=\(landed.id) \(landed.frame)")

default:
    fail("unknown command \(args[1]) — expected windows, wm, place, point, activate, hover, hoverpid or park")
}
