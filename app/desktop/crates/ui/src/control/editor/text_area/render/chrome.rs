//! Border, fill and shadows of a text area for each condition.

use crate::control::editor::field::{Chrome, chrome};
use crate::control::editor::text_area::render::text_area_condition::TextAreaCondition;
use crate::foundation::Appearance;
use crate::foundation::Palette;
use crate::foundation::Pose;

const INVALID_BORDER: f32 = 0.36;
const INVALID_FOCUSED_BORDER: f32 = 0.64;
const INVALID_RING_LIGHT: f32 = 0.16;
const INVALID_RING_DARK: f32 = 0.24;

pub(crate) fn text_area_chrome(
    palette: &Palette,
    appearance: Appearance,
    focus: Pose,
    condition: TextAreaCondition,
) -> Chrome {
    let mut frame = chrome(palette, appearance, focus);
    match condition {
        TextAreaCondition::Normal => {},
        TextAreaCondition::Disabled => frame.shadows.clear(),
        TextAreaCondition::Invalid => flag(&mut frame, palette, appearance, focus),
    }
    frame
}

fn flag(
    frame: &mut Chrome,
    palette: &Palette,
    appearance: Appearance,
    focus: Pose,
) {
    let destructive = palette.destructive();
    if focus == Pose::Off {
        frame.border = destructive.opacity(INVALID_BORDER);
        frame.shadows.clear();
        return;
    }
    let ring = match appearance {
        Appearance::Light => INVALID_RING_LIGHT,
        Appearance::Dark => INVALID_RING_DARK,
    };
    frame.border = destructive.opacity(INVALID_FOCUSED_BORDER);
    frame.shadows.clear();
    frame.shadows.extend(
        chrome(palette, appearance, Pose::On)
            .shadows
            .into_iter()
            .skip(1)
            .map(|shadow| gpui::BoxShadow {
                color: destructive.opacity(ring),
                ..shadow
            }),
    );
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::foundation::Surface;
    use crate::foundation::Theme;

    #[test]
    fn each_condition_recolours_the_border_and_drops_or_keeps_the_ring() {
        let palette = Theme::default().palette(Surface::Window);
        let destructive = palette.destructive();
        let cases = [
            (TextAreaCondition::Normal, Pose::Off, palette.input(), 1),
            (TextAreaCondition::Normal, Pose::On, palette.ring(), 2),
            (
                TextAreaCondition::Invalid,
                Pose::Off,
                destructive.opacity(INVALID_BORDER),
                0,
            ),
            (
                TextAreaCondition::Invalid,
                Pose::On,
                destructive.opacity(INVALID_FOCUSED_BORDER),
                1,
            ),
            (TextAreaCondition::Disabled, Pose::Off, palette.input(), 0),
        ];
        for (condition, focus, border, shadows) in cases {
            let frame = text_area_chrome(palette, Appearance::Light, focus, condition);
            assert_eq!(frame.border, border, "{condition:?} {focus:?} border");
            assert_eq!(
                frame.shadows.len(),
                shadows,
                "{condition:?} {focus:?} shadows"
            );
        }
    }

    #[test]
    fn the_invalid_ring_is_tinted_destructive() {
        let palette = Theme::default().palette(Surface::Window);
        let frame = text_area_chrome(
            palette,
            Appearance::Dark,
            Pose::On,
            TextAreaCondition::Invalid,
        );
        let ring = frame.shadows.first().map(|shadow| shadow.color);
        assert_eq!(ring, Some(palette.destructive().opacity(INVALID_RING_DARK)));
    }
}
