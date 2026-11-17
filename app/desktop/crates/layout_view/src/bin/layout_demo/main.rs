//! Shows the pane layout over a fake model: the panes, sashes, ring, handle and drop zones.
//!
//! Usage: `layout_demo [--scenario single|split|three] [--mode light|dark] [--size WxH]`. The
//! left column holds chat chips that can be dragged onto a pane; a pane's handle drags the pane.

mod arguments;
mod chip;
mod demo;
mod demo_source;
mod scenario;

use std::cell::RefCell;
use std::error::Error;
use std::rc::Rc;

use gpui::{App, AppContext as _, Application, Bounds, WindowBounds, WindowOptions, px, size};

use crate::arguments::Arguments;
use crate::demo::Demo;
use crate::scenario::DemoResult;

fn open(
    cx: &mut App,
    arguments: &Arguments,
) -> DemoResult<()> {
    let (width, height) = arguments.size();
    let bounds = Bounds::centered(None, size(px(width), px(height)), cx);
    let options = WindowOptions {
        window_bounds: Some(WindowBounds::Windowed(bounds)),
        ..WindowOptions::default()
    };
    let scenario = arguments.scenario();
    let content = (scenario::build(&scenario)?, scenario::chips()?);
    cx.open_window(options, |_, cx| {
        cx.new(|cx| Demo::new(content, cx))
    })?;
    Ok(())
}

type Failure = Box<dyn Error>;

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

fn main() -> DemoResult<()> {
    let arguments = Arguments::from_env();
    let failure: Rc<RefCell<Option<Failure>>> = Rc::default();
    let report = Rc::clone(&failure);
    Application::new()
        .with_assets(ui::assets())
        .run(move |cx: &mut App| remember_failure(start(cx, &arguments), &report, cx));
    let error = failure.borrow_mut().take();
    error.map_or(Ok(()), Err)
}

fn start(
    cx: &mut App,
    arguments: &Arguments,
) -> DemoResult<()> {
    ui::init(cx)?;
    arguments.apply(cx);
    open(cx, arguments)?;
    cx.activate(true);
    Ok(())
}
