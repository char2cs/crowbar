//! The editable text of one row: a one-line input, or a two-row area for a list.

use gpui::{AnyElement, App, Entity, FocusHandle, Focusable as _, IntoElement as _, Window};
use ui::{TextArea, TextInput};

#[derive(Clone)]
pub(crate) enum Field {
    Line(Entity<TextInput>),
    Area(Entity<TextArea>),
}

impl Field {
    pub(crate) fn text(
        &self,
        cx: &App,
    ) -> String {
        match self {
            Self::Line(input) => input.read(cx).text().to_owned(),
            Self::Area(area) => area.read(cx).text().to_owned(),
        }
    }

    pub(crate) fn set_text(
        &self,
        text: &str,
        cx: &mut App,
    ) {
        match self {
            Self::Line(input) => input.update(cx, |input, cx| input.set_text(text, cx)),
            Self::Area(area) => area.update(cx, |area, cx| area.set_text(text, cx)),
        }
    }

    pub(crate) fn focus_handle(
        &self,
        cx: &App,
    ) -> FocusHandle {
        match self {
            Self::Line(input) => input.focus_handle(cx),
            Self::Area(area) => area.focus_handle(cx),
        }
    }

    pub(crate) fn select(
        &self,
        window: &mut Window,
        cx: &mut App,
    ) {
        let handle = self.focus_handle(cx);
        window.focus(&handle, cx);
        if let Self::Line(input) = self {
            input.update(cx, TextInput::select_all);
        }
    }

    pub(crate) fn element(&self) -> AnyElement {
        match self {
            Self::Line(input) => input.clone().into_any_element(),
            Self::Area(area) => area.clone().into_any_element(),
        }
    }
}

#[cfg(test)]
mod tests {
    use gpui::{AppContext as _, TestAppContext};

    use super::*;

    #[gpui::test]
    fn both_kinds_hold_and_replace_their_text(cx: &mut TestAppContext) {
        let line = cx.update(|cx| Field::Line(cx.new(|cx| TextInput::new("", cx))));
        let area = cx.update(|cx| Field::Area(TextArea::build("", cx)));
        for field in [line, area] {
            cx.update(|cx| field.set_text("a, b", cx));
            assert_eq!(cx.update(|cx| field.text(cx)), "a, b");
        }
    }
}
