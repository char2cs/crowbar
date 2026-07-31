//! The §5 tolerance table, as data.
//!
//! Every numeric tolerance the differ applies lives in exactly one struct. No
//! comparison anywhere in this crate carries its own constant, so changing what
//! the oracle will forgive is a single edit in a single place, and `grep
//! Tolerances` finds every caller.
//!
//! That is not tidiness. ANCHORS.md §5 says plainly that loosening a tolerance
//! is *the single cheapest way to make a gate pass while it tells you nothing*,
//! and requires any loosening to land in its own commit stating the measurement
//! that justified it and the class of defect the looser value can no longer
//! catch. A tolerance scattered across a dozen `0.5`s in comparison code cannot
//! be reviewed that way; a struct can.
//!
//! # What is deliberately not here
//!
//! `fg`/`bg`/`border.color` RGB, `font.weight`, `text`, `clipped`, `visible`
//! and — since v1.1 — `border.w` are **exact** in §5. They have no field in this
//! struct and no knob, because a knob for them would only ever be used to make a
//! failing gate pass. Alpha is the one colour component with a tolerance, and it
//! has one because 8-bit rounding is a real engine difference rather than a
//! defect.

/// The numeric slack the differ allows, per field family.
///
/// [`Tolerances::V1`] is the ANCHORS.md §5 table. Construct a modified copy with
/// struct-update syntax; nothing in the crate reads a tolerance from anywhere
/// else.
///
/// ```
/// # use oracle::Tolerances;
/// let tighter = Tolerances {
///     bounds_px: 0.25,
///     ..Tolerances::V1
/// };
/// assert!(tighter.bounds_px < Tolerances::V1.bounds_px);
/// ```
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Tolerances {
    /// `bounds` x/y/w/h, in logical px. §5: **±0.5** — sub-pixel layout
    /// legitimately differs between engines.
    pub bounds_px: f64,
    /// `text_width`, in logical px. §5: **±1.0** — shaping and hinting differ.
    pub text_width_px: f64,
    /// `font.size`, in logical px. §5: **±0.5**.
    pub font_size_px: f64,
    /// `font.line_height`, in logical px. §5: **±0.5**.
    pub line_height_px: f64,
    /// Alpha on any colour field, in 1/255 units. §5: **±1** for rounding.
    /// RGB has no equivalent because §5 makes it exact.
    pub alpha: u8,
    /// `radius`, in logical px. §5: **±0.5** *(v1.1)*.
    ///
    /// v1 defined `radius` as a field and gave it no tolerance; v1.1 ruling 1
    /// closed that with ±0.5, the same as `bounds`, because it is the same kind
    /// of quantity. `border.w` was ruled the other way in the same breath and
    /// so has no field here at all — see the module docs.
    pub radius_px: f64,
}

impl Tolerances {
    /// The ANCHORS.md §5 table, verbatim.
    pub const V1: Self = Self {
        bounds_px: 0.5,
        text_width_px: 1.0,
        font_size_px: 0.5,
        line_height_px: 0.5,
        alpha: 1,
        radius_px: 0.5,
    };
}

impl Default for Tolerances {
    fn default() -> Self {
        Self::V1
    }
}

/// Whether two numbers agree to within `tolerance`.
///
/// Inclusive at the boundary: a difference of exactly the tolerance passes, so
/// `±0.5` means what it reads like. Never uses `==` on `f64`; both inputs are
/// guaranteed finite by the parser, so the subtraction is well defined and the
/// comparison has no `NaN` case to get wrong.
#[must_use]
pub fn within(a: f64, b: f64, tolerance: f64) -> bool {
    (a - b).abs() <= tolerance
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Bit-exact equality, so the table assertions below do not themselves go
    /// through a tolerance.
    fn exactly(a: f64, b: f64) -> bool {
        a.to_bits() == b.to_bits()
    }

    #[test]
    fn v1_is_the_anchors_md_table() {
        let t = Tolerances::V1;
        assert!(exactly(t.bounds_px, 0.5));
        assert!(exactly(t.text_width_px, 1.0));
        assert!(exactly(t.font_size_px, 0.5));
        assert!(exactly(t.line_height_px, 0.5));
        assert_eq!(t.alpha, 1);
        assert!(exactly(t.radius_px, 0.5));
        assert_eq!(Tolerances::default(), Tolerances::V1);
    }

    #[test]
    fn within_is_inclusive_at_the_boundary() {
        // Values chosen to be exactly representable, so the boundary case is
        // testing the comparison and not the arithmetic.
        assert!(within(8.5, 8.0, 0.5), "exactly at tolerance must pass");
        assert!(within(7.5, 8.0, 0.5), "exactly at tolerance, other side");
        assert!(!within(8.625, 8.0, 0.5), "just past tolerance must fail");
        assert!(!within(7.375, 8.0, 0.5), "just past tolerance, other side");
        assert!(
            within(8.0, 8.0, 0.0),
            "a zero tolerance still matches equals"
        );
        assert!(!within(8.25, 8.0, 0.0));
        assert!(within(-0.5, 0.0, 0.5));
        assert!(within(0.0, -0.0, 0.0), "signed zero is not a difference");
    }

    #[test]
    fn tolerances_are_overridable_wholesale() {
        let loose = Tolerances {
            bounds_px: 2.0,
            ..Tolerances::V1
        };
        assert!(within(10.0, 8.0, loose.bounds_px));
        assert!(!within(10.0, 8.0, Tolerances::V1.bounds_px));
    }
}
