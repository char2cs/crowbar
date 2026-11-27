use gpui::{
    AnyElement, App, ClickEvent, ElementId, FocusHandle, InteractiveElement, IntoElement,
    MouseButton, ParentElement, Pixels, RenderOnce, SharedString, StatefulInteractiveElement,
    Styled, Window, div, prelude::FluentBuilder as _, px,
};
use ui::{HoverFill, Icon, IconName, Surface, Theme, Tooltip, focus_ring, radius};

use crate::column::header::tile_shape::TileShape;

const SIZE: f32 = 28.0;
const ICON: f32 = 16.0;

type ClickHandler = Box<dyn Fn(&ClickEvent, &mut Window, &mut App) + 'static>;

#[derive(IntoElement)]
pub(crate) struct IconButton {
    id: ElementId,
    icon: IconName,
    tooltip: Option<SharedString>,
    shape: TileShape,
    on_click: Option<ClickHandler>,
    focus: Option<FocusHandle>,
}

impl IconButton {
    pub(crate) fn new(
        id: impl Into<ElementId>,
        icon: IconName,
    ) -> Self {
        Self {
            id: id.into(),
            icon,
            tooltip: None,
            shape: TileShape::Small,
            on_click: None,
            focus: None,
        }
    }

    pub(crate) fn tooltip(
        mut self,
        text: impl Into<SharedString>,
    ) -> Self {
        self.tooltip = Some(text.into());
        self
    }

    pub(crate) fn focus(
        mut self,
        focus: &FocusHandle,
    ) -> Self {
        self.focus = Some(focus.clone());
        self
    }

    pub(crate) fn shape(
        mut self,
        shape: TileShape,
    ) -> Self {
        self.shape = shape;
        self
    }

    pub(crate) fn on_click(
        mut self,
        handler: impl Fn(&ClickEvent, &mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_click = Some(Box::new(handler));
        self
    }

    fn radius(&self) -> Pixels {
        match self.shape {
            TileShape::Small => radius::SM,
            TileShape::Tile => radius::MD,
        }
    }
}

impl RenderOnce for IconButton {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Chrome);
        let radius = self.radius();
        let base = div()
            .flex()
            .flex_none()
            .items_center()
            .justify_center()
            .size(px(SIZE))
            .rounded(radius)
            .cursor_pointer()
            .child(
                Icon::new(self.icon)
                    .size(px(ICON))
                    .color(palette.muted_foreground()),
            );
        let (id, ring, focus) = (self.id.clone(), focus_ring(palette), self.focus);
        let pressed = focus.clone();
        let rung = focus
            .as_ref()
            .is_some_and(|focus| ui::Modality::rings(focus, window, cx));
        let button = HoverFill::new(self.id, palette.sidebar_element_hover())
            .apply(base, (window, cx))
            .when_some(focus.as_ref(), InteractiveElement::track_focus)
            .when(rung, move |button| button.shadow(ring))
            .on_mouse_down(MouseButton::Left, move |_, window, cx| {
                window.prevent_default();
                cx.stop_propagation();
                take_keyboard(pressed.as_ref(), window, cx);
            });
        let button = match self.on_click {
            Some(handler) => button.on_click(handler),
            None => button,
        };
        let Some(text) = self.tooltip else {
            return button.into_any_element();
        };
        tipped(Tooltip::new((id, "tip"), text, button), focus.as_ref())
    }
}

fn tipped(
    tooltip: Tooltip,
    focus: Option<&FocusHandle>,
) -> AnyElement {
    match focus {
        Some(focus) => tooltip.focused_by(focus).into_any_element(),
        None => tooltip.into_any_element(),
    }
}

// PLATFORM: linux WebKitGTK focuses a pressed button, so develop's gear keeps the keyboard after a click.
fn take_keyboard(
    focus: Option<&FocusHandle>,
    window: &mut Window,
    cx: &mut App,
) {
    if let Some(focus) = focus {
        window.focus(focus, cx);
    }
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::{Context, IntoElement, Render, TestAppContext, Window, point, px};
    use ui::IconName;

    use super::*;

    struct Host {
        clicks: Rc<Cell<u32>>,
    }

    impl Render for Host {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            let clicks = Rc::clone(&self.clicks);
            IconButton::new("button", IconName::Plus)
                .on_click(move |_, _, _| clicks.set(clicks.get() + 1))
        }
    }

    struct Focusing {
        focus: FocusHandle,
    }

    impl Render for Focusing {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            IconButton::new("button", IconName::Gear).focus(&self.focus)
        }
    }

    #[gpui::test]
    fn a_press_puts_the_keyboard_on_a_focusable_button(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).expect("ui installs"));
        let (view, cx) = cx.add_window_view(|_, cx| Focusing {
            focus: cx.focus_handle(),
        });
        cx.simulate_click(point(px(4.0), px(4.0)), gpui::Modifiers::none());
        let focused = view.update_in(cx, |view, window, _| view.focus.is_focused(window));
        assert!(
            focused,
            "the button holds the keyboard, as WebKit's does after a click"
        );
    }

    #[gpui::test]
    fn a_click_runs_the_handler(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).expect("ui installs"));
        let clicks = Rc::new(Cell::new(0));
        let shared = Rc::clone(&clicks);
        let (_, cx) = cx.add_window_view(|_, _| Host { clicks: shared });
        cx.simulate_click(point(px(4.0), px(4.0)), gpui::Modifiers::none());
        assert_eq!(clicks.get(), 1);
    }

    #[test]
    fn the_tile_shape_rounds_more_than_the_small_one() {
        let small = IconButton::new("a", IconName::Plus);
        let tile = IconButton::new("b", IconName::Plus).shape(TileShape::Tile);
        assert!(tile.radius() > small.radius());
    }
}
