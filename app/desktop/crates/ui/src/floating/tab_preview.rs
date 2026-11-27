use gpui::prelude::FluentBuilder as _;
use gpui::{
    App, Context, InteractiveElement, IntoElement, ParentElement, Pixels, Render, SharedString,
    Styled, Window, div, px, relative,
};

use crate::floating::Portal;
use crate::foundation::ScaledText;
use crate::foundation::Surface;
use crate::foundation::Theme;
use crate::foundation::opaque_debug;
use crate::foundation::tokens::layer;
use crate::foundation::tokens::radius;
use crate::foundation::tokens::shadow;
use crate::foundation::tokens::spacing;

const LABEL_MAX: f32 = 200.0;
const LINE: f32 = 1.5;
const SHADOW_ALPHA: f32 = 0.15;
const DIRTY_DOT: &str = "•";

#[derive(Clone)]
pub struct TabPreview {
    name: SharedString,
    unsaved: bool,
    width: Option<Pixels>,
}

impl TabPreview {
    pub fn new(name: impl Into<SharedString>) -> Self {
        Self {
            name: name.into(),
            unsaved: false,
            width: None,
        }
    }

    #[must_use]
    pub fn wide(
        mut self,
        width: Pixels,
    ) -> Self {
        self.width = Some(width);
        self
    }

    #[must_use]
    pub fn unsaved(mut self) -> Self {
        self.unsaved = true;
        self
    }
}

impl Render for TabPreview {
    fn render(
        &mut self,
        _window: &mut Window,
        _cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let tab = self.clone();
        Portal::lifted("tab-preview", move |_, cx| preview(&tab, cx)).priority(layer::TOOLTIP)
    }
}

fn preview(
    tab: &TabPreview,
    cx: &App,
) -> gpui::Div {
    let theme = *Theme::of(cx);
    let palette = theme.palette(Surface::Window);
    let dot = tab.unsaved.then(|| {
        div()
            .ml(spacing::STEP_1)
            .text_color(palette.muted_foreground())
            .child(DIRTY_DOT)
    });
    let label = div()
        .flex()
        .max_w(px(LABEL_MAX))
        .overflow_hidden()
        .text_color(palette.control_foreground())
        .child(div().truncate().child(tab.name.clone()))
        .children(dot);
    let pill = div()
        .flex()
        .flex_none()
        .items_center()
        .gap(spacing::STEP_1_5)
        .px(spacing::STEP_2)
        .py(spacing::STEP_1)
        .rounded(radius::LG)
        .bg(palette.control_surface())
        .border_1()
        .border_color(palette.border())
        .shadow(shadow::md(SHADOW_ALPHA))
        .debug_selector(|| "drag-ghost".into())
        .font_family(theme.ui_family())
        .text_size(ScaledText::Xs.size(&theme))
        .line_height(relative(LINE))
        .when_some(tab.width, Styled::w)
        .child(label);
    div().flex().child(crate::Shaped::seen(pill, radius::LG))
}

opaque_debug!(TabPreview);

#[cfg(test)]
mod tests {
    use gpui::{AppContext as _, TestAppContext};

    use super::*;
    use crate::test_support::probe::{measure, open, settle};

    #[gpui::test]
    fn the_preview_fits_its_name_on_an_eleven_pixel_line(cx: &mut TestAppContext) {
        let short = measure(cx, |_, cx| preview(&TabPreview::new("a.rs"), cx));
        let long = measure(cx, |_, cx| {
            preview(&TabPreview::new("a-longer-name.rs"), cx)
        });
        assert!(long.size.width > short.size.width, "{short:?} {long:?}");
        let tall = f32::from(short.size.height);
        assert!((26.0..=27.0).contains(&tall), "{tall}");
    }

    #[gpui::test]
    fn a_long_name_stops_at_two_hundred_pixels(cx: &mut TestAppContext) {
        let name = "n".repeat(400);
        let bounds = measure(cx, move |_, cx| preview(&TabPreview::new(name.clone()), cx));
        let most = LABEL_MAX + 2.0 * f32::from(spacing::STEP_2) + 2.0;
        assert!(f32::from(bounds.size.width) <= most, "{bounds:?}");
    }

    #[gpui::test]
    fn a_widened_preview_keeps_its_width_when_its_holder_has_none(cx: &mut TestAppContext) {
        let tab = gpui::size(px(116.0), px(32.0));
        let held = std::rc::Rc::new(std::cell::OnceCell::new());
        let owner = open(cx, move |_, cx| {
            let preview =
                held.get_or_init(|| cx.new(|_| TabPreview::new("Untitled").wide(tab.width)));
            div().relative().size_0().child(preview.clone())
        });
        settle(owner);
        let ghost = owner.debug_bounds("drag-ghost").unwrap_or_default();
        assert_eq!(
            ghost.size.width, tab.width,
            "a drag view lays its ghost out at no size"
        );
        let tall = f32::from(ghost.size.height);
        assert!(
            (26.0..=27.0).contains(&tall),
            "develop's preview keeps its own height: {tall}"
        );
    }
}
