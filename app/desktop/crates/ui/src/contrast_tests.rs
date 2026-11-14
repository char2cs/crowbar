use gpui::rgba;

use super::*;
use crate::appearance::Appearance;
use crate::surface::Surface;
use crate::theme::Theme;
use crate::theme_id::ThemeId;
use crate::theme_mode::ThemeMode;

#[test]
fn black_on_white_is_twenty_one() {
    let ratio = contrast_ratio(rgba(0x000000ff).into(), rgba(0xffffffff).into());
    assert!((ratio - 21.0).abs() < 0.01, "ratio {ratio}");
}

#[test]
fn translucent_foreground_is_composited() {
    let half = contrast_ratio(rgba(0x00000080).into(), rgba(0xffffffff).into());
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

#[test]
fn text_pairs_meet_aa() {
    const MINIMUM: f32 = 4.5;
    for (id, appearance) in themes() {
        for surface in [Surface::Window, Surface::Chrome] {
            let palette = theme_for(id, appearance).palette(surface);
            let pairs = [
                ("foreground/background", palette.foreground(), palette.background()),
                ("muted_foreground/background", palette.muted_foreground(), palette.background()),
                ("primary_foreground/primary", palette.primary_foreground(), palette.primary()),
                ("control_foreground/control_surface", palette.control_foreground(), palette.control_surface()),
                ("foreground/popover", palette.foreground(), palette.popover()),
                ("destructive/background", palette.destructive(), palette.background()),
            ];
            for (name, foreground, background) in pairs {
                let ratio = contrast_ratio(foreground, background);
                assert!(ratio >= MINIMUM, "{id:?} {appearance:?} {surface:?} {name}: {ratio:.2}");
            }
        }
    }
}
