#![allow(
    clippy::print_stderr,
    reason = "the gallery reports startup failures on the terminal"
)]
//! Shows the sidebar over a stand-in content area; used to capture parity screenshots.
//!
//! Usage: `sidebar_gallery [--theme crowbar|frosted] [--mode light|dark] [--size WxH]
//! [--data reference|rich|loading] [--panel chats|files|changes] [--width N] [--closed]`.

mod arguments;
mod content;

use std::rc::Rc;

use gpui::{App, AppContext as _, Application, Bounds, WindowBounds, WindowOptions, px, size};
use sidebar::Sidebar;
use ui::{Surface, Theme};

use crate::arguments::Arguments;
use crate::content::Content;

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
    let data = Rc::new(arguments.data());
    let preferences = arguments.preferences();
    let opened = cx.open_window(options, |_, cx| {
        let background = Theme::of(cx).palette(Surface::Window).background();
        let content = cx.new(|_| Content { background }).into();
        cx.new(|cx| Sidebar::new(Rc::clone(&data) as _, data, content, (preferences, cx)))
    });
    if let Err(error) = opened {
        eprintln!("sidebar_gallery: open window: {error}");
        cx.quit();
    }
}

fn start(
    cx: &mut App,
    arguments: &Arguments,
) {
    if let Err(error) = ui::init(cx) {
        eprintln!("sidebar_gallery: {error}");
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
        .with_assets(ui::assets())
        .run(move |cx: &mut App| start(cx, &arguments));
}
