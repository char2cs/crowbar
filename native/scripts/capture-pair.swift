#!/usr/bin/env swift
// capture-pair.swift — the S0.6 review instrument: a labelled, same-size PNG
// pair of two named windows, for a human to eyeball side by side.
//
// This is what replaces the retired per-component parity oracle (see
// `docs/superpowers/specs/2026-08-04-slice-based-port-method-design.md` §3.3).
// "0 deltas over N anchors" stopped being the gate; "the user has seen it
// beside Crowbar-React and accepted it" is the gate now, and this script
// produces the two images that review sits on. It has to be boring and
// reliable, because it is reused every slice from here on.
//
// USAGE
//
//   swift capture-pair.swift \
//     --a-label react --a-pid <pid> \
//     --b-label rust  --b-pid <pid> \
//     --width 1200 --height 800 \
//     --out-dir native/capture-out
//
// Each side is identified by `--<x>-pid <pid>` (preferred — unambiguous) or by
// `--<x>-owner <substring>` plus an optional `--<x>-title <substring>`
// (case-insensitive `CGWindowOwnerName` / `CGWindowName` match). At least one
// of pid/owner is required per side. If more than one on-screen window
// matches, the script refuses to guess and lists every candidate so the
// caller can add `--pid` — see "WHY PID, NOT JUST OWNER NAME" below.
//
// Full flag list: run with no arguments, or see `printUsage()` below.
//
// FOR THE REAL REACT-VS-RUST PAIR
//
// Launch Crowbar-React (`make dev-desktop`) and `crowbar-app` yourself,
// against the same `CROWBAR_HOME`, each already sized to the pair's target
// width/height by its own launch mechanism (Tauri's `tauri.conf.json` /
// window API on the React side; whatever `crowbar-app`'s S0.4 window-chrome
// work exposes on the Rust side once it lands — this script does not resize
// either window itself, see "WHY THIS SCRIPT DOES NOT RESIZE WINDOWS" below).
// Then:
//
//   swift native/scripts/capture-pair.swift \
//     --a-label react --a-owner Crowbar --a-pid <react-pid> \
//     --b-label rust  --b-owner crowbar-app --b-pid <rust-pid> \
//     --width 1200 --height 800 \
//     --out-dir native/capture-out
//
// `--a-pid`/`--b-pid` are what actually disambiguate — get them from
// `pgrep -f <binary>` or from whatever launched each process, not from
// `--owner` alone (both a dev build and a production build of Crowbar-React
// report `CGWindowOwnerName == "Crowbar"`, and so does a second dev instance
// from a sibling worktree — see the doc comment below).
//
// WHY PID, NOT JUST OWNER NAME
//
// `refdrive.swift` (`native/tools/refdrive/refdrive.swift`) already recorded
// this: "Sibling agent sessions run their own Crowbar from a different
// worktree, and macOS cascades both to the same origin, so 'the window at
// (262,122)' is not a unique thing to point at." The same is true here —
// `CGWindowOwnerName` is the app's display name, not a per-launch identity,
// so on a machine with more than one Crowbar-React window open, `--a-owner
// Crowbar` alone is ambiguous by design, not by bug. This script treats that
// ambiguity as a hard failure (`resolveOne`, below) rather than picking the
// first match, because a pair captured from the wrong window is worse than
// no pair — it looks like a finding and isn't one.
//
// WHY THIS SCRIPT DOES NOT RESIZE WINDOWS
//
// The obvious design would be "warp both windows to WxH, then capture."
// `refdrive.swift`'s `place`/`point` commands document at length why external
// window manipulation on this machine is not simple: moving or resizing
// another process's window needs either Accessibility permission (denied —
// `AXIsProcessTrusted()` is `false` in this process tree, see the finding
// below) or a tiling window manager's cooperation (AeroSpace, which resizes
// by *tiling*, not by setting an arbitrary WxH). Fighting that from here
// would reintroduce exactly the fragility refdrive spent a whole file
// documenting a way around for pointer input alone.
//
// So sizing is the launcher's job, not this script's: each app is asked to
// come up already at the target size, through its own mechanism (a CLI flag,
// a config file, a window API call at startup — whichever it has). This
// script's job is to *verify* that happened and *refuse to produce a pair
// that silently differs* — see "FORCED-EQUAL SIZE" below — which is a
// strictly safer contract than attempting a resize whose success can't be
// independently confirmed either.
//
// THE SETTLE STEP
//
// Recorded hazard (this file's header spec, §5.5, quoting the queue): "After
// any nav-stack push, carousel scroll or modal, the UI can sit mid-transition.
// A capture taken then is contaminated, not informative — one recorded case
// produced eight bogus `visible: false` deltas and a 147px height gap from a
// nav stack stuck at `x: -63`."
//
// A window's on-screen frame is the one thing CGWindowList can observe
// without any permission at all (proven below — `windows`-style listing needs
// no Screen Recording, no Accessibility). So "settled" here means: poll the
// specific window id's frame every `--settle-interval-ms`, and require
// `--settle-samples` consecutive identical reads before proceeding. If it
// never stabilises within `--settle-timeout-s`, this is a hard failure with
// the observed frame history printed — not a capture taken anyway. A short
// fixed `--grace-ms` pause runs after stability, because some transitions
// (CSS fades, opacity animations) do not move the window's frame at all and
// so are invisible to bounds-polling; the caller driving the app under test
// is still responsible for knowing when *its own* content is ready, this is
// a cheap safety margin on top, not a substitute for that.
//
// FORCED-EQUAL SIZE
//
// After settling, each side's frame is checked against `--width`/`--height`
// (within a 1pt tolerance — window managers occasionally round). A mismatch
// on either side aborts before any pixel is captured, and says which side and
// by how much, because "the two windows differ" is exactly the situation
// where producing a pair anyway would look like a rendering finding instead
// of a sizing bug.
//
// THE CAPTURE API — AND WHY IT IS NOT WHAT THE SPEC NAMED
//
// The method-design spec (§5.5) says: "`screencapture` is blocked for
// secondary windows; a `CGWindowList` Swift script is the working path."
// That was true when it was written. It no longer compiles on this machine:
//
//   error: 'CGWindowListCreateImage' is unavailable in macOS:
//   Please use ScreenCaptureKit instead.
//   (CGWindow.h: 'CGWindowListCreateImage' was obsoleted in macOS 15.0)
//
// This machine runs macOS 26.4. Apple removed the pixel-capture half of
// CGWindowList outright (window *listing* — `CGWindowListCopyWindowInfo`,
// used throughout this file and by `refdrive.swift` — is untouched and still
// needs no special permission; only *imaging* moved). So pixel capture here
// goes through `ScreenCaptureKit` (`SCScreenshotManager.captureImage`), which
// is the API Apple's own compiler error points at, and which needs Screen
// Recording (`kTCCServiceScreenCapture`) rather than the AX permission the
// old imaging path implicitly required in some configurations.
//
// THE PERMISSION FINDING, RECORDED RATHER THAN WORKED AROUND
//
// On the machine this was written on, Screen Recording is **not** granted to
// this process tree. Every process this agent shell spawns is a descendant of
// `/Applications/Crowbar.app/Contents/MacOS/crowbar-desktop` (confirmed via
// `ps -o pid,ppid,comm=` up the chain to launchd) — the production Crowbar
// app hosting this very agent session — and that is the TCC-responsible
// bundle for Screen Recording purposes. `CGPreflightScreenCaptureAccess()`
// reads `false`. A live `SCShareableContent` call fails with:
//
//   Error Domain=com.apple.ScreenCaptureKit.SCStreamErrorDomain Code=-3801
//   "The user declined TCCs for application, window, display capture"
//
// This matches, and updates, a finding already on record in `native/QUEUE.md`
// ("`screencapture` and the AX API are both permission-blocked in this
// shell") and `native/mapping/spinner.md` ("`screencapture` is denied Screen
// Recording for this process"). What is new here: the block is specifically
// against `Crowbar.app`, and it is not something this script — or any process
// in this tree — can grant itself. TCC screen-recording consent is a modal
// system dialog that (a) macOS will not even show for a process it has
// already recorded a decline for, and (b) requires a human with a mouse in
// System Settings → Privacy & Security → Screen Recording regardless. No
// tool available to this agent can click that checkbox, and guessing around
// it (e.g. `tccutil reset`, which only returns the state to "not
// determined" and would then hang this script on a system dialog nobody can
// dismiss) was deliberately not attempted.
//
// So: `preflightScreenCapture()` checks this up front and fails loudly with
// the remediation step, before wasting a settle cycle on a capture that can
// only fail. `--dry-run` (below) exists so the rest of the pipeline —
// resolution, disambiguation, settle-detection, forced-equal-size — can still
// be exercised and proven on a machine in this state, without pretending a
// pixel was captured when it was not.
//
// --dry-run: resolves, settles and size-checks both windows, prints exactly
// what a real run would, but skips `SCScreenshotManager` and writes nothing.
// It exists for exercising this pipeline on a machine where Screen Recording
// is not (yet) granted — see the finding above — and its output says so
// explicitly. It is a diagnostic mode, not a substitute deliverable: a
// `--dry-run` invocation never produces the PNG pair this tool exists to
// make, and never claims to.
//
// THE BLANK-CAPTURE SELF-CHECK
//
// A black or blank PNG is "the classic silent failure here" (this file's
// governing spec, again). Rather than trust that a non-throwing capture call
// means a good image, this script samples the resulting `CGImage` on a grid
// (`sampleIsBlank`, up to 64×64 points) and refuses to call the capture a
// success if every sampled pixel came back byte-identical — printing the
// sampled/distinct counts either way, so "not blank" is something the output
// states with a number behind it, not something assumed from a lack of error.

