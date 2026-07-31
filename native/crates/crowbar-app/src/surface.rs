//! What a measurable surface is, and how one registers without editing a list.
//!
//! # The shape, and why it is two traits and a plain struct
//!
//! A surface is [`Surface`] — **data**, not a trait object: a name, a root
//! anchor, the §8.3 flags its original does not have, and two function pointers.
//! Being data is what lets it live in a `static`, which is what lets `build.rs`
//! collect it without anybody writing a list (see that file for the mechanism
//! and its costs).
//!
//! The *behaviour* is [`SurfaceParams`], which each surface implements for its
//! own parameter struct. Three methods, all of them things only that surface
//! knows: which options it takes, what its caption should say, and what element
//! tree its cell means.
//!
//! [`Params`] is the object-safe half, and nothing implements it by hand — the
//! blanket impl below supplies it for anything that is `SurfaceParams`. It
//! exists for one reason: [`crate::row_surface::Cell`] is
//! `Clone + Debug + PartialEq + Eq`, those assertions are the archived gate's
//! evidence, and a `Box<dyn Trait>` is none of the four without help.
//!
//! # Where a surface's options live
//!
//! **In its own `Params` struct.** That is the rule for every surface added from
//! Phase 2 onwards, and it is what keeps four parallel worktrees off each
//! other's lines.
//!
//! The two Phase 1 surfaces are the exception and are not the pattern. Nine of
//! their parameters — `depth`, `show_directory`, `additions`, `git_status` and
//! the rest — are still fields on `Cell`, because the tests that name them are
//! the Phase 1 gate's evidence and moving the fields would rewrite assertions
//! that are the record of what converged. They stay put; their `Params` structs
//! are empty and read the cell. Nothing new goes there.

use std::any::Any;
use std::fmt::Debug;

use crowbar_ui::Theme;
use crowbar_ui::components::AnchorSink;
use gpui::AnyElement;

use crate::row_surface::{Cell, ParseError, StateFlag};

