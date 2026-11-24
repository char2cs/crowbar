//! The Editor page.

use super::number_setting::NumberSetting;
use super::rows::row;
use crate::schema::Control;
use crate::schema::ControlWidth;
use crate::schema::Page;
use crate::schema::Row;
use crate::schema::Section;
use crate::values::Value;

const FONT_SIZE: NumberSetting = NumberSetting::new((8.0, 32.0, 1.0), 14.0);
const LINE_HEIGHT: NumberSetting = NumberSetting::normalized((1.0, 2.0, 0.1), 1.4);
const TAB_SIZE: NumberSetting = NumberSetting::new((1.0, 8.0, 1.0), 2.0);
const MAX_OPEN_TABS: NumberSetting = NumberSetting::new((1.0, 100.0, 1.0), 100.0);
const MARKDOWN_FONT_SIZE: NumberSetting = NumberSetting::normalized((12.0, 24.0, 1.0), 16.0);

pub(super) fn page() -> Page {
    Page::new("editor", "Editor", "code-block")
        .indexed(super::search_index::entries(super::search_index::EDITOR))
        .block(assist(saving(display(typing()))))
        .block(markdown())
}

fn font_family() -> Row {
    row(
        ("fontFamily", "Editor Font Family"),
        "Font family for code editor",
        Control::provided_select("monoFonts", ControlWidth::Text),
        "Geist Mono Variable",
    )
}

fn typing() -> Section {
    let compact = ControlWidth::NumberCompact;
    Section::new("Editor")
        .headerless()
        .row(font_family())
        .row(row(
            ("fontSize", "Font Size"),
            "Editor font size in pixels",
            FONT_SIZE.plain(compact),
            FONT_SIZE.default,
        ))
        .row(row(
            ("editorLineHeight", "Line Height"),
            "Editor line height multiplier",
            LINE_HEIGHT.snapped(compact),
            LINE_HEIGHT.default,
        ))
        .row(row(
            ("tabSize", "Tab Size"),
            "Number of spaces per tab",
            TAB_SIZE.plain(compact),
            TAB_SIZE.default,
        ))
}

fn render_whitespace() -> Row {
    row(
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
    )
}

fn display(section: Section) -> Section {
    section
        .row(row(
            ("wordWrap", "Word Wrap"),
            "Wrap lines that exceed viewport width",
            Control::switch(),
            Value::Bool(false),
        ))
        .row(row(
            ("lineNumbers", "Line Numbers"),
            "Show line numbers in the editor",
            Control::switch(),
            Value::Bool(true),
        ))
        .row(render_whitespace())
        .row(row(
            ("renderIndentGuides", "Indent Guides"),
            "Show vertical guides for indentation levels",
            Control::switch(),
            Value::Bool(true),
        ))
        .row(row(
            ("semanticHighlighting", "Semantic Highlighting"),
            "Color functions, types, and properties using language servers and grammars",
            Control::switch(),
            Value::Bool(true),
        ))
}

fn saving(section: Section) -> Section {
    let compact = ControlWidth::NumberCompact;
    section
        .row(row(
            ("highlightOccurrences", "Highlight Occurrences"),
            "Highlight visible matches for the word under the cursor",
            Control::switch(),
            Value::Bool(false),
        ))
        .row(row(
            ("maxOpenTabs", "Max Open Tabs"),
            "Maximum number of tabs before oldest closes",
            MAX_OPEN_TABS.plain(compact),
            MAX_OPEN_TABS.default,
        ))
        .row(row(
            ("autoSave", "Auto Save"),
            "Automatically save files when editing",
            Control::switch(),
            Value::Bool(false),
        ))
        .row(row(
            ("formatOnSave", "Format on Save"),
            "Format with the language server when saving (manual saves only)",
            Control::switch(),
            Value::Bool(false),
        ))
}

fn assist(section: Section) -> Section {
    section
        .row(row(
            ("inlineBlame", "Inline Blame"),
            "Show who last changed the cursor's line, and when",
            Control::switch(),
            Value::Bool(true),
        ))
        .row(row(
            ("autoCompletion", "Auto Completion"),
            "Show completion suggestions while typing",
            Control::switch(),
            Value::Bool(true),
        ))
        .row(row(
            ("parameterHints", "Parameter Hints"),
            "Show function parameter hints",
            Control::switch(),
            Value::Bool(true),
        ))
}

fn markdown() -> Section {
    Section::new("Markdown").row(row(
        ("markdownFontSize", "Markdown Font Size"),
        "Base text size for the rich markdown editor, in pixels",
        MARKDOWN_FONT_SIZE.plain(ControlWidth::NumberCompact),
        MARKDOWN_FONT_SIZE.default,
    ))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn every_editor_setting_of_the_spec_has_a_row_with_a_default() {
        let page = page();
        assert_eq!(
            page.rows().count(),
            17,
            "develop's rows without the minimap (plan D4)"
        );
        assert!(
            page.rows().all(|row| row.default.is_some()),
            "every row is resettable"
        );
        let first = page.rows().next().map(|row| row.key().as_str());
        assert_eq!(first, Some("fontFamily"));
        assert_eq!(
            page.rows().last().map(|row| row.key().as_str()),
            Some("markdownFontSize")
        );
    }
}
