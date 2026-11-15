use std::rc::Rc;

use gpui::{
    App, AppContext as _, CursorStyle, ElementId, Hsla, InteractiveElement, IntoElement,
    MouseButton, ParentElement, RenderOnce, SharedString, StatefulInteractiveElement, Styled,
    Window, div, px,
};
use model::DragSubject;
use protocol::PaneId;
use ui::{DragGhost, Icon, IconName, IconWeight, Surface, Theme, Tooltip, shadow};

const WIDTH: f32 = 69.0;
const HEIGHT: f32 = 21.0;
const BUTTON: f32 = 12.0;
const GAP: f32 = 9.0;
const SIDE_PADDING: f32 = 7.5;
const TOP_PADDING: f32 = 5.0;
const BOTTOM_PADDING: f32 = 4.0;
const BOTTOM_RADIUS: f32 = 8.0;
const GRIP: f32 = 10.0;
const DETACH: f32 = 10.0;
const CLOSE: f32 = 8.0;
const GHOST_WIDTH: f32 = 160.0;

pub(crate) type HandleAction = Rc<dyn Fn(&mut Window, &mut App)>;

#[derive(IntoElement)]
pub(crate) struct PaneHandle {
    pub(crate) pane: PaneId,
    pub(crate) label: SharedString,
    pub(crate) opacity: f32,
    pub(crate) on_detach: HandleAction,
    pub(crate) on_close: HandleAction,
}

impl PaneHandle {
    pub(crate) const fn size() -> (f32, f32) {
        (WIDTH, HEIGHT)
    }
}

impl RenderOnce for PaneHandle {
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        let ink = palette.primary_foreground();
        let name = self.pane.as_str().to_owned();
        let grip = grip(self.pane, self.label, ink);
        div()
            .flex()
            .items_start()
            .gap(px(GAP))
            .px(px(SIDE_PADDING))
            .pt(px(TOP_PADDING))
            .pb(px(BOTTOM_PADDING))
            .rounded_b(px(BOTTOM_RADIUS))
            .bg(palette.primary())
            .shadow(shadow::handle())
            .opacity(self.opacity)
            .child(Tooltip::new(
                element_id(&name, "tip-grip"),
                "Drag this chat",
                grip,
            ))
            .child(button(
                &name,
                ("detach", "Open in its own tab"),
                (IconName::ArrowsOutSimple, DETACH, ink),
                self.on_detach,
            ))
            .child(button(
                &name,
                ("close", "Close this chat"),
                (IconName::X, CLOSE, ink),
                self.on_close,
            ))
    }
}

fn element_id(
    pane: &str,
    kind: &'static str,
) -> ElementId {
    ElementId::from((ElementId::from(SharedString::from(pane.to_owned())), kind))
}

fn grip(
    pane: PaneId,
    label: SharedString,
    ink: Hsla,
) -> impl IntoElement {
    div()
        .id(element_id(pane.as_str(), "handle-grip"))
        .size(px(BUTTON))
        .flex()
        .items_center()
        .justify_center()
        .cursor(CursorStyle::OpenHand)
        .on_drag(DragSubject::Pane(pane), move |_, _, _, cx| {
            cx.new(|_| DragGhost::new(label.clone(), GHOST_WIDTH))
        })
        .child(
            Icon::new(IconName::DotsSixVertical)
                .weight(IconWeight::Bold)
                .size(px(GRIP))
                .rotate(std::f32::consts::FRAC_PI_2)
                .color(ink),
        )
}

fn button(
    pane: &str,
    (kind, tip): (&'static str, &'static str),
    (icon, size, ink): (IconName, f32, Hsla),
    action: HandleAction,
) -> impl IntoElement {
    let face = div()
        .id(element_id(pane, kind))
        .size(px(BUTTON))
        .flex()
        .items_center()
        .justify_center()
        .cursor(CursorStyle::PointingHand)
        .on_mouse_down(MouseButton::Left, move |_, window, cx| action(window, cx))
        .child(Icon::new(icon).size(px(size)).color(ink));
    Tooltip::new(element_id(pane, tip), tip, face)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_handle_is_sixty_nine_by_twenty_one() {
        assert_eq!(
            PaneHandle::size(),
            (69.0, 21.0),
            "three 12px buttons, two 9px gaps, padding"
        );
    }

    #[test]
    fn the_buttons_and_gaps_add_up_to_the_width() {
        let inner = 3.0 * BUTTON + 2.0 * GAP;
        assert_eq!(
            inner + 2.0 * SIDE_PADDING,
            WIDTH,
            "inner width plus padding"
        );
        assert_eq!(
            BUTTON + TOP_PADDING + BOTTOM_PADDING,
            HEIGHT,
            "button height plus padding"
        );
    }
}
