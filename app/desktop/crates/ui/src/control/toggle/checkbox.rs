use gpui::prelude::FluentBuilder;
use gpui::{
    App, ElementId, Hsla, InteractiveElement, IntoElement, MouseButton, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, px,
};

use crate::control::toggle::handler::Handler;
use crate::foundation::Appearance;
use crate::foundation::Icon;
use crate::foundation::IconName;
use crate::foundation::IconWeight;
use crate::foundation::Palette;
use crate::foundation::Pose;
use crate::foundation::Surface;
use crate::foundation::Theme;
use crate::foundation::focus::modality::kept_focus::kept_focus;
use crate::foundation::focus_ring;
use crate::foundation::opaque_debug;
use crate::foundation::tokens::opacity;
use crate::foundation::tokens::shadow;

const SIZE: f32 = 16.0;
const MARK: f32 = 12.0;
const RADIUS: f32 = 4.0;
const DISABLED_OPACITY: f32 = 0.64;

#[derive(IntoElement)]
pub struct Checkbox {
    id: ElementId,
    checked: bool,
    mixed: Pose,
    disabled: bool,
    surface: Surface,
    on_change: Option<Handler>,
}

impl Checkbox {
    pub fn new(
        id: impl Into<ElementId>,
        checked: impl Into<Pose>,
    ) -> Self {
        Self {
            id: id.into(),
            checked: bool::from(checked.into()),
            mixed: Pose::Off,
            disabled: false,
            surface: Surface::default(),
            on_change: None,
        }
    }

    #[must_use]
    pub fn indeterminate(mut self) -> Self {
        self.mixed = Pose::from(!self.checked);
        self
    }

    #[must_use]
    pub fn disabled(mut self) -> Self {
        self.disabled = true;
        self
    }

    #[must_use]
    pub fn on(
        mut self,
        surface: Surface,
    ) -> Self {
        self.surface = surface;
        self
    }

    #[must_use]
    pub fn on_change(
        mut self,
        handler: impl Fn(Pose, &mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_change = Some(Box::new(handler));
        self
    }
}

fn fill_at_rest(
    palette: &Palette,
    appearance: Appearance,
) -> Hsla {
    if appearance == Appearance::Light {
        return palette.background();
    }
    palette.input().opacity(opacity::DARK_INPUT)
}

fn box_colors(
    checked: Pose,
    palette: &Palette,
    appearance: Appearance,
) -> (Hsla, Hsla) {
    if checked == Pose::On {
        return (palette.primary(), palette.primary());
    }
    (fill_at_rest(palette, appearance), palette.input())
}

fn mark(palette: &Palette) -> Icon {
    Icon::new(IconName::Check)
        .weight(IconWeight::Bold)
        .size(px(MARK))
        .color(palette.primary_foreground())
}

fn dash(palette: &Palette) -> Icon {
    Icon::new(IconName::Minus)
        .size(px(MARK))
        .color(palette.foreground())
}

impl RenderOnce for Checkbox {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let focus = kept_focus(&self.id, window, cx);
        let rung = crate::Modality::rings(&focus, window, cx);
        let theme = *Theme::of(cx);
        let palette = theme.palette(self.surface);
        let (checked, enabled) = (self.checked, !self.disabled);
        let (fill, border) = box_colors(Pose::from(checked), palette, theme.appearance());
        div()
            .id(self.id)
            .flex()
            .flex_none()
            .items_center()
            .justify_center()
            .size(px(SIZE))
            .rounded(px(RADIUS))
            .border_1()
            .border_color(border)
            .bg(fill)
            .when(!checked && enabled, |d| {
                d.shadow(shadow::xs(opacity::PERCENT_5))
            })
            .when(enabled, |d| d.cursor_pointer().track_focus(&focus))
            .when(!enabled, |d| d.opacity(DISABLED_OPACITY))
            .when(rung, |d| d.shadow(focus_ring(palette)))
            .on_mouse_down(MouseButton::Left, |_, window, _| window.prevent_default())
            .when(checked, |d| d.child(mark(palette)))
            .when(self.mixed == Pose::On, |d| d.child(dash(palette)))
            .when_some(self.on_change.filter(|_| enabled), move |d, handler| {
                d.on_click(move |_, window, cx| handler(Pose::from(!checked), window, cx))
            })
    }
}

opaque_debug!(Checkbox);

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::{TestAppContext, point, px};

    use super::*;
    use crate::test_support::probe::{measure, open};

    #[gpui::test]
    fn box_is_sixteen_pixels(cx: &mut TestAppContext) {
        let bounds = measure(cx, |_, _| Checkbox::new("c", false));
        assert_eq!(
            (f32::from(bounds.size.width), f32::from(bounds.size.height)),
            (16.0, 16.0)
        );
    }

    fn clicked(
        cx: &mut TestAppContext,
        checkbox: fn() -> Checkbox,
    ) -> Option<bool> {
        let seen = Rc::new(Cell::new(None));
        let sink = Rc::clone(&seen);
        let cx = open(cx, move |_, _| {
            let sink = Rc::clone(&sink);
            checkbox().on_change(move |value, _, _| sink.set(Some(bool::from(value))))
        });
        cx.simulate_click(point(px(8.0), px(8.0)), gpui::Modifiers::none());
        seen.get()
    }

    #[gpui::test]
    fn click_requests_the_opposite_value(cx: &mut TestAppContext) {
        assert_eq!(clicked(cx, || Checkbox::new("c", false)), Some(true));
    }

    #[gpui::test]
    fn a_partly_checked_box_asks_for_checked(cx: &mut TestAppContext) {
        assert_eq!(
            clicked(cx, || Checkbox::new("c", false).indeterminate()),
            Some(true)
        );
    }
}
