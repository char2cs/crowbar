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
        cx.new(|_| RowSurface::new(cell, Box::new(row_snapshot::DriverAnchors), caption))
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

/// Where the React app's UI font lives, unless `CROWBAR_ROW_FONT` says
/// otherwise.
fn ui_font_path() -> PathBuf {
    std::env::var("CROWBAR_ROW_FONT").map_or_else(
        |_| {
            PathBuf::from(env!("CARGO_MANIFEST_DIR"))
                .join("../../../web/public/fonts/CalSansUI.woff2")
        },
        PathBuf::from,
    )
}

/// Tries to give gpui the same font the React app renders with, and says
/// plainly whether it worked.
///
/// **This is a parity dependency, not a nicety.** `text_width` is the field the
/// gate was chosen for, and it is a property of the shaped face: the two apps
/// can agree on every box and still disagree on where the ellipsis lands if
/// they are shaping with different fonts. The face the React app uses is
/// `CalSansUI`, distributed as **WOFF2** — a web container format that gpui
/// hands straight to CoreText, which does not read it. The attempt is made
/// anyway, because the answer belongs in the caption rather than in a guess,
/// and because pointing `CROWBAR_ROW_FONT` at a TTF of the same face then makes
/// it work with no code change.
fn load_ui_font(cx: &mut App) -> String {
    let path = ui_font_path();
    let Ok(bytes) = std::fs::read(&path) else {
        return format!("font: {} not found", path.display());
    };
    match cx.text_system().add_fonts(vec![Cow::Owned(bytes)]) {
        Ok(()) => "font: CalSansUI loaded".to_owned(),
        Err(err) => format!("font: CalSansUI rejected ({err})"),
    }
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

    use super::{Cell, Report, RowSurface, ui_font_path, window_options};

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

    /// The default points at the React app's own font file, so a checkout is
    /// the only setup a parity run needs.
    #[test]
    fn the_font_path_defaults_into_the_react_app() {
        let path = ui_font_path();

        assert!(
            path.ends_with("web/public/fonts/CalSansUI.woff2"),
            "{}",
            path.display(),
        );
    }
}
