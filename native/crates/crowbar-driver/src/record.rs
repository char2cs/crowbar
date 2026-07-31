//! What an anchor looks like the instant it is observed — window coordinates,
//! gpui types, nothing serialised yet.
//!
//! Everything here is a pure function of values that are already known at
//! `prepaint`. Keeping it free of `Window` is what makes the interesting parts
//! — visibility, clipping, which paint is expressible — testable without a
//! window, and it is why `src/element.rs` is little more than "gather the
//! arguments and call these".

use crowbar_ui::gpui::{
    Bounds, Display, Fill, Hsla, Pixels, SharedString, Style, TextStyle, Visibility, px,
};

/// Half a logical pixel: `native/oracle/ANCHORS.md` §5's bounds tolerance,
/// reused here so that "does the string fit in its box" cannot answer `true`
/// on a difference the differ would have forgiven anyway.
pub(crate) const TOLERANCE: Pixels = px(0.5);

/// An element's **own** background paint.
///
/// Not composited with ancestors — `native/oracle/ANCHORS.md` §3 is explicit
/// that `bg` is what this element paints and nothing else.
#[derive(Clone, Copy, Debug, PartialEq)]
pub enum Paint {
    /// The element paints no background of its own.
    None,
    /// A solid colour.
    Solid(Hsla),
    /// A fill the v1 contract has no way to express — a gradient or a pattern.
    ///
    /// Deliberately *not* collapsed into [`Paint::None`]: a gradient is not
    /// transparent, and emitting `#00000000` for one would hand the differ a
    /// wrong value that parses, which is worse than a missing one.
    Unrepresentable,
}

/// The font facts the contract asks for, already resolved to pixels.
#[derive(Clone, Debug, PartialEq)]
pub struct FontFacts {
    /// Font size in logical pixels.
    pub size: Pixels,
    /// Numeric weight, 100–900.
    pub weight: f32,
    /// The resolved first family. See the crate docs for what "resolved" can
    /// and cannot mean on the gpui side.
    pub family: SharedString,
    /// Line height in logical pixels, snapped to the device pixel grid exactly
    /// as gpui's own text element snaps it.
    pub line_height: Pixels,
}

/// Everything a text-painting anchor contributes over and above a box.
///
/// One struct rather than five parallel `Option`s because the contract's
/// "required: if it paints text" applies to the whole group — an anchor with
/// `text` but no `font` would be a half-populated record the differ cannot act
/// on.
#[derive(Clone, Debug, PartialEq)]
pub struct TextFacts {
    /// The full string, before any visual truncation.
    pub content: SharedString,
    /// The unclipped shaped advance width of `content`.
    pub width: Pixels,
    /// Whether the string is visually truncated in its box.
    pub clipped: bool,
    /// Resolved text colour.
    pub color: Hsla,
    /// Resolved font.
    pub font: FontFacts,
}

/// One anchor as observed during `prepaint`, in **window** coordinates.
///
/// The shift to root-relative coordinates happens in `src/schema.rs`, at
/// serialisation time, because the root's own bounds are not known until every
/// anchor in the frame has been seen.
#[derive(Clone, Debug, PartialEq)]
pub struct RawAnchor {
    /// The stable semantic name. Identical string on both sides of the differ.
    pub id: SharedString,
    /// Border box, logical pixels, window-relative.
    pub bounds: Bounds<Pixels>,
    /// This element's own background paint.
    pub background: Paint,
    /// Present exactly when the anchor paints text.
    pub text: Option<TextFacts>,
    /// Whether the anchor is actually painted.
    pub visible: bool,
    /// Corner radius in logical pixels — the top-left corner. See the crate
    /// docs for the case where the four corners differ.
    pub radius: Pixels,
    /// Border width in logical pixels — the top edge. See the crate docs.
    pub border_width: Pixels,
    /// Border colour, or `None` when gpui would paint no border at all.
    pub border_color: Option<Hsla>,
    /// Whether this anchor's box sizes to its own text
    /// (`native/oracle/ANCHORS.md` v1.5).
    ///
    /// **Declared by the component, never detected here.** A heuristic on this
    /// side would be `width: None` plus a text child, which flex-grow
    /// falsifies; the DOM side's equivalent guess is falsifiable the same way.
    /// Two extractors each guessing is the silent divergence the contract
    /// exists to prevent, and a mis-guess announces nothing — so the flag comes
    /// in from the caller, where it is a visible line in a component.
    pub content_sized: bool,
}

