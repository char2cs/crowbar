//! The Files and Changes panels' look, read from the stored settings.

use settings_ui::Value;
use sidebar::{FileDensity, FileTreeFlag, FileTreeView, IconTheme};

const INDENT: &str = "fileTreeIndentSize";
const DENSITY: &str = "fileTreeDensity";
const COMPACT_FOLDERS: &str = "compactFoldersInFileTree";
const HIDDEN_FILES: &str = "hiddenFilePatterns";
const HIDDEN_DIRECTORIES: &str = "hiddenDirectoryPatterns";
const COMPACT_BADGES: &str = "compactGitStatusBadges";
const ICON_THEME: &str = "iconTheme";

fn stored_flag(
    read: &impl Fn(&str) -> Option<Value>,
    key: &str,
) -> Option<bool> {
    read(key).and_then(|value| value.as_bool())
}

fn patterns(
    read: &impl Fn(&str) -> Option<Value>,
    key: &str,
) -> Vec<String> {
    read(key)
        .and_then(|value| value.as_list().map(<[String]>::to_vec))
        .unwrap_or_default()
}

#[expect(
    clippy::cast_possible_truncation,
    reason = "an indent in pixels fits an f32 exactly enough"
)]
fn indent(
    read: &impl Fn(&str) -> Option<Value>,
    fallback: f32,
) -> f32 {
    read(INDENT)
        .and_then(|value| value.as_number())
        .map_or(fallback, |pixels| pixels as f32)
}

pub(crate) fn view_of(read: impl Fn(&str) -> Option<Value>) -> FileTreeView {
    let base = FileTreeView::default();
    let shown = FileTreeFlag::ALL
        .into_iter()
        .filter(|flag| stored_flag(&read, flag.setting_key()).unwrap_or_else(|| base.flag(*flag)))
        .collect();
    FileTreeView {
        indent: indent(&read, base.indent),
        density: read(DENSITY)
            .and_then(|value| value.as_text().map(FileDensity::from_setting))
            .unwrap_or(base.density),
        compact_folders: stored_flag(&read, COMPACT_FOLDERS).unwrap_or(base.compact_folders),
        shown,
        hidden_files: patterns(&read, HIDDEN_FILES),
        hidden_directories: patterns(&read, HIDDEN_DIRECTORIES),
        compact_badges: stored_flag(&read, COMPACT_BADGES).unwrap_or(base.compact_badges),
        icons: read(ICON_THEME)
            .and_then(|value| value.as_text().map(IconTheme::from_setting))
            .unwrap_or(base.icons),
    }
}

#[cfg(test)]
mod tests {
    use std::collections::BTreeMap;

    use super::*;

    type Case = (&'static str, Value, fn(&FileTreeView) -> bool);

    fn stored(pairs: &[(&str, Value)]) -> impl Fn(&str) -> Option<Value> {
        let map: BTreeMap<String, Value> = pairs
            .iter()
            .map(|(key, value)| ((*key).to_owned(), value.clone()))
            .collect();
        move |key| map.get(key).cloned()
    }

    #[test]
    fn nothing_stored_is_the_default_look() {
        assert_eq!(view_of(stored(&[])), FileTreeView::default());
    }

    #[test]
    fn each_stored_setting_moves_its_own_part_of_the_look() {
        let patterns = Value::List(vec!["*.log".into(), "tmp".into()]);
        let cases: [Case; 10] = [
            ("fileTreeIndentSize", Value::Number(24.0), |view| {
                (view.indent - 24.0).abs() < f32::EPSILON
            }),
            ("fileTreeDensity", Value::Text("compact".into()), |view| {
                view.density == FileDensity::Compact
            }),
            ("compactFoldersInFileTree", Value::Bool(true), |view| {
                view.compact_folders
            }),
            ("showHiddenFilesInFileTree", Value::Bool(false), |view| {
                !view.flag(FileTreeFlag::Hidden) && view.any_filter_off()
            }),
            (
                "showGitignoredFilesInFileTree",
                Value::Bool(false),
                |view| !view.flag(FileTreeFlag::Gitignored),
            ),
            ("showGitStatusInFileTree", Value::Bool(false), |view| {
                !view.flag(FileTreeFlag::GitStatus)
            }),
            ("compactGitStatusBadges", Value::Bool(true), |view| {
                view.compact_badges
            }),
            ("iconTheme", Value::Text("classic".into()), |view| {
                view.icons == IconTheme::Classic
            }),
            ("hiddenFilePatterns", patterns.clone(), |view| {
                view.hidden_files == ["*.log", "tmp"] && view.hidden_directories.is_empty()
            }),
            ("hiddenDirectoryPatterns", patterns, |view| {
                view.hidden_directories == ["*.log", "tmp"] && view.hidden_files.is_empty()
            }),
        ];
        for (key, value, holds) in cases {
            assert!(holds(&view_of(stored(&[(key, value)]))), "{key}");
        }
    }

    #[test]
    fn a_value_of_the_wrong_type_is_ignored() {
        let view = view_of(stored(&[
            ("fileTreeIndentSize", Value::Text("wide".into())),
            ("compactFoldersInFileTree", Value::Number(1.0)),
        ]));
        assert_eq!(view, FileTreeView::default());
    }
}
