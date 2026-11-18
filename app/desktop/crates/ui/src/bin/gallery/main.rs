//! Renders every component of the `ui` crate in every state; used to capture parity screenshots.
//!
//! Usage: `gallery [--theme crowbar|frosted] [--mode light|dark] [--section NAME] [--size WxH]`.

mod arguments;
mod page;

use std::cell::RefCell;
use std::rc::Rc;

use gpui::{App, AppContext as _, Application, Bounds, WindowBounds, WindowOptions, px, size};

use arguments::Arguments;
use page::Page;

type FailureSlot = Rc<RefCell<Option<Failure>>>;

#[derive(Debug, thiserror::Error)]
enum Failure {
    #[error("open window: {0}")]
    OpenWindow(String),
    #[error("init: {0}")]
    Init(#[from] ui::Error),
}

fn open(
    cx: &mut App,
    arguments: &Arguments,
    failure: &FailureSlot,
) {
    let (width, height) = arguments.size();
    let bounds = Bounds::centered(None, size(px(width), px(height)), cx);
    let options = WindowOptions {
        window_bounds: Some(WindowBounds::Windowed(bounds)),
        ..WindowOptions::default()
    };
    let only = arguments.section();
    if let Err(error) = cx.open_window(options, |_, cx| cx.new(|cx| Page::new(only, cx))) {
        *failure.borrow_mut() = Some(Failure::OpenWindow(error.to_string()));
        cx.quit();
    }
}

fn start(
    cx: &mut App,
    arguments: &Arguments,
    failure: &FailureSlot,
) {
    if let Err(error) = ui::init(cx) {
        *failure.borrow_mut() = Some(Failure::from(error));
        cx.quit();
        return;
    }
    arguments.apply(cx);
    open(cx, arguments, failure);
    cx.activate(true);
}

fn main() -> Result<(), Failure> {
    let arguments = Arguments::from_env();
    let failure = FailureSlot::default();
    let slot = Rc::clone(&failure);
    Application::new()
        .with_assets(ui::assets())
        .run(move |cx: &mut App| start(cx, &arguments, &slot));
    failure.take().map_or(Ok(()), Err)
}