/// Records a box-shaped anchor from its already-resolved [`Style`].
///
/// [`RawAnchor::content_sized`] is **not** an argument: it is not a fact about
/// the style, it is a claim the component made, and the element that carries
/// the claim folds it in with struct-update syntax. A trailing positional
/// `bool` on a six-argument function would read as `…, px(16.0), false)` at
/// every call site, which says nothing.
pub(crate) fn box_facts(
    id: SharedString,
    bounds: Bounds<Pixels>,
    clip: Bounds<Pixels>,
    style: &Style,
    rem_size: Pixels,
) -> RawAnchor {
    RawAnchor {
        id,
        bounds,
        background: background_of(style),
        text: None,
        visible: is_visible(bounds, clip)
            && style.display != Display::None
            && style.visibility != Visibility::Hidden,
        radius: style.corner_radii.to_pixels(rem_size).top_left,
        border_width: style.border_widths.to_pixels(rem_size).top,
        border_color: style.border_color,
        content_sized: false,
    }
}

/// Everything the caller has to have measured before a text anchor can be
/// recorded. A struct rather than eight positional arguments.
pub(crate) struct TextInput<'a> {
    /// The stable semantic name.
    pub id: SharedString,
    /// Border box, logical pixels, window-relative.
    pub bounds: Bounds<Pixels>,
    /// The clip rect in force at this point in the tree.
    pub clip: Bounds<Pixels>,
    /// The inherited, fully resolved text style.
    pub style: &'a TextStyle,
    /// `style.font_size` already resolved against the window's rem size.
    pub font_size: Pixels,
    /// The line height gpui's own text element would use, already snapped.
    pub line_height: Pixels,
    /// The full string, before truncation.
    pub content: SharedString,
    /// The shaped advance width of the *whole* `content`.
    pub shaped_width: Pixels,
    /// Whether the run's box sizes to the run (v1.5). See
    /// [`RawAnchor::content_sized`].
    pub content_sized: bool,
}

/// Records a text-painting anchor.
pub(crate) fn text_facts(input: &TextInput) -> RawAnchor {
    RawAnchor {
        id: input.id.clone(),
        bounds: input.bounds,
        background: input
            .style
            .background_color
            .map_or(Paint::None, Paint::Solid),
        text: Some(TextFacts {
            content: input.content.clone(),
            width: input.shaped_width,
            clipped: is_clipped(input.bounds, input.clip, input.shaped_width),
            color: input.style.color,
            font: FontFacts {
                size: input.font_size,
                weight: input.style.font_weight.0,
                family: input.style.font_family.clone(),
                line_height: input.line_height,
            },
        }),
        visible: is_visible(input.bounds, input.clip),
        // A gpui text run paints neither corners nor a border. Emitted as zero
        // rather than omitted so that every anchor carries the same field set —
        // a field one extractor omits is invisible to the differ.
        radius: px(0.0),
        border_width: px(0.0),
        border_color: None,
        content_sized: input.content_sized,
    }
}

/// Whether the anchor is actually painted: non-zero area, and not entirely
/// outside the clip rect in force.
fn is_visible(bounds: Bounds<Pixels>, clip: Bounds<Pixels>) -> bool {
    !bounds.is_empty() && !bounds.intersect(&clip).is_empty()
}

