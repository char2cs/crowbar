//! A real shell in a native terminal view, over a local PTY that stands in for the daemon.
//!
//! `cargo run -p terminalview --features gallery --bin terminalgallery -- --mode dark --type "ls --color" --size 1000x640`

mod arguments;
mod local_pty;

use gpui::{App, AppContext, Application, Bounds, WindowBounds, WindowOptions, px, size};
use terminalview::{TerminalLink, TerminalSettings, TerminalView};
use ui::Theme;

use crate::arguments::Arguments;
use crate::local_pty::spawn_local_pty;

fn main() {
    tracing_subscriber::fmt().init();
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
        tracing::error!(%error, "ui could not start");
        return;
    }
    Theme::of(cx).with_mode(arguments.mode).install(cx);
    if let Err(error) = open_terminal_window(arguments, cx) {
        tracing::error!(%error, "window could not open");
    }
    cx.activate(true);
}

fn open_terminal_window(
    arguments: &Arguments,
    cx: &mut App,
) -> gpui::Result<()> {
    let size = size(
        px(f32::from(arguments.width)),
        px(f32::from(arguments.height)),
    );
    let bounds = Bounds::centered(None, size, cx);
    let options = WindowOptions {
        window_bounds: Some(WindowBounds::Windowed(bounds)),
        ..WindowOptions::default()
    };
    let launch = arguments.clone();
    cx.open_window(options, move |window, cx| open_session(launch, window, cx))?;
    Ok(())
}

fn open_session(
    arguments: Arguments,
    window: &mut gpui::Window,
    cx: &mut App,
) -> gpui::Entity<TerminalView> {
    let (link, peer) = TerminalLink::open();
    if let Err(error) = spawn_local_pty(peer, arguments.width, arguments.height) {
        tracing::error!(%error, "shell could not start");
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
