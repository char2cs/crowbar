//! A panel of arbitrary content anchored below a trigger.

use std::rc::Rc;

use gpui::{
    AnyElement, App, ElementId, Entity, InteractiveElement, IntoElement, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, px,
};

use crate::font::UI_FAMILY;
use crate::opaque_debug::opaque_debug;
use crate::popover_actions::{CONTEXT, ClosePopover};
use crate::popover_state::PopoverState;
use crate::popup;
use crate::radius;
use crate::shadow;
use crate::surface::Surface;
use crate::theme::Theme;

const OFFSET: f32 = 4.0;
const PADDING: f32 = 16.0;
const SHADOW_ALPHA: f32 = 0.05;

type Content = Rc<dyn Fn(&mut Window, &mut App) -> AnyElement>;

/// Wraps a trigger; clicking it shows the content below, a press outside or `Escape` closes it.
#[derive(IntoElement)]
pub struct Popover {
    id: ElementId,
    trigger: AnyElement,
    content: Content,
}

impl Popover {
    /// A popover on `trigger` whose panel shows what `content` builds.
    pub fn new<E: IntoElement>(
        id: impl Into<ElementId>,
        trigger: impl IntoElement,
        content: impl Fn(&mut Window, &mut App) -> E + 'static,
    ) -> Self {
        Self {
            id: id.into(),
            trigger: trigger.into_any_element(),
            content: Rc::new(move |window, cx| content(window, cx).into_any_element()),
        }
    }
}

impl RenderOnce for Popover {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let state =
            window.use_keyed_state((self.id.clone(), "popover"), cx, |_, cx| PopoverState {
                open: false,
                focus: cx.focus_handle(),
            });
        let opener = state.clone();
        let wrapper = div()
            .id(self.id)
            .relative()
            .child(self.trigger)
            .on_click(move |_, window, cx| open(&opener, window, cx));
        if !state.read(cx).open {
            return wrapper.into_any_element();
        }
        let closer = state.clone();
        let panel = panel(&state, &self.content, window, cx);
        wrapper
            .child(popup::click_away(window, move |_, cx| close(&closer, cx)))
            .child(popup::below(OFFSET, panel))
            .into_any_element()
    }
}

fn panel(
    state: &Entity<PopoverState>,
    content: &Content,
    window: &mut Window,
    cx: &mut App,
) -> AnyElement {
    let palette = Theme::of(cx).palette(Surface::Window);
    let focus = state.read(cx).focus.clone();
    let closer = state.clone();
    div()
        .id("ui-popover")
        .key_context(CONTEXT)
        .track_focus(&focus)
        .on_action(move |_: &ClosePopover, _, cx| close(&closer, cx))
        .occlude()
        .p(px(PADDING))
        .rounded(radius::LG)
        .border_1()
        .border_color(palette.border())
        .bg(palette.popover())
        .text_color(palette.foreground())
        .font_family(UI_FAMILY)
        .shadow(shadow::lg(SHADOW_ALPHA))
        .child(content(window, cx))
        .into_any_element()
}

fn open(
    state: &Entity<PopoverState>,
    window: &mut Window,
    cx: &mut App,
) {
    if state.read(cx).open {
        return;
    }
    let focus = state.read(cx).focus.clone();
    state.update(cx, |state, cx| {
        state.open = true;
        cx.notify();
    });
    window.focus(&focus);
}

fn close(
    state: &Entity<PopoverState>,
    cx: &mut App,
) {
    state.update(cx, |state, cx| {
        state.open = false;
        cx.notify();
    });
}

opaque_debug!(Popover);

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, point, px};

    use super::*;
    use crate::button::Button;
    use crate::test_support::open;

    fn build(
        _: &mut Window,
        _: &mut App,
    ) -> Popover {
        Popover::new("pop", Button::new("trigger", "Open"), |_, _| {
            div()
                .id("popover-body")
                .debug_selector(|| "popover-body".into())
                .w(px(80.0))
                .h(px(20.0))
        })
    }

    #[gpui::test]
    fn clicking_the_trigger_shows_the_content_and_escape_hides_it(cx: &mut TestAppContext) {
        let cx = open(cx, build);
        assert!(cx.debug_bounds("popover-body").is_none());
        cx.simulate_click(point(px(10.0), px(10.0)), gpui::Modifiers::none());
        cx.run_until_parked();
        assert!(cx.debug_bounds("popover-body").is_some());
        cx.simulate_keystrokes("escape");
        assert!(cx.debug_bounds("popover-body").is_none());
    }

    #[gpui::test]
    fn pressing_outside_closes_it(cx: &mut TestAppContext) {
        let cx = open(cx, build);
        cx.simulate_click(point(px(10.0), px(10.0)), gpui::Modifiers::none());
        cx.run_until_parked();
        cx.simulate_click(point(px(700.0), px(500.0)), gpui::Modifiers::none());
        cx.run_until_parked();
        assert!(cx.debug_bounds("popover-body").is_none());
    }
}
