//! Tooltips shown after a short hover delay.

use gpui::StatefulInteractiveElement as _;
use gpui::prelude::FluentBuilder;
use gpui::{
    AnyElement, App, ElementId, FocusHandle, InteractiveElement, IntoElement, ParentElement,
    Pixels, Point, RenderOnce, SharedString, Styled, Window, anchored, div, px,
};

use crate::display::Kbd;
use crate::floating::portal::Portal;
use crate::floating::tooltip_claim::TooltipClaim;
use crate::floating::tooltip_side::TooltipSide;
use crate::foundation::CubicBezier;
use crate::foundation::Motion;
use crate::foundation::Pose;
use crate::foundation::ScaledText;
use crate::foundation::Surface;
use crate::foundation::Theme;
use crate::foundation::Tween;
use crate::foundation::motion::adapters::clock as system_clock;
use crate::foundation::motion::hover;
use crate::foundation::opaque_debug;
use crate::foundation::tokens::layer;
use crate::foundation::tokens::opacity;
use crate::foundation::tokens::radius;
use crate::foundation::tokens::shadow;
use crate::foundation::tokens::spacing;
use crate::foundation::tokens::timing;

const WINDOW_MARGIN: f32 = 8.0;
const SIDE_OFFSET: f32 = 6.0;
const SLIDE: f32 = 4.0;
const BORDER_OPACITY: f32 = 0.7;

/// Wraps a trigger and shows `text` (and an optional shortcut) after the pointer rests on it for 150ms.
#[derive(IntoElement)]
pub struct Tooltip {
    id: ElementId,
    text: SharedString,
    shortcut: Option<SharedString>,
    side: TooltipSide,
    trigger: AnyElement,
    focus: Option<FocusHandle>,
}

impl Tooltip {
    /// A tooltip for `trigger`.
    pub fn new(
        id: impl Into<ElementId>,
        text: impl Into<SharedString>,
        trigger: impl IntoElement,
    ) -> Self {
        Self {
            id: id.into(),
            text: text.into(),
            shortcut: None,
            side: TooltipSide::default(),
            trigger: trigger.into_any_element(),
            focus: None,
        }
    }

    /// Adds a shortcut hint.
    #[must_use]
    pub fn shortcut(
        mut self,
        keys: impl Into<SharedString>,
    ) -> Self {
        self.shortcut = Some(keys.into());
        self
    }

    /// Also shows the card while `focus` holds keyboard focus that the keyboard put there, as a
    /// focused button's tooltip does in develop.
    #[must_use]
    pub fn focused_by(
        mut self,
        focus: &FocusHandle,
    ) -> Self {
        self.focus = Some(focus.clone());
        self
    }

    /// Chooses the side of the trigger.
    #[must_use]
    pub fn side(
        mut self,
        side: TooltipSide,
    ) -> Self {
        self.side = side;
        self
    }
}

fn card(
    (text, shortcut): (SharedString, Option<SharedString>),
    theme: Theme,
    opacity: f32,
) -> gpui::Div {
    let palette = theme.palette(Surface::Window);
    let edge = palette
        .card()
        .blend(palette.border().opacity(BORDER_OPACITY));
    div()
        .flex()
        .items_center()
        .gap(spacing::STEP_2)
        .px(spacing::STEP_2_5)
        .py(spacing::STEP_1_5)
        .rounded(radius::LG)
        .border_1()
        .border_color(edge)
        .bg(palette.card())
        .text_color(palette.foreground())
        .text_size(ScaledText::Sm.size(&theme))
        .font_family(theme.ui_family())
        .whitespace_nowrap()
        .shadow(shadow::lg(opacity::PERCENT_10))
        .opacity(opacity)
        .debug_selector(|| "tooltip-card".into())
        .child(text)
        .children(shortcut.map(Kbd::new))
}

impl Tooltip {
    fn placement(
        &self,
        slide: f32,
    ) -> gpui::Div {
        let placement = div().absolute().left_0().w_full().flex().justify_center();
        match self.side {
            TooltipSide::Bottom => placement.top_full().mt(px(SIDE_OFFSET + slide)),
            TooltipSide::Top => placement.bottom_full().mb(px(SIDE_OFFSET - slide)),
        }
    }
}

impl Tooltip {
    fn keyboard_focused(
        &self,
        window: &Window,
    ) -> bool {
        let focused = self
            .focus
            .as_ref()
            .is_some_and(|focus| focus.is_focused(window));
        focused && window.last_input_was_keyboard()
    }
}

