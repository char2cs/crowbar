//! The v1 snapshot wire shape — `native/oracle/ANCHORS.md` v1.1, §2 and §3.
//!
//! This module is the only place window coordinates turn into root-relative
//! ones (§4) and the only place gpui types turn into JSON. It has no gpui
//! `Window` in it on purpose: the arithmetic that decides whether the two apps
//! agree is arithmetic, and it should be testable as such.
//!
//! Two v1.1 rules shape the types here rather than the code:
//!
//! * **Unknown fields are a hard failure** at document and anchor level, so
//!   every `Serialize` in this module carries exactly the contract's field set
//!   and nothing speculative. Adding a field here without adding it to
//!   `ANCHORS.md` first breaks the differ outright, which is the intent.
//! * **`state` has a fixed vocabulary**, and a mismatched matrix cell is a
//!   *refusal* rather than a delta — one side emitting `"Dark"` against the
//!   other's `"dark"` would make every comparison refuse. So [`Theme`],
//!   [`Content`] and [`Flag`] are enums: the wrong spelling is not
//!   representable, which is stronger than validating it.

use std::collections::BTreeSet;

use crowbar_ui::gpui::{Bounds, Pixels};
use serde::Serialize;

use crate::color;
use crate::record::{Paint, RawAnchor};

/// The schema version this crate emits. `native/oracle/ANCHORS.md` is the
/// contract; bumping this without bumping that file is the bug it warns about.
/// v1.1 clarified the contract without changing the wire shape, so this stays 1.
pub const SCHEMA: u32 = 1;

/// One snapshot of one surface in one state.
#[derive(Clone, Debug, PartialEq, Serialize)]
pub struct Snapshot {
    /// Always [`SCHEMA`].
    pub schema: u32,
    /// The component under test.
    pub surface: String,
    /// The §8.3 matrix cell that produced this snapshot. Data, not decoration:
    /// the differ refuses to compare two snapshots whose `state` differs.
    pub state: SurfaceState,
    /// The anchor all geometry is relative to.
    pub root: String,
    /// Pre-order: the order the anchors were reached during `prepaint`.
    pub anchors: Vec<Anchor>,
}

/// Which theme the snapshot was taken in (v1.1).
#[derive(Clone, Copy, Debug, PartialEq, Eq, PartialOrd, Ord, Serialize)]
#[serde(rename_all = "lowercase")]
pub enum Theme {
    /// `light`.
    Light,
    /// `dark`.
    Dark,
}

/// How much content the surface was given (v1.1).
#[derive(Clone, Copy, Debug, PartialEq, Eq, PartialOrd, Ord, Serialize)]
#[serde(rename_all = "lowercase")]
pub enum Content {
    /// Comfortably shorter than the box.
    Short,
    /// A typical length.
    Normal,
    /// Longer than the box, so something has to truncate.
    Overflow,
}

/// One of the §8.3 state flags (v1.1).
#[derive(Clone, Copy, Debug, PartialEq, Eq, PartialOrd, Ord, Serialize)]
#[serde(rename_all = "lowercase")]
pub enum Flag {
    /// The surface has no content to show.
    Empty,
    /// The surface is loading.
    Loading,
    /// The surface is in an error state.
    Error,
    /// The pointer is over it.
    Hover,
    /// It holds keyboard focus.
    Focus,
    /// It is selected.
    Selected,
}

/// The matrix cell a snapshot was taken in.
///
/// Constructed rather than assembled field by field, because `flags` is a
/// **set** — v1.1 rejects duplicates on load, and sorts, so both are done here
/// and a caller cannot get them wrong.
#[derive(Clone, Debug, PartialEq, Eq, Serialize)]
pub struct SurfaceState {
    /// Surface width in logical pixels.
    width: u32,
    /// Theme.
    theme: Theme,
    /// Content shape.
    content: Content,
    /// State flags: sorted, deduplicated.
    flags: Vec<Flag>,
}

impl SurfaceState {
    /// The matrix cell a surface was rendered in.
    #[must_use]
    pub fn new(
        width: u32,
        theme: Theme,
        content: Content,
        flags: impl IntoIterator<Item = Flag>,
    ) -> Self {
        Self {
            width,
            theme,
            content,
            flags: flags.into_iter().collect::<BTreeSet<_>>().into_iter().collect(),
        }
    }
}

