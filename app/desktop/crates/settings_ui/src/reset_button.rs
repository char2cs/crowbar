//! The reset icon of a row whose value differs from its default.

use gpui::{
    App, InteractiveElement, IntoElement, ParentElement, StatefulInteractiveElement, Styled, div,
    px,
};
use ui::{Icon, IconName, Surface, Theme, Tooltip, radius};

use crate::element_id::element_id;
use crate::frame::Frame;
use crate::intent::Intent;
use crate::row::Row;

const BOX: f32 = 20.0;
const ICON: f32 = 16.0;

pub(crate) fn reset_button(
    frame: &Frame<'_>,
    row: &Row,
    cx: &App,
) -> impl IntoElement {
    let slot = div().flex_none().size(px(BOX));
    if !frame.state.resettable(row) {
        return slot.into_any_element();
    }
    let palette = Theme::of(cx).palette(Surface::Window);
    let id = element_id("reset", row.key.as_str());
    let button = div()
        .id(id.clone())
        .flex()
        .items_center()
        .justify_center()
        .size(px(BOX))
        .rounded(radius::MD)
        .cursor_pointer()
        .hover(move |style| style.bg(palette.accent()))
        .on_click(frame.sink.send(Intent::Reset(row.clone())))
        .child(
            Icon::new(IconName::ArrowCounterClockwise)
                .size(px(ICON))
                .color(palette.muted_foreground()),
        );
    slot.child(Tooltip::new(id, format!("Reset {}", row.label), button))
        .into_any_element()
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::intent::Intent;
    use crate::key::Key;
    use crate::settings_backend::SettingsBackend;
    use crate::test_support::{inspect, open, row, run};
    use crate::value::Value;

    #[gpui::test]
    fn reset_writes_the_default_back_and_the_row_stops_being_resettable(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "appearance");
        let target = row("sidebarFollowsFocus");
        assert!(!inspect(&harness, cx, |modal| modal
            .state
            .resettable(&target)));
        run(&harness, Intent::Toggle(target.clone()), cx);
        run(&harness, Intent::Reset(target.clone()), cx);
        assert_eq!(
            harness.backend.get(&Key::new("sidebarFollowsFocus")),
            Some(Value::Bool(false))
        );
        assert!(!inspect(&harness, cx, |modal| modal
            .state
            .resettable(&target)));
    }
}
