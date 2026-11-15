use protocol::ThemeTokens;
use ui::{Appearance, Theme, ThemeId};

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

#[cfg(test)]
mod tests {
    use ui::ThemeMode;

    use super::*;

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
