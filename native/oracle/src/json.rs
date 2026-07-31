//! A minimal, strict JSON reader.
//!
//! Hand-rolled because §4.2 gives this crate **no dependencies** — not
//! `serde_json`, not anything. The oracle judges the app; the shortest way to
//! keep it from importing what it judges is to let it import nothing at all.
//!
//! Strictness is the point, not a side effect. This parser is fed
//! machine-generated JSON from two *different* implementations, and every
//! lenient decision a parser can make is a way for those two to disagree
//! silently:
//!
//! - **Duplicate object keys are an error.** Taking the first or the last is a
//!   coin flip that hides an extractor bug.
//! - **Non-finite numbers are an error.** `1e999` parses to `f64::INFINITY` in
//!   Rust and JSON has no `NaN` literal, so both arrive here as malformed
//!   input. A `NaN` that reached the differ would compare unequal to itself and
//!   the resulting delta would be unexplainable.
//! - **Trailing content is an error.** Two concatenated snapshots must not read
//!   as one.
//! - **Nesting is depth-limited.** A pathological input must produce a message,
//!   not a stack overflow — a crash is indistinguishable from the tool being
//!   broken.
//!
//! Every error carries a byte offset, because "malformed JSON" with no position
//! is a bug report nobody can act on.

use std::fmt;

/// How deep object/array nesting may go before the parser refuses.
///
/// The snapshot schema bottoms out at depth 4 (document → `anchors` → an anchor
/// → `font`), so this is roughly an order of magnitude of headroom. It exists
/// solely so that adversarial or corrupted input produces [`ErrorKind::TooDeep`]
/// instead of blowing the stack.
const MAX_DEPTH: usize = 64;

/// A parsed JSON value.
///
/// `Object` keeps insertion order and is a `Vec` rather than a map: snapshot
/// objects have single-digit key counts, so linear lookup beats hashing, and
/// preserving order lets an error name keys in the order the extractor wrote
/// them.
#[derive(Debug, Clone, PartialEq)]
pub enum Value {
    /// `null`.
    Null,
    /// `true` or `false`.
    Bool(bool),
    /// A finite number. Non-finite values are rejected at parse time.
    Number(f64),
    /// A string, with escapes already resolved.
    String(String),
    /// An array.
    Array(Vec<Value>),
    /// An object, in source order, with duplicate keys already rejected.
    Object(Vec<(String, Value)>),
}

impl Value {
    /// The value stored under `key`, or `None` if this is not an object or the
    /// key is absent.
    #[must_use]
    pub fn get(&self, key: &str) -> Option<&Self> {
        match self {
            Self::Object(fields) => fields
                .iter()
                .find_map(|(k, v)| (k.as_str() == key).then_some(v)),
            _ => None,
        }
    }

    /// The items, if this is an array.
    #[must_use]
    pub fn as_array(&self) -> Option<&[Self]> {
        match self {
            Self::Array(items) => Some(items),
            _ => None,
        }
    }

    /// The name of this value's type, for error messages.
    #[must_use]
    pub const fn type_name(&self) -> &'static str {
        match self {
            Self::Null => "null",
            Self::Bool(_) => "boolean",
            Self::Number(_) => "number",
            Self::String(_) => "string",
            Self::Array(_) => "array",
            Self::Object(_) => "object",
        }
    }
}

/// What went wrong, specifically.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum ErrorKind {
    /// The input ended in the middle of a value.
    UnexpectedEof,
    /// A byte that cannot start or continue the construct being parsed.
    Unexpected {
        /// What the parser was willing to accept here.
        expected: &'static str,
        /// What it found instead, rendered printably.
        found: String,
    },
    /// A number that is syntactically JSON but is not a finite `f64`.
    NonFiniteNumber(String),
    /// Digits that do not form JSON's number grammar.
    MalformedNumber(String),
    /// A `\u` escape that is not four hex digits, or an unpaired surrogate.
    BadEscape(String),
    /// The same key twice in one object.
    DuplicateKey(String),
    /// Nesting exceeded the depth limit.
    TooDeep,
    /// A complete value was parsed but bytes followed it.
    TrailingContent,
    /// The input was not valid UTF-8.
    NotUtf8,
}

