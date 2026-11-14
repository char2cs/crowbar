use gpui::rgba;

use super::*;
use gpui::Hsla;

use crate::appearance::Appearance;
use crate::palette::Palette;
use crate::surface::Surface;
use crate::theme::Theme;
use crate::theme_id::ThemeId;
use crate::theme_mode::ThemeMode;

#[test]
fn black_on_white_is_twenty_one() {
    let ratio = contrast_ratio(rgba(0x0000_00ff).into(), rgba(0xffff_ffff).into());
    assert!((ratio - 21.0).abs() < 0.01, "ratio {ratio}");
}

#[test]
fn translucent_foreground_is_composited() {
    let half = contrast_ratio(rgba(0x0000_0080).into(), rgba(0xffff_ffff).into());
    assert!(half > 3.0 && half < 21.0, "ratio {half}");
}

fn themes() -> impl Iterator<Item = (ThemeId, Appearance)> {
    [ThemeId::Crowbar, ThemeId::Frosted]
        .into_iter()
        .flat_map(|id| [(id, Appearance::Light), (id, Appearance::Dark)])
}

fn theme_for(id: ThemeId, appearance: Appearance) -> Theme {
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

fn failures(minimum: f32) -> Vec<String> {
    let surfaces = [Surface::Window, Surface::Chrome];
    themes()
        .flat_map(|(id, appearance)| surfaces.map(|surface| (id, appearance, surface)))
        .flat_map(|(id, appearance, surface)| {
            let palette = theme_for(id, appearance).palette(surface);
            text_pairs(palette).map(|(name, foreground, background)| {
                (
                    format!("{id:?} {appearance:?} {surface:?} {name}"),
                    contrast_ratio(foreground, background),
                )
            })
        })
        .filter(|(_, ratio)| *ratio < minimum)
        .map(|(name, ratio)| format!("{name}: {ratio:.2}"))
        .collect()
}

#[test]
fn text_pairs_meet_aa() {
    let failing = failures(4.5);
    assert!(failing.is_empty(), "below AA: {failing:?}");
}
