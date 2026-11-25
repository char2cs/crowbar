//! A panel of arbitrary content anchored to a trigger.

mod actions;
mod state;

pub(crate) use actions::bindings;

use std::rc::Rc;

use gpui::{
    AnyElement, App, ElementId, Entity, InteractiveElement, IntoElement, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, px,
};

use crate::floating::placement::Placement;
use crate::floating::popover::actions::{CONTEXT, ClosePopover};
use crate::floating::popover::state::State;
use crate::floating::popup;
use crate::foundation::Surface;
use crate::foundation::Theme;
use crate::foundation::opaque_debug;
use crate::foundation::tokens::radius;
use crate::foundation::tokens::shadow;

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
    placement: Placement,
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
            placement: Placement::default(),
        }
    }

    /// Opens the panel on `placement` of the trigger instead of below it.
    #[must_use]
    pub fn placement(
        mut self,
        placement: Placement,
    ) -> Self {
        self.placement = placement;
        self
    }
}

impl RenderOnce for Popover {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let state = window.use_keyed_state((self.id.clone(), "popover"), cx, |_, cx| State {
            open: false,
            focus: cx.focus_handle(),
        });
        let opener = state.clone();
        let id = self.id.clone();
        let wrapper = div()
            .id(self.id)
            .relative()
            .child(self.trigger)
            .on_click(move |_, window, cx| open(&opener, window, cx));
        if !state.read(cx).open {
            return wrapper.into_any_element();
        }
        let closer = state.clone();
        let content = self.content;
        let panel: popup::Content = Rc::new(move |window, cx| panel(&state, &content, window, cx));
        wrapper
            .child(popup::click_away((id.clone(), "away"), move |_, cx| {
                close(&closer, cx);
            }))
            .child(popup::placed(
                (id, "panel").into(),
                self.placement,
                OFFSET,
                panel,
            ))
            .into_any_element()
    }
}

fn panel(
    state: &Entity<State>,
    content: &Content,
    window: &mut Window,
    cx: &mut App,
) -> AnyElement {
    let theme = *Theme::of(cx);
    let palette = theme.palette(Surface::Window);
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
        .font_family(theme.ui_family())
        .shadow(shadow::lg(SHADOW_ALPHA))
        .child(content(window, cx))
        .into_any_element()
}

fn open(
    state: &Entity<State>,
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
    window.focus(&focus, cx);
}

fn close(
    state: &Entity<State>,
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
    use crate::control::Button;
    use crate::test_support::probe::{open, open_floating, settle};

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
    fn a_right_placement_opens_beside_the_trigger(cx: &mut TestAppContext) {
        let cx = open(cx, |window, cx| {
            build(window, cx).placement(Placement::Right)
        });
        cx.simulate_click(point(px(10.0), px(10.0)), gpui::Modifiers::none());
        cx.run_until_parked();
        let trigger = cx.debug_bounds("probe").unwrap_or_default();
        let body = cx.debug_bounds("popover-body").unwrap_or_default();
        assert!(
            body.origin.x >= trigger.right(),
            "{body:?} right of {trigger:?}"
        );
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

    #[gpui::test]
    fn an_open_popover_floats_in_the_top_layer_and_takes_the_keyboard(cx: &mut TestAppContext) {
        let (mut owner, mut top) = open_floating(cx, build);
        owner.simulate_click(point(px(10.0), px(10.0)), gpui::Modifiers::none());
        owner.run_until_parked();
        assert!(owner.debug_bounds("popover-body").is_none());
        assert!(top.debug_bounds("popover-body").is_some());
        assert!(top.update(|window, cx| window.focused(cx).is_some()));
        top.simulate_keystrokes("escape");
        settle(&mut top);
        assert!(top.debug_bounds("popover-body").is_none());
    }
}