impl fmt::Display for ErrorKind {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::UnexpectedEof => f.write_str("input ended inside a value"),
            Self::Unexpected { expected, found } => write!(f, "expected {expected}, found {found}"),
            Self::NonFiniteNumber(raw) => write!(
                f,
                "`{raw}` is not a finite number; every comparison against a non-finite \
                 value is meaningless"
            ),
            Self::MalformedNumber(raw) => write!(f, "`{raw}` is not a JSON number"),
            Self::BadEscape(raw) => write!(f, "bad string escape `{raw}`"),
            Self::DuplicateKey(key) => write!(
                f,
                "duplicate object key `{key}`; picking one of the two would hide an \
                 extractor bug"
            ),
            Self::TooDeep => write!(f, "nesting deeper than {MAX_DEPTH} levels"),
            Self::TrailingContent => f.write_str("trailing content after the top-level value"),
            Self::NotUtf8 => f.write_str("input is not valid UTF-8"),
        }
    }
}

/// A parse failure, with the byte offset it happened at.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Error {
    /// What went wrong.
    pub kind: ErrorKind,
    /// Byte offset into the input.
    pub offset: usize,
}

impl fmt::Display for Error {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "at byte {}: {}", self.offset, self.kind)
    }
}

impl std::error::Error for Error {}

/// Parse a complete JSON document.
///
/// # Errors
///
/// [`Error`] describing the first problem found, with its byte offset. Trailing
/// content after the top-level value is a failure, not a stopping point.
pub fn parse(input: &str) -> Result<Value, Error> {
    let mut p = Parser { input, pos: 0 };
    let value = p.value(0)?;
    p.skip_whitespace();
    if p.pos < input.len() {
        return Err(p.err(ErrorKind::TrailingContent));
    }
    Ok(value)
}

/// Parse a complete JSON document from raw bytes.
///
/// # Errors
///
/// [`ErrorKind::NotUtf8`] if the bytes are not UTF-8, otherwise as [`parse`].
pub fn parse_bytes(input: &[u8]) -> Result<Value, Error> {
    let text = std::str::from_utf8(input).map_err(|e| Error {
        kind: ErrorKind::NotUtf8,
        offset: e.valid_up_to(),
    })?;
    parse(text)
}

struct Parser<'a> {
    input: &'a str,
    pos: usize,
}

