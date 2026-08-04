//! `scroll_area` — the component where the **wrap test came back split**, and
//! the seam is recorded rather than the verdict alone.
//!
//! The native half of `web/src/components/ui/scroll-area.tsx`, a set of Tailwind
//! class lists over `@base-ui/react`'s `ScrollArea`. It exports two things:
//! `ScrollArea` — a `Root` wrapping a `Viewport`, two `ScrollBar`s and a
//! `Corner` — and `ScrollBar`, a track holding a `Thumb`.
//!
//! # Wrap or build: **build**, and the seam evidence that decided it
//!
//! `native/vendor/gpui-component/src/scroll/` exists, so the first question was
//! whether to wrap it. The test is `ANCHORS.md`'s: a widget is
//! wrappable-*and-measurable* exactly when it lets the caller supply an
//! **element**, not merely a style, because [`AnchorSink::root`] takes a [`Div`]
//! **this crate holds** and a box we never hold cannot carry an anchor.
//!
//! The vendor answers that question **differently for its two halves**, which is
//! why the verdict needed both.
//!
//! ## The container seam exists, and it is the `focus_trap` shape
//!
//! `scroll/scrollable.rs:16` declares
//! `pub trait ScrollableElement: InteractiveElement + Styled + ParentElement + Element`,
//! whose `scrollbar(self, scroll_handle, axis) -> Self` is one `.child()` push
//! onto **the caller's own element** — `self` in, `Self` out, id and handle
//! intact. `overflow_y_scrollbar(self) -> Scrollable<Self>` is the
//! `Something<Self>` wrapper form, and `Scrollable<E>` even implements
//! `ParentElement` by forwarding `extend()` to the element it was handed.
//!
//! **A grep over member names would have missed both**, because the trait is
//! named `ScrollableElement` rather than `Scrollable*Ext` and is bounded on
//! `Element` rather than on `IntoElement`. So the honest answer to "is there a
//! seam" is **yes**, and it is exactly the shape the brief warned a name grep
//! would not find.
//!
//! ## It is the wrong half. The scrollbar takes no element, ever
//!
//! What this component needs from a vendor is the **scrollbar**, and that is the
//! half with no seam at all. `scroll/scrollbar.rs`:
//!
//! * `Scrollbar::new/horizontal/vertical` take a `&H: ScrollbarHandle + Clone`
//!   and nothing else; the remaining builders are an `ElementId`, a
//!   `ScrollbarShow` enum, a `Size<Pixels>` and a `ScrollbarAxis`. **Style
//!   knobs, no element.** Within that whole file the strings `AnyElement`,
//!   `ParentElement`, `impl Fn`, `dyn Fn` and `.child(` **do not appear** —
//!   measured, not read off the API docs.
//! * Its `request_layout` builds a bare `Style::default()` with
//!   `size.width = relative(1.)`, `size.height = relative(1.)` and calls
//!   `window.request_layout(style, None, cx)` — the `None` **is** the child
//!   `LayoutId` slice. So its layout box is the **whole parent**, not the
//!   `m-1 w-1.5` track React draws, and a `div()` wrapped around it to carry an
//!   anchor would compare a box whose bounds do not even coincide. That is the
//!   fake convergence `ANCHORS.md` exists to refuse, one step worse than the
//!   coincidence the brief describes.
//! * The **thumb** is `cx.paint_quad(fill(state.thumb_fill_bounds, …))` at
//!   `scrollbar.rs:804` — painted straight into the scene during `paint`, with
//!   no layout node anywhere. The driver reads **layout** bounds at prepaint, so
//!   there is nothing for it to read. `ANCHORS.md` v1.11's "an element that
//!   generates no box is not an anchor" applies to the port's own side.
//!
//! **Verdict: build.** The two boxes this surface anchors are `Root` and
//! `Viewport`, which are plain `div()`s either way; the one box a wrap would
//! have supplied is unanchorable and mis-shaped. Wrapping would have bought the
//! vendor's fade timing and drag handling — behaviour a snapshot cannot see —
//! at the cost of the only two things it can.
//!
//! # The scrollbars are **ported and unanchored**, and that is v1.8 *and* v1.11
//!
//! Not a shortcut: base-ui does not render them.
//! `node_modules/@base-ui/react/scroll-area/scrollbar/ScrollAreaScrollbar.js`:
//!
//! ```js
//! const shouldRender = keepMounted || !isHidden;
//! // …
//! if (!shouldRender) { return null; }
//! ```
//!
//! `scroll-area.tsx` never passes `keepMounted`, so it defaults to `false`, and
//! `isHidden` is that axis's overflow state. `ScrollAreaCorner` is the same
//! shape (`if (hiddenState.corner) return null`). **No overflow, no DOM node** —
//! confirmed on all three live instances, every one of which reports
//! `root.children.length === 1`.
//!
//! Two rules follow, and they point the same way:
//!
//! * **v1.11** — an element that generates no box is not an anchor, so in every
//!   cell this app can currently reach there is nothing to anchor.
//! * **v1.8** — a surface may declare its anchor set only when the set is a
//!   property of the *surface* rather than of the *cell*. Scrollbar presence is
//!   a function of content overflow, which is a cell property, so declaring them
//!   would make an honest no-overflow capture a **refusal** on every run.
//!
//! So [`ScrollBar`] and [`Corner`] exist here — the component genuinely has them
//! and §17 wants the picture — rendered under the same condition base-ui uses,
//! and carrying no anchor. This is `kbd`'s `KbdGroup` arrangement with a second
//! reason stacked on it.
//!
//! # What the oracle can and cannot see here
//!
//! `scrollFade`'s four `mask-*-from-…` gradients are `ANCHORS.md` §6 material —
//! there is no mask field on either side — and so is the scrollbars'
//! `transition-opacity delay-300`. The port carries [`FADE_SIZE`] and
//! [`GUTTER`] as data because they are real lengths, and the differ will neither
//! confirm nor deny the first.
//!
//! **`clipped` matters here and v1.12 is why.** The viewport's computed
//! `overflow` is `scroll` on the live element, so it genuinely hides what
//! overruns it — an anchored text run inside one *would* report `clipped`
//! honestly on both sides. This surface anchors no text, so nothing turns on it
//! today; it is written down because a viewport is the one box in the tree whose
//! `overflow` is not `visible`.

