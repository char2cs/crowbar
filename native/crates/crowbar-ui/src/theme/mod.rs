//! The Crowbar design system's value layer: sealed tokens and the [`Theme`]
//! that holds them.
//!
//! This directory is the *only* place in the workspace allowed to construct a
//! colour. [`token`] explains the seal; [`generated`] carries the two value
//! tables transcribed from the React app's `theme.css`; and
//! `scripts/check-invariants.sh` rule 4 closes the hole the seal cannot, by
//! banning raw `gpui` colour construction everywhere outside this directory.

mod generated;
mod token;

pub use generated::Theme;
pub use token::{Color, Duration, FontFamily, FontSize, Radius, Scale, Space};

/// Which of the two token tables is in force.
///
/// The React app spells this as a `dark` class on the document element, which
/// is one element, not two — a token declared only in `:root` but referring to
/// one that `.dark` overrides still picks up the dark value. The two tables
/// were resolved with that in mind, so choosing between them here is the whole
/// of appearance switching.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, Hash)]
pub enum Appearance {
    /// `:root` alone.
    Light,
    /// `:root` with `.dark` layered over it.
    #[default]
    Dark,
}

impl Theme {
    /// The token table for an appearance.
    #[must_use]
    pub fn for_appearance(appearance: Appearance) -> Self {
        match appearance {
            Appearance::Light => Self::LIGHT,
            Appearance::Dark => Self::DARK,
        }
    }
}

#[cfg(test)]
mod tests {
    use gpui::{Rgba, rgb};

    use super::generated::{DUAL_DECLARED, TOKEN_VARIANCE};
    use super::{Appearance, Color, Theme};

    /// Every token in `theme.css`, plus the three the file tree declares in its
    /// own stylesheet.
    const TOKEN_COUNT: usize = 183;

    /// `theme.css` declares 74 tokens in both `:root` and `.dark`, but ten of
    /// those declare the *same* value in both, and sixty tokens that are
    /// declared once still vary because they alias one that does. 64 + 60.
    const VARYING: usize = 124;

    /// Every token name paired with whether the two tables disagree about it.
    fn variance() -> Vec<(&'static str, bool)> {
        TOKEN_VARIANCE
            .iter()
            .map(|(name, differs)| (*name, differs(&Theme::LIGHT, &Theme::DARK)))
            .collect()
    }

    fn assert_hex(got: Color, want: u32) {
        let got = Rgba::from(got.value());
        let want: Rgba = rgb(want);
        let tolerance = 0.6 / 255.0;
        for (channel, g, w) in [
            ("r", got.r, want.r),
            ("g", got.g, want.g),
            ("b", got.b, want.b),
        ] {
            assert!(
                (g - w).abs() < tolerance,
                "{channel}: got {g}, want {w} ({got:?} vs {want:?})"
            );
        }
    }

    #[test]
    fn carries_every_token() {
        let rows = variance();
        assert_eq!(rows.len(), TOKEN_COUNT);

        let mut names: Vec<&str> = rows.iter().map(|(name, _)| *name).collect();
        names.sort_unstable();
        let before = names.len();
        names.dedup();
        assert_eq!(names.len(), before, "a token name is emitted twice");
        assert!(names.iter().all(|n| n.starts_with("--")));
    }

    #[test]
    fn the_two_tables_differ_exactly_where_the_css_does() {
        let rows = variance();
        let varying = rows.iter().filter(|(_, differs)| *differs).count();
        assert_eq!(varying, VARYING);
        assert_ne!(Theme::LIGHT, Theme::DARK);
    }

    /// The dual-declared names are a real subset of the emitted tokens: if the
    /// generator ever dropped one of the tokens `theme.css` overrides per
    /// appearance, this is where it shows up.
    #[test]
    fn every_dual_declared_token_was_emitted() {
        let rows = variance();
        for name in DUAL_DECLARED {
            assert!(
                rows.iter().any(|(emitted, _)| *emitted == name),
                "{name} is declared in both :root and .dark but was not emitted"
            );
        }
    }

