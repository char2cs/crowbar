use gpui::{Hsla, Rgba};

const LINEAR_THRESHOLD: f32 = 0.04045;
const LUMINANCE_WEIGHTS: [f32; 3] = [0.2126, 0.7152, 0.0722];
const FLARE: f32 = 0.05;
const LINEAR_SLOPE: f32 = 12.92;
const GAMMA_OFFSET: f32 = 0.055;
const GAMMA: f32 = 2.4;

fn linear(channel: f32) -> f32 {
    if channel <= LINEAR_THRESHOLD {
        return channel / LINEAR_SLOPE;
    }
    ((channel + GAMMA_OFFSET) / (1.0 + GAMMA_OFFSET)).powf(GAMMA)
}

fn luminance(color: Rgba) -> f32 {
    [color.r, color.g, color.b]
        .into_iter()
        .zip(LUMINANCE_WEIGHTS)
        .map(|(channel, weight)| linear(channel) * weight)
        .sum()
}

fn over(
    foreground: Rgba,
    background: Rgba,
) -> Rgba {
    let mix = |top: f32, bottom: f32| top * foreground.a + bottom * (1.0 - foreground.a);
    Rgba {
        r: mix(foreground.r, background.r),
        g: mix(foreground.g, background.g),
        b: mix(foreground.b, background.b),
        a: 1.0,
    }
}

pub(crate) fn contrast_ratio(
    foreground: Hsla,
    background: Hsla,
) -> f32 {
    let background = Rgba::from(background);
    let foreground = over(Rgba::from(foreground), background);
    let (lighter, darker) = {
        let (a, b) = (luminance(foreground), luminance(background));
        (a.max(b), a.min(b))
    };
    (lighter + FLARE) / (darker + FLARE)
}

#[cfg(test)]
mod tests {
    use gpui::rgba;

    use super::*;
    use gpui::Hsla;

    use crate::foundation::Appearance;
    use crate::foundation::Palette;
    use crate::foundation::Surface;
    use crate::foundation::Theme;
    use crate::foundation::ThemeId;
    use crate::foundation::ThemeMode;

    type Case = (&'static str, u32, u32, f32, f32);

    const CASES: &[Case] = &[
        (
            "black on white is twenty one",
            0x0000_00ff,
            0xffff_ffff,
            20.99,
            21.01,
        ),
        (
            "translucent foreground is composited",
            0x0000_0080,
            0xffff_ffff,
            3.0,
            21.0,
        ),
    ];

    #[test]
    fn ratios_fall_in_the_expected_range() {
        for &(name, foreground, background, low, high) in CASES {
            let ratio = contrast_ratio(rgba(foreground).into(), rgba(background).into());
            assert!(ratio > low && ratio < high, "{name}: ratio {ratio}");
        }
    }

    fn themes() -> impl Iterator<Item = (ThemeId, Appearance)> {
        [ThemeId::Crowbar, ThemeId::Frosted]
            .into_iter()
            .flat_map(|id| [(id, Appearance::Light), (id, Appearance::Dark)])
    }

    fn theme_for(
        id: ThemeId,
        appearance: Appearance,
    ) -> Theme {
        let mode = match appearance {
            Appearance::Light => ThemeMode::Light,
            Appearance::Dark => ThemeMode::Dark,
        };
        Theme::default().with_id(id).with_mode(mode)
    }

    type Pair = (&'static str, Hsla, Hsla);

    fn text_pairs(palette: &Palette) -> [Pair; 6] {
        [
            (
                "foreground/background",
                palette.foreground(),
                palette.background(),
            ),
            (
                "muted_foreground/background",
                palette.muted_foreground(),
                palette.background(),
            ),
            (
                "primary_foreground/primary",
                palette.primary_foreground(),
                palette.primary(),
            ),
            (
                "control_foreground/control_surface",
                palette.control_foreground(),
                palette.control_surface(),
            ),
            (
                "foreground/popover",
                palette.foreground(),
                palette.popover(),
            ),
            (
                "destructive/background",
                palette.destructive(),
                palette.background(),
            ),
        ]
    }

    fn ratings(
        id: ThemeId,
        appearance: Appearance,
        surface: Surface,
    ) -> [(String, f32); 6] {
        let palette = theme_for(id, appearance).palette(surface);
        text_pairs(palette).map(|(name, foreground, background)| {
            (
                format!("{id:?} {appearance:?} {surface:?} {name}"),
                contrast_ratio(foreground, background),
            )
        })
    }

    fn failures(minimum: f32) -> Vec<String> {
        let surfaces = [Surface::Window, Surface::Chrome];
        themes()
            .flat_map(|(id, appearance)| surfaces.map(|surface| (id, appearance, surface)))
            .flat_map(|(id, appearance, surface)| ratings(id, appearance, surface))
            .filter(|(_, ratio)| *ratio < minimum)
            .map(|(name, ratio)| format!("{name}: {ratio:.2}"))
            .collect()
    }

    #[test]
    fn text_pairs_meet_aa() {
        let failing = failures(4.5);
        assert!(failing.is_empty(), "below AA: {failing:?}");
    }
}
