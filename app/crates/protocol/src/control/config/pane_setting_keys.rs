/// The settings a version 1 `settings` frame may carry, by their names in the settings
/// catalog: editor, terminal, browser, file tree, typography, icon theme, tab limit, chat
/// landing surface and the user's keybinding overrides for the chords the panes serve themselves. The theme itself travels in the `theme` frame, and shell-only settings
/// (sidebar, tab list, window) never leave the shell.
pub const PANE_SETTING_KEYS: [&str; 46] = [
    "autoSave",
    "fontFamily",
    "fontSize",
    "markdownFontSize",
    "editorLineHeight",
    "tabSize",
    "wordWrap",
    "lineNumbers",
    "renderWhitespace",
    "renderIndentGuides",
    "semanticHighlighting",
    "highlightOccurrences",
    "showMinimap",
    "formatOnSave",
    "inlineBlame",
    "autoCompletion",
    "parameterHints",
    "maxOpenTabs",
    "iconTheme",
    "uiFontFamily",
    "uiFontSize",
    "headingFontFamily",
    "terminalFontFamily",
    "terminalFontSize",
    "terminalLineHeight",
    "terminalLetterSpacing",
    "terminalScrollback",
    "terminalCursorStyle",
    "terminalCursorBlink",
    "terminalCursorWidth",
    "browserHomepage",
    "browserSearchEngine",
    "browserCustomSearchUrl",
    "browserZoom",
    "chatIsDefaultPresentation",
    "fileTreeIndentSize",
    "compactFoldersInFileTree",
    "fileTreeDensity",
    "showHiddenFilesInFileTree",
    "showGitignoredFilesInFileTree",
    "hiddenFilePatterns",
    "hiddenDirectoryPatterns",
    "showGitStatusInFileTree",
    "compactGitStatusBadges",
    "diffViewMode",
    "keybindingUserOverrides",
];

#[cfg(test)]
mod tests {
    use super::*;
    use crate::control::config::setting_key::SettingKey;

    #[test]
    fn every_key_is_valid_and_listed_once() {
        let mut seen = std::collections::BTreeSet::new();
        for key in PANE_SETTING_KEYS {
            assert!(SettingKey::new(key).is_ok(), "{key}");
            assert!(seen.insert(key), "{key} twice");
        }
        assert!(
            seen.contains("keybindingUserOverrides"),
            "panes serve their own chords"
        );
    }

    #[test]
    fn shell_owned_settings_stay_out() {
        for key in [
            "theme",
            "themeMode",
            "sidebarPosition",
            "tabListScope",
            "windowTransparency",
        ] {
            assert!(!PANE_SETTING_KEYS.contains(&key), "{key}");
        }
    }
}