use gpui::{AnyElement, Div, IntoElement as _, ParentElement as _, Pixels, Styled as _};
use gpui::{div, px};

use crate::anchor::AnchorSink;
use crate::theme::Theme;

/// The root anchor: `ScrollAreaPrimitive.Root`, the `size-full min-h-0` box
/// every other bound on this surface is reported relative to
/// (`native/oracle/ANCHORS.md` §4).
///
/// It carries no `data-slot` of its own in `scroll-area.tsx` — base-ui gives it
/// only `role="presentation"` and an inline `position: relative` — so the id is
/// this port's, spelled to match the three that do exist.
pub const ID_ROOT: &str = "scroll-area-root";

/// `ScrollAreaPrimitive.Viewport` — `data-slot="scroll-area-viewport"`, the
/// clipping box the call site's children sit in.
pub const ID_VIEWPORT: &str = "scroll-area-viewport";

/// The anchors whose boxes size to their own text
/// (`native/oracle/ANCHORS.md` v1.5).
///
/// **None**, and that is arithmetic rather than an omission. Neither box paints
/// text at all: the root is `size-full` inside whatever the call site gave it
/// and the viewport is `h-full` inside the root, so both widths are a
/// containing block's rather than a run's max-content width.
pub const CONTENT_SIZED: [&str; 0] = [];

/// The anchors whose box height is their own line box
/// (`native/oracle/ANCHORS.md` v1.6).
///
/// **None**, for the sharper half of the same reason: v1.6 refuses a
/// `line_sized` declaration on an anchor with no `font`, and neither of these
/// two paints a run.
pub const LINE_SIZED: [&str; 0] = [];

/// `--spacing`, Tailwind's stock `0.25rem` at a 16px root.
const SPACING: f32 = 4.0;

/// **0**, on both boxes, measured live: `borderTopWidth: "0px"`.
///
/// Preflight sets `border: 0 solid` on every element and neither class list puts
/// it back — the same reading `kbd` records, and the opposite of the one
/// `keybinding` records one module over. `ANCHORS.md` v1.3 ruling 2 then makes
/// `border.color` uncompared below this threshold, which is why the reference's
/// `#ffffff0f` on a zero-width border is not a delta.
pub const BORDER_WIDTH: Pixels = px(0.0);

/// **0**, measured live on both boxes.
///
/// The root authors no radius at all and the viewport's `rounded-[inherit]`
/// inherits it, so a call site that rounds its `ScrollArea` rounds both. Neither
/// live call site does, and modelling the inheritance as a parameter would be
/// modelling the call site rather than the primitive.
pub const RADIUS: Pixels = px(0.0);