impl Parser<'_> {
    fn err(&self, kind: ErrorKind) -> Error {
        Error {
            kind,
            offset: self.pos,
        }
    }

    fn bytes(&self) -> &[u8] {
        self.input.as_bytes()
    }

    fn peek(&self) -> Option<u8> {
        self.bytes().get(self.pos).copied()
    }

    /// The character at the cursor. `None` at end of input.
    fn peek_char(&self) -> Option<char> {
        self.input.get(self.pos..).and_then(|s| s.chars().next())
    }

    /// A source slice, clamped so an error path can never index out of range.
    fn slice(&self, from: usize, to: usize) -> String {
        self.input.get(from..to).unwrap_or_default().to_owned()
    }

    fn skip_whitespace(&mut self) {
        while matches!(self.peek(), Some(b' ' | b'\t' | b'\n' | b'\r')) {
            self.pos += 1;
        }
    }

    /// The character at the cursor, rendered for an error message.
    fn found(&self) -> String {
        self.peek_char()
            .map_or_else(|| "end of input".to_owned(), |c| format!("`{c}`"))
    }

    fn unexpected(&self, expected: &'static str) -> Error {
        self.err(ErrorKind::Unexpected {
            expected,
            found: self.found(),
        })
    }

    fn expect(&mut self, byte: u8, expected: &'static str) -> Result<(), Error> {
        if self.peek() == Some(byte) {
            self.pos += 1;
            Ok(())
        } else {
            Err(self.unexpected(expected))
        }
    }

    fn value(&mut self, depth: usize) -> Result<Value, Error> {
        if depth > MAX_DEPTH {
            return Err(self.err(ErrorKind::TooDeep));
        }
        self.skip_whitespace();
        match self.peek() {
            None => Err(self.err(ErrorKind::UnexpectedEof)),
            Some(b'{') => self.object(depth),
            Some(b'[') => self.array(depth),
            Some(b'"') => self.string().map(Value::String),
            Some(b't') => self.literal("true", Value::Bool(true)),
            Some(b'f') => self.literal("false", Value::Bool(false)),
            Some(b'n') => self.literal("null", Value::Null),
            Some(b'-' | b'0'..=b'9') => self.number(),
            Some(_) => Err(self.unexpected("a JSON value")),
        }
    }

    fn literal(&mut self, word: &'static str, value: Value) -> Result<Value, Error> {
        if self
            .input
            .get(self.pos..)
            .is_some_and(|s| s.starts_with(word))
        {
            self.pos += word.len();
            Ok(value)
        } else {
            Err(self.unexpected(word))
        }
    }

    fn object(&mut self, depth: usize) -> Result<Value, Error> {
        self.pos += 1; // the `{` the caller peeked
        let mut fields: Vec<(String, Value)> = Vec::new();
        self.skip_whitespace();
        if self.peek() == Some(b'}') {
            self.pos += 1;
            return Ok(Value::Object(fields));
        }
        loop {
            self.skip_whitespace();
            let key_at = self.pos;
            let key = self.string()?;
            if fields.iter().any(|(k, _)| *k == key) {
                return Err(Error {
                    kind: ErrorKind::DuplicateKey(key),
                    offset: key_at,
                });
            }
            self.skip_whitespace();
            self.expect(b':', "`:` after an object key")?;
            let value = self.value(depth + 1)?;
            fields.push((key, value));
            self.skip_whitespace();
            match self.peek() {
                Some(b',') => self.pos += 1,
                Some(b'}') => {
                    self.pos += 1;
                    return Ok(Value::Object(fields));
                }
                _ => return Err(self.unexpected("`,` or `}`")),
            }
        }
    }

    fn array(&mut self, depth: usize) -> Result<Value, Error> {
        self.pos += 1; // the `[` the caller peeked
        let mut items = Vec::new();
        self.skip_whitespace();
        if self.peek() == Some(b']') {
            self.pos += 1;
            return Ok(Value::Array(items));
        }
        loop {
            items.push(self.value(depth + 1)?);
            self.skip_whitespace();
            match self.peek() {
                Some(b',') => self.pos += 1,
                Some(b']') => {
                    self.pos += 1;
                    return Ok(Value::Array(items));
                }
                _ => return Err(self.unexpected("`,` or `]`")),
            }
        }
    }

    fn string(&mut self) -> Result<String, Error> {
        self.expect(b'"', "a string")?;
        let mut out = String::new();
        loop {
            let Some(ch) = self.peek_char() else {
                return Err(self.err(ErrorKind::UnexpectedEof));
            };
            match ch {
                '"' => {
                    self.pos += 1;
                    return Ok(out);
                }
                '\\' => {
                    self.pos += 1;
                    self.escape(&mut out)?;
                }
                c if (c as u32) < 0x20 => {
                    return Err(self.err(ErrorKind::Unexpected {
                        expected: "a printable character (control bytes must be escaped)",
                        found: format!("U+{:04X}", c as u32),
                    }));
                }
                c => {
                    out.push(c);
                    self.pos += c.len_utf8();
                }
            }
        }
    }

    fn escape(&mut self, out: &mut String) -> Result<(), Error> {
        let Some(byte) = self.peek() else {
            return Err(self.err(ErrorKind::UnexpectedEof));
        };
        let at = self.pos;
        self.pos += 1;
        let ch = match byte {
            b'"' => '"',
            b'\\' => '\\',
            b'/' => '/',
            b'b' => '\u{8}',
            b'f' => '\u{c}',
            b'n' => '\n',
            b'r' => '\r',
            b't' => '\t',
            b'u' => return self.unicode_escape(out),
            _ => {
                return Err(Error {
                    kind: ErrorKind::BadEscape(format!("\\{}", byte as char)),
                    offset: at,
                });
            }
        };
        out.push(ch);
        Ok(())
    }

    fn unicode_escape(&mut self, out: &mut String) -> Result<(), Error> {
        let at = self.pos;
        let first = self.hex4()?;
        // A high surrogate must be followed by a low one. Anything else is an
        // unpaired surrogate, which has no `char` representation — an error,
        // never a silent U+FFFD.
        let scalar = if (0xd800..0xdc00).contains(&first) {
            if !self
                .input
                .get(self.pos..)
                .is_some_and(|s| s.starts_with("\\u"))
            {
                return Err(Error {
                    kind: ErrorKind::BadEscape(format!("unpaired high surrogate U+{first:04X}")),
                    offset: at,
                });
            }
            self.pos += 2;
            let low = self.hex4()?;
            if !(0xdc00..0xe000).contains(&low) {
                return Err(Error {
                    kind: ErrorKind::BadEscape(format!(
                        "U+{first:04X} followed by U+{low:04X}, which is not a low surrogate"
                    )),
                    offset: at,
                });
            }
            0x1_0000 + ((first - 0xd800) << 10) + (low - 0xdc00)
        } else {
            first
        };
        // The lone-low-surrogate case (`\uDC00`..`\uDFFF`) is caught here rather
        // than by a second range check: `char::from_u32` already knows exactly
        // which scalars are unrepresentable, and one gate is easier to trust
        // than two that must agree.
        let Some(ch) = char::from_u32(scalar) else {
            return Err(Error {
                kind: ErrorKind::BadEscape(format!("unpaired low surrogate U+{scalar:04X}")),
                offset: at,
            });
        };
        out.push(ch);
        Ok(())
    }

    fn hex4(&mut self) -> Result<u32, Error> {
        let Some(chunk) = self.bytes().get(self.pos..self.pos + 4) else {
            return Err(self.err(ErrorKind::UnexpectedEof));
        };
        let mut acc: u32 = 0;
        for &b in chunk {
            let digit = match b {
                b'0'..=b'9' => u32::from(b - b'0'),
                b'a'..=b'f' => u32::from(b - b'a') + 10,
                b'A'..=b'F' => u32::from(b - b'A') + 10,
                _ => {
                    return Err(self.err(ErrorKind::BadEscape(format!(
                        "\\u{}",
                        String::from_utf8_lossy(chunk)
                    ))));
                }
            };
            acc = acc * 16 + digit;
        }
        self.pos += 4;
        Ok(acc)
    }

    /// Consume ASCII digits from `cursor`, returning how many.
    fn digits(&self, cursor: &mut usize) -> usize {
        let from = *cursor;
        while self.bytes().get(*cursor).is_some_and(u8::is_ascii_digit) {
            *cursor += 1;
        }
        *cursor - from
    }

    fn malformed(&self, start: usize, cursor: usize) -> Error {
        // One byte past the cursor so the offending character is in the
        // message, clamped by `slice`.
        Error {
            kind: ErrorKind::MalformedNumber(self.slice(start, cursor + 1)),
            offset: start,
        }
    }

    fn number(&mut self) -> Result<Value, Error> {
        let start = self.pos;
        let mut cursor = self.pos;

        if self.bytes().get(cursor) == Some(&b'-') {
            cursor += 1;
        }
        let int_digits = self.digits(&mut cursor);
        // JSON forbids an empty integer part and a leading zero on a
        // multi-digit one.
        if int_digits == 0
            || (int_digits > 1 && self.bytes().get(cursor - int_digits) == Some(&b'0'))
        {
            return Err(self.malformed(start, cursor));
        }
        if self.bytes().get(cursor) == Some(&b'.') {
            cursor += 1;
            if self.digits(&mut cursor) == 0 {
                return Err(self.malformed(start, cursor));
            }
        }
        if matches!(self.bytes().get(cursor), Some(b'e' | b'E')) {
            cursor += 1;
            if matches!(self.bytes().get(cursor), Some(b'+' | b'-')) {
                cursor += 1;
            }
            if self.digits(&mut cursor) == 0 {
                return Err(self.malformed(start, cursor));
            }
        }

        let raw = self.slice(start, cursor);
        // The grammar checked above is a strict subset of Rust's float syntax,
        // so this parse cannot fail. Mapping a hypothetical failure to `NaN`
        // routes it into the finiteness check below rather than adding an
        // unreachable arm — the input is rejected either way, and there is no
        // branch that can never be exercised.
        let parsed = raw.parse::<f64>().unwrap_or(f64::NAN);
        if !parsed.is_finite() {
            return Err(Error {
                kind: ErrorKind::NonFiniteNumber(raw),
                offset: start,
            });
        }
        self.pos = cursor;
        Ok(Value::Number(parsed))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn kind(input: &str) -> ErrorKind {
        parse(input).expect_err("expected a parse error").kind
    }

    #[test]
    fn parses_the_scalar_types() {
        let cases: &[(&str, Value)] = &[
            ("null", Value::Null),
            ("true", Value::Bool(true)),
            ("false", Value::Bool(false)),
            ("0", Value::Number(0.0)),
            ("-0", Value::Number(-0.0)),
            ("12", Value::Number(12.0)),
            ("-4.25", Value::Number(-4.25)),
            ("1e2", Value::Number(100.0)),
            ("1E+2", Value::Number(100.0)),
            ("1e-2", Value::Number(0.01)),
            ("\"\"", Value::String(String::new())),
            ("\"hi\"", Value::String("hi".to_owned())),
        ];
        for (input, want) in cases {
            assert_eq!(parse(input).as_ref(), Ok(want), "input {input}");
        }
    }

    #[test]
    fn parses_containers_and_preserves_key_order() {
        let v = parse(r#"  { "b": [1, 2], "a": {} }  "#).expect("valid");
        // Compared structurally rather than by index, so key *order* is part of
        // the assertion.
        assert_eq!(
            v,
            Value::Object(vec![
                (
                    "b".to_owned(),
                    Value::Array(vec![Value::Number(1.0), Value::Number(2.0)]),
                ),
                ("a".to_owned(), Value::Object(Vec::new())),
            ])
        );
        assert_eq!(v.get("a"), Some(&Value::Object(Vec::new())));
        assert_eq!(v.get("missing"), None);
        assert_eq!(Value::Null.get("a"), None);
        assert_eq!(v.as_array(), None);
        assert_eq!(
            v.get("b").and_then(Value::as_array).map(<[Value]>::len),
            Some(2)
        );
    }

    #[test]
    fn parses_empty_containers_and_nesting() {
        assert_eq!(parse("[]"), Ok(Value::Array(Vec::new())));
        assert_eq!(parse("{}"), Ok(Value::Object(Vec::new())));
        assert_eq!(
            parse("[[[1]]]"),
            Ok(Value::Array(vec![Value::Array(vec![Value::Array(vec![
                Value::Number(1.0)
            ])])]))
        );
    }

    #[test]
    fn resolves_string_escapes() {
        let cases: &[(&str, &str)] = &[
            (r#""a\"b""#, "a\"b"),
            (r#""a\\b""#, "a\\b"),
            (r#""a\/b""#, "a/b"),
            (r#""a\bb""#, "a\u{8}b"),
            (r#""a\fb""#, "a\u{c}b"),
            (r#""a\nb""#, "a\nb"),
            (r#""a\rb""#, "a\rb"),
            (r#""a\tb""#, "a\tb"),
            (r#""A""#, "A"),
            (r#""😀""#, "\u{1f600}"),
            ("\"é☃\u{1f600}\"", "é☃\u{1f600}"),
        ];
        for (input, want) in cases {
            assert_eq!(
                parse(input),
                Ok(Value::String((*want).to_owned())),
                "input {input}"
            );
        }
    }

    #[test]
    fn resolves_unicode_escapes_including_surrogate_pairs() {
        // Built rather than written literally, so the test source contains no
        // `\u` sequence that a tool could rewrite behind the test's back.
        let esc = String::from_utf8(vec![0x5c, 0x75]).expect("ascii");

        for (hex, want) in [
            ("0041", "A"),
            ("0061", "a"),
            ("00e9", "é"),
            ("00E9", "é"),
            ("2603", "☃"),
            ("0000", "\0"),
        ] {
            let input = format!("\"{esc}{hex}\"");
            assert_eq!(
                parse(&input),
                Ok(Value::String(want.to_owned())),
                "input {input}"
            );
        }

        // A surrogate pair is the only way JSON can express an astral scalar.
        for (hi, lo) in [("D83D", "DE00"), ("d83d", "de00")] {
            let input = format!("\"{esc}{hi}{esc}{lo}\"");
            assert_eq!(
                parse(&input),
                Ok(Value::String("\u{1f600}".to_owned())),
                "input {input}"
            );
        }

        // Escapes compose with ordinary text.
        assert_eq!(
            parse(&format!("\"a{esc}0041b\"")),
            Ok(Value::String("aAb".to_owned()))
        );

        // A high surrogate followed by a well-formed but non-low escape.
        let input = format!("\"{esc}D83D{esc}0041\"");
        let e = parse(&input).expect_err("not a low surrogate");
        assert!(
            matches!(&e.kind, ErrorKind::BadEscape(m) if m.contains("not a low surrogate")),
            "{e}"
        );
    }

    #[test]
    fn rejects_non_finite_numbers() {
        assert_eq!(
            kind("1e999"),
            ErrorKind::NonFiniteNumber("1e999".to_owned())
        );
        assert_eq!(
            kind("-1e999"),
            ErrorKind::NonFiniteNumber("-1e999".to_owned())
        );
        // JSON has no NaN literal, so it never reaches the float parser at all.
        assert!(matches!(kind("NaN"), ErrorKind::Unexpected { .. }));
        assert!(matches!(kind("Infinity"), ErrorKind::Unexpected { .. }));
    }

    #[test]
    fn rejects_malformed_numbers() {
        for input in ["01", "-", "1.", "1e", "1e+", "1.e2", "-.5", "00", "-x"] {
            let k = kind(input);
            assert!(
                matches!(k, ErrorKind::MalformedNumber(_)),
                "input {input} should be a malformed number, got {k:?}"
            );
        }
    }

    #[test]
    fn rejects_duplicate_keys() {
        assert_eq!(
            kind(r#"{"a":1,"a":2}"#),
            ErrorKind::DuplicateKey("a".to_owned())
        );
    }

    #[test]
    fn rejects_trailing_content() {
        assert_eq!(kind("{} {}"), ErrorKind::TrailingContent);
        assert_eq!(kind("1 2"), ErrorKind::TrailingContent);
    }

    #[test]
    fn rejects_excessive_nesting() {
        let deep = format!("{}1{}", "[".repeat(200), "]".repeat(200));
        assert_eq!(kind(&deep), ErrorKind::TooDeep);
    }

    #[test]
    fn rejects_truncated_input() {
        for input in [
            "", "  ", "[1,", "{\"a\":", "\"abc", "{\"a\"", "[", "{", "\"a\\",
        ] {
            let k = kind(input);
            assert!(
                matches!(k, ErrorKind::UnexpectedEof | ErrorKind::Unexpected { .. }),
                "input {input:?} gave {k:?}"
            );
        }
    }

    #[test]
    fn rejects_structural_junk() {
        for input in [
            "[1 2]",
            "{\"a\":1 \"b\":2}",
            "{\"a\" 1}",
            "{1:2}",
            "tru",
            "fals",
            "nul",
            "@",
            "é",
        ] {
            let k = kind(input);
            assert!(
                matches!(k, ErrorKind::Unexpected { .. } | ErrorKind::UnexpectedEof),
                "input {input:?} gave {k:?}"
            );
        }
    }

    #[test]
    fn rejects_bad_escapes_and_control_bytes() {
        assert_eq!(kind(r#""a\qb""#), ErrorKind::BadEscape("\\q".to_owned()));
        assert!(matches!(kind(r#""\u00zz""#), ErrorKind::BadEscape(_)));
        assert!(matches!(kind(r#""\uD83D""#), ErrorKind::BadEscape(_)));
        assert!(matches!(kind(r#""\uD83Dx""#), ErrorKind::BadEscape(_)));
        assert!(matches!(kind(r#""\uD83DA""#), ErrorKind::BadEscape(_)));
        assert!(matches!(kind(r#""\uDE00""#), ErrorKind::BadEscape(_)));
        assert!(matches!(kind(r#""\u00"#), ErrorKind::UnexpectedEof));
        assert!(matches!(kind("\"a\u{1}b\""), ErrorKind::Unexpected { .. }));
    }

    #[test]
    fn error_offsets_point_at_the_problem() {
        let e = parse(r#"{"a":1,"a":2}"#).expect_err("duplicate");
        assert_eq!(e.offset, 7);
        assert!(e.to_string().starts_with("at byte 7: duplicate object key"));
    }

    #[test]
    fn parse_bytes_rejects_non_utf8() {
        let e = parse_bytes(&[b'"', 0xff, b'"']).expect_err("invalid utf-8");
        assert_eq!(e.kind, ErrorKind::NotUtf8);
        assert_eq!(parse_bytes(b"[]"), Ok(Value::Array(Vec::new())));
    }

    #[test]
    fn type_names_cover_every_variant() {
        assert_eq!(Value::Null.type_name(), "null");
        assert_eq!(Value::Bool(true).type_name(), "boolean");
        assert_eq!(Value::Number(1.0).type_name(), "number");
        assert_eq!(Value::String(String::new()).type_name(), "string");
        assert_eq!(Value::Array(Vec::new()).type_name(), "array");
        assert_eq!(Value::Object(Vec::new()).type_name(), "object");
    }

    #[test]
    fn every_error_kind_renders() {
        let kinds = [
            ErrorKind::UnexpectedEof,
            ErrorKind::Unexpected {
                expected: "x",
                found: "y".to_owned(),
            },
            ErrorKind::NonFiniteNumber("1e999".to_owned()),
            ErrorKind::MalformedNumber("01".to_owned()),
            ErrorKind::BadEscape("\\q".to_owned()),
            ErrorKind::DuplicateKey("a".to_owned()),
            ErrorKind::TooDeep,
            ErrorKind::TrailingContent,
            ErrorKind::NotUtf8,
        ];
        for k in kinds {
            assert!(!k.to_string().is_empty(), "{k:?} rendered empty");
        }
    }
}
