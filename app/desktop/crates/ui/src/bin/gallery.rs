#![allow(clippy::print_stderr, reason = "the gallery reports startup failures on the terminal")]
//! Renders every component of the `ui` crate in every state; used to capture parity screenshots.
//!
//! Usage: `gallery [--theme crowbar|frosted] [--mode light|dark] [--section NAME] [--size WxH]`.

mod arguments;
mod controls;
mod feedback;
mod icons;
mod overlays;
mod page;
mod structure;

use gpui::{App, Application, Bounds, WindowBounds, WindowOptions, px, size};

use crate::arguments::Arguments;
use crate::page::Page;

fn main() -> Result<(), ui::Error> {
    let arguments = Arguments::from_env();
    let application = Application::new().with_assets(ui::Assets);
    application.run(move |cx: &mut App| {
        if let Err(error) = ui::init(cx) {
            eprintln!("gallery: {error}");
            cx.quit();
            return;
        }
        arguments.apply(cx);
        let bounds = Bounds::centered(None, size(px(arguments.width()), px(arguments.height())), cx);
        let options = WindowOptions { window_bounds: Some(WindowBounds::Windowed(bounds)), ..WindowOptions::default() };
        let opened = cx.open_window(options, |window, cx| cx.new(|cx| Page::new(arguments.section(), window, cx)));
        if let Err(error) = opened {
            eprintln!("gallery: open window: {error}");
            cx.quit();
        }
        cx.activate(true);
    });
    Ok(())
}