/// One anchor, root-relative and serialised.
#[derive(Clone, Debug, PartialEq, Serialize)]
pub struct Anchor {
    /// Stable semantic name.
    pub id: String,
    /// Border box, logical px, relative to the snapshot's `root`.
    pub bounds: Rect,
    /// Resolved text colour. Present iff the anchor paints text.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub fg: Option<String>,
    /// The element's own background paint. `#00000000` when it paints none.
    ///
    /// **Omitted** — not `null`, not a placeholder — when gpui's fill is one v1
    /// cannot express (see [`Paint::Unrepresentable`]). `bg` is a required
    /// field, so a snapshot containing such an anchor fails to load *by name*.
    /// That is deliberate: a gradient is not transparent, and `#00000000` would
    /// compare as a real delta and point the reader at the wrong thing.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub bg: Option<String>,
    /// Full string content, before truncation. Present iff it paints text.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub text: Option<String>,
    /// Unclipped advance width of `text`.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub text_width: Option<f64>,
    /// Whether `text` is visually truncated in this box.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub clipped: Option<bool>,
    /// Resolved font. Present iff it paints text.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub font: Option<Font>,
    /// Whether the anchor is actually painted.
    pub visible: bool,
    /// Corner radius in logical px.
    pub radius: f64,
    /// Border width and colour.
    pub border: Border,
}

/// A box, logical pixels.
#[derive(Clone, Copy, Debug, PartialEq, Serialize)]
pub struct Rect {
    /// Left edge, relative to the root anchor.
    pub x: f64,
    /// Top edge, relative to the root anchor.
    pub y: f64,
    /// Width.
    pub w: f64,
    /// Height.
    pub h: f64,
}

/// The resolved font of a text-painting anchor.
#[derive(Clone, Debug, PartialEq, Serialize)]
pub struct Font {
    /// Font size in logical px.
    pub size: f64,
    /// Numeric weight, 100–900.
    pub weight: u32,
    /// The resolved first family.
    pub family: String,
    /// Line height in logical px.
    pub line_height: f64,
}

/// A border: width and colour.
#[derive(Clone, Debug, PartialEq, Serialize)]
pub struct Border {
    /// Width in logical px.
    pub w: f64,
    /// `#rrggbbaa`. `#00000000` when gpui would paint no border.
    pub color: String,
}

/// Why a snapshot could not be built.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum SnapshotError {
    /// The declared root anchor was never reached this frame, so there is no
    /// origin to make the other anchors relative to.
    ///
    /// This is a real finding, not a plumbing failure: it means the root was
    /// inside a `display: none` subtree, or the anchor id is misspelt. Both
    /// have to surface loudly, because a snapshot silently anchored to
    /// something else would be uniformly wrong by a constant.
    MissingRoot {
        /// The root that was asked for.
        root: String,
        /// The anchors that were actually recorded, in the order seen.
        recorded: Vec<String>,
    },
}

impl std::fmt::Display for SnapshotError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::MissingRoot { root, recorded } => write!(
                f,
                "the root anchor {root:?} was not recorded this frame; \
                 the anchors that were: {recorded:?}",
            ),
        }
    }
}

impl std::error::Error for SnapshotError {}

impl Snapshot {
    /// Turns a frame's worth of window-space records into a v1 snapshot.
    ///
    /// # Errors
    ///
    /// [`SnapshotError::MissingRoot`] when no recorded anchor carries the
    /// declared root id.
    pub fn build(
        surface: impl Into<String>,
        state: SurfaceState,
        root: &str,
        records: &[RawAnchor],
    ) -> Result<Self, SnapshotError> {
        let origin = records
            .iter()
            .find(|record| record.id == root)
            .map(|record| record.bounds.origin)
            .ok_or_else(|| SnapshotError::MissingRoot {
                root: root.to_owned(),
                recorded: records
                    .iter()
                    .map(|record| record.id.to_string())
                    .collect(),
            })?;

        Ok(Self {
            schema: SCHEMA,
            surface: surface.into(),
            state,
            root: root.to_owned(),
            anchors: records
                .iter()
                .map(|record| anchor_of(record, origin))
                .collect(),
        })
    }

