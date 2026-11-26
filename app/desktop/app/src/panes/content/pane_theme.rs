use protocol::ThemeTokens;
use ui::{Appearance, Surface, Theme, ThemeId};

const THEME_ID: &str = "theme-id";
const THEME_TYPE: &str = "theme-type";

fn id_name(id: ThemeId) -> &'static str {
    match id {
        ThemeId::Frosted => "frosted",
        _ => "crowbar",
    }
}

fn type_name(appearance: Appearance) -> &'static str {
    match appearance {
        Appearance::Light => "light",
        Appearance::Dark => "dark",
    }
}

pub(crate) fn tokens_of(theme: Theme) -> protocol::Result<ThemeTokens> {
    ThemeTokens::default()
        .with(THEME_ID, id_name(theme.id()))?
        .with(THEME_TYPE, type_name(theme.appearance()))
}

pub(crate) fn surface_color(theme: Theme) -> webview::Color {
    let rgba = gpui::Rgba::from(theme.palette(Surface::Window).pane_background());
    let [red, green, blue, alpha] = u32::from(rgba).to_be_bytes();
    webview::Color {
        red,
        green,
        blue,
        alpha,
    }
}

#[cfg(test)]
mod tests {
    use ui::ThemeMode;

    use super::*;

    #[test]
    fn the_surface_colour_is_the_pane_background_of_the_resolved_mode() {
        let light = Theme::default().with_mode(ThemeMode::Light);
        let dark = Theme::default().with_mode(ThemeMode::Dark);
        let rgb = |theme| {
            let color = surface_color(theme);
            (color.red, color.green, color.blue, color.alpha)
        };
        assert_eq!(rgb(light), (0xfc, 0xfc, 0xfa, 255), "light");
        assert_eq!(rgb(dark), (0x1c, 0x1c, 0x1a, 255), "dark");
    }

    #[test]
    fn the_tokens_name_the_theme_and_its_resolved_mode() {
        let theme = Theme::default()
            .with_id(ThemeId::Frosted)
            .with_mode(ThemeMode::Light);
        let tokens = tokens_of(theme).unwrap();
        assert_eq!(
            (tokens.get("theme-id"), tokens.get("theme-type")),
            (Some("frosted"), Some("light")),
            "tokens"
        );
    }
}
