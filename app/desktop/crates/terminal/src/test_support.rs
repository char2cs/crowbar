//! Builds terminals with known content for the unit tests.
#![cfg(test)]

use alacritty_terminal::Term;
use alacritty_terminal::term::Config as EmulatorConfig;

use crate::snapshot::CursorShape;
use crate::snapshot::Palette;
use crate::snapshot::Rgb;
use crate::snapshot::Row;
use crate::terminal::Config;
use crate::terminal::DaemonFrame;
use crate::terminal::Feed;
use crate::terminal::GridSize;
use crate::terminal::Terminal;
use crate::terminal::{EventQueue, EventSink};

pub(crate) const FOREGROUND: u32 = 0xaa_aaaa;
const BACKGROUND: u32 = 0x11_1111;
const ANSI_BASE: u32 = 0x10_0000;
const SCROLLBACK: usize = 1_000;

pub(crate) fn ansi(index: u32) -> Rgb {
    Rgb::from_hex(ANSI_BASE + index)
}

pub(crate) fn empty_terminal(
    columns: usize,
    lines: usize,
) -> Terminal {
    let palette = Palette {
        foreground: Rgb::from_hex(FOREGROUND),
        background: Rgb::from_hex(BACKGROUND),
        cursor: Rgb::from_hex(FOREGROUND),
        ansi: std::array::from_fn(|index| ansi(u32::try_from(index).unwrap_or_default())),
    };
    Terminal::new(Config {
        columns,
        lines,
        scrollback: SCROLLBACK,
        cursor_shape: CursorShape::Block,
        cursor_blink: false,
        palette,
    })
}

pub(crate) fn terminal_showing(
    output: &str,
    columns: usize,
    lines: usize,
) -> Terminal {
    let mut terminal = empty_terminal(columns, lines);
    terminal.apply(&DaemonFrame::Output(output.as_bytes().to_vec()));
    terminal
}

pub(crate) fn row_text(row: &Row) -> String {
    let mut text = String::new();
    for segment in &row.segments {
        let padding = segment.column.saturating_sub(text.chars().count());
        text.push_str(&" ".repeat(padding));
        text.push_str(&segment.text);
    }
    text
}

pub(crate) fn emulator_showing(
    output: &str,
    columns: usize,
    lines: usize,
) -> Term<EventSink> {
    let size = GridSize { columns, lines };
    let mut term = Term::new(
        EmulatorConfig::default(),
        &size,
        EventSink::new(EventQueue::default()),
    );
    Feed::default().apply(&mut term, &DaemonFrame::Output(output.as_bytes().to_vec()));
    term
}