    /// Serialises to the JSON the differ consumes.
    ///
    /// # Errors
    ///
    /// Propagates any `serde_json` failure. There is no data-dependent way to
    /// provoke one here — every field is a string, a bool or a finite number —
    /// but swallowing it would be worse than returning it.
    pub fn to_json(&self) -> Result<String, serde_json::Error> {
        serde_json::to_string_pretty(self)
    }
}

/// Shifts one record into the root's coordinate space and serialises it.
fn anchor_of(record: &RawAnchor, origin: crowbar_ui::gpui::Point<Pixels>) -> Anchor {
    let text = record.text.as_ref();

    Anchor {
        id: record.id.to_string(),
        bounds: rect_of(record.bounds, origin),
        fg: text.map(|text| color::hex(text.color)),
        bg: hex_of(record.background),
        text: text.map(|text| text.content.to_string()),
        text_width: text.map(|text| px_of(text.width)),
        clipped: text.map(|text| text.clipped),
        font: text.map(|text| Font {
            size: px_of(text.font.size),
            weight: weight_of(text.font.weight),
            family: text.font.family.to_string(),
            line_height: px_of(text.font.line_height),
        }),
        visible: record.visible,
        radius: px_of(record.radius),
        border: Border {
            w: px_of(record.border_width),
            color: record
                .border_color
                .map_or_else(|| color::TRANSPARENT.to_owned(), color::hex),
        },
    }
}

/// §4: bounds are relative to the root anchor's top-left, so the root itself
/// lands at exactly `{x: 0, y: 0}` and the two apps' window origins cancel.
fn rect_of(bounds: Bounds<Pixels>, origin: crowbar_ui::gpui::Point<Pixels>) -> Rect {
    Rect {
        x: px_of(bounds.origin.x - origin.x),
        y: px_of(bounds.origin.y - origin.y),
        w: px_of(bounds.size.width),
        h: px_of(bounds.size.height),
    }
}

/// A paint as the contract spells it. `None` — which serialises to *nothing*,
/// not to `null` — only for a fill v1 cannot express; see
/// [`Paint::Unrepresentable`].
fn hex_of(paint: Paint) -> Option<String> {
    match paint {
        Paint::None => Some(color::TRANSPARENT.to_owned()),
        Paint::Solid(color) => Some(color::hex(color)),
        Paint::Unrepresentable => None,
    }
}

/// Logical pixels, rounded to a thousandth.
///
/// gpui measures in `f32`, so the exact `f64` widening of a laid-out coordinate
/// is full of `17.549999237060547`-shaped noise that says nothing — §5's
/// tolerance is ±0.5 px. Rounding at 1/1000 px is three orders of magnitude
/// inside that and makes a snapshot diffable by eye.
fn px_of(value: Pixels) -> f64 {
    (value.to_f64() * 1000.0).round() / 1000.0
}

/// §5 compares `font.weight` exactly, and gpui carries it as an `f32`.
///
/// v1.1 widened the accepted range to CSS's 1–1000 and says **do not clamp**: a
/// legitimate variable-font weight must not fail to load, and a clamp would
/// silently rewrite one into a neighbour it is not. So this only rounds. Rust's
/// float→integer cast saturates rather than wrapping, which keeps a nonsensical
/// input (a negative, an infinity) from becoming a plausible weight.
#[expect(
    clippy::cast_possible_truncation,
    clippy::cast_sign_loss,
    reason = "a float→integer `as` cast saturates in Rust; a weight outside \
              1..=1000 is already a contract violation and lands at 0 or \
              u32::MAX, which is loud rather than plausible"
)]
fn weight_of(weight: f32) -> u32 {
    weight.round() as u32
}

#[cfg(test)]
mod tests {
    use crowbar_ui::gpui::{Hsla, SharedString, bounds, point, px, rgb, size};

    use super::{Content, Flag, SCHEMA, Snapshot, SnapshotError, SurfaceState, Theme};
    use crate::record::{FontFacts, Paint, RawAnchor, TextFacts};