/// Whether the full string is visually truncated in this box.
///
/// Two ways that happens, and the contract's `text_width` exists to catch the
/// first:
///
/// 1. The box is narrower than the string. A gpui text element that truncates
///    shrinks to the truncated width, so this is exactly "the string did not
///    fit in the box it was given".
/// 2. The box itself is cut horizontally by an ancestor's clip, which is what
///    an unstyled overflowing run does — it keeps its full width and gets
///    masked.
///
/// Vertical clipping is deliberately not counted: `clipped` is about the
/// truncation *point*, and a row cut off at the bottom of a scroll viewport has
/// not truncated its text.
fn is_clipped(bounds: Bounds<Pixels>, clip: Bounds<Pixels>, shaped_width: Pixels) -> bool {
    shaped_width > bounds.size.width + TOLERANCE
        || bounds.origin.x < clip.origin.x - TOLERANCE
        || bounds.origin.x + bounds.size.width > clip.origin.x + clip.size.width + TOLERANCE
}

/// Reads the element's own background, if it is one v1 can express.
fn background_of(style: &Style) -> Paint {
    match style.background.as_ref().and_then(Fill::color) {
        None => Paint::None,
        Some(fill) => fill.as_solid().map_or(Paint::Unrepresentable, Paint::Solid),
    }
}

#[cfg(test)]
mod tests {
    use crowbar_ui::gpui::{
        AbsoluteLength, Bounds, Corners, Display, Edges, Fill, FontWeight, Hsla, Pixels, Point,
        Size, Style, TextStyle, Visibility, blue, bounds, linear_color_stop, linear_gradient,
        point, px, red, rems, size,
    };

    use super::{Paint, TextInput, box_facts, text_facts};

    fn a_box() -> Bounds<Pixels> {
        bounds(point(px(10.0), px(20.0)), size(px(100.0), px(30.0)))
    }

    fn the_window() -> Bounds<Pixels> {
        bounds(point(px(0.0), px(0.0)), size(px(1000.0), px(800.0)))
    }

    fn corners(value: AbsoluteLength) -> Corners<AbsoluteLength> {
        Corners {
            top_left: value,
            top_right: value,
            bottom_right: value,
            bottom_left: value,
        }
    }

    fn edges(value: AbsoluteLength) -> Edges<AbsoluteLength> {
        Edges {
            top: value,
            right: value,
            bottom: value,
            left: value,
        }
    }

    #[test]
    fn a_plain_box_reports_no_paint_and_no_text() {
        let record = box_facts(
            "panel".into(),
            a_box(),
            the_window(),
            &Style::default(),
            px(16.0),
        );

        assert_eq!(record.background, Paint::None);
        assert!(record.text.is_none());
        assert!(record.visible);
        assert_eq!(record.radius, px(0.0));
        assert_eq!(record.border_width, px(0.0));
        assert_eq!(record.border_color, None);
        assert_eq!(record.bounds.origin, point(px(10.0), px(20.0)));
    }

    #[test]
    fn a_solid_background_is_read_back_as_a_solid() {
        let style = Style {
            background: Some(red().into()),
            ..Style::default()
        };

        let record = box_facts("panel".into(), a_box(), the_window(), &style, px(16.0));

        assert_eq!(record.background, Paint::Solid(red()));
    }

    /// A gradient is not transparent. Reporting `#00000000` for one would be a
    /// wrong value that parses, and the differ would point at the wrong thing.
    #[test]
    fn a_gradient_background_is_unrepresentable_rather_than_transparent() {
        let style = Style {
            background: Some(Fill::Color(linear_gradient(
                0.0,
                linear_color_stop(red(), 0.0),
                linear_color_stop(blue(), 1.0),
            ))),
            ..Style::default()
        };

        let record = box_facts("panel".into(), a_box(), the_window(), &style, px(16.0));

        assert_eq!(record.background, Paint::Unrepresentable);
    }

    #[test]
    fn absolute_lengths_are_read_straight_through() {
        let style = Style {
            corner_radii: corners(AbsoluteLength::Pixels(px(4.0))),
            border_widths: edges(AbsoluteLength::Pixels(px(1.0))),
            border_color: Some(blue()),
            ..Style::default()
        };

        let record = box_facts("panel".into(), a_box(), the_window(), &style, px(16.0));

        assert_eq!(record.radius, px(4.0));
        assert_eq!(record.border_width, px(1.0));
        assert_eq!(record.border_color, Some(blue()));
    }

