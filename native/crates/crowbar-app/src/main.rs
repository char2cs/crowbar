#![forbid(unsafe_code)]

//! `crowbar-app` — the Crowbar native binary.
//!
//! Slice 0 (item S0.2) moved the capture harness behind `--features driver`,
//! because the app cannot grow while its `main` is a test rig:
//!
//! * **`cargo run -p crowbar-app`** (no `driver` feature) opens a plain,
//!   contentless window. No matrix cell, no row, no chrome — later slices
//!   build the real frame; this one only proves the window opens.
//! * **`cargo run -p crowbar-app --features driver`** is unchanged from
//!   before this item: **the matrix surfaces** ([`row_surface`] over
//!   [`surfaces`]) — one surface, drawn in one cell of the §8.3 matrix, chosen
//!   by the command line. A surface registers by being a file in
//!   `src/surfaces/`; nothing dispatches on which one it is. Alongside it,
//!   **the driver's proving surface** (`driver_surface`) — the fixture
//!   `crowbar-driver`'s extractor was built against.
//! * `row_surface`, `surface` and `surfaces` also compile under `cargo test`
//!   even without the feature: `crowbar-driver` is a `[dev-dependencies]` on
//!   purpose (see `Cargo.toml`) so `cargo test --workspace` keeps measuring
//!   layout through the driver's extractor rather than skipping it.
//!
//! Item 0.4's daemon round trip survives in both binaries, as the window's
//! caption: the binary still derives the socket, asks `GET /v0/health` and
//! shows the answer, so the transport does not quietly stop being exercised.
//!
//! **This binary never touches the socket itself.** §4.2 makes `crowbar-client`
//! the only crate that talks to the daemon, and that stays true from the first
//! commit: a `UnixStream` here would be a hole in the layering that later items
//! would build on.
//!
//! S0.3 adds the piece item 0.4 deliberately deferred: the shipping `main`
//! (not the driver/capture-harness one — a parity run over hundreds of matrix
//! cells has no business spawning or supervising a daemon per invocation, and
//! stays a read-only client exactly as 0.4 left it) now calls
//! [`crowbar_sidecar::ensure_daemon`] before opening its window, so
//! `cargo run -p crowbar-app` no longer needs Crowbar-React to have started
//! the daemon first. `crowbar-sidecar` reaches the daemon through
//! `crowbar-client` too, so this does not reopen the hole the paragraph above
//! describes.

use std::borrow::Cow;
/// Only [`ui_font_paths`]/[`ui_mono_font_paths`] need this — both test-only,
/// see their own doc comments — so it would be unused outside `cargo test`
/// without the same gate.
#[cfg(test)]
use std::path::PathBuf;
use std::process::ExitCode;

use crowbar_client::{Health, HealthError, Probe};
/// Only the shipping (non-driver) window options need this — the driver
/// build's own `window_options` below never sets `window_background`, so
/// under `--features driver` this import would otherwise be unused.
#[cfg(not(feature = "driver"))]
use gpui::WindowBackgroundAppearance;
use gpui::{App, AppContext as _, SharedString, TitlebarOptions, WindowOptions};

/// Only the driver build's `main` parses a command line into a [`Cell`]; a
/// plain `cargo test --workspace` never calls [`Cell::parse`], so this stays
/// out of the `any(feature = "driver", test)` import above or it would be
/// unused there.
#[cfg(feature = "driver")]
use row_surface::ParseError;
/// The harness's own cell type. Needed by the driver build's `main`/`open`,
/// and by the tests below — `cargo test --workspace` measures layout through
/// it even without `--features driver` (see the module doc comment).
#[cfg(any(feature = "driver", test))]
use row_surface::{Cell, RowSurface};

/// The driver's proving surface (item P1.2). `--features driver` only, and even
/// then only when `CROWBAR_DRIVER_SNAPSHOT` asks for it: a driver build that is
/// *not* taking a measurement has to be the ordinary app with the driver
/// linked, because that is the configuration the oracle measures.
#[cfg(feature = "driver")]
mod driver_surface;
/// Compiled for `cargo test` as well as for a driver build. `state_of` is the
/// single place a command line becomes the differ's §8.3 matrix cell — and
/// since that cell is keyed on the **viewport** and the command line carries a
/// surface width too, a mapping only a feature build could test is a mapping
/// nobody tests.
#[cfg(any(feature = "driver", test))]
mod row_snapshot;
/// The shipping window's content (S1a). Not built under `--features driver`:
/// that build's `main` opens a matrix cell, and the sidebar is not one.
#[cfg(not(feature = "driver"))]
mod shell;

/// The driver-backed [`crowbar_ui::AnchorSink`]. A driver build
/// emits through it; `row_layout.rs` measures through it under a plain
/// `cargo test`, which is why it is not behind the feature alone.
#[cfg(any(feature = "driver", test))]
mod driver_anchors;

#[cfg(test)]
mod row_layout;
/// The capture harness (item S0.2). `cargo test --workspace` needs it too —
/// see the module doc comment and the `[dev-dependencies] crowbar-driver`
/// note in `Cargo.toml`.
#[cfg(any(feature = "driver", test))]
mod row_surface;
/// What a measurable surface is, and the traits its own file implements.
#[cfg(any(feature = "driver", test))]
mod surface;
/// One file per surface, and no list — the module declarations and the registry
/// are generated by `build.rs`. See `src/surfaces/mod.rs`.
#[cfg(any(feature = "driver", test))]
mod surfaces;
#[cfg(test)]
mod ui_font_fallback;
#[cfg(test)]
mod ui_font_mono;
#[cfg(test)]
mod ui_font_weight;

/// The capture harness's own entry point, unchanged by item S0.2: parses the
/// §8.3 cell off the command line, probes the daemon, and opens the matrix
/// surface (or, under `CROWBAR_DRIVER_SNAPSHOT`, runs the driver's own
/// proving surface instead — see [`driver_surface`]).
#[cfg(feature = "driver")]
fn main() -> ExitCode {
    let cell = match Cell::parse(std::env::args().skip(1)) {
        Ok(cell) => cell,
        Err(ParseError::HelpRequested) => {
            print!("{}", row_surface::usage());
            return ExitCode::SUCCESS;
        }
        Err(ParseError::Rejected(complaint)) => {
            eprintln!("crowbar-app: {complaint}\n");
            eprint!("{}", row_surface::usage());
            return ExitCode::from(2);
        }
    };

    for flag in cell.unmodelled_flags() {
        eprintln!(
            "crowbar-app: {} has no `{}` state in the React original — this cell renders \
             the resting surface, so a comparison of it proves nothing.",
            cell.surface.name,
            flag.name(),
        );
    }
    eprintln!("crowbar-app: {}", cell.describe());

    // Blocking, before the window exists, and on purpose: the daemon is a local
    // unix socket a few hundred microseconds away and this is a single request.
    let report = Report::probe();

    gpui_platform::application().run(move |cx: &mut App| {
        gpui_component::init(cx);
        let sans_fonts = load_ui_font(cx);
        let mono_fonts = load_ui_mono_font(cx);
        let fonts = format!("{sans_fonts}; {mono_fonts}");
        // On stderr as well as in the caption: a parity run that is silently
        // shaping with a fallback face would produce `text_width` deltas with
        // no visible cause.
        eprintln!("crowbar-app: {fonts}");

        if let Some(request) = driver_surface::Request::from_env() {
            driver_surface::run(request, cx);
            return;
        }

        let caption = format!("{} · {} · {fonts}", cell.describe(), report.summary());
        if let Err(err) = open(cell.clone(), caption, cx) {
            // No window means nothing can display the failure, so stderr is the
            // only channel left. Quitting beats sitting in a run loop with no UI.
            eprintln!("crowbar-app: could not open a window: {err}");
            cx.quit();
        }
    });

    ExitCode::SUCCESS
}

