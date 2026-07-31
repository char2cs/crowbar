#![forbid(unsafe_code)]

//! `crowbar-app` — the Crowbar native binary.
//!
//! Two surfaces live here so far, and both of them are measurements:
//!
//! * **the Phase 1 gate** ([`row_surface`]) — one row of the git status panel,
//!   drawn in one cell of the §8.3 matrix, chosen by the command line. This is
//!   what a bare `cargo run -p crowbar-app` opens.
//! * **the driver's proving surface** (`driver_surface`, `--features driver`) —
//!   the fixture `crowbar-driver`'s extractor was built against.
//!
//! Item 0.4's daemon round trip survives as the caption's middle: the binary
//! still derives the socket, asks `GET /v0/health` and shows the answer, so the
//! transport does not quietly stop being exercised.
//!
//! **This binary never touches the socket itself.** §4.2 makes `crowbar-client`
//! the only crate that talks to the daemon, and that stays true from the first
//! commit: a `UnixStream` here would be a hole in the layering that later items
//! would build on.

use std::borrow::Cow;
use std::path::PathBuf;
use std::process::ExitCode;

use crowbar_client::{Health, HealthError, Probe};
use gpui::{App, AppContext as _, SharedString, TitlebarOptions, WindowOptions};

use row_surface::{Cell, ParseError, RowSurface};

/// The driver's proving surface (item P1.2). `--features driver` only, and even
/// then only when `CROWBAR_DRIVER_SNAPSHOT` asks for it: a driver build that is
/// *not* taking a measurement has to be the ordinary app with the driver
/// linked, because that is the configuration the oracle measures.
#[cfg(feature = "driver")]
mod driver_surface;
#[cfg(feature = "driver")]
mod row_snapshot;

/// The driver-backed [`crowbar_ui::components::AnchorSink`]. A driver build
/// emits through it; `row_layout.rs` measures through it under a plain
/// `cargo test`, which is why it is not behind the feature alone.
#[cfg(any(feature = "driver", test))]
mod driver_anchors;

