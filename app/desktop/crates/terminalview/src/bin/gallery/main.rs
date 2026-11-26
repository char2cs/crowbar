//! A real shell in a native terminal view, over a local PTY that stands in for the daemon.
//!
//! `cargo run -p terminalview --features gallery --bin terminalgallery -- --mode dark --type "ls --color" --size 1000x640`

mod arguments;
mod local_pty;

use std::cell::RefCell;
use std::error::Error;
use std::rc::Rc;

use gpui::{App, AppContext, Bounds, WindowBounds, WindowOptions, px, size};
use terminalview::{Link, Settings, TerminalView};
use ui::Theme;

use crate::arguments::Arguments;
use crate::local_pty::spawn_local_pty;

type Failure = Box<dyn Error>;

fn main() -> Result<(), Failure> {
    tracing_subscriber::fmt().init();
    let arguments = Arguments::from_env();
    let failure: Rc<RefCell<Option<Failure>>> = Rc::default();
    let report = Rc::clone(&failure);
    gpui_platform::application()
        .with_assets(ui::assets())
        .run(move |cx: &mut App| remember_failure(start(&arguments, cx), &report, cx));
    let error = failure.borrow_mut().take();
    error.map_or(Ok(()), Err)
}

fn remember_failure(
    result: Result<(), Failure>,
    slot: &Rc<RefCell<Option<Failure>>>,
    cx: &mut App,
) {
    let Err(error) = result else {
        return;
    };
    *slot.borrow_mut() = Some(error);
    cx.quit();
}

fn start(
    arguments: &Arguments,
    cx: &mut App,
) -> Result<(), Failure> {
    ui::init(cx)?;
    Theme::of(cx).with_mode(arguments.mode).install(cx);
    open_terminal_window(arguments, cx)?;
    cx.activate(true);
    Ok(())
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
    let (link, peer) = Link::open();
    if let Err(error) = spawn_local_pty(peer, arguments.width, arguments.height) {
        tracing::error!(%error, "shell could not start");
    }
    let view = cx.new(|cx| attached_view(link, arguments.command, window, cx));
    view.update(cx, |view, cx| view.focus(window, cx));
    view
}

fn attached_view(
    link: Link,
    command: String,
    window: &mut gpui::Window,
    cx: &mut gpui::Context<'_, TerminalView>,
) -> TerminalView {
    let mut view = TerminalView::new(link, Settings::default(), window, cx);
    view.run_after_attach(command);
    view
}
