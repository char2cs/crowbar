//! `sidebar_peek` — the hover-to-peek host: while the sidebar is collapsed,
//! bringing the pointer to the window edge slides a floating copy of it in
//! over the editor.
//!
//! The native half of `web/src/components/layout/sidebar-peek.tsx`. See
//! `native/mapping/sidebar-peek.md`.
//!
//! # Re-derived, not inherited: the same cluster-4 reasoning, and it still holds
//!
//! `native/mapping/layout-denominator.md` §4 pairs this file with
//! `nav-stack.tsx` as the tier's two judgment calls: a store-driven wrapper
//! whose CSS-transition **end states** are the port target, with the
//! transition itself out of the oracle's reach (`ANCHORS.md` §6, "a
//! snapshot is one instant"). §4 itself flags this file as the *closer*
//! call of the two, because the *trigger* — a `document`-level
//! `pointermove` listener computing a `hovered` boolean from raw cursor
//! coordinates — is continuous interaction, not a discrete store action the
//! way `push`/`pop` are. Checked directly against this file rather than
//! taken on that flag: what the trigger produces is a single `hovered`
//! boolean collapsed with `hidden` into one `data-state`, which drives
//! **three** concrete, static geometries (`docked`/`closed`/`peeking`) —
//! every one of them drivable by setting the state directly, with no
//! simulated pointer motion needed, exactly the shape `sidebar-carousel`'s
//! own `selected` cell already established as in scope. The reasoning holds
//! for this file too; the trigger's own continuity is real but orthogonal
//! to what gets ported here, the same way `nav-stack.tsx`'s own store calls
//! are orthogonal to the 280ms transition wrapping them.
//!
//! # No anchor on the outer wrapper — argued, not asserted
//!
//! `sidebar-peek.tsx`'s outer `<div data-sidebar-peek data-state={...}
//! className={cn('flex min-h-0 flex-col', hidden ? 'contents' : 'h-full')}>`
//! is the shape `workspace-switcher.tsx`'s own wrapper is, extended by one
//! step. Two facts, together, are why it carries no `data-oracle-id`:
//!
//! 1. **In two of the three states it generates no box at all.** When
//!    `hidden`, its class list resolves to `contents` — `ANCHORS.md` v1.11
//!    ("an element that generates no box is not an anchor") forbids
//!    anchoring it in the `closed` and `peeking` cells outright.
//! 2. **In the third state its box is byte-identical to its own sole
//!    child's.** When *not* `hidden`, the outer wrapper's class list is
//!    `flex min-h-0 flex-col h-full` — and the inner `<div>`'s own
//!    `hidden ? [...] : 'h-full'` resolves, in that same branch, to
//!    `flex min-h-0 flex-col h-full`: the identical string. Neither
//!    carries padding, margin or border, so one box exactly contains the
//!    other with zero offset in every `docked` cell there is. An anchor
//!    here would report the same bounds as [`ID_ROOT`] on every cell that
//!    could ever produce it.
//!
//! So there is no cell in which anchoring the outer wrapper would add
//! anything the [`ID_ROOT`] anchor (on the inner div, which is a real box
//! in **all three** states) does not already carry. [`ID_ROOT`] is placed
//! on the inner div for exactly that reason.
//!
//! # `before:`, the muted wash and `bg-clip-padding` are unpainted — the `popover` precedent
//!
//! `sidebar-peek.tsx`'s own comment says its card recipe *is*
//! `popover.tsx`/`dialog.tsx`'s: a `before:` inset highlight, a `bg-clip-
//! padding` border blend and `shadow-lg/5`. `popover.rs`'s own module docs
//! already settle this for the identical recipe: "the `before:` inset
//! shadow [is] `ANCHORS.md` §6 material — no field, either side." `shadow_lg()`
//! is painted anyway, the same call `popover.rs` and `fps_overlay.rs` both
//! make, for visual fidelity though it moves nothing the differ can see;
//! the `before:` pseudo-element and the clip-padding blend are not
//! reproduced at all, because there is no gpui primitive for either and no
//! contract field would ever compare them.
//!
//! # The card's screen position is computed, not transformed
//!
//! `translate-x-0` (peeking) and `translate-x-[calc(±100%+1rem)]` (closed)
//! both resolve against the card's own border box — which is a literal
//! pixel width here (`w-(--peek-width)`), not a percentage — so the two
//! resting offsets are ordinary arithmetic on [`SidebarPeek::peek_width`]
//! rather than anything that needs gpui's absent `transform` support. See
//! [`SidebarPeek::edge_offset`].
//!
//! # `children` is opaque
//!
//! The card wraps the *entire* sidebar column (`SidebarProjectHeader`,
//! `ContextPill`, `SidebarTabBar`, `SidebarCarousel`, `SidebarToastOverlay`
//! — see `ide-shell.tsx`), which this component does not own and cannot
//! repaint a second copy of. [`SidebarPeek::content_width`] is the same
//! drivable-filler argument `sidebar_carousel`'s `panel_content_width` and
//! `nav_stack`'s `content_width` already make: none of the three states'
//! box sizing depends on what is inside it.

