//! The lifted editor tab under the pointer while it is dragged (develop's `TabDragPreview`).

use gpui::prelude::FluentBuilder as _;
use gpui::{
    App, Context, InteractiveElement, IntoElement, ParentElement, Pixels, Render, SharedString,
    Size, Styled, Window, div, px, relative,
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

/// An editor tab's drag preview: the lifted control surface, `rounded-lg`, `px-2 py-1`, its
/// `ui-text-xs` name (at most 200px) with the unsaved dot after it, as large as the tab it lifts
/// or else as its name. Return it from `on_drag`.
#[derive(Clone)]
pub struct TabPreview {
    name: SharedString,
    unsaved: bool,
    size: Option<Size<Pixels>>,
}

impl TabPreview {
    /// A preview of the tab named `name`.
    pub fn new(name: impl Into<SharedString>) -> Self {
        Self {
            name: name.into(),
            unsaved: false,
            size: None,
        }
    }

    /// Takes the size of the tab it lifts, as develop's drag overlay takes its source's.
    #[must_use]
    pub fn sized(
        mut self,
        size: Size<Pixels>,
    ) -> Self {
        self.size = Some(size);
        self
    }

    /// Shows the unsaved dot after the name.
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
        .when_some(tab.size, |pill, size| pill.w(size.width).h(size.height))
        .child(label);
    div().flex().child(pill)
}

opaque_debug!(TabPreview);

#[cfg(test)]
mod tests {
    use gpui::{AppContext as _, TestAppContext};

    use super::*;
    use crate::test_support::probe::{measure, open_floating, settle};

    #[gpui::test]
    fn the_preview_fits_its_name_on_an_eleven_pixel_line(cx: &mut TestAppContext) {
        let short = measure(cx, |_, cx| cx.new(|_| TabPreview::new("a.rs")));
        let long = measure(cx, |_, cx| cx.new(|_| TabPreview::new("a-longer-name.rs")));
        assert!(long.size.width > short.size.width, "{short:?} {long:?}");
        let tall = f32::from(short.size.height);
        assert!((26.0..=27.0).contains(&tall), "{tall}");
    }

    #[gpui::test]
    fn a_long_name_stops_at_two_hundred_pixels(cx: &mut TestAppContext) {
        let name = "n".repeat(400);
        let bounds = measure(cx, move |_, cx| cx.new(|_| TabPreview::new(name.clone())));
        let most = LABEL_MAX + 2.0 * f32::from(spacing::STEP_2) + 2.0;
        assert!(f32::from(bounds.size.width) <= most, "{bounds:?}");
    }

    #[gpui::test]
    fn a_sized_preview_keeps_its_size_when_its_holder_has_none(cx: &mut TestAppContext) {
        let tab = gpui::size(px(116.0), px(32.0));
        let (_, mut top) = open_floating(cx, move |_, cx| {
            let preview = cx.new(|_| TabPreview::new("Untitled").sized(tab));
            div().relative().size_0().child(preview)
        });
        settle(&mut top);
        let ghost = top.debug_bounds("drag-ghost").unwrap_or_default();
        assert_eq!(ghost.size, tab, "a drag view lays its ghost out at no size");
    }
}
