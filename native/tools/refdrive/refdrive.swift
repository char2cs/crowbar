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
// `calibrate` therefore measures it instead of computing it. The caller installs
// a mousemove listener, this tool warps to a known screen point, and the page
// reports the clientX/clientY it observed. One sample solves the offset exactly,
// because the mapping is a translation: the scale is 1, as client coordinates
// are CSS pixels and so are the AppKit points CGEvent uses. Retina changes the
// backing store, not this mapping.
//
// USAGE
//   swift refdrive.swift windows                 list on-screen Crowbar windows
//   swift refdrive.swift activate <pid>          make that app frontmost first
//   swift refdrive.swift hover <screenX> <screenY>
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

/// Place the cursor and tell the system it moved.
///
/// The order matters. `CGWarpMouseCursorPosition` updates the cursor location
/// but posts no event, so a webview that has not seen a mouse event since the
/// last one still believes the pointer is wherever it was. Posting the move
/// afterwards, at the same location, gives WebKit the event it needs to
/// recompute `:hover`.
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

let args = CommandLine.arguments
guard args.count >= 2 else {
    fail("usage: refdrive.swift windows | hover <x> <y> | park")
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
    guard args.count == 3, let pid = Int32(args[2]) else {
        fail("usage: refdrive.swift activate <pid>")
    }
    guard let app = NSRunningApplication(processIdentifier: pid) else {
        fail("no running application with pid \(pid)")
    }
    let ok = app.activate(options: [])
    print("activate pid=\(pid) -> \(ok) (bundle=\(app.bundleIdentifier ?? "?"))")

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

default:
    fail("unknown command \(args[1]) — expected windows, hover or park")
}