use gpui::{AnyElement, Div, ParentElement as _, Pixels, Styled as _, div, px};

use crate::anchor::{AnchorId, AnchorSink};
use crate::theme::Theme;

/// The one anchor this surface carries — the inner div. See the module docs
/// for why the outer `data-sidebar-peek` wrapper carries none.
pub const ID_ROOT: &str = "sidebar-peek";

/// **Empty.** The card's own box is authored in every state: `h-full` when
/// docked, `w-(--peek-width)` plus `inset-y-2` when hidden. Nothing sizes to
/// its own content — the content is the opaque sidebar column.
pub const CONTENT_SIZED: [&str; 0] = [];
/// **Empty**, for the same reason: this surface paints no text run of its
/// own.
pub const LINE_SIZED: [&str; 0] = [];

/// `inset-y-2` / `left-2` / `right-2` — Tailwind's `0.5rem` at a 16px root.
pub const PEEK_MARGIN: Pixels = px(8.0);
/// The `+1rem` in `translate-x-[calc(±100%+1rem)]` — clearance so no sliver
/// of the parked card shows past its own margin and shadow.
pub const OFFSCREEN_GAP: Pixels = px(16.0);
/// `border` — Tailwind's default width.
pub const BORDER_WIDTH: Pixels = px(1.0);
/// The `- 1rem` in `max-w-[calc(100vw-1rem)]`.
pub const MAX_WIDTH_GAP: Pixels = px(16.0);

/// `data-state`'s own three words.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum PeekState {
    /// `!hidden` — the ordinary, pinned-open sidebar. No fixed positioning,
    /// no card chrome: `h-full` in normal flow.
    Docked,
    /// `hidden && !hovered` — parked off-screen, past [`OFFSCREEN_GAP`].
    Closed,
    /// `hidden && hovered` — on screen, at [`PEEK_MARGIN`] off the docked
    /// edge.
    Peeking,
}

impl PeekState {
    /// Its word in `data-state` and on the command line.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Docked => "docked",
            Self::Closed => "closed",
            Self::Peeking => "peeking",
        }
    }

    /// Whether this state renders the fixed-position card at all.
    #[must_use]
    pub const fn hidden(self) -> bool {
        !matches!(self, Self::Docked)
    }
}

