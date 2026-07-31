//! The v1 snapshot model (ANCHORS.md §2–§4) and its loader.
//!
//! The loader is deliberately unforgiving. Its whole job is to turn JSON
//! written by two *independent* extractors into a value the differ can trust,
//! and every place it could shrug instead of failing is a place those two could
//! disagree without the gate noticing:
//!
//! - a field of the wrong type is an error, never a coerced default;
//! - a duplicate anchor `id` is an error, because "which of the two did we
//!   compare?" has no good answer;
//! - `root` must name an anchor that exists, and per §4 that anchor must sit at
//!   the origin — an extractor that forgot to rebase its coordinates would
//!   otherwise offset *every* anchor by a constant and drown the real deltas,
//!   which is exactly what §4 exists to prevent;
//! - unknown keys, at the document level or inside an anchor, are an error.
//!   ANCHORS.md's opening rule is that a field one extractor emits and the
//!   other does not is "worse than useless — it looks like coverage and is
//!   not". Ignoring it silently is how that stays invisible; a new field is
//!   supposed to reach the contract first, with a version bump.
//!
//! No `f64` in this module is ever converted with a lossy `as` cast. The two
//! integer-valued fields (`schema`, `state.width`, `font.weight`) go through
//! [`u32_from_whole`], which is exact or returns `None`.

use std::fmt;

use crate::color::{self, Color};
use crate::json::{self, Value};
use crate::tolerance::within;

/// The only schema version this build understands.
pub const SCHEMA_V1: u32 = 1;

/// A rectangle in logical px, relative to the snapshot's `root` anchor (§4).
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Bounds {
    /// Left edge, relative to `root`. May be negative.
    pub x: f64,
    /// Top edge, relative to `root`. May be negative.
    pub y: f64,
    /// Border-box width. Never negative.
    pub w: f64,
    /// Border-box height. Never negative.
    pub h: f64,
}

/// Resolved text style (§3).
#[derive(Debug, Clone, PartialEq)]
pub struct Font {
    /// Size in logical px.
    pub size: f64,
    /// Numeric weight, 100–900.
    pub weight: u32,
    /// Resolved *first* family, not the whole stack.
    pub family: String,
    /// Line height in logical px.
    pub line_height: f64,
}

/// A border: width plus colour (§3).
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Border {
    /// Width in logical px.
    pub w: f64,
    /// Border colour.
    pub color: Color,
}

/// One tagged semantic anchor (§3).
///
/// The `Option` fields are the ones ANCHORS.md marks "if it paints text" or
/// "no". They stay `Option` rather than being defaulted, so the differ can say
/// "one side emitted this and the other did not" — a contract defect, and a
/// different fact from a value mismatch.
#[derive(Debug, Clone, PartialEq)]
pub struct Anchor {
    /// Stable semantic name; identical on both sides.
    pub id: String,
    /// Border box, relative to `root`.
    pub bounds: Bounds,
    /// Resolved text colour.
    pub fg: Option<Color>,
    /// The element's own background paint, not composited with ancestors.
    pub bg: Color,
    /// Full string content, before visual truncation.
    pub text: Option<String>,
    /// Rendered advance width of `text`, unclipped.
    pub text_width: Option<f64>,
    /// Whether the text is visually truncated in this box.
    pub clipped: Option<bool>,
    /// Resolved text style.
    pub font: Option<Font>,
    /// Actually painted.
    pub visible: bool,
    /// Corner radius; top-left if the corners differ.
    pub radius: Option<f64>,
    /// Border width and colour.
    pub border: Option<Border>,
}

/// The §8.3 matrix cell a snapshot was taken in.
///
/// Compared as a whole, and a difference in any field is a refusal rather than
/// a delta (§2). `flags` is normalised to a sorted set on the way in: two
/// extractors listing `["selected","hover"]` and `["hover","selected"]` are in
/// the same cell, and refusing on that would be a false alarm that trains
/// people to ignore the refusal.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct State {
    /// Viewport width in logical px. Whole pixels only.
    pub width: u32,
    /// `"light"` / `"dark"`. Compared exactly; never interpreted.
    pub theme: String,
    /// Content-length bucket — short / long / overflowing. Compared exactly.
    pub content: String,
    /// Interaction flags, sorted, no duplicates.
    pub flags: Vec<String>,
}

impl fmt::Display for State {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(
            f,
            "width={} theme={} content={} flags=[{}]",
            self.width,
            self.theme,
            self.content,
            self.flags.join(", ")
        )
    }
}

/// A complete v1 snapshot (§2).
#[derive(Debug, Clone, PartialEq)]
pub struct Snapshot {
    /// Schema version. Always [`SCHEMA_V1`] for a snapshot this build accepts.
    pub schema: u32,
    /// The component under test.
    pub surface: String,
    /// The matrix cell.
    pub state: State,
    /// The anchor every `bounds` is relative to.
    pub root: String,
    /// The anchors, in the order the extractor emitted them.
    pub anchors: Vec<Anchor>,
}

