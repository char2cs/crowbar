//! How a component opts its elements into being *pressable* without depending
//! on the app.
//!
//! The sibling of [`crate::anchor`], and deliberately the same shape. That
//! module exists because a component cannot call `crowbar-driver` (the driver
//! depends on this crate, not the other way round), so it takes a sink and the
//! binary decides which one to hand it. Interaction has exactly the same
//! problem: `crowbar-ui` is the design system and cannot know that clicking a
//! workspace row means "make it the active scope" — that is `crowbar-app`'s
//! knowledge, and `crowbar-app` depends on this crate.
//!
//! # Why this exists at all
//!
//! Until S1a, **nothing in `crowbar-ui` could be clicked.** `grep -rc on_click
//! crates/crowbar-ui/src/` returned zero across the whole crate. Every surface
//! was a value struct with one `render` method producing geometry for the
//! anchor differ, because the retired component-parity method measured
//! anchored boxes and a box does not need a listener to be measured. So "26 of
//! the 28 sidebar components are already ported" was true of their appearance
//! and false of their behaviour, and composing them as they stood produced a
//! pixel-accurate sidebar that could not respond to a single click.
//!
//! # The one property this must have
//!
//! **Attaching interaction must not change layout.** `crate::anchor`'s own
//! docs give the reason — *"an oracle that measures a differently-shaped tree
//! measures nothing"* — and the frozen corpus (§3.2 of the slice-method spec)
//! is the regression net over 69 already-verified components. If the shipping
//! build's element tree differed from the measured one, every one of those
//! verdicts would be about a tree the app never renders.
//!
//! Two things secure it:
//!
//! * **No wrapper element.** [`ActionSink::clickable`] takes a [`Div`] and
//!   returns a [`Div`]. It does not nest, so the tree's shape is identical
//!   whichever sink is used.
//! * **`on_mouse_up`, not `on_click`.** `on_click` lives on
//!   `StatefulInteractiveElement` and needs `.id()`, which turns a `Div` into
//!   a `Stateful<Div>` — a different type, which would have forced either a
//!   wrapper or a widened [`crate::anchor::AnchorSink`]. `on_mouse_down` and
//!   `on_mouse_up` are on plain `InteractiveElement`, so the element that
//!   comes out is the element that went in.
//!
//! [`Inert`] is the identity, and is what the oracle build and every existing
//! surface test use.

use gpui::{App, Div, InteractiveElement as _, MouseButton, MouseUpEvent, SharedString, Window};

/// What was pressed: which part of a surface, and which record it belongs to.
///
/// Split into two fields rather than one opaque string because the app
/// dispatches on `part` and looks up on `subject`, and joining them into
/// `"row:w-123"` would mean every call site re-parsing what it just built.
///
/// `crowbar-ui` deliberately does **not** model what a part *means*. A
/// workspace row knows it has a primary target and a disclosure triangle; it
/// does not know that pressing one changes the active scope. That is the same
/// boundary [`crate::anchor::AnchorId`] keeps: the component names its own
/// parts, and the layer above assigns them meaning.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct ActionId {
    /// Which part of the surface — `"row"`, `"disclosure"`, `"tab"`.
    pub part: SharedString,
    /// The record it belongs to: a workspace id, a repo id, a tab name.
    /// Empty when the surface renders exactly one of this part.
    pub subject: SharedString,
}

impl ActionId {
    /// A part that belongs to a specific record.
    #[must_use]
    pub fn new(part: impl Into<SharedString>, subject: impl Into<SharedString>) -> Self {
        Self {
            part: part.into(),
            subject: subject.into(),
        }
    }

    /// A part a surface renders exactly one of.
    #[must_use]
    pub fn part(part: impl Into<SharedString>) -> Self {
        Self {
            part: part.into(),
            subject: SharedString::default(),
        }
    }
}

/// How a component makes one of its boxes pressable.
pub trait ActionSink {
    /// Make `element` respond to a primary-button release as `id`.
    ///
    /// Returns a [`Div`], not an [`gpui::AnyElement`], so a call site can keep
    /// styling it and can still hand it to
    /// [`crate::anchor::AnchorSink::boxed`] afterwards — the two sinks compose
    /// in either order because neither changes the element's type or nests it.
    #[must_use]
    fn clickable(&self, id: ActionId, element: Div) -> Div;
}

/// The identity sink: every method returns its element untouched.
///
/// This is what the oracle build and every surface unit test use, and what
/// makes the measured tree and the shipping tree the same tree.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Inert;

impl ActionSink for Inert {
    fn clickable(&self, _id: ActionId, element: Div) -> Div {
        element
    }
}

/// A sink that routes every press to one handler.
///
/// The app builds one of these per rendered surface tree, closing over
/// whatever it needs to dispatch — usually a `WeakEntity` of the store.
pub struct Dispatch<F>(pub F)
where
    F: Fn(&ActionId, &mut Window, &mut App) + Clone + 'static;

impl<F> ActionSink for Dispatch<F>
where
    F: Fn(&ActionId, &mut Window, &mut App) + Clone + 'static,
{
    fn clickable(&self, id: ActionId, element: Div) -> Div {
        let handler = self.0.clone();
        element.on_mouse_up(
            MouseButton::Left,
            move |_event: &MouseUpEvent, window, cx| handler(&id, window, cx),
        )
    }
}