/// The shipping entry point (item S0.2): no matrix cell, no `--surface`
/// parsing, no harness at all — this build links none of it (see the module
/// doc comment). It keeps exactly one thing from the harness build: item
/// 0.4's daemon round trip, probed before the window opens and shown in its
/// caption, so the transport stays exercised by an ordinary `cargo run`.
///
/// S0.3 adds the other half of that round trip: [`crowbar_sidecar::ensure_daemon`]
/// runs first, adopting a daemon Crowbar-React already started or spawning one
/// of its own — `crowbar-app` no longer needs another app to have opened first
/// (`native/README.md`'s old "start Crowbar-React first" instruction is gone
/// because of this). `daemon` is bound in `main`'s own scope and shared
/// (`Rc`, not `Arc` — gpui callbacks are same-thread) with the
/// [`gpui::App::on_app_quit`] hook registered below, so a daemon this call
/// *spawned* is stopped on a graceful quit (Cmd+Q or the last window
/// closing) and not just when `run` happens to return — verified live
/// 2026-08-04: without this hook, killing the owning instance (as opposed to
/// letting it quit normally) orphaned the daemon it had spawned, because a
/// killed process runs no destructors at all. `Handle::shutdown` is a no-op
/// for an *adopted* daemon regardless of how this process ends — see
/// `crowbar_sidecar::Handle`'s own doc comment — which is what makes
/// "quitting the adopting app leaves the other one working" true even for an
/// abrupt kill, confirmed by the same live run.
///
/// The window itself is a placeholder on purpose. Later slices build the real
/// frame; this one only proves the app opens a window that is not the capture
/// harness.
#[cfg(not(feature = "driver"))]
fn main() -> ExitCode {
    // Blocking, before the window exists, and on purpose: adopting an
    // already-healthy daemon is one local request, and spawning one still
    // has to finish before there is anything for the window to show.
    let daemon = std::rc::Rc::new(
        match crowbar_sidecar::ensure_daemon(&crowbar_sidecar::Options::default()) {
            Ok(handle) => Some(handle),
            Err(err) => {
                // Not fatal: the window still opens, and `Report::probe` below
                // renders the same "daemon unreachable" state it always has —
                // a missing sidecar binary or a daemon that never comes up is a
                // displayed state, not a crash.
                eprintln!("crowbar-app: could not ensure a daemon: {err}");
                None
            }
        },
    );

    // Blocking, before the window exists, and on purpose: the daemon is a local
    // unix socket a few hundred microseconds away and this is a single request.
    let report = Report::probe();

    gpui_platform::application().run(move |cx: &mut App| {
        gpui_component::init(cx);
        let sans_fonts = load_ui_font(cx);
        let mono_fonts = load_ui_mono_font(cx);
        let fonts = format!("{sans_fonts}; {mono_fonts}");
        // On stderr as well as in the caption: a parity run that is silently
        // shaping with a fallback face would produce `text_width` deltas with
        // no visible cause.
        eprintln!("crowbar-app: {fonts}");

        // `on_quit`'s body runs synchronously before it returns a future, so
        // the blocking SIGTERM-then-grace-then-SIGKILL sequence inside
        // `shutdown` completes before this closure hands anything back to
        // gpui — `App::SHUTDOWN_TIMEOUT` (200ms) budgets the *future*, which
        // by then is already `Ready`, so it never has to race the shutdown.
        let quit_daemon = std::rc::Rc::clone(&daemon);
        cx.on_app_quit(move |_cx| {
            if let Some(handle) = quit_daemon.as_ref() {
                handle.shutdown("app quit");
            }
            std::future::ready(())
        })
        .detach();

        let caption = format!("{} · {fonts}", report.summary());
        if let Err(err) = open_shell(&report, caption, cx) {
            // No window means nothing can display the failure, so stderr is the
            // only channel left. Quitting beats sitting in a run loop with no UI.
            eprintln!("crowbar-app: could not open a window: {err}");
            cx.quit();
        }
    });

    ExitCode::SUCCESS
}

/// Opens the gate surface, and — in a driver build asked for a snapshot —
/// emits one snapshot of the **settled** frame and quits.
///
/// # Not the first frame, and that is this function's whole subject
///
/// This used to be a bare `on_next_frame`, which delivers at the top of the
/// platform's first frame request — after exactly one draw, the synchronous one
/// `App::open_window` performs before it returns. That is one draw, and a
/// deferred anchored popup is not in it: `gpui_component::Popover` renders its
/// popup only once the trigger's `prepaint` has measured it, which is after
/// `render` has returned, so the first draw is the trigger and nothing else and
/// the run died with `the root anchor "popover-popup" was not recorded this
/// frame`.
///
/// [`crowbar_driver::on_settled_frame`] replaces it with a signal instead of an
/// index: the capture happens on the first completed draw that reproduced the
/// previous completed draw's anchors. `crowbar-driver`'s `src/frame.rs`
/// documents why that is the right frame and why a surface that was already
/// correct is unaffected by it.
#[cfg(feature = "driver")]
fn open(cell: Cell, caption: String, cx: &mut App) -> gpui::Result<()> {
    let destination = row_snapshot::Destination::from_env();
    let registry = destination.is_some().then(|| crowbar_driver::install(cx));
    let measured = cell.clone();

    cx.open_window(window_options(&cell), move |window, cx| {
        if let (Some(registry), Some(destination)) = (registry, destination) {
            let watched = registry.clone();
            crowbar_driver::on_settled_frame(window, &watched, move |frame, window, cx| {
                let outcome = match frame {
                    crowbar_driver::Observation::Settled => {
                        // The drawable area the platform **granted**, read off
                        // the frame that was just drawn rather than the size
                        // that was asked for. macOS constrains a window to its
                        // display, so the two are not the same number on a
                        // display too short for the cell — and a surface cut by
                        // the window is a snapshot `emit` must refuse rather
                        // than write. See `row_snapshot::cut_by_the_window`.
                        let granted = window.viewport_size();
                        row_snapshot::emit(&registry, &measured, &destination, granted)
                    }
                    _ => Err(never_settled(&measured)),
                };
                row_snapshot::report(&outcome);
                cx.quit();
            });
        }
        cx.new(|_| RowSurface::new(cell, Box::new(driver_anchors::DriverAnchors), caption))
    })?;
    cx.activate(true);
    Ok(())
}