impl Snapshot {
    /// The anchor with this id, if present.
    #[must_use]
    pub fn anchor(&self, id: &str) -> Option<&Anchor> {
        self.anchors.iter().find(|a| a.id == id)
    }

    /// Parse a snapshot from JSON text.
    ///
    /// # Errors
    ///
    /// [`LoadError`], carrying a dotted path to the offending field.
    pub fn from_json(text: &str) -> Result<Self, LoadError> {
        let value = json::parse(text).map_err(|e| LoadError {
            path: String::new(),
            kind: LoadErrorKind::Json(e),
        })?;
        Self::from_value(&value)
    }

    /// Parse a snapshot from already-parsed JSON.
    ///
    /// # Errors
    ///
    /// [`LoadError`], carrying a dotted path to the offending field.
    pub fn from_value(value: &Value) -> Result<Self, LoadError> {
        let ctx = Path::root();
        known_keys(
            value,
            &ctx,
            &["schema", "surface", "state", "root", "anchors"],
        )?;

        let schema = whole_at(
            value,
            &ctx,
            "schema",
            0.0..=f64::from(u32::MAX),
            "must be a non-negative whole number",
        )?;
        if schema != SCHEMA_V1 {
            return Err(LoadError {
                path: ctx.child("schema").0,
                kind: LoadErrorKind::UnsupportedSchema(schema),
            });
        }
        let surface = non_empty_string_at(value, &ctx, "surface")?;
        let state = state_at(value, &ctx)?;
        let root = non_empty_string_at(value, &ctx, "root")?;

        let anchors_path = ctx.child("anchors");
        let anchors_value = required(value, &ctx, "anchors")?;
        let Some(items) = anchors_value.as_array() else {
            return Err(type_error(&anchors_path, "array", anchors_value));
        };
        let mut anchors: Vec<Anchor> = Vec::with_capacity(items.len());
        for (i, item) in items.iter().enumerate() {
            let path = anchors_path.index(i);
            let anchor = anchor_at(item, &path)?;
            if anchors.iter().any(|a| a.id == anchor.id) {
                return Err(LoadError {
                    path: path.child("id").0,
                    kind: LoadErrorKind::DuplicateAnchorId(anchor.id),
                });
            }
            anchors.push(anchor);
        }

        let snapshot = Self {
            schema,
            surface,
            state,
            root,
            anchors,
        };
        snapshot.validate_root(&ctx)?;
        Ok(snapshot)
    }

    /// §4: `root` must name an anchor, and that anchor sits at the origin.
    ///
    /// Skipped for a snapshot with no anchors — there is nothing for `root` to
    /// be relative to, and a surface with nothing tagged yet is a legitimate if
    /// useless snapshot. "Useless" is prevented from masquerading as
    /// "converged" one layer up, where the report carries the compared-anchor
    /// count and the CLI refuses an empty comparison by default.
    fn validate_root(&self, ctx: &Path) -> Result<(), LoadError> {
        if self.anchors.is_empty() {
            return Ok(());
        }
        let Some(root) = self.anchor(&self.root) else {
            return Err(LoadError {
                path: ctx.child("root").0,
                kind: LoadErrorKind::RootNotAnAnchor(self.root.clone()),
            });
        };
        // A zero tolerance, so `-0.0` passes and `0.0000001` does not.
        if !within(root.bounds.x, 0.0, 0.0) || !within(root.bounds.y, 0.0, 0.0) {
            return Err(LoadError {
                path: ctx.child("root").0,
                kind: LoadErrorKind::RootNotAtOrigin {
                    id: self.root.clone(),
                    x: root.bounds.x,
                    y: root.bounds.y,
                },
            });
        }
        Ok(())
    }
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

/// A dotted path to the field a load failed on, e.g. `anchors[3].font.weight`.
#[derive(Debug, Clone, PartialEq, Eq)]
struct Path(String);

impl Path {
    fn root() -> Self {
        Self(String::new())
    }

    fn child(&self, key: &str) -> Self {
        if self.0.is_empty() {
            Self(key.to_owned())
        } else {
            Self(format!("{}.{key}", self.0))
        }
    }