#[cfg(test)]
mod row_layout;
mod row_surface;

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
            "crowbar-app: the git status row has no `{}` state in the React original — \
             this cell renders the resting row, so a comparison of it proves nothing.",
            flag.name(),
        );
    }
    eprintln!("crowbar-app: {}", cell.describe());

    // Blocking, before the window exists, and on purpose: the daemon is a local
    // unix socket a few hundred microseconds away and this is a single request.
    let report = Report::probe();

    gpui_platform::application().run(move |cx: &mut App| {
        gpui_component::init(cx);
        let fonts = load_ui_font(cx);
        // On stderr as well as in the caption: a parity run that is silently
        // shaping with a fallback face would produce `text_width` deltas with
        // no visible cause.
        eprintln!("crowbar-app: {fonts}");

        #[cfg(feature = "driver")]
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

/// Opens the gate surface, and — in a driver build asked for a snapshot —
/// emits one snapshot of the first frame and quits.
#[cfg(feature = "driver")]
fn open(cell: Cell, caption: String, cx: &mut App) -> gpui::Result<()> {
    let destination = row_snapshot::Destination::from_env();
    let registry = destination.is_some().then(|| crowbar_driver::install(cx));
    let measured = cell.clone();

    cx.open_window(window_options(&cell), move |window, cx| {
        if let (Some(registry), Some(destination)) = (registry, destination) {
            window.on_next_frame(move |_window, cx| {
                row_snapshot::report(&row_snapshot::emit(&registry, &measured, &destination));
                cx.quit();
            });
        }
        cx.new(|_| RowSurface::new(cell, Box::new(driver_anchors::DriverAnchors), caption))
    })?;
    cx.activate(true);
    Ok(())
}

/// Opens the gate surface. The shipping build carries no oracle code at all.
#[cfg(not(feature = "driver"))]
fn open(cell: Cell, caption: String, cx: &mut App) -> gpui::Result<()> {
    use crowbar_ui::components::Unanchored;

    cx.open_window(window_options(&cell), move |_window, cx| {
        cx.new(|_| RowSurface::new(cell, Box::new(Unanchored), caption))
    })?;
    cx.activate(true);
    Ok(())
}

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

/// The family the row declares, and therefore the family the snapshot reports.
///
/// It is the *stylesheet's* `@font-face` name (`web/src/styles/theme.css`), not
/// the name inside the original font file — see [`UI_FONT_FILES`].
const UI_FONT_FAMILY: &str = "CalSansUI";

/// The faces the React app renders the row with, converted for CoreText.
///
/// **Provenance, because these are derived files.** Both are produced from the
/// repo's own `web/public/fonts/CalSansUI.woff2` — which stays, and which the
/// web app still loads — by `fontTools`:
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
///   with its 400 default — and the badge's `font-weight: 500` would shape at
///   400 while the snapshot went on reporting the declared 500. Two instances
///   give `find_best_match` something real to choose between. `GEOM` is pinned
///   at its default because nothing in the app sets `font-variation-settings`.
/// * **The family is renamed `Cal Sans UI` → `CalSansUI`.** font-kit's
///   `MemSource` matches a family by exact name, and the name in the file is
///   `Cal Sans UI` while the stylesheet's `@font-face` declares `CalSansUI`.
///   Leaving them different means either the family never resolves (and the row
///   silently shapes with a fallback, which is the failure this whole comment
///   exists to prevent) or the row declares `Cal Sans UI` and every text
///   anchor reports a `font.family` the DOM will never produce.
///   `licenses/CalSans-OFL-1.1.txt` declares **no Reserved Font Name**, so
///   OFL-1.1 §3 places no restriction on a Modified Version's name.
const UI_FONT_FILES: [&str; 2] = ["CalSansUI-Regular.ttf", "CalSansUI-Medium.ttf"];

/// Where the row's faces live, unless `CROWBAR_ROW_FONT` says otherwise.
///
/// The override takes a **single** path — it exists so a parity run can point
/// the row at some other face to prove a suspected shaping difference, and one
/// file is enough for that.
fn ui_font_paths() -> Vec<PathBuf> {
    if let Ok(overridden) = std::env::var("CROWBAR_ROW_FONT") {
        return vec![PathBuf::from(overridden)];
    }
    let fonts = PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../../web/public/fonts");
    UI_FONT_FILES.iter().map(|file| fonts.join(file)).collect()
}

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
fn load_ui_font(cx: &mut App) -> String {
    let paths = ui_font_paths();
    let mut faces = Vec::with_capacity(paths.len());
    for path in &paths {
        match std::fs::read(path) {
            Ok(bytes) => faces.push(Cow::Owned(bytes)),
            Err(err) => return format!("font: {} not read ({err})", path.display()),
        }
    }

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

/// The daemon round trip item 0.4 proved, kept alive as one caption line.
///
/// Pre-rendered to strings at probe time rather than held as a
/// `Result<Health, HealthError>`, because `Render` runs on every frame and
/// formatting is not the view's job.
struct Report {
    lines: Vec<SharedString>,
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
            },
        }
    }

    fn from_probe(probe: &Probe) -> Self {
        let socket: SharedString = probe.socket.display().to_string().into();
        let mut lines = match &probe.result {
            Ok(health) => Self::describe(health),
            Err(err) => Self::describe_failure(err),
        };
        lines.push(socket);
        Self { lines }
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

    use super::{Cell, Report, RowSurface, UI_FONT_FAMILY, ui_font_paths, window_options};

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

    /// The defaults point at the React app's own font directory, so a checkout
    /// is the only setup a parity run needs — and both faces are there, because
    /// one of them is the badge's weight 500.
    #[test]
    fn the_font_paths_default_into_the_react_app() {
        let paths = ui_font_paths();

        assert_eq!(paths.len(), 2, "{paths:?}");
        assert!(
            paths[0].ends_with("web/public/fonts/CalSansUI-Regular.ttf"),
            "{}",
            paths[0].display(),
        );
        assert!(
            paths[1].ends_with("web/public/fonts/CalSansUI-Medium.ttf"),
            "{}",
            paths[1].display(),
        );
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