import AppKit
import CoreGraphics
import Foundation
import ScreenCaptureKit

// ─── small utilities ──────────────────────────────────────────────────────

// Unbuffer stdout so it interleaves correctly with the unbuffered stderr
// writes below — this is a diagnostic tool a human reads top to bottom, and
// block-buffered stdout (the default once output is piped rather than a
// live tty) otherwise prints an error before the "resolved"/"settled" lines
// that logically preceded it.
setvbuf(stdout, nil, _IOLBF, 0)

func eprint(_ s: String) {
    FileHandle.standardError.write((s + "\n").data(using: .utf8)!)
}

func fail(_ message: String) -> Never {
    eprint("capture-pair: \(message)")
    exit(2)
}

struct CaptureError: Error, CustomStringConvertible {
    let description: String
    init(_ s: String) { self.description = s }
}

// ─── window discovery (CGWindowList — no permission required) ────────────

/// One on-screen, ordinary (layer 0) window as CGWindowList reports it, in
/// the same coordinate space `refdrive.swift`'s `windowFrames` uses.
struct WindowState: Equatable {
    let id: Int
    let pid: Int32
    let owner: String
    let title: String
    let frame: CGRect
}

/// Every on-screen, real (layer 0, non-trivial area) window, unfiltered.
/// This alone needs no Screen Recording and no Accessibility permission —
/// proven live on the machine this was written on: `refdrive.swift windows`
/// and this function both return real data with
/// `CGPreflightScreenCaptureAccess() == false`.
func allWindows() -> [WindowState] {
    let opts: CGWindowListOption = [.optionOnScreenOnly, .excludeDesktopElements]
    guard let infos = CGWindowListCopyWindowInfo(opts, kCGNullWindowID) as? [[String: Any]] else {
        return []
    }
    return infos.compactMap { info in
        guard (info[kCGWindowLayer as String] as? Int) == 0,
              let b = info[kCGWindowBounds as String] as? [String: Any],
              let x = b["X"] as? Double, let y = b["Y"] as? Double,
              let w = b["Width"] as? Double, let h = b["Height"] as? Double,
              // Matches refdrive's own threshold: strips out menu-bar-overlay
              // slivers and zero-area entries that are not a capturable window.
              w > 100, h > 100,
              let wid = info[kCGWindowNumber as String] as? Int,
              let rawPid = info[kCGWindowOwnerPID as String] as? Int
        else { return nil }
        let owner = info[kCGWindowOwnerName as String] as? String ?? ""
        let title = info[kCGWindowName as String] as? String ?? ""
        return WindowState(id: wid, pid: Int32(rawPid), owner: owner, title: title,
                            frame: CGRect(x: x, y: y, width: w, height: h))
    }
}

