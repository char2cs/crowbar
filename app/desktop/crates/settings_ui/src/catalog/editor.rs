//! The Editor page.

use super::rows::{number, row, snapped};
use crate::control::Control;
use crate::control_width::ControlWidth;
use crate::page::Page;
use crate::section::Section;

pub(super) fn page() -> Page {
    Page::new("editor", "Editor", "code-block")
        .block(assist(saving(display(typing()))))
        .block(markdown())
}

fn typing() -> Section {
    let compact = ControlWidth::NumberCompact;
    Section::new("Editor")
        .headerless()
        .row(row(
            ("fontFamily", "Editor Font Family"),
            "Font family for code editor",
            Control::provided_select("monoFonts", ControlWidth::Text),
            "Geist Mono Variable",
        ))
        .row(row(
            ("fontSize", "Font Size"),
            "Editor font size in pixels",
            number((8.0, 32.0, 1.0), compact),
            14.0,
        ))
        .row(row(
            ("editorLineHeight", "Line Height"),
            "Editor line height multiplier",
            snapped((1.0, 2.0, 0.1), compact),
            1.4,
        ))
        .row(row(
            ("tabSize", "Tab Size"),
            "Number of spaces per tab",
            number((1.0, 8.0, 1.0), compact),
            2.0,
        ))
}

fn display(section: Section) -> Section {
    section
        .row(row(
            ("wordWrap", "Word Wrap"),
            "Wrap lines that exceed viewport width",
            Control::switch(),
            false,
        ))
        .row(row(
            ("lineNumbers", "Line Numbers"),
            "Show line numbers in the editor",
            Control::switch(),
            true,
        ))
        .row(row(
            ("renderWhitespace", "Render Whitespace"),
            "Show visible markers for spaces and tabs",
            Control::select(
                &[
                    ("none", "None"),
                    ("boundary", "Boundary"),
                    ("trailing", "Trailing"),
                    ("all", "All"),
                ],
                ControlWidth::Default,
            ),
            "none",
        ))
        .row(row(
            ("renderIndentGuides", "Indent Guides"),
            "Show vertical guides for indentation levels",
            Control::switch(),
            true,
        ))
        .row(row(
            ("semanticHighlighting", "Semantic Highlighting"),
            "Color functions, types, and properties using language servers and grammars",
            Control::switch(),
            true,
        ))
}

fn saving(section: Section) -> Section {
    let compact = ControlWidth::NumberCompact;
    section
        .row(row(
            ("highlightOccurrences", "Highlight Occurrences"),
            "Highlight visible matches for the word under the cursor",
            Control::switch(),
            false,
        ))
        .row(row(
            ("showMinimap", "Show Minimap"),
            "Show a minimap overview on the right side of the editor",
            Control::switch(),
            false,
        ))
        .row(row(
            ("maxOpenTabs", "Max Open Tabs"),
            "Maximum number of tabs before oldest closes",
            number((1.0, 100.0, 1.0), compact),
            100.0,
        ))
        .row(row(
            ("autoSave", "Auto Save"),
            "Automatically save files when editing",
            Control::switch(),
            false,
        ))
        .row(row(
            ("formatOnSave", "Format on Save"),
            "Format with the language server when saving (manual saves only)",
            Control::switch(),
            false,
        ))
}

fn assist(section: Section) -> Section {
    section
        .row(row(
            ("inlineBlame", "Inline Blame"),
            "Show who last changed the cursor's line, and when",
            Control::switch(),
            true,
        ))
        .row(row(
            ("autoCompletion", "Auto Completion"),
            "Show completion suggestions while typing",
            Control::switch(),
            true,
        ))
        .row(row(
            ("parameterHints", "Parameter Hints"),
            "Show function parameter hints",
            Control::switch(),
            true,
        ))
}

fn markdown() -> Section {
    Section::new("Markdown").row(row(
        ("markdownFontSize", "Markdown Font Size"),
        "Base text size for the rich markdown editor, in pixels",
        number((12.0, 24.0, 1.0), ControlWidth::NumberCompact),
        16.0,
    ))
}

#[cfg(test)]
#[path = "editor_tests.rs"]
mod tests;
