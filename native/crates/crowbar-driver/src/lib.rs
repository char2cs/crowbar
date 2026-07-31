#![forbid(unsafe_code)]

//! `crowbar-driver` — the in-process test driver: element-tree extractor, input
//! injector and MCP server over stdio (§10.4, D7).
//!
//! Item P1.2 builds the first of those: the **gpui half of the anchored
//! geometry oracle**, `native/oracle/ANCHORS.md` (written against **v1.1**).
//! Given a running window whose elements carry anchor ids, it produces a v1
//! snapshot — the same JSON shape the React extractor produces — reflecting the
//! tree *after* layout, so the bounds are real.
//!
//! Dependency contract (§4.2): everything.
//!
//! **Feature-gated.** This crate is an *optional* dependency of `crowbar-app`
//! behind that binary's `driver` feature, so a default build links none of it.
//! Build the oracle against `--release --features driver` so the optimisation
//! level matches shipping.
//!
//! The driver adds a control channel; **it must not alter rendering.** If it
//! ever does, that is a bug of the highest severity — every oracle result taken
//! while it was true is void. `src/element.rs` documents the three properties
//! that keep it true.
//!
//! # Using it
//!
//! ```ignore
//! crowbar_driver::install(cx);            // once, at startup
//!
//! anchor_root("row", div().bg(…)          // the frame boundary and the origin
//!     .child(anchor("row-icon", div()…))  // a box anchor
//!     .child(anchor_text("row-name", name)))
//!
//! let snapshot = registry(cx)?.snapshot("git-status-row", state, "row")?;
//! println!("{}", snapshot.to_json()?);
//! ```
//!
//! # What gpui can and cannot supply
//!
//! Stated here rather than in a report, because the next person to read this
//! crate needs it more than the person who commissioned it did.
//!
//! * **`bounds`** — exact. The border box gpui laid out, in logical pixels,
//!   shifted so the root anchor sits at the origin (§4).
//! * **`bg` / `fg` / `border.color`** — exact for solid fills, always eight hex
//!   digits (v1.1 rule 2). A **gradient or pattern fill has no v1
//!   representation**: `bg` is *omitted* for one, never `#00000000` and never
//!   `null`, because a gradient is not transparent. `bg` is required, so such a
//!   snapshot fails to load by name, which is loud and diagnosable — a wrong
//!   value that parses would be worse.
//! * **`text`, `text_width`, `clipped`** — exact. `text_width` is the shaped
//!   advance width of the *whole* string, which is the one thing `bounds`
//!   cannot substitute for.
//! * **`font.weight`** — carried through, never clamped (v1.1 rule 7 accepts
//!   the full CSS 1–1000 range, so a variable-font weight must not be rewritten
//!   into a neighbouring hundred).
//! * **`font.family`** — this is the **declared** family, not the family the
//!   platform resolved to. gpui exposes `Font { family }` and a `FontId`, and
//!   no way back from the `FontId` to a name, so a style that inherits macOS's
//!   `.SystemUIFont` reports that string and the DOM side will not. Anchored
//!   surfaces must name a family explicitly, or this field must be dropped from
//!   the contract.
//! * **`font.line_height`** — gpui snaps line height to the **device** pixel
//!   grid (`Window::pixel_snap`), so at 2× it is a multiple of 0.5 px where CSS
//!   is continuous. Inside §5's ±0.5 px, but it is a systematic difference, not
//!   noise.
//! * **`content_sized`** *(v1.5)* — **declared by the caller, never detected
//!   here.** gpui `ceil()`s a text run's max-content width
//!   (`elements/text.rs`), so a content-sized box is always a whole number of
//!   logical pixels and the DOM's fraction is a target this engine cannot hit.
//!   The contract models that by comparing such a box against
//!   `ceil(reference)`, which needs the anchor to say it is content-sized:
//!   `Declared::content_sized`, through `anchor_declared` /
//!   `anchor_text_declared`. Guessing it from `width: None` plus a text child
//!   is falsifiable by flex-grow, and a wrong guess is invisible — it opens a
//!   blind spot or invents a delta and reports neither.
//! * **`line_sized`** *(v1.6)* — **declared by the caller as well**, and for a
//!   sharper reason. gpui snaps a line height to the *device* grid where
//!   `WebKit` floors it to a whole logical pixel, so the two engines' boxes
//!   round the same fractional line box in different directions and can land a
//!   full pixel apart. The contract models that by comparing such a box's
//!   `bounds.h` against the reference's `font.line_height` — which needs the
//!   anchor to say its height *is* its line box: `Declared::line_sized`. A
//!   detector ("one text child, no explicit height") would be plainly wrong on
//!   the gate row's own badge, whose `sm:h-4` pins a 16px border box around a
//!   13.33px line box, and would invent a 2.67px delta out of nothing.
//! * **`radius` / `border.w`** — gpui carries four corners and four edges. v1
//!   carries one of each, so these are the **top-left corner** and the **top
//!   edge**. A surface with asymmetric corners or edges is silently
//!   under-reported.
//! * **`visible`** — zero-area and clipped-away are both detected, and so is an
//!   anchor's own `display: none` / `visibility: hidden`. A `display: none`
//!   *ancestor* is detected implicitly, because `prepaint` never reaches the
//!   anchor and it is absent from the snapshot entirely. A `visibility: hidden`
//!   **ancestor** is a blind spot: it is laid out and prepainted, so a
//!   descendant anchor reports `visible: true`. Put the anchor on the element
//!   that carries the visibility.
//! * **`visible` and opacity** *(v1.7)* — an anchor at zero opacity, or under an
//!   **anchored** ancestor at zero opacity, reports `visible: false`. What that
//!   costs is worth stating exactly, because gpui makes the general case
//!   unreachable from here:
//!
//!   * The **element's own** opacity is exact. `Style::opacity` is a public
//!     field of the already-resolved style `AnchoredBox` captures, so it is read
//!     the same way `radius` and `border` are.
//!   * The **ancestor chain** is reconstructed by the driver, not read off the
//!     window. gpui accumulates ancestor opacity in `Window::element_opacity`,
//!     but (a) it is `pub(crate)`, with no public accessor anywhere in the
//!     crate, and (b) `with_element_opacity` is called from
//!     `Interactivity::paint` only — `Interactivity::prepaint` pushes the text
//!     style and the content mask and *not* the opacity. So at the instant the
//!     extractor runs the value does not exist yet, and at the instant it does
//!     it cannot be read. The driver therefore keeps its own chain in
//!     [`AnchorRegistry`], pushed by each [`AnchoredBox`] around its child's
//!     `prepaint`.
//!   * The consequence: **an unanchored ancestor's opacity is invisible.**
//!     `anchor("a", div().child(div().opacity(0.).child(anchor("b", …))))`
//!     reports `b` as visible, and so does an `opacity: 0` layer above the root
//!     anchor. Put the opacity on an anchor — the root anchor will do, since
//!     opacity changes no geometry and every bound in a snapshot is relative to
//!     that root either way.
//!   * Zero is **exact**, matching `oracleIsVisible`'s `parseFloat(…) === 0`,
//!     and it is tested per level rather than as a product so that an `f32`
//!     product cannot underflow into a disagreement.
//! * **Hover and active state** — `AnchoredBox` reads the element's *base*
//!   style. gpui applies `hover`/`active` refinements from runtime state that
//!   only exists once a hitbox has been hit, so a snapshot of a hovered state
//!   has to come from a component that takes the state as a prop.
//! * **Shadows, blur, vibrancy, compositing, paint order** — out of scope by
//!   `native/oracle/ANCHORS.md` §6, not by omission here.
//!
//! Say this plainly in any report that uses it: **this is a geometry-and-colour
//! oracle.** That is enough for the surfaces under strict parity, and it is not
//! enough for shadows.

mod color;
mod element;
mod leak;
mod record;
mod schema;

pub use element::{
    AnchorRegistry, AnchoredBox, AnchoredText, Declared, anchor, anchor_declared, anchor_root,
    anchor_text, anchor_text_declared, install, registry,
};
pub use record::{FontFacts, Paint, RawAnchor, TextFacts};
pub use schema::{
    Anchor, Border, Content, Flag, Font, Rect, SCHEMA, Snapshot, SnapshotError, SurfaceState, Theme,
};
