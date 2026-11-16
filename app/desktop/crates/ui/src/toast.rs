//! The toast card.

use gpui::prelude::FluentBuilder as _;
use gpui::{
    Animation, AnimationExt, App, Div, ElementId, Hsla, IntoElement, ParentElement, RenderOnce,
    SharedString, Styled, Window, div, px, relative,
};

use crate::close_handler::CloseHandler;
use crate::cubic_bezier::CubicBezier;
use crate::icon::Icon;
use crate::icon_name::IconName;
use crate::icon_weight::IconWeight;
use crate::keyframes::Keyframes;
use crate::motion::Motion;
use crate::opacity;
use crate::opaque_debug::opaque_debug;
use crate::palette::Palette;
use crate::radius;
use crate::shadow;
use crate::spacing;
use crate::spinner::Spinner;
use crate::surface::Surface;
use crate::text_size;
use crate::theme::Theme;
use crate::timing;
use crate::toast_close::ToastClose;
use crate::toast_kind::ToastKind;
use crate::toast_phase::ToastPhase;

const CLOSE_CLEARANCE: f32 = 24.0;

/// A message card: icon by tone and text.
///
/// An updated error shakes and an updated success pulses; the card fades in when it first appears and
/// out when it is [`Toast::leaving`]. With [`Toast::on_close`] it shows the corner close button.
#[derive(IntoElement)]
pub struct Toast {
    id: ElementId,
    kind: ToastKind,
    message: SharedString,
    updates: u32,
    on_close: Option<CloseHandler>,
    fill: bool,
    phase: ToastPhase,
}

fn shake_offset(
    kind: ToastKind,
    updates: u32,
    delta: f32,
) -> f32 {
    if kind != ToastKind::Error || updates == 0 {
        return 0.0;
    }
    Keyframes::TOAST_ERROR.sample(delta)
}

fn pulse_scale(
    kind: ToastKind,
    updates: u32,
    delta: f32,
) -> f32 {
    if kind != ToastKind::Success || updates == 0 {
        return 1.0;
    }
    Keyframes::TOAST_SUCCESS.sample(delta)
}

fn fade(
    phase: ToastPhase,
    delta: f32,
) -> f32 {
    match phase {
        ToastPhase::Shown => delta,
        ToastPhase::Leaving => 1.0 - delta,
    }
}

impl Toast {
    /// A toast of `kind` showing `message`.
    pub fn new(
        id: impl Into<ElementId>,
        kind: ToastKind,
        message: impl Into<SharedString>,
    ) -> Self {
        Self {
            id: id.into(),
            kind,
            message: message.into(),
            updates: 0,
            on_close: None,
            fill: false,
            phase: ToastPhase::Shown,
        }
    }

    /// Marks the toast as updated `updates` times; an updated error shakes again, a success pulses.
    #[must_use]
    pub fn updated(
        mut self,
        updates: u32,
    ) -> Self {
        self.updates = updates;
        self
    }

    /// Shows the close button; `handler` runs when it is clicked.
    #[must_use]
    pub fn on_close(
        mut self,
        handler: CloseHandler,
    ) -> Self {
        self.on_close = Some(handler);
        self
    }

    /// Stretches the card to the width of its container instead of fitting its text.
    #[must_use]
    pub fn full_width(mut self) -> Self {
        self.fill = true;
        self
    }

    /// Fades the card out instead of in; the owner removes it once the fade is done.
    #[must_use]
    pub fn leaving(mut self) -> Self {
        self.phase = ToastPhase::Leaving;
        self
    }
}

fn tone(
    kind: ToastKind,
    palette: &Palette,
) -> (IconName, Hsla) {
    match kind {
        ToastKind::Info => (IconName::Info, palette.info()),
        ToastKind::Success => (IconName::CheckCircle, palette.success()),
        ToastKind::Warning => (IconName::Warning, palette.warning()),
        ToastKind::Error => (IconName::WarningCircle, palette.destructive()),
        ToastKind::Loading => (IconName::CircleNotch, palette.muted_foreground()),
    }
}

impl Toast {
    fn glyph(
        &self,
        palette: &Palette,
    ) -> gpui::AnyElement {
        let (name, color) = tone(self.kind, palette);
        match self.kind {
            ToastKind::Loading => Spinner::new((self.id.clone(), "spin")).into_any_element(),
            _ => Icon::new(name)
                .weight(IconWeight::Fill)
                .color(color)
                .into_any_element(),
        }
    }

    fn frame(
        &self,
        theme: &Theme,
    ) -> Div {
        let palette = theme.palette(Surface::Window);
        div()
            .relative()
            .when(self.fill, Styled::w_full)
            .px(spacing::STEP_3_5)
            .py(spacing::STEP_3)
            .rounded(radius::LG)
            .border_1()
            .border_color(palette.border())
            .bg(palette.popover())
            .text_color(palette.foreground())
            .text_size(text_size::SM)
            .font_family(theme.ui_family())
            .shadow(shadow::lg(opacity::PERCENT_5))
    }
}