/// Why a cell whose picture never stopped moving gets no snapshot.
///
/// A refusal rather than a capture of whichever frame happened to be current: a
/// snapshot is a statement about a surface at rest, and one taken mid-motion
/// would compare against the reference's rest state and report deltas belonging
/// to neither side. See `crowbar-driver`'s `UNSETTLED_FRAME_LIMIT`.
#[cfg(feature = "driver")]
fn never_settled(cell: &Cell) -> String {
    format!(
        "`{}` recorded a different frame on each of {} consecutive draws, so it has no settled \
         picture to capture — its anchored geometry is a function of time, which a v1 snapshot \
         cannot represent. Put the motion on an unanchored child, as `spinner` does. Nothing was \
         written.",
        cell.surface.name,
        crowbar_driver::UNSETTLED_FRAME_LIMIT,
    )
}

/// The driver build's own window options: sized to the cell's surface, so the
/// window is exactly what the matrix run needs and nothing more.
///
/// Also reached by `cargo test --workspace` without the feature (the
/// `the_window_is_titled_and_sized_to_the_cell` test below), for the same
/// reason `row_surface`/`surface`/`surfaces` are: the harness is a
/// `[dev-dependencies]`, not only a `[features]`.
#[cfg(any(feature = "driver", test))]
fn window_options(cell: &Cell) -> WindowOptions {
    WindowOptions {
        titlebar: Some(TitlebarOptions {
            title: Some(SharedString::new_static("Crowbar (native)")),
            ..TitlebarOptions::default()
        }),
        window_bounds: Some(gpui::WindowBounds::Windowed(gpui::Bounds {
            origin: gpui::point(gpui::px(0.0), gpui::px(0.0)),
            size: RowSurface::window_size(cell),
        })),
        ..WindowOptions::default()
    }
}

/// Opens the shipping build's placeholder window (item S0.2).
///
/// **Deliberately minimal** in its *content*: this is Slice 0's frame, not a
/// surface — no row, no anchors, no sidebar/tabs/panes, those are later
/// slices' work. All it draws is `caption`, which is item 0.4's daemon round
/// trip, so a bare `cargo run -p crowbar-app` still proves the transport
/// works. Its *chrome* is not a placeholder, though (item S0.5, spec §5.4):
/// [`decorate_window`] applies the real vibrancy blur and appearance pin
/// immediately after the window exists, before the root view is built —
/// mirroring `Window::new` returning before `build_root_view` runs (see
/// `gpui::App::open_window`'s own body) — so every later slice's content
/// drops into the real Crowbar frame from its first frame, not a bare one a
/// later item would have to retrofit.
#[cfg(not(feature = "driver"))]
fn open_shell(report: &Report, caption: String, cx: &mut App) -> gpui::Result<()> {
    // No resolvable home means nothing to stream from. The window still
    // opens — a connection error is displayed, never panicked on, which is
    // the rule item 0.4 set and every slice since has kept.
    let socket = report.socket().map(std::path::Path::to_path_buf);

    // Restored from the daemon's own `global` UI rows rather than from a file
    // of this app's own, so both frontends come up at the same width against
    // one CROWBAR_HOME — which is what makes a side-by-side capture a
    // comparison rather than a coincidence of defaults. S1a reads the
    // defaults; writing them back is the settings slice's own item.
    let store = crowbar_state::SidebarStore::build(cx, None, None, None);

    if let Some(socket) = socket {
        let mut sync = crowbar_state::DaemonSync::new(&socket, &store);

        // The project list first: it is what tells the app a project exists at
        // all. Everything else is opened by the coordinator once there is an
        // active project to scope it to.
        shell::coordinator::reconcile(&store, &mut sync, cx);

        // Re-run the decision on every store change. `observe` rather than a
        // hand-rolled notify chain so a mutation cannot forget to re-scope: a
        // repo arriving on the repo stream has to open its own workspace
        // stream, and that repo arrives as a frame, not as a user action.
        cx.observe(&store, move |store, cx| {
            shell::coordinator::adopt_first_project(&store, cx);
            shell::coordinator::reconcile(&store, &mut sync, cx);
        })
        .detach();
    }

    cx.open_window(placeholder_window_options(), move |window, cx| {
        decorate_window(window);
        let sidebar = shell::Sidebar::build(&store, cx);
        cx.new(|_| shell::Shell {
            sidebar,
            caption: caption.into(),
            store,
        })
    })?;
    cx.activate(true);
    Ok(())
}

/// [`open_placeholder`]'s window: no cell to size against, so this asks the
/// platform for its own default bounds rather than choosing a number that
/// would be this item's own invented chrome. The chrome itself (item S0.5)
/// mirrors `desktop/src-tauri/tauri.conf.json`'s `app.windows[0]` entry:
/// `transparent: true` → [`WindowBackgroundAppearance::Transparent`] (so
/// [`decorate_window`]'s vibrancy view is visible rather than painted over —
/// see `crowbar_platform::vibrancy::apply_vibrancy`'s own doc comment for why
/// this has to be set here, at window-creation time, rather than by the
/// vibrancy call itself); `titleBarStyle: "Overlay"` + `hiddenTitle: true` →
/// `appears_transparent: true` (GPUI's `gpui_macos` backend turns this into
/// `NSFullSizeContentViewWindowMask` + `titlebarAppearsTransparent` +
/// `NSWindowTitleHidden` itself — see `vendor/zed-deps/gpui_macos/src/
/// window.rs` around its `titlebar.appears_transparent` checks — so nothing
/// in this crate has to reach for `unsafe` to get the overlay titlebar);
/// `trafficLightPosition: {x: 12, y: 23}` → the identical `traffic_light_
/// position`. Rounded window edges need no setting at all: they are what a
/// normal titled `NSWindow` (`titlebar: Some(...)`, not `None` — the
/// borderless-window shape `gpui_macos` builds when `titlebar` is absent)
/// already draws; nothing about `appears_transparent` or the vibrancy view
/// changes the window's own corner shape.
#[cfg(not(feature = "driver"))]
fn placeholder_window_options() -> WindowOptions {
    WindowOptions {
        titlebar: Some(TitlebarOptions {
            title: Some(SharedString::new_static("Crowbar (native)")),
            appears_transparent: true,
            traffic_light_position: Some(gpui::point(gpui::px(12.0), gpui::px(23.0))),
        }),
        window_background: WindowBackgroundAppearance::Transparent,
        ..WindowOptions::default()
    }
}

