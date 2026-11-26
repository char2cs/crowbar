use chord::Primary;
use gpui::{
    App, CursorStyle, ElementId, InteractiveElement, IntoElement, MouseButton, ParentElement,
    RenderOnce, SharedString, StatefulInteractiveElement, Styled, Window, div, px,
};
use ui::{Icon, Surface, Theme, Tooltip, TooltipSide};

use crate::tabbar::tab::strip_tab::{HEIGHT, group_of};
use crate::tabbar::tab::tab_end::TabEnd;
use crate::tabbar::tab::tab_state::TabState;
use crate::tabbar::tab_actions::TabAction;

const CLOSE: f32 = 20.0;
const CLOSE_INSET: f32 = 6.0;
const HOVER_OPACITY: f32 = 0.6;

#[derive(IntoElement)]
pub(crate) struct CloseButton {
    pub(crate) name: String,
    pub(crate) state: TabState,
    pub(crate) end: TabEnd,
    pub(crate) action: TabAction,
}

impl RenderOnce for CloseButton {
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        let (rest, ink) = (
            self.end.rest_opacity(self.state),
            self.end.ink(self.state, palette),
        );
        let on_close = self.action;
        let id = format!("strip-close-{}", self.name);
        let button = div()
            .id(ElementId::from(SharedString::from(id.clone())))
            .size(px(CLOSE))
            .flex()
            .items_center()
            .justify_center()
            .rounded(ui::radius::MD)
            .cursor(CursorStyle::PointingHand)
            .opacity(rest)
            .group_hover(group_of(&self.name), |button| button.opacity(HOVER_OPACITY))
            .on_mouse_down(MouseButton::Left, |_, _, cx| cx.stop_propagation())
            .on_click(move |_, window, cx| {
                cx.stop_propagation();
                on_close(window, cx);
            })
            .child(
                Icon::new(self.end.icon())
                    .weight(self.end.weight())
                    .size(px(self.end.glyph()))
                    .color(ink),
            );
        div()
            .absolute()
            .right(px(CLOSE_INSET))
            .top(px((HEIGHT - CLOSE) / 2.0))
            .child(tipped(&id, self.end, button))
    }
}

fn tipped(
    id: &str,
    end: TabEnd,
    button: impl IntoElement,
) -> Tooltip {
    let tip = Tooltip::new(SharedString::from(format!("{id}-tip")), end.tip(), button)
        .side(TooltipSide::Bottom);
    let Some(chord) = end.chord() else {
        return tip;
    };
    tip.shortcut(chord.label(Primary::native()))
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::{Context, Modifiers, Render, TestAppContext, point};
    use ui::{Motion, Theme};

    use super::*;

    struct Host {
        presses: Rc<Cell<u32>>,
    }

    impl Render for Host {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            let presses = Rc::clone(&self.presses);
            let action: TabAction = Rc::new(move |_, _| presses.set(presses.get() + 1));
            let button = CloseButton {
                name: "a".into(),
                state: TabState::Active,
                end: TabEnd::Close,
                action,
            };
            div().relative().w(px(100.0)).h(px(HEIGHT)).child(button)
        }
    }

    #[gpui::test]
    fn the_button_acts_on_release_not_on_press(cx: &mut TestAppContext) {
        cx.update(|cx| {
            cx.set_global(Theme::default());
            cx.set_global(Motion::default());
        });
        let presses = Rc::new(Cell::new(0));
        let host = Rc::clone(&presses);
        let (_, cx) = cx.add_window_view(move |_, _| Host { presses: host });
        let at = point(px(100.0 - CLOSE_INSET - CLOSE / 2.0), px(HEIGHT / 2.0));
        cx.simulate_mouse_down(at, MouseButton::Left, Modifiers::none());
        assert_eq!(presses.get(), 0, "a press alone closes nothing");
        cx.simulate_mouse_up(at, MouseButton::Left, Modifiers::none());
        assert_eq!(presses.get(), 1, "the release closes");
    }
}