impl RenderOnce for Toast {
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let closable = self.on_close.is_some();
        let body = div()
            .flex()
            .items_start()
            .gap(spacing::STEP_2)
            .min_w_0()
            .when(closable, |body| body.pr(px(CLOSE_CLEARANCE)))
            .child(self.glyph(theme.palette(Surface::Window)))
            .child(div().min_w_0().child(self.message.clone()));
        let close = self
            .on_close
            .clone()
            .map(|handler| ToastClose::new((self.id.clone(), "close"), handler));
        let card = self.frame(&theme).child(body).children(close);
        let motion = Motion::of(cx);
        let (kind, updates, phase) = (self.kind, self.updates, self.phase);
        let replayed = card.with_animation(
            (self.id.clone(), format!("replay-{updates}")),
            Animation::new(motion.duration(timing::TOAST_ERROR.max(timing::TOAST_SUCCESS))),
            move |card, delta| settle(card, kind, updates, delta),
        );
        div()
            .when(self.fill, Styled::w_full)
            .child(replayed)
            .with_animation(
                (self.id, format!("fade-{phase:?}")),
                motion.animation(timing::SLOW, CubicBezier::DEFAULT),
                move |wrapper, delta| wrapper.opacity(fade(phase, delta)),
            )
    }
}

fn settle(
    card: Div,
    kind: ToastKind,
    updates: u32,
    delta: f32,
) -> Div {
    let shift = shake_offset(kind, updates, delta);
    let grow = (pulse_scale(kind, updates, delta) - 1.0) / 2.0;
    match kind {
        ToastKind::Error => card.ml(px(shift.max(0.0))).mr(px((-shift).max(0.0))),
        ToastKind::Success => card.mx(relative(-grow)),
        ToastKind::Info | ToastKind::Warning | ToastKind::Loading => card,
    }
}

opaque_debug!(Toast);

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use gpui::TestAppContext;

    use super::*;
    use crate::test_support::measure;

    #[test]
    fn only_updated_errors_shake_and_only_updated_successes_pulse() {
        type Case = (ToastKind, u32, f32, (f32, f32));
        const CASES: &[Case] = &[
            (ToastKind::Error, 0, 0.25, (0.0, 1.0)),
            (ToastKind::Error, 1, 0.25, (-3.0, 1.0)),
            (ToastKind::Success, 1, 0.3, (0.0, 1.025)),
            (ToastKind::Success, 1, 0.6, (0.0, 0.99)),
            (ToastKind::Success, 0, 0.3, (0.0, 1.0)),
            (ToastKind::Info, 1, 0.3, (0.0, 1.0)),
        ];
        for &(kind, updates, delta, (shake, pulse)) in CASES {
            let moved = shake_offset(kind, updates, delta);
            assert!(
                (moved - shake).abs() < 1e-3,
                "{kind:?} {updates} shake {moved}"
            );
            let scaled = pulse_scale(kind, updates, delta);
            assert!(
                (scaled - pulse).abs() < 1e-3,
                "{kind:?} {updates} pulse {scaled}"
            );
        }
    }

    #[test]
    fn fade_runs_in_for_a_new_toast_and_out_for_a_leaving_one() {
        const CASES: &[(ToastPhase, f32, f32)] = &[
            (ToastPhase::Shown, 0.0, 0.0),
            (ToastPhase::Shown, 1.0, 1.0),
            (ToastPhase::Leaving, 0.0, 1.0),
            (ToastPhase::Leaving, 1.0, 0.0),
        ];
        for &(phase, delta, opacity) in CASES {
            assert!(
                (fade(phase, delta) - opacity).abs() < 1e-6,
                "{phase:?} {delta}"
            );
        }
    }

    #[gpui::test]
    fn every_kind_renders_a_card(cx: &mut TestAppContext) {
        for kind in [
            ToastKind::Info,
            ToastKind::Success,
            ToastKind::Warning,
            ToastKind::Error,
            ToastKind::Loading,
        ] {
            let bounds = measure(cx, move |_, _| Toast::new("t", kind, "Branch created"));
            assert!(
                f32::from(bounds.size.height) >= 2.0 * 12.0 + 16.0,
                "{kind:?} keeps its padding"
            );
        }
    }

    #[gpui::test]
    fn a_closable_toast_keeps_its_height_and_clicking_close_runs_the_handler(
        cx: &mut TestAppContext
    ) {
        let plain = measure(cx, |_, _| Toast::new("t", ToastKind::Info, "Saved"));
        let closed = Rc::new(std::cell::Cell::new(0));
        let seen = Rc::clone(&closed);
        let toast = move |_: &mut Window, _: &mut App| {
            let count = Rc::clone(&seen);
            Toast::new("t", ToastKind::Info, "Saved")
                .on_close(Rc::new(move |_, _| count.set(count.get() + 1)))
        };
        let closable = measure(cx, toast.clone());
        assert_eq!(closable.size.height, plain.size.height);
        let cx = crate::test_support::open(cx, toast);
        cx.simulate_click(
            gpui::point(
                closable.right() - px(12.0 + 9.0),
                closable.top() + px(12.0 + 9.0),
            ),
            gpui::Modifiers::none(),
        );
        assert_eq!(closed.get(), 1);
    }

    #[gpui::test]
    fn a_full_width_toast_fills_its_container(cx: &mut TestAppContext) {
        let bounds = measure(cx, |_, _| {
            div()
                .w(px(288.0))
                .child(Toast::new("t", ToastKind::Info, "Saved").full_width())
        });
        assert_eq!(f32::from(bounds.size.width), 288.0);
    }
}