impl RenderOnce for Tooltip {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let keyboard_focused = self.keyboard_focused(window);
        let hover_state = hover::track(&self.id, Pose::Off, window, cx);
        let entry =
            window.use_keyed_state((self.id.clone(), "entry"), cx, |_, _| None::<Point<Pixels>>);
        let tween = Tween::new(timing::DEFAULT, CubicBezier::EASE).delayed(timing::TOOLTIP_DELAY);
        let shown = hover_state
            .read(cx)
            .entered(tween, Motion::of(cx), system_clock::now());
        if shown.running {
            window.request_animation_frame();
        }
        let opacity = shown.value.max(f32::from(u8::from(keyboard_focused)));
        let id = self.id.clone();
        let placement = self
            .placement(SLIDE * (1.0 - opacity))
            .debug_selector(move || format!("tooltip-{id}"));
        let (text, shortcut) = (self.text, self.shortcut);
        let floating = Portal::here((self.id.clone(), "card"), move |_, _| {
            let card = card((text.clone(), shortcut.clone()), theme, opacity);
            div().w_full().flex().justify_center().child(
                anchored()
                    .snap_to_window_with_margin(px(WINDOW_MARGIN))
                    .child(card),
            )
        })
        .priority(layer::TOOLTIP);
        div()
            .id(self.id)
            .relative()
            .flex_none()
            .on_hover(note_entry(&hover_state, &entry))
            .on_mouse_move(arm(&hover_state, &entry))
            .capture_any_mouse_down(dismiss(&hover_state))
            .child(TooltipClaim::reset_per_move())
            .child(self.trigger)
            .when(opacity > 0.0, |d| d.child(placement.child(floating)))
    }
}

fn note_entry(
    state: &gpui::Entity<crate::foundation::ToggleState>,
    entry: &gpui::Entity<Option<Point<Pixels>>>,
) -> impl Fn(&bool, &mut Window, &mut App) + 'static {
    let (state, entry) = (state.clone(), entry.clone());
    move |hovered, window, cx| {
        let at = hovered.then(|| window.mouse_position());
        entry.update(cx, |entry, _| *entry = at);
        state.update(cx, |s, cx| s.set(Pose::Off, cx));
    }
}

fn arm(
    state: &gpui::Entity<crate::foundation::ToggleState>,
    entry: &gpui::Entity<Option<Point<Pixels>>>,
) -> impl Fn(&gpui::MouseMoveEvent, &mut Window, &mut App) + 'static {
    let (state, entry) = (state.clone(), entry.clone());
    move |event, _, cx| arm_on_move(&state, &entry, event.position, cx)
}

fn arm_on_move(
    state: &gpui::Entity<crate::foundation::ToggleState>,
    entry: &gpui::Entity<Option<Point<Pixels>>>,
    position: Point<Pixels>,
    cx: &mut App,
) {
    if !TooltipClaim::take(cx) {
        entry.update(cx, |entry, _| *entry = Some(position));
        state.update(cx, |s, cx| s.set(Pose::Off, cx));
        return;
    }
    if !entry.read(cx).is_some_and(|at| at != position) {
        return;
    }
    entry.update(cx, |entry, _| *entry = None);
    state.update(cx, |s, cx| s.set(Pose::On, cx));
}

fn dismiss(
    state: &gpui::Entity<crate::foundation::ToggleState>
) -> impl Fn(&gpui::MouseDownEvent, &mut Window, &mut App) + 'static {
    let state = state.clone();
    move |_, _, cx| state.update(cx, |s, cx| s.set(Pose::Off, cx))
}