/// Applies Crowbar-React's window chrome (item S0.5, spec §5.4) to a freshly
/// created window: the `HudWindow` vibrancy blur, then the appearance pin
/// that keeps its frost matching the app's own theme instead of the OS's.
/// Both live in `crowbar-platform` — see `crowbar_platform::vibrancy`'s
/// module doc comment for why this crate needs no `unsafe` of its own to
/// call them (`apply_vibrancy` is safe at its call boundary; `pin_appearance`
/// and `inspect` are `crowbar-platform`'s own, proven unsafe).
///
/// `dark: true` is hardcoded rather than read from a theme store because
/// Slice 1a builds no settings surface and the shell's own root view
/// hardcodes `crowbar_ui::Theme::DARK` — pinning the frost to match is
/// "theme applied at app level" for exactly as much theme as this slice has.
/// A real theme switcher (slice 2) re-pins on change; nothing here prevents
/// that.
///
/// Every step is logged rather than silently swallowed, on failure *and* on
/// success: a build that links `apply_vibrancy` and renders no blur is
/// exactly the failure this item's acceptance gate exists to catch (spec
/// §5.4), so [`crowbar_platform::inspect`] reads the state back from `AppKit`
/// itself and reports it — this is also the non-pixel evidence this item's
/// own acceptance report leans on where a screenshot could not be taken (see
/// `crowbar_platform::vibrancy::Inspection`'s doc comment).
#[cfg(all(not(feature = "driver"), target_os = "macos"))]
fn decorate_window(window: &gpui::Window) {
    if let Err(err) = crowbar_platform::apply_vibrancy(window) {
        eprintln!("crowbar-app: failed to apply window vibrancy: {err}");
        return;
    }
    if let Err(err) = crowbar_platform::pin_appearance(window, true) {
        eprintln!("crowbar-app: failed to pin the vibrancy appearance: {err}");
        return;
    }
    match crowbar_platform::inspect(window) {
        Ok(inspection) => eprintln!(
            "crowbar-app: window chrome: blur_view_present={} window_is_opaque={}",
            inspection.blur_view_present, inspection.window_is_opaque
        ),
        Err(err) => eprintln!("crowbar-app: could not inspect the window chrome: {err}"),
    }
}

/// Off macOS, `crowbar-platform`'s vibrancy module does not exist at all (see
/// its `#[cfg(target_os = "macos")]`), so there is nothing to call here —
/// mirrors `desktop/src-tauri/src/lib.rs`'s own `#[cfg(not(target_os =
/// "macos"))] fn decorate_window` no-op.
#[cfg(all(not(feature = "driver"), not(target_os = "macos")))]
fn decorate_window(_window: &gpui::Window) {}

/// The family the row declares, and therefore the family the snapshot reports.
///
/// It is the *stylesheet's* `@font-face` name (`web/src/styles/theme.css`), not
/// the name inside the original font file — see [`UI_FONT_BYTES`].
const UI_FONT_FAMILY: &str = "CalSansUI";

/// The three source files [`UI_FONT_BYTES`] is embedded from — test-only.
/// Nothing in the shipping binary reads these filenames at runtime; they exist
/// so a test can (a) assert the vendored files genuinely sit at
/// `native/assets/fonts/`, the location [`ui_font_paths`]'s own doc comment
/// explains, and (b) hand real paths to [`crate::ui_font_fallback`] /
/// [`crate::ui_font_mono`], which need a *second*, freshly built platform text
/// system loaded from something other than the one `Vec` [`load_ui_font`]
/// already consumed.
///
/// **Provenance, because these are derived files.** All three are produced from
/// the repo's own `web/public/fonts/CalSansUI.woff2` — which stays, and which
/// the web app still loads — by `fontTools`:
///
/// ```text
/// instantiateVariableFont(TTFont('CalSansUI.woff2'), {'wght': W, 'GEOM': 0})
/// font.flavor = None                       # drop the WOFF2 container
/// name IDs 1/4/16 := "CalSansUI"           # the @font-face name, see below
/// ```
///
/// Three decisions are load-bearing:
///
/// * **WOFF2 → bare sfnt.** gpui hands `add_fonts` bytes to CoreText through
///   font-kit, and CoreText does not read the WOFF2 container: the original
///   file fails with `font: CalSansUI rejected (parse error)`. Nothing but the
///   container changes — the `glyf`, `hmtx`, `GPOS` and `cmap` tables are the
///   ones the browser shapes with, so the advance widths are the same numbers.
/// * **Static instances, not the variable face.** `CalSansUI.woff2` is variable
///   (`wght` 400–700, `GEOM` 0–100). font-kit picks a face out of a family by
///   reading `OS/2.usWeightClass` and has no way to ask for a `wght`
///   coordinate, so a single variable face would answer *every* weight request
///   with its 400 default. Two instances (400/500) left a 600 request with
///   nothing closer than 500 to fall back on — P3.25's defect: `font-semibold`
///   (`--font-weight-semibold: 600`, 41 call sites in `web/src`) snapped to
///   Medium natively while `WebKit` shaped a real semibold. A third instance,
///   `CalSansUI-SemiBold.ttf` (`wght: 600`), gives `find_best_match` a 600
///   candidate to prefer over 500 (`|600-600|=0` beats `|600-500|=100`); no
///   live call site requests 700 (`avatar.rs`'s `WEIGHT_BOLD` constant is
///   defined but never reaches a `.font_weight()` call), so a fourth,
///   Bold instance is not this item's defect and was not added. `GEOM` is
///   pinned at its default because nothing in the app sets
///   `font-variation-settings`.
/// * **The family is renamed `Cal Sans UI` → `CalSansUI`.** font-kit's
///   `MemSource` matches a family by exact name, and the name in the file is
///   `Cal Sans UI` while the stylesheet's `@font-face` declares `CalSansUI`.
///   Leaving them different means either the family never resolves (and the row
///   silently shapes with a fallback, which is the failure this whole comment
///   exists to prevent) or the row declares `Cal Sans UI` and every text
///   anchor reports a `font.family` the DOM will never produce.
///   `licenses/CalSans-OFL-1.1.txt` declares **no Reserved Font Name**, so
///   OFL-1.1 §3 places no restriction on a Modified Version's name.
///
/// `CalSansUI-SemiBold.ttf`'s `OS/2.fsSelection` additionally has its
/// `REGULAR` bit cleared (matching `CalSansUI-Medium.ttf`'s own `128`, not the
/// variable source's inherited `192`) — the source font's `OS/2` table is the
/// *default* instance's (`wght: 400`, `REGULAR`), and `instantiateVariableFont`
/// does not revise it for a pinned non-Regular weight on its own.
#[cfg(test)]
const UI_FONT_FILES: [&str; 3] = [
    "CalSansUI-Regular.ttf",
    "CalSansUI-Medium.ttf",
    "CalSansUI-SemiBold.ttf",
];