/// One side's identification criteria. At least one of `pid`/`ownerContains`
/// must be set — enforced by the CLI parser, not here.
struct Target {
    let label: String
    let pid: Int32?
    let ownerContains: String?
    let titleContains: String?
}

/// Resolve a target to exactly one window, or fail with a diagnostic dump.
///
/// Refuses ambiguity rather than picking a first match — see the file header,
/// "WHY PID, NOT JUST OWNER NAME." A wrong-window capture is not a smaller
/// version of the right one; it is a false finding wearing a correct-looking
/// filename.
func resolveOne(_ target: Target) throws -> WindowState {
    let candidates = allWindows().filter { w in
        if let pid = target.pid, w.pid != pid { return false }
        if let oc = target.ownerContains, !w.owner.lowercased().contains(oc.lowercased()) { return false }
        if let tc = target.titleContains, !w.title.lowercased().contains(tc.lowercased()) { return false }
        return true
    }
    if candidates.isEmpty {
        let all = allWindows()
        let dump = all.map { "  id=\($0.id) pid=\($0.pid) owner=\($0.owner) title=\($0.title) frame=\($0.frame)" }
            .joined(separator: "\n")
        throw CaptureError("""
            [\(target.label)] no on-screen window matched \(describe(target)). \
            Every on-screen window right now:
            \(dump.isEmpty ? "  (none)" : dump)
            """)
    }
    if candidates.count > 1 {
        let dump = candidates.map { "  id=\($0.id) pid=\($0.pid) owner=\($0.owner) title=\($0.title) frame=\($0.frame)" }
            .joined(separator: "\n")
        throw CaptureError("""
            [\(target.label)] \(candidates.count) windows matched \(describe(target)), \
            and this script will not guess which one you meant:
            \(dump)
            Disambiguate with --\(target.label)-pid.
            """)
    }
    return candidates[0]
}

