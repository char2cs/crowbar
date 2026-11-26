//! The lifted pill that follows the pointer while a tab or chat is dragged.

use gpui::{
    App, Context, FontWeight, InteractiveElement, IntoElement, ParentElement, Render, SharedString,
    Styled, Window, div, px,
};

use crate::floating::Portal;

use crate::foundation::Surface;
use crate::foundation::Theme;
use crate::foundation::opaque_debug;
use crate::foundation::tokens::layer;
use crate::foundation::tokens::radius;
use crate::foundation::tokens::shadow;
use crate::foundation::tokens::text_size;

const HEIGHT: f32 = 32.0;
const SHADOW_ALPHA: f32 = 0.15;
const PADDING: f32 = 12.0;

/// A drag preview: the light "control surface" pill with a 1px outline and `shadow-md/15`,
/// painted from the light palette in every theme. Return it from `on_drag`.
pub struct DragGhost {
    label: SharedString,
    width: f32,
}

impl DragGhost {
    /// A ghost `width` logical pixels wide showing `label`.
    pub fn new(
        label: impl Into<SharedString>,
        width: f32,
    ) -> Self {
        Self {
            label: label.into(),
            width,
        }
    }
}

impl Render for DragGhost {
    fn render(
        &mut self,
        _window: &mut Window,
        _cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let (label, width) = (self.label.clone(), self.width);
        Portal::lifted("drag-ghost", move |_, cx| {
            crate::Shaped::seen(pill(label.clone(), width, cx), radius::FULL)
        })
        .priority(layer::TOOLTIP)
    }
}

fn pill(
    label: SharedString,
    width: f32,
    cx: &App,
) -> gpui::Div {
    let theme = *Theme::of(cx);
    let palette = theme.palette(Surface::Window);
    div()
        .w(px(width))
        .h(px(HEIGHT))
        .px(px(PADDING))
        .flex()
        .items_center()
        .overflow_hidden()
        .rounded(radius::FULL)
        .bg(palette.control_surface())
        .border_1()
        .border_color(palette.border())
        .shadow(shadow::md(SHADOW_ALPHA))
        .debug_selector(|| "drag-ghost".into())
        .text_color(palette.control_foreground())
        .font_family(theme.ui_family())
        .text_size(text_size::SM)
        .font_weight(FontWeight::MEDIUM)
        .child(label)
}

opaque_debug!(DragGhost);

#[cfg(test)]
mod tests {
    use gpui::{
        AppContext as _, InteractiveElement, MouseButton, StatefulInteractiveElement,
        TestAppContext, point, px,
    };

    use super::*;
    use crate::test_support::floats::windows;
    use crate::test_support::probe::{measure, open_floating};
    use crate::testing::covered;

    #[gpui::test]
    fn the_ghost_is_a_thirty_two_pixel_pill_of_the_requested_width(cx: &mut TestAppContext) {
        let bounds = measure(cx, |_, cx| pill("Untitled chat".into(), 160.0, cx));
        assert_eq!(bounds.size.width, px(160.0), "requested width");
        assert_eq!(bounds.size.height, px(32.0), "pill height");
    }

    #[derive(Clone)]
    struct Payload;

    fn lift(
        _: &Payload,
        _: gpui::Point<gpui::Pixels>,
        _: &mut Window,
        cx: &mut App,
    ) -> gpui::Entity<DragGhost> {
        cx.new(|_| DragGhost::new("Chat", 100.0))
    }

    #[gpui::test]
    fn the_ghost_follows_the_pointer_in_the_window_and_covers_all_of_it(cx: &mut TestAppContext) {
        let mut owner = open_floating(cx, |_, _| {
            div()
                .id("source")
                .w(px(50.0))
                .h(px(20.0))
                .on_drag(Payload, lift)
        });
        let modifiers = gpui::Modifiers::none();
        owner.simulate_mouse_down(point(px(10.0), px(10.0)), MouseButton::Left, modifiers);
        owner.simulate_mouse_move(point(px(120.0), px(90.0)), MouseButton::Left, modifiers);
        owner.simulate_mouse_move(point(px(150.0), px(100.0)), MouseButton::Left, modifiers);
        owner.run_until_parked();
        assert_eq!(windows(&mut owner), 1, "no window of its own");
        let ghost = owner.debug_bounds("drag-ghost").unwrap_or_default();
        assert_eq!(ghost.size.width, px(100.0), "{ghost:?}");
        assert_eq!(
            ghost.origin,
            point(px(30.0), px(10.0)),
            "moved with the pointer"
        );
        assert!(
            covered(&mut owner).0,
            "every web view is hidden during a drag"
        );
    }
}
