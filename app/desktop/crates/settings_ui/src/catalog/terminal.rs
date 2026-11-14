//! The Terminal page.

use super::rows::{number, row, snapped};
use crate::control::Control;
use crate::control_width::ControlWidth;
use crate::page::Page;
use crate::section::Section;

const FONT_HINT: &str = "Note: Selected font must be installed on your system to work correctly. If icons are missing, try installing a Nerd Font.";

pub(super) fn page() -> Page {
    Page::new("terminal", "Terminal", "terminal-window")
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
            number((8.0, 32.0, 1.0), compact),
            14.0,
        ))
        .row(row(
            ("terminalLineHeight", "Line Height"),
            "Line height multiplier",
            snapped((1.0, 2.0, 0.1), compact),
            1.0,
        ))
        .row(row(
            ("terminalLetterSpacing", "Letter Spacing"),
            "Additional spacing between characters",
            snapped((-5.0, 5.0, 0.1), compact),
            0.0,
        ))
        .row(row(
            ("terminalScrollback", "Scrollback"),
            "How many lines of terminal history to keep in memory",
            number((1000.0, 100_000.0, 1000.0), ControlWidth::Default),
            10_000.0,
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
            true,
        ))
        .row(row(
            ("terminalCursorWidth", "Cursor Width"),
            "Thickness of the bar or block cursor",
            number((1.0, 6.0, 1.0), ControlWidth::Compact),
            2.0,
        ))
}

#[cfg(test)]
#[path = "terminal_tests.rs"]
mod tests;