    fn index(&self, i: usize) -> Self {
        Self(format!("{}[{i}]", self.0))
    }
}

/// Why a snapshot would not load.
#[derive(Debug, Clone, PartialEq)]
pub enum LoadErrorKind {
    /// The document was not JSON.
    Json(json::Error),
    /// A required field was absent.
    Missing,
    /// A field had the wrong JSON type.
    WrongType {
        /// What the schema requires.
        expected: &'static str,
        /// What the document had.
        found: &'static str,
    },
    /// A string field was present but empty.
    Empty,
    /// A number that had to be whole was not.
    NotAnInteger(f64),
    /// A number outside the range the schema allows.
    OutOfRange {
        /// The offending value.
        value: f64,
        /// The rule it broke, in prose.
        rule: &'static str,
    },
    /// A colour string that is not `#rrggbbaa`.
    Color(color::ParseError),
    /// A key the v1 schema does not define.
    UnknownKey {
        /// The offending key.
        key: String,
        /// The keys the schema does define here.
        known: String,
    },
    /// `schema` named a version this build does not implement.
    UnsupportedSchema(u32),
    /// Two anchors shared an `id`.
    DuplicateAnchorId(String),
    /// `root` did not name any anchor in the snapshot.
    RootNotAnAnchor(String),
    /// The root anchor was not at `{x: 0, y: 0}` (§4).
    RootNotAtOrigin {
        /// The root's id.
        id: String,
        /// Its x.
        x: f64,
        /// Its y.
        y: f64,
    },
    /// The same flag twice in `state.flags`.
    DuplicateFlag(String),
}

impl fmt::Display for LoadErrorKind {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Json(e) => write!(f, "not valid JSON: {e}"),
            Self::Missing => f.write_str("required by ANCHORS.md §2–§3, but absent"),
            Self::WrongType { expected, found } => {
                write!(f, "must be {expected}, found {found}")
            }
            Self::Empty => f.write_str("must not be empty"),
            Self::NotAnInteger(v) => write!(f, "must be a whole number, found {v:?}"),
            Self::OutOfRange { value, rule } => write!(f, "{value:?} is out of range: {rule}"),
            Self::Color(e) => write!(f, "{e}"),
            Self::UnknownKey { key, known } => write!(
                f,
                "`{key}` is not a v1 field (known: {known}). ANCHORS.md requires a new field to \
                 reach the contract first, with a version bump, because a field one extractor \
                 emits and the other does not is invisible to the differ"
            ),
            Self::UnsupportedSchema(v) => write!(
                f,
                "schema {v} is not supported; this build implements v{SCHEMA_V1}"
            ),
            Self::DuplicateAnchorId(id) => write!(
                f,
                "anchor id `{id}` appears twice; the differ matches by id and would have no way \
                 to say which of the two it compared"
            ),
            Self::RootNotAnAnchor(id) => write!(
                f,
                "`{id}` is not one of the anchors; §4 makes every `bounds` relative to the root, \
                 so a root that is not in the snapshot leaves the coordinate space undefined"
            ),
            Self::RootNotAtOrigin { id, x, y } => write!(
                f,
                "root anchor `{id}` is at ({x:?}, {y:?}), not the origin; §4 requires every \
                 bounds to be relative to it. An extractor emitting window coordinates would \
                 offset every anchor by a constant and drown the real deltas"
            ),
            Self::DuplicateFlag(flag) => write!(f, "flag `{flag}` is listed twice"),
        }
    }
}

/// A snapshot that would not load, and where.
#[derive(Debug, Clone, PartialEq)]
pub struct LoadError {
    /// Dotted path to the field; empty for a document-level failure.
    pub path: String,
    /// What was wrong with it.
    pub kind: LoadErrorKind,
}

impl fmt::Display for LoadError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        if self.path.is_empty() {
            write!(f, "{}", self.kind)
        } else {
            write!(f, "`{}`: {}", self.path, self.kind)
        }
    }
}

impl std::error::Error for LoadError {}

// ---------------------------------------------------------------------------
// Field readers
// ---------------------------------------------------------------------------

/// Exact `f64` → `u32`, or `None` if the value is not a whole number in range.
///
/// Bisection rather than `as`: a float-to-int `as` cast saturates silently on
/// overflow and truncates silently on a fractional value, which is precisely
/// the class of quiet wrongness this crate exists to catch. `u32 → f64` is
/// lossless, so every comparison below is exact.
fn u32_from_whole(value: f64) -> Option<u32> {
    // `-0.0 + 0.0` is `+0.0`, which keeps a negative zero from failing the
    // final bit-exact check.
    let value = value + 0.0;
    if !(0.0..=f64::from(u32::MAX)).contains(&value) {
        return None;
    }
    let (mut lo, mut hi) = (0_u32, u32::MAX);
    while lo < hi {
        let mid = lo + (hi - lo) / 2;
        if f64::from(mid) < value {
            lo = mid + 1;
        } else {
            hi = mid;
        }
    }
    (f64::from(lo).to_bits() == value.to_bits()).then_some(lo)
}

fn type_error(path: &Path, expected: &'static str, found: &Value) -> LoadError {
    LoadError {
        path: path.0.clone(),
        kind: LoadErrorKind::WrongType {
            expected,
            found: found.type_name(),
        },
    }
}

fn known_keys(value: &Value, path: &Path, known: &[&str]) -> Result<(), LoadError> {
    let Value::Object(fields) = value else {
        return Err(type_error(path, "an object", value));
    };
    for (key, _) in fields {
        if !known.contains(&key.as_str()) {
            return Err(LoadError {
                path: path.child(key).0,
                kind: LoadErrorKind::UnknownKey {
                    key: key.clone(),
                    known: known.join(", "),
                },
            });
        }
    }
    Ok(())
}

fn required<'a>(value: &'a Value, path: &Path, key: &str) -> Result<&'a Value, LoadError> {
    value.get(key).ok_or_else(|| LoadError {
        path: path.child(key).0,
        kind: LoadErrorKind::Missing,
    })
}

fn f64_at(parent: &Value, path: &Path, key: &str) -> Result<f64, LoadError> {
    match required(parent, path, key)? {
        Value::Number(n) => Ok(*n),
        other => Err(type_error(&path.child(key), "a number", other)),
    }
}

