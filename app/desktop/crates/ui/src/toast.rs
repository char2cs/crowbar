//! The toast card.

use std::rc::Rc;

use gpui::{
    Animation, AnimationExt, App, ElementId, Hsla, IntoElement, ParentElement, RenderOnce, SharedString, Styled, Window,
    div, px,
};

use crate::font::UI_FAMILY;
use crate::icon::Icon;
use crate::icon_name::IconName;
use crate::icon_weight::IconWeight;
use crate::keyframes::Keyframes;
use crate::motion::Motion;
use crate::palette::Palette;
use crate::radius;
use crate::shadow;
use crate::spinner::Spinner;
use crate::surface::Surface;
use crate::text_size;
use crate::theme::Theme;
use crate::timing;
use crate::toast_kind::ToastKind;

/// A message card: icon by tone, text, optional dismiss; error toasts shake once when added.
#[derive(IntoElement)]
pub struct Toast {
    id: ElementId,
    kind: ToastKind,
    message: SharedString,
}

impl Toast {
    /// A toast of `kind` showing `message`.
    pub fn new(id: impl Into<ElementId>, kind: ToastKind, message: impl Into<SharedString>) -> Self {
        Self { id: id.into(), kind, message: message.into() }
    }
}

fn tone(kind: ToastKind, palette: &Palette) -> (IconName, Hsla) {
    match kind {
        ToastKind::Info => (IconName::Info, palette.info()),
        ToastKind::Success => (IconName::CheckCircle, palette.success()),
        ToastKind::Warning => (IconName::Warning, palette.warning()),
        ToastKind::Error => (IconName::WarningCircle, palette.destructive()),
        ToastKind::Loading => (IconName::CircleNotch, palette.muted_foreground()),
    }
}

impl RenderOnce for Toast {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let palette = theme.palette(Surface::Window);
        let (name, color) = tone(self.kind, palette);
        let glyph = match self.kind {
            ToastKind::Loading => Spinner::new((self.id.clone(), "spin")).into_any_element(),
            _ => Icon::new(name).weight(IconWeight::Fill).color(color).into_any_element(),
        };
        let card = div()
            .flex()
            .items_start()
            .gap(px(8.0))
            .px(px(14.0))
            .py(px(12.0))
            .rounded(radius::LG)
            .border_1()
            .border_color(palette.border())
            .bg(palette.popover())
            .text_color(palette.foreground())
            .text_size(text_size::SM)
            .font_family(UI_FAMILY)
            .shadow(shadow::lg(0.05))
            .child(glyph)
            .child(self.message);
        let motion = Motion::of(cx);
        let shake = Rc::new(Keyframes::TOAST_ERROR);
        card.with_animation(
            self.id,
            Animation::new(motion.duration(timing::TOAST_ERROR)),
            move |card, delta| match self.kind {
                ToastKind::Error => card.ml(px(shake.sample(delta).max(0.0))).mr(px((-shake.sample(delta)).max(0.0))),
                _ => card,
            },
        )
    }
}

#[cfg(test)]
#[path = "toast_tests.rs"]
mod tests;
