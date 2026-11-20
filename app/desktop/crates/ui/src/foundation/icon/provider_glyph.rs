use gpui::prelude::FluentBuilder as _;
use gpui::{
    App, Hsla, IntoElement, ParentElement as _, Pixels, RenderOnce, Rgba, SharedString, Styled,
    Window, div, img, px,
};

use crate::foundation::icon::glyph_images::GlyphImages;

const CURRENT_COLOR: &str = "currentColor";
const DEFAULT_SIZE: Pixels = px(16.0);
const CHANNEL: f32 = 255.0;

/// A provider's own artwork, an SVG the daemon serves, drawn with its `currentColor` parts in
/// the host's ink as an inline SVG inherits the text colour.
#[derive(Debug, Clone, IntoElement)]
pub struct ProviderGlyph {
    svg: SharedString,
    color: Hsla,
    size: Pixels,
}

impl ProviderGlyph {
    /// The artwork `svg` in `color`, 16px square.
    #[must_use]
    pub fn new(
        svg: impl Into<SharedString>,
        color: Hsla,
    ) -> Self {
        Self {
            svg: svg.into(),
            color,
            size: DEFAULT_SIZE,
        }
    }

    /// Draws the artwork `size` square.
    #[must_use]
    pub fn size(
        mut self,
        size: Pixels,
    ) -> Self {
        self.size = size;
        self
    }
}

impl RenderOnce for ProviderGlyph {
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let image = GlyphImages::drawn(&inked(&self.svg, self.color), cx);
        div()
            .size(self.size)
            .flex_none()
            .when_some(image, |frame, image| frame.child(img(image).size_full()))
    }
}

fn inked(
    svg: &str,
    color: Hsla,
) -> String {
    svg.replace(CURRENT_COLOR, &css_color(color))
}

fn css_color(color: Hsla) -> String {
    let rgba = Rgba::from(color);
    let channel = |value: f32| (value * CHANNEL).round();
    format!(
        "rgba({},{},{},{})",
        channel(rgba.r),
        channel(rgba.g),
        channel(rgba.b),
        rgba.a
    )
}

#[cfg(test)]
mod tests {
    use gpui::hsla;

    use super::*;

    #[test]
    fn the_parts_drawn_in_the_current_colour_take_the_ink_and_the_rest_keep_theirs() {
        let svg = r##"<svg><path fill="currentColor"/><path fill="#d97757"/></svg>"##;
        let white = hsla(0.0, 0.0, 1.0, 1.0);
        assert_eq!(
            inked(svg, white),
            r##"<svg><path fill="rgba(255,255,255,1)"/><path fill="#d97757"/></svg>"##
        );
    }
}