/// One measurable surface, as data.
///
/// Every field is `pub` and every one of them is read by `crowbar-app` and by
/// nothing else. A surface declares exactly one of these, in a `static` called
/// `SURFACE`, and `build.rs` finds it.
pub struct Surface {
    /// The word `--surface` takes, and the snapshot's `surface` field. The
    /// differ refuses to compare two snapshots that disagree on it.
    pub name: &'static str,
    /// The anchor every bound on this surface is reported relative to
    /// (`native/oracle/ANCHORS.md` §4).
    ///
    /// Only the snapshot path reads it, and that path is
    /// `#[cfg(any(feature = "driver", test))]` — so in a plain shipping build
    /// this is genuinely dead, exactly as `row_snapshot.rs` as a whole is. It
    /// lives here rather than there because it is a fact about the surface, and
    /// putting the two halves of `--surface` in two places is how they come to
    /// disagree.
    #[cfg_attr(not(any(feature = "driver", test)), allow(dead_code))]
    pub root: &'static str,
    /// The §8.3 flags whose cells **cannot fail**, because this surface's React
    /// original has no such state to disagree with.
    ///
    /// Driving one renders the resting surface on both sides, so it compares
    /// equal and proves nothing — which is why the binary names them on stderr
    /// rather than rendering them quietly.
    pub unmodelled: &'static [StateFlag],
    /// The **least** room below the window's top inset this surface is given, in
    /// whole logical px, caption included.
    ///
    /// A floor and not a ceiling (P2.5). The window follows whatever height the
    /// cell drives the surface to — see [`SurfaceParams::driven_height`] — and
    /// this is what it falls back to when the cell drives none: a surface whose
    /// height is decided by its own content, like a menu popup, has no number
    /// for the window to follow, so it keeps the one it was measured at.
    ///
    /// Per surface because they differ by an order of magnitude — a row is one
    /// line, a menu popup is a column of them — and because the two Phase 1
    /// surfaces' number is part of a configuration that has already converged:
    /// `git-status-row` and `file-tree-row` drive no height, so their window is
    /// exactly the one the archived gate runs were taken at, by construction
    /// rather than by arithmetic that happens to land on it.
    ///
    /// A `u16` rather than the `f32` the window arithmetic works in, so that
    /// `Cell` can stay `Eq`: `f32` is not, and a window height with a fractional
    /// part is not a thing anyone needs.
    pub min_window_height: u16,
    /// Whether this surface's original **fills its window**, so that the surface
    /// width and the viewport width are one quantity (P2.12).
    ///
    /// The width counterpart of [`Self::min_window_height`], and the declaration
    /// that decides the driver's **horizontal inset**: a full-bleed surface is
    /// drawn flush with the window's left edge, an ordinary one at
    /// [`crate::row_surface::INSET_X`]. See that constant for why the inset is
    /// non-zero in the ordinary case and what a full-bleed surface gives up by
    /// dropping it.
    ///
    /// Declared by the surface rather than passed on the command line for the
    /// same reason `min_window_height` is: it is a fact about the thing being
    /// ported, not a choice a caller makes per run. `resizable`'s reference is
    /// the IDE shell root in `web/src/components/layout/ide-shell.tsx` — measured
    /// live at `innerWidth` 1200 with a 1200×800 `resize-group` — so its surface
    /// width *is* its viewport width, and a caller who could get that wrong is a
    /// caller who can emit a cell that compares against nothing.
    ///
    /// **What it is not.** It is not "this surface is always drawn at the
    /// viewport width": `--width` still moves independently, and driving
    /// `resizable` at 600px in an 800px viewport is a legitimate synthetic cell
    /// that measures the same flex division at another width. What the flag says
    /// is that the surface takes **no horizontal inset**, so a cell that does set
    /// the two equal is drawable rather than refused.
    pub full_bleed: bool,
    /// This surface's own options, as `(spelling, description)`, for `--help`.
    ///
    /// A function rather than a `&'static [_]` because a vocabulary is sometimes
    /// generated from the type that owns it, and a usage that restated one by
    /// hand could advertise a word the parser does not accept.
    pub options: fn() -> Vec<(String, String)>,
    /// A fresh parameter bag, at this surface's own defaults.
    pub params: fn() -> Box<dyn Params>,
}

/// The name, and nothing else.
///
/// A derived `Debug` would print two function-pointer addresses into every
/// `Cell`'s debug output, where they are noise that changes between builds — and
/// `Cell`'s `Debug` is what a failing assertion prints.
impl Debug for Surface {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_tuple("Surface").field(&self.name).finish()
    }
}

/// **By name.** The registry guarantees names are unique — `surface.rs`'s own
/// test asserts it — so the name is an identity rather than a label.
///
/// Derived equality would compare the two function-pointer fields, which rustc
/// warns about for good reason: the address of the same function can differ
/// between codegen units and two different functions can be merged onto one
/// address. Either direction would make `Cell == Cell` answer a question about
/// the linker instead of about the picture.
impl PartialEq for Surface {
    fn eq(&self, other: &Self) -> bool {
        self.name == other.name
    }
}

impl Eq for Surface {}

impl Surface {
    /// Whether this surface's original has no such state, so the cell cannot
    /// fail.
    #[must_use]
    pub fn unmodelled(&self, flag: StateFlag) -> bool {
        self.unmodelled.contains(&flag)
    }

    /// The surface a `--surface` word names.
    ///
    /// # Errors
    ///
    /// [`ParseError::Rejected`], listing every registered name — "invalid" is
    /// not something anyone can act on.
    pub fn parse(raw: &str) -> Result<&'static Self, ParseError> {
        crate::surfaces::ALL
            .iter()
            .copied()
            .find(|surface| surface.name == raw)
            .ok_or_else(|| {
                ParseError::Rejected(format!(
                    "--surface takes one of {}, not {raw}",
                    Self::names().join(", "),
                ))
            })
    }

    /// Every registered surface's name, in registry order.
    #[must_use]
    pub fn names() -> Vec<&'static str> {
        crate::surfaces::ALL
            .iter()
            .map(|surface| surface.name)
            .collect()
    }
}

