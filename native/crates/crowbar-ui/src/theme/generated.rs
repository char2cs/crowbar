//! The Crowbar design tokens, resolved from the React app's `theme.css`.
//!
//! **Generated file — do not edit.** Regenerate with
//! `python3 crates/crowbar-ui/tools/gen-theme.py` after changing
//! `web/src/styles/theme.css`.
//!
//! The React app carries 180 distinct token names across 254
//! declaration lines: 74 are declared in both `:root` and `.dark` and so
//! carry a light *and* a dark value, and 106 are declared once and are
//! theme-invariant. That is why [`Theme`] is 183 fields and two `const`
//! tables rather than 254 flat fields — the light/dark split is in the
//! tables, not in the field list. The last 3 fields come from the file
//! tree's own stylesheet, which declares tokens the first components need.
//!
//! Every value here was resolved the way a browser resolves it: `var()` chains
//! followed to their source, `oklch()` converted through `OKLab` to sRGB and then
//! to gpui's `Hsla`, `color-mix()` evaluated with premultiplied alpha, `calc()`
//! folded, and `rem` taken at the browser root of 16px.

use gpui::{Hsla, Rems, px};
use std::time::Duration as StdDuration;

use super::token::{Color, Duration, FontFamily, FontSize, Radius, Scale, Space};