/// `m-1` on a scrollbar — `calc(var(--spacing) * 1)`.
pub const SCROLLBAR_MARGIN: Pixels = px(SPACING);

/// `w-1.5` on a vertical scrollbar, `h-1.5` on a horizontal one —
/// `calc(var(--spacing) * 1.5)`.
pub const SCROLLBAR_THICKNESS: Pixels = px(SPACING * 1.5);

/// `rounded-full` on the thumb — **exactly `f32::MAX`**, not `px(9999.)`.
///
/// `switch`'s track records the measurement: Tailwind 4 compiles `rounded-full`
/// to `border-radius: calc(infinity * 1px)` and `WebKit` resolves that to
/// `3.4028234663852886e+38`. gpui's `rounded_full()` preset is `px(9999.)`, a
/// 3.4e38 delta on a field compared at ±0.5.
///
/// Nothing compares it **here**, because the thumb carries no anchor — but a
/// constant that is right only because nobody looks is a constant that will be
/// wrong the day somebody does.
pub const THUMB_RADIUS: Pixels = px(f32::MAX);

/// `bg-foreground/20` on the thumb — the `--foreground` token at 20%.
pub const THUMB_TINT_PERCENT: f32 = 20.0;

/// `data-has-overflow-y:pe-2.5` / `data-has-overflow-x:pb-2.5` under
/// `scrollbarGutter` — `calc(var(--spacing) * 2.5)`.
///
/// Applied **only** on the axis that overflows, which is the same condition that
/// mounts the scrollbar: the gutter exists to keep content out from under a
/// track that is actually there.
pub const GUTTER: Pixels = px(SPACING * 2.5);

/// `[--fade-size:1.5rem]` under `scrollFade` — 24px at a 16px root.
///
/// `ANCHORS.md` §6 material: the four `mask-*-from-…` rules it feeds have no
/// field on either side. Carried because it is a real length, and because a port
/// that dropped it silently would look identical to one that implemented it.
pub const FADE_SIZE: Pixels = px(24.0);

/// Which axes overflow, and therefore which scrollbars base-ui mounts.
///
/// **A cell property, never a surface one.** See the module docs: this is what
/// `ANCHORS.md` v1.8 forbids putting in a declared anchor set, and it is
/// modelled here so the port can draw the picture without claiming it.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Overflow {
    /// Whether the vertical scrollbar is mounted.
    pub y: bool,
    /// Whether the horizontal scrollbar is mounted.
    pub x: bool,
}

impl Overflow {
    /// Neither axis — the resting picture, and the only one all three live
    /// instances have ever been in.
    pub const NONE: Self = Self { x: false, y: false };

    /// Whether `ScrollAreaCorner` renders: base-ui hides it unless **both**
    /// tracks are there to meet at it.
    #[must_use]
    pub const fn has_corner(self) -> bool {
        self.x && self.y
    }
}

/// Which axis a [`ScrollBar`] runs along.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Orientation {
    /// `orientation="vertical"` — `w-1.5`, pinned to the inline end.
    #[default]
    Vertical,
    /// `orientation="horizontal"` — `h-1.5 flex-col`, pinned to the bottom.
    Horizontal,
}

/// One `<ScrollBar>`: the track, and the thumb inside it.
///
/// **Carries no anchor.** See the module docs for the v1.8 + v1.11 reason.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct ScrollBar {
    /// Which axis.
    pub orientation: Orientation,
}

impl ScrollBar {
    /// The track and its thumb.
    ///
    /// `opacity-0` is the resting state — the track only becomes visible under
    /// `data-hovering` or `data-scrolling`, neither of which is a §8.3 flag —
    /// so this paints at zero opacity exactly as the DOM does. `ANCHORS.md`
    /// v1.7 makes that `visible: false` on both sides, which is the honest
    /// reading of a track nobody is touching.
    fn render(self, theme: &Theme) -> AnyElement {
        let thumb = div()
            .relative()
            // `flex-1` — `flex: 1 1 0%`, which is gpui's `flex_1()` exactly.
            .flex_1()
            .rounded(THUMB_RADIUS)
            .bg(theme
                .foreground
                .mix(THUMB_TINT_PERCENT, crate::theme::Color::TRANSPARENT));

        let track = div().absolute().flex().m(SCROLLBAR_MARGIN).opacity(0.0);
        match self.orientation {
            Orientation::Vertical => track
                .top_0()
                .bottom_0()
                .right_0()
                .w(SCROLLBAR_THICKNESS)
                .child(thumb),
            Orientation::Horizontal => track
                .left_0()
                .right_0()
                .bottom_0()
                .h(SCROLLBAR_THICKNESS)
                .flex_col()
                .child(thumb),
        }
        .into_any_element()
    }
}