fn non_negative_at(
    parent: &Value,
    path: &Path,
    key: &str,
    rule: &'static str,
) -> Result<f64, LoadError> {
    let value = f64_at(parent, path, key)?;
    if value < 0.0 {
        return Err(LoadError {
            path: path.child(key).0,
            kind: LoadErrorKind::OutOfRange { value, rule },
        });
    }
    Ok(value)
}

fn whole_at(
    parent: &Value,
    path: &Path,
    key: &str,
    range: std::ops::RangeInclusive<f64>,
    rule: &'static str,
) -> Result<u32, LoadError> {
    let value = f64_at(parent, path, key)?;
    if !range.contains(&value) {
        return Err(LoadError {
            path: path.child(key).0,
            kind: LoadErrorKind::OutOfRange { value, rule },
        });
    }
    u32_from_whole(value).ok_or_else(|| LoadError {
        path: path.child(key).0,
        kind: LoadErrorKind::NotAnInteger(value),
    })
}

fn string_of(value: &Value, path: &Path) -> Result<String, LoadError> {
    match value {
        Value::String(s) => Ok(s.clone()),
        other => Err(type_error(path, "a string", other)),
    }
}

fn string_at(parent: &Value, path: &Path, key: &str) -> Result<String, LoadError> {
    string_of(required(parent, path, key)?, &path.child(key))
}

fn non_empty_string_at(parent: &Value, path: &Path, key: &str) -> Result<String, LoadError> {
    let s = string_at(parent, path, key)?;
    if s.is_empty() {
        return Err(LoadError {
            path: path.child(key).0,
            kind: LoadErrorKind::Empty,
        });
    }
    Ok(s)
}

fn bool_at(parent: &Value, path: &Path, key: &str) -> Result<bool, LoadError> {
    match required(parent, path, key)? {
        Value::Bool(b) => Ok(*b),
        other => Err(type_error(&path.child(key), "a boolean", other)),
    }
}

fn color_at(parent: &Value, path: &Path, key: &str) -> Result<Color, LoadError> {
    let raw = string_at(parent, path, key)?;
    Color::parse(&raw).map_err(|e| LoadError {
        path: path.child(key).0,
        kind: LoadErrorKind::Color(e),
    })
}

// ---------------------------------------------------------------------------
// Composite readers
// ---------------------------------------------------------------------------

fn state_at(parent: &Value, path: &Path) -> Result<State, LoadError> {
    let p = path.child("state");
    let value = required(parent, path, "state")?;
    known_keys(value, &p, &["width", "theme", "content", "flags"])?;

    let width = whole_at(
        value,
        &p,
        "width",
        0.0..=f64::from(u32::MAX),
        "a viewport width is a non-negative whole number of logical pixels",
    )?;
    let theme = non_empty_string_at(value, &p, "theme")?;
    let content = non_empty_string_at(value, &p, "content")?;

    let flags_path = p.child("flags");
    let flags_value = required(value, &p, "flags")?;
    let Some(items) = flags_value.as_array() else {
        return Err(type_error(&flags_path, "an array", flags_value));
    };
    let mut flags: Vec<String> = Vec::with_capacity(items.len());
    for (i, item) in items.iter().enumerate() {
        let flag = string_of(item, &flags_path.index(i))?;
        if flags.contains(&flag) {
            return Err(LoadError {
                path: flags_path.index(i).0,
                kind: LoadErrorKind::DuplicateFlag(flag),
            });
        }
        flags.push(flag);
    }
    // Sorted, so `["selected","hover"]` and `["hover","selected"]` are the same
    // matrix cell. A set, not a sequence.
    flags.sort();

    Ok(State {
        width,
        theme,
        content,
        flags,
    })
}

fn bounds_at(parent: &Value, path: &Path) -> Result<Bounds, LoadError> {
    let p = path.child("bounds");
    let value = required(parent, path, "bounds")?;
    known_keys(value, &p, &["x", "y", "w", "h"])?;
    Ok(Bounds {
        // x and y may be negative: an anchor can sit above or left of the root.
        x: f64_at(value, &p, "x")?,
        y: f64_at(value, &p, "y")?,
        w: non_negative_at(value, &p, "w", "a border box cannot have negative width")?,
        h: non_negative_at(value, &p, "h", "a border box cannot have negative height")?,
    })
}

fn font_at(parent: &Value, path: &Path) -> Result<Option<Font>, LoadError> {
    let p = path.child("font");
    let Some(value) = parent.get("font") else {
        return Ok(None);
    };
    known_keys(value, &p, &["size", "weight", "family", "line_height"])?;
    Ok(Some(Font {
        size: non_negative_at(value, &p, "size", "a font size cannot be negative")?,
        weight: whole_at(
            value,
            &p,
            "weight",
            100.0..=900.0,
            "ANCHORS.md §3 defines `weight` as numeric 100-900",
        )?,
        family: non_empty_string_at(value, &p, "family")?,
        line_height: non_negative_at(value, &p, "line_height", "a line height cannot be negative")?,
    }))
}

