//! The driver's proving surface: a handful of anchored elements, one per field
//! type the v1 contract carries, drawn in a real window so the extractor is
//! exercised against real post-layout bounds.
//!
//! **This is not the Phase 1 gate component.** A later item builds that against
//! a real Crowbar surface; this one exists only to prove the extractor, so it
//! stays small and invents no styling beyond what it takes to make each field
//! non-default.
//!
//! **The colour literals have moved onto the tokens** (P1.5). This file was
//! written on a branch that had no `crowbar_ui::Theme` — the sealed token system
//! (§4.3 rule 3, §6.1) landed in parallel — so it carried `rgb(…)` literals and
//! failed `check-invariants.sh` rule 4 the moment the two met. Every one of them
//! now reads a token instead. The surface's *appearance* is not the point and
//! never was; what matters is that each field the v1 contract carries is
//! non-default, which the tokens satisfy as well as the literals did.
//!
//! Compiled only under `--features driver`.

use std::env;
use std::fs;
use std::io::{self, Write as _};
use std::path::PathBuf;

use crowbar_driver::{
    AnchorRegistry, Content, SurfaceState, Theme as SnapshotTheme, anchor, anchor_root, anchor_text,
};
use crowbar_ui::Theme;
use gpui::{
    App, AppContext as _, Context, IntoElement, ParentElement as _, Render, SharedString,
    Styled as _, TitlebarOptions, Window, WindowOptions, div, px, size,
};

/// The surface's name in the snapshot, and the anchor everything is relative to.
const SURFACE: &str = "driver-surface";
const ROOT: &str = "surface-root";

/// The width the surface is pinned to, in logical pixels. Also the snapshot's
/// `state.width`, so the two cannot drift.
const WIDTH_PX: u32 = 320;

/// The same width, as gpui takes it.
#[expect(
    clippy::cast_precision_loss,
    reason = "320 is exactly representable; the two spellings are one constant \
              so the surface and the matrix cell cannot drift apart"
)]
const WIDTH: f32 = WIDTH_PX as f32;

/// Where an emitted snapshot goes: a file, or stdout.
pub enum Destination {
    /// Write to this path.
    File(PathBuf),
    /// Write to stdout. Requested with `-`.
    Stdout,
}

/// A request to render the driver surface and emit one snapshot of it.
pub struct Request {
    destination: Destination,
}

impl Request {
    /// Reads the request out of the environment, if there is one.
    ///
    /// A driver build with `CROWBAR_DRIVER_SNAPSHOT` unset is the ordinary app
    /// with the driver linked — which is the configuration the oracle measures,
    /// so it has to stay reachable.
    pub fn from_env() -> Option<Self> {
        let raw = env::var("CROWBAR_DRIVER_SNAPSHOT").ok()?;
        Some(Self {
            destination: if raw == "-" {
                Destination::Stdout
            } else {
                Destination::File(PathBuf::from(raw))
            },
        })
    }
}

/// Opens a window on the driver surface, emits one snapshot of the first frame,
/// and quits.
///
/// Quitting is the point: this is a measurement, and a window left open after
/// it would leave the caller waiting on a UI nobody is driving.
pub fn run(request: Request, cx: &mut App) {
    let anchors = crowbar_driver::install(cx);

    let opened = cx.open_window(window_options(), |window, cx| {
        window.on_next_frame(move |_window, cx| {
            report(&emit(&anchors, &request.destination));
            cx.quit();
        });
        cx.new(|_| Surface)
    });

    if let Err(err) = opened {
        eprintln!("crowbar-app: could not open the driver window: {err}");
        cx.quit();
    }
}

fn window_options() -> WindowOptions {
    WindowOptions {
        titlebar: Some(TitlebarOptions {
            title: Some(SharedString::new_static("Crowbar driver surface")),
            ..TitlebarOptions::default()
        }),
        window_bounds: Some(gpui::WindowBounds::Windowed(gpui::Bounds {
            origin: gpui::point(px(0.0), px(0.0)),
            size: size(px(640.0), px(400.0)),
        })),
        ..WindowOptions::default()
    }
}

/// The §8.3 matrix cell this surface stands in. Fixed, because the surface is
/// fixed — a matrix over it would be measuring the driver, not a component.
///
/// The vocabulary is `ANCHORS.md` v1.1's and not a description: `overflow`
/// because the narrow row's string does not fit, `dark` because the palette
/// below is dark, no flags because nothing here has a state.
fn state() -> SurfaceState {
    SurfaceState::new(WIDTH_PX, SnapshotTheme::Dark, Content::Overflow, [])
}

/// Serialises the recorded frame and writes it where it was asked for.
fn emit(anchors: &AnchorRegistry, destination: &Destination) -> Result<PathBuf, String> {
    let snapshot = anchors
        .snapshot(SURFACE, state(), ROOT)
        .map_err(|err| err.to_string())?;
    let json = snapshot.to_json().map_err(|err| err.to_string())?;

    match destination {
        Destination::Stdout => {
            let mut out = io::stdout().lock();
            writeln!(out, "{json}").map_err(|err| err.to_string())?;
            out.flush().map_err(|err| err.to_string())?;
            Ok(PathBuf::from("-"))
        }
        Destination::File(path) => {
            fs::write(path, format!("{json}\n")).map_err(|err| err.to_string())?;
            Ok(path.clone())
        }
    }
}

/// Says on stderr what happened, so a failed emit is never a silent one.
fn report(outcome: &Result<PathBuf, String>) {
    match outcome {
        Ok(path) => eprintln!("crowbar-app: snapshot written to {}", path.display()),
        Err(err) => eprintln!("crowbar-app: no snapshot: {err}"),
    }
}

/// The proving surface.
struct Surface;

impl Render for Surface {
    fn render(&mut self, _window: &mut Window, _cx: &mut Context<Self>) -> impl IntoElement {
        let theme = Theme::DARK;
        // Offset from the window origin on purpose: with the root anchor at
        // {0, 0} the root-relative arithmetic would be a tautology, and a
        // snapshot that is only right at the origin proves nothing.
        div().pl(px(24.0)).pt(px(16.0)).child(anchor_root(
            ROOT,
            div()
                .w(px(WIDTH))
                .flex()
                .flex_col()
                .gap(px(8.0))
                .p(px(8.0))
                .bg(theme.background)
                .font_family("Helvetica")
                .text_size(px(13.0))
                .text_color(theme.foreground)
                // A box: background, radius, border.
                .child(anchor(
                    "panel",
                    div()
                        .h(px(28.0))
                        .rounded(px(4.0))
                        .border_1()
                        .border_color(theme.border)
                        .bg(theme.card)
                        .px(px(6.0))
                        .flex()
                        .items_center()
                        // A text run that fits.
                        .child(anchor_text("panel-label", "crowbar")),
                ))
                // A text run that does not fit, in a box narrow enough to
                // force the truncation `text_width` exists to catch.
                .child(anchor(
                    "narrow",
                    div()
                        .w(px(90.0))
                        .overflow_hidden()
                        .whitespace_nowrap()
                        .text_ellipsis()
                        .child(anchor_text(
                            "narrow-label",
                            "resolve-terminal-connection.ts",
                        )),
                ))
                // A nested child, so the root-relative arithmetic has two
                // levels to get right rather than one.
                .child(anchor(
                    "nested",
                    div().pl(px(12.0)).child(anchor(
                        "nested-dot",
                        div().w(px(8.0)).h(px(8.0)).rounded(px(4.0)).bg(theme.info),
                    )),
                )),
        ))
    }
}