/// `ScrollAreaPrimitive.Corner` — the square where two tracks meet.
///
/// `scroll-area.tsx` gives it a `data-slot` and **no class list at all**, so it
/// paints nothing; base-ui sizes it inline from `cornerSize`. Modelled as the
/// empty box it is, and unanchored for [`ScrollBar`]'s reasons.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Corner;

impl Corner {
    /// The corner's box: the two tracks' thickness plus their margins.
    #[must_use]
    pub fn extent() -> Pixels {
        SCROLLBAR_THICKNESS + SCROLLBAR_MARGIN * 2.0
    }

    /// The corner's box. An associated function rather than a method: the type
    /// is a unit struct and there is nothing about *this* corner to read.
    fn render() -> AnyElement {
        div()
            .absolute()
            .bottom_0()
            .right_0()
            .w(Self::extent())
            .h(Self::extent())
            .into_any_element()
    }
}

/// The root, its viewport, and whatever the call site put inside it.
///
/// # Why the body is an **extent** rather than an element tree
///
/// `ScrollArea`'s children are the call site's, and every live one is a
/// different sub-UI: `workspace-tree` puts a tree in it, `git-panel` a diff
/// list, `autocomplete` a command list. None of that is this primitive, and a
/// port that reproduced one call site's contents would be measuring that call
/// site — the call `popover` makes about its `--body-height`, for the same
/// reason.
///
/// The **width and height are the call site's too**, and more completely than
/// popover's: `size-full min-h-0` means this box has no size of its own at all.
/// `workspace-tree` adds `flex-1` and gets `344 × 936`; the port takes those two
/// numbers as parameters rather than inventing a box the primitive never
/// authors.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct ScrollArea {
    /// The extent the call site's layout gives the root. `size-full` inside a
    /// `flex-1` column, measured live at 344.
    pub width: Pixels,
    /// The same, vertically. Measured live at 936.
    pub height: Pixels,
    /// Which scrollbars base-ui has mounted.
    pub overflow: Overflow,
    /// `scrollFade` — the four edge masks. §6: no field, either side.
    pub fade: bool,
    /// `scrollbarGutter` — [`GUTTER`] of padding on an overflowing axis.
    pub gutter: bool,
}

impl ScrollArea {
    /// The live `workspace-tree` area, which is the instance the reference was
    /// taken from: `344 × 936`, no overflow on either axis, neither prop set.
    ///
    /// `git-panel`'s is `344 × 920` and the command palette's `574 × 46`; all
    /// three are the same picture at different extents, which is exactly what
    /// makes the extent a parameter.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            width: px(344.0),
            height: px(936.0),
            overflow: Overflow::NONE,
            fade: false,
            gutter: false,
        }
    }

    /// The viewport's inline-end padding: [`GUTTER`] where `scrollbarGutter` is
    /// set **and** the vertical axis overflows, and nothing otherwise.
    #[must_use]
    pub fn gutter_end(&self) -> Pixels {
        if self.gutter && self.overflow.y {
            GUTTER
        } else {
            px(0.0)
        }
    }

    /// The viewport's block-end padding, on the same condition for the other
    /// axis.
    #[must_use]
    pub fn gutter_bottom(&self) -> Pixels {
        if self.gutter && self.overflow.x {
            GUTTER
        } else {
            px(0.0)
        }
    }

    /// `ScrollAreaPrimitive.Root`'s own box: `size-full min-h-0`, plus the
    /// `position: relative` base-ui writes inline.
    ///
    /// No background, no radius, no border — measured live, all three. It is a
    /// positioning context and a size, and nothing else.
    fn root(&self) -> Div {
        div().relative().w(self.width).h(self.height)
    }

    /// `ScrollAreaPrimitive.Viewport`: `h-full rounded-[inherit] outline-none`,
    /// filling the root on both axes.
    ///
    /// `h-full` is the only sizing utility in the class list; the width comes
    /// from the viewport being a block-level box in a block root, which measured
    /// live as the root's own 344. gpui's `div()` is a flex container, so the
    /// fill is spelled `.w_full()` — the same translation `popover`'s viewport
    /// makes for `size-full`.
    ///
    /// The overflow is `.overflow_hidden()` for `popover`'s reason: gpui's
    /// scrolling overflow lives on `StatefulInteractiveElement`, taffy zeroes
    /// the automatic minimum size for any non-visible overflow either way, and
    /// the live computed value is `scroll` — which clips identically for
    /// anything that is not being scrolled. See the module docs on `clipped`.
    fn viewport(&self) -> Div {
        div()
            .w_full()
            .h_full()
            .rounded(RADIUS)
            .overflow_hidden()
            .pr(self.gutter_end())
            .pb(self.gutter_bottom())
    }

    /// The call site's children, as the box they occupy.
    ///
    /// **Unanchored on purpose**, exactly as `popover`'s body is: it is not part
    /// of `scroll-area.tsx`, it stands in for whatever a call site rendered, and
    /// anchoring it would invite a comparison against an element that is a
    /// different thing on every call site.
    fn body() -> Div {
        div().w_full().h_full()
    }

    /// The element, with its two contract anchors.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let mut root = self
            .root()
            .child(anchors.boxed(ID_VIEWPORT.into(), self.viewport().child(Self::body())));

        // The order `scroll-area.tsx` writes them in. All three are
        // `position: absolute`, so the order is paint order and nothing else —
        // but a port that reordered them would be a port whose tree no longer
        // reads like the original's.
        if self.overflow.y {
            root = root.child(
                ScrollBar {
                    orientation: Orientation::Vertical,
                }
                .render(theme),
            );
        }
        if self.overflow.x {
            root = root.child(
                ScrollBar {
                    orientation: Orientation::Horizontal,
                }
                .render(theme),
            );
        }
        if self.overflow.has_corner() {
            root = root.child(Corner::render());
        }

        anchors.root(ID_ROOT.into(), root)
    }
}

