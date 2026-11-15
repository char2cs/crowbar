//! The toast card.

use std::rc::Rc;

use gpui::{
    Animation, AnimationExt, App, Div, ElementId, Hsla, IntoElement, ParentElement, RenderOnce,
    SharedString, Styled, Window, div, px,
};

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
use crate::toast_kind::ToastKind;

/// A message card: icon by tone and text; an error toast shakes when it is updated.
#[derive(IntoElement)]
pub struct Toast {
    id: ElementId,
    kind: ToastKind,
    message: SharedString,
    updates: u32,
}

fn shakes(
    kind: ToastKind,
    updates: u32,
) -> bool {
    kind == ToastKind::Error && updates > 0
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
        }
    }

    /// Marks the toast as updated `updates` times; an updated error shakes again.
    #[must_use]
    pub fn updated(
        mut self,
        updates: u32,
    ) -> Self {
        self.updates = updates;
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

impl RenderOnce for Toast {
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let palette = theme.palette(Surface::Window);
        let (name, color) = tone(self.kind, palette);
        let glyph = match self.kind {
            ToastKind::Loading => Spinner::new((self.id.clone(), "spin")).into_any_element(),
            _ => Icon::new(name)
                .weight(IconWeight::Fill)
                .color(color)
                .into_any_element(),
        };
        let card = div()
            .flex()
            .items_start()
            .gap(spacing::STEP_2)
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
            .child(glyph)
            .child(self.message);
        let motion = Motion::of(cx);
        let shake = shakes(self.kind, self.updates).then(|| Rc::new(Keyframes::TOAST_ERROR));
        card.with_animation(
            (self.id, format!("replay-{}", self.updates)),
            Animation::new(motion.duration(timing::TOAST_ERROR)),
            move |card, delta| offset(card, shake.as_ref().map_or(0.0, |k| k.sample(delta))),
        )
    }
}

fn offset(
    card: Div,
    shift: f32,
) -> Div {
    card.ml(px(shift.max(0.0))).mr(px((-shift).max(0.0)))
}

opaque_debug!(Toast);

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::test_support::measure;

    #[test]
    fn only_updated_errors_shake() {
        const CASES: &[(ToastKind, u32, bool)] = &[
            (ToastKind::Error, 0, false),
            (ToastKind::Error, 1, true),
            (ToastKind::Success, 1, false),
        ];
        for &(kind, updates, shaking) in CASES {
            assert_eq!(shakes(kind, updates), shaking, "{kind:?} {updates}");
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
}