func describe(_ t: Target) -> String {
    var parts: [String] = []
    if let pid = t.pid { parts.append("pid=\(pid)") }
    if let oc = t.ownerContains { parts.append("owner~=\"\(oc)\"") }
    if let tc = t.titleContains { parts.append("title~=\"\(tc)\"") }
    return parts.joined(separator: " ")
}

// ─── the settle step ───────────────────────────────────────────────────────

/// Poll one window id's frame until it stops moving, or fail with the
/// observed history. See the file header, "THE SETTLE STEP."
func settle(windowID: Int, samples: Int, intervalMs: Int, timeoutS: Double) throws -> WindowState {
    let deadline = Date().addingTimeInterval(timeoutS)
    var history: [CGRect] = []
    var stableRun = 0
    var previousFrame: CGRect?
    while true {
        guard let current = allWindows().first(where: { $0.id == windowID }) else {
            throw CaptureError("window id \(windowID) disappeared while settling — it closed, or the id was recycled")
        }
        history.append(current.frame)
        if let prev = previousFrame, prev == current.frame {
            stableRun += 1
        } else {
            stableRun = 1
        }
        previousFrame = current.frame
        if stableRun >= samples {
            return current
        }
        if Date() >= deadline {
            let tail = history.suffix(8).map { "\($0)" }.joined(separator: " -> ")
            throw CaptureError("""
                window id \(windowID) never settled within \(timeoutS)s \
                (\(history.count) samples, needed \(samples) consecutive identical reads). \
                Last frames observed: \(tail)
                """)
        }
        usleep(UInt32(intervalMs * 1000))
    }
}

// ─── forced-equal size ──────────────────────────────────────────────────────

func assertSize(_ w: WindowState, label: String, width: Double, height: Double, tolerance: Double = 1.0) throws {
    let dw = abs(w.frame.width - width)
    let dh = abs(w.frame.height - height)
    guard dw <= tolerance, dh <= tolerance else {
        throw CaptureError("""
            [\(label)] window id \(w.id) is \(w.frame.width)x\(w.frame.height), \
            not the requested \(width)x\(height) (off by \(dw)x\(dh)pt). \
            This script does not resize windows — see the file header, \
            "WHY THIS SCRIPT DOES NOT RESIZE WINDOWS" — size the app at launch \
            and re-run.
            """)
    }
}

// ─── the blank-capture self-check ──────────────────────────────────────────

