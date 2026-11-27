use gpui::{
    App, ClickEvent, ElementId, InteractiveElement, IntoElement, MouseButton, ParentElement,
    RenderOnce, SharedString, StatefulInteractiveElement, Styled, Window, div, px,
};
use ui::{Icon, IconName, Theme};

use crate::look::{BODY, DISABLED_OPACITY, PILL_PAD_X, PILL_PAD_Y, black, white};

const ARROW: f32 = 16.0;
const ARROW_GAP: f32 = 6.0;

type Handler = Box<dyn Fn(&ClickEvent, &mut Window, &mut App) + 'static>;

#[derive(IntoElement)]
pub(crate) struct PillButton {
    id: ElementId,
    label: SharedString,
    enabled: bool,
    on_click: Handler,
}

impl PillButton {
    pub(crate) fn new(
        id: impl Into<ElementId>,
        label: &'static str,
        on_click: impl Fn(&ClickEvent, &mut Window, &mut App) + 'static,
    ) -> Self {
        Self {
            id: id.into(),
            label: label.into(),
            enabled: true,
            on_click: Box::new(on_click),
        }
    }

    pub(crate) fn disabled(mut self) -> Self {
        self.enabled = false;
        self
    }
}

impl RenderOnce for PillButton {
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let ink = black(1.0);
        let pill = div()
            .flex()
            .flex_row()
            .items_center()
            .justify_center()
            .gap(px(ARROW_GAP))
            .rounded_full()
            .px(PILL_PAD_X)
            .py(PILL_PAD_Y)
            .bg(white(1.0))
            .text_color(ink)
            .text_size(BODY)
            .font_family(Theme::of(cx).ui_family())
            .child(self.label)
            .child(Icon::new(IconName::ArrowRight).size(px(ARROW)).color(ink))
            .on_mouse_down(MouseButton::Left, |_, window, _| window.prevent_default())
            .id(self.id);
        if !self.enabled {
            return pill.opacity(DISABLED_OPACITY);
        }
        pill.cursor_pointer().tab_stop(true).on_click(self.on_click)
    }
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::{Bounds, TestAppContext, point};

    use super::*;
    use crate::test_support::click_across;

    type Lock = fn(PillButton) -> PillButton;

    struct Host {
        pressed: Rc<Cell<u32>>,
        lock: Lock,
    }

    impl gpui::Render for Host {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut gpui::Context<'_, Self>,
        ) -> impl IntoElement {
            let pressed = Rc::clone(&self.pressed);
            let button =
                PillButton::new("pill", "Go", move |_, _, _| pressed.set(pressed.get() + 1));
            div().child((self.lock)(button))
        }
    }

    fn presses_with(
        cx: &mut TestAppContext,
        lock: Lock,
    ) -> u32 {
        let pressed = Rc::new(Cell::new(0));
        let counted = Rc::clone(&pressed);
        let (_, cx) = cx.add_window_view(move |_, _| Host {
            pressed: counted,
            lock,
        });
        cx.run_until_parked();
        let area = cx.update(|window, _| window.viewport_size());
        click_across(cx, Bounds::new(point(px(0.0), px(0.0)), area));
        pressed.get()
    }

    #[gpui::test]
    fn a_pill_answers_clicks_unless_it_is_disabled(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).expect("the design system installs"));
        let answered = presses_with(cx, std::convert::identity);
        let ignored = presses_with(cx, PillButton::disabled);
        assert_eq!((answered > 0, ignored), (true, 0), "enabled, then disabled");
    }
}