/// The peek host, at one resting state.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct SidebarPeek {
    /// `data-state`.
    pub state: PeekState,
    /// `side === 'right'`.
    pub is_right: bool,
    /// `width` — the user's remembered sidebar width. Read in every state:
    /// it sizes the docked column's own call site too, though this
    /// component only paints it explicitly in [`PeekState::Closed`]/
    /// [`PeekState::Peeking`] (`--peek-width`); [`PeekState::Docked`] takes
    /// its width from whatever the call site gives it (`w_full`), the same
    /// convention every ordinary row surface's `--width` already is.
    pub peek_width: Pixels,
    /// The window's own width — `100vw` in the card's `max-w` calc. Unread
    /// outside [`PeekState::Closed`]/[`PeekState::Peeking`].
    pub viewport_width: Pixels,
    /// The window's own **content** height — the quantity `inset-y-2`'s
    /// `top`/`bottom` resolve against in a real browser. Unread outside
    /// [`PeekState::Closed`]/[`PeekState::Peeking`]. See
    /// [`SidebarPeek::render`]'s own doc comment for why this is an
    /// explicit field rather than gpui computing it from `top`+`bottom`
    /// alone.
    pub content_height: Pixels,
    /// The max-content width of a filler placed inside the card — see the
    /// module docs.
    pub content_width: Pixels,
}

impl Default for SidebarPeek {
    fn default() -> Self {
        Self {
            state: PeekState::Docked,
            is_right: false,
            peek_width: px(280.0),
            viewport_width: px(1200.0),
            content_height: px(600.0),
            content_width: px(0.0),
        }
    }
}

impl SidebarPeek {
    /// The live default: docked, on the left.
    #[must_use]
    pub fn fixture() -> Self {
        Self::default()
    }

    /// The card's own leading-edge offset in [`PeekState::Closed`]/
    /// [`PeekState::Peeking`] — the side [`SidebarPeek::is_right`] names,
    /// [`PEEK_MARGIN`] on screen or [`PEEK_MARGIN`] minus its own width and
    /// [`OFFSCREEN_GAP`] parked off it. See the module docs: this is
    /// ordinary arithmetic on the card's own literal width, standing in for
    /// a CSS transform gpui has no primitive for.
    #[must_use]
    pub fn edge_offset(&self) -> Pixels {
        match self.state {
            PeekState::Docked => px(0.0),
            PeekState::Peeking => PEEK_MARGIN,
            PeekState::Closed => PEEK_MARGIN - self.peek_width - OFFSCREEN_GAP,
        }
    }

    fn filler(&self) -> Option<Div> {
        (self.content_width > px(0.0)).then(|| div().w(self.content_width))
    }

    /// Renders the card, opting [`ID_ROOT`] into `anchors`.
    ///
    /// # `top`+`bottom` alone do not stretch the height — taffy needs it explicit
    ///
    /// `inset-y-2` is `top: 0.5rem; bottom: 0.5rem` with no authored
    /// `height`, and in a real browser that is enough: CSS's absolute-
    /// positioning algorithm computes an auto height as the containing
    /// block's own height minus `top` minus `bottom`. Measured live against
    /// this port (`row_layout::sidebar_peek`): `.absolute().top(8px).bottom(8px)`
    /// with no `.h()` call renders a box **2px tall** — exactly
    /// [`BORDER_WIDTH`] doubled, i.e. taffy computed an auto height from the
    /// (empty) *content*, the same shrink-to-fit answer it would give a
    /// `position: relative` box, and never consulted `bottom` at all. So the
    /// height is computed here instead, from [`SidebarPeek::content_height`]
    /// — the quantity a real browser's own containing block would report —
    /// and only `top` is set; `bottom` is dropped rather than kept
    /// redundant, since an explicit height already fixes the box's own
    /// extent and a taffy that silently preferred one input over the other
    /// would be a second thing to go stale against.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let mut shell = div().flex().flex_col().min_h(px(0.0));

