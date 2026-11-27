mod member_glyph;
mod member_kind;
mod member_view;
mod shell_look;
mod tab_shell;
mod tab_view;

pub use member_glyph::MemberGlyph;
pub use member_kind::MemberKind;
pub use member_view::MemberView;
pub(crate) use shell_look::ShellLook;
pub(crate) use tab_shell::TabShell;
pub use tab_view::TabView;

use std::rc::Rc;

use gpui::prelude::FluentBuilder as _;
use gpui::{
    AnyElement, App, CursorStyle, ElementId, Hsla, InteractiveElement, IntoElement, MouseButton,
    ParentElement, SharedString, StatefulInteractiveElement, Styled, Window, div, px,
};
use ui::{Icon, IconName, Palette, radius};

use crate::strip_metrics::TAB_HEIGHT;

const CLOSE: f32 = 24.0;
const CLOSE_GLYPH: f32 = 12.0;
const CLOSE_INSET: f32 = 6.0;

pub(crate) type CloseAction = Rc<dyn Fn(&mut Window, &mut App)>;

type CloseInk = (Hsla, Hsla, Hsla);

fn close_of(
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

const FADE_REACH: f32 = 34.0;
const FADE_SPAN: f32 = 10.0;
const FADE_ANGLE: f32 = 90.0;

fn fade_ends(
    look: ShellLook,
    palette: &Palette,
) -> (Hsla, Hsla) {
    let ground = palette.chrome_bg();
    match look {
        ShellLook::Idle => (ground, ground.blend(palette.sidebar_element_hover())),
        ShellLook::Active | ShellLook::Lifted => {
            (palette.control_surface(), palette.control_surface())
        },
        ShellLook::MergeTarget => {
            let nest = ground.blend(palette.sidebar_drop_nest());
            (nest, nest)
        },
    }
}

fn fade(
    group: &SharedString,
    (rest, hovered): (Hsla, Hsla),
) -> impl IntoElement {
    div()
        .absolute()
        .top_0()
        .right_0()
        .h_full()
        .w(px(FADE_REACH))
        .rounded_r(radius::FULL)
        .bg(fading_into(rest))
        .when(rest != hovered, |fade| {
            fade.group_hover(group.clone(), move |style| style.bg(fading_into(hovered)))
        })
}

fn fading_into(fill: Hsla) -> gpui::Background {
    gpui::linear_gradient(
        FADE_ANGLE,
        gpui::linear_color_stop(fill.opacity(0.0), 0.0),
        gpui::linear_color_stop(fill, FADE_SPAN / FADE_REACH),
    )
}

#[cfg(test)]
mod tests {
    use gpui::black;
    use ui::{Surface, Theme};

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

    #[test]
    fn the_fade_under_the_close_button_ends_in_the_fill_the_tab_shows_in_each_state() {
        let theme = Theme::default();
        let palette = theme.palette(Surface::Chrome);
        let ground = palette.chrome_bg();
        let hovered = ground.blend(palette.sidebar_element_hover());
        let raised = palette.control_surface();
        let nest = ground.blend(palette.sidebar_drop_nest());
        let cases = [
            (ShellLook::Idle, (ground, hovered)),
            (ShellLook::Active, (raised, raised)),
            (ShellLook::Lifted, (raised, raised)),
            (ShellLook::MergeTarget, (nest, nest)),
        ];
        for (look, expected) in cases {
            assert_eq!(fade_ends(look, palette), expected, "{look:?}");
        }
    }
}
