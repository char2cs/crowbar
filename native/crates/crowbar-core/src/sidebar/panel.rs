//! The sidebar panel's width, its open state, and which resizes count as the
//! user's intent.
//!
//! Ported from `web/src/components/layout/use-sidebar-panel.ts`.
//!
//! # Only a separator drag expresses a width preference
//!
//! Everything else that resizes the sidebar is a consequence of something
//! else, and persisting those indiscriminately — as the React hook used to, on
//! every resize tick — let one narrow window redefine the sidebar's width for
//! good: at a 700 px window the content pane's 20% minimum squeezes a 640 px
//! sidebar down to 559 px, and 559 was then written to storage as if the user
//! had chosen it.
//!
//! # The discrimination is a latch, not a per-event test
//!
//! **One window resize emits two resize events at the same group width.**
//! First the sidebar's existing percentage applied to the new width, then
//! `preserve-pixel-size` correcting that back to pixels — measured, on a
//! 1714 px → 700 px narrowing: 245 px at 35%, then 559 px at 80%. Only the
//! *first* carries a changed group width, so a per-event test lets the second
//! through as if the user had chosen it. [`SidebarPanel::note_pointer_down`]
//! clears the latch; a changed group width sets it.
//!
//! **Note which way it fails.** A missed pointer-down means a width is not
//! remembered. It never means a squeezed width is remembered as a choice.
//!
//! # The chosen width is committed once, not per frame
//!
//! [`SidebarPanel::on_resize`] runs on every frame of a drag. React learned
//! the cost the expensive way: a `setState` there re-rendered the whole shell
//! and a synchronous `localStorage` write blocked it, ~90 times per drag. The
//! width lands in a pending slot and [`SidebarPanel::commit_preferred_width`]
//! flushes it once, when the layout settles.

/// Narrowest the panel may be dragged, in pixels.
pub const MIN_PX: f32 = 250.0;
/// Widest the panel may be dragged, in pixels.
pub const MAX_PX: f32 = 640.0;
/// Width a sidebar with no remembered preference opens at, in pixels.
pub const DEFAULT_PX: f32 = 294.0;

/// Clamp a width into the panel's range.
///
/// Clamped at **both** ends, deliberately. The panel itself is bounded by its
/// own min/max, but the hover-peek card takes this number raw — so a stale or
/// hand-edited over-large stored value would render the peek past the opposite
/// window edge.
#[must_use]
pub fn clamp_width(px: f32) -> f32 {
    if px.is_nan() {
        return DEFAULT_PX;
    }
    px.clamp(MIN_PX, MAX_PX)
}

/// Interpret the daemon's stored `sidebar-width` row.
///
/// Mirrors `parseInt(stored ?? '', 10)` followed by a finiteness check: a value
/// that does not begin with an integer is not a width, and the default is used
/// rather than an error raised. A row written by a newer build must never stop
/// the sidebar from opening.
#[must_use]
pub fn width_from_stored(stored: Option<&str>) -> f32 {
    stored
        .and_then(parse_int_prefix)
        .map_or(DEFAULT_PX, clamp_width)
}

/// Interpret the daemon's stored `sidebar-open` row.
///
/// Mirrors `localStorage.getItem('sidebar-open') !== 'false'` — **absent means
/// open**. Only the exact string `false` closes it, so a missing or corrupt row
/// opens the sidebar rather than hiding the app's primary navigation.
#[must_use]
pub fn open_from_stored(stored: Option<&str>) -> bool {
    stored != Some("false")
}

/// JavaScript's `parseInt(s, 10)`: skip leading whitespace, take an optional
/// sign and then as many decimal digits as follow, and stop at the first
/// character that is not one. `None` where JS yields `NaN`.
///
/// Ported rather than replaced with [`str::parse`] because the two disagree on
/// real stored values: `"294px"` is 294 to `parseInt` and an error to
/// `str::parse`, and a row written by the React app can carry either shape.
fn parse_int_prefix(s: &str) -> Option<f32> {
    let s = s.trim_start();
    let (negative, digits) = match s.strip_prefix('-') {
        Some(rest) => (true, rest),
        None => (false, s.strip_prefix('+').unwrap_or(s)),
    };
    let end = digits
        .find(|c: char| !c.is_ascii_digit())
        .unwrap_or(digits.len());
    let taken = &digits[..end];
    if taken.is_empty() {
        return None;
    }
    // Parsed as a float, not an integer, for two reasons: `parseInt` yields a
    // JS number and a 40-digit string is a finite (if inexact) one rather than
    // an overflow, and an integer here would have to be cast back to a pixel
    // width — a cast this module then has to justify. `f32::from_str` on a
    // digits-only string is exact for every value a width can be.
    let value = taken.parse::<f32>().ok()?;
    Some(if negative { -value } else { value })
}

