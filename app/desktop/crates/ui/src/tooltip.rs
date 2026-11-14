//! Tooltips shown after a short hover delay.

use std::time::Instant;

use gpui::prelude::FluentBuilder;
use gpui::{
    AnyElement, App, ElementId, InteractiveElement, IntoElement, ParentElement, RenderOnce, SharedString, Styled, Window,
    deferred, div, px,
};

use crate::cubic_bezier::CubicBezier;
use crate::font::UI_FAMILY;
use crate::hover;
use crate::kbd::Kbd;
use crate::layer;
use crate::motion::Motion;
use crate::radius;
use crate::shadow;
use crate::surface::Surface;
use crate::text_size::UiText;
use crate::theme::Theme;
use crate::timing;
use crate::tooltip_side::TooltipSide;

const SIDE_OFFSET: f32 = 6.0;
const SLIDE: f32 = 4.0;

/// Wraps a trigger and shows `text` (and an optional shortcut) after the pointer rests on it for 150ms.
#[derive(IntoElement)]
pub struct Tooltip {
    id: ElementId,
    text: SharedString,
    shortcut: Option<SharedString>,
    side: TooltipSide,
    trigger: AnyElement,
}

impl Tooltip {
    /// A tooltip for `trigger`.
    pub fn new(id: impl Into<ElementId>, text: impl Into<SharedString>, trigger: impl IntoElement) -> Self {
        Self {
            id: id.into(),
            text: text.into(),
            shortcut: None,
            side: TooltipSide::default(),
            trigger: trigger.into_any_element(),
        }
    }

    /// Adds a shortcut hint.
    #[must_use]
    pub fn shortcut(mut self, keys: impl Into<SharedString>) -> Self {
        self.shortcut = Some(keys.into());
        self
    }

    /// Chooses the side of the trigger.
    #[must_use]
    pub fn side(mut self, side: TooltipSide) -> Self {
        self.side = side;
        self
    }
}

impl RenderOnce for Tooltip {
    fn render(self, window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let palette = theme.palette(Surface::Window);
        let hover_state = hover::track(&self.id, "tooltip", false, window, cx);
        let shown = hover_state
            .read(cx)
            .entered(timing::TOOLTIP_DELAY, timing::DEFAULT, CubicBezier::EASE, Motion::of(cx), Instant::now());
        if shown.running {
            window.request_animation_frame();
        }
        let slide = SLIDE * (1.0 - shown.value);
        let card = div()
            .flex()
            .items_center()
            .gap(px(8.0))
            .px(px(10.0))
            .py(px(6.0))
            .rounded(radius::LG)
            .border_1()
            .border_color(palette.border().opacity(0.7))
            .bg(palette.card().opacity(0.95))
            .text_color(palette.foreground())
            .text_size(UiText::Sm.size(&theme))
            .font_family(UI_FAMILY)
            .whitespace_nowrap()
            .shadow(shadow::lg(0.1))
            .opacity(shown.value)
            .child(self.text)
            .children(self.shortcut.map(Kbd::new));
        let placement = div().absolute().left_0().w_full().flex().justify_center();
        let placement = match self.side {
            TooltipSide::Bottom => placement.top_full().mt(px(SIDE_OFFSET + slide)),
            TooltipSide::Top => placement.bottom_full().mb(px(SIDE_OFFSET - slide)),
        };
        div()
            .id(self.id)
            .relative()
            .flex_none()
            .on_hover(hover::listener(&hover_state))
            .child(self.trigger)
            .when(shown.value > 0.0, |d| {
                d.child(deferred(placement.child(card)).with_priority(layer::TOOLTIP))
            })
    }
}

#[cfg(test)]
#[path = "tooltip_tests.rs"]
mod tests;