opaque_debug!(Tooltip);

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, point, px};

    use super::*;
    use crate::control::Button;
    use crate::floating::Portal;
    use crate::foundation::IconName;
    use crate::test_support::probe::{measure, open, open_floating, settle};

    #[gpui::test]
    fn wrapping_does_not_change_the_trigger_size(cx: &mut TestAppContext) {
        let bare = measure(cx, |_, _| Button::icon_only("b", IconName::Gear));
        let wrapped = measure(cx, |_, _| {
            Tooltip::new("t", "Settings", Button::icon_only("b", IconName::Gear))
        });
        assert_eq!(bare.size, wrapped.size);
    }

    #[gpui::test]
    fn the_card_stays_inside_the_window_beside_a_trigger_at_its_edge(cx: &mut TestAppContext) {
        let cx = open(cx, |_, _| {
            Tooltip::new(
                "t",
                "Settings and more",
                Button::icon_only("b", IconName::Gear),
            )
        });
        cx.update(|_, cx| cx.set_global(Motion::reduced()));
        cx.simulate_mouse_move(point(px(10.0), px(10.0)), None, gpui::Modifiers::none());
        cx.run_until_parked();
        cx.simulate_mouse_move(point(px(11.0), px(11.0)), None, gpui::Modifiers::none());
        cx.run_until_parked();
        let card = cx.debug_bounds("tooltip-card");
        let trigger = cx.debug_bounds("probe").unwrap_or_default();
        let card = card.unwrap_or_default();
        assert!(f32::from(card.origin.x) >= WINDOW_MARGIN, "{card:?}");
        assert!(
            card.origin.y >= trigger.bottom(),
            "below the trigger: {card:?}"
        );
    }

    #[gpui::test]
    fn pressing_the_trigger_dismisses_the_card(cx: &mut TestAppContext) {
        let cx = open(cx, |_, _| {
            Tooltip::new("t", "Settings", Button::icon_only("b", IconName::Gear))
        });
        cx.update(|_, cx| cx.set_global(Motion::reduced()));
        cx.simulate_mouse_move(point(px(10.0), px(10.0)), None, gpui::Modifiers::none());
        cx.run_until_parked();
        cx.simulate_mouse_move(point(px(11.0), px(11.0)), None, gpui::Modifiers::none());
        cx.run_until_parked();
        assert!(cx.debug_bounds("tooltip-card").is_some(), "shown at rest");
        cx.simulate_mouse_down(
            point(px(11.0), px(11.0)),
            gpui::MouseButton::Right,
            gpui::Modifiers::none(),
        );
        cx.run_until_parked();
        assert!(
            cx.debug_bounds("tooltip-card").is_none(),
            "gone after a press"
        );
    }

    #[gpui::test]
    fn hover_gained_under_a_still_pointer_shows_no_card_until_it_moves(cx: &mut TestAppContext) {
        let cx = open(cx, |_, _| {
            Tooltip::new("t", "Settings", Button::icon_only("b", IconName::Gear))
        });
        cx.update(|_, cx| cx.set_global(Motion::reduced()));
        cx.simulate_mouse_move(point(px(10.0), px(10.0)), None, gpui::Modifiers::none());
        cx.run_until_parked();
        cx.simulate_mouse_move(point(px(10.0), px(10.0)), None, gpui::Modifiers::none());
        cx.run_until_parked();
        assert!(
            cx.debug_bounds("tooltip-card").is_none(),
            "the pointer has not moved"
        );
        cx.simulate_mouse_move(point(px(11.0), px(11.0)), None, gpui::Modifiers::none());
        cx.run_until_parked();
        assert!(
            cx.debug_bounds("tooltip-card").is_some(),
            "shown once it moves"
        );
    }

    #[gpui::test]
    fn a_tooltip_inside_another_shows_alone_under_the_pointer(cx: &mut TestAppContext) {
        let cx = open(cx, |_, _| {
            let close = Tooltip::new("inner", "Close", Button::icon_only("x", IconName::X));
            let tab = div().w(px(200.0)).flex().justify_end().child(close);
            Tooltip::new("outer", "Tab", tab)
        });
        cx.update(|_, cx| cx.set_global(Motion::reduced()));
        let x = cx.debug_bounds("probe").unwrap_or_default().right() - px(10.0);
        for nudge in [0.0, 1.0] {
            let at = point(x + px(nudge), px(10.0));
            cx.simulate_mouse_move(at, None, gpui::Modifiers::none());
            cx.run_until_parked();
        }
        assert!(
            cx.debug_bounds("tooltip-inner").is_some(),
            "the inner one shows"
        );
        assert!(
            cx.debug_bounds("tooltip-outer").is_none(),
            "the outer one stays shut"
        );
    }

    #[gpui::test]
    fn a_trigger_focused_from_the_keyboard_shows_its_card(cx: &mut TestAppContext) {
        let focus = cx.update(|cx| cx.focus_handle());
        let held = focus.clone();
        let cx = open(cx, move |_, _| {
            let gear = Button::icon_only("b", IconName::Gear).focus_handle(held.clone());
            Tooltip::new("t", "Settings", gear).focused_by(&held)
        });
        cx.update(|_, cx| cx.set_global(Motion::reduced()));
        cx.update(|window, cx| window.focus(&focus, cx));
        cx.run_until_parked();
        assert!(
            cx.debug_bounds("tooltip-card").is_none(),
            "no card before a key is pressed"
        );
        cx.simulate_keystrokes("shift");
        cx.run_until_parked();
        assert!(
            cx.debug_bounds("tooltip-card").is_some(),
            "shown while keyboard focus rests on it"
        );
    }

    #[gpui::test]
    fn a_key_typed_in_the_top_layer_shows_the_card_of_a_trigger_it_focuses(
        cx: &mut TestAppContext
    ) {
        let (gear, field) = cx.update(|cx| (cx.focus_handle(), cx.focus_handle()));
        let (held, inner) = (gear.clone(), field.clone());
        let (mut owner, mut top) = open_floating(cx, move |_, _| {
            let trigger = Button::icon_only("b", IconName::Gear).focus_handle(held.clone());
            let inner = inner.clone();
            let dialog = Portal::here("d", move |_, _| div().size(px(10.0)).track_focus(&inner));
            div()
                .child(Tooltip::new("t", "Settings", trigger).focused_by(&held))
                .child(dialog)
        });
        owner.update(|_, cx| cx.set_global(Motion::reduced()));
        top.update(|window, cx| window.focus(&field, cx));
        settle(&mut top);
        top.simulate_keystrokes("escape");
        owner.update(|window, cx| window.focus(&gear, cx));
        settle(&mut owner);
        settle(&mut top);
        assert!(
            top.debug_bounds("tooltip-card").is_some(),
            "the escape that left the dialog was a key for the window beneath"
        );
    }

    #[gpui::test]
    fn card_is_absent_until_the_pointer_rests(cx: &mut TestAppContext) {
        let cx = open(cx, |_, _| {
            Tooltip::new("t", "Settings", Button::icon_only("b", IconName::Gear))
                .side(TooltipSide::Top)
        });
        cx.simulate_mouse_move(point(px(10.0), px(10.0)), None, gpui::Modifiers::none());
        let before = cx.debug_bounds("probe").map(|b| b.size);
        assert_eq!(before.map(|s| f32::from(s.height)), Some(32.0));
    }

    #[gpui::test]
    fn a_resting_tooltip_floats_in_the_top_layer_and_lets_clicks_through(cx: &mut TestAppContext) {
        let (mut owner, mut top) = open_floating(cx, |_, _| {
            Tooltip::new("t", "Settings", Button::icon_only("b", IconName::Gear))
        });
        owner.update(|_, cx| cx.set_global(Motion::reduced()));
        owner.simulate_mouse_move(point(px(10.0), px(10.0)), None, gpui::Modifiers::none());
        owner.run_until_parked();
        owner.simulate_mouse_move(point(px(11.0), px(11.0)), None, gpui::Modifiers::none());
        owner.run_until_parked();
        assert_eq!(
            owner.debug_bounds("tooltip-card"),
            None,
            "nothing in the window"
        );
        let card = top.debug_bounds("tooltip-card").unwrap_or_default();
        let trigger = owner.debug_bounds("probe").unwrap_or_default();
        assert!(
            card.origin.y >= trigger.bottom(),
            "{card:?} below {trigger:?}"
        );
        assert!(top.update(|window, _| window.input_region().is_empty()));
    }

    #[gpui::test]
    fn without_a_compositor_the_whole_card_shows_behind_its_text(cx: &mut TestAppContext) {
        let (mut owner, mut top) = open_floating(cx, |_, _| {
            Tooltip::new("t", "Settings", Button::icon_only("b", IconName::Gear))
        });
        owner.update(|_, cx| cx.set_global(Motion::reduced()));
        owner.simulate_mouse_move(point(px(10.0), px(10.0)), None, gpui::Modifiers::none());
        owner.run_until_parked();
        owner.simulate_mouse_move(point(px(11.0), px(11.0)), None, gpui::Modifiers::none());
        owner.run_until_parked();
        let card = top.debug_bounds("tooltip-card").unwrap_or_default();
        let content = top.update(|window, _| window.content_region().to_vec());
        assert!(content.contains(&card), "{card:?} shows: {content:?}");
    }
}