    #[test]
    fn rems_are_resolved_against_the_windows_rem_size() {
        let style = Style {
            corner_radii: corners(AbsoluteLength::Rems(rems(0.25))),
            border_widths: edges(AbsoluteLength::Rems(rems(0.125))),
            ..Style::default()
        };

        let record = box_facts("panel".into(), a_box(), the_window(), &style, px(16.0));

        assert_eq!(record.radius, px(4.0));
        assert_eq!(record.border_width, px(2.0));
    }

    /// gpui reserves layout space for a border whose colour was never set, so
    /// the width is real even when nothing is painted.
    #[test]
    fn a_colourless_border_still_reports_its_layout_width() {
        let style = Style {
            border_widths: edges(AbsoluteLength::Pixels(px(3.0))),
            ..Style::default()
        };

        let record = box_facts("panel".into(), a_box(), the_window(), &style, px(16.0));

        assert_eq!(record.border_width, px(3.0));
        assert_eq!(record.border_color, None);
    }

    #[test]
    fn a_zero_area_box_is_not_visible() {
        let flat = bounds(point(px(10.0), px(20.0)), size(px(0.0), px(30.0)));

        let record = box_facts(
            "panel".into(),
            flat,
            the_window(),
            &Style::default(),
            px(16.0),
        );

        assert!(!record.visible);
    }

    #[test]
    fn a_box_entirely_outside_the_clip_is_not_visible() {
        let clip = bounds(point(px(0.0), px(0.0)), size(px(5.0), px(5.0)));

        let record = box_facts("panel".into(), a_box(), clip, &Style::default(), px(16.0));

        assert!(!record.visible);
    }

    #[test]
    fn a_box_partly_inside_the_clip_is_still_visible() {
        let clip = bounds(point(px(0.0), px(0.0)), size(px(50.0), px(50.0)));

        let record = box_facts("panel".into(), a_box(), clip, &Style::default(), px(16.0));

        assert!(record.visible);
    }

    #[test]
    fn display_none_is_not_visible() {
        let style = Style {
            display: Display::None,
            ..Style::default()
        };

        let record = box_facts("panel".into(), a_box(), the_window(), &style, px(16.0));

        assert!(!record.visible);
    }

    #[test]
    fn visibility_hidden_is_laid_out_but_not_visible() {
        let style = Style {
            visibility: Visibility::Hidden,
            ..Style::default()
        };

        let record = box_facts("panel".into(), a_box(), the_window(), &style, px(16.0));

        assert_eq!(record.bounds.size.width, px(100.0));
        assert!(!record.visible);
    }

