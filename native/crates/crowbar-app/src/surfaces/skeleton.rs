//! `--surface skeleton` — a loading placeholder, and the second P3.6 surface
//! with **no reference**.
//!
//! The default cell is a `SidebarSkeleton` chat row's leading icon block. It is
//! not a captured fixture: `<Skeleton>`'s only call site is the fallback of a
//! `<Suspense>` boundary whose subtree contains **no suspending source**, so the
//! fallback never mounts and a live count of `[data-slot="skeleton"]` was **0**
//! in every state including a fresh reload. The reasoning is at
//! `crowbar_ui::components::skeleton`; see also `native/mapping/skeleton.md`.
//!
//! **Every value the surface renders is the app's own compiled CSS**, so this is
//! `git-row-dir`'s precedent — rendered by the port, absent from the product —
//! rather than a guess. Here it is absent *by construction* rather than by the
//! fixture, which is the stronger form.
//!
//! # `ANCHORS.md` v1.9 does not apply, and that is checked rather than assumed
//!
//! The element is animated — `animation: skeleton 2s -1s infinite linear` — so
//! v1.9's "a snapshot has no way to say *when* it was taken" is the first thing
//! to rule out. It does not reach this surface: the only property in flight is
//! `background-position`, and the contract records no such field. `bg` is read
//! from `background-color`, which the keyframes never touch, and `bounds` are
//! the call site's. A capture here would be timing-independent in every
//! recorded field. See the component's module docs for the extractor line that
//! settles it.
//!
//! # What each axis can and cannot do here
//!
//! | Axis | Here |
//! |---|---|
//! | `--theme` | **real**: `bg-muted` is a different token in the two tables |
//! | `--width` | **real for `--call-site title` only** — it is the `flex-1` shape. The other two pin their own width |
//! | `--content` | **vacuous.** A skeleton paints no text — it is the *absence* of content |
//! | `--viewport-width` | **vacuous.** `skeleton.tsx` contains no `sm:` variant |
//!
//! # The state axis
//!
//! `loading` is the one flag whose word describes this surface exactly, and it
//! is still **unmodelled** — for `avatar`'s reason. A skeleton is *only ever*
//! the loading state; there is no resting rendering for a `loading` cell to
//! differ from, so driving the flag renders the same picture on both sides and
//! proves nothing.
//!
//! `empty` is the one that is real: the bare primitive pins neither axis, so it
//! has **zero area** and reports `visible: false`. It is the same cell
//! `--call-site none` reaches from the other direction, and the surface's test
//! asserts the two spellings of "nothing in it" agree — `avatar`'s precedent.
//! The remaining four have no original at all: `skeleton.tsx` has no interaction
//! rule of any kind.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::skeleton::{ALL_CALL_SITES, CallSite, Skeleton};
use crowbar_ui::components::{AnchorSink, skeleton};
use gpui::{AnyElement, IntoElement as _, ParentElement as _, Pixels, Styled as _, div, px};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "skeleton",
    root: skeleton::ID_SKELETON,
    unmodelled: &[
        // A skeleton *is* the loading state — see the module docs.
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // [`HOST_HEIGHT`] plus `CAPTION_HEIGHT`'s 29, with room. A floor rather
    // than a ceiling; this surface drives no height.
    min_window_height: 72,
    // A placeholder sits inside a sidebar row — never the window.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// The height of the row the placeholders sit in.
///
/// `sidebar-skeleton.tsx`'s rows are `flex h-9 items-center gap-2` — 36px. The
/// surface's number rather than the component's: a skeleton has no height of its
/// own, which is the whole point of [`CallSite`].
pub const HOST_HEIGHT: Pixels = px(36.0);

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--call-site`: which of `sidebar-skeleton.tsx`'s shapes this is. It
    /// carries the box **and** the radius, because every live shape overrides
    /// the primitive's `rounded-sm`.
    pub call_site: CallSite,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            call_site: Skeleton::fixture().call_site,
        }
    }
}

impl Params {
    /// The placeholder this cell describes.
    #[must_use]
    pub fn skeleton(&self, cell: &Cell) -> Skeleton {
        Skeleton {
            // `empty` overrides the call site: §8.3's word is "a surface with
            // nothing in it", and a shape with an authored box is not that.
            call_site: if cell.has(StateFlag::Empty) {
                CallSite::None
            } else {
                self.call_site
            },
        }
    }
}

impl SurfaceParams for Params {
    fn accept(
        &mut self,
        option: &str,
        args: &mut dyn Iterator<Item = String>,
    ) -> Result<bool, ParseError> {
        match option {
            "--call-site" => self.call_site = parse_call_site(&value(args, option)?)?,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** Every live shape's height is its call site's `h-*`, and this
    /// command line sets no other.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(out, " · class {}", self.skeleton(cell).call_site.name());
        if cell.has(StateFlag::Empty) {
            out.push_str(
                " · empty: neither axis pinned, so the box has zero area and is not \
                 painted — the same cell --call-site none reaches",
            );
        }
        match self.skeleton(cell).call_site {
            CallSite::None => out.push_str(
                " · class: the bare primitive, the only cell that keeps its own \
                 rounded-sm, and no live call site renders it",
            ),
            CallSite::Title => out.push_str(" · flex-1: the one shape --width can move"),
            CallSite::Icon | CallSite::Meta => {}
        }
        out.push_str(
            " · no reference: the only call site is a <Suspense> fallback whose subtree \
             has no suspending source, so it never mounts",
        );
        out.push_str(" · the sheen is unmodelled and is outside every field the contract records");
    }

    /// The placeholder, inside the sidebar row it sits in.
    ///
    /// The row carries no anchor, so the snapshot holds exactly one record. It
    /// is a real row rather than a bare box because `--call-site title` is
    /// `flex-1` and has no width without one.
    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        div()
            .flex()
            .flex_row()
            .items_center()
            .h(HOST_HEIGHT)
            .w(px(f32::from(cell.width)))
            .child(self.skeleton(cell).render(theme, anchors))
            .into_any_element()
    }
}

/// A call site's `className` bundle.
///
/// **There is deliberately no numeric form**, the line P3.1 drew for
/// `--class-radius`: a knob may supply the same *input* both engines resolve,
/// never the reference's *output*.
fn parse_call_site(raw: &str) -> Result<CallSite, ParseError> {
    ALL_CALL_SITES
        .into_iter()
        .find(|site| site.name() == raw)
        .ok_or_else(|| {
            ParseError::Rejected(format!(
                "--call-site takes one of {}, not {raw}; it names the className bundle a \
                 call site merges, never a pixel value",
                names(ALL_CALL_SITES.into_iter().map(CallSite::name)),
            ))
        })
}

/// A vocabulary as one line, for a usage line and for a rejection.
fn names<I: Iterator<Item = &'static str>>(words: I) -> String {
    words.collect::<Vec<_>>().join(", ")
}

fn options() -> Vec<(String, String)> {
    [(
        "--call-site <name>".to_owned(),
        format!(
            "one of {} — the className bundle a call site merges, never a pixel value; it \
             carries the box and the radius, every live shape overriding the primitive's \
             rounded-sm [{}]",
            names(ALL_CALL_SITES.into_iter().map(CallSite::name)),
            Skeleton::fixture().call_site.name(),
        ),
    )]
    .into_iter()
    .collect()
}