/// Sample the image on a coarse grid and report how many distinct pixel
/// values were seen. `distinct <= 1` over a real GUI window's content is the
/// classic silent-failure signature (a black or solid-colour capture) this
/// file's governing spec calls out by name.
func sampleIsBlank(_ image: CGImage, gridSize: Int = 64) -> (blank: Bool, distinct: Int, sampled: Int) {
    guard let data = image.dataProvider?.data as Data? else { return (true, 0, 0) }
    let bytesPerRow = image.bytesPerRow
    let bytesPerPixel = image.bitsPerPixel / 8
    let width = image.width
    let height = image.height
    guard width > 0, height > 0, bytesPerPixel > 0 else { return (true, 0, 0) }
    let stepsX = min(gridSize, width)
    let stepsY = min(gridSize, height)
    var seen = Set<UInt32>()
    var sampled = 0
    for iy in 0..<stepsY {
        let y = iy * height / stepsY
        for ix in 0..<stepsX {
            let x = ix * width / stepsX
            let offset = y * bytesPerRow + x * bytesPerPixel
            guard offset + bytesPerPixel <= data.count else { continue }
            var value: UInt32 = 0
            for b in 0..<min(bytesPerPixel, 4) {
                value = (value << 8) | UInt32(data[data.startIndex + offset + b])
            }
            seen.insert(value)
            sampled += 1
        }
    }
    return (seen.count <= 1, seen.count, sampled)
}

// ─── capture (ScreenCaptureKit) ─────────────────────────────────────────────

/// `CGPreflightScreenCaptureAccess()` up front, so a doomed capture fails in
/// milliseconds with the remediation step, rather than after a full settle
/// cycle. See the file header, "THE PERMISSION FINDING."
func preflightScreenCapture() throws {
    guard CGPreflightScreenCaptureAccess() else {
        throw CaptureError("""
            Screen Recording is not granted to this process tree \
            (CGPreflightScreenCaptureAccess() == false). This is an OS-level \
            permission, not a bug in this script: grant it via System \
            Settings -> Privacy & Security -> Screen Recording -> enable for \
            whichever app hosts this shell (check with \
            `ps -o pid,ppid,comm= -p $$` up to the first .app bundle), then \
            relaunch that app so the new grant takes effect. No process in \
            this tree can grant this to itself. Use --dry-run to exercise \
            window resolution, settle-detection and size-checking without \
            pixel capture in the meantime.
            """)
    }
}

func captureWindow(_ w: WindowState, scale: Double) async throws -> CGImage {
    let content: SCShareableContent
    do {
        content = try await SCShareableContent.excludingDesktopWindows(false, onScreenWindowsOnly: true)
    } catch {
        throw CaptureError("SCShareableContent lookup failed: \(error)")
    }
    guard let scWindow = content.windows.first(where: { $0.windowID == CGWindowID(w.id) }) else {
        throw CaptureError("window id \(w.id) is visible to CGWindowList but ScreenCaptureKit did not enumerate it (closed between settle and capture?)")
    }
    let filter = SCContentFilter(desktopIndependentWindow: scWindow)
    let config = SCStreamConfiguration()
    config.width = max(1, Int(w.frame.width * scale))
    config.height = max(1, Int(w.frame.height * scale))
    config.showsCursor = false
    do {
        let image: CGImage = try await SCScreenshotManager.captureImage(contentFilter: filter, configuration: config)
        return image
    } catch {
        throw CaptureError("SCScreenshotManager.captureImage failed: \(error)")
    }
}

// ─── PNG writing ────────────────────────────────────────────────────────────

func writePNG(_ image: CGImage, to url: URL) throws {
    let rep = NSBitmapImageRep(cgImage: image)
    guard let data = rep.representation(using: .png, properties: [:]) else {
        throw CaptureError("could not encode PNG for \(url.path)")
    }
    try data.write(to: url)
}

// ─── CLI ────────────────────────────────────────────────────────────────────