/// Where [`UI_FONT_FILES`]' faces live on disk — test-only; see
/// [`UI_FONT_BYTES`] for what the shipping binary actually loads and why it is
/// not this.
///
/// `CROWBAR_ROW_FONT` is still honoured here (mirroring [`load_ui_font`]'s own
/// override check) purely so a test run made under a parity session that has
/// the variable already exported does not silently read two different faces
/// from its two loaders.
#[cfg(test)]
fn ui_font_paths() -> Vec<PathBuf> {
    if let Ok(overridden) = std::env::var("CROWBAR_ROW_FONT") {
        return vec![PathBuf::from(overridden)];
    }
    let fonts = PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../assets/fonts");
    UI_FONT_FILES.iter().map(|file| fonts.join(file)).collect()
}

/// The three faces [`load_ui_font`] actually registers by default, embedded
/// into the binary at compile time from `native/assets/fonts/` — the same
/// three files [`UI_FONT_FILES`] names and [`ui_font_paths`] resolves on disk
/// (proved equal by `the_embedded_bytes_match_the_vendored_files` below), not
/// read from a filesystem path at startup.
///
/// **Why embedded, and not `env!("CARGO_MANIFEST_DIR")).join(...)` the way
/// this read `web/public/fonts` before S0.7.** That join is a *runtime*
/// filesystem read of a *compile-time* absolute path: it resolves on the
/// machine that built the binary, for as long as that machine's checkout
/// still has the joined-to directory sitting next to it. S0.7 exists because
/// that stopped being good enough on two counts — `native/`'s whole point is
/// to stop needing `web/` to exist at all (the eventual plan is to delete it),
/// and a *packaged* build is not run on the machine that built it, so even a
/// path that stayed under `native/` would still break the moment the binary
/// is copied anywhere else without its `assets/` directory in tow.
/// `include_bytes!` sidesteps both: the six files (~560KB total, measured)
/// are folded into the compiled artifact at build time, so the binary itself
/// is the only thing a shipped build has to carry. This is not a novel
/// choice — `vendor/gpui/examples/text.rs` embeds its own default face the
/// identical way
/// (`include_bytes!("../../../assets/fonts/lilex/Lilex-Regular.ttf")`), so
/// this follows gpui's own idiom for a default font rather than inventing one.
///
/// `CROWBAR_ROW_FONT` still reads an arbitrary file from *disk* at runtime
/// (see [`load_ui_font`]) — deliberately not embedded, because the override
/// exists so a parity run can point the row at some other face **without a
/// rebuild**, which is a different requirement from "the shipping default
/// must not depend on a path surviving."
const UI_FONT_BYTES: [&[u8]; 3] = [
    include_bytes!("../../../assets/fonts/CalSansUI-Regular.ttf"),
    include_bytes!("../../../assets/fonts/CalSansUI-Medium.ttf"),
    include_bytes!("../../../assets/fonts/CalSansUI-SemiBold.ttf"),
];

/// Gives gpui the same faces the React app renders with, and says plainly
/// whether it worked.
///
/// **This is a parity dependency, not a nicety.** `text_width` is the field the
/// gate was chosen for, and it is a property of the shaped face: the two apps
/// can agree on every box and still disagree on where the ellipsis lands if
/// they are shaping with different fonts.
///
/// The registration is checked as well as attempted, and that is the point.
/// `add_fonts` returning `Ok` only says CoreText parsed the bytes — it says
/// nothing about whether `font_family("CalSansUI")` will *find* them. If it
/// does not, gpui falls back silently, the row goes on **declaring** the family
/// it was told to, and `font.family` compares equal on both sides while every
/// `text_width` underneath is the wrong typeface. That is the worst kind of
/// agreement: the one field that would reveal the problem is the field that
/// matches. So the family is looked up by name afterwards and a miss is a
/// different message, not a quieter one.
///
/// **`CROWBAR_ROW_FONT`** overrides the default entirely with a single file
/// read from disk at startup — for a parity run that wants to point the row
/// at some other face to prove a suspected shaping difference, without a
/// rebuild. One file is enough for that; it is not a general multi-face
/// mechanism.
fn load_ui_font(cx: &mut App) -> String {
    let faces: Vec<Cow<'static, [u8]>> = match std::env::var("CROWBAR_ROW_FONT") {
        Ok(overridden) => match std::fs::read(&overridden) {
            Ok(bytes) => vec![Cow::Owned(bytes)],
            Err(err) => return format!("font: {overridden} not read ({err})"),
        },
        Err(_) => UI_FONT_BYTES
            .iter()
            .map(|bytes| Cow::Borrowed(*bytes))
            .collect(),
    };

    if let Err(err) = cx.text_system().add_fonts(faces) {
        return format!("font: {UI_FONT_FAMILY} rejected ({err})");
    }
    if !cx
        .text_system()
        .all_font_names()
        .iter()
        .any(|name| name == UI_FONT_FAMILY)
    {
        return format!("font: {UI_FONT_FAMILY} parsed but the family did not register");
    }
    format!("font: {UI_FONT_FAMILY} loaded")
}

/// `theme.font_mono`'s primary stack entry (`theme/generated.rs`) — the
/// *stylesheet's* `--font-mono: var(--editor-font-family, 'JetBrains Mono
/// Variable', …)` name, not the family the source npm package's own `name`
/// table carries before this item renames it (see [`UI_MONO_FONT_BYTES`]).
///
/// # This item's defect, and the two it repeats the shape of
///
/// Before this item, **no monospace font was registered at all.** Four
/// ported components (`fps_overlay`, `file_tree_row`, `inline_error`, and —
/// only in a module doc comment, never a live call site; see
/// `crowbar-ui/src/components/command.rs`'s own docs — `command`) declare
/// `.font_family(theme.font_mono.primary()…)`, and every one of them shaped
/// with whatever CoreText's own fallback cascade supplied instead, silently:
/// `font_family("JetBrains Mono Variable")` is a request, not a guarantee,
/// exactly the lesson `UI_FONT_FILES`'s own doc comment already draws for
/// `CalSansUI` (P3.24) and `UI_MONO_FONT_FILES` draws again here. Measured
/// live against the reference (`fps-overlay`, the one anchor in this cluster
/// that is `content_sized` and therefore the one surface a missing mono font
/// could not hide behind): the badge's text advance came out to 157px
/// against `WebKit`'s own 182.41px for the identical string — a **narrower**
/// fallback, not a wider one, so this was never a case a human glancing at a
/// screenshot would flag as "obviously wrong font."
///
/// # Where the reference gets it from — established, not assumed
///
/// `web/src/styles/editor-theme.css` line 1: `@import
/// '@fontsource-variable/jetbrains-mono';`, pinned at `5.2.8` in `web/bun.lock`.
/// This is an **npm-distributed, project-pinned dependency** — not a font
/// that merely happens to be installed on any one developer's machine — so
/// shipping the app without it would itself be the defect the brief for this
/// item warned to check for. It was not that: the family the reference
/// resolves is exactly the family this npm package's own `@font-face`
/// declares.
const UI_MONO_FONT_FAMILY: &str = "JetBrains Mono Variable";