    /// Exact equality on a value `px_of` already rounded to a thousandth of a
    /// pixel, spelled so `clippy::float_cmp` can see it is deliberate rather
    /// than an accident of writing `==` on a float.
    #[track_caller]
    fn assert_px(actual: f64, expected: f64) {
        assert!(
            (actual - expected).abs() < 1e-9,
            "expected {expected}, got {actual}",
        );
    }

    fn a_state() -> SurfaceState {
        SurfaceState::new(320, Theme::Dark, Content::Overflow, [Flag::Selected])
    }

    fn a_box(id: &str, x: f32, y: f32, w: f32, h: f32) -> RawAnchor {
        RawAnchor {
            id: SharedString::from(id.to_owned()),
            bounds: bounds(point(px(x), px(y)), size(px(w), px(h))),
            background: Paint::None,
            text: None,
            visible: true,
            radius: px(0.0),
            border_width: px(0.0),
            border_color: None,
        }
    }

    fn a_text(id: &str, x: f32, y: f32, w: f32, h: f32) -> RawAnchor {
        RawAnchor {
            text: Some(TextFacts {
                content: "resolve-terminal-connection.ts".into(),
                width: px(186.5),
                clipped: true,
                color: Hsla::from(rgb(0x00c8_ccd4)),
                font: FontFacts {
                    size: px(13.0),
                    weight: 500.0,
                    family: "Inter".into(),
                    line_height: px(17.5),
                },
            }),
            ..a_box(id, x, y, w, h)
        }
    }

    #[test]
    fn the_root_anchor_lands_at_the_origin() {
        let records = [a_box("root", 120.0, 64.0, 320.0, 22.0)];

        let snapshot = Snapshot::build("git-status-row", a_state(), "root", &records)
            .expect("the root is present");

        assert_px(snapshot.anchors[0].bounds.x, 0.0);
        assert_px(snapshot.anchors[0].bounds.y, 0.0);
        assert_px(snapshot.anchors[0].bounds.w, 320.0);
        assert_px(snapshot.anchors[0].bounds.h, 22.0);
    }

    /// The whole reason §4 exists: the two apps do not share a window origin,
    /// so a constant offset on every anchor has to cancel exactly.
    #[test]
    fn a_constant_window_offset_cancels_on_every_anchor() {
        let near = [
            a_box("root", 0.0, 0.0, 320.0, 22.0),
            a_box("icon", 12.0, 3.0, 16.0, 16.0),
        ];
        let far = [
            a_box("root", 1000.0, 700.0, 320.0, 22.0),
            a_box("icon", 1012.0, 703.0, 16.0, 16.0),
        ];

        let from_near =
            Snapshot::build("row", a_state(), "root", &near).expect("the root is present");
        let from_far =
            Snapshot::build("row", a_state(), "root", &far).expect("the root is present");

        assert_eq!(from_near, from_far);
    }

    #[test]
    fn an_anchor_above_or_left_of_the_root_gets_a_negative_coordinate() {
        let records = [
            a_box("root", 100.0, 100.0, 320.0, 22.0),
            a_box("badge", 92.0, 96.0, 8.0, 8.0),
        ];

        let snapshot =
            Snapshot::build("row", a_state(), "root", &records).expect("the root is present");

        assert_px(snapshot.anchors[1].bounds.x, -8.0);
        assert_px(snapshot.anchors[1].bounds.y, -4.0);
    }

    #[test]
    fn a_missing_root_is_an_error_and_names_what_was_seen() {
        let records = [a_box("icon", 12.0, 3.0, 16.0, 16.0)];

        let error = Snapshot::build("row", a_state(), "root", &records)
            .expect_err("the root was never recorded");

        assert_eq!(
            error,
            SnapshotError::MissingRoot {
                root: "root".to_owned(),
                recorded: vec!["icon".to_owned()],
            },
        );
        assert!(error.to_string().contains("\"root\""));
        assert!(error.to_string().contains("icon"));
    }

    #[test]
    fn an_empty_frame_is_a_missing_root_rather_than_an_empty_snapshot() {
        let error =
            Snapshot::build("row", a_state(), "root", &[]).expect_err("nothing was recorded");

        assert_eq!(
            error,
            SnapshotError::MissingRoot {
                root: "root".to_owned(),
                recorded: Vec::new(),
            },
        );
    }