/// One resize event from the layout, reduced to what the decision needs.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Resize {
    /// The sidebar panel's new width, in pixels.
    pub width_px: f32,
    /// The width of the group the panel sits in. A separator drag moves the
    /// divider inside a group whose own width never changes, which is the one
    /// signal that separates a drag from a window resize.
    pub group_width_px: f32,
    /// Whether this event collapsed the panel.
    pub collapsed: bool,
    /// The panel's previous state, or `None` on its **first** resize — mount,
    /// and the re-registration a side flip causes. A first resize carries no
    /// preference because there is nothing it could be a change from.
    pub previous: Option<Previous>,
}

/// The panel's state before a resize.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Previous {
    /// Whether the panel was collapsed. Growing back out of collapsed restores
    /// whatever width the panel had when it was collapsed, which may itself
    /// have been a squeezed one — so it is not a preference either.
    pub collapsed: bool,
}

/// Open/collapsed state and remembered width, and the rules for deciding which
/// resizes are the user's intent.
#[derive(Debug, Clone, PartialEq)]
pub struct SidebarPanel {
    open: bool,
    preferred_width: f32,
    window_driven: bool,
    last_group_width: Option<f32>,
    pending_width: Option<f32>,
}

impl SidebarPanel {
    /// Restore from the daemon's two stored rows.
    #[must_use]
    pub fn restore(stored_open: Option<&str>, stored_width: Option<&str>) -> Self {
        Self {
            open: open_from_stored(stored_open),
            preferred_width: width_from_stored(stored_width),
            // The React hook starts latched: nothing has happened yet that
            // could be a drag, so the first resizes must not be trusted.
            window_driven: true,
            last_group_width: None,
            pending_width: None,
        }
    }

    /// Whether the panel is showing.
    #[must_use]
    pub const fn is_open(&self) -> bool {
        self.open
    }

    /// The width the **user** chose, as opposed to whatever width the panel
    /// happens to have right now. It is what the sidebar comes back at when
    /// the panel re-registers on a side flip, what the hover-peek card is
    /// sized to, and what survives in storage across restarts.
    #[must_use]
    pub const fn preferred_width(&self) -> f32 {
        self.preferred_width
    }

    /// Open or close the panel. Returns the value to persist when it changed.
    pub fn set_open(&mut self, open: bool) -> Option<bool> {
        if self.open == open {
            return None;
        }
        self.open = open;
        Some(open)
    }

    /// Toggle the panel. Returns the value to persist.
    pub fn toggle(&mut self) -> bool {
        self.open = !self.open;
        self.open
    }

    /// A pointer went down on the group. Clears the window-driven latch: the
    /// resizes that follow may be a separator drag, which is the one thing
    /// allowed to redefine the remembered width.
    ///
    /// Deliberately the whole group rather than the separator alone — the
    /// drag hit target extends past that 1 px element onto the adjacent panel
    /// edges.
    pub fn note_pointer_down(&mut self) {
        self.window_driven = false;
    }

    /// Fold one resize event in. Returns `Some(open)` when the panel's open
    /// state changed and that change needs persisting.
    pub fn on_resize(&mut self, resize: Resize) -> Option<bool> {
        let changed_open = self.set_open(!resize.collapsed);
        if resize.collapsed || resize.width_px <= 0.0 {
            return changed_open;
        }

        if self
            .last_group_width
            .is_some_and(|last| !approx_eq(last, resize.group_width_px))
        {
            self.window_driven = true;
        }
        self.last_group_width = Some(resize.group_width_px);

        let is_preference =
            !self.window_driven && resize.previous.is_some_and(|previous| !previous.collapsed);
        if is_preference {
            // Held, not committed: this runs on every frame of a drag.
            self.pending_width = Some(resize.width_px.round());
        }
        changed_open
    }

    /// Flush the width a drag landed on, once, after the pointer is released.
    /// Returns the width to persist, or `None` when no drag chose one.
    pub fn commit_preferred_width(&mut self) -> Option<f32> {
        let width = self.pending_width.take()?;
        let width = clamp_width(width);
        self.preferred_width = width;
        Some(width)
    }
}

/// Group widths arrive as floats and a window resize is detected by them
/// *changing*, so an exact comparison would treat sub-pixel jitter as a new
/// window size and latch out a real drag.
fn approx_eq(a: f32, b: f32) -> bool {
    (a - b).abs() < 0.5
}

