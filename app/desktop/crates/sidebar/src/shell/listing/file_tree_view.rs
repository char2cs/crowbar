//! How the shell's settings shape the Files and Changes panels.

pub(super) mod file_density;
pub(super) mod file_tree_flag;
pub(super) mod icon_theme;
pub(super) mod indent_guides;

use std::collections::BTreeSet;

use crate::shell::listing::file_tree_view::file_density::FileDensity;
use crate::shell::listing::file_tree_view::file_tree_flag::FileTreeFlag;
use crate::shell::listing::file_tree_view::icon_theme::IconTheme;
use crate::shell::listing::file_tree_view::indent_guides::IndentGuides;

const DEFAULT_INDENT: f32 = 16.0;

/// The file-tree settings the sidebar draws with. The shell reads them from its settings store
/// on every render, so a change shows without a restart.
#[derive(Debug, Clone, PartialEq)]
pub struct FileTreeView {
    /// Pixels per nesting level (`fileTreeIndentSize`).
    pub indent: f32,
    /// Row pitch (`fileTreeDensity`).
    pub density: FileDensity,
    /// Whether single-child folder chains share one row (`compactFoldersInFileTree`).
    pub compact_folders: bool,
    /// The filter-menu switches that are on: dotfiles (`showHiddenFilesInFileTree`), gitignored
    /// files (`showGitignoredFilesInFileTree`) and git status (`showGitStatusInFileTree`).
    pub shown: BTreeSet<FileTreeFlag>,
    /// Gitignore-style patterns of hidden files (`hiddenFilePatterns`).
    pub hidden_files: Vec<String>,
    /// Gitignore-style patterns of hidden directories (`hiddenDirectoryPatterns`).
    pub hidden_directories: Vec<String>,
    /// Whether the change counts sit closer together (`compactGitStatusBadges`).
    pub compact_badges: bool,
    /// The glyph set of files and folders (`iconTheme`).
    pub icons: IconTheme,
    /// Whether nested rows draw a rule per ancestor level (`renderIndentGuides`).
    pub guides: IndentGuides,
}

impl Default for FileTreeView {
    fn default() -> Self {
        Self {
            indent: DEFAULT_INDENT,
            density: FileDensity::Standard,
            compact_folders: false,
            shown: FileTreeFlag::ALL.into_iter().collect(),
            hidden_files: Vec::new(),
            hidden_directories: Vec::new(),
            compact_badges: false,
            icons: IconTheme::default(),
            guides: IndentGuides::Drawn,
        }
    }
}

impl FileTreeView {
    /// Whether `flag` is on.
    pub fn flag(
        &self,
        flag: FileTreeFlag,
    ) -> bool {
        self.shown.contains(&flag)
    }

    /// Whether any switch of the filter menu is off, which lights the filter button.
    pub fn any_filter_off(&self) -> bool {
        self.shown.len() < FileTreeFlag::ALL.len()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_defaults_match_the_settings_defaults_and_light_no_filter() {
        let view = FileTreeView::default();
        assert!((view.indent - 16.0).abs() < f32::EPSILON);
        assert!(!view.compact_folders && !view.compact_badges);
        assert!(!view.any_filter_off());
    }

    fn view_without(off: FileTreeFlag) -> FileTreeView {
        FileTreeView {
            shown: FileTreeFlag::ALL
                .into_iter()
                .filter(|flag| *flag != off)
                .collect(),
            ..FileTreeView::default()
        }
    }

    #[test]
    fn turning_a_switch_off_lights_the_filter_and_reads_back() {
        for flag in FileTreeFlag::ALL {
            let view = view_without(flag);
            assert!(view.any_filter_off(), "{flag:?}");
            assert!(!view.flag(flag), "{flag:?}");
        }
    }
}