    /// Spot checks against values that are legible in `theme.css` itself, so a
    /// wholesale colour-pipeline error cannot hide behind "it is generated".
    #[test]
    fn resolves_the_var_chains_to_the_right_colours() {
        // `--foreground: var(--color-neutral-800)` -> Tailwind's #262626.
        assert_hex(Theme::LIGHT.foreground, 0x0026_2626);
        // `.dark` swaps it for `--color-neutral-100` -> #f5f5f5.
        assert_hex(Theme::DARK.foreground, 0x00f5_f5f5);
        // `--card: var(--color-white)` in light, `var(--background)` in dark.
        assert_hex(Theme::LIGHT.card, 0x00ff_ffff);
        assert_eq!(Theme::DARK.card, Theme::DARK.background);
        // A literal hex, straight through: `--syntax-keyword` in dark.
        assert_hex(Theme::DARK.syntax_keyword, 0x00d9_7757);
        // `@theme inline` aliases resolve to their base token, per appearance.
        assert_eq!(Theme::LIGHT.color_background, Theme::LIGHT.background);
        assert_eq!(Theme::DARK.color_background, Theme::DARK.background);
    }

    /// `--radius-*` are `calc(var(--radius) * k)` over a `0.625rem` base, at the
    /// browser's 16px root.
    #[test]
    fn folds_the_radius_scale() {
        let base = Theme::LIGHT.radius.value();
        assert_eq!(base, gpui::px(10.0));
        assert_eq!(Theme::LIGHT.radius_sm.value(), base * 0.6);
        assert_eq!(Theme::LIGHT.radius_lg.value(), base);
        assert_eq!(Theme::LIGHT.radius_4xl.value(), base * 2.6);
    }

    #[test]
    fn keeps_type_sizes_in_rems_so_the_ui_scale_still_applies() {
        assert_eq!(Theme::LIGHT.ui_text_xs.value(), gpui::rems(0.6875));
        assert_eq!(Theme::LIGHT.ui_text_base.value(), gpui::rems(0.875));
        assert!((Theme::LIGHT.app_ui_scale.value() - 1.0).abs() < f32::EPSILON);
    }

    #[test]
    fn pulls_the_duration_out_of_the_animation_shorthand() {
        assert_eq!(
            Theme::LIGHT.animate_toast_success_odd.value().as_millis(),
            320
        );
        assert_eq!(
            Theme::LIGHT.animate_toast_error_odd.value().as_millis(),
            280
        );
        assert_eq!(Theme::LIGHT.animate_label_in.value().as_millis(), 140);
    }

    #[test]
    fn records_font_stacks_in_fallback_order() {
        assert_eq!(Theme::LIGHT.font_sans.value(), &["CalSansUI", "sans-serif"]);
        assert_eq!(Theme::LIGHT.font_sans.primary(), Some("CalSansUI"));
        // `--font-editor: var(--editor-font-family)` has no CSS-level fallback:
        // the value is supplied by Settings at runtime, so there is none here.
        assert!(Theme::LIGHT.font_editor.value().is_empty());
        assert_eq!(Theme::LIGHT.font_editor.primary(), None);
    }

    /// The generated table and `crowbar_core::color::color_mix` are two
    /// independent routes to the same value — one resolved from `theme.css` at
    /// generation time, one computed at runtime from a token. They have to
    /// agree, or a component that mixes on the fly will drift away from a
    /// component that reads the token.
    #[test]
    fn mixing_a_token_reproduces_the_mixed_token() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            assert_eq!(
                theme.accent.mix(68.0, Color::TRANSPARENT),
                theme.file_tree_hover_bg,
            );
            assert_eq!(
                theme.muted_foreground.mix(18.0, Color::TRANSPARENT),
                theme.tree_guide_color,
            );
        }
    }

    /// `color-mix(in srgb, var(--accent) 42%, var(--border))` — the file tree's
    /// focus ring. Both operands are translucent, so this exercises the mix of
    /// two visible colours rather than the alpha-scaling special case.
    #[test]
    fn mixes_two_theme_colours() {
        let mixed = Theme::DARK.accent.mix(42.0, Theme::DARK.border);
        let alpha = Theme::DARK
            .accent
            .value()
            .a
            .mul_add(0.42, Theme::DARK.border.value().a * 0.58);
        assert!((mixed.value().a - alpha).abs() < 1.0 / 255.0);
        // Both operands are white in the dark theme, so the mix stays white.
        assert!((mixed.value().l - 1.0).abs() < 1.0 / 255.0);
    }

    #[test]
    fn appearance_selects_the_table() {
        assert_eq!(Theme::for_appearance(Appearance::Light), Theme::LIGHT);
        assert_eq!(Theme::for_appearance(Appearance::Dark), Theme::DARK);
        assert_eq!(Appearance::default(), Appearance::Dark);
    }
}
