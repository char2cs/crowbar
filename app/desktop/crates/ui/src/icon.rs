//! The icon element.

use gpui::{App, Hsla, IntoElement, Pixels, RenderOnce, Styled, Transformation, Window, px, radians, svg};

use crate::icon_files::FILES;
use crate::icon_name::IconName;
use crate::icon_weight::IconWeight;

const MISSING_PATH: &str = "";
const DEFAULT_SIZE: Pixels = px(16.0);

/// A Phosphor icon drawn with a colour token; defaults to 16px regular in the text colour.
#[derive(Debug, Clone, IntoElement)]
pub struct Icon {
    name: IconName,
    weight: IconWeight,
    size: Pixels,
    color: Option<Hsla>,
    rotation: f32,
}

impl Icon {
    /// An icon at 16px, regular weight, inheriting the text colour.
    pub fn new(name: IconName) -> Self {
        Self {
            name,
            weight: IconWeight::default(),
            size: DEFAULT_SIZE,
            color: None,
            rotation: 0.0,
        }
    }

    /// Selects the stroke weight; the file must be embedded or nothing is drawn.
    #[must_use]
    pub fn weight(mut self, weight: IconWeight) -> Self {
        self.weight = weight;
        self
    }

    /// Sets the square size.
    #[must_use]
    pub fn size(mut self, size: Pixels) -> Self {
        self.size = size;
        self
    }

    /// Sets the colour; without it the glyph takes the inherited text colour.
    #[must_use]
    pub fn color(mut self, color: Hsla) -> Self {
        self.color = Some(color);
        self
    }

    /// Turns the glyph clockwise by `radians`.
    #[must_use]
    pub fn rotate(mut self, radians: f32) -> Self {
        self.rotation = radians;
        self
    }

    /// Asset path of the embedded file for this name and weight, if it exists.
    pub fn path(name: IconName, weight: IconWeight) -> Option<&'static str> {
        FILES
            .iter()
            .find(|file| file.name == name && file.weight == weight)
            .map(|file| file.path)
    }
}

impl RenderOnce for Icon {
    fn render(self, window: &mut Window, _cx: &mut App) -> impl IntoElement {
        let path = Self::path(self.name, self.weight).unwrap_or(MISSING_PATH);
        let color = self.color.unwrap_or_else(|| window.text_style().color);
        svg()
            .path(path)
            .size(self.size)
            .flex_none()
            .text_color(color)
            .with_transformation(Transformation::rotate(radians(self.rotation)))
    }
}

#[cfg(test)]
#[path = "icon_tests.rs"]
mod tests;