#[cfg(test)]
mod tests {
    use super::{
        DEFAULT_PX, MAX_PX, MIN_PX, Previous, Resize, SidebarPanel, clamp_width, open_from_stored,
        width_from_stored,
    };
    use crate::sidebar::fixtures::assert_px;

    /// A resize that is not collapsed and follows a previous open state — the
    /// shape every drag frame has.
    fn drag(width_px: f32, group_width_px: f32) -> Resize {
        Resize {
            width_px,
            group_width_px,
            collapsed: false,
            previous: Some(Previous { collapsed: false }),
        }
    }

    // --- stored rows -------------------------------------------------------

    #[test]
    fn an_absent_width_row_opens_at_the_default() {
        assert_px!(width_from_stored(None), DEFAULT_PX);
    }

    #[test]
    fn a_stored_width_is_clamped_at_both_ends() {
        assert_px!(width_from_stored(Some("300")), 300.0);
        assert_px!(width_from_stored(Some("10")), MIN_PX);
        assert_px!(
            width_from_stored(Some("9999")),
            MAX_PX,
            "the peek card takes this raw, so an over-large row must not escape"
        );
    }

    /// `parseInt` takes a leading integer and stops; `str::parse` would error.
    /// A row written by the React app can carry either shape.
    #[test]
    fn a_width_row_parses_like_parse_int() {
        assert_px!(width_from_stored(Some("294px")), 294.0);
        assert_px!(width_from_stored(Some("  310  ")), 310.0);
        assert_px!(width_from_stored(Some("+300")), 300.0);
        assert_px!(width_from_stored(Some("-40")), MIN_PX);
    }

    #[test]
    fn an_unparseable_width_row_falls_back_to_the_default() {
        assert_px!(width_from_stored(Some("")), DEFAULT_PX);
        assert_px!(width_from_stored(Some("wide")), DEFAULT_PX);
        assert_px!(width_from_stored(Some("px294")), DEFAULT_PX);
    }

    /// Absent means open — a missing or corrupt row must not hide the app's
    /// primary navigation.
    #[test]
    fn only_the_exact_string_false_closes_the_sidebar() {
        assert!(open_from_stored(None));
        assert!(open_from_stored(Some("true")));
        assert!(open_from_stored(Some("False")));
        assert!(open_from_stored(Some("")));
        assert!(!open_from_stored(Some("false")));
    }

    #[test]
    fn clamping_a_nan_yields_the_default() {
        assert_px!(clamp_width(f32::NAN), DEFAULT_PX);
    }

    // --- open state --------------------------------------------------------

    #[test]
    fn restore_reads_both_rows() {
        let panel = SidebarPanel::restore(Some("false"), Some("320"));
        assert!(!panel.is_open());
        assert_px!(panel.preferred_width(), 320.0);
    }

    #[test]
    fn set_open_reports_only_real_changes() {
        let mut panel = SidebarPanel::restore(None, None);
        assert_eq!(panel.set_open(true), None, "already open");
        assert_eq!(panel.set_open(false), Some(false));
        assert!(!panel.is_open());
    }

    #[test]
    fn toggle_flips_and_reports() {
        let mut panel = SidebarPanel::restore(None, None);
        assert!(!panel.toggle());
        assert!(panel.toggle());
    }

    #[test]
    fn a_collapsing_resize_closes_the_panel() {
        let mut panel = SidebarPanel::restore(None, None);
        let changed = panel.on_resize(Resize {
            collapsed: true,
            ..drag(0.0, 1200.0)
        });
        assert_eq!(changed, Some(false));
        assert!(!panel.is_open());
    }

    // --- which resizes are the user's intent -------------------------------

    /// The panel starts latched, so the resizes a mount emits are not a
    /// preference.
    #[test]
    fn a_drag_is_only_trusted_after_a_pointer_down() {
        let mut panel = SidebarPanel::restore(None, None);
        panel.on_resize(drag(400.0, 1200.0));
        assert_px!(
            panel.commit_preferred_width(),
            None,
            "no pointer went down, so nothing chose this width"
        );
        assert_px!(panel.preferred_width(), DEFAULT_PX);
    }

    #[test]
    fn a_pointer_down_then_a_drag_records_the_width() {
        let mut panel = SidebarPanel::restore(None, None);
        panel.note_pointer_down();
        panel.on_resize(drag(400.0, 1200.0));
        assert_px!(panel.commit_preferred_width(), Some(400.0));
        assert_px!(panel.preferred_width(), 400.0);
    }

