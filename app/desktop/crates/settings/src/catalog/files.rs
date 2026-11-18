//! The Files page.

use super::number_setting::NumberSetting;
use super::rows::row;
use crate::control::Control;
use crate::control_width::ControlWidth;
use crate::page::Page;
use crate::row::Row;
use crate::section::Section;
use crate::value::Value;

const INDENT_SIZE: NumberSetting = NumberSetting::new((8.0, 32.0, 1.0), 16.0);

pub(super) fn page() -> Page {
    Page::new("file-explorer", "Files", "tree-structure")
        .block(display())
        .block(filters())
}

fn display() -> Section {
    Section::new("Display")
        .headerless()
        .row(row(
            ("fileTreeIndentSize", "Indent Size"),
            "Pixels per nesting level",
            INDENT_SIZE.plain(ControlWidth::NumberCompact),
            INDENT_SIZE.default,
        ))
        .row(density())
        .row(row(
            ("compactFoldersInFileTree", "Compact Folders"),
            "Collapse single-child folder chains",
            Control::switch(),
            Value::Bool(false),
        ))
        .row(row(
            ("showHiddenFilesInFileTree", "Show Hidden Files"),
            "Show dotfiles and hidden directories",
            Control::switch(),
            Value::Bool(true),
        ))
        .row(row(
            ("showGitignoredFilesInFileTree", "Respect .gitignore"),
            "Hide files matched by root and nested .gitignore files",
            Control::inverted_switch(),
            Value::Bool(true),
        ))
}

fn density() -> Row {
    row(
        ("fileTreeDensity", "Density"),
        "Choose file tree row spacing",
        Control::select(
            &[
                ("compact", "Compact"),
                ("default", "Default"),
                ("comfortable", "Comfortable"),
            ],
            ControlWidth::Default,
        ),
        "default",
    )
}

fn filters() -> Section {
    Section::new("Filters")
        .row(patterns(
            "hiddenFilePatterns",
            "Hidden Files",
            "*.log, *.tmp, **/*.bak",
        ))
        .row(patterns(
            "hiddenDirectoryPatterns",
            "Hidden Directories",
            "node_modules, .git, build/",
        ))
}

fn patterns(
    key: &str,
    label: &str,
    placeholder: &str,
) -> Row {
    row(
        (key, label),
        "Comma-separated glob patterns",
        Control::text_list(placeholder, ControlWidth::Text),
        Value::List(Vec::new()),
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn gitignore_is_inverted_and_filters_default_to_empty_lists() {
        let page = page();
        let gitignore = page
            .rows()
            .find(|row| row.key().as_str() == "showGitignoredFilesInFileTree");
        assert!(matches!(
            gitignore.map(|row| &row.control),
            Some(Control::Switch { invert: true })
        ));
        let filters: Vec<&Row> = page
            .rows()
            .filter(|row| row.key().as_str().starts_with("hidden"))
            .collect();
        assert_eq!(filters.len(), 2);
        assert!(
            filters
                .iter()
                .all(|row| row.default == Some(Value::List(Vec::new())))
        );
    }
}
