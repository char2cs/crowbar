use super::*;
use crate::surface::Surface;
use crate::theme::Theme;

#[test]
fn heights_follow_the_spec() {
    assert!((TabsStyle::Pill.tab_height() - 32.0).abs() < f32::EPSILON);
    assert!((TabsStyle::Segmented.tab_height() - 30.0).abs() < f32::EPSILON);
}

#[test]
fn selected_text_colour_differs_per_style() {
    let palette = Theme::default().palette(Surface::Window);
    assert_eq!(
        TabsStyle::Pill.active_text(palette),
        palette.control_foreground()
    );
    assert_eq!(
        TabsStyle::Segmented.active_text(palette),
        palette.foreground()
    );
}
