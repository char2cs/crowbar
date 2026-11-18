//! A focused text area in a test window.

use gpui::{
    AppContext as _, Entity, ParentElement as _, Render, Styled as _, TestAppContext,
    VisualTestContext, div, px,
};

use crate::control::editor::text_area::TextArea;
use crate::foundation::Motion;
use crate::foundation::Theme;

struct Host {
    area: Entity<TextArea>,
}

impl Render for Host {
    fn render(
        &mut self,
        _: &mut gpui::Window,
        _: &mut gpui::Context<'_, Self>,
    ) -> impl gpui::IntoElement {
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
    cx.update(|cx| {
        cx.set_global(Theme::default());
        cx.set_global(Motion::default());
        cx.bind_keys(crate::control::editor::text_area::bindings());
    });
    let (host, cx) = cx.add_window_view(|_, cx| Host {
        area: cx.new(|cx| build(TextArea::new("Write something", cx))),
    });
    let area = host.read_with(cx, |host, _| host.area.clone());
    cx.update(|window, cx| area.read(cx).focus(window));
    (area, cx)
}

/// The text currently in `area`.
pub(crate) fn area_text(
    area: &Entity<TextArea>,
    cx: &mut VisualTestContext,
) -> String {
    area.read_with(cx, |area, _| area.text().to_owned())
}
