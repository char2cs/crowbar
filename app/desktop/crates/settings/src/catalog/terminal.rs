//! The Terminal page.

use super::number_setting::NumberSetting;
use super::rows::row;
use crate::schema::Control;
use crate::schema::ControlWidth;
use crate::schema::Page;
use crate::schema::Section;
use crate::values::Value;

const FONT_HINT: &str = "Note: Selected font must be installed on your system to work correctly. If icons are missing, try installing a Nerd Font.";

const FONT_SIZE: NumberSetting = NumberSetting::new((8.0, 32.0, 1.0), 14.0);
const LINE_HEIGHT: NumberSetting = NumberSetting::new((1.0, 2.0, 0.1), 1.0);
const LETTER_SPACING: NumberSetting = NumberSetting::new((-5.0, 5.0, 0.1), 0.0);
const SCROLLBACK: NumberSetting = NumberSetting::new((1000.0, 100_000.0, 1000.0), 10_000.0);
const CURSOR_WIDTH: NumberSetting = NumberSetting::new((1.0, 6.0, 1.0), 2.0);

pub(super) fn page() -> Page {
    Page::new("terminal", "Terminal", "terminal-window")
        .indexed(super::search_index::entries(super::search_index::TERMINAL))
        .block(typography())
        .block(cursor())
}

fn typography() -> Section {
    let compact = ControlWidth::Compact;
    Section::new("Typography")
        .headerless()
        .row(row(
            ("terminalFontFamily", "Font Family"),
            "Font family for the integrated terminal. Select a Nerd Font for best icon support.",
            Control::provided_select("terminalFonts", ControlWidth::ExtraWide)
                .placeholder("Select font...")
                .hint(FONT_HINT),
            "Geist Mono Variable",
        ))
        .row(row(
            ("terminalFontSize", "Font Size"),
            "Terminal font size in pixels",
            FONT_SIZE.plain(compact),
            FONT_SIZE.default,
        ))
        .row(row(
            ("terminalLineHeight", "Line Height"),
            "Line height multiplier",
            LINE_HEIGHT.snapped(compact),
            LINE_HEIGHT.default,
        ))
        .row(row(
            ("terminalLetterSpacing", "Letter Spacing"),
            "Additional spacing between characters",
            LETTER_SPACING.snapped(compact),
            LETTER_SPACING.default,
        ))
        .row(row(
            ("terminalScrollback", "Scrollback"),
            "How many lines of terminal history to keep in memory",
            SCROLLBACK.plain(ControlWidth::Default),
            SCROLLBACK.default,
        ))
}

fn cursor() -> Section {
    Section::new("Cursor")
        .row(row(
            ("terminalCursorStyle", "Cursor Style"),
            "Shape of the cursor",
            Control::select(
                &[
                    ("block", "Block"),
                    ("underline", "Underline"),
                    ("bar", "Bar"),
                ],
                ControlWidth::Default,
            ),
            "block",
        ))
        .row(row(
            ("terminalCursorBlink", "Blinking Cursor"),
            "Whether the cursor should blink",
            Control::switch(),
            Value::Bool(true),
        ))
        .row(row(
            ("terminalCursorWidth", "Cursor Width"),
            "Thickness of the bar or block cursor",
            CURSOR_WIDTH.plain(ControlWidth::Compact),
            CURSOR_WIDTH.default,
        ))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_font_row_explains_nerd_fonts_and_all_eight_consumers_have_rows() {
        let page = page();
        assert_eq!(page.rows().count(), 8);
        let font = page.rows().next().map(|row| &row.control);
        assert!(matches!(
            font,
            Some(Control::Select {
                hint: Some(_),
                placeholder: Some(_),
                ..
            })
        ));
    }
}