fn border_at(parent: &Value, path: &Path) -> Result<Option<Border>, LoadError> {
    let p = path.child("border");
    let Some(value) = parent.get("border") else {
        return Ok(None);
    };
    known_keys(value, &p, &["w", "color"])?;
    Ok(Some(Border {
        w: non_negative_at(value, &p, "w", "a border width cannot be negative")?,
        color: color_at(value, &p, "color")?,
    }))
}

fn anchor_at(value: &Value, path: &Path) -> Result<Anchor, LoadError> {
    known_keys(
        value,
        path,
        &[
            "id",
            "bounds",
            "fg",
            "bg",
            "text",
            "text_width",
            "clipped",
            "font",
            "visible",
            "radius",
            "border",
        ],
    )?;

    Ok(Anchor {
        id: non_empty_string_at(value, path, "id")?,
        bounds: bounds_at(value, path)?,
        fg: if value.get("fg").is_none() {
            None
        } else {
            Some(color_at(value, path, "fg")?)
        },
        bg: color_at(value, path, "bg")?,
        text: if value.get("text").is_none() {
            None
        } else {
            // An empty string is legitimate: an element can paint no glyphs and
            // still be the anchor that proves it.
            Some(string_at(value, path, "text")?)
        },
        text_width: if value.get("text_width").is_none() {
            None
        } else {
            Some(non_negative_at(
                value,
                path,
                "text_width",
                "an advance width cannot be negative",
            )?)
        },
        clipped: if value.get("clipped").is_none() {
            None
        } else {
            Some(bool_at(value, path, "clipped")?)
        },
        font: font_at(value, path)?,
        visible: bool_at(value, path, "visible")?,
        radius: if value.get("radius").is_none() {
            None
        } else {
            Some(non_negative_at(
                value,
                path,
                "radius",
                "a corner radius cannot be negative",
            )?)
        },
        border: border_at(value, path)?,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    /// A minimal well-formed snapshot: one anchor, which is also the root.
    const MINIMAL: &str = r##"{
      "schema": 1,
      "surface": "git-status-row",
      "state": { "width": 320, "theme": "dark", "content": "overflow",
                 "flags": ["selected", "hover"] },
      "root": "git-row-item",
      "anchors": [
        { "id": "git-row-item",
          "bounds": { "x": 0.0, "y": 0.0, "w": 320.0, "h": 24.0 },
          "bg": "#00000000",
          "visible": true }
      ]
    }"##;

    fn err_of(text: &str) -> LoadError {
        Snapshot::from_json(text).expect_err("expected a load error")
    }

    /// `MINIMAL` with the single anchor's JSON replaced.
    fn with_anchor(anchor_json: &str) -> String {
        format!(
            r#"{{"schema":1,"surface":"s","state":{{"width":320,"theme":"dark",
               "content":"short","flags":[]}},"root":"a","anchors":[{anchor_json}]}}"#
        )
    }

    #[test]
    fn loads_the_anchors_md_example() {
        let snap = Snapshot::from_json(MINIMAL).expect("valid");
        assert_eq!(snap.schema, SCHEMA_V1);
        assert_eq!(snap.surface, "git-status-row");
        assert_eq!(snap.root, "git-row-item");
        assert_eq!(snap.state.width, 320);
        assert_eq!(snap.state.theme, "dark");
        assert_eq!(snap.state.content, "overflow");
        // Normalised to a sorted set.
        assert_eq!(snap.state.flags, vec!["hover", "selected"]);
        assert_eq!(snap.anchors.len(), 1);
        let a = snap.anchor("git-row-item").expect("present");
        assert_eq!(a.bg, Color::parse("#00000000").expect("valid"));
        assert!(a.visible);
        assert_eq!(a.fg, None);
        assert_eq!(a.text, None);
        assert_eq!(a.text_width, None);
        assert_eq!(a.clipped, None);
        assert_eq!(a.font, None);
        assert_eq!(a.radius, None);
        assert_eq!(a.border, None);
        assert_eq!(snap.anchor("nope"), None);
    }

    #[test]
    fn loads_a_fully_populated_anchor() {
        let json = with_anchor(
            r##"{ "id": "a",
                 "bounds": { "x": 0, "y": 0, "w": 118.0, "h": 16.0 },
                 "fg": "#c8ccd4ff", "bg": "#00000000",
                 "text": "resolve-terminal-connection.ts",
                 "text_width": 186.5, "clipped": true,
                 "font": { "size": 13.0, "weight": 500, "family": "Inter",
                           "line_height": 17.55 },
                 "visible": true, "radius": 2.0,
                 "border": { "w": 1.0, "color": "#00000000" } }"##,
        );
        let snap = Snapshot::from_json(&json).expect("valid");
        let a = snap.anchor("a").expect("present");
        assert_eq!(a.fg, Some(Color::parse("#c8ccd4ff").expect("valid")));
        assert_eq!(a.text.as_deref(), Some("resolve-terminal-connection.ts"));
        assert_eq!(a.clipped, Some(true));
        let font = a.font.as_ref().expect("font");
        assert_eq!(font.weight, 500);
        assert_eq!(font.family, "Inter");
        assert_eq!(a.border.map(|b| b.color), Color::parse("#00000000").ok());
        // An empty text string is allowed.
        let empty = with_anchor(
            r##"{ "id":"a","bounds":{"x":0,"y":0,"w":1,"h":1},"bg":"#00000000",
                 "text":"","visible":true }"##,
        );
        let snap = Snapshot::from_json(&empty).expect("valid");
        assert_eq!(snap.anchor("a").and_then(|a| a.text.as_deref()), Some(""));
    }

    #[test]
    fn an_empty_anchor_list_loads_and_skips_the_root_check() {
        let json = r#"{"schema":1,"surface":"s","state":{"width":1,"theme":"dark",
                       "content":"short","flags":[]},"root":"absent","anchors":[]}"#;
        let snap = Snapshot::from_json(json).expect("valid");
        assert!(snap.anchors.is_empty());
    }

    #[test]
    fn rejects_a_bad_document_shape() {
        assert!(matches!(err_of("not json").kind, LoadErrorKind::Json(_)));
        assert!(matches!(
            err_of("[]").kind,
            LoadErrorKind::WrongType {
                expected: "an object",
                ..
            }
        ));
        let e = err_of(
            r#"{"schema":1,"surface":"s","state":{"width":1,"theme":"d",
                           "content":"c","flags":[]},"root":"r","anchors":[],"extra":1}"#,
        );
        assert_eq!(e.path, "extra");
        assert!(matches!(e.kind, LoadErrorKind::UnknownKey { .. }));
    }

    #[test]
    fn rejects_wrong_schema_versions() {
        let e = err_of(&MINIMAL.replace("\"schema\": 1", "\"schema\": 2"));
        assert_eq!(e.kind, LoadErrorKind::UnsupportedSchema(2));
        let e = err_of(&MINIMAL.replace("\"schema\": 1", "\"schema\": 1.5"));
        assert!(matches!(e.kind, LoadErrorKind::NotAnInteger(_)));
        let e = err_of(&MINIMAL.replace("\"schema\": 1", "\"schema\": -1"));
        assert!(matches!(e.kind, LoadErrorKind::OutOfRange { .. }));
        let e = err_of(&MINIMAL.replace("\"schema\": 1", "\"schema\": \"1\""));
        assert!(matches!(e.kind, LoadErrorKind::WrongType { .. }));
        let e = err_of(&MINIMAL.replace("\"schema\": 1,", ""));
        assert_eq!(e.kind, LoadErrorKind::Missing);
        assert_eq!(e.path, "schema");
    }

    #[test]
    fn rejects_a_malformed_state() {
        let cases: &[(&str, &str)] = &[
            (r#""width": 320"#, r#""width": 320.5"#),
            (r#""width": 320"#, r#""width": "320""#),
            (r#""theme": "dark""#, r#""theme": """#),
            (r#""theme": "dark""#, r#""theme": 1"#),
            (r#""content": "overflow""#, r#""content": null"#),
            (r#""flags": ["selected", "hover"]"#, r#""flags": "hover""#),
            (r#""flags": ["selected", "hover"]"#, r#""flags": [1]"#),
            (
                r#""flags": ["selected", "hover"]"#,
                r#""flags": ["hover", "hover"]"#,
            ),
            (
                r#""flags": ["selected", "hover"]"#,
                r#""flags": ["hover"], "zoom": 2"#,
            ),
        ];
        for (from, to) in cases {
            let json = MINIMAL.replace(from, to);
            assert!(
                Snapshot::from_json(&json).is_err(),
                "should have rejected {to}"
            );
        }
        let e =
            err_of(&MINIMAL.replace(r#""flags": ["selected", "hover"]"#, r#""flags": ["a","a"]"#));
        assert_eq!(e.kind, LoadErrorKind::DuplicateFlag("a".to_owned()));
        assert_eq!(e.path, "state.flags[1]");
    }

    #[test]
    fn rejects_a_missing_or_mistyped_state() {
        let json = r#"{"schema":1,"surface":"s","root":"r","anchors":[]}"#;
        let e = err_of(json);
        assert_eq!(e.path, "state");
        assert_eq!(e.kind, LoadErrorKind::Missing);
        let json = r#"{"schema":1,"surface":"s","state":7,"root":"r","anchors":[]}"#;
        assert!(matches!(err_of(json).kind, LoadErrorKind::WrongType { .. }));
    }

    #[test]
    fn rejects_a_malformed_anchors_list() {
        let json = r#"{"schema":1,"surface":"s","state":{"width":1,"theme":"d",
                       "content":"c","flags":[]},"root":"r","anchors":{}}"#;
        let e = err_of(json);
        assert_eq!(e.path, "anchors");
        assert!(matches!(e.kind, LoadErrorKind::WrongType { .. }));
    }

    #[test]
    fn rejects_duplicate_anchor_ids() {
        let json = r##"{"schema":1,"surface":"s","state":{"width":1,"theme":"d",
          "content":"c","flags":[]},"root":"a","anchors":[
          {"id":"a","bounds":{"x":0,"y":0,"w":1,"h":1},"bg":"#00000000","visible":true},
          {"id":"a","bounds":{"x":0,"y":0,"w":1,"h":1},"bg":"#00000000","visible":true}]}"##;
        let e = err_of(json);
        assert_eq!(e.kind, LoadErrorKind::DuplicateAnchorId("a".to_owned()));
        assert_eq!(e.path, "anchors[1].id");
    }

    #[test]
    fn enforces_the_section_4_root_rules() {
        let json = with_anchor(
            r##"{"id":"other","bounds":{"x":0,"y":0,"w":1,"h":1},"bg":"#00000000","visible":true}"##,
        );
        let e = err_of(&json);
        assert_eq!(e.kind, LoadErrorKind::RootNotAnAnchor("a".to_owned()));
        assert_eq!(e.path, "root");

        let json = with_anchor(
            r##"{"id":"a","bounds":{"x":12,"y":84,"w":1,"h":1},"bg":"#00000000","visible":true}"##,
        );
        let e = err_of(&json);
        assert!(matches!(e.kind, LoadErrorKind::RootNotAtOrigin { .. }));

        let json = with_anchor(
            r##"{"id":"a","bounds":{"x":0,"y":3,"w":1,"h":1},"bg":"#00000000","visible":true}"##,
        );
        assert!(matches!(
            err_of(&json).kind,
            LoadErrorKind::RootNotAtOrigin { .. }
        ));

        // Signed zero is still the origin.
        let json = with_anchor(
            r##"{"id":"a","bounds":{"x":-0,"y":-0,"w":1,"h":1},"bg":"#00000000","visible":true}"##,
        );
        assert!(Snapshot::from_json(&json).is_ok());
    }

    #[test]
    fn rejects_malformed_anchor_fields() {
        let cases: &[&str] = &[
            // id
            r##"{"bounds":{"x":0,"y":0,"w":1,"h":1},"bg":"#00000000","visible":true}"##,
            r##"{"id":"","bounds":{"x":0,"y":0,"w":1,"h":1},"bg":"#00000000","visible":true}"##,
            r##"{"id":7,"bounds":{"x":0,"y":0,"w":1,"h":1},"bg":"#00000000","visible":true}"##,
            // bounds
            r##"{"id":"a","bg":"#00000000","visible":true}"##,
            r##"{"id":"a","bounds":[0,0,1,1],"bg":"#00000000","visible":true}"##,
            r##"{"id":"a","bounds":{"x":0,"y":0,"w":1},"bg":"#00000000","visible":true}"##,
            r##"{"id":"a","bounds":{"x":0,"y":0,"w":-1,"h":1},"bg":"#00000000","visible":true}"##,
            r##"{"id":"a","bounds":{"x":0,"y":0,"w":1,"h":-1},"bg":"#00000000","visible":true}"##,
            r##"{"id":"a","bounds":{"x":"0","y":0,"w":1,"h":1},"bg":"#00000000","visible":true}"##,
            r##"{"id":"a","bounds":{"x":0,"y":0,"w":1,"h":1,"z":3},"bg":"#00000000","visible":true}"##,
            // colours
            r#"{"id":"a","bounds":{"x":0,"y":0,"w":1,"h":1},"visible":true}"#,
            r##"{"id":"a","bounds":{"x":0,"y":0,"w":1,"h":1},"bg":"#000000","visible":true}"##,
            r##"{"id":"a","bounds":{"x":0,"y":0,"w":1,"h":1},"bg":"#00000000","fg":"red","visible":true}"##,
            r#"{"id":"a","bounds":{"x":0,"y":0,"w":1,"h":1},"bg":123,"visible":true}"#,
            // visible
            r##"{"id":"a","bounds":{"x":0,"y":0,"w":1,"h":1},"bg":"#00000000"}"##,
            r##"{"id":"a","bounds":{"x":0,"y":0,"w":1,"h":1},"bg":"#00000000","visible":"yes"}"##,
            // text group
            r##"{"id":"a","bounds":{"x":0,"y":0,"w":1,"h":1},"bg":"#00000000","visible":true,"text":5}"##,
            r##"{"id":"a","bounds":{"x":0,"y":0,"w":1,"h":1},"bg":"#00000000","visible":true,"text_width":-1}"##,
            r##"{"id":"a","bounds":{"x":0,"y":0,"w":1,"h":1},"bg":"#00000000","visible":true,"clipped":1}"##,
            // font
            r##"{"id":"a","bounds":{"x":0,"y":0,"w":1,"h":1},"bg":"#00000000","visible":true,"font":{}}"##,
            r##"{"id":"a","bounds":{"x":0,"y":0,"w":1,"h":1},"bg":"#00000000","visible":true,
                "font":{"size":-1,"weight":400,"family":"Inter","line_height":1}}"##,
            r##"{"id":"a","bounds":{"x":0,"y":0,"w":1,"h":1},"bg":"#00000000","visible":true,
                "font":{"size":13,"weight":50,"family":"Inter","line_height":1}}"##,
            r##"{"id":"a","bounds":{"x":0,"y":0,"w":1,"h":1},"bg":"#00000000","visible":true,
                "font":{"size":13,"weight":1000,"family":"Inter","line_height":1}}"##,
            r##"{"id":"a","bounds":{"x":0,"y":0,"w":1,"h":1},"bg":"#00000000","visible":true,
                "font":{"size":13,"weight":450.5,"family":"Inter","line_height":1}}"##,
            r##"{"id":"a","bounds":{"x":0,"y":0,"w":1,"h":1},"bg":"#00000000","visible":true,
                "font":{"size":13,"weight":400,"family":"","line_height":1}}"##,
            r##"{"id":"a","bounds":{"x":0,"y":0,"w":1,"h":1},"bg":"#00000000","visible":true,
                "font":{"size":13,"weight":400,"family":"Inter","line_height":-1}}"##,
            r##"{"id":"a","bounds":{"x":0,"y":0,"w":1,"h":1},"bg":"#00000000","visible":true,
                "font":{"size":13,"weight":400,"family":"Inter","line_height":1,"style":"italic"}}"##,
            // radius and border
            r##"{"id":"a","bounds":{"x":0,"y":0,"w":1,"h":1},"bg":"#00000000","visible":true,"radius":-2}"##,
            r##"{"id":"a","bounds":{"x":0,"y":0,"w":1,"h":1},"bg":"#00000000","visible":true,"border":{"w":1}}"##,
            r##"{"id":"a","bounds":{"x":0,"y":0,"w":1,"h":1},"bg":"#00000000","visible":true,
                "border":{"w":-1,"color":"#00000000"}}"##,
            r##"{"id":"a","bounds":{"x":0,"y":0,"w":1,"h":1},"bg":"#00000000","visible":true,
                "border":{"w":1,"color":"#00000000","style":"solid"}}"##,
            // unknown anchor key
            r##"{"id":"a","bounds":{"x":0,"y":0,"w":1,"h":1},"bg":"#00000000","visible":true,"z":3}"##,
            // not an object at all
            r"42",
        ];
        for anchor in cases {
            let json = with_anchor(anchor);
            assert!(
                Snapshot::from_json(&json).is_err(),
                "should have rejected the anchor {anchor}"
            );
        }
    }

    #[test]
    fn u32_from_whole_is_exact_or_none() {
        assert_eq!(u32_from_whole(0.0), Some(0));
        assert_eq!(u32_from_whole(-0.0), Some(0));
        assert_eq!(u32_from_whole(1.0), Some(1));
        assert_eq!(u32_from_whole(320.0), Some(320));
        assert_eq!(u32_from_whole(f64::from(u32::MAX)), Some(u32::MAX));
        assert_eq!(u32_from_whole(0.5), None);
        assert_eq!(u32_from_whole(319.999), None);
        assert_eq!(u32_from_whole(-1.0), None);
        assert_eq!(u32_from_whole(f64::from(u32::MAX) + 1.0), None);
    }

    #[test]
    fn paths_render_the_way_a_reader_would_write_them() {
        let root = Path::root();
        assert_eq!(root.0, "");
        assert_eq!(
            root.child("anchors").index(3).child("font").child("size").0,
            "anchors[3].font.size"
        );
        let e = LoadError {
            path: String::new(),
            kind: LoadErrorKind::Empty,
        };
        assert_eq!(e.to_string(), "must not be empty");
        let e = LoadError {
            path: "surface".to_owned(),
            kind: LoadErrorKind::Empty,
        };
        assert_eq!(e.to_string(), "`surface`: must not be empty");
    }

    #[test]
    fn every_load_error_kind_renders() {
        let kinds = [
            LoadErrorKind::Json(json::Error {
                kind: json::ErrorKind::TooDeep,
                offset: 0,
            }),
            LoadErrorKind::Missing,
            LoadErrorKind::WrongType {
                expected: "a number",
                found: "string",
            },
            LoadErrorKind::Empty,
            LoadErrorKind::NotAnInteger(1.5),
            LoadErrorKind::OutOfRange {
                value: -1.0,
                rule: "r",
            },
            LoadErrorKind::Color(color::ParseError::MissingHash("x".to_owned())),
            LoadErrorKind::UnknownKey {
                key: "z".to_owned(),
                known: "a, b".to_owned(),
            },
            LoadErrorKind::UnsupportedSchema(2),
            LoadErrorKind::DuplicateAnchorId("a".to_owned()),
            LoadErrorKind::RootNotAnAnchor("a".to_owned()),
            LoadErrorKind::RootNotAtOrigin {
                id: "a".to_owned(),
                x: 1.0,
                y: 2.0,
            },
            LoadErrorKind::DuplicateFlag("f".to_owned()),
        ];
        for k in kinds {
            assert!(!k.to_string().is_empty(), "{k:?} rendered empty");
        }
    }

    #[test]
    fn state_renders_its_matrix_cell() {
        let snap = Snapshot::from_json(MINIMAL).expect("valid");
        assert_eq!(
            snap.state.to_string(),
            "width=320 theme=dark content=overflow flags=[hover, selected]"
        );
    }
}
