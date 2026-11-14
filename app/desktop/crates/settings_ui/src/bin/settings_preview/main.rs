#![allow(
    clippy::print_stderr,
    reason = "the preview reports startup failures on the terminal"
)]
//! Opens the settings modal over a fake backend; used to capture parity screenshots.
//!
//! Usage: `settings_preview [--theme crowbar|frosted] [--mode light|dark] [--page ID] [--query TEXT] [--size WxH]`.

mod arguments;
mod demo;

use std::rc::Rc;

use gpui::{App, AppContext as _, Application, Bounds, WindowBounds, WindowOptions, px, size};
use settings_ui::{ModalConfig, SettingsModal, default_schema};

use crate::arguments::Arguments;

fn open(cx: &mut App, arguments: &Arguments) -> Result<(), String> {
    let (width, height) = arguments.size();
    let bounds = Bounds::centered(None, size(px(width), px(height)), cx);
    let options = WindowOptions {
        window_bounds: Some(WindowBounds::Windowed(bounds)),
        ..WindowOptions::default()
    };
    let schema = default_schema().map_err(|error| error.to_string())?;
    let backend = Rc::new(demo::backend());
    let mut config =
        ModalConfig::new(schema, Rc::clone(&backend) as _, backend).on_close(|_, cx| cx.quit());
    if let Some(page) = arguments.page() {
        config = config.page(page);
    }
    let query = arguments.query().map(str::to_owned);
    cx.open_window(options, move |window, cx| {
        cx.new(|cx| SettingsModal::new(config, window, cx).with_query(query.as_deref(), cx))
    })
    .map(drop)
    .map_err(|error| error.to_string())
}

fn main() {
    let arguments = Arguments::from_env();
    Application::new()
        .with_assets(ui::Assets)
        .run(move |cx: &mut App| {
            if let Err(error) = ui::init(cx) {
                eprintln!("settings_preview: {error}");
                cx.quit();
                return;
            }
            arguments.apply(cx);
            if let Err(error) = open(cx, &arguments) {
                eprintln!("settings_preview: {error}");
                cx.quit();
                return;
            }
            cx.activate(true);
        });
}