func printUsage() {
    print("""
        usage: swift capture-pair.swift \\
            --a-label <name> [--a-pid <pid>] [--a-owner <substr>] [--a-title <substr>] \\
            --b-label <name> [--b-pid <pid>] [--b-owner <substr>] [--b-title <substr>] \\
            --width <points> --height <points> \\
            --out-dir <dir> \\
            [--settle-samples 5] [--settle-interval-ms 200] [--settle-timeout-s 8] \\
            [--grace-ms 300] [--scale <factor>] [--dry-run]

        At least one of --<x>-pid / --<x>-owner is required per side; --<x>-pid
        is the disambiguating one and is preferred (see the file's header
        comment, "WHY PID, NOT JUST OWNER NAME"). --dry-run resolves, settles
        and size-checks both windows but skips pixel capture — useful on a
        machine without Screen Recording granted, see "THE PERMISSION FINDING."

        Output: <out-dir>/<label>-<width>x<height>-<timestamp>.png per side,
        plus a <out-dir>/pair-<timestamp>.json manifest with both paths and
        the settle/size evidence for each.
        """)
}

struct Options {
    var aLabel = ""
    var aPid: Int32?
    var aOwner: String?
    var aTitle: String?
    var bLabel = ""
    var bPid: Int32?
    var bOwner: String?
    var bTitle: String?
    var width: Double = 0
    var height: Double = 0
    var outDir = ""
    var settleSamples = 5
    var settleIntervalMs = 200
    var settleTimeoutS = 8.0
    var graceMs = 300
    var scale = (NSScreen.main?.backingScaleFactor).map(Double.init) ?? 2.0
    var dryRun = false
}

func parseArgs(_ args: [String]) -> Options {
    var o = Options()
    var i = 0
    func next() -> String {
        i += 1
        guard i < args.count else { fail("missing value after \(args[i - 1])") }
        return args[i]
    }
    func nextInt32(_ flag: String) -> Int32 {
        guard let v = Int32(next()) else { fail("\(flag) must be an integer") }
        return v
    }
    func nextInt(_ flag: String) -> Int {
        guard let v = Int(next()) else { fail("\(flag) must be an integer") }
        return v
    }
    func nextDouble(_ flag: String) -> Double {
        guard let v = Double(next()) else { fail("\(flag) must be a number") }
        return v
    }
    while i < args.count {
        let a = args[i]
        switch a {
        case "--a-label": o.aLabel = next()
        case "--a-pid": o.aPid = nextInt32(a)
        case "--a-owner": o.aOwner = next()
        case "--a-title": o.aTitle = next()
        case "--b-label": o.bLabel = next()
        case "--b-pid": o.bPid = nextInt32(a)
        case "--b-owner": o.bOwner = next()
        case "--b-title": o.bTitle = next()
        case "--width": o.width = nextDouble(a)
        case "--height": o.height = nextDouble(a)
        case "--out-dir": o.outDir = next()
        case "--settle-samples": o.settleSamples = nextInt(a)
        case "--settle-interval-ms": o.settleIntervalMs = nextInt(a)
        case "--settle-timeout-s": o.settleTimeoutS = nextDouble(a)
        case "--grace-ms": o.graceMs = nextInt(a)
        case "--scale": o.scale = nextDouble(a)
        case "--dry-run": o.dryRun = true
        case "-h", "--help": printUsage(); exit(0)
        default: fail("unknown argument \(a) — run with --help")
        }
        i += 1
    }
    guard !o.aLabel.isEmpty, !o.bLabel.isEmpty else { fail("--a-label and --b-label are required — run with --help") }
    guard o.aPid != nil || o.aOwner != nil else { fail("--a-pid or --a-owner is required — run with --help") }
    guard o.bPid != nil || o.bOwner != nil else { fail("--b-pid or --b-owner is required — run with --help") }
    guard o.width > 0, o.height > 0 else { fail("--width and --height are required and must be positive — run with --help") }
    guard !o.outDir.isEmpty else { fail("--out-dir is required — run with --help") }
    return o
}

// ─── orchestration ──────────────────────────────────────────────────────────

struct SideResult {
    let label: String
    let window: WindowState
    let path: String?
    let sampledDistinct: Int?
    let sampledCount: Int?
}

