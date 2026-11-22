//! gpui-kit's global theme as a projection of the shell's theme, which stays its owner (D6).

mod highlight;

use gpui::{App, Hsla};
use gpui_kit::component::{Theme as KitTheme, ThemeMode as KitMode};
use std::sync::Arc;

use gpui_kit::component::highlighter::HighlightThemeStyle;
use highlight::highlight_style;
use ui::{Appearance, Surface, Theme};

pub(crate) fn follow_shell(cx: &mut App) {
    cx.observe_global::<Theme>(install).detach();
    cx.defer(install);
}

pub(crate) fn ink(cx: &App) -> Hsla {
    let theme = KitTheme::global(cx);
    theme
        .highlight_theme
        .style
        .editor_foreground
        .unwrap_or(theme.foreground)
}

fn install(cx: &mut App) {
    if !cx.has_global::<Theme>() {
        return;
    }
    let mode = match Theme::of(cx).appearance() {
        Appearance::Light => KitMode::Light,
        Appearance::Dark => KitMode::Dark,
    };
    KitTheme::change(mode, None, cx);
    match highlight_style(Theme::of(cx).palette(Surface::Window)) {
        Ok(style) => paint_with(style, cx),
        Err(error) => tracing::warn!("editor highlight colours: {error}"),
    }
}

fn paint_with(
    style: HighlightThemeStyle,
    cx: &mut App,
) {
    KitTheme::update(cx, |theme| {
        let mut highlight = (*theme.highlight_theme).clone();
        highlight.style = style;
        theme.highlight_theme = Arc::new(highlight);
    });
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use ui::ThemeMode;

    use super::*;

    fn drawn_dark(theme: &KitTheme) -> (bool, bool) {
        (theme.is_dark(), theme.highlight_theme.appearance.is_dark())
    }

    fn painted_and_projected(cx: &App) -> (HighlightThemeStyle, HighlightThemeStyle) {
        let palette = Theme::of(cx).palette(Surface::Window);
        let painted = KitTheme::global(cx).highlight_theme.style.clone();
        (painted, highlight_style(palette).unwrap())
    }

    #[gpui::test]
    fn the_editor_paints_develop_s_token_colours_in_either_mode(cx: &mut TestAppContext) {
        cx.update(|cx| {
            Theme::default().install(cx);
            crate::init(cx);
        });
        for mode in [ThemeMode::Dark, ThemeMode::Light] {
            cx.update(|cx| Theme::default().with_mode(mode).install(cx));
            let (painted, expected) = cx.read(painted_and_projected);
            assert_eq!(painted, expected, "{mode:?}");
        }
    }

    #[gpui::test]
    fn the_editor_s_ink_reads_on_its_light_or_dark_ground(cx: &mut TestAppContext) {
        const CASES: [(ThemeMode, bool); 2] = [(ThemeMode::Dark, true), (ThemeMode::Light, false)];
        cx.update(|cx| {
            Theme::default().install(cx);
            crate::init(cx);
        });
        for (mode, light_ink) in CASES {
            cx.update(|cx| Theme::default().with_mode(mode).install(cx));
            assert_eq!(cx.read(|cx| ink(cx).l > 0.5), light_ink, "{mode:?}");
        }
    }

    #[gpui::test]
    fn a_shell_theme_installed_in_the_update_that_starts_the_editor_is_followed(
        cx: &mut TestAppContext
    ) {
        cx.update(|cx| {
            Theme::default().with_mode(ThemeMode::Light).install(cx);
            crate::init(cx);
            Theme::default().with_mode(ThemeMode::Dark).install(cx);
        });
        assert_eq!(cx.read(|cx| drawn_dark(KitTheme::global(cx))), (true, true));
    }

    #[gpui::test]
    fn the_editor_theme_follows_the_shell_s_light_and_dark_switch(cx: &mut TestAppContext) {
        const CASES: [(ThemeMode, bool); 3] = [
            (ThemeMode::Light, false),
            (ThemeMode::Dark, true),
            (ThemeMode::Light, false),
        ];
        cx.update(|cx| {
            Theme::default().install(cx);
            crate::init(cx);
        });
        for (mode, dark) in CASES {
            cx.update(|cx| Theme::default().with_mode(mode).install(cx));
            assert_eq!(
                cx.read(|cx| drawn_dark(KitTheme::global(cx))),
                (dark, dark),
                "{mode:?}"
            );
        }
    }
}
