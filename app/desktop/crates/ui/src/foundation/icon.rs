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
pub use material_table::MaterialTable;
pub use provider_glyph::ProviderGlyph;
pub use wordmark::Wordmark;

use gpui::{
    App, Bounds, Hsla, IntoElement, Pixels, RenderOnce, Styled as _, TransformationMatrix, Window,
    canvas, px, radians,
};

use crate::foundation::icon::file::File;
use crate::foundation::icon::files::FILES;

const DEFAULT_SIZE: Pixels = px(16.0);

/// A Phosphor icon drawn with a colour token; defaults to 16px regular in the text colour.
///
/// Without a colour it takes the text colour it is painted in, a parent's hover colour included
/// (stock `svg()` paints only a colour set on itself).
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
        file(name, weight).map(|file| file.path)
    }
}

fn file(
    name: IconName,
    weight: IconWeight,
) -> Option<&'static File> {
    FILES
        .iter()
        .find(|file| file.name == name && file.weight == weight)
}

fn paint(
    icon: &Icon,
    bounds: Bounds<Pixels>,
    window: &mut Window,
    cx: &mut App,
) {
    let Some(file) = file(icon.name, icon.weight) else {
        return;
    };
    let color = icon.color.unwrap_or_else(|| window.text_style().color);
    let center = bounds.center().scale(window.scale_factor());
    let turn = TransformationMatrix::unit()
        .translate(center)
        .rotate(radians(icon.rotation))
        .translate(-center);
    let painted = window.paint_svg(bounds, file.path.into(), Some(file.bytes), turn, color, cx);
    if let Err(error) = painted {
        tracing::warn!(%error, path = file.path, "an icon could not be painted");
    }
}

impl RenderOnce for Icon {
    fn render(
        self,
        _window: &mut Window,
        _cx: &mut App,
    ) -> impl IntoElement {
        let size = self.size;
        canvas(
            |_, _, _| (),
            move |bounds, (), window, cx| paint(&self, bounds, window, cx),
        )
        .size(size)
        .flex_none()
    }
}

#[cfg(test)]
mod tests {
    use gpui::{ParentElement as _, TestAppContext, div, red};

    use super::*;
    use crate::test_support::probe::open;

    #[gpui::test]
    fn an_icon_without_a_colour_paints_in_the_text_colour_around_it(cx: &mut TestAppContext) {
        let cx = open(cx, |_, _| {
            div()
                .text_color(red())
                .child(Icon::new(IconName::Folder).weight(IconWeight::Fill))
        });
        let colours: Vec<Hsla> = cx.update(|window, _| {
            let sprites = &window.rendered_scene().monochrome_sprites;
            sprites.iter().map(|sprite| sprite.color).collect()
        });
        assert_eq!(colours, [red()]);
    }

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