/// Every Crowbar design token, resolved for one appearance.
///
/// Construct nothing: use [`Theme::LIGHT`], [`Theme::DARK`] or
/// [`Theme::for_appearance`]. The fields are sealed newtypes, so a view crate
/// can read a token and cannot mint one.
#[derive(Clone, Debug, PartialEq)]
pub struct Theme {
    /// `--font-sans`: `var(--app-font-family, 'CalSansUI', sans-serif)`
    pub font_sans: FontFamily,
    /// `--font-heading`: `var(--app-font-family, 'CalSans', sans-serif)`
    pub font_heading: FontFamily,
    /// `--font-mono`: `var(--editor-font-family, 'JetBrains Mono Variable', ui-monospace, monospace)`
    pub font_mono: FontFamily,
    /// `--color-background`: `var(--background)`
    pub color_background: Color,
    /// `--color-pane-background`: `var(--pane-background)`
    pub color_pane_background: Color,
    /// `--color-foreground`: `var(--foreground)`
    pub color_foreground: Color,
    /// `--color-card`: `var(--card)`
    pub color_card: Color,
    /// `--color-card-foreground`: `var(--card-foreground)`
    pub color_card_foreground: Color,
    /// `--color-popover`: `var(--popover)`
    pub color_popover: Color,
    /// `--color-popover-foreground`: `var(--popover-foreground)`
    pub color_popover_foreground: Color,
    /// `--color-primary`: `var(--primary)`
    pub color_primary: Color,
    /// `--color-primary-foreground`: `var(--primary-foreground)`
    pub color_primary_foreground: Color,
    /// `--color-secondary`: `var(--secondary)`
    pub color_secondary: Color,
    /// `--color-secondary-foreground`: `var(--secondary-foreground)`
    pub color_secondary_foreground: Color,
    /// `--color-muted`: `var(--muted)`
    pub color_muted: Color,
    /// `--color-muted-foreground`: `var(--muted-foreground)`
    pub color_muted_foreground: Color,
    /// `--color-accent`: `var(--accent)`
    pub color_accent: Color,
    /// `--color-accent-foreground`: `var(--accent-foreground)`
    pub color_accent_foreground: Color,
    /// `--color-destructive`: `var(--destructive)`
    pub color_destructive: Color,
    /// `--color-destructive-foreground`: `var(--destructive-foreground)`
    pub color_destructive_foreground: Color,
    /// `--color-border`: `var(--border)`
    pub color_border: Color,
    /// `--color-input`: `var(--input)`
    pub color_input: Color,
    /// `--color-ring`: `var(--ring)`
    pub color_ring: Color,
    /// `--color-chart-1`: `var(--chart-1)`
    pub color_chart_1: Color,
    /// `--color-chart-2`: `var(--chart-2)`
    pub color_chart_2: Color,
    /// `--color-chart-3`: `var(--chart-3)`
    pub color_chart_3: Color,
    /// `--color-chart-4`: `var(--chart-4)`
    pub color_chart_4: Color,
    /// `--color-chart-5`: `var(--chart-5)`
    pub color_chart_5: Color,
    /// `--color-sidebar`: `var(--sidebar)`
    pub color_sidebar: Color,
    /// `--color-sidebar-foreground`: `var(--sidebar-foreground)`
    pub color_sidebar_foreground: Color,
    /// `--color-sidebar-primary`: `var(--sidebar-primary)`
    pub color_sidebar_primary: Color,
    /// `--color-sidebar-primary-foreground`: `var(--sidebar-primary-foreground)`
    pub color_sidebar_primary_foreground: Color,
    /// `--color-sidebar-accent`: `var(--sidebar-accent)`
    pub color_sidebar_accent: Color,
    /// `--color-sidebar-accent-foreground`: `var(--sidebar-accent-foreground)`
    pub color_sidebar_accent_foreground: Color,
    /// `--color-sidebar-border`: `var(--sidebar-border)`
    pub color_sidebar_border: Color,
    /// `--color-sidebar-ring`: `var(--sidebar-ring)`
    pub color_sidebar_ring: Color,
    /// `--color-sidebar-element-idle`: `var(--sidebar-element-idle)`
    pub color_sidebar_element_idle: Color,
    /// `--color-sidebar-element-hover`: `var(--sidebar-element-hover)`
    pub color_sidebar_element_hover: Color,
    /// `--color-sidebar-element-press`: `var(--sidebar-element-press)`
    pub color_sidebar_element_press: Color,
    /// `--color-success`: `var(--success)`
    pub color_success: Color,
    /// `--color-success-foreground`: `var(--success-foreground)`
    pub color_success_foreground: Color,
    /// `--color-warning`: `var(--warning)`
    pub color_warning: Color,
    /// `--color-warning-foreground`: `var(--warning-foreground)`
    pub color_warning_foreground: Color,
    /// `--color-info`: `var(--info)`
    pub color_info: Color,
    /// `--color-info-foreground`: `var(--info-foreground)`
    pub color_info_foreground: Color,
    /// `--color-code`: `var(--code)`
    pub color_code: Color,
    /// `--color-code-foreground`: `var(--code-foreground)`
    pub color_code_foreground: Color,
    /// `--color-code-highlight`: `var(--code-highlight)`
    pub color_code_highlight: Color,
    /// `--color-chrome-bg`: `var(--chrome-bg)`
    pub color_chrome_bg: Color,
    /// `--color-git-added`: `var(--git-added)`
    pub color_git_added: Color,
    /// `--color-git-deleted`: `var(--git-deleted)`
    pub color_git_deleted: Color,
    /// `--color-git-modified`: `var(--git-modified)`
    pub color_git_modified: Color,
    /// `--color-git-modified-staged`: `var(--git-modified-staged)`
    pub color_git_modified_staged: Color,
    /// `--color-git-untracked`: `var(--git-untracked)`
    pub color_git_untracked: Color,
    /// `--color-git-renamed`: `var(--git-renamed)`
    pub color_git_renamed: Color,
    /// `--font-editor`: `var(--editor-font-family)`
    pub font_editor: FontFamily,
    /// `--radius-sm`: `calc(var(--radius) * 0.6)`
    pub radius_sm: Radius,
    /// `--radius-md`: `calc(var(--radius) * 0.8)`
    pub radius_md: Radius,
    /// `--radius-lg`: `var(--radius)`
    pub radius_lg: Radius,
    /// `--radius-xl`: `calc(var(--radius) * 1.4)`
    pub radius_xl: Radius,
    /// `--radius-2xl`: `calc(var(--radius) * 1.8)`
    pub radius_2xl: Radius,
    /// `--radius-3xl`: `calc(var(--radius) * 2.2)`
    pub radius_3xl: Radius,
    /// `--radius-4xl`: `calc(var(--radius) * 2.6)`
    pub radius_4xl: Radius,
    /// `--animate-skeleton`: `skeleton 2s -1s infinite linear`
    pub animate_skeleton: Duration,
    /// `--animate-caret-blink`: `1s ease-out infinite caret-blink`
    pub animate_caret_blink: Duration,
    /// `--animate-toast-success-odd`: `toast-success-odd 0.32s cubic-bezier(0.5, 1, 0.89, 1)`
    pub animate_toast_success_odd: Duration,
    /// `--animate-toast-success-even`: `toast-success-even 0.32s cubic-bezier(0.5, 1, 0.89, 1)`
    pub animate_toast_success_even: Duration,
    /// `--animate-toast-error-odd`: `toast-error-odd 0.28s cubic-bezier(0.5, 1, 0.89, 1)`
    pub animate_toast_error_odd: Duration,
    /// `--animate-toast-error-even`: `toast-error-even 0.28s cubic-bezier(0.5, 1, 0.89, 1)`
    pub animate_toast_error_even: Duration,
    /// `--animate-label-in`: `label-in 140ms cubic-bezier(0, 0, 0.2, 1) both`
    pub animate_label_in: Duration,
    /// `--radius`: `0.625rem`
    pub radius: Radius,
    /// `--background`: `oklch(0.994 0.001 106.4)`
    pub background: Color,
    /// `--pane-background`: `oklch(0.994 0.001 106.4)`
    pub pane_background: Color,
    /// `--foreground`: `var(--color-neutral-800)`
    pub foreground: Color,
    /// `--card`: `var(--color-white)`
    pub card: Color,
    /// `--card-foreground`: `var(--color-neutral-800)`
    pub card_foreground: Color,
    /// `--popover`: `var(--color-white)`
    pub popover: Color,
    /// `--popover-foreground`: `var(--color-neutral-800)`
    pub popover_foreground: Color,
    /// `--primary`: `oklch(0.49 0.082 130)`
    pub primary: Color,
    /// `--primary-foreground`: `oklch(0.98 0.027 98)`
    pub primary_foreground: Color,
    /// `--secondary`: `oklch(0.753 0.054 112.4)`
    pub secondary: Color,
    /// `--secondary-foreground`: `oklch(0.98 0.027 98)`
    pub secondary_foreground: Color,
    /// `--muted`: `oklch(0 0 0 / 4%)`
    pub muted: Color,
    /// `--muted-foreground`: `oklch(0.4 0 0)`
    pub muted_foreground: Color,
    /// `--accent`: `oklch(0 0 0 / 4%)`
    pub accent: Color,
    /// `--accent-foreground`: `var(--color-neutral-800)`
    pub accent_foreground: Color,
    /// `--editor-selection`: `oklch(0.62 0.13 235 / 0.3)`
    pub editor_selection: Color,
    /// `--destructive`: `var(--color-red-500)`
    pub destructive: Color,
    /// `--destructive-foreground`: `var(--color-red-700)`
    pub destructive_foreground: Color,
    /// `--info`: `var(--color-blue-500)`
    pub info: Color,
    /// `--info-foreground`: `var(--color-blue-700)`
    pub info_foreground: Color,
    /// `--success`: `var(--color-emerald-500)`
    pub success: Color,
    /// `--success-foreground`: `var(--color-emerald-700)`
    pub success_foreground: Color,
    /// `--warning`: `var(--color-amber-500)`
    pub warning: Color,
    /// `--warning-foreground`: `var(--color-amber-700)`
    pub warning_foreground: Color,
    /// `--border`: `oklch(0 0 0 / 8%)`
    pub border: Color,
    /// `--input`: `oklch(0 0 0 / 10%)`
    pub input: Color,
    /// `--ring`: `var(--color-neutral-400)`
    pub ring: Color,
    /// `--chart-1`: `var(--color-orange-600)`
    pub chart_1: Color,
    /// `--chart-2`: `var(--color-teal-600)`
    pub chart_2: Color,
    /// `--chart-3`: `var(--color-cyan-900)`
    pub chart_3: Color,
    /// `--chart-4`: `var(--color-amber-400)`
    pub chart_4: Color,
    /// `--chart-5`: `var(--color-amber-500)`
    pub chart_5: Color,
    /// `--sidebar`: `oklch(0 0 0 / 0%)`
    pub sidebar: Color,
    /// `--sidebar-foreground`: `var(--color-neutral-800)`
    pub sidebar_foreground: Color,
    /// `--sidebar-primary`: `var(--color-neutral-800)`
    pub sidebar_primary: Color,
    /// `--sidebar-primary-foreground`: `var(--color-neutral-50)`
    pub sidebar_primary_foreground: Color,
    /// `--sidebar-accent`: `oklch(0 0 0 / 4%)`
    pub sidebar_accent: Color,
    /// `--sidebar-accent-foreground`: `var(--color-neutral-800)`
    pub sidebar_accent_foreground: Color,
    /// `--sidebar-border`: `oklch(0 0 0 / 6%)`
    pub sidebar_border: Color,
    /// `--sidebar-ring`: `var(--color-neutral-400)`
    pub sidebar_ring: Color,
    /// `--sidebar-element-idle`: `color-mix(in oklch, var(--foreground) 6%, transparent)`
    pub sidebar_element_idle: Color,
    /// `--sidebar-element-hover`: `oklch(0 0 0 / 8%)`
    pub sidebar_element_hover: Color,
    /// `--sidebar-element-press`: `color-mix(in oklch, var(--foreground) 8%, transparent)`
    pub sidebar_element_press: Color,
    /// `--code`: `var(--color-white)`
    pub code: Color,
    /// `--code-foreground`: `var(--foreground)`
    pub code_foreground: Color,
    /// `--code-highlight`: `oklch(0 0 0 / 4%)`
    pub code_highlight: Color,
    /// `--git-modified`: `var(--warning)`
    pub git_modified: Color,
    /// `--git-modified-staged`: `var(--success)`
    pub git_modified_staged: Color,
    /// `--git-added`: `var(--success)`
    pub git_added: Color,
    /// `--git-untracked`: `var(--color-lime-600)`
    pub git_untracked: Color,
    /// `--git-deleted`: `var(--destructive)`
    pub git_deleted: Color,
    /// `--git-renamed`: `var(--color-sky-600)`
    pub git_renamed: Color,
    /// `--chrome-bg`: `color-mix(in oklch, var(--color-neutral-50) 45%, transparent)`
    pub chrome_bg: Color,
    /// `--app-ui-scale`: `1`
    pub app_ui_scale: Scale,
    /// `--ui-text-xs`: `calc(0.6875rem * var(--app-ui-scale))`
    pub ui_text_xs: FontSize,
    /// `--ui-text-sm`: `calc(0.75rem * var(--app-ui-scale))`
    pub ui_text_sm: FontSize,
    /// `--ui-text-base`: `calc(0.875rem * var(--app-ui-scale))`
    pub ui_text_base: FontSize,
    /// `--ui-text-lg`: `calc(1rem * var(--app-ui-scale))`
    pub ui_text_lg: FontSize,
    /// `--ui-text-xl`: `calc(1.25rem * var(--app-ui-scale))`
    pub ui_text_xl: FontSize,
    /// `--app-scrollbar-size`: `11px`
    pub app_scrollbar_size: Space,
    /// `--app-scrollbar-thumb`: `oklch(0.55 0 0 / 42%)`
    pub app_scrollbar_thumb: Color,
    /// `--app-scrollbar-thumb-border`: `3px solid transparent`
    pub app_scrollbar_thumb_border: Space,
    /// `--app-scrollbar-thumb-hover`: `oklch(0.55 0 0 / 58%)`
    pub app_scrollbar_thumb_hover: Color,
    /// `--app-scrollbar-track`: `transparent`
    pub app_scrollbar_track: Color,
    /// `--app-scrollbar-radius`: `999px`
    pub app_scrollbar_radius: Radius,
    /// `--syntax-comment`: `var(--muted-foreground)`
    pub syntax_comment: Color,
    /// `--syntax-variable`: `var(--foreground)`
    pub syntax_variable: Color,
    /// `--syntax-punctuation`: `var(--muted-foreground)`
    pub syntax_punctuation: Color,
    /// `--syntax-operator`: `var(--muted-foreground)`
    pub syntax_operator: Color,
    /// `--syntax-error`: `var(--destructive)`
    pub syntax_error: Color,
    /// `--syntax-keyword`: `#b26045`
    pub syntax_keyword: Color,
    /// `--syntax-string`: `#578141`
    pub syntax_string: Color,
    /// `--syntax-number`: `#976f30`
    pub syntax_number: Color,
    /// `--syntax-constant`: `#368189`
    pub syntax_constant: Color,
    /// `--syntax-function`: `#2f6fae`
    pub syntax_function: Color,
    /// `--syntax-type`: `#8257a8`
    pub syntax_type: Color,
    /// `--syntax-property`: `#5a564d`
    pub syntax_property: Color,
    /// `--syntax-tag`: `#2f6fae`
    pub syntax_tag: Color,
    /// `--syntax-attribute`: `#976f30`
    pub syntax_attribute: Color,
    /// `--syntax-boolean`: `#368189`
    pub syntax_boolean: Color,
    /// `--syntax-null`: `#8257a8`
    pub syntax_null: Color,
    /// `--syntax-regex`: `#368189`
    pub syntax_regex: Color,
    /// `--syntax-jsx`: `#2f6fae`
    pub syntax_jsx: Color,
    /// `--syntax-jsx-attribute`: `#976f30`
    pub syntax_jsx_attribute: Color,
    /// `--syntax-markdown-heading`: `#2f6fae`
    pub syntax_markdown_heading: Color,
    /// `--syntax-markdown-bold`: `#976f30`
    pub syntax_markdown_bold: Color,
    /// `--syntax-markdown-italic`: `#b26045`
    pub syntax_markdown_italic: Color,
    /// `--syntax-markdown-strikethrough`: `var(--muted-foreground)`
    pub syntax_markdown_strikethrough: Color,
    /// `--syntax-markdown-link`: `#2f6fae`
    pub syntax_markdown_link: Color,
    /// `--syntax-markdown-link-text`: `#578141`
    pub syntax_markdown_link_text: Color,
    /// `--syntax-markdown-code`: `#578141`
    pub syntax_markdown_code: Color,
    /// `--syntax-markdown-list`: `#b26045`
    pub syntax_markdown_list: Color,
    /// `--syntax-markdown-quote`: `var(--muted-foreground)`
    pub syntax_markdown_quote: Color,
    /// `--terminal-black`: `oklch(0.27 0 0)`
    pub terminal_black: Color,
    /// `--terminal-red`: `var(--destructive)`
    pub terminal_red: Color,
    /// `--terminal-green`: `var(--success)`
    pub terminal_green: Color,
    /// `--terminal-yellow`: `var(--warning)`
    pub terminal_yellow: Color,
    /// `--terminal-blue`: `var(--info)`
    pub terminal_blue: Color,
    /// `--terminal-magenta`: `var(--syntax-type)`
    pub terminal_magenta: Color,
    /// `--terminal-cyan`: `var(--syntax-constant)`
    pub terminal_cyan: Color,
    /// `--terminal-white`: `var(--muted-foreground)`
    pub terminal_white: Color,
    /// `--terminal-bright-black`: `var(--muted-foreground)`
    pub terminal_bright_black: Color,
    /// `--terminal-bright-red`: `var(--destructive)`
    pub terminal_bright_red: Color,
    /// `--terminal-bright-green`: `var(--success)`
    pub terminal_bright_green: Color,
    /// `--terminal-bright-yellow`: `var(--warning)`
    pub terminal_bright_yellow: Color,
    /// `--terminal-bright-blue`: `var(--info)`
    pub terminal_bright_blue: Color,
    /// `--terminal-bright-magenta`: `var(--syntax-type)`
    pub terminal_bright_magenta: Color,
    /// `--terminal-bright-cyan`: `var(--syntax-constant)`
    pub terminal_bright_cyan: Color,
    /// `--terminal-bright-white`: `var(--foreground)`
    pub terminal_bright_white: Color,
    /// `--file-tree-hover-bg`: `color-mix(in srgb, var(--accent) 68%, transparent)`
    pub file_tree_hover_bg: Color,
    /// `--file-tree-guide-icon-offset`: `7px`
    pub file_tree_guide_icon_offset: Space,
    /// `--tree-guide-color`: `color-mix(in srgb, var(--muted-foreground) 18%, transparent)`
    pub tree_guide_color: Color,
}

