//! The toast card.

mod beneath;
mod close;
mod motion;
mod phase;
#[cfg(test)]
mod test_support;
mod toast_kind;
mod toast_queue;

pub(crate) use phase::Phase;
pub use toast_kind::ToastKind;
pub use toast_queue::{
    ToastContent, ToastId, ToastLifetime, ToastPlacement, ToastQueue, ToastQueueEvent, ToastSide,
};

use gpui::prelude::FluentBuilder as _;
use gpui::{
    Animation, AnimationExt, App, Div, ElementId, FontWeight, Hsla, InteractiveElement,
    IntoElement, ParentElement, RenderOnce, SharedString, Styled, Window, div, px, relative,
};

use crate::display::spinner::Spinner;
use crate::display::toast::beneath::beneath;
use crate::display::toast::close::Close;
use crate::display::toast::motion::{fade, pulse_scale, shake_offset};
use crate::foundation::CloseHandler;
use crate::foundation::CubicBezier;
use crate::foundation::Icon;
use crate::foundation::IconName;
use crate::foundation::Motion;
use crate::foundation::Palette;
use crate::foundation::Surface;
use crate::foundation::Theme;
use crate::foundation::opaque_debug;
use crate::foundation::tokens::opacity;
use crate::foundation::tokens::radius;
use crate::foundation::tokens::shadow;
use crate::foundation::tokens::spacing;
use crate::foundation::tokens::text_size;
use crate::foundation::tokens::timing;

const CLOSE_CLEARANCE: f32 = 24.0;
const TITLE_GAP: f32 = 2.0;

/// A message card: icon by tone and text, under a title when it has one.
///
/// An updated error shakes and an updated success pulses; the card fades in when it first appears and
/// out when it is [`Toast::leaving`]. With [`Toast::on_close`] it shows the corner close button.
#[derive(IntoElement)]
pub struct Toast {
    id: ElementId,
    kind: ToastKind,
    title: Option<SharedString>,
    message: SharedString,
    updates: u32,
    on_close: Option<CloseHandler>,
    fill: bool,
    phase: Phase,
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
            title: None,
            message: message.into(),
            updates: 0,
            on_close: None,
            fill: false,
            phase: Phase::Shown,
        }
    }

    /// Puts `title` above the message in medium weight; the message then reads muted, as detail.
    #[must_use]
    pub fn title(
        mut self,
        title: SharedString,
    ) -> Self {
        self.title = Some(title);
        self
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
        self.phase = Phase::Leaving;
        self
    }
}

fn tone(
    kind: ToastKind,
    palette: &Palette,
) -> Icon {
    let (name, color): (IconName, Hsla) = match kind {
        ToastKind::Info => (IconName::Info, palette.info()),
        ToastKind::Success => (IconName::CheckCircle, palette.success()),
        ToastKind::Warning => (IconName::Warning, palette.warning()),
        ToastKind::Error => (IconName::WarningCircle, palette.destructive()),
        ToastKind::Loading => (IconName::CircleNotch, palette.muted_foreground()),
    };
    Icon::new(name).color(color)
}

impl Toast {
    fn glyph(
        &self,
        palette: &Palette,
    ) -> gpui::AnyElement {
        match self.kind {
            ToastKind::Loading => Spinner::new((self.id.clone(), "spin")).into_any_element(),
            kind => tone(kind, palette).into_any_element(),
        }
    }

    fn text(
        &self,
        palette: &Palette,
    ) -> Div {
        let text = div().min_w_0();
        let Some(title) = self.title.clone() else {
            return text.child(self.message.clone());
        };
        let detail = div()
            .text_color(palette.muted_foreground())
            .child(self.message.clone());
        text.flex()
            .flex_col()
            .gap(px(TITLE_GAP))
            .child(div().font_weight(FontWeight::MEDIUM).child(title))
            .child(detail)
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
            .line_height(text_size::LINE_HEIGHT)
            .font_family(theme.ui_family())
            .shadow(shadow::lg(opacity::PERCENT_5))
    }
}

impl RenderOnce for Toast {
    fn render(
        self,
        window: &mut Window,
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
            .child(self.text(theme.palette(Surface::Window)));
        let close = self
            .on_close
            .clone()
            .map(|handler| Close::new((self.id.clone(), "close"), handler));
        let card = self.frame(&theme).child(body).children(close);
        let motion = Motion::of(cx);
        let (kind, updates, phase) = (self.kind, self.updates, self.phase);
        let replayed = card.with_animation(
            (self.id.clone(), format!("replay-{updates}")),
            Animation::new(motion.duration(timing::TOAST_ERROR.max(timing::TOAST_SUCCESS))),
            move |card, delta| settle(card, kind, updates, delta),
        );
        let beneath = beneath(&self.id, phase, window, cx);
        let faded = div()
            .when(self.fill, Styled::w_full)
            .child(replayed)
            .with_animation(
                (self.id, format!("fade-{phase:?}")),
                motion.animation(timing::SLOW, CubicBezier::DEFAULT),
                move |wrapper, delta| wrapper.opacity(fade(phase, delta)),
            );
        div()
            .debug_selector(|| "ui-toast".into())
            .relative()
            .when(self.fill, Styled::w_full)
            .child(beneath)
            .child(faded)
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
    use crate::test_support::probe::measure;

    #[test]
    fn glyphs_are_outlined_as_on_develop() {
        let palette = Theme::default();
        let palette = palette.palette(Surface::Window);
        for kind in [
            ToastKind::Info,
            ToastKind::Success,
            ToastKind::Warning,
            ToastKind::Error,
        ] {
            let glyph = format!("{:?}", tone(kind, palette));
            assert!(glyph.contains("Regular"), "{kind:?}: {glyph}");
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
        let cx = crate::test_support::probe::open(cx, toast);
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
    fn a_titled_toast_stacks_its_title_over_the_message(cx: &mut TestAppContext) {
        let plain = measure(cx, |_, _| Toast::new("t", ToastKind::Error, "busy"));
        let titled = measure(cx, |_, _| {
            Toast::new("t", ToastKind::Error, "busy").title("Couldn't remove x".into())
        });
        assert!(
            titled.size.height > plain.size.height,
            "the title takes a line of its own"
        );
        let lines = 2.0 * 20.0 + TITLE_GAP;
        assert_eq!(
            f32::from(titled.size.height),
            2.0 * 12.0 + 2.0 + lines,
            "py-3, a 1px border and two text-sm lines of 20px, as develop's card"
        );
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