    fn a_text_input(
        style: &TextStyle,
        box_width: f32,
        shaped: f32,
        clip: Bounds<Pixels>,
    ) -> TextInput<'_> {
        TextInput {
            id: "row-name".into(),
            bounds: bounds(point(px(30.0), px(4.0)), size(px(box_width), px(16.0))),
            clip,
            style,
            font_size: px(13.0),
            line_height: px(17.5),
            content: "resolve-terminal-connection.ts".into(),
            shaped_width: px(shaped),
            content_sized: false,
        }
    }

    #[test]
    fn a_text_anchor_carries_the_whole_font_group() {
        let style = TextStyle {
            color: red(),
            font_family: "Inter".into(),
            font_weight: FontWeight(500.0),
            ..TextStyle::default()
        };

        let record = text_facts(&a_text_input(&style, 200.0, 186.5, the_window()));
        let text = record.text.expect("a text anchor paints text");

        assert_eq!(text.content, "resolve-terminal-connection.ts");
        assert_eq!(text.width, px(186.5));
        assert_eq!(text.color, red());
        assert_eq!(text.font.size, px(13.0));
        assert!((text.font.weight - 500.0).abs() < f32::EPSILON);
        assert_eq!(text.font.family, "Inter");
        assert_eq!(text.font.line_height, px(17.5));
        assert!(!text.clipped);
        assert!(record.visible);
    }

    #[test]
    fn a_string_wider_than_its_box_is_clipped() {
        let style = TextStyle::default();

        let record = text_facts(&a_text_input(&style, 118.0, 186.5, the_window()));

        assert!(record.text.expect("paints text").clipped);
    }

    /// The half-pixel margin exists so that `clipped` cannot flip on a
    /// difference §5 would have forgiven in `bounds` anyway.
    #[test]
    fn a_string_within_half_a_pixel_of_its_box_is_not_clipped() {
        let style = TextStyle::default();

        let record = text_facts(&a_text_input(&style, 118.0, 118.4, the_window()));

        assert!(!record.text.expect("paints text").clipped);
    }

    #[test]
    fn a_box_cut_horizontally_by_an_ancestor_is_clipped() {
        let style = TextStyle::default();
        let clip = bounds(point(px(0.0), px(0.0)), size(px(100.0), px(800.0)));

        let record = text_facts(&a_text_input(&style, 118.0, 118.0, clip));

        assert!(record.text.expect("paints text").clipped);
    }

    #[test]
    fn a_box_cut_off_at_its_left_edge_is_clipped() {
        let style = TextStyle::default();
        let clip = bounds(point(px(50.0), px(0.0)), size(px(1000.0), px(800.0)));

        let record = text_facts(&a_text_input(&style, 118.0, 118.0, clip));

        assert!(record.text.expect("paints text").clipped);
    }

    /// A row cut off at the bottom of a scroll viewport has not truncated its
    /// text, and saying it has would send the differ hunting a phantom.
    #[test]
    fn a_box_cut_only_vertically_is_not_clipped() {
        let style = TextStyle::default();
        let clip = bounds(point(px(0.0), px(0.0)), size(px(1000.0), px(10.0)));

        let record = text_facts(&a_text_input(&style, 118.0, 118.0, clip));

        assert!(!record.text.expect("paints text").clipped);
    }

    #[test]
    fn a_text_run_reports_its_own_background_and_no_box_paint() {
        let style = TextStyle {
            background_color: Some(blue()),
            ..TextStyle::default()
        };

        let record = text_facts(&a_text_input(&style, 200.0, 186.5, the_window()));

        assert_eq!(record.background, Paint::Solid(blue()));
        assert_eq!(record.radius, px(0.0));
        assert_eq!(record.border_width, px(0.0));
        assert_eq!(record.border_color, None);
    }

    #[test]
    fn a_text_run_with_no_background_paints_none() {
        let style = TextStyle::default();

        let record = text_facts(&a_text_input(&style, 200.0, 186.5, the_window()));

        assert_eq!(record.background, Paint::None);
    }

    #[test]
    fn a_text_run_squeezed_to_nothing_is_not_visible() {
        let style = TextStyle::default();

        let record = text_facts(&a_text_input(&style, 0.0, 186.5, the_window()));

        assert!(!record.visible);
    }

    /// Guards the assumption `is_visible` rests on: `intersect` never returns a
    /// negative size for disjoint boxes, it returns a degenerate one.
    #[test]
    fn intersecting_disjoint_boxes_gives_an_empty_box_not_a_negative_one() {
        let left = bounds(point(px(0.0), px(0.0)), size(px(10.0), px(10.0)));
        let right = bounds(point(px(100.0), px(100.0)), size(px(10.0), px(10.0)));

        let overlap: Bounds<Pixels> = left.intersect(&right);

        assert!(overlap.is_empty());
        assert_eq!(overlap.size, Size::default());
    }

    #[test]
    fn an_hsla_is_carried_verbatim_into_the_record() {
        let colour = Hsla {
            h: 0.25,
            s: 0.5,
            l: 0.75,
            a: 0.5,
        };
        let style = Style {
            background: Some(colour.into()),
            ..Style::default()
        };

        let record = box_facts("panel".into(), a_box(), the_window(), &style, px(16.0));

        assert_eq!(record.background, Paint::Solid(colour));
        assert_eq!(
            record.bounds.origin,
            Point {
                x: px(10.0),
                y: px(20.0),
            },
        );
    }
}