impl Theme {
    /// The light appearance — `theme.css`'s `:root` block.
    pub const LIGHT: Self = Self {
        font_sans: FontFamily::seal(&["CalSansUI", "sans-serif"]),
        font_heading: FontFamily::seal(&["CalSans", "sans-serif"]),
        font_mono: FontFamily::seal(&["JetBrains Mono Variable", "ui-monospace", "monospace"]),
        color_background: Color::seal(Hsla {
            h: 0.166_602_54,
            s: 0.163_459_3,
            l: 0.990_881_9,
            a: 1.0,
        }),
        color_pane_background: Color::seal(Hsla {
            h: 0.166_602_54,
            s: 0.163_459_3,
            l: 0.990_881_9,
            a: 1.0,
        }),
        color_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.149_382_08,
            a: 1.0,
        }),
        color_card: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 1.0,
            a: 1.0,
        }),
        color_card_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.149_382_08,
            a: 1.0,
        }),
        color_popover: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 1.0,
            a: 1.0,
        }),
        color_popover_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.149_382_08,
            a: 1.0,
        }),
        color_primary: Color::seal(Hsla {
            h: 0.246_745_41,
            s: 0.324_235_02,
            l: 0.312_937_68,
            a: 1.0,
        }),
        color_primary_foreground: Color::seal(Hsla {
            h: 0.139_881_2,
            s: 0.858_157_63,
            l: 0.944_533_1,
            a: 1.0,
        }),
        color_secondary: Color::seal(Hsla {
            h: 0.183_558_82,
            s: 0.204_891_79,
            l: 0.624_652_86,
            a: 1.0,
        }),
        color_secondary_foreground: Color::seal(Hsla {
            h: 0.139_881_2,
            s: 0.858_157_63,
            l: 0.944_533_1,
            a: 1.0,
        }),
        color_muted: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.0,
            a: 0.04,
        }),
        color_muted_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.280_604_24,
            a: 1.0,
        }),
        color_accent: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.0,
            a: 0.04,
        }),
        color_accent_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.149_382_08,
            a: 1.0,
        }),
        color_destructive: Color::seal(Hsla {
            h: 0.991_516_65,
            s: 0.958_988_25,
            l: 0.577_229_26,
            a: 1.0,
        }),
        color_destructive_foreground: Color::seal(Hsla {
            h: 0.993_668_26,
            s: 1.0,
            l: 0.378_442_35,
            a: 1.0,
        }),
        color_border: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.0,
            a: 0.08,
        }),
        color_input: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.0,
            a: 0.1,
        }),
        color_ring: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.630_163_2,
            a: 1.0,
        }),
        color_chart_1: Color::seal(Hsla {
            h: 0.049_996_372,
            s: 1.0,
            l: 0.480_352_73,
            a: 1.0,
        }),
        color_chart_2: Color::seal(Hsla {
            h: 0.485_298_22,
            s: 1.0,
            l: 0.294_634_13,
            a: 1.0,
        }),
        color_chart_3: Color::seal(Hsla {
            h: 0.543_954_3,
            s: 0.721_388_1,
            l: 0.228_734_96,
            a: 1.0,
        }),
        color_chart_4: Color::seal(Hsla {
            h: 0.121_209_286,
            s: 1.0,
            l: 0.5,
            a: 1.0,
        }),
        color_chart_5: Color::seal(Hsla {
            h: 0.100_926_95,
            s: 1.0,
            l: 0.497_135_73,
            a: 1.0,
        }),
        color_sidebar: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.0,
            a: 0.0,
        }),
        color_sidebar_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.149_382_08,
            a: 1.0,
        }),
        color_sidebar_primary: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.149_382_08,
            a: 1.0,
        }),
        color_sidebar_primary_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.980_255_96,
            a: 1.0,
        }),
        color_sidebar_accent: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.0,
            a: 0.04,
        }),
        color_sidebar_accent_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.149_382_08,
            a: 1.0,
        }),
        color_sidebar_border: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.0,
            a: 0.06,
        }),
        color_sidebar_ring: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.630_163_2,
            a: 1.0,
        }),
        color_sidebar_element_idle: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.149_382_08,
            a: 0.06,
        }),
        color_sidebar_element_hover: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.0,
            a: 0.08,
        }),
        color_sidebar_element_press: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.149_382_08,
            a: 0.08,
        }),
        color_success: Color::seal(Hsla {
            h: 0.443_712_32,
            s: 1.0,
            l: 0.369_394_57,
            a: 1.0,
        }),
        color_success_foreground: Color::seal(Hsla {
            h: 0.449_579_33,
            s: 1.0,
            l: 0.239_109_77,
            a: 1.0,
        }),
        color_warning: Color::seal(Hsla {
            h: 0.100_926_95,
            s: 1.0,
            l: 0.497_135_73,
            a: 1.0,
        }),
        color_warning_foreground: Color::seal(Hsla {
            h: 0.068_558_78,
            s: 1.0,
            l: 0.365_894_4,
            a: 1.0,
        }),
        color_info: Color::seal(Hsla {
            h: 0.600_712_3,
            s: 1.0,
            l: 0.584_666_25,
            a: 1.0,
        }),
        color_info_foreground: Color::seal(Hsla {
            h: 0.625_958_74,
            s: 0.840_952_93,
            l: 0.489_841_1,
            a: 1.0,
        }),
        color_code: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 1.0,
            a: 1.0,
        }),
        color_code_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.149_382_08,
            a: 1.0,
        }),
        color_code_highlight: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.0,
            a: 0.04,
        }),
        color_chrome_bg: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.980_255_96,
            a: 0.45,
        }),
        color_git_added: Color::seal(Hsla {
            h: 0.443_712_32,
            s: 1.0,
            l: 0.369_394_57,
            a: 1.0,
        }),
        color_git_deleted: Color::seal(Hsla {
            h: 0.991_516_65,
            s: 0.958_988_25,
            l: 0.577_229_26,
            a: 1.0,
        }),
        color_git_modified: Color::seal(Hsla {
            h: 0.100_926_95,
            s: 1.0,
            l: 0.497_135_73,
            a: 1.0,
        }),
        color_git_modified_staged: Color::seal(Hsla {
            h: 0.443_712_32,
            s: 1.0,
            l: 0.369_394_57,
            a: 1.0,
        }),
        color_git_untracked: Color::seal(Hsla {
            h: 0.238_423_08,
            s: 1.0,
            l: 0.323_744_54,
            a: 1.0,
        }),
        color_git_renamed: Color::seal(Hsla {
            h: 0.561_322_57,
            s: 1.0,
            l: 0.409_899_83,
            a: 1.0,
        }),
        font_editor: FontFamily::seal(&[]),
        radius_sm: Radius::seal(px(6.0)),
        radius_md: Radius::seal(px(8.0)),
        radius_lg: Radius::seal(px(10.0)),
        radius_xl: Radius::seal(px(14.0)),
        radius_2xl: Radius::seal(px(18.0)),
        radius_3xl: Radius::seal(px(22.0)),
        radius_4xl: Radius::seal(px(26.0)),
        animate_skeleton: Duration::seal(StdDuration::from_secs(2)),
        animate_caret_blink: Duration::seal(StdDuration::from_secs(1)),
        animate_toast_success_odd: Duration::seal(StdDuration::from_millis(320)),
        animate_toast_success_even: Duration::seal(StdDuration::from_millis(320)),
        animate_toast_error_odd: Duration::seal(StdDuration::from_millis(280)),
        animate_toast_error_even: Duration::seal(StdDuration::from_millis(280)),
        animate_label_in: Duration::seal(StdDuration::from_millis(140)),
        radius: Radius::seal(px(10.0)),
        background: Color::seal(Hsla {
            h: 0.166_602_54,
            s: 0.163_459_3,
            l: 0.990_881_9,
            a: 1.0,
        }),
        pane_background: Color::seal(Hsla {
            h: 0.166_602_54,
            s: 0.163_459_3,
            l: 0.990_881_9,
            a: 1.0,
        }),
        foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.149_382_08,
            a: 1.0,
        }),
        card: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 1.0,
            a: 1.0,
        }),
        card_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.149_382_08,
            a: 1.0,
        }),
        popover: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 1.0,
            a: 1.0,
        }),
        popover_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.149_382_08,
            a: 1.0,
        }),
        primary: Color::seal(Hsla {
            h: 0.246_745_41,
            s: 0.324_235_02,
            l: 0.312_937_68,
            a: 1.0,
        }),
        primary_foreground: Color::seal(Hsla {
            h: 0.139_881_2,
            s: 0.858_157_63,
            l: 0.944_533_1,
            a: 1.0,
        }),
        secondary: Color::seal(Hsla {
            h: 0.183_558_82,
            s: 0.204_891_79,
            l: 0.624_652_86,
            a: 1.0,
        }),
        secondary_foreground: Color::seal(Hsla {
            h: 0.139_881_2,
            s: 0.858_157_63,
            l: 0.944_533_1,
            a: 1.0,
        }),
        muted: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.0,
            a: 0.04,
        }),
        muted_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.280_604_24,
            a: 1.0,
        }),
        accent: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.0,
            a: 0.04,
        }),
        accent_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.149_382_08,
            a: 1.0,
        }),
        editor_selection: Color::seal(Hsla {
            h: 0.548_130_8,
            s: 0.923_393_67,
            l: 0.409_010_4,
            a: 0.3,
        }),
        destructive: Color::seal(Hsla {
            h: 0.991_516_65,
            s: 0.958_988_25,
            l: 0.577_229_26,
            a: 1.0,
        }),
        destructive_foreground: Color::seal(Hsla {
            h: 0.993_668_26,
            s: 1.0,
            l: 0.378_442_35,
            a: 1.0,
        }),
        info: Color::seal(Hsla {
            h: 0.600_712_3,
            s: 1.0,
            l: 0.584_666_25,
            a: 1.0,
        }),
        info_foreground: Color::seal(Hsla {
            h: 0.625_958_74,
            s: 0.840_952_93,
            l: 0.489_841_1,
            a: 1.0,
        }),
        success: Color::seal(Hsla {
            h: 0.443_712_32,
            s: 1.0,
            l: 0.369_394_57,
            a: 1.0,
        }),
        success_foreground: Color::seal(Hsla {
            h: 0.449_579_33,
            s: 1.0,
            l: 0.239_109_77,
            a: 1.0,
        }),
        warning: Color::seal(Hsla {
            h: 0.100_926_95,
            s: 1.0,
            l: 0.497_135_73,
            a: 1.0,
        }),
        warning_foreground: Color::seal(Hsla {
            h: 0.068_558_78,
            s: 1.0,
            l: 0.365_894_4,
            a: 1.0,
        }),
        border: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.0,
            a: 0.08,
        }),
        input: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.0,
            a: 0.1,
        }),
        ring: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.630_163_2,
            a: 1.0,
        }),
        chart_1: Color::seal(Hsla {
            h: 0.049_996_372,
            s: 1.0,
            l: 0.480_352_73,
            a: 1.0,
        }),
        chart_2: Color::seal(Hsla {
            h: 0.485_298_22,
            s: 1.0,
            l: 0.294_634_13,
            a: 1.0,
        }),
        chart_3: Color::seal(Hsla {
            h: 0.543_954_3,
            s: 0.721_388_1,
            l: 0.228_734_96,
            a: 1.0,
        }),
        chart_4: Color::seal(Hsla {
            h: 0.121_209_286,
            s: 1.0,
            l: 0.5,
            a: 1.0,
        }),
        chart_5: Color::seal(Hsla {
            h: 0.100_926_95,
            s: 1.0,
            l: 0.497_135_73,
            a: 1.0,
        }),
        sidebar: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.0,
            a: 0.0,
        }),
        sidebar_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.149_382_08,
            a: 1.0,
        }),
        sidebar_primary: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.149_382_08,
            a: 1.0,
        }),
        sidebar_primary_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.980_255_96,
            a: 1.0,
        }),
        sidebar_accent: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.0,
            a: 0.04,
        }),
        sidebar_accent_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.149_382_08,
            a: 1.0,
        }),
        sidebar_border: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.0,
            a: 0.06,
        }),
        sidebar_ring: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.630_163_2,
            a: 1.0,
        }),
        sidebar_element_idle: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.149_382_08,
            a: 0.06,
        }),
        sidebar_element_hover: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.0,
            a: 0.08,
        }),
        sidebar_element_press: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.149_382_08,
            a: 0.08,
        }),
        code: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 1.0,
            a: 1.0,
        }),
        code_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.149_382_08,
            a: 1.0,
        }),
        code_highlight: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.0,
            a: 0.04,
        }),
        git_modified: Color::seal(Hsla {
            h: 0.100_926_95,
            s: 1.0,
            l: 0.497_135_73,
            a: 1.0,
        }),
        git_modified_staged: Color::seal(Hsla {
            h: 0.443_712_32,
            s: 1.0,
            l: 0.369_394_57,
            a: 1.0,
        }),
        git_added: Color::seal(Hsla {
            h: 0.443_712_32,
            s: 1.0,
            l: 0.369_394_57,
            a: 1.0,
        }),
        git_untracked: Color::seal(Hsla {
            h: 0.238_423_08,
            s: 1.0,
            l: 0.323_744_54,
            a: 1.0,
        }),
        git_deleted: Color::seal(Hsla {
            h: 0.991_516_65,
            s: 0.958_988_25,
            l: 0.577_229_26,
            a: 1.0,
        }),
        git_renamed: Color::seal(Hsla {
            h: 0.561_322_57,
            s: 1.0,
            l: 0.409_899_83,
            a: 1.0,
        }),
        chrome_bg: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.980_255_96,
            a: 0.45,
        }),
        app_ui_scale: Scale::seal(1.0),
        ui_text_xs: FontSize::seal(Rems(0.6875)),
        ui_text_sm: FontSize::seal(Rems(0.75)),
        ui_text_base: FontSize::seal(Rems(0.875)),
        ui_text_lg: FontSize::seal(Rems(1.0)),
        ui_text_xl: FontSize::seal(Rems(1.25)),
        app_scrollbar_size: Space::seal(px(11.0)),
        app_scrollbar_thumb: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.444_695_95,
            a: 0.42,
        }),
        app_scrollbar_thumb_border: Space::seal(px(3.0)),
        app_scrollbar_thumb_hover: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.444_695_95,
            a: 0.58,
        }),
        app_scrollbar_track: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.0,
            a: 0.0,
        }),
        app_scrollbar_radius: Radius::seal(px(999.0)),
        syntax_comment: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.280_604_24,
            a: 1.0,
        }),
        syntax_variable: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.149_382_08,
            a: 1.0,
        }),
        syntax_punctuation: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.280_604_24,
            a: 1.0,
        }),
        syntax_operator: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.280_604_24,
            a: 1.0,
        }),
        syntax_error: Color::seal(Hsla {
            h: 0.991_516_65,
            s: 0.958_988_25,
            l: 0.577_229_26,
            a: 1.0,
        }),
        syntax_keyword: Color::seal(Hsla {
            h: 0.041_284_405,
            s: 0.441_295_53,
            l: 0.484_313_73,
            a: 1.0,
        }),
        syntax_string: Color::seal(Hsla {
            h: 0.276_041_66,
            s: 0.329_896_9,
            l: 0.380_392_16,
            a: 1.0,
        }),
        syntax_number: Color::seal(Hsla {
            h: 0.101_941_75,
            s: 0.517_587_96,
            l: 0.390_196_08,
            a: 1.0,
        }),
        syntax_constant: Color::seal(Hsla {
            h: 0.516_064_3,
            s: 0.434_554_96,
            l: 0.374_509_8,
            a: 1.0,
        }),
        syntax_function: Color::seal(Hsla {
            h: 0.582_677_2,
            s: 0.574_660_66,
            l: 0.433_333_34,
            a: 1.0,
        }),
        syntax_type: Color::seal(Hsla {
            h: 0.755_144_06,
            s: 0.317_647_07,
            l: 0.5,
            a: 1.0,
        }),
        syntax_property: Color::seal(Hsla {
            h: 0.115_384_616,
            s: 0.077_844_314,
            l: 0.327_451,
            a: 1.0,
        }),
        syntax_tag: Color::seal(Hsla {
            h: 0.582_677_2,
            s: 0.574_660_66,
            l: 0.433_333_34,
            a: 1.0,
        }),
        syntax_attribute: Color::seal(Hsla {
            h: 0.101_941_75,
            s: 0.517_587_96,
            l: 0.390_196_08,
            a: 1.0,
        }),
        syntax_boolean: Color::seal(Hsla {
            h: 0.516_064_3,
            s: 0.434_554_96,
            l: 0.374_509_8,
            a: 1.0,
        }),
        syntax_null: Color::seal(Hsla {
            h: 0.755_144_06,
            s: 0.317_647_07,
            l: 0.5,
            a: 1.0,
        }),
        syntax_regex: Color::seal(Hsla {
            h: 0.516_064_3,
            s: 0.434_554_96,
            l: 0.374_509_8,
            a: 1.0,
        }),
        syntax_jsx: Color::seal(Hsla {
            h: 0.582_677_2,
            s: 0.574_660_66,
            l: 0.433_333_34,
            a: 1.0,
        }),
        syntax_jsx_attribute: Color::seal(Hsla {
            h: 0.101_941_75,
            s: 0.517_587_96,
            l: 0.390_196_08,
            a: 1.0,
        }),
        syntax_markdown_heading: Color::seal(Hsla {
            h: 0.582_677_2,
            s: 0.574_660_66,
            l: 0.433_333_34,
            a: 1.0,
        }),
        syntax_markdown_bold: Color::seal(Hsla {
            h: 0.101_941_75,
            s: 0.517_587_96,
            l: 0.390_196_08,
            a: 1.0,
        }),
        syntax_markdown_italic: Color::seal(Hsla {
            h: 0.041_284_405,
            s: 0.441_295_53,
            l: 0.484_313_73,
            a: 1.0,
        }),
        syntax_markdown_strikethrough: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.280_604_24,
            a: 1.0,
        }),
        syntax_markdown_link: Color::seal(Hsla {
            h: 0.582_677_2,
            s: 0.574_660_66,
            l: 0.433_333_34,
            a: 1.0,
        }),
        syntax_markdown_link_text: Color::seal(Hsla {
            h: 0.276_041_66,
            s: 0.329_896_9,
            l: 0.380_392_16,
            a: 1.0,
        }),
        syntax_markdown_code: Color::seal(Hsla {
            h: 0.276_041_66,
            s: 0.329_896_9,
            l: 0.380_392_16,
            a: 1.0,
        }),
        syntax_markdown_list: Color::seal(Hsla {
            h: 0.041_284_405,
            s: 0.441_295_53,
            l: 0.484_313_73,
            a: 1.0,
        }),
        syntax_markdown_quote: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.280_604_24,
            a: 1.0,
        }),
        terminal_black: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.150_332_24,
            a: 1.0,
        }),
        terminal_red: Color::seal(Hsla {
            h: 0.991_516_65,
            s: 0.958_988_25,
            l: 0.577_229_26,
            a: 1.0,
        }),
        terminal_green: Color::seal(Hsla {
            h: 0.443_712_32,
            s: 1.0,
            l: 0.369_394_57,
            a: 1.0,
        }),
        terminal_yellow: Color::seal(Hsla {
            h: 0.100_926_95,
            s: 1.0,
            l: 0.497_135_73,
            a: 1.0,
        }),
        terminal_blue: Color::seal(Hsla {
            h: 0.600_712_3,
            s: 1.0,
            l: 0.584_666_25,
            a: 1.0,
        }),
        terminal_magenta: Color::seal(Hsla {
            h: 0.755_144_06,
            s: 0.317_647_07,
            l: 0.5,
            a: 1.0,
        }),
        terminal_cyan: Color::seal(Hsla {
            h: 0.516_064_3,
            s: 0.434_554_96,
            l: 0.374_509_8,
            a: 1.0,
        }),
        terminal_white: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.280_604_24,
            a: 1.0,
        }),
        terminal_bright_black: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.280_604_24,
            a: 1.0,
        }),
        terminal_bright_red: Color::seal(Hsla {
            h: 0.991_516_65,
            s: 0.958_988_25,
            l: 0.577_229_26,
            a: 1.0,
        }),
        terminal_bright_green: Color::seal(Hsla {
            h: 0.443_712_32,
            s: 1.0,
            l: 0.369_394_57,
            a: 1.0,
        }),
        terminal_bright_yellow: Color::seal(Hsla {
            h: 0.100_926_95,
            s: 1.0,
            l: 0.497_135_73,
            a: 1.0,
        }),
        terminal_bright_blue: Color::seal(Hsla {
            h: 0.600_712_3,
            s: 1.0,
            l: 0.584_666_25,
            a: 1.0,
        }),
        terminal_bright_magenta: Color::seal(Hsla {
            h: 0.755_144_06,
            s: 0.317_647_07,
            l: 0.5,
            a: 1.0,
        }),
        terminal_bright_cyan: Color::seal(Hsla {
            h: 0.516_064_3,
            s: 0.434_554_96,
            l: 0.374_509_8,
            a: 1.0,
        }),
        terminal_bright_white: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.149_382_08,
            a: 1.0,
        }),
        file_tree_hover_bg: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.0,
            a: 0.0272,
        }),
        file_tree_guide_icon_offset: Space::seal(px(7.0)),
        tree_guide_color: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.280_604_24,
            a: 0.18,
        }),
    };

    /// The dark appearance — `:root` with `.dark` layered over it.
    pub const DARK: Self = Self {
        font_sans: FontFamily::seal(&["CalSansUI", "sans-serif"]),
        font_heading: FontFamily::seal(&["CalSans", "sans-serif"]),
        font_mono: FontFamily::seal(&["JetBrains Mono Variable", "ui-monospace", "monospace"]),
        color_background: Color::seal(Hsla {
            h: 0.166_487_02,
            s: 0.017_503_321,
            l: 0.119_594_134,
            a: 1.0,
        }),
        color_pane_background: Color::seal(Hsla {
            h: 0.166_487_02,
            s: 0.017_503_321,
            l: 0.119_594_134,
            a: 1.0,
        }),
        color_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.960_586_97,
            a: 1.0,
        }),
        color_card: Color::seal(Hsla {
            h: 0.166_487_02,
            s: 0.017_503_321,
            l: 0.119_594_134,
            a: 1.0,
        }),
        color_card_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.960_586_97,
            a: 1.0,
        }),
        color_popover: Color::seal(Hsla {
            h: 0.166_487_02,
            s: 0.017_503_321,
            l: 0.119_594_134,
            a: 1.0,
        }),
        color_popover_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.960_586_97,
            a: 1.0,
        }),
        color_primary: Color::seal(Hsla {
            h: 0.246_745_41,
            s: 0.324_235_02,
            l: 0.312_937_68,
            a: 1.0,
        }),
        color_primary_foreground: Color::seal(Hsla {
            h: 0.139_881_2,
            s: 0.858_157_63,
            l: 0.944_533_1,
            a: 1.0,
        }),
        color_secondary: Color::seal(Hsla {
            h: 0.246_745_41,
            s: 0.324_235_02,
            l: 0.312_937_68,
            a: 1.0,
        }),
        color_secondary_foreground: Color::seal(Hsla {
            h: 0.139_881_2,
            s: 0.858_157_63,
            l: 0.944_533_1,
            a: 1.0,
        }),
        color_muted: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 1.0,
            a: 0.04,
        }),
        color_muted_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.64471,
            a: 1.0,
        }),
        color_accent: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 1.0,
            a: 0.04,
        }),
        color_accent_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.960_586_97,
            a: 1.0,
        }),
        color_destructive: Color::seal(Hsla {
            h: 0.993_566_5,
            s: 0.935_879_95,
            l: 0.613_170_56,
            a: 1.0,
        }),
        color_destructive_foreground: Color::seal(Hsla {
            h: 0.996_522_37,
            s: 1.0,
            l: 0.695_576_37,
            a: 1.0,
        }),
        color_border: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 1.0,
            a: 0.06,
        }),
        color_input: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 1.0,
            a: 0.08,
        }),
        color_ring: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.451_519_25,
            a: 1.0,
        }),
        color_chart_1: Color::seal(Hsla {
            h: 0.625_958_74,
            s: 0.840_952_93,
            l: 0.489_841_1,
            a: 1.0,
        }),
        color_chart_2: Color::seal(Hsla {
            h: 0.443_712_32,
            s: 1.0,
            l: 0.369_394_57,
            a: 1.0,
        }),
        color_chart_3: Color::seal(Hsla {
            h: 0.100_926_95,
            s: 1.0,
            l: 0.497_135_73,
            a: 1.0,
        }),
        color_chart_4: Color::seal(Hsla {
            h: 0.759_193_24,
            s: 1.0,
            l: 0.637_971,
            a: 1.0,
        }),
        color_chart_5: Color::seal(Hsla {
            h: 0.959_182_14,
            s: 1.0,
            l: 0.562_307_6,
            a: 1.0,
        }),
        color_sidebar: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.0,
            a: 0.18,
        }),
        color_sidebar_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.960_586_97,
            a: 1.0,
        }),
        color_sidebar_primary: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.960_586_97,
            a: 1.0,
        }),
        color_sidebar_primary_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.149_382_08,
            a: 1.0,
        }),
        color_sidebar_accent: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 1.0,
            a: 0.04,
        }),
        color_sidebar_accent_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.960_586_97,
            a: 1.0,
        }),
        color_sidebar_border: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 1.0,
            a: 0.05,
        }),
        color_sidebar_ring: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.630_163_2,
            a: 1.0,
        }),
        color_sidebar_element_idle: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.960_586_97,
            a: 0.11,
        }),
        color_sidebar_element_hover: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 1.0,
            a: 0.1,
        }),
        color_sidebar_element_press: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.960_586_97,
            a: 0.07,
        }),
        color_success: Color::seal(Hsla {
            h: 0.443_712_32,
            s: 1.0,
            l: 0.369_394_57,
            a: 1.0,
        }),
        color_success_foreground: Color::seal(Hsla {
            h: 0.447_903_5,
            s: 1.0,
            l: 0.416_296_06,
            a: 1.0,
        }),
        color_warning: Color::seal(Hsla {
            h: 0.100_926_95,
            s: 1.0,
            l: 0.497_135_73,
            a: 1.0,
        }),
        color_warning_foreground: Color::seal(Hsla {
            h: 0.121_209_286,
            s: 1.0,
            l: 0.5,
            a: 1.0,
        }),
        color_info: Color::seal(Hsla {
            h: 0.600_712_3,
            s: 1.0,
            l: 0.584_666_25,
            a: 1.0,
        }),
        color_info_foreground: Color::seal(Hsla {
            h: 0.588_767_7,
            s: 1.0,
            l: 0.657_899_7,
            a: 1.0,
        }),
        color_code: Color::seal(Hsla {
            h: 0.166_487_02,
            s: 0.017_503_321,
            l: 0.119_594_134,
            a: 1.0,
        }),
        color_code_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.960_586_97,
            a: 1.0,
        }),
        color_code_highlight: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 1.0,
            a: 0.04,
        }),
        color_chrome_bg: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.149_382_08,
            a: 0.65,
        }),
        color_git_added: Color::seal(Hsla {
            h: 0.443_712_32,
            s: 1.0,
            l: 0.369_394_57,
            a: 1.0,
        }),
        color_git_deleted: Color::seal(Hsla {
            h: 0.993_566_5,
            s: 0.935_879_95,
            l: 0.613_170_56,
            a: 1.0,
        }),
        color_git_modified: Color::seal(Hsla {
            h: 0.100_926_95,
            s: 1.0,
            l: 0.497_135_73,
            a: 1.0,
        }),
        color_git_modified_staged: Color::seal(Hsla {
            h: 0.443_712_32,
            s: 1.0,
            l: 0.369_394_57,
            a: 1.0,
        }),
        color_git_untracked: Color::seal(Hsla {
            h: 0.221_976_52,
            s: 1.0,
            l: 0.450_835_62,
            a: 1.0,
        }),
        color_git_renamed: Color::seal(Hsla {
            h: 0.543_931_37,
            s: 1.0,
            l: 0.5,
            a: 1.0,
        }),
        font_editor: FontFamily::seal(&[]),
        radius_sm: Radius::seal(px(6.0)),
        radius_md: Radius::seal(px(8.0)),
        radius_lg: Radius::seal(px(10.0)),
        radius_xl: Radius::seal(px(14.0)),
        radius_2xl: Radius::seal(px(18.0)),
        radius_3xl: Radius::seal(px(22.0)),
        radius_4xl: Radius::seal(px(26.0)),
        animate_skeleton: Duration::seal(StdDuration::from_secs(2)),
        animate_caret_blink: Duration::seal(StdDuration::from_secs(1)),
        animate_toast_success_odd: Duration::seal(StdDuration::from_millis(320)),
        animate_toast_success_even: Duration::seal(StdDuration::from_millis(320)),
        animate_toast_error_odd: Duration::seal(StdDuration::from_millis(280)),
        animate_toast_error_even: Duration::seal(StdDuration::from_millis(280)),
        animate_label_in: Duration::seal(StdDuration::from_millis(140)),
        radius: Radius::seal(px(10.0)),
        background: Color::seal(Hsla {
            h: 0.166_487_02,
            s: 0.017_503_321,
            l: 0.119_594_134,
            a: 1.0,
        }),
        pane_background: Color::seal(Hsla {
            h: 0.166_487_02,
            s: 0.017_503_321,
            l: 0.119_594_134,
            a: 1.0,
        }),
        foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.960_586_97,
            a: 1.0,
        }),
        card: Color::seal(Hsla {
            h: 0.166_487_02,
            s: 0.017_503_321,
            l: 0.119_594_134,
            a: 1.0,
        }),
        card_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.960_586_97,
            a: 1.0,
        }),
        popover: Color::seal(Hsla {
            h: 0.166_487_02,
            s: 0.017_503_321,
            l: 0.119_594_134,
            a: 1.0,
        }),
        popover_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.960_586_97,
            a: 1.0,
        }),
        primary: Color::seal(Hsla {
            h: 0.246_745_41,
            s: 0.324_235_02,
            l: 0.312_937_68,
            a: 1.0,
        }),
        primary_foreground: Color::seal(Hsla {
            h: 0.139_881_2,
            s: 0.858_157_63,
            l: 0.944_533_1,
            a: 1.0,
        }),
        secondary: Color::seal(Hsla {
            h: 0.246_745_41,
            s: 0.324_235_02,
            l: 0.312_937_68,
            a: 1.0,
        }),
        secondary_foreground: Color::seal(Hsla {
            h: 0.139_881_2,
            s: 0.858_157_63,
            l: 0.944_533_1,
            a: 1.0,
        }),
        muted: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 1.0,
            a: 0.04,
        }),
        muted_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.64471,
            a: 1.0,
        }),
        accent: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 1.0,
            a: 0.04,
        }),
        accent_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.960_586_97,
            a: 1.0,
        }),
        editor_selection: Color::seal(Hsla {
            h: 0.555_137,
            s: 0.755_794_5,
            l: 0.553_139_6,
            a: 0.32,
        }),
        destructive: Color::seal(Hsla {
            h: 0.993_566_5,
            s: 0.935_879_95,
            l: 0.613_170_56,
            a: 1.0,
        }),
        destructive_foreground: Color::seal(Hsla {
            h: 0.996_522_37,
            s: 1.0,
            l: 0.695_576_37,
            a: 1.0,
        }),
        info: Color::seal(Hsla {
            h: 0.600_712_3,
            s: 1.0,
            l: 0.584_666_25,
            a: 1.0,
        }),
        info_foreground: Color::seal(Hsla {
            h: 0.588_767_7,
            s: 1.0,
            l: 0.657_899_7,
            a: 1.0,
        }),
        success: Color::seal(Hsla {
            h: 0.443_712_32,
            s: 1.0,
            l: 0.369_394_57,
            a: 1.0,
        }),
        success_foreground: Color::seal(Hsla {
            h: 0.447_903_5,
            s: 1.0,
            l: 0.416_296_06,
            a: 1.0,
        }),
        warning: Color::seal(Hsla {
            h: 0.100_926_95,
            s: 1.0,
            l: 0.497_135_73,
            a: 1.0,
        }),
        warning_foreground: Color::seal(Hsla {
            h: 0.121_209_286,
            s: 1.0,
            l: 0.5,
            a: 1.0,
        }),
        border: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 1.0,
            a: 0.06,
        }),
        input: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 1.0,
            a: 0.08,
        }),
        ring: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.451_519_25,
            a: 1.0,
        }),
        chart_1: Color::seal(Hsla {
            h: 0.625_958_74,
            s: 0.840_952_93,
            l: 0.489_841_1,
            a: 1.0,
        }),
        chart_2: Color::seal(Hsla {
            h: 0.443_712_32,
            s: 1.0,
            l: 0.369_394_57,
            a: 1.0,
        }),
        chart_3: Color::seal(Hsla {
            h: 0.100_926_95,
            s: 1.0,
            l: 0.497_135_73,
            a: 1.0,
        }),
        chart_4: Color::seal(Hsla {
            h: 0.759_193_24,
            s: 1.0,
            l: 0.637_971,
            a: 1.0,
        }),
        chart_5: Color::seal(Hsla {
            h: 0.959_182_14,
            s: 1.0,
            l: 0.562_307_6,
            a: 1.0,
        }),
        sidebar: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.0,
            a: 0.18,
        }),
        sidebar_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.960_586_97,
            a: 1.0,
        }),
        sidebar_primary: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.960_586_97,
            a: 1.0,
        }),
        sidebar_primary_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.149_382_08,
            a: 1.0,
        }),
        sidebar_accent: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 1.0,
            a: 0.04,
        }),
        sidebar_accent_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.960_586_97,
            a: 1.0,
        }),
        sidebar_border: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 1.0,
            a: 0.05,
        }),
        sidebar_ring: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.630_163_2,
            a: 1.0,
        }),
        sidebar_element_idle: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.960_586_97,
            a: 0.11,
        }),
        sidebar_element_hover: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 1.0,
            a: 0.1,
        }),
        sidebar_element_press: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.960_586_97,
            a: 0.07,
        }),
        code: Color::seal(Hsla {
            h: 0.166_487_02,
            s: 0.017_503_321,
            l: 0.119_594_134,
            a: 1.0,
        }),
        code_foreground: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.960_586_97,
            a: 1.0,
        }),
        code_highlight: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 1.0,
            a: 0.04,
        }),
        git_modified: Color::seal(Hsla {
            h: 0.100_926_95,
            s: 1.0,
            l: 0.497_135_73,
            a: 1.0,
        }),
        git_modified_staged: Color::seal(Hsla {
            h: 0.443_712_32,
            s: 1.0,
            l: 0.369_394_57,
            a: 1.0,
        }),
        git_added: Color::seal(Hsla {
            h: 0.443_712_32,
            s: 1.0,
            l: 0.369_394_57,
            a: 1.0,
        }),
        git_untracked: Color::seal(Hsla {
            h: 0.221_976_52,
            s: 1.0,
            l: 0.450_835_62,
            a: 1.0,
        }),
        git_deleted: Color::seal(Hsla {
            h: 0.993_566_5,
            s: 0.935_879_95,
            l: 0.613_170_56,
            a: 1.0,
        }),
        git_renamed: Color::seal(Hsla {
            h: 0.543_931_37,
            s: 1.0,
            l: 0.5,
            a: 1.0,
        }),
        chrome_bg: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.149_382_08,
            a: 0.65,
        }),
        app_ui_scale: Scale::seal(1.0),
        ui_text_xs: FontSize::seal(Rems(0.6875)),
        ui_text_sm: FontSize::seal(Rems(0.75)),
        ui_text_base: FontSize::seal(Rems(0.875)),
        ui_text_lg: FontSize::seal(Rems(1.0)),
        ui_text_xl: FontSize::seal(Rems(1.25)),
        app_scrollbar_size: Space::seal(px(11.0)),
        app_scrollbar_thumb: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.444_695_95,
            a: 0.42,
        }),
        app_scrollbar_thumb_border: Space::seal(px(3.0)),
        app_scrollbar_thumb_hover: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.444_695_95,
            a: 0.58,
        }),
        app_scrollbar_track: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.0,
            a: 0.0,
        }),
        app_scrollbar_radius: Radius::seal(px(999.0)),
        syntax_comment: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.64471,
            a: 1.0,
        }),
        syntax_variable: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.960_586_97,
            a: 1.0,
        }),
        syntax_punctuation: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.64471,
            a: 1.0,
        }),
        syntax_operator: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.64471,
            a: 1.0,
        }),
        syntax_error: Color::seal(Hsla {
            h: 0.993_566_5,
            s: 0.935_879_95,
            l: 0.613_170_56,
            a: 1.0,
        }),
        syntax_keyword: Color::seal(Hsla {
            h: 0.041_025_642,
            s: 0.631_067_93,
            l: 0.596_078_46,
            a: 1.0,
        }),
        syntax_string: Color::seal(Hsla {
            h: 0.255_208_34,
            s: 0.355_555_56,
            l: 0.647_058_84,
            a: 1.0,
        }),
        syntax_number: Color::seal(Hsla {
            h: 0.105_191_25,
            s: 0.598_039_2,
            l: 0.6,
            a: 1.0,
        }),
        syntax_constant: Color::seal(Hsla {
            h: 0.513_201_3,
            s: 0.461_187_2,
            l: 0.570_588_23,
            a: 1.0,
        }),
        syntax_function: Color::seal(Hsla {
            h: 0.570_796_5,
            s: 0.645_714_3,
            l: 0.656_862_74,
            a: 1.0,
        }),
        syntax_type: Color::seal(Hsla {
            h: 0.757_575_75,
            s: 0.447_154_46,
            l: 0.758_823_5,
            a: 1.0,
        }),
        syntax_property: Color::seal(Hsla {
            h: 0.111_111_11,
            s: 0.157_894_73,
            l: 0.776_470_6,
            a: 1.0,
        }),
        syntax_tag: Color::seal(Hsla {
            h: 0.570_796_5,
            s: 0.645_714_3,
            l: 0.656_862_74,
            a: 1.0,
        }),
        syntax_attribute: Color::seal(Hsla {
            h: 0.105_191_25,
            s: 0.598_039_2,
            l: 0.6,
            a: 1.0,
        }),
        syntax_boolean: Color::seal(Hsla {
            h: 0.513_201_3,
            s: 0.461_187_2,
            l: 0.570_588_23,
            a: 1.0,
        }),
        syntax_null: Color::seal(Hsla {
            h: 0.757_575_75,
            s: 0.447_154_46,
            l: 0.758_823_5,
            a: 1.0,
        }),
        syntax_regex: Color::seal(Hsla {
            h: 0.530_864_2,
            s: 0.457_627_12,
            l: 0.652_941_17,
            a: 1.0,
        }),
        syntax_jsx: Color::seal(Hsla {
            h: 0.570_796_5,
            s: 0.645_714_3,
            l: 0.656_862_74,
            a: 1.0,
        }),
        syntax_jsx_attribute: Color::seal(Hsla {
            h: 0.105_191_25,
            s: 0.598_039_2,
            l: 0.6,
            a: 1.0,
        }),
        syntax_markdown_heading: Color::seal(Hsla {
            h: 0.570_796_5,
            s: 0.645_714_3,
            l: 0.656_862_74,
            a: 1.0,
        }),
        syntax_markdown_bold: Color::seal(Hsla {
            h: 0.105_191_25,
            s: 0.598_039_2,
            l: 0.6,
            a: 1.0,
        }),
        syntax_markdown_italic: Color::seal(Hsla {
            h: 0.041_025_642,
            s: 0.631_067_93,
            l: 0.596_078_46,
            a: 1.0,
        }),
        syntax_markdown_strikethrough: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.64471,
            a: 1.0,
        }),
        syntax_markdown_link: Color::seal(Hsla {
            h: 0.570_796_5,
            s: 0.645_714_3,
            l: 0.656_862_74,
            a: 1.0,
        }),
        syntax_markdown_link_text: Color::seal(Hsla {
            h: 0.255_208_34,
            s: 0.355_555_56,
            l: 0.647_058_84,
            a: 1.0,
        }),
        syntax_markdown_code: Color::seal(Hsla {
            h: 0.255_208_34,
            s: 0.355_555_56,
            l: 0.647_058_84,
            a: 1.0,
        }),
        syntax_markdown_list: Color::seal(Hsla {
            h: 0.041_025_642,
            s: 0.631_067_93,
            l: 0.596_078_46,
            a: 1.0,
        }),
        syntax_markdown_quote: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.64471,
            a: 1.0,
        }),
        terminal_black: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.150_332_24,
            a: 1.0,
        }),
        terminal_red: Color::seal(Hsla {
            h: 0.993_566_5,
            s: 0.935_879_95,
            l: 0.613_170_56,
            a: 1.0,
        }),
        terminal_green: Color::seal(Hsla {
            h: 0.443_712_32,
            s: 1.0,
            l: 0.369_394_57,
            a: 1.0,
        }),
        terminal_yellow: Color::seal(Hsla {
            h: 0.100_926_95,
            s: 1.0,
            l: 0.497_135_73,
            a: 1.0,
        }),
        terminal_blue: Color::seal(Hsla {
            h: 0.600_712_3,
            s: 1.0,
            l: 0.584_666_25,
            a: 1.0,
        }),
        terminal_magenta: Color::seal(Hsla {
            h: 0.757_575_75,
            s: 0.447_154_46,
            l: 0.758_823_5,
            a: 1.0,
        }),
        terminal_cyan: Color::seal(Hsla {
            h: 0.513_201_3,
            s: 0.461_187_2,
            l: 0.570_588_23,
            a: 1.0,
        }),
        terminal_white: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.64471,
            a: 1.0,
        }),
        terminal_bright_black: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.64471,
            a: 1.0,
        }),
        terminal_bright_red: Color::seal(Hsla {
            h: 0.993_566_5,
            s: 0.935_879_95,
            l: 0.613_170_56,
            a: 1.0,
        }),
        terminal_bright_green: Color::seal(Hsla {
            h: 0.443_712_32,
            s: 1.0,
            l: 0.369_394_57,
            a: 1.0,
        }),
        terminal_bright_yellow: Color::seal(Hsla {
            h: 0.100_926_95,
            s: 1.0,
            l: 0.497_135_73,
            a: 1.0,
        }),
        terminal_bright_blue: Color::seal(Hsla {
            h: 0.600_712_3,
            s: 1.0,
            l: 0.584_666_25,
            a: 1.0,
        }),
        terminal_bright_magenta: Color::seal(Hsla {
            h: 0.757_575_75,
            s: 0.447_154_46,
            l: 0.758_823_5,
            a: 1.0,
        }),
        terminal_bright_cyan: Color::seal(Hsla {
            h: 0.513_201_3,
            s: 0.461_187_2,
            l: 0.570_588_23,
            a: 1.0,
        }),
        terminal_bright_white: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.960_586_97,
            a: 1.0,
        }),
        file_tree_hover_bg: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 1.0,
            a: 0.0272,
        }),
        file_tree_guide_icon_offset: Space::seal(px(7.0)),
        tree_guide_color: Color::seal(Hsla {
            h: 0.0,
            s: 0.0,
            l: 0.64471,
            a: 0.18,
        }),
    };
}