func runSide(_ target: Target, opts: Options, outDir: URL, stamp: String) async -> SideResult {
    let initial: WindowState
    do {
        initial = try resolveOne(target)
    } catch {
        eprint("\(error)")
        exit(3)
    }
    print("[\(target.label)] resolved id=\(initial.id) pid=\(initial.pid) owner=\(initial.owner) title=\(initial.title) frame=\(initial.frame)")

    let settled: WindowState
    do {
        settled = try settle(windowID: initial.id, samples: opts.settleSamples,
                              intervalMs: opts.settleIntervalMs, timeoutS: opts.settleTimeoutS)
    } catch {
        eprint("\(error)")
        exit(4)
    }
    print("[\(target.label)] settled at frame=\(settled.frame) (\(opts.settleSamples) consecutive identical reads)")

    do {
        try assertSize(settled, label: target.label, width: opts.width, height: opts.height)
    } catch {
        eprint("\(error)")
        exit(5)
    }
    print("[\(target.label)] size confirmed \(opts.width)x\(opts.height)pt")

    usleep(UInt32(opts.graceMs * 1000))

    if opts.dryRun {
        print("[\(target.label)] --dry-run: skipping pixel capture")
        return SideResult(label: target.label, window: settled, path: nil, sampledDistinct: nil, sampledCount: nil)
    }

    let image: CGImage
    do {
        image = try await captureWindow(settled, scale: opts.scale)
    } catch {
        eprint("[\(target.label)] \(error)")
        exit(6)
    }
    let (blank, distinct, sampled) = sampleIsBlank(image)
    print("[\(target.label)] captured \(image.width)x\(image.height)px — blank-check: \(distinct)/\(sampled) distinct sampled pixel values")
    if blank {
        eprint("""
            [\(target.label)] REFUSING this capture: every sampled pixel was \
            byte-identical (\(sampled) samples). This is the blank/black \
            silent-failure signature this tool exists to catch, not a valid \
            capture of a real UI.
            """)
        exit(7)
    }

    let fileName = "\(target.label)-\(Int(opts.width))x\(Int(opts.height))-\(stamp).png"
    let path = outDir.appendingPathComponent(fileName)
    do {
        try writePNG(image, to: path)
    } catch {
        eprint("[\(target.label)] \(error)")
        exit(8)
    }
    print("[\(target.label)] wrote \(path.path)")
    return SideResult(label: target.label, window: settled, path: path.path, sampledDistinct: distinct, sampledCount: sampled)
}

let opts = parseArgs(Array(CommandLine.arguments.dropFirst()))

if !opts.dryRun {
    do {
        try preflightScreenCapture()
    } catch {
        fail("\(error)")
    }
}

try? FileManager.default.createDirectory(at: URL(fileURLWithPath: opts.outDir), withIntermediateDirectories: true)
let outDir = URL(fileURLWithPath: opts.outDir)

let formatter = DateFormatter()
formatter.dateFormat = "yyyyMMdd-HHmmss"
let stamp = formatter.string(from: Date())

let targetA = Target(label: opts.aLabel, pid: opts.aPid, ownerContains: opts.aOwner, titleContains: opts.aTitle)
let targetB = Target(label: opts.bLabel, pid: opts.bPid, ownerContains: opts.bOwner, titleContains: opts.bTitle)

let resultA = await runSide(targetA, opts: opts, outDir: outDir, stamp: stamp)
let resultB = await runSide(targetB, opts: opts, outDir: outDir, stamp: stamp)

func manifestEntry(_ r: SideResult) -> String {
    """
        {
          "label": "\(r.label)",
          "window_id": \(r.window.id),
          "pid": \(r.window.pid),
          "owner": "\(r.window.owner)",
          "title": "\(r.window.title.replacingOccurrences(of: "\"", with: "\\\""))",
          "frame": {"x": \(r.window.frame.origin.x), "y": \(r.window.frame.origin.y), "width": \(r.window.frame.width), "height": \(r.window.frame.height)},
          "path": \(r.path.map { "\"\($0)\"" } ?? "null"),
          "sampled_distinct_pixel_values": \(r.sampledDistinct.map { "\($0)" } ?? "null"),
          "sampled_pixel_count": \(r.sampledCount.map { "\($0)" } ?? "null")
        }
        """
}

let manifest = """
    {
      "stamp": "\(stamp)",
      "width": \(opts.width),
      "height": \(opts.height),
      "dry_run": \(opts.dryRun),
      "a": \(manifestEntry(resultA)),
      "b": \(manifestEntry(resultB))
    }
    """
let manifestPath = outDir.appendingPathComponent("pair-\(stamp).json")
try? manifest.write(to: manifestPath, atomically: true, encoding: .utf8)
print("wrote manifest \(manifestPath.path)")
