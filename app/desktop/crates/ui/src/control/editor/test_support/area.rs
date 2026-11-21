//! A focused text area in a test window.

use std::cell::Cell;
use std::rc::Rc;

use gpui::{
    AppContext as _, Entity, ParentElement as _, Render, Styled as _, TestAppContext,
    VisualTestContext, div, px,
};

use crate::control::editor::test_support::drawn::Drawn;
use crate::control::editor::text_area::TextArea;
use crate::foundation::Motion;
use crate::foundation::Theme;

struct Host {
    area: Entity<TextArea>,
    draws: Rc<Cell<usize>>,
}

impl Render for Host {
    fn render(
        &mut self,
        _: &mut gpui::Window,
        _: &mut gpui::Context<'_, Self>,
    ) -> impl gpui::IntoElement {
        self.draws.set(self.draws.get() + 1);
        div().w(px(300.0)).child(self.area.clone())
    }
}

/// Opens a window holding a focused 300px wide [`TextArea`] with the default bindings installed.
pub(crate) fn area(cx: &mut TestAppContext) -> (Entity<TextArea>, &mut VisualTestContext) {
    area_with(cx, |area| area)
}

/// Like [`area`], with the field configured by `build`.
pub(crate) fn area_with(
    cx: &mut TestAppContext,
    build: impl FnOnce(TextArea) -> TextArea + 'static,
) -> (Entity<TextArea>, &mut VisualTestContext) {
    let (area, _, cx) = hosted(cx, build);
    (area, cx)
}

/// Like [`area`], in the active window, with the number of times the window was drawn.
pub(crate) fn drawn_area(cx: &mut TestAppContext) -> Drawn<'_, TextArea> {
    let (area, draws, cx) = hosted(cx, |area| area);
    cx.update(|window, _| window.activate_window());
    cx.run_until_parked();
    (area, draws, cx)
}

fn hosted(
    cx: &mut TestAppContext,
    build: impl FnOnce(TextArea) -> TextArea + 'static,
) -> Drawn<'_, TextArea> {
    cx.update(|cx| {
        cx.set_global(Theme::default());
        cx.set_global(Motion::default());
        cx.bind_keys(crate::control::editor::text_area::bindings());
    });
    let draws = Rc::new(Cell::new(0));
    let counter = Rc::clone(&draws);
    let (host, cx) = cx.add_window_view(|_, cx| Host {
        area: cx.new(|cx| build(TextArea::new("Write something", cx))),
        draws: counter,
    });
    let area = host.read_with(cx, |host, _| host.area.clone());
    cx.update(|window, cx| area.update(cx, |field, cx| field.focus(window, cx)));
    (area, draws, cx)
}

/// The text currently in `area`.
pub(crate) fn area_text(
    area: &Entity<TextArea>,
    cx: &mut VisualTestContext,
) -> String {
    area.read_with(cx, |area, _| area.text().to_owned())
}
