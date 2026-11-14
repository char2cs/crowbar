//! The colour tokens of one theme in one mode.

use gpui::{Hsla, Rgba};

mod crowbar_chrome_light;
mod crowbar_dark;
mod crowbar_light;
mod frosted_dark;
mod frosted_light;

macro_rules! palette {
    ($($field:ident => $css:literal),* $(,)?) => {
        /// Resolved colour tokens of one theme and mode; one accessor per CSS custom property.
        #[derive(Debug, Clone, Copy, PartialEq)]
        pub struct Palette {
            $($field: Rgba,)*
        }

        impl Palette {
            $(
                #[doc = concat!("The `", $css, "` token.")]
                pub fn $field(&self) -> Hsla {
                    Hsla::from(self.$field)
                }
            )*
        }
    };
}

palette! {
    background => "--background",
    pane_background => "--pane-background",
    pane_chrome_bg => "--pane-chrome-bg",
    chrome_bg => "--chrome-bg",
    foreground => "--foreground",
    card => "--card",
    popover => "--popover",
    muted => "--muted",
    muted_foreground => "--muted-foreground",
    accent => "--accent",
    accent_foreground => "--accent-foreground",
    border => "--border",
    input => "--input",
    ring => "--ring",
    primary => "--primary",
    primary_foreground => "--primary-foreground",
    secondary => "--secondary",
    secondary_foreground => "--secondary-foreground",
    destructive => "--destructive",
    destructive_foreground => "--destructive-foreground",
    success => "--success",
    success_foreground => "--success-foreground",
    warning => "--warning",
    warning_foreground => "--warning-foreground",
    info => "--info",
    info_foreground => "--info-foreground",
    control_surface => "--control-surface",
    control_foreground => "--control-foreground",
    control_muted_foreground => "--control-muted-foreground",
    control_highlight => "--control-highlight",
    elevated_highlight => "--elevated-highlight",
    pane_accent => "--pane-accent",
    pane_border => "--pane-border",
    editor_selection => "--editor-selection",
    sidebar => "--sidebar",
    sidebar_foreground => "--sidebar-foreground",
    sidebar_border => "--sidebar-border",
    sidebar_accent => "--sidebar-accent",
    sidebar_element_idle => "--sidebar-element-idle",
    sidebar_element_hover => "--sidebar-element-hover",
    sidebar_element_press => "--sidebar-element-press",
    sidebar_count_added => "--sidebar-count-added",
    sidebar_count_deleted => "--sidebar-count-deleted",
    sidebar_drop_line => "--sidebar-drop-line",
    sidebar_drop_nest => "--sidebar-drop-nest",
    sidebar_drop_nest_edge => "--sidebar-drop-nest-edge",
    git_added => "--git-added",
    git_deleted => "--git-deleted",
    git_modified => "--git-modified",
    git_modified_staged => "--git-modified-staged",
    git_untracked => "--git-untracked",
}

impl Palette {
    pub(crate) const CROWBAR_LIGHT: Self = crowbar_light::PALETTE;
    pub(crate) const CROWBAR_DARK: Self = crowbar_dark::PALETTE;
    pub(crate) const CROWBAR_CHROME_LIGHT: Self = crowbar_chrome_light::PALETTE;
    pub(crate) const FROSTED_LIGHT: Self = frosted_light::PALETTE;
    pub(crate) const FROSTED_DARK: Self = frosted_dark::PALETTE;
}
