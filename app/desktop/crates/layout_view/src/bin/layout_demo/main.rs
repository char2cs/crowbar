#![allow(
    clippy::print_stderr,
    reason = "the demo reports startup failures on the terminal"
)]
#![allow(
    clippy::expect_used,
    reason = "the demo feeds itself static ids that are always valid"
)]
//! Shows the pane layout over a fake model: the panes, sashes, ring, handle and drop zones.
//!
//! Usage: `layout_demo [--scenario single|split|three] [--mode light|dark] [--size WxH]`. The
//! left column holds chat chips that can be dragged onto a pane; a pane's handle drags the pane.

mod arguments;
mod chip;
mod demo;
mod demo_source;
mod scenario;

use gpui::{App, AppContext as _, Application, Bounds, WindowBounds, WindowOptions, px, size};

use crate::arguments::Arguments;
use crate::demo::Demo;

fn open(
    cx: &mut App,
    arguments: &Arguments,
) {
    let (width, height) = arguments.size();
    let bounds = Bounds::centered(None, size(px(width), px(height)), cx);
    let options = WindowOptions {
        window_bounds: Some(WindowBounds::Windowed(bounds)),
        ..WindowOptions::default()
    };
    let scenario = arguments.scenario();
    if let Err(error) = cx.open_window(options, |_, cx| cx.new(|cx| Demo::new(&scenario, cx))) {
        eprintln!("layout_demo: open window: {error}");
        cx.quit();
    }
}

fn main() {
    let arguments = Arguments::from_env();
    Application::new()
        .with_assets(ui::assets())
        .run(move |cx: &mut App| start(cx, &arguments));
}

fn start(
    cx: &mut App,
    arguments: &Arguments,
) {
    if let Err(error) = ui::init(cx) {
        eprintln!("layout_demo: {error}");
        cx.quit();
        return;
    }
    arguments.apply(cx);
    open(cx, arguments);
    cx.activate(true);
}