        shell = if matches!(self.state, PeekState::Docked) {
            shell.w_full().h_full()
        } else {
            let mut card = shell
                .absolute()
                .top(PEEK_MARGIN)
                .h(self.content_height - PEEK_MARGIN * 2.0)
                .w(self.peek_width)
                .max_w(self.viewport_width - MAX_WIDTH_GAP)
                .overflow_hidden()
                .rounded(theme.radius_xl.value())
                .border(BORDER_WIDTH)
                .border_color(theme.border)
                .bg(theme.popover)
                .text_color(theme.popover_foreground)
                .shadow_lg();
            card = if self.is_right {
                card.right(self.edge_offset())
            } else {
                card.left(self.edge_offset())
            };
            card
        };

        if let Some(filler) = self.filler() {
            shell = shell.child(filler);
        }

        anchors.root(AnchorId::from(ID_ROOT), shell)
    }
}

#[cfg(test)]
mod tests {
    use super::{
        BORDER_WIDTH, CONTENT_SIZED, LINE_SIZED, MAX_WIDTH_GAP, OFFSCREEN_GAP, PEEK_MARGIN,
        PeekState, SidebarPeek,
    };
    use gpui::px;

    #[test]
    fn every_length_is_the_measured_value() {
        assert_eq!(PEEK_MARGIN, px(8.0)); // inset-y-2 / left-2 / right-2
        assert_eq!(OFFSCREEN_GAP, px(16.0)); // +1rem
        assert_eq!(MAX_WIDTH_GAP, px(16.0)); // -1rem
        assert_eq!(BORDER_WIDTH, px(1.0));
    }

    #[test]
    fn neither_declaration_list_has_an_entry() {
        assert!(CONTENT_SIZED.is_empty());
        assert!(LINE_SIZED.is_empty());
    }

    #[test]
    fn the_fixture_is_the_docked_default() {
        let peek = SidebarPeek::fixture();
        assert_eq!(peek.state, PeekState::Docked);
        assert!(!peek.is_right);
        assert!(!peek.state.hidden());
    }

    #[test]
    fn only_docked_is_not_hidden() {
        assert!(!PeekState::Docked.hidden());
        assert!(PeekState::Closed.hidden());
        assert!(PeekState::Peeking.hidden());
    }

    /// `data-state`'s vocabulary is exactly the three words the React
    /// source computes: `hidden ? (peeking ? 'peeking' : 'closed') :
    /// 'docked'`.
    #[test]
    fn the_state_vocabulary_matches_the_source() {
        assert_eq!(PeekState::Docked.name(), "docked");
        assert_eq!(PeekState::Closed.name(), "closed");
        assert_eq!(PeekState::Peeking.name(), "peeking");
    }

    /// **Peeking sits at the margin; closed sits fully off past the gap** —
    /// the two-cell picture `edge_offset` exists to compute rather than
    /// transform.
    ///
    /// **Mutation, run:** swapping the `-` for a `+` before `self.peek_width`
    /// in `edge_offset`'s `Closed` arm moves the parked card *toward* the
    /// window instead of away from it — confirmed red (`left: 272px, right:
    /// -288px`) against this test's own arithmetic, duplicated independently
    /// below rather than called back into the function under test.
    #[test]
    fn peeking_and_closed_are_a_fixed_distance_apart() {
        let width = px(280.0);
        let peek = SidebarPeek {
            peek_width: width,
            ..SidebarPeek::fixture()
        };

        let peeking = SidebarPeek {
            state: PeekState::Peeking,
            ..peek
        };
        let closed = SidebarPeek {
            state: PeekState::Closed,
            ..peek
        };

        assert_eq!(peeking.edge_offset(), PEEK_MARGIN);
        assert_eq!(closed.edge_offset(), PEEK_MARGIN - width - OFFSCREEN_GAP);
        // Independently: the card is fully parked past its own width plus
        // clearance, measured from the on-screen resting edge.
        assert_eq!(
            peeking.edge_offset() - closed.edge_offset(),
            width + OFFSCREEN_GAP,
        );
        // And docked has no edge to speak of.
        assert_eq!(SidebarPeek::fixture().edge_offset(), px(0.0));
    }
}