/// Every token, paired with a predicate that asks whether the two
/// tables disagree about it.
///
/// Generated alongside the tables from the same field list, so the
/// structural tests cannot drift out of step with what was emitted.
#[cfg(test)]
pub(super) type VarianceRow = (&'static str, fn(&Theme, &Theme) -> bool);

#[cfg(test)]
pub(super) const TOKEN_VARIANCE: [VarianceRow; 183] = [
    ("--font-sans", |light, dark| {
        light.font_sans != dark.font_sans
    }),
    ("--font-heading", |light, dark| {
        light.font_heading != dark.font_heading
    }),
    ("--font-mono", |light, dark| {
        light.font_mono != dark.font_mono
    }),
    ("--color-background", |light, dark| {
        light.color_background != dark.color_background
    }),
    ("--color-pane-background", |light, dark| {
        light.color_pane_background != dark.color_pane_background
    }),
    ("--color-foreground", |light, dark| {
        light.color_foreground != dark.color_foreground
    }),
    ("--color-card", |light, dark| {
        light.color_card != dark.color_card
    }),
    ("--color-card-foreground", |light, dark| {
        light.color_card_foreground != dark.color_card_foreground
    }),
    ("--color-popover", |light, dark| {
        light.color_popover != dark.color_popover
    }),
    ("--color-popover-foreground", |light, dark| {
        light.color_popover_foreground != dark.color_popover_foreground
    }),
    ("--color-primary", |light, dark| {
        light.color_primary != dark.color_primary
    }),
    ("--color-primary-foreground", |light, dark| {
        light.color_primary_foreground != dark.color_primary_foreground
    }),
    ("--color-secondary", |light, dark| {
        light.color_secondary != dark.color_secondary
    }),
    ("--color-secondary-foreground", |light, dark| {
        light.color_secondary_foreground != dark.color_secondary_foreground
    }),
    ("--color-muted", |light, dark| {
        light.color_muted != dark.color_muted
    }),
    ("--color-muted-foreground", |light, dark| {
        light.color_muted_foreground != dark.color_muted_foreground
    }),
    ("--color-accent", |light, dark| {
        light.color_accent != dark.color_accent
    }),
    ("--color-accent-foreground", |light, dark| {
        light.color_accent_foreground != dark.color_accent_foreground
    }),
    ("--color-destructive", |light, dark| {
        light.color_destructive != dark.color_destructive
    }),
    ("--color-destructive-foreground", |light, dark| {
        light.color_destructive_foreground != dark.color_destructive_foreground
    }),
    ("--color-border", |light, dark| {
        light.color_border != dark.color_border
    }),
    ("--color-input", |light, dark| {
        light.color_input != dark.color_input
    }),
    ("--color-ring", |light, dark| {
        light.color_ring != dark.color_ring
    }),
    ("--color-chart-1", |light, dark| {
        light.color_chart_1 != dark.color_chart_1
    }),
    ("--color-chart-2", |light, dark| {
        light.color_chart_2 != dark.color_chart_2
    }),
    ("--color-chart-3", |light, dark| {
        light.color_chart_3 != dark.color_chart_3
    }),
    ("--color-chart-4", |light, dark| {
        light.color_chart_4 != dark.color_chart_4
    }),
    ("--color-chart-5", |light, dark| {
        light.color_chart_5 != dark.color_chart_5
    }),
    ("--color-sidebar", |light, dark| {
        light.color_sidebar != dark.color_sidebar
    }),
    ("--color-sidebar-foreground", |light, dark| {
        light.color_sidebar_foreground != dark.color_sidebar_foreground
    }),
    ("--color-sidebar-primary", |light, dark| {
        light.color_sidebar_primary != dark.color_sidebar_primary
    }),
    ("--color-sidebar-primary-foreground", |light, dark| {
        light.color_sidebar_primary_foreground != dark.color_sidebar_primary_foreground
    }),
    ("--color-sidebar-accent", |light, dark| {
        light.color_sidebar_accent != dark.color_sidebar_accent
    }),
    ("--color-sidebar-accent-foreground", |light, dark| {
        light.color_sidebar_accent_foreground != dark.color_sidebar_accent_foreground
    }),
    ("--color-sidebar-border", |light, dark| {
        light.color_sidebar_border != dark.color_sidebar_border
    }),
    ("--color-sidebar-ring", |light, dark| {
        light.color_sidebar_ring != dark.color_sidebar_ring
    }),
    ("--color-sidebar-element-idle", |light, dark| {
        light.color_sidebar_element_idle != dark.color_sidebar_element_idle
    }),
    ("--color-sidebar-element-hover", |light, dark| {
        light.color_sidebar_element_hover != dark.color_sidebar_element_hover
    }),
    ("--color-sidebar-element-press", |light, dark| {
        light.color_sidebar_element_press != dark.color_sidebar_element_press
    }),
    ("--color-success", |light, dark| {
        light.color_success != dark.color_success
    }),
    ("--color-success-foreground", |light, dark| {
        light.color_success_foreground != dark.color_success_foreground
    }),
    ("--color-warning", |light, dark| {
        light.color_warning != dark.color_warning
    }),
    ("--color-warning-foreground", |light, dark| {
        light.color_warning_foreground != dark.color_warning_foreground
    }),
    ("--color-info", |light, dark| {
        light.color_info != dark.color_info
    }),
    ("--color-info-foreground", |light, dark| {
        light.color_info_foreground != dark.color_info_foreground
    }),
    ("--color-code", |light, dark| {
        light.color_code != dark.color_code
    }),
    ("--color-code-foreground", |light, dark| {
        light.color_code_foreground != dark.color_code_foreground
    }),
    ("--color-code-highlight", |light, dark| {
        light.color_code_highlight != dark.color_code_highlight
    }),
    ("--color-chrome-bg", |light, dark| {
        light.color_chrome_bg != dark.color_chrome_bg
    }),
    ("--color-git-added", |light, dark| {
        light.color_git_added != dark.color_git_added
    }),
    ("--color-git-deleted", |light, dark| {
        light.color_git_deleted != dark.color_git_deleted
    }),
    ("--color-git-modified", |light, dark| {
        light.color_git_modified != dark.color_git_modified
    }),
    ("--color-git-modified-staged", |light, dark| {
        light.color_git_modified_staged != dark.color_git_modified_staged
    }),
    ("--color-git-untracked", |light, dark| {
        light.color_git_untracked != dark.color_git_untracked
    }),
    ("--color-git-renamed", |light, dark| {
        light.color_git_renamed != dark.color_git_renamed
    }),
    ("--font-editor", |light, dark| {
        light.font_editor != dark.font_editor
    }),
    ("--radius-sm", |light, dark| {
        light.radius_sm != dark.radius_sm
    }),
    ("--radius-md", |light, dark| {
        light.radius_md != dark.radius_md
    }),
    ("--radius-lg", |light, dark| {
        light.radius_lg != dark.radius_lg
    }),
    ("--radius-xl", |light, dark| {
        light.radius_xl != dark.radius_xl
    }),
    ("--radius-2xl", |light, dark| {
        light.radius_2xl != dark.radius_2xl
    }),
    ("--radius-3xl", |light, dark| {
        light.radius_3xl != dark.radius_3xl
    }),
    ("--radius-4xl", |light, dark| {
        light.radius_4xl != dark.radius_4xl
    }),
    ("--animate-skeleton", |light, dark| {
        light.animate_skeleton != dark.animate_skeleton
    }),
    ("--animate-caret-blink", |light, dark| {
        light.animate_caret_blink != dark.animate_caret_blink
    }),
    ("--animate-toast-success-odd", |light, dark| {
        light.animate_toast_success_odd != dark.animate_toast_success_odd
    }),
    ("--animate-toast-success-even", |light, dark| {
        light.animate_toast_success_even != dark.animate_toast_success_even
    }),
    ("--animate-toast-error-odd", |light, dark| {
        light.animate_toast_error_odd != dark.animate_toast_error_odd
    }),
    ("--animate-toast-error-even", |light, dark| {
        light.animate_toast_error_even != dark.animate_toast_error_even
    }),
    ("--animate-label-in", |light, dark| {
        light.animate_label_in != dark.animate_label_in
    }),
    ("--radius", |light, dark| light.radius != dark.radius),
    ("--background", |light, dark| {
        light.background != dark.background
    }),
    ("--pane-background", |light, dark| {
        light.pane_background != dark.pane_background
    }),
    ("--foreground", |light, dark| {
        light.foreground != dark.foreground
    }),
    ("--card", |light, dark| light.card != dark.card),
    ("--card-foreground", |light, dark| {
        light.card_foreground != dark.card_foreground
    }),
    ("--popover", |light, dark| light.popover != dark.popover),
    ("--popover-foreground", |light, dark| {
        light.popover_foreground != dark.popover_foreground
    }),
    ("--primary", |light, dark| light.primary != dark.primary),
    ("--primary-foreground", |light, dark| {
        light.primary_foreground != dark.primary_foreground
    }),
    ("--secondary", |light, dark| {
        light.secondary != dark.secondary
    }),
    ("--secondary-foreground", |light, dark| {
        light.secondary_foreground != dark.secondary_foreground
    }),
    ("--muted", |light, dark| light.muted != dark.muted),
    ("--muted-foreground", |light, dark| {
        light.muted_foreground != dark.muted_foreground
    }),
    ("--accent", |light, dark| light.accent != dark.accent),
    ("--accent-foreground", |light, dark| {
        light.accent_foreground != dark.accent_foreground
    }),
    ("--editor-selection", |light, dark| {
        light.editor_selection != dark.editor_selection
    }),
    ("--destructive", |light, dark| {
        light.destructive != dark.destructive
    }),
    ("--destructive-foreground", |light, dark| {
        light.destructive_foreground != dark.destructive_foreground
    }),
    ("--info", |light, dark| light.info != dark.info),
    ("--info-foreground", |light, dark| {
        light.info_foreground != dark.info_foreground
    }),
    ("--success", |light, dark| light.success != dark.success),
    ("--success-foreground", |light, dark| {
        light.success_foreground != dark.success_foreground
    }),
    ("--warning", |light, dark| light.warning != dark.warning),
    ("--warning-foreground", |light, dark| {
        light.warning_foreground != dark.warning_foreground
    }),
    ("--border", |light, dark| light.border != dark.border),
    ("--input", |light, dark| light.input != dark.input),
    ("--ring", |light, dark| light.ring != dark.ring),
    ("--chart-1", |light, dark| light.chart_1 != dark.chart_1),
    ("--chart-2", |light, dark| light.chart_2 != dark.chart_2),
    ("--chart-3", |light, dark| light.chart_3 != dark.chart_3),
    ("--chart-4", |light, dark| light.chart_4 != dark.chart_4),
    ("--chart-5", |light, dark| light.chart_5 != dark.chart_5),
    ("--sidebar", |light, dark| light.sidebar != dark.sidebar),
    ("--sidebar-foreground", |light, dark| {
        light.sidebar_foreground != dark.sidebar_foreground
    }),
    ("--sidebar-primary", |light, dark| {
        light.sidebar_primary != dark.sidebar_primary
    }),
    ("--sidebar-primary-foreground", |light, dark| {
        light.sidebar_primary_foreground != dark.sidebar_primary_foreground
    }),
    ("--sidebar-accent", |light, dark| {
        light.sidebar_accent != dark.sidebar_accent
    }),
    ("--sidebar-accent-foreground", |light, dark| {
        light.sidebar_accent_foreground != dark.sidebar_accent_foreground
    }),
    ("--sidebar-border", |light, dark| {
        light.sidebar_border != dark.sidebar_border
    }),
    ("--sidebar-ring", |light, dark| {
        light.sidebar_ring != dark.sidebar_ring
    }),
    ("--sidebar-element-idle", |light, dark| {
        light.sidebar_element_idle != dark.sidebar_element_idle
    }),
    ("--sidebar-element-hover", |light, dark| {
        light.sidebar_element_hover != dark.sidebar_element_hover
    }),
    ("--sidebar-element-press", |light, dark| {
        light.sidebar_element_press != dark.sidebar_element_press
    }),
    ("--code", |light, dark| light.code != dark.code),
    ("--code-foreground", |light, dark| {
        light.code_foreground != dark.code_foreground
    }),
    ("--code-highlight", |light, dark| {
        light.code_highlight != dark.code_highlight
    }),
    ("--git-modified", |light, dark| {
        light.git_modified != dark.git_modified
    }),
    ("--git-modified-staged", |light, dark| {
        light.git_modified_staged != dark.git_modified_staged
    }),
    ("--git-added", |light, dark| {
        light.git_added != dark.git_added
    }),
    ("--git-untracked", |light, dark| {
        light.git_untracked != dark.git_untracked
    }),
    ("--git-deleted", |light, dark| {
        light.git_deleted != dark.git_deleted
    }),
    ("--git-renamed", |light, dark| {
        light.git_renamed != dark.git_renamed
    }),
    ("--chrome-bg", |light, dark| {
        light.chrome_bg != dark.chrome_bg
    }),
    ("--app-ui-scale", |light, dark| {
        light.app_ui_scale != dark.app_ui_scale
    }),
    ("--ui-text-xs", |light, dark| {
        light.ui_text_xs != dark.ui_text_xs
    }),
    ("--ui-text-sm", |light, dark| {
        light.ui_text_sm != dark.ui_text_sm
    }),
    ("--ui-text-base", |light, dark| {
        light.ui_text_base != dark.ui_text_base
    }),
    ("--ui-text-lg", |light, dark| {
        light.ui_text_lg != dark.ui_text_lg
    }),
    ("--ui-text-xl", |light, dark| {
        light.ui_text_xl != dark.ui_text_xl
    }),
    ("--app-scrollbar-size", |light, dark| {
        light.app_scrollbar_size != dark.app_scrollbar_size
    }),
    ("--app-scrollbar-thumb", |light, dark| {
        light.app_scrollbar_thumb != dark.app_scrollbar_thumb
    }),
    ("--app-scrollbar-thumb-border", |light, dark| {
        light.app_scrollbar_thumb_border != dark.app_scrollbar_thumb_border
    }),
    ("--app-scrollbar-thumb-hover", |light, dark| {
        light.app_scrollbar_thumb_hover != dark.app_scrollbar_thumb_hover
    }),
    ("--app-scrollbar-track", |light, dark| {
        light.app_scrollbar_track != dark.app_scrollbar_track
    }),
    ("--app-scrollbar-radius", |light, dark| {
        light.app_scrollbar_radius != dark.app_scrollbar_radius
    }),
    ("--syntax-comment", |light, dark| {
        light.syntax_comment != dark.syntax_comment
    }),
    ("--syntax-variable", |light, dark| {
        light.syntax_variable != dark.syntax_variable
    }),
    ("--syntax-punctuation", |light, dark| {
        light.syntax_punctuation != dark.syntax_punctuation
    }),
    ("--syntax-operator", |light, dark| {
        light.syntax_operator != dark.syntax_operator
    }),
    ("--syntax-error", |light, dark| {
        light.syntax_error != dark.syntax_error
    }),
    ("--syntax-keyword", |light, dark| {
        light.syntax_keyword != dark.syntax_keyword
    }),
    ("--syntax-string", |light, dark| {
        light.syntax_string != dark.syntax_string
    }),
    ("--syntax-number", |light, dark| {
        light.syntax_number != dark.syntax_number
    }),
    ("--syntax-constant", |light, dark| {
        light.syntax_constant != dark.syntax_constant
    }),
    ("--syntax-function", |light, dark| {
        light.syntax_function != dark.syntax_function
    }),
    ("--syntax-type", |light, dark| {
        light.syntax_type != dark.syntax_type
    }),
    ("--syntax-property", |light, dark| {
        light.syntax_property != dark.syntax_property
    }),
    ("--syntax-tag", |light, dark| {
        light.syntax_tag != dark.syntax_tag
    }),
    ("--syntax-attribute", |light, dark| {
        light.syntax_attribute != dark.syntax_attribute
    }),
    ("--syntax-boolean", |light, dark| {
        light.syntax_boolean != dark.syntax_boolean
    }),
    ("--syntax-null", |light, dark| {
        light.syntax_null != dark.syntax_null
    }),
    ("--syntax-regex", |light, dark| {
        light.syntax_regex != dark.syntax_regex
    }),
    ("--syntax-jsx", |light, dark| {
        light.syntax_jsx != dark.syntax_jsx
    }),
    ("--syntax-jsx-attribute", |light, dark| {
        light.syntax_jsx_attribute != dark.syntax_jsx_attribute
    }),
    ("--syntax-markdown-heading", |light, dark| {
        light.syntax_markdown_heading != dark.syntax_markdown_heading
    }),
    ("--syntax-markdown-bold", |light, dark| {
        light.syntax_markdown_bold != dark.syntax_markdown_bold
    }),
    ("--syntax-markdown-italic", |light, dark| {
        light.syntax_markdown_italic != dark.syntax_markdown_italic
    }),
    ("--syntax-markdown-strikethrough", |light, dark| {
        light.syntax_markdown_strikethrough != dark.syntax_markdown_strikethrough
    }),
    ("--syntax-markdown-link", |light, dark| {
        light.syntax_markdown_link != dark.syntax_markdown_link
    }),
    ("--syntax-markdown-link-text", |light, dark| {
        light.syntax_markdown_link_text != dark.syntax_markdown_link_text
    }),
    ("--syntax-markdown-code", |light, dark| {
        light.syntax_markdown_code != dark.syntax_markdown_code
    }),
    ("--syntax-markdown-list", |light, dark| {
        light.syntax_markdown_list != dark.syntax_markdown_list
    }),
    ("--syntax-markdown-quote", |light, dark| {
        light.syntax_markdown_quote != dark.syntax_markdown_quote
    }),
    ("--terminal-black", |light, dark| {
        light.terminal_black != dark.terminal_black
    }),
    ("--terminal-red", |light, dark| {
        light.terminal_red != dark.terminal_red
    }),
    ("--terminal-green", |light, dark| {
        light.terminal_green != dark.terminal_green
    }),
    ("--terminal-yellow", |light, dark| {
        light.terminal_yellow != dark.terminal_yellow
    }),
    ("--terminal-blue", |light, dark| {
        light.terminal_blue != dark.terminal_blue
    }),
    ("--terminal-magenta", |light, dark| {
        light.terminal_magenta != dark.terminal_magenta
    }),
    ("--terminal-cyan", |light, dark| {
        light.terminal_cyan != dark.terminal_cyan
    }),
    ("--terminal-white", |light, dark| {
        light.terminal_white != dark.terminal_white
    }),
    ("--terminal-bright-black", |light, dark| {
        light.terminal_bright_black != dark.terminal_bright_black
    }),
    ("--terminal-bright-red", |light, dark| {
        light.terminal_bright_red != dark.terminal_bright_red
    }),
    ("--terminal-bright-green", |light, dark| {
        light.terminal_bright_green != dark.terminal_bright_green
    }),
    ("--terminal-bright-yellow", |light, dark| {
        light.terminal_bright_yellow != dark.terminal_bright_yellow
    }),
    ("--terminal-bright-blue", |light, dark| {
        light.terminal_bright_blue != dark.terminal_bright_blue
    }),
    ("--terminal-bright-magenta", |light, dark| {
        light.terminal_bright_magenta != dark.terminal_bright_magenta
    }),
    ("--terminal-bright-cyan", |light, dark| {
        light.terminal_bright_cyan != dark.terminal_bright_cyan
    }),
    ("--terminal-bright-white", |light, dark| {
        light.terminal_bright_white != dark.terminal_bright_white
    }),
    ("--file-tree-hover-bg", |light, dark| {
        light.file_tree_hover_bg != dark.file_tree_hover_bg
    }),
    ("--file-tree-guide-icon-offset", |light, dark| {
        light.file_tree_guide_icon_offset != dark.file_tree_guide_icon_offset
    }),
    ("--tree-guide-color", |light, dark| {
        light.tree_guide_color != dark.tree_guide_color
    }),
];

/// The token names `theme.css` declares in **both** `:root` and `.dark`.
///
/// Ten of them declare the *same* value in both blocks (`--primary` and
/// friends), so being on this list is not the same as varying by
/// appearance; and sixty tokens that are declared once still vary,
/// because they alias one that does. `token_variance` is the truth about
/// what actually differs — this is the truth about what the CSS declares.
#[cfg(test)]
pub(super) const DUAL_DECLARED: [&str; 74] = [
    "--accent",
    "--accent-foreground",
    "--background",
    "--border",
    "--card",
    "--card-foreground",
    "--chart-1",
    "--chart-2",
    "--chart-3",
    "--chart-4",
    "--chart-5",
    "--chrome-bg",
    "--code",
    "--code-foreground",
    "--code-highlight",
    "--destructive",
    "--destructive-foreground",
    "--editor-selection",
    "--foreground",
    "--git-added",
    "--git-deleted",
    "--git-modified",
    "--git-modified-staged",
    "--git-renamed",
    "--git-untracked",
    "--info",
    "--info-foreground",
    "--input",
    "--muted",
    "--muted-foreground",
    "--pane-background",
    "--popover",
    "--popover-foreground",
    "--primary",
    "--primary-foreground",
    "--ring",
    "--secondary",
    "--secondary-foreground",
    "--sidebar",
    "--sidebar-accent",
    "--sidebar-accent-foreground",
    "--sidebar-border",
    "--sidebar-element-hover",
    "--sidebar-element-idle",
    "--sidebar-element-press",
    "--sidebar-foreground",
    "--sidebar-primary",
    "--sidebar-primary-foreground",
    "--sidebar-ring",
    "--success",
    "--success-foreground",
    "--syntax-attribute",
    "--syntax-boolean",
    "--syntax-constant",
    "--syntax-function",
    "--syntax-jsx",
    "--syntax-jsx-attribute",
    "--syntax-keyword",
    "--syntax-markdown-bold",
    "--syntax-markdown-code",
    "--syntax-markdown-heading",
    "--syntax-markdown-italic",
    "--syntax-markdown-link",
    "--syntax-markdown-link-text",
    "--syntax-markdown-list",
    "--syntax-null",
    "--syntax-number",
    "--syntax-property",
    "--syntax-regex",
    "--syntax-string",
    "--syntax-tag",
    "--syntax-type",
    "--warning",
    "--warning-foreground",
];