/// What a surface does with a cell: take its own options, describe them, and
/// turn the cell into an element tree.
///
/// The one trait a new surface writes. Implement it for a struct that derives
/// `Clone + Debug + PartialEq`, and [`Params`] follows.
pub trait SurfaceParams: Clone + Debug + PartialEq + 'static {
    /// Consumes one command-line option, pulling its value out of `args` if it
    /// takes one.
    ///
    /// `Ok(false)` means "not mine" and lets the shared parser carry on to its
    /// rejection. Returning `Ok(true)` for an option another surface also
    /// spells is harmless: only the selected surface's bag is ever asked.
    ///
    /// # Errors
    ///
    /// [`ParseError::Rejected`] for a missing or out-of-vocabulary value.
    fn accept(
        &mut self,
        option: &str,
        args: &mut dyn Iterator<Item = String>,
    ) -> Result<bool, ParseError>;

    /// The height **this cell drives the surface to**, in whole logical px, or
    /// `None` where this surface has no option that sets one.
    ///
    /// This is what makes the driver's window follow the surface (P2.5). Before
    /// it, every surface's window was a fixed number and each height option was
    /// *capped* below it — `--shell-height 1..=160` on `resizable`,
    /// `--height 1..=640` on `sidebar-carousel` — which was sound reasoning
    /// applied to the wrong side of the equation: it kept the surface out of the
    /// window's clip by making the live IDE shell's own 1119px unreachable, so
    /// the only real reference could not be driven at all.
    ///
    /// `None` is not "zero". It is "nothing on this command line decides how
    /// tall this is", which is the honest answer for a row (one line) and for a
    /// menu popup (a column of rows whose height is its content's). Those keep
    /// [`Surface::min_window_height`], and that is also what pins the two
    /// Phase 1 surfaces' window to the geometry their archived runs were taken
    /// at.
    ///
    /// Whole px, and `u16`, for the reason [`Surface::min_window_height`] is:
    /// this reaches `Cell`, which has to stay `Eq`.
    fn driven_height(&self, cell: &Cell) -> Option<u16>;

    /// Appends this surface's own half of the caption.
    ///
    /// The shared half — surface, widths, theme, content, flags, depth — is
    /// already there. What belongs here is what only this surface renders, and
    /// **only where it is honoured**: a caption that announced a parameter the
    /// surface has no prop for would be describing something nothing painted.
    fn describe(&self, cell: &Cell, out: &mut String);

    /// The element tree this cell means.
    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement;
}

/// The object-safe half of [`SurfaceParams`], plus the three methods that make
/// `Box<dyn Params>` behave like the value it holds.
///
/// **Nothing implements this by hand.** The blanket impl below covers every
/// [`SurfaceParams`], and implementing it directly would be a way to have a
/// parameter bag that is not comparable to itself.
pub trait Params: Debug {
    /// See [`SurfaceParams::accept`].
    ///
    /// # Errors
    ///
    /// See [`SurfaceParams::accept`].
    fn accept(
        &mut self,
        option: &str,
        args: &mut dyn Iterator<Item = String>,
    ) -> Result<bool, ParseError>;

    /// See [`SurfaceParams::driven_height`].
    fn driven_height(&self, cell: &Cell) -> Option<u16>;

    /// See [`SurfaceParams::describe`].
    fn describe(&self, cell: &Cell, out: &mut String);

    /// See [`SurfaceParams::render`].
    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement;

    /// A boxed clone, so `Cell` can stay `Clone`.
    #[must_use]
    fn clone_params(&self) -> Box<dyn Params>;

    /// Whether `other` is the same surface's bag holding the same values, so
    /// `Cell` can stay `PartialEq`.
    ///
    /// Two *different* surfaces' bags are never equal, whatever they hold —
    /// which is right: they describe different pictures.
    fn same(&self, other: &dyn Params) -> bool;

    /// The downcast target [`Params::same`] needs.
    #[must_use]
    fn as_any(&self) -> &dyn Any;
}

impl<T: SurfaceParams> Params for T {
    fn accept(
        &mut self,
        option: &str,
        args: &mut dyn Iterator<Item = String>,
    ) -> Result<bool, ParseError> {
        <T as SurfaceParams>::accept(self, option, args)
    }

