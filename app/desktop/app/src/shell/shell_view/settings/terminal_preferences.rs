use settings::LEGACY_TERMINAL_LINE_HEIGHT;
use settings::Value;
use terminal::CursorShape;

use crate::panes::content::single_precision::single_precision;

pub(crate) const KEYS: [&str; 8] = [
    "terminalFontFamily",
    "terminalFontSize",
    "terminalLineHeight",
    "terminalLetterSpacing",
    "terminalScrollback",
    "terminalCursorStyle",
    "terminalCursorBlink",
    "terminalCursorWidth",
];

const FONT_SIZE: (f64, f64) = (8.0, 32.0);
const LINE_HEIGHT: (f64, f64) = (1.0, 2.0);
const LETTER_SPACING: (f64, f64) = (-5.0, 5.0);
const SCROLLBACK: (f64, f64) = (1000.0, 100_000.0);
const CURSOR_WIDTH: (f64, f64) = (1.0, 6.0);

pub(crate) fn terminal_settings(read: impl Fn(&str) -> Option<Value>) -> terminalview::Settings {
    let base = terminalview::Settings::default();
    let number = |key: &str, (low, high): (f64, f64)| {
        read(key)
            .and_then(|value| value.as_number())
            .map(|value| value.clamp(low, high))
    };
    terminalview::Settings {
        font_family: family(read("terminalFontFamily").as_ref()).unwrap_or(base.font_family),
        font_size: narrowed(number("terminalFontSize", FONT_SIZE), base.font_size),
        line_height: narrowed(
            number("terminalLineHeight", LINE_HEIGHT)
                .map(|stored| LEGACY_TERMINAL_LINE_HEIGHT.loaded(stored)),
            base.line_height,
        ),
        letter_spacing: narrowed(
            number("terminalLetterSpacing", LETTER_SPACING),
            base.letter_spacing,
        ),
        scrollback: rows(number("terminalScrollback", SCROLLBACK), base.scrollback),
        cursor_shape: shape(read("terminalCursorStyle").as_ref()).unwrap_or(base.cursor_shape),
        cursor_blink: read("terminalCursorBlink")
            .and_then(|value| value.as_bool())
            .unwrap_or(base.cursor_blink),
        cursor_width: narrowed(
            number("terminalCursorWidth", CURSOR_WIDTH),
            base.cursor_width,
        ),
    }
}

fn family(stored: Option<&Value>) -> Option<String> {
    let name = stored.and_then(Value::as_text)?.trim();
    (!name.is_empty()).then(|| ui::font::registered_family(name).to_owned())
}

fn shape(stored: Option<&Value>) -> Option<CursorShape> {
    match stored.and_then(Value::as_text)? {
        "block" => Some(CursorShape::Block),
        "underline" => Some(CursorShape::Underline),
        "bar" => Some(CursorShape::Bar),
        _ => None,
    }
}

fn narrowed(
    stored: Option<f64>,
    default: f32,
) -> f32 {
    stored.map_or(default, single_precision)
}

fn rows(
    stored: Option<f64>,
    default: usize,
) -> usize {
    stored.map_or(default, |value| {
        terminal::floor_cells(single_precision(value))
    })
}

#[cfg(test)]
mod tests {
    use std::collections::BTreeMap;

    use super::*;

    type Stored = BTreeMap<&'static str, Value>;

    fn read(stored: &Stored) -> impl Fn(&str) -> Option<Value> + '_ {
        |key| stored.get(key).cloned()
    }

    #[test]
    fn nothing_stored_keeps_every_default() {
        let settings = terminal_settings(|_| None);
        assert_eq!(settings, terminalview::Settings::default());
    }

    #[test]
    fn every_stored_setting_reaches_the_terminal() {
        let stored: Stored = BTreeMap::from([
            ("terminalFontFamily", Value::Text("Fira Code".into())),
            ("terminalFontSize", Value::Number(18.0)),
            ("terminalLineHeight", Value::Number(1.4)),
            ("terminalLetterSpacing", Value::Number(0.5)),
            ("terminalScrollback", Value::Number(5000.0)),
            ("terminalCursorStyle", Value::Text("bar".into())),
            ("terminalCursorBlink", Value::Bool(false)),
            ("terminalCursorWidth", Value::Number(3.0)),
        ]);
        let settings = terminal_settings(read(&stored));
        assert_eq!(settings.font_family, "Fira Code");
        assert!((settings.font_size - 18.0).abs() < f32::EPSILON);
        assert!((settings.line_height - 1.4).abs() < 1e-6);
        assert!((settings.letter_spacing - 0.5).abs() < f32::EPSILON);
        assert_eq!(settings.scrollback, 5000);
        assert_eq!(settings.cursor_shape, CursorShape::Bar);
        assert!(!settings.cursor_blink);
        assert!((settings.cursor_width - 3.0).abs() < f32::EPSILON);
    }

    #[test]
    fn the_stored_web_mono_name_draws_with_the_bundled_face() {
        let web = Value::Text("Geist Mono Variable".into());
        let stored: Stored = BTreeMap::from([("terminalFontFamily", web)]);
        let settings = terminal_settings(read(&stored));
        assert_eq!(settings.font_family, ui::font::MONO_FAMILY);
    }

    #[test]
    fn the_retired_line_height_default_reads_as_todays() {
        let stored: Stored = BTreeMap::from([("terminalLineHeight", Value::Number(1.2))]);
        let settings = terminal_settings(read(&stored));
        assert!((settings.line_height - 1.0).abs() < f32::EPSILON);
    }

    type Case = (&'static str, &'static str, Value, terminalview::Settings);

    fn clamped(edit: impl FnOnce(&mut terminalview::Settings)) -> terminalview::Settings {
        let mut settings = terminalview::Settings::default();
        edit(&mut settings);
        settings
    }

    #[test]
    fn out_of_range_or_unknown_values_are_held_to_what_the_page_allows() {
        let cases: [Case; 5] = [
            (
                "tiny",
                "terminalFontSize",
                Value::Number(2.0),
                clamped(|s| s.font_size = 8.0),
            ),
            (
                "huge",
                "terminalFontSize",
                Value::Number(99.0),
                clamped(|s| s.font_size = 32.0),
            ),
            (
                "long",
                "terminalScrollback",
                Value::Number(1e9),
                clamped(|s| s.scrollback = 100_000),
            ),
            (
                "odd shape",
                "terminalCursorStyle",
                Value::Text("wavy".into()),
                terminalview::Settings::default(),
            ),
            (
                "blank font",
                "terminalFontFamily",
                Value::Text("  ".into()),
                terminalview::Settings::default(),
            ),
        ];
        for (name, key, value, expected) in cases {
            let stored: Stored = BTreeMap::from([(key, value)]);
            assert_eq!(terminal_settings(read(&stored)), expected, "{name}");
        }
    }
}
