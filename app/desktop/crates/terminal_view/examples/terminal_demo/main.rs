#![allow(
    clippy::print_stderr,
    reason = "the demo reports startup failures on the terminal"
)]
#![allow(
    clippy::cast_possible_truncation,
    clippy::cast_sign_loss,
    reason = "window pixel sizes are small positive numbers"
)]
//! A real shell in a native terminal view, over a local PTY that stands in for the daemon.
//!
//! `cargo run -p terminal_view --example terminal_demo -- --mode dark --type "ls --color" --size 1000x640`

mod arguments;
mod local_pty;

use gpui::{App, AppContext, Application, Bounds, WindowBounds, WindowOptions, px, size};
use terminal_view::{TerminalLink, TerminalSettings, TerminalView};
use ui::Theme;

use crate::arguments::Arguments;
use crate::local_pty::spawn_local_pty;

fn main() {
    let arguments = Arguments::from_env();
    Application::new()
        .with_assets(ui::assets())
        .run(move |cx: &mut App| start(&arguments, cx));
}

fn start(
    arguments: &Arguments,
    cx: &mut App,
) {
    if let Err(error) = ui::init(cx) {
        eprintln!("ui could not start: {error}");
        return;
    }
    Theme::of(cx).with_mode(arguments.mode).install(cx);
    if let Err(error) = open_terminal_window(arguments, cx) {
        eprintln!("window could not open: {error}");
    }
    cx.activate(true);
}

fn open_terminal_window(
    arguments: &Arguments,
    cx: &mut App,
) -> Result<(), String> {
    let bounds = Bounds::centered(None, size(px(arguments.width), px(arguments.height)), cx);
    let options = WindowOptions {
        window_bounds: Some(WindowBounds::Windowed(bounds)),
        ..WindowOptions::default()
    };
    let launch = arguments.clone();
    cx.open_window(options, move |window, cx| open_session(launch, window, cx))
        .map(|_| ())
        .map_err(|error| error.to_string())
}

fn open_session(
    arguments: Arguments,
    window: &mut gpui::Window,
    cx: &mut App,
) -> gpui::Entity<TerminalView> {
    let (link, peer) = TerminalLink::open();
    if let Err(error) = spawn_local_pty(peer, arguments.width, arguments.height) {
        eprintln!("shell could not start: {error}");
    }
    let view = cx.new(|cx| attached_view(link, arguments.command, window, cx));
    view.update(cx, |view, _| view.focus(window));
    view
}

fn attached_view(
    link: TerminalLink,
    command: String,
    window: &mut gpui::Window,
    cx: &mut gpui::Context<'_, TerminalView>,
) -> TerminalView {
    let mut view = TerminalView::new(link, TerminalSettings::default(), window, cx);
    view.run_after_attach(command);
    view
}