/// The faces this row shapes `theme.font_mono` text with, converted for
/// CoreText — three static instances (`wght` 400/500/700) pulled out of the
/// same **variable** font the browser itself instances live for every
/// `--font-mono` weight request.
///
/// **Provenance.** `web/bun.lock` pins
/// `@fontsource-variable/jetbrains-mono@5.2.8`; the bytes came from that exact
/// version's own package tree (`~/.bun/install/cache/@fontsource-variable/
/// jetbrains-mono@5.2.8@@@1/files/jetbrains-mono-latin-wght-normal.woff2` —
/// the same tree `bun install` places under `web/node_modules`, which this
/// checkout does not carry). Converted with the same `fontTools` recipe
/// [`UI_FONT_FILES`]'s own doc comment records for `CalSansUI`:
///
/// ```text
/// instantiateVariableFont(TTFont('jetbrains-mono-latin-wght-normal.woff2'), {'wght': W})
/// font.flavor = None                          # drop the WOFF2 container
/// name IDs 1/4/16 := "JetBrains Mono Variable" # the @font-face name, see below
/// ```
///
/// Three decisions, each the mono sibling of one `UI_FONT_FILES` already
/// makes:
///
/// * **WOFF2 → bare sfnt**, for the reason `UI_FONT_FILES`'s doc comment
///   gives verbatim: `add_fonts` hands bytes to CoreText through font-kit,
///   and CoreText does not read the WOFF2 container.
/// * **Three static instances, not the variable face** — the same
///   `find_best_match`/`OS/2.usWeightClass` limit `UI_FONT_FILES` documents
///   for `CalSansUI`'s three weights. The three chosen are exactly the three
///   `theme.font_mono` text is ever painted at: `FontWeight::NORMAL` (every
///   plain run — `fps_overlay`'s " fps"/"max "/separators, `file_tree_row`'s
///   status letter, `inline_error`'s detail line), `FontWeight::MEDIUM`
///   (`fps_overlay`'s `drops > 0` run), and `FontWeight::BOLD`
///   (`fps_overlay`'s fps digit). No call site under `theme.font_mono` ever
///   requests a fourth weight.
///
///   **Deliberately not the separate `@fontsource/jetbrains-mono` static
///   package**, even though it ships pre-baked 400/500/700 cuts under its own
///   `files/`: that package's `@font-face` declares the family `'JetBrains
///   Mono'`, a name `--font-mono` never asks for (`editor-theme.css` imports
///   it only so plain `'JetBrains Mono'` stays selectable in Settings/
///   Terminal, alongside the Variable cut). `--font-mono`'s every weight —
///   400 from the unconditional `@import`, 500 and 700 from `font-weight`
///   requests the browser interpolates live — is shaped from the **variable**
///   file. Instancing from that same file, at the same three coordinates, is
///   the more faithful source, not merely the more convenient one.
/// * **The family is renamed** `JetBrains Mono` (nameID 1 in the upstream
///   file) **→ `JetBrains Mono Variable`**, for the same reason `UI_FONT_FILES`
///   renames `Cal Sans UI` → `CalSansUI`: font-kit's `MemSource` matches a
///   family by exact string, and the CSS token this item's family constant
///   documents is `'JetBrains Mono Variable'`. `licenses/
///   JetBrainsMono-OFL-1.1.txt` declares **no Reserved Font Name** (only the
///   license's own boilerplate *definition* of the term appears, the same
///   check `UI_FONT_FILES`'s doc comment already performed for `CalSans`), so
///   OFL-1.1 §3 places no restriction on a Modified Version's name.
///
/// `Medium.ttf`'s `OS/2.fsSelection` has its `REGULAR` bit cleared (`0x80`,
/// not the `Regular`/`Bold` instances' `0xC0`/`0xA0`) — the same "not
/// Regular, but not a distinct legacy style either" treatment
/// `CalSansUI-Medium.ttf` already carries, and for the same reason: the
/// upstream package's own pre-baked 500 cut (not used here — see above)
/// leaves that bit set, which this conversion does not inherit.
#[cfg(test)]
const UI_MONO_FONT_FILES: [&str; 3] = [
    "JetBrainsMonoVariable-Regular.ttf",
    "JetBrainsMonoVariable-Medium.ttf",
    "JetBrainsMonoVariable-Bold.ttf",
];

/// [`ui_font_paths`]'s mono sibling — test-only, see [`UI_MONO_FONT_BYTES`]
/// for what actually ships.
#[cfg(test)]
fn ui_mono_font_paths() -> Vec<PathBuf> {
    if let Ok(overridden) = std::env::var("CROWBAR_ROW_MONO_FONT") {
        return vec![PathBuf::from(overridden)];
    }
    let fonts = PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../assets/fonts");
    UI_MONO_FONT_FILES
        .iter()
        .map(|file| fonts.join(file))
        .collect()
}

/// [`UI_FONT_BYTES`]'s mono sibling — same embedding, same reasoning (see that
/// constant's own doc comment for why `include_bytes!` and not a runtime
/// path), a separate family.
const UI_MONO_FONT_BYTES: [&[u8]; 3] = [
    include_bytes!("../../../assets/fonts/JetBrainsMonoVariable-Regular.ttf"),
    include_bytes!("../../../assets/fonts/JetBrainsMonoVariable-Medium.ttf"),
    include_bytes!("../../../assets/fonts/JetBrainsMonoVariable-Bold.ttf"),
];