    fn driven_height(&self, cell: &Cell) -> Option<u16> {
        <T as SurfaceParams>::driven_height(self, cell)
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        <T as SurfaceParams>::describe(self, cell, out);
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        <T as SurfaceParams>::render(self, cell, theme, anchors)
    }

    fn clone_params(&self) -> Box<dyn Params> {
        Box::new(self.clone())
    }

    fn same(&self, other: &dyn Params) -> bool {
        other
            .as_any()
            .downcast_ref::<T>()
            .is_some_and(|other| self == other)
    }

    fn as_any(&self) -> &dyn Any {
        self
    }
}

/// A boxed [`Params`] that behaves like the value inside it.
///
/// A newtype rather than a bare `Box<dyn Params>` on `Cell`, because `Cell`
/// derives `Clone + Debug + PartialEq + Eq` and its tests — the archived gate's
/// evidence — are written against all four. `Box<dyn Trait>` has none of them
/// without help, and the help has to land somewhere; a newtype is the one place
/// it can land without touching `Box`'s own impls or `Cell`'s derives.
///
/// `Deref` keeps every call site reading as though it were the box.
pub struct SurfaceOptions(Box<dyn Params>);

impl SurfaceOptions {
    /// Wraps a surface's fresh parameter bag.
    #[must_use]
    pub fn new(params: Box<dyn Params>) -> Self {
        Self(params)
    }
}

impl std::ops::Deref for SurfaceOptions {
    type Target = dyn Params;

    fn deref(&self) -> &Self::Target {
        self.0.as_ref()
    }
}

impl std::ops::DerefMut for SurfaceOptions {
    fn deref_mut(&mut self) -> &mut Self::Target {
        self.0.as_mut()
    }
}

impl Clone for SurfaceOptions {
    fn clone(&self) -> Self {
        Self(self.0.clone_params())
    }
}

impl Debug for SurfaceOptions {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        self.0.fmt(f)
    }
}

/// Two bags are equal when they are the **same surface's** bag holding the same
/// values. Two different surfaces' bags are never equal, whatever they hold —
/// which is right: they describe different pictures, and without it two empty
/// bags on two surfaces would compare equal.
impl PartialEq for SurfaceOptions {
    fn eq(&self, other: &Self) -> bool {
        self.0.same(other.0.as_ref())
    }
}

impl Eq for SurfaceOptions {}

/// Reads an option's value, or says which option was left dangling.
///
/// Shared with the top-level parser's own `value`, because a surface's options
/// have to reject a missing value in exactly the same words.
///
/// # Errors
///
/// [`ParseError::Rejected`] when `args` is exhausted.
pub fn value(args: &mut dyn Iterator<Item = String>, option: &str) -> Result<String, ParseError> {
    args.next()
        .ok_or_else(|| ParseError::Rejected(format!("{option} needs a value")))
}

/// A whole number of logical pixels, or a rejection naming what was given.
///
/// # Errors
///
/// [`ParseError::Rejected`] when `raw` is not a `u16`.
pub fn pixels(raw: &str, option: &str) -> Result<u16, ParseError> {
    raw.parse()
        .map_err(|_| ParseError::Rejected(format!("{option} takes a whole number, not {raw}")))
}

/// A zero-based index, or a rejection naming what was given.
///
/// # Errors
///
/// [`ParseError::Rejected`] when `raw` is not a `usize`.
pub fn index(raw: &str, option: &str) -> Result<usize, ParseError> {
    raw.parse()
        .map_err(|_| ParseError::Rejected(format!("{option} takes a row index, not {raw}")))
}

#[cfg(test)]
mod tests {
    use super::Surface;
    use crate::row_surface::{Cell, ParseError, StateFlag};

