use super::*;

#[test]
fn light_and_dark_palettes_differ() {
    assert_ne!(
        Palette::CROWBAR_LIGHT.background(),
        Palette::CROWBAR_DARK.background()
    );
    assert_ne!(
        Palette::FROSTED_LIGHT.background(),
        Palette::FROSTED_DARK.background()
    );
}

#[test]
fn text_is_darker_than_the_page_in_light_mode() {
    let palette = Palette::CROWBAR_LIGHT;
    assert!(palette.foreground().l < palette.background().l);
}