/// [`load_ui_font`]'s mono sibling — same registration, same "checked as well
/// as attempted" discipline, same `CROWBAR_ROW_MONO_FONT` override shape (one
/// file, read from disk, no rebuild required), a separate family.
fn load_ui_mono_font(cx: &mut App) -> String {
    let faces: Vec<Cow<'static, [u8]>> = match std::env::var("CROWBAR_ROW_MONO_FONT") {
        Ok(overridden) => match std::fs::read(&overridden) {
            Ok(bytes) => vec![Cow::Owned(bytes)],
            Err(err) => return format!("font: {overridden} not read ({err})"),
        },
        Err(_) => UI_MONO_FONT_BYTES
            .iter()
            .map(|bytes| Cow::Borrowed(*bytes))
            .collect(),
    };

    if let Err(err) = cx.text_system().add_fonts(faces) {
        return format!("font: {UI_MONO_FONT_FAMILY} rejected ({err})");
    }
    if !cx
        .text_system()
        .all_font_names()
        .iter()
        .any(|name| name == UI_MONO_FONT_FAMILY)
    {
        return format!("font: {UI_MONO_FONT_FAMILY} parsed but the family did not register");
    }
    format!("font: {UI_MONO_FONT_FAMILY} loaded")
}

/// The daemon round trip item 0.4 proved, kept alive as one caption line.
///
/// Pre-rendered to strings at probe time rather than held as a
/// `Result<Health, HealthError>`, because `Render` runs on every frame and
/// formatting is not the view's job.
struct Report {
    lines: Vec<SharedString>,
    /// The socket the probe dialled, kept so the shell can stream from the
    /// same daemon the caption describes. `None` when no home could be
    /// resolved at all, which is the one case where there is nothing to
    /// stream from.
    socket: Option<std::path::PathBuf>,
}

impl Report {
    /// Derives the socket, probes the daemon, and turns either outcome into
    /// displayable lines. A daemon that is down is a normal outcome here — it
    /// is shown, not panicked on.
    fn probe() -> Self {
        match crowbar_client::probe_daemon() {
            Ok(probe) => Self::from_probe(&probe),
            Err(err) => Self {
                lines: vec![
                    "no daemon socket could be derived".into(),
                    err.to_string().into(),
                ],
                socket: None,
            },
        }
    }

    /// The daemon this report probed, when one could be resolved.
    fn socket(&self) -> Option<&std::path::Path> {
        self.socket.as_deref()
    }

    fn from_probe(probe: &Probe) -> Self {
        let socket: SharedString = probe.socket.display().to_string().into();
        let mut lines = match &probe.result {
            Ok(health) => Self::describe(health),
            Err(err) => Self::describe_failure(err),
        };
        lines.push(socket);
        Self {
            lines,
            socket: Some(probe.socket.clone()),
        }
    }

    fn describe(health: &Health) -> Vec<SharedString> {
        vec![
            "daemon reached".into(),
            format!("pid {}", health.pid).into(),
            format!("status {}", health.status).into(),
            format!("version {}", health.version).into(),
        ]
    }

    fn describe_failure(err: &HealthError) -> Vec<SharedString> {
        vec!["daemon unreachable".into(), err.to_string().into()]
    }

    /// The verdict and its one-word reason, which is all a caption has room
    /// for.
    fn summary(&self) -> String {
        self.lines
            .iter()
            .take(2)
            .map(SharedString::to_string)
            .collect::<Vec<_>>()
            .join(" ")
    }
}

#[cfg(test)]
mod tests {
    use std::path::PathBuf;

    use crowbar_client::{Health, Probe};
    use gpui::SharedString;

    use super::{
        Cell, Report, RowSurface, UI_FONT_BYTES, UI_FONT_FAMILY, UI_MONO_FONT_BYTES,
        UI_MONO_FONT_FAMILY, ui_font_paths, ui_mono_font_paths, window_options,
    };

    /// Pins what the daemon probe reports, because the window itself cannot be
    /// read back: `screencapture` and the accessibility API are both
    /// permission-gated for an agent shell, so "what did it render" would
    /// otherwise be an unverifiable claim.
    #[test]
    fn a_reachable_daemon_renders_pid_status_and_version() {
        let report = Report::from_probe(&Probe {
            socket: PathBuf::from("/tmp/crowbar-6d4f21ce150add3c.sock"),
            result: Ok(Health {
                pid: 62909,
                status: "ok".to_owned(),
                version: "0.1.0".to_owned(),
            }),
        });

        assert_eq!(
            report.lines,
            vec![
                SharedString::from("daemon reached"),
                SharedString::from("pid 62909"),
                SharedString::from("status ok"),
                SharedString::from("version 0.1.0"),
                SharedString::from("/tmp/crowbar-6d4f21ce150add3c.sock"),
            ],
        );
        assert_eq!(report.summary(), "daemon reached pid 62909");
    }

    /// A down daemon is a rendered state, not a crash. The socket stays on the
    /// last line either way: when the probe fails it is the single most useful
    /// thing to show.
    #[test]
    fn an_unreachable_daemon_renders_the_reason_not_a_panic() {
        let socket = PathBuf::from("/tmp/crowbar-nothing-listening.sock");
        let err = crowbar_client::fetch_health(&socket).expect_err("nothing is bound there");
        let reason = err.to_string();

        let report = Report::from_probe(&Probe {
            socket: socket.clone(),
            result: Err(err),
        });

        assert_eq!(
            report.lines,
            vec![
                SharedString::from("daemon unreachable"),
                SharedString::from(reason),
                SharedString::from(socket.display().to_string()),
            ],
        );
    }

    #[test]
    fn the_window_is_titled_and_sized_to_the_cell() {
        let cell = Cell::default();
        let options = window_options(&cell);

        assert_eq!(
            options.titlebar.and_then(|titlebar| titlebar.title),
            Some(SharedString::new_static("Crowbar (native)")),
        );
        match options.window_bounds {
            Some(gpui::WindowBounds::Windowed(bounds)) => {
                assert_eq!(bounds.size, RowSurface::window_size(&cell));
            }
            other => panic!("expected a windowed bound, got {other:?}"),
        }
    }

    /// The vendored files sit at `native/assets/fonts/` (S0.7 moved them out of
    /// `web/public/fonts/`, so `native/` no longer needs `web/` to have text) —
    /// and all three faces are there: the badge's weight 500, and P3.25's
    /// weight 600. Canonicalized rather than compared as constructed, so the
    /// assertion is about where the file actually *resolves*, not about the
    /// literal `..` components [`ui_font_paths`] happened to join with.
    #[test]
    fn the_font_paths_default_into_native_assets() {
        let paths = ui_font_paths();

        assert_eq!(paths.len(), 3, "{paths:?}");
        for (path, want) in paths.iter().zip([
            "CalSansUI-Regular.ttf",
            "CalSansUI-Medium.ttf",
            "CalSansUI-SemiBold.ttf",
        ]) {
            let canonical = path
                .canonicalize()
                .unwrap_or_else(|err| panic!("{} does not resolve: {err}", path.display()));
            assert!(
                canonical.ends_with(format!("native/assets/fonts/{want}")),
                "{} did not resolve under native/assets/fonts/",
                canonical.display(),
            );
        }
    }