    /// The registry is populated, and every surface in it is distinct in the two
    /// fields a snapshot carries.
    ///
    /// A repeated `name` would make `--surface` ambiguous; a repeated `root`
    /// would let a snapshot claim one surface while being anchored to another's
    /// origin, which offsets every bound by a constant *and* passes the differ's
    /// surface check.
    #[test]
    fn every_registered_surface_has_its_own_name_and_root() {
        let all = crate::surfaces::ALL;
        assert!(all.len() >= 3, "{:?}", Surface::names());

        for field in [
            all.iter().map(|s| s.name).collect::<Vec<_>>(),
            all.iter().map(|s| s.root).collect::<Vec<_>>(),
        ] {
            let mut sorted = field.clone();
            sorted.sort_unstable();
            let before = sorted.len();
            sorted.dedup();
            assert_eq!(sorted.len(), before, "{field:?}");
        }
    }

    /// `build.rs` sorts by module name, so the registry order is stable across
    /// machines — which is what makes `--help`'s output diffable.
    ///
    /// **This is the one assertion a new surface has to touch**, and it is
    /// deliberately not "adding a file is enough": the whole value of it is that
    /// the generated list is *enumerated by hand* somewhere, so a surface that
    /// appeared or vanished by accident — a stray `.rs` under `src/surfaces/`, a
    /// `build.rs` that stopped globbing — fails here rather than being noticed
    /// when a matrix run comes back with nothing to compare.
    #[test]
    fn the_registry_is_sorted_and_holds_every_surface() {
        assert_eq!(
            Surface::names(),
            [
                "dropdown-menu",
                "file-tree-row",
                "git-status-row",
                "resizable",
                "sidebar-carousel",
            ],
        );
    }

    /// The lookup is the registry's, and the rejection lists it — because
    /// "invalid" is not something anyone can act on.
    #[test]
    fn the_lookup_names_every_registered_surface_when_it_fails() {
        for name in Surface::names() {
            assert_eq!(Surface::parse(name).expect("registered").name, name);
        }

        let Err(ParseError::Rejected(complaint)) = Surface::parse("menu") else {
            panic!("`menu` is not a surface");
        };
        for name in Surface::names() {
            assert!(complaint.contains(name), "{complaint}");
        }
    }

    /// Every surface hands out a bag that compares equal to another fresh one of
    /// its own, and **never** to another surface's — which is what stops two
    /// cells on different surfaces comparing equal because both bags are empty.
    #[test]
    fn a_bag_is_equal_to_its_own_kind_and_to_no_other() {
        let bag = |surface: &'static Surface| super::SurfaceOptions::new((surface.params)());

        for (position, one) in crate::surfaces::ALL.iter().enumerate() {
            assert_eq!(bag(one), bag(one), "{}", one.name);
            for other in &crate::surfaces::ALL[position + 1..] {
                // Two *empty* bags on two surfaces must not compare equal, or a
                // `Cell` on one surface would equal a `Cell` on another.
                assert_ne!(bag(one), bag(other), "{}/{}", one.name, other.name);
            }
        }
    }

    /// A clone is equal to its original, and stays equal after the original has
    /// been driven — the property `Cell: Clone` rests on.
    #[test]
    fn a_cloned_bag_is_equal_to_its_original() {
        let cell = Cell::default();
        assert_eq!(cell.clone(), cell);

        let driven = Cell::parse(
            ["--surface", "dropdown-menu", "--min-width", "200"]
                .iter()
                .map(|arg| (*arg).to_owned()),
        )
        .expect("well-formed");
        assert_eq!(driven.clone(), driven);
        assert_ne!(driven, Cell::default());
    }

    /// Every surface's unmodelled list is a subset of the §8.3 vocabulary, and
    /// no surface claims *every* flag is unmodelled — a surface with no real
    /// state axis at all would be one whose whole matrix cannot fail.
    #[test]
    fn no_surface_declares_its_entire_state_axis_unmodelled() {
        for surface in crate::surfaces::ALL {
            let real = crate::row_surface::ALL_FLAGS
                .iter()
                .filter(|flag| !surface.unmodelled(**flag))
                .count();
            assert!(real > 0, "{}", surface.name);
            assert!(surface.unmodelled(StateFlag::Loading), "{}", surface.name);
            assert!(surface.unmodelled(StateFlag::Error), "{}", surface.name);
        }
    }
}