    #[test]
    fn a_box_anchor_omits_every_text_field_and_still_carries_bg() {
        let records = [a_box("root", 0.0, 0.0, 320.0, 22.0)];

        let snapshot =
            Snapshot::build("row", a_state(), "root", &records).expect("the root is present");
        let anchor = &snapshot.anchors[0];

        assert_eq!(anchor.fg, None);
        assert_eq!(anchor.text, None);
        assert_eq!(anchor.text_width, None);
        assert_eq!(anchor.clipped, None);
        assert_eq!(anchor.font, None);
        assert_eq!(anchor.bg.as_deref(), Some("#00000000"));
    }

    #[test]
    fn a_text_anchor_carries_the_whole_group() {
        let records = [
            a_box("root", 0.0, 0.0, 320.0, 22.0),
            a_text("row-name", 30.0, 4.0, 118.0, 16.0),
        ];

        let snapshot =
            Snapshot::build("row", a_state(), "root", &records).expect("the root is present");
        let anchor = &snapshot.anchors[1];
        let font = anchor.font.as_ref().expect("a text anchor has a font");

        assert_eq!(anchor.fg.as_deref(), Some("#c8ccd4ff"));
        assert_eq!(anchor.text.as_deref(), Some("resolve-terminal-connection.ts"));
        assert_eq!(anchor.text_width, Some(186.5));
        assert_eq!(anchor.clipped, Some(true));
        assert_px(font.size, 13.0);
        assert_eq!(font.weight, 500);
        assert_eq!(font.family, "Inter");
        assert_px(font.line_height, 17.5);
    }

    /// Not `#00000000` and not `null`: a gradient is not transparent, and v1
    /// has no way to say "we cannot express this". Omitting the required field
    /// makes the differ reject the document *by name*, which is loud and
    /// diagnosable; a placeholder would compare as a real delta and point the
    /// reader at the wrong thing.
    #[test]
    fn an_unrepresentable_fill_omits_bg_rather_than_inventing_one() {
        let records = [RawAnchor {
            background: Paint::Unrepresentable,
            ..a_box("root", 0.0, 0.0, 10.0, 10.0)
        }];

        let snapshot =
            Snapshot::build("row", a_state(), "root", &records).expect("the root is present");
        let json = snapshot.to_json().expect("the snapshot serialises");

        assert_eq!(snapshot.anchors[0].bg, None);
        assert!(!json.contains("\"bg\""));
        assert!(!json.contains("null"));
    }

    #[test]
    fn a_solid_fill_serialises_as_hex() {
        let records = [RawAnchor {
            background: Paint::Solid(Hsla::from(rgb(0x0020_2734))),
            border_width: px(1.0),
            border_color: Some(Hsla::from(rgb(0x003a_3f4b))),
            radius: px(4.0),
            ..a_box("root", 0.0, 0.0, 10.0, 10.0)
        }];

        let snapshot =
            Snapshot::build("row", a_state(), "root", &records).expect("the root is present");
        let anchor = &snapshot.anchors[0];

        assert_eq!(anchor.bg.as_deref(), Some("#202734ff"));
        assert_eq!(anchor.border.color, "#3a3f4bff");
        assert_px(anchor.border.w, 1.0);
        assert_px(anchor.radius, 4.0);
    }

    #[test]
    fn a_colourless_border_serialises_as_transparent() {
        let records = [RawAnchor {
            border_width: px(2.0),
            ..a_box("root", 0.0, 0.0, 10.0, 10.0)
        }];

        let snapshot =
            Snapshot::build("row", a_state(), "root", &records).expect("the root is present");

        assert_eq!(snapshot.anchors[0].border.color, "#00000000");
        assert_px(snapshot.anchors[0].border.w, 2.0);
    }

    /// `f32` widened to `f64` is full of noise three orders of magnitude inside
    /// §5's tolerance. Rounding it away is what makes a snapshot readable.
    #[test]
    fn float_noise_is_rounded_out_of_the_wire_shape() {
        let records = [
            a_box("root", 0.0, 0.0, 320.0, 22.0),
            a_box("noisy", 0.1, 0.2, 17.549_9, 0.3),
        ];

        let snapshot =
            Snapshot::build("row", a_state(), "root", &records).expect("the root is present");

        assert_px(snapshot.anchors[1].bounds.w, 17.55);
        assert!(!snapshot.to_json().expect("serialises").contains("17.5499"));
    }