    /// The faces exist in the checkout and are what CoreText will accept: a
    /// bare sfnt, not the WOFF2 container that made the first gate run shape
    /// with a fallback while reporting the right family name.
    #[test]
    fn the_default_faces_are_present_and_are_not_woff2() {
        for path in ui_font_paths() {
            let bytes = std::fs::read(&path)
                .unwrap_or_else(|err| panic!("{} is missing: {err}", path.display()));
            assert_eq!(
                &bytes[..4],
                // TrueType outlines. `wOF2` here is the whole original defect.
                &[0x00, 0x01, 0x00, 0x00],
                "{} is not a bare sfnt",
                path.display(),
            );
        }
    }

    /// The declared family has to be the stylesheet's `@font-face` name, or the
    /// snapshot's `font.family` cannot compare equal to the DOM's.
    #[test]
    fn the_declared_family_is_the_themes_family() {
        assert_eq!(
            crowbar_ui::Theme::DARK.font_sans.primary(),
            Some(UI_FONT_FAMILY),
        );
    }

    /// And the *files* have to carry that same family, or `add_fonts` succeeds
    /// and the lookup still misses. Read straight out of the `name` table's
    /// records rather than trusted from the filename.
    #[test]
    fn the_faces_name_the_family_the_row_declares() {
        for path in ui_font_paths() {
            let bytes = std::fs::read(&path).expect("the face is in the checkout");
            assert!(
                sfnt_names(&bytes).any(|name| name == UI_FONT_FAMILY),
                "{} does not name the family {UI_FONT_FAMILY}",
                path.display(),
            );
        }
    }

    /// **What actually ships.** `UI_FONT_BYTES` is what [`load_ui_font`]
    /// registers by default; this proves those `include_bytes!` literals
    /// resolved to the exact same six — well, three, for the sans family —
    /// files [`ui_font_paths`] computes from `CARGO_MANIFEST_DIR`, so the
    /// embedded copy and the vendored-on-disk copy cannot silently diverge
    /// (a hand-edit to one `include_bytes!` path, or a stale file left behind
    /// by a future re-vendor, would fail here rather than ship silently).
    #[test]
    fn the_embedded_bytes_match_the_vendored_files() {
        for (embedded, path) in UI_FONT_BYTES.iter().zip(ui_font_paths()) {
            let disk = std::fs::read(&path)
                .unwrap_or_else(|err| panic!("{} is missing: {err}", path.display()));
            assert_eq!(
                *embedded,
                disk.as_slice(),
                "{} diverges from the bytes main.rs embeds",
                path.display(),
            );
        }
    }

    /// [`the_font_paths_default_into_native_assets`]'s mono sibling: the
    /// three weights `theme.font_mono` text is ever painted at.
    #[test]
    fn the_mono_font_paths_default_into_native_assets() {
        let paths = ui_mono_font_paths();

        assert_eq!(paths.len(), 3, "{paths:?}");
        for (path, want) in paths.iter().zip([
            "JetBrainsMonoVariable-Regular.ttf",
            "JetBrainsMonoVariable-Medium.ttf",
            "JetBrainsMonoVariable-Bold.ttf",
        ]) {
            let canonical = path
                .canonicalize()
                .unwrap_or_else(|err| panic!("{} does not resolve: {err}", path.display()));
            assert!(
                canonical.ends_with(format!("native/assets/fonts/{want}")),
                "{} did not resolve under native/assets/fonts/",
                canonical.display(),
            );
        }
    }

    /// [`the_embedded_bytes_match_the_vendored_files`]'s mono sibling.
    #[test]
    fn the_embedded_mono_bytes_match_the_vendored_files() {
        for (embedded, path) in UI_MONO_FONT_BYTES.iter().zip(ui_mono_font_paths()) {
            let disk = std::fs::read(&path)
                .unwrap_or_else(|err| panic!("{} is missing: {err}", path.display()));
            assert_eq!(
                *embedded,
                disk.as_slice(),
                "{} diverges from the bytes main.rs embeds",
                path.display(),
            );
        }
    }

    /// [`the_default_faces_are_present_and_are_not_woff2`]'s mono sibling.
    #[test]
    fn the_default_mono_faces_are_present_and_are_not_woff2() {
        for path in ui_mono_font_paths() {
            let bytes = std::fs::read(&path)
                .unwrap_or_else(|err| panic!("{} is missing: {err}", path.display()));
            assert_eq!(
                &bytes[..4],
                &[0x00, 0x01, 0x00, 0x00],
                "{} is not a bare sfnt",
                path.display(),
            );
        }
    }

    /// [`the_declared_family_is_the_themes_family`]'s mono sibling.
    #[test]
    fn the_declared_mono_family_is_the_themes_family() {
        assert_eq!(
            crowbar_ui::Theme::DARK.font_mono.primary(),
            Some(UI_MONO_FONT_FAMILY),
        );
    }

    /// [`the_faces_name_the_family_the_row_declares`]'s mono sibling.
    #[test]
    fn the_mono_faces_name_the_family_the_row_declares() {
        for path in ui_mono_font_paths() {
            let bytes = std::fs::read(&path).expect("the face is in the checkout");
            assert!(
                sfnt_names(&bytes).any(|name| name == UI_MONO_FONT_FAMILY),
                "{} does not name the family {UI_MONO_FONT_FAMILY}",
                path.display(),
            );
        }
    }

    /// Every string in an sfnt's `name` table, as UTF-8 where it can be.
    ///
    /// A deliberately small reader: enough to prove the family name is in the
    /// file, not enough to pretend to be a font library. Both encodings that
    /// matter appear — Windows records are UTF-16BE, Macintosh Roman records
    /// are one byte per character — so both are decoded.
    fn sfnt_names(bytes: &[u8]) -> impl Iterator<Item = String> + '_ {
        let be16 = |at: usize| u16::from_be_bytes([bytes[at], bytes[at + 1]]) as usize;
        let tables = be16(4);
        let name_table = (0..tables)
            .map(|ix| 12 + ix * 16)
            .find(|&at| &bytes[at..at + 4] == b"name")
            .map(|at| u32::from_be_bytes(bytes[at + 8..at + 12].try_into().unwrap()) as usize)
            .expect("an sfnt with no name table is not a font");

        let count = be16(name_table + 2);
        let strings = name_table + be16(name_table + 4);
        (0..count).map(move |ix| {
            let record = name_table + 6 + ix * 12;
            let (platform, length, offset) = (be16(record), be16(record + 8), be16(record + 10));
            let raw = &bytes[strings + offset..strings + offset + length];
            if platform == 1 {
                raw.iter().map(|&byte| byte as char).collect()
            } else {
                let wide: Vec<u16> = raw
                    .chunks_exact(2)
                    .map(|pair| u16::from_be_bytes([pair[0], pair[1]]))
                    .collect();
                String::from_utf16_lossy(&wide)
            }
        })
    }
}
