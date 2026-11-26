use std::rc::Rc;

use gpui::{
    AnyElement, App, CursorStyle, ElementId, Hsla, InteractiveElement, IntoElement, MouseButton,
    ParentElement, StatefulInteractiveElement, Styled, Window, div, px,
};
use ui::{Icon, IconName, radius};

use crate::pill::shell_look::ShellLook;
use crate::strip_metrics::TAB_HEIGHT;

pub(crate) const CLOSE: f32 = 24.0;
const CLOSE_GLYPH: f32 = 12.0;
const CLOSE_INSET: f32 = 6.0;

pub(crate) type CloseAction = Rc<dyn Fn(&mut Window, &mut App)>;

type CloseInk = (Hsla, Hsla, Hsla);

pub(crate) fn close_of(
    id: &ElementId,
    (look, ink): (ShellLook, CloseInk),
    close: Option<CloseAction>,
) -> Option<AnyElement> {
    if let Some(close) = close {
        return Some(close_button(id, ink, close).into_any_element());
    }
    (look == ShellLook::Lifted).then(|| close_face(id, ink).into_any_element())
}

fn close_face(
    id: &ElementId,
    (_, muted, _): CloseInk,
) -> gpui::Stateful<gpui::Div> {
    div()
        .id(ElementId::from((id.clone(), "close")))
        .absolute()
        .right(px(CLOSE_INSET))
        .top(px((TAB_HEIGHT - CLOSE) / 2.0 - 1.0))
        .size(px(CLOSE))
        .flex()
        .items_center()
        .justify_center()
        .rounded(radius::FULL)
        .text_color(muted)
        .child(Icon::new(IconName::X).size(px(CLOSE_GLYPH)))
}

fn close_button(
    id: &ElementId,
    ink: CloseInk,
    close: CloseAction,
) -> impl IntoElement {
    let (strong, _, hover_fill) = ink;
    close_face(id, ink)
        .cursor(CursorStyle::PointingHand)
        .hover(move |style| style.bg(hover_fill).text_color(strong))
        .on_mouse_down(MouseButton::Left, |_, _, cx| cx.stop_propagation())
        .on_click(move |_, window, cx| close(window, cx))
}

#[cfg(test)]
mod tests {
    use gpui::black;

    use super::*;

    fn close_without_action(look: ShellLook) -> Option<AnyElement> {
        close_of(&"tab".into(), (look, (black(), black(), black())), None)
    }

    #[test]
    fn a_lifted_tab_shows_its_x_with_nothing_to_close_and_a_resting_one_does_not() {
        assert!(close_without_action(ShellLook::Lifted).is_some(), "lifted");
        assert!(close_without_action(ShellLook::Idle).is_none(), "idle");
        assert!(close_without_action(ShellLook::Active).is_none(), "active");
    }
}