#[cfg(test)]
mod tests {
    use super::{
        BORDER_WIDTH, CONTENT_SIZED, Corner, FADE_SIZE, GUTTER, ID_ROOT, ID_VIEWPORT, LINE_SIZED,
        Orientation, Overflow, RADIUS, SCROLLBAR_MARGIN, SCROLLBAR_THICKNESS, ScrollArea,
        ScrollBar, THUMB_RADIUS, THUMB_TINT_PERCENT,
    };
    use gpui::px;

    /// Every length, against the `calc(var(--spacing) * n)` the app's own
    /// Tailwind compiles the class to.
    #[test]
    fn every_length_is_the_compiled_spacing_multiple() {
        const STEP: f32 = 4.0;

        assert_eq!(SCROLLBAR_MARGIN, px(STEP)); // m-1
        assert_eq!(SCROLLBAR_THICKNESS, px(STEP * 1.5)); // w-1.5 / h-1.5
        assert_eq!(GUTTER, px(STEP * 2.5)); // pe-2.5 / pb-2.5
        assert_eq!(FADE_SIZE, px(24.0)); // [--fade-size:1.5rem]
    }

    /// **Both boxes carry a zero border and a zero radius**, which is the
    /// reading `kbd` shares and `keybinding` inverts — and `border.w` is
    /// compared *exactly*, so getting it the other way round fails every cell.
    #[test]
    fn neither_box_pays_a_border_or_a_radius_pixel() {
        assert_eq!(BORDER_WIDTH, px(0.0));
        assert_eq!(RADIUS, px(0.0));
        // The inverse reading, one module over, on the same element name.
        assert_ne!(BORDER_WIDTH, super::super::keybinding::BORDER_WIDTH);
    }

    /// **Neither anchor declares anything**, and both lists are empty for
    /// reasons that are arithmetic rather than oversight: no run to size to, and
    /// no `font` for v1.6 to compare against.
    #[test]
    fn neither_anchor_is_content_sized_or_line_sized() {
        assert_eq!(CONTENT_SIZED, [] as [&str; 0]);
        assert_eq!(LINE_SIZED, [] as [&str; 0]);
    }

    /// The two ids are distinct and namespaced — a repeat would make two anchors
    /// one record, which `ANCHORS.md` v1.8 ranks a refusal rather than a
    /// first-wins.
    #[test]
    fn every_anchor_id_is_distinct_and_namespaced() {
        assert_ne!(ID_ROOT, ID_VIEWPORT);
        assert!(ID_ROOT.starts_with("scroll-area-"));
        assert!(ID_VIEWPORT.starts_with("scroll-area-"));
    }

