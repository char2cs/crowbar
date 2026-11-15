#![allow(
    clippy::print_stderr,
    reason = "the gallery reports startup failures on the terminal"
)]
//! Renders every component of the `ui` crate in every state; used to capture parity screenshots.
//!
//! Usage: `gallery [--theme crowbar|frosted] [--mode light|dark] [--section NAME] [--size WxH]`.

mod arguments;
mod buttons;
mod feedback;
mod icons;
mod inputs;
mod overlays;
mod page;
mod section;
mod structure;
mod toggles;

use gpui::{App, AppContext as _, Application, Bounds, WindowBounds, WindowOptions, px, size};

use crate::arguments::Arguments;
use crate::page::Page;

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
    let only = arguments.section();
    if let Err(error) = cx.open_window(options, |_, cx| cx.new(|cx| Page::new(only, cx))) {
        eprintln!("gallery: open window: {error}");
        cx.quit();
    }
}

fn start(
    cx: &mut App,
    arguments: &Arguments,
) {
    if let Err(error) = ui::init(cx) {
        eprintln!("gallery: {error}");
        cx.quit();
        return;
    }
    arguments.apply(cx);
    open(cx, arguments);
    cx.activate(true);
}

fn main() {
    let arguments = Arguments::from_env();
    Application::new()
        .with_assets(ui::Assets)
        .run(move |cx: &mut App| start(cx, &arguments));
}
