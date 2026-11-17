//! Shows the sidebar over a stand-in content area; used to capture parity screenshots.
//!
//! Usage: `sidebar_gallery [--theme crowbar|frosted] [--mode light|dark] [--size WxH]
//! [--data reference|rich|loading|merge] [--panel chats|files|changes] [--width N] [--closed]
//! [--drag subject:target:before|after|into] [--toasts]`; `--data merge` opens a merge box with
//! uncommitted files.

mod arguments;
mod content;

use std::cell::RefCell;
use std::error::Error;
use std::rc::Rc;

use gpui::{
    App, AppContext as _, Application, Bounds, Context, Entity, WindowBounds, WindowOptions, px,
    size,
};
use sidebar::Sidebar;
use ui::{Surface, Theme};

use crate::arguments::Arguments;
use crate::content::Content;

fn show_toasts(
    sidebar: &mut Sidebar,
    arguments: &Arguments,
    cx: &mut Context<'_, Sidebar>,
) {
    for (content, lifetime) in arguments.toasts() {
        sidebar.toast(content, (None, lifetime), cx);
    }
}

fn preview(
    sidebar: &Entity<Sidebar>,
    arguments: &Arguments,
    cx: &mut App,
) {
    sidebar.update(cx, |this, cx| show_toasts(this, arguments, cx));
    let Some(drag) = arguments.drag() else {
        return;
    };
    sidebar.update(cx, |this, cx| this.preview_drag(drag, cx));
}

fn open(
    cx: &mut App,
    arguments: &Arguments,
) -> Result<(), Box<dyn Error>> {
    let (width, height) = arguments.size();
    let bounds = Bounds::centered(None, size(px(width), px(height)), cx);
    let options = WindowOptions {
        window_bounds: Some(WindowBounds::Windowed(bounds)),
        ..WindowOptions::default()
    };
    let data = Rc::new(arguments.data());
    let preferences = arguments.preferences();
    cx.open_window(options, |_, cx| {
        let background = Theme::of(cx).palette(Surface::Window).background();
        let content = cx.new(|_| Content { background }).into();
        let sidebar =
            cx.new(|cx| Sidebar::new(Rc::clone(&data) as _, data, content, (preferences, cx)));
        preview(&sidebar, arguments, cx);
        sidebar
    })?;
    Ok(())
}

fn start(
    cx: &mut App,
    arguments: &Arguments,
) -> Result<(), Box<dyn Error>> {
    ui::init(cx)?;
    arguments.apply(cx);
    open(cx, arguments)?;
    cx.activate(true);
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

fn main() -> Result<(), Box<dyn Error>> {
    let arguments = Arguments::from_env();
    let failure: Rc<RefCell<Option<Failure>>> = Rc::default();
    let report = Rc::clone(&failure);
    Application::new()
        .with_assets(ui::assets())
        .run(move |cx: &mut App| remember_failure(start(cx, &arguments), &report, cx));
    let error = failure.borrow_mut().take();
    error.map_or(Ok(()), Err)
}