    #[test]
    fn the_wire_shape_matches_the_contract() {
        let records = [
            RawAnchor {
                background: Paint::Solid(Hsla::from(rgb(0x0020_2734))),
                radius: px(2.0),
                ..a_box("git-row-item", 100.0, 200.0, 320.0, 24.0)
            },
            a_text("git-row-name", 130.0, 204.0, 118.0, 16.0),
        ];

        let snapshot = Snapshot::build("git-status-row", a_state(), "git-row-item", &records)
            .expect("the root is present");

        assert_eq!(snapshot.schema, SCHEMA);
        assert_eq!(
            snapshot.to_json().expect("serialises"),
            r##"{
  "schema": 1,
  "surface": "git-status-row",
  "state": {
    "width": 320,
    "theme": "dark",
    "content": "overflow",
    "flags": [
      "selected"
    ]
  },
  "root": "git-row-item",
  "anchors": [
    {
      "id": "git-row-item",
      "bounds": {
        "x": 0.0,
        "y": 0.0,
        "w": 320.0,
        "h": 24.0
      },
      "bg": "#202734ff",
      "visible": true,
      "radius": 2.0,
      "border": {
        "w": 0.0,
        "color": "#00000000"
      }
    },
    {
      "id": "git-row-name",
      "bounds": {
        "x": 30.0,
        "y": 4.0,
        "w": 118.0,
        "h": 16.0
      },
      "fg": "#c8ccd4ff",
      "bg": "#00000000",
      "text": "resolve-terminal-connection.ts",
      "text_width": 186.5,
      "clipped": true,
      "font": {
        "size": 13.0,
        "weight": 500,
        "family": "Inter",
        "line_height": 17.5
      },
      "visible": true,
      "radius": 0.0,
      "border": {
        "w": 0.0,
        "color": "#00000000"
      }
    }
  ]
}"##,
        );
    }

    #[test]
    fn anchors_keep_the_order_they_were_recorded_in() {
        let records = [
            a_box("root", 0.0, 0.0, 320.0, 22.0),
            a_box("second", 1.0, 0.0, 1.0, 1.0),
            a_box("third", 2.0, 0.0, 1.0, 1.0),
        ];

        let snapshot =
            Snapshot::build("row", a_state(), "root", &records).expect("the root is present");

        let ids: Vec<&str> = snapshot
            .anchors
            .iter()
            .map(|anchor| anchor.id.as_str())
            .collect();
        assert_eq!(ids, ["root", "second", "third"]);
    }

    /// The root need not be first: `prepaint` order is pre-order, but a caller
    /// is free to nominate any recorded anchor as the origin.
    #[test]
    fn the_root_does_not_have_to_be_the_first_record() {
        let records = [
            a_box("wrapper", 0.0, 0.0, 400.0, 40.0),
            a_box("root", 10.0, 10.0, 320.0, 22.0),
        ];

        let snapshot =
            Snapshot::build("row", a_state(), "root", &records).expect("the root is present");

        assert_px(snapshot.anchors[0].bounds.x, -10.0);
        assert_px(snapshot.anchors[1].bounds.x, 0.0);
    }

    fn weight_in_snapshot(weight: f32) -> u32 {
        let records = [RawAnchor {
            text: Some(TextFacts {
                content: "x".into(),
                width: px(1.0),
                clipped: false,
                color: Hsla::from(rgb(0)),
                font: FontFacts {
                    size: px(13.0),
                    weight,
                    family: "Inter".into(),
                    line_height: px(17.5),
                },
            }),
            ..a_box("root", 0.0, 0.0, 10.0, 10.0)
        }];

        let snapshot =
            Snapshot::build("row", a_state(), "root", &records).expect("the root is present");
        snapshot.anchors[0]
            .font
            .as_ref()
            .expect("a text anchor has a font")
            .weight
    }

    /// v1.1 widened the accepted range to CSS's 1–1000 and said **do not
    /// clamp**: a legitimate variable-font weight must not fail to load, and a
    /// clamp would silently rewrite one into a neighbour it is not.
    #[test]
    fn a_variable_font_weight_outside_the_hundreds_is_carried_through() {
        assert_eq!(weight_in_snapshot(1.0), 1);
        assert_eq!(weight_in_snapshot(437.0), 437);
        assert_eq!(weight_in_snapshot(1000.0), 1000);
    }

    /// A nonsensical weight saturates rather than wrapping, so it lands
    /// somewhere obviously wrong instead of somewhere plausible.
    #[test]
    fn a_nonsensical_weight_saturates_rather_than_wrapping() {
        assert_eq!(weight_in_snapshot(-100.0), 0);
        assert_eq!(weight_in_snapshot(f32::INFINITY), u32::MAX);
    }

    /// v1.1: `flags` is a set. Sorting and deduplicating on the way out means a
    /// caller cannot produce a snapshot the differ would reject.
    #[test]
    fn state_flags_are_sorted_and_deduplicated() {
        let state = SurfaceState::new(
            320,
            Theme::Light,
            Content::Short,
            [Flag::Selected, Flag::Hover, Flag::Selected],
        );

        let json = serde_json::to_string(&state).expect("serialises");

        assert_eq!(
            json,
            r#"{"width":320,"theme":"light","content":"short","flags":["hover","selected"]}"#,
        );
    }

    /// The whole vocabulary, spelled out, because one side emitting `"Dark"`
    /// against the other's `"dark"` makes *every* comparison refuse.
    #[test]
    fn the_state_vocabulary_is_lowercase_and_has_no_synonyms() {
        let state = SurfaceState::new(
            1024,
            Theme::Dark,
            Content::Normal,
            [
                Flag::Empty,
                Flag::Loading,
                Flag::Error,
                Flag::Hover,
                Flag::Focus,
                Flag::Selected,
            ],
        );

        let json = serde_json::to_string(&state).expect("serialises");

        assert_eq!(
            json,
            concat!(
                r#"{"width":1024,"theme":"dark","content":"normal","flags":"#,
                r#"["empty","loading","error","hover","focus","selected"]}"#,
            ),
        );
    }

    /// v1.1 rule 4: an extractor that forgot §4 would emit window coordinates,
    /// and the root sitting anywhere but the origin is what catches it.
    #[test]
    fn the_roots_own_bounds_are_exactly_the_origin_whatever_the_window_did() {
        for (x, y) in [(0.0, 0.0), (17.5, 3.25), (-40.0, 1000.0)] {
            let records = [a_box("root", x, y, 320.0, 22.0)];

            let snapshot =
                Snapshot::build("row", a_state(), "root", &records).expect("the root is present");

            assert_px(snapshot.anchors[0].bounds.x, 0.0);
            assert_px(snapshot.anchors[0].bounds.y, 0.0);
            assert_eq!(snapshot.root, "root");
            assert!(snapshot.anchors.iter().any(|anchor| anchor.id == "root"));
        }
    }

    /// v1.1 rule 2: always eight hex digits. Six would make "opaque" and "the
    /// extractor forgot alpha" indistinguishable.
    #[test]
    fn every_colour_in_a_snapshot_is_eight_hex_digits() {
        let records = [
            RawAnchor {
                background: Paint::Solid(Hsla::from(rgb(0x0020_2734))),
                border_color: Some(Hsla::from(rgb(0x003a_3f4b))),
                ..a_box("root", 0.0, 0.0, 10.0, 10.0)
            },
            a_text("row-name", 0.0, 0.0, 10.0, 10.0),
        ];

        let snapshot =
            Snapshot::build("row", a_state(), "root", &records).expect("the root is present");

        for anchor in &snapshot.anchors {
            for colour in [anchor.bg.as_deref(), anchor.fg.as_deref()]
                .into_iter()
                .flatten()
                .chain([anchor.border.color.as_str()])
            {
                assert_eq!(colour.len(), 9, "{colour} is not #rrggbbaa");
                assert!(colour.starts_with('#'), "{colour} is not #rrggbbaa");
            }
        }
    }
}