    /// The width is held until the drag settles: ~90 frames must produce one
    /// commit, not ninety.
    #[test]
    fn only_the_last_frame_of_a_drag_is_committed() {
        let mut panel = SidebarPanel::restore(None, None);
        panel.note_pointer_down();
        for width in [300.0, 350.0, 400.0, 420.0] {
            panel.on_resize(drag(width, 1200.0));
        }
        assert_px!(panel.commit_preferred_width(), Some(420.0));
        assert_px!(
            panel.commit_preferred_width(),
            None,
            "the pending slot is emptied by the commit"
        );
    }

    /// **The bug this latch exists for.** One window resize emits two events
    /// at the same group width — the percentage applied to the new width, then
    /// `preserve-pixel-size` correcting it back. A per-event test lets the
    /// second through, and 559 px gets stored as if the user chose it.
    #[test]
    fn a_window_resize_emits_two_events_and_neither_is_a_preference() {
        let mut panel = SidebarPanel::restore(None, Some("640"));
        panel.note_pointer_down();
        // A real drag first, so `last_group_width` is primed and the latch is
        // clear — exactly the state a user leaves the sidebar in.
        panel.on_resize(drag(640.0, 1714.0));
        assert_px!(panel.commit_preferred_width(), Some(640.0));

        // The window narrows: 1714 -> 700. Event one carries the new group
        // width, event two does not.
        panel.on_resize(drag(245.0, 700.0));
        panel.on_resize(drag(559.0, 700.0));

        assert_px!(
            panel.commit_preferred_width(),
            None,
            "the squeezed width is not a choice"
        );
        assert_px!(panel.preferred_width(), 640.0, "the user's width survives");
    }

    /// Which way the latch fails matters: a missed pointer-down loses a width,
    /// it never records a squeezed one.
    #[test]
    fn a_window_resize_after_a_pointer_down_still_latches_out() {
        let mut panel = SidebarPanel::restore(None, Some("500"));
        panel.note_pointer_down();
        panel.on_resize(drag(500.0, 1200.0));
        panel.on_resize(drag(300.0, 800.0));
        assert_px!(panel.commit_preferred_width(), Some(500.0));
    }

    /// `prevSize === undefined`: mount, and the re-registration a side flip
    /// causes. Nothing it could be a change from.
    #[test]
    fn the_first_resize_of_a_panel_is_not_a_preference() {
        let mut panel = SidebarPanel::restore(None, None);
        panel.note_pointer_down();
        panel.on_resize(Resize {
            previous: None,
            ..drag(400.0, 1200.0)
        });
        assert_px!(panel.commit_preferred_width(), None);
    }

    /// Growing back out of collapsed restores whatever width the panel had
    /// when it collapsed, which may itself have been squeezed.
    #[test]
    fn expanding_from_collapsed_is_not_a_preference() {
        let mut panel = SidebarPanel::restore(None, None);
        panel.note_pointer_down();
        panel.on_resize(Resize {
            previous: Some(Previous { collapsed: true }),
            ..drag(400.0, 1200.0)
        });
        assert_px!(panel.commit_preferred_width(), None);
    }

    #[test]
    fn a_zero_width_resize_records_nothing() {
        let mut panel = SidebarPanel::restore(None, None);
        panel.note_pointer_down();
        panel.on_resize(drag(0.0, 1200.0));
        assert_px!(panel.commit_preferred_width(), None);
    }

    /// Group widths are floats; sub-pixel jitter must not read as a new window
    /// size and cancel a live drag.
    #[test]
    fn sub_pixel_group_jitter_does_not_latch_out_a_drag() {
        let mut panel = SidebarPanel::restore(None, None);
        panel.note_pointer_down();
        panel.on_resize(drag(400.0, 1200.0));
        panel.on_resize(drag(410.0, 1200.2));
        assert_px!(panel.commit_preferred_width(), Some(410.0));
    }

    #[test]
    fn a_committed_width_is_clamped() {
        let mut panel = SidebarPanel::restore(None, None);
        panel.note_pointer_down();
        panel.on_resize(drag(5000.0, 6000.0));
        assert_px!(panel.commit_preferred_width(), Some(MAX_PX));
    }

    #[test]
    fn a_fractional_drag_width_is_rounded() {
        let mut panel = SidebarPanel::restore(None, None);
        panel.note_pointer_down();
        panel.on_resize(drag(400.6, 1200.0));
        assert_px!(panel.commit_preferred_width(), Some(401.0));
    }
}
