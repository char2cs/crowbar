//! Shows the halt gate.
//!
//! Usage: `startupgallery [--theme crowbar|frosted] [--mode light|dark] [--size WxH]`.

mod phases;

use std::cell::RefCell;
use std::rc::Rc;

use gpui::{App, AppContext as _, Bounds, Context, WindowBounds, WindowOptions, px, size};
use startup::{Command, View};
use ui::{Theme, ThemeId, ThemeMode};

use crate::phases::halted;

const DEFAULT_SIZE: (f32, f32) = (1200.0, 800.0);

struct Actions {
    quits_on: Command,
}

impl startup::Actions for Actions {
    fn perform(
        &self,
        command: Command,
        cx: &mut App,
    ) {
        if command == self.quits_on {
            cx.quit();
        }
    }
}

fn flag(name: &str) -> Option<String> {
    let args: Vec<String> = std::env::args().collect();
    let position = args
        .iter()
        .position(|arg| arg.strip_prefix("--") == Some(name))?;
    args.get(position + 1).cloned()
}

fn parse_size(value: &str) -> Option<(f32, f32)> {
    let (width, height) = value.split_once('x')?;
    Some((width.parse().ok()?, height.parse().ok()?))
}

fn build(cx: &mut Context<'_, View>) -> View {
    let mut view = View::new(Rc::new(Actions {
        quits_on: Command::Quit,
    }));
    view.apply(halted(), cx);
    view
}

#[derive(Debug, thiserror::Error)]
enum Failure {
    #[error("cannot open the window")]
    OpenWindow,
    #[error("init: {0}")]
    Init(#[from] ui::Error),
}

type FailureSlot = Rc<RefCell<Option<Failure>>>;

fn open(
    cx: &mut App,
    failure: &FailureSlot,
) {
    let (width, height) = flag("size")
        .as_deref()
        .and_then(parse_size)
        .unwrap_or(DEFAULT_SIZE);
    let bounds = Bounds::centered(None, size(px(width), px(height)), cx);
    let options = WindowOptions {
        window_bounds: Some(WindowBounds::Windowed(bounds)),
        ..WindowOptions::default()
    };
    if cx.open_window(options, |_, cx| cx.new(build)).is_err() {
        *failure.borrow_mut() = Some(Failure::OpenWindow);
        cx.quit();
    }
}

fn main() -> Result<(), Failure> {
    let id = match flag("theme").as_deref() {
        Some("frosted") => ThemeId::Frosted,
        _ => ThemeId::Crowbar,
    };
    let mode = match flag("mode").as_deref() {
        Some("dark") => ThemeMode::Dark,
        _ => ThemeMode::Light,
    };
    let failure = FailureSlot::default();
    let slot = Rc::clone(&failure);
    gpui_platform::application()
        .with_assets(ui::assets())
        .run(move |cx: &mut App| start(cx, &Look { id, mode }, &slot));
    failure.take().map_or(Ok(()), Err)
}

struct Look {
    id: ThemeId,
    mode: ThemeMode,
}

fn start(
    cx: &mut App,
    look: &Look,
    failure: &FailureSlot,
) {
    if let Err(error) = ui::init(cx) {
        *failure.borrow_mut() = Some(Failure::from(error));
        cx.quit();
        return;
    }
    Theme::of(cx)
        .with_id(look.id)
        .with_mode(look.mode)
        .install(cx);
    open(cx, failure);
    cx.activate(true);
}
