//! The Files page.

use super::rows::{number, row};
use crate::control::Control;
use crate::control_width::ControlWidth;
use crate::page::Page;
use crate::row::Row;
use crate::section::Section;
use crate::value::Value;

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
            number((8.0, 32.0, 1.0), ControlWidth::NumberCompact),
            16.0,
        ))
        .row(row(
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
        ))
        .row(row(
            ("compactFoldersInFileTree", "Compact Folders"),
            "Collapse single-child folder chains",
            Control::switch(),
            false,
        ))
        .row(row(
            ("showHiddenFilesInFileTree", "Show Hidden Files"),
            "Show dotfiles and hidden directories",
            Control::switch(),
            true,
        ))
        .row(row(
            ("showGitignoredFilesInFileTree", "Respect .gitignore"),
            "Hide files matched by root and nested .gitignore files",
            Control::inverted_switch(),
            true,
        ))
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

fn patterns(key: &str, label: &str, placeholder: &str) -> Row {
    row(
        (key, label),
        "Comma-separated glob patterns",
        Control::text_list(placeholder, ControlWidth::Text),
        Value::List(Vec::new()),
    )
}

#[cfg(test)]
#[path = "files_tests.rs"]
mod tests;
