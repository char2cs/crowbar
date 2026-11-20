use gpui::{
    App, CursorStyle, ElementId, FontWeight, InteractiveElement, IntoElement, MouseButton,
    ParentElement, RenderOnce, SharedString, Styled, Window, div, px,
};
use protocol::PaneId;
use ui::{Icon, IconName, Surface, Theme};

use crate::tabbar::tab_actions::TabAction;
use crate::tabbar::tab_frame::HEIGHT;
use crate::tabbar::tab_state::TabState;

const PAD_X: f32 = 10.0;
const GAP: f32 = 6.0;
const GLYPH: f32 = 14.0;
const LABEL_SIZE: f32 = 13.0;
const TITLE_MAX: f32 = 160.0;

/// The chat's entry leading the tab bar when the group shows one view at a time. It never closes
/// and never moves, so it sits before the editor tabs.
#[derive(IntoElement)]
pub struct ChatTab {
    /// The chat pane.
    pub pane: PaneId,
    /// The chat's title.
    pub label: SharedString,
    /// The chat's glyph.
    pub icon: IconName,
    /// Whether the chat is the view shown.
    pub state: TabState,
    /// What a press does: show the chat.
    pub select: TabAction,
}

ui::opaque_debug!(ChatTab);

impl RenderOnce for ChatTab {
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        let (hover_fill, hover_ink) = (palette.sidebar_element_hover(), palette.foreground());
        let name = self.pane.as_str().to_owned();
        let select = self.select;
        let title = title_of(self.label);
        div()
            .id(ElementId::from(SharedString::from(format!(
                "chat-tab-{name}"
            ))))
            .debug_selector(move || format!("chat-tab-{name}"))
            .flex()
            .flex_none()
            .items_center()
            .gap(px(GAP))
            .h(px(HEIGHT))
            .px(px(PAD_X))
            .rounded(ui::radius::SM)
            .bg(self.state.fill(palette))
            .text_color(self.state.ink(palette))
            .text_size(px(LABEL_SIZE))
            .font_weight(FontWeight::MEDIUM)
            .whitespace_nowrap()
            .cursor(CursorStyle::PointingHand)
            .hover(move |tab| tab.bg(hover_fill).text_color(hover_ink))
            .on_mouse_down(MouseButton::Left, move |_, window, cx| {
                cx.stop_propagation();
                select(window, cx);
            })
            .child(
                Icon::new(self.icon)
                    .size(px(GLYPH))
                    .color(self.state.ink(palette)),
            )
            .child(title)
    }
}

fn title_of(label: SharedString) -> gpui::Div {
    div()
        .min_w_0()
        .max_w(px(TITLE_MAX))
        .overflow_hidden()
        .text_ellipsis()
        .child(label)
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::{Context, Modifiers, Render, TestAppContext};
    use ui::Motion;

    use super::*;

    struct Shown {
        presses: Rc<Cell<u32>>,
    }

    impl Render for Shown {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            let presses = Rc::clone(&self.presses);
            div().child(ChatTab {
                pane: PaneId::new("c1").expect("valid"),
                label: "Fix the build".into(),
                icon: IconName::ChatsCircle,
                state: TabState::Idle,
                select: Rc::new(move |_, _| presses.set(presses.get() + 1)),
            })
        }
    }

    #[gpui::test]
    fn a_press_shows_the_chat(cx: &mut TestAppContext) {
        cx.update(|cx| {
            cx.set_global(Theme::default());
            cx.set_global(Motion::default());
        });
        let presses = Rc::new(Cell::new(0));
        let counted = Rc::clone(&presses);
        let (_, cx) = cx.add_window_view(move |_, _| Shown { presses: counted });
        cx.run_until_parked();
        let tab = cx.debug_bounds("chat-tab-c1").unwrap_or_default();
        assert_eq!(tab.size.height, px(HEIGHT), "as tall as an editor tab");
        cx.simulate_click(tab.center(), Modifiers::none());
        assert_eq!(presses.get(), 1);
    }
}
