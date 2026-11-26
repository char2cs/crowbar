use editor::WhitespaceMode;
use editorview::WordWrap;
use settings::Value;

use crate::panes::content::single_precision::single_precision;

pub(crate) const KEYS: [&str; 7] = [
    "fontSize",
    "editorLineHeight",
    "tabSize",
    "wordWrap",
    "lineNumbers",
    "renderIndentGuides",
    "renderWhitespace",
];

const FONT_SIZE: (f64, f64) = (8.0, 32.0);
const LINE_HEIGHT: (f64, f64) = (1.0, 2.0);
const LINE_HEIGHT_STEPS: f64 = 10.0;
const TAB_SIZES: std::ops::RangeInclusive<u8> = 1..=8;

pub(crate) fn editor_settings(read: impl Fn(&str) -> Option<Value>) -> editorview::Settings {
    let base = editorview::Settings::default();
    let number = |key: &str| read(key).and_then(|value| value.as_number());
    let flag = |key: &str| read(key).and_then(|value| value.as_bool());
    editorview::Settings {
        font_size: number("fontSize")
            .map(|size| size.clamp(FONT_SIZE.0, FONT_SIZE.1))
            .map_or(base.font_size, single_precision),
        line_height: number("editorLineHeight")
            .and_then(line_height)
            .map_or(base.line_height, single_precision),
        tab_size: number("tabSize")
            .and_then(tab_size)
            .unwrap_or(base.tab_size),
        word_wrap: match flag("wordWrap") {
            Some(true) => WordWrap::On,
            Some(false) => WordWrap::Off,
            None => base.word_wrap,
        },
        line_numbers: flag("lineNumbers").unwrap_or(base.line_numbers),
        indent_guides: flag("renderIndentGuides").unwrap_or(base.indent_guides),
        whitespace: whitespace(read("renderWhitespace").as_ref()).unwrap_or(base.whitespace),
    }
}

fn line_height(stored: f64) -> Option<f64> {
    let snapped = (stored * LINE_HEIGHT_STEPS).round() / LINE_HEIGHT_STEPS;
    stored
        .is_finite()
        .then(|| snapped.clamp(LINE_HEIGHT.0, LINE_HEIGHT.1))
}

fn tab_size(stored: f64) -> Option<usize> {
    let rounded = stored.round();
    TAB_SIZES
        .clone()
        .find(|size| f64::from(*size) >= rounded)
        .or_else(|| rounded.is_finite().then_some(*TAB_SIZES.end()))
        .map(usize::from)
}

fn whitespace(stored: Option<&Value>) -> Option<WhitespaceMode> {
    match stored.and_then(Value::as_text)? {
        "none" => Some(WhitespaceMode::None),
        "boundary" => Some(WhitespaceMode::Boundary),
        "trailing" => Some(WhitespaceMode::Trailing),
        "all" => Some(WhitespaceMode::All),
        _ => None,
    }
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
    fn nothing_stored_keeps_develop_s_defaults() {
        assert_eq!(editor_settings(|_| None), editorview::Settings::default());
    }

    #[test]
    fn every_stored_setting_reaches_the_editor() {
        let stored: Stored = BTreeMap::from([
            ("fontSize", Value::Number(18.0)),
            ("editorLineHeight", Value::Number(1.8)),
            ("tabSize", Value::Number(4.0)),
            ("wordWrap", Value::Bool(true)),
            ("lineNumbers", Value::Bool(false)),
            ("renderIndentGuides", Value::Bool(false)),
            ("renderWhitespace", Value::Text("boundary".into())),
        ]);
        let expected = editorview::Settings {
            font_size: 18.0,
            line_height: 1.8,
            tab_size: 4,
            word_wrap: WordWrap::On,
            line_numbers: false,
            indent_guides: false,
            whitespace: WhitespaceMode::Boundary,
        };
        assert_eq!(editor_settings(read(&stored)), expected);
    }

    fn with(edit: impl FnOnce(&mut editorview::Settings)) -> editorview::Settings {
        let mut settings = editorview::Settings::default();
        edit(&mut settings);
        settings
    }

    #[test]
    fn out_of_range_or_unknown_values_are_held_to_what_develop_allows() {
        let default = editorview::Settings::default();
        let cases: [(&str, Value, editorview::Settings); 6] = [
            (
                "fontSize",
                Value::Number(99.0),
                with(|s| s.font_size = 32.0),
            ),
            (
                "editorLineHeight",
                Value::Number(14.0),
                with(|s| s.line_height = 2.0),
            ),
            ("editorLineHeight", Value::Number(1.43), default),
            ("tabSize", Value::Number(0.0), with(|s| s.tab_size = 1)),
            ("tabSize", Value::Number(12.0), with(|s| s.tab_size = 8)),
            ("renderWhitespace", Value::Text("selection".into()), default),
        ];
        for (key, value, expected) in cases {
            let stored: Stored = BTreeMap::from([(key, value.clone())]);
            assert_eq!(editor_settings(read(&stored)), expected, "{key} {value:?}");
        }
    }
}
