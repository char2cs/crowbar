//! Opens the settings modal over a fake backend; used to capture parity screenshots.
//!
//! Usage: `gallery [--theme crowbar|frosted] [--mode light|dark] [--page ID] [--query TEXT] [--size WxH]`.

mod arguments;
mod demo;

use std::cell::RefCell;
use std::rc::Rc;

use gpui::{App, AppContext as _, Application, Bounds, WindowBounds, WindowOptions, px, size};
use settings::default_schema;
use settingsview::{ModalConfig, SettingsModal};

use crate::arguments::Arguments;

#[derive(Debug, thiserror::Error)]
enum Failure {
    #[error("settings schema: {0}")]
    Schema(#[from] settings::Error),
    #[error("init: {0}")]
    Init(#[from] ui::Error),
    #[error("open window: {0}")]
    OpenWindow(String),
}

fn open(
    cx: &mut App,
    arguments: &Arguments,
) -> Result<(), Failure> {
    let (width, height) = arguments.size();
    let bounds = Bounds::centered(None, size(px(width), px(height)), cx);
    let options = WindowOptions {
        window_bounds: Some(WindowBounds::Windowed(bounds)),
        ..WindowOptions::default()
    };
    let schema = default_schema()?;
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
    .map_err(|error| Failure::OpenWindow(error.to_string()))
}

fn start(
    cx: &mut App,
    arguments: &Arguments,
) -> Result<(), Failure> {
    ui::init(cx)?;
    arguments.apply(cx);
    open(cx, arguments)?;
    cx.activate(true);
    Ok(())
}

fn launch(
    cx: &mut App,
    arguments: &Arguments,
    failure: &Rc<RefCell<Option<Failure>>>,
) {
    let Err(error) = start(cx, arguments) else {
        return;
    };
    *failure.borrow_mut() = Some(error);
    cx.quit();
}

fn main() -> Result<(), Failure> {
    let arguments = Arguments::from_env();
    let failure = Rc::new(RefCell::new(None));
    let slot = Rc::clone(&failure);
    Application::new()
        .with_assets(ui::assets())
        .run(move |cx: &mut App| launch(cx, &arguments, &slot));
    failure.take().map_or(Ok(()), Err)
}