    /// **`rounded-full` is `f32::MAX`, not gpui's preset** — the trap `switch`
    /// measured, carried here so it is right the day something compares it.
    #[test]
    fn the_thumb_radius_is_the_webkit_infinity_and_not_the_gpui_preset() {
        assert_eq!(THUMB_RADIUS, px(f32::MAX));
        assert_ne!(THUMB_RADIUS, px(9999.0), "gpui's rounded_full() preset");
        assert_eq!(THUMB_RADIUS, super::super::switch::TRACK_RADIUS);
    }

    /// **The resting picture has no scrollbar at all**, which is base-ui's
    /// `shouldRender` and not a simplification — and the corner needs *both*.
    #[test]
    fn the_scrollbars_mount_only_on_an_overflowing_axis() {
        assert_eq!(Overflow::default(), Overflow::NONE);
        assert!(!Overflow::NONE.has_corner());
        assert!(!Overflow { y: true, x: false }.has_corner());
        assert!(!Overflow { y: false, x: true }.has_corner());
        assert!(Overflow { y: true, x: true }.has_corner());
    }

    /// The gutter is padding on **the axis that overflows**, and only where the
    /// prop asks — the same condition that mounts the track it makes room for.
    ///
    /// A control: the prop alone moves nothing, which is what makes the
    /// conjunction load-bearing rather than decorative.
    #[test]
    fn the_gutter_needs_both_the_prop_and_the_overflow() {
        let plain = ScrollArea::fixture();
        assert_eq!(plain.gutter_end(), px(0.0));
        assert_eq!(plain.gutter_bottom(), px(0.0));

        let asked = ScrollArea {
            gutter: true,
            ..ScrollArea::fixture()
        };
        assert_eq!(asked.gutter_end(), px(0.0), "the prop alone moves nothing");
        assert_eq!(asked.gutter_bottom(), px(0.0));

        let both = ScrollArea {
            gutter: true,
            overflow: Overflow { y: true, x: true },
            ..ScrollArea::fixture()
        };
        assert_eq!(both.gutter_end(), GUTTER);
        assert_eq!(both.gutter_bottom(), GUTTER);

        // And each axis answers for itself.
        let vertical_only = ScrollArea {
            gutter: true,
            overflow: Overflow { y: true, x: false },
            ..ScrollArea::fixture()
        };
        assert_eq!(vertical_only.gutter_end(), GUTTER);
        assert_eq!(vertical_only.gutter_bottom(), px(0.0));
    }

    /// The corner is the two tracks' thickness plus their margins: `6 + 4 + 4`.
    #[test]
    fn the_corner_is_the_track_and_its_two_margins() {
        assert_eq!(Corner::extent(), px(14.0));
        assert_eq!(
            Corner::extent(),
            SCROLLBAR_THICKNESS + SCROLLBAR_MARGIN * 2.0,
        );
    }

    /// The fixture is the live `workspace-tree` area, measured off the running
    /// app at a 1714px viewport.
    #[test]
    fn the_fixture_is_the_live_workspace_tree_area() {
        let area = ScrollArea::fixture();

        assert_eq!(area.width, px(344.0));
        assert_eq!(area.height, px(936.0));
        assert_eq!(area.overflow, Overflow::NONE);
        assert!(!area.fade);
        assert!(!area.gutter);
    }

    /// The thumb's tint is the `--foreground` token at 20%, and the two tables
    /// give different colours — so a theme cell on the scrollbar is not vacuous
    /// even though nothing compares it.
    #[test]
    fn the_thumb_tint_is_the_foreground_token_at_a_fifth() {
        use crate::theme::{Color, Theme};

        assert!((THUMB_TINT_PERCENT - 20.0).abs() < f32::EPSILON);
        let dark = Theme::DARK
            .foreground
            .mix(THUMB_TINT_PERCENT, Color::TRANSPARENT);
        let light = Theme::LIGHT
            .foreground
            .mix(THUMB_TINT_PERCENT, Color::TRANSPARENT);
        assert_ne!(dark.value(), light.value());
        // A fifth of the way to transparent is not the token itself.
        assert_ne!(dark.value(), Theme::DARK.foreground.value());
    }

    /// Both orientations are expressible and they are different pictures — a
    /// vertical track is `w-1.5` and a horizontal one `h-1.5 flex-col`.
    #[test]
    fn the_two_orientations_are_distinct() {
        assert_eq!(Orientation::default(), Orientation::Vertical);
        assert_ne!(
            ScrollBar {
                orientation: Orientation::Vertical
            },
            ScrollBar {
                orientation: Orientation::Horizontal
            },
        );
    }
}
