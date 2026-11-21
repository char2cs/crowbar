//! The icon element.

mod assets;
mod file;
mod files;
mod glyph_images;
mod icon_name;
mod icon_weight;
mod material_entry;
mod material_icon;
mod material_table;
mod provider_glyph;
mod wordmark;

pub use assets::assets;
pub use icon_name::IconName;
pub use icon_weight::IconWeight;
pub use material_icon::MaterialIcon;
pub use provider_glyph::ProviderGlyph;
pub use wordmark::Wordmark;

use gpui::prelude::FluentBuilder as _;
use gpui::{
    App, Hsla, IntoElement, Pixels, RenderOnce, Styled, Transformation, Window, px, radians, svg,
};

use crate::foundation::icon::files::FILES;

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
    pub fn weight(
        mut self,
        weight: IconWeight,
    ) -> Self {
        self.weight = weight;
        self
    }

    /// Sets the square size.
    #[must_use]
    pub fn size(
        mut self,
        size: Pixels,
    ) -> Self {
        self.size = size;
        self
    }

    /// Sets the colour; without it the glyph takes the inherited text colour.
    #[must_use]
    pub fn color(
        mut self,
        color: Hsla,
    ) -> Self {
        self.color = Some(color);
        self
    }

    /// Turns the glyph clockwise by `radians`.
    #[must_use]
    pub fn rotate(
        mut self,
        radians: f32,
    ) -> Self {
        self.rotation = radians;
        self
    }

    /// Asset path of the embedded file for this name and weight, if it exists.
    pub fn path(
        name: IconName,
        weight: IconWeight,
    ) -> Option<&'static str> {
        FILES
            .iter()
            .find(|file| file.name == name && file.weight == weight)
            .map(|file| file.path)
    }
}

impl RenderOnce for Icon {
    fn render(
        self,
        _window: &mut Window,
        _cx: &mut App,
    ) -> impl IntoElement {
        let path = Self::path(self.name, self.weight).unwrap_or(MISSING_PATH);
        svg()
            .path(path)
            .size(self.size)
            .flex_none()
            .when_some(self.color, Styled::text_color)
            .with_transformation(Transformation::rotate(radians(self.rotation)))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn path_resolves_only_embedded_weights() {
        type Case = (IconName, IconWeight, Option<&'static str>);
        const CASES: &[Case] = &[
            (
                IconName::Gear,
                IconWeight::Regular,
                Some("icons/regular/gear.svg"),
            ),
            (
                IconName::GitBranch,
                IconWeight::Fill,
                Some("icons/fill/git-branch.svg"),
            ),
            (IconName::Gear, IconWeight::Duotone, None),
            (
                IconName::Keyboard,
                IconWeight::Regular,
                Some("icons/regular/keyboard.svg"),
            ),
        ];
        for &(name, weight, path) in CASES {
            assert_eq!(Icon::path(name, weight), path, "{name:?} {weight:?}");
        }
    }

    #[test]
    fn every_icon_has_at_least_one_weight() {
        for &name in IconName::ALL {
            let weights = [
                IconWeight::Regular,
                IconWeight::Bold,
                IconWeight::Fill,
                IconWeight::Duotone,
            ];
            assert!(
                weights.iter().any(|&w| Icon::path(name, w).is_some()),
                "{name:?}"
            );
        }
    }
}
