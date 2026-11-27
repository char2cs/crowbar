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

#[derive(Debug, Clone, PartialEq)]
pub struct FileTreeView {
    pub indent: f32,
    pub density: FileDensity,
    pub compact_folders: bool,
    pub shown: BTreeSet<FileTreeFlag>,
    pub hidden_files: Vec<String>,
    pub hidden_directories: Vec<String>,
    pub compact_badges: bool,
    pub icons: IconTheme,
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
    pub fn flag(
        &self,
        flag: FileTreeFlag,
    ) -> bool {
        self.shown.contains(&flag)
    }

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
