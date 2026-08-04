//! `sidebar` — the second component this design system **wraps**, and the first
//! where wrapping reaches only *part* of the file.
//!
//! The React half is `web/src/components/ui/sidebar.tsx`, which exports nine
//! names. Three of them — `useSidebar`, `useSidebarOptional` and
//! `SidebarProvider` — are React context plumbing and author no box; the other
//! six are the visual surface. `native/mapping/sidebar.md` carries the values,
//! the live counts and the whole of what wrapping cost. This module carries the
//! two components that can be brought to parity and says, in place, why the
//! other four cannot.
//!
//! # The seam, measured
//!
//! Spec §6.2 names `sidebar` among the primitives `gpui-component` provides and
//! says we wrap it. `gpui_component::sidebar` offers three boxes —
//! `Sidebar`, `SidebarHeader` and `SidebarFooter` — and **exactly one of them
//! can carry this port's geometry**:
//!
//! | vendor box | `Styled` refines… | reachable? |
//! |---|---|---|
//! | [`gpui_component::sidebar::SidebarHeader`] | **its own rendered box** (`refine_style(&self.style)` inside `render`) | **yes** — [`Header`] |
//! | `gpui_component::sidebar::SidebarFooter` | its *inner* `base`, not the box `render` builds around it | no |
//! | `gpui_component::sidebar::Sidebar` | its own box, but the header and footer are put inside a hard-coded `pt_3().px_3()` wrapper | no |
//!
//! The general test `select` states — *a widget is wrappable-and-measurable
//! exactly when it lets the caller supply an element, not merely a style* — is
//! **necessary and not sufficient**, and this module is where that shows.
//! All three of these vendor boxes take caller-supplied children. Two of them
//! still cannot be measured, because taking a child is not the same as letting
//! the child *be* the box: the vendor puts geometry between its own border box
//! and the child, and a `crowbar-ui` anchor can only ever land on the child.
//!
//! ## Why `SidebarFooter` resists — one line of the vendor, quoted
//!
//! ```ignore
//! impl Styled for SidebarFooter {
//!     fn style(&mut self) -> &mut StyleRefinement { self.base.style() }
//! }
//! impl RenderOnce for SidebarFooter {
//!     fn render(self, _, cx) -> impl IntoElement {
//!         h_flex().id("sidebar-footer").gap_2().p_2().w_full()
//!             .justify_between().rounded(cx.theme().radius)
//!             .hover(…).when(self.selected, …)
//!             .child(self.base)          // ← everything `Styled` can reach
//!     }
//! }
//! ```
//!
//! `SidebarFooter`'s box in `sidebar.tsx` is `flex flex-col gap-2 p-2` — one
//! `<div>`, 8px of padding, a column. The vendor's outer `h_flex()` is built
//! from literals inside `render` and **nothing addresses it**: not `Styled`,
//! which lands on `base`; not `ParentElement`, which extends `base`. So its
//! `p_2()` cannot be moved onto an anchored box, its `justify_between()` cannot
//! become `start`, its row cannot become a column, and its
//! `rounded(cx.theme().radius)` — *`gpui-component`'s* radius, not ours — cannot
//! become the 0 that `sidebar.tsx` compiles to. An anchor placed on the only
//! element this crate holds lands **inside** that padding: origin (8, 8) against
//! (0, 0), extent W − 16 × H − 16 against W × H. Four fields wrong on one box.
//!
//! ## Why `Sidebar` resists — the wrapper React does not have
//!
//! `gpui_component::sidebar::Sidebar::render` puts the header in
//! `h_flex().id("header").pt_3().px_3().gap_2()` and the footer in
//! `h_flex().id("footer").pb_3().px_3().gap_2()`, both from literals with no
//! seam, and its content in a `v_flex().id("inner").px_3().gap_y_3()` around a
//! virtualised `list()` whose items take `pt_3()` / `pb_3()`. `sidebar.tsx`'s
//! `sidebar-inner` is `flex h-full w-full flex-col bg-sidebar` and puts its
//! children **flush**. So a header rendered inside the vendor's `Sidebar` sits
//! at (12, 12) with width W − 24 where React's sits at (0, 0) with width W.
//! `Sidebar::render` also opens with `self.style.padding =
//! EdgesRefinement::default()`, which discards a caller's padding outright, and
//! its content children are bounded by `SidebarItem: Collapsible + Clone` — a
//! `gpui-component` trait no `gpui::Div` implements, so the body cannot be this
//! crate's at all.
//!
//! Per the item's own instruction, that is **reported rather than rebuilt**:
//! there is no `div()`-built `Sidebar` or `Footer` here. Both are dead exports
//! in the React tree as well — `grep '<Sidebar[ />]'` and `grep '<SidebarFooter'`
//! over `web/src` find **zero** call sites outside `__tests__` — so nothing on
//! screen is missing a port.
//!
//! # What the wrap buys on the one box it reaches
//!
//! Honestly: **no geometry**. Recorded because the point of §6.2 is to know.
//!
//! `SidebarHeader` in `sidebar.tsx` is six Tailwind classes and no behaviour.
//! `gpui_component::sidebar::SidebarHeader` is a different component that shares
//! the name: it paints a hover fill and a selected fill, implements `Collapsible`
//! and `DropdownMenu`, and lays itself out as a `justify_between` **row** with
//! the vendor theme's radius. Every one of those is something `sidebar.tsx` does
//! not have, so [`Header::render`] refines the vendor's box down to a
//! passthrough and hands it the anchored box as its only child — the same
//! division `popover` draws with `appearance(false)`, one step further along:
//! there the vendor's box merely stopped *painting*, here it also stops taking
//! room.
//!
//! What survives the wrap and is worth having: the vendor's element identity and
//! `w_full()`, its `InteractiveElement`/`Selectable`/`Collapsible` impls, and —
//! the actual reason §6.2 asks for it — the fact that a `gpui-component` bump
//! changes this file and no other.
//!
//! **The hover and selected fills cannot be removed.** They are applied *after*
//! `refine_style` and live in a separate style map, so no refinement reaches
//! them. They are invisible to a snapshot, which is one instant at rest, and
//! they are a real difference from `sidebar.tsx` — which is why
//! `surfaces/sidebar_header.rs` declares `hover` and `selected` **unmodelled**
//! rather than rendering a cell whose two sides disagree by construction.
//!
//! # `SidebarEmptyActionState` has no vendor equivalent
//!
//! `gpui-component` has no empty state, so [`EmptyActionState`] is built from
//! `div()`. That is not a §6.2 violation: the instruction is not to rebuild what
//! the library provides, and it provides nothing here.

use gpui::{
    AnyElement, Div, FontWeight, IntoElement as _, ParentElement as _, Pixels, SharedString,
    Styled as _, div, px, relative,
};
use gpui_component::sidebar::SidebarHeader as GpuiSidebarHeader;

use crate::anchor::{AnchorId, AnchorSink};
use crate::theme::Theme;

/// `SidebarHeader`'s `<div>` — the root anchor of the `sidebar-header` surface.
pub const ID_HEADER: &str = "sidebar-header";

/// `SidebarEmptyActionState`'s own `<div>` — the root anchor of the
/// `sidebar-empty` surface.
pub const ID_EMPTY: &str = "sidebar-empty";

/// The `<span>` `SidebarEmptyActionState` wraps an `icon` prop in.
pub const ID_EMPTY_ICON: &str = "sidebar-empty-icon";

/// The `message` `<div>`, which `SidebarEmptyActionState` renders
/// unconditionally.
pub const ID_EMPTY_MESSAGE: &str = "sidebar-empty-message";

/// The `description` `<div>`, rendered only where a call site passes one.
pub const ID_EMPTY_DESCRIPTION: &str = "sidebar-empty-description";

/// The anchors whose boxes size to their own text
/// (`native/oracle/ANCHORS.md` v1.5).
///
/// **The header is not one**, and that is measured rather than assumed: it is a
/// stretched child of a column flex parent, so its used width is the parent's —
/// 344px in the reference, against a 328px child.
///
/// The empty state's two are, both because the container that holds them is
/// `items-center`: a column flex container whose cross-axis alignment is
/// `center` does **not** stretch its items, so each one's width is its own
/// max-content. The root's is that plus 24px of `px-3`, and integral padding
/// carries `ceil` through unchanged — `ceil(99.94) + 24 == ceil(123.94)`, which
/// is the arithmetic v1.5 needs and the reference's two numbers confirm.
pub const CONTENT_SIZED: [&str; 2] = [ID_EMPTY, ID_EMPTY_MESSAGE];

/// The anchors whose **box height is their own line box**
/// (`native/oracle/ANCHORS.md` v1.6).
///
/// **The message, and only the message.** `ui-text-sm leading-[1.35]` on a
/// block-level `<div>` with no padding and no authored height puts a 12px run in
/// a 16.2px line box and makes that box the element's border box — v1.6's shape
/// exactly. Measured: the reference's `bounds.h` is **16** and its
/// `font.line_height` is **16.2**, which is `WebKit`'s floor of the same number.
///
/// The root is **not**: `min-h-24` authors 96px around a 64px content column, so
/// declaring it would compare 96 against a 24px line box — the mistake v1.6's
/// own badge warning is about. The description is not declared either: it is
/// `max-w-[24ch]`, so it is a line box only while the string fits on one line,
/// and the component cannot know that.
///
/// This list is the **painting** case. A message that is the empty string paints
/// no run, so [`EmptyActionState::render`] drops the declaration for that cell
/// — see there.
pub const LINE_SIZED: [&str; 1] = [ID_EMPTY_MESSAGE];

/// `--spacing`, Tailwind's stock `0.25rem` at a 16px root.
const SPACING: f32 = 4.0;

/// The document root's font size, which every `rem` in this file resolves
/// against. Tailwind preflight leaves it at the browser's own 16.
const ROOT_FONT_SIZE: f32 = 16.0;

/// `p-2` on `SidebarHeader`. 8px on all four sides.
pub const HEADER_PADDING: Pixels = px(SPACING * 2.0);

/// `gap-2` on `SidebarHeader`, between the call site's children.
pub const HEADER_GAP: Pixels = px(SPACING * 2.0);

/// `min-h-24` on `SidebarEmptyActionState` — 96px, and the number that decides
/// the reference's height: its one line of content is 64px tall.
pub const EMPTY_MIN_HEIGHT: Pixels = px(SPACING * 24.0);

/// `px-3`.
pub const EMPTY_PADDING_X: Pixels = px(SPACING * 3.0);

/// `py-6`.
pub const EMPTY_PADDING_Y: Pixels = px(SPACING * 6.0);

/// `gap-1.5`.
pub const EMPTY_GAP: Pixels = px(SPACING * 1.5);

/// `size-7` on the icon `<span>`.
pub const EMPTY_ICON_EXTENT: Pixels = px(SPACING * 7.0);

/// `mb-0.5` on the icon `<span>`.
pub const EMPTY_ICON_MARGIN_BOTTOM: Pixels = px(SPACING * 0.5);

/// `leading-[1.35]`, on the message and on the description.
///
/// An arbitrary bracketed value rather than a Tailwind step, and the one this
/// app writes everywhere — `ANCHORS.md` v1.6 exists because of it.
pub const EMPTY_LINE_HEIGHT: f32 = 1.35;

/// `max-w-[24ch]` on the description — **recorded, not applied**.
///
/// `ch` is the advance width of `0` in the resolved font, and gpui has no such
/// unit: reaching it would mean shaping a glyph at layout time through an API
/// `Styled` does not expose. The description has **no live call site** — nothing
/// in `web/src` passes `description` to `SidebarEmptyActionState` — so nothing
/// measures the clamp, and a port that invented a pixel number for it would be
/// asserting a width the reference has no opinion about. Named here so the gap
/// is a declaration rather than an omission.
pub const EMPTY_DESCRIPTION_MAX_WIDTH_CH: f32 = 24.0;

/// Which of `SidebarEmptyActionState`'s three tones a cell takes.
///
/// The prop drives the icon's and the message's colour, and nothing else: the
/// root and the description keep `text-muted-foreground` in every tone.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Tone {
    /// The prop's own default.
    #[default]
    Neutral,
    /// `tone="error"` — `text-destructive`.
    Error,
    /// `tone="success"` — `text-success`.
    Success,
}

/// Every tone, for a matrix that wants to walk them.
pub const ALL_TONES: [Tone; 3] = [Tone::Neutral, Tone::Error, Tone::Success];

impl Tone {
    /// The `--tone` spelling.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Neutral => "neutral",
            Self::Error => "error",
            Self::Success => "success",
        }
    }

    /// Whether any live call site passes this tone.
    ///
    /// **None of them do.** Both `SidebarEmptyActionState` call sites in
    /// `web/src` are the file explorer's, and neither passes `tone`, so
    /// `Neutral` is the only arm a reference can be taken from — and even that
    /// one is reachable by way of the prop's default rather than by being asked
    /// for.
    #[must_use]
    pub const fn live(self) -> bool {
        false
    }

    /// What the icon and the message are painted in.
    ///
    /// `cn()` puts the tone class **after** `text-muted-foreground` on the icon,
    /// so tailwind-merge keeps the tone; the message authors no colour of its
    /// own and inherits the root's `text-muted-foreground` unless a tone
    /// replaces it. Both end up here.
    #[must_use]
    pub fn foreground(self, theme: &Theme) -> crate::theme::Color {
        match self {
            Self::Neutral => theme.muted_foreground,
            Self::Error => theme.destructive,
            Self::Success => theme.success,
        }
    }
}

/// `SidebarHeader` — a padded column the call site fills.
///
/// # Why the body is a **height** rather than an element tree
///
/// `SidebarHeader`'s children are the call site's, and the one live call site
/// (`file-explorer-tree.tsx`) puts a search `<Input>` and a filter `<Button>` in
/// there — two primitives this port already has as their own surfaces, and
/// neither of them is *this* component. So the body is its measured extent,
/// exactly as `popover`'s is: a runtime quantity this primitive does not decide,
/// declared rather than computed, so that the one box which genuinely **is**
/// `SidebarHeader` compares against a reference whose contents are whatever they
/// happen to be.
///
/// **Only the height**, and that is measurement rather than economy. The header
/// is a column whose cross-axis alignment `sidebar.tsx` leaves at `normal`, so
/// its child is *stretched*: the live one is 328 wide inside a 344 header
/// without authoring a width at all. A `body_width` parameter would therefore
/// have decided nothing, and a parameter that decides nothing is one more way
/// for a cell to be wrong in a way no assertion can see.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct Header {
    /// How tall the call site's children come out, inside the padding.
    pub body_height: Pixels,
}

impl Header {
    /// The live file-explorer header, measured off the running app rather than
    /// read off a class list: the `<div>` is 344 × 44 and its single child is
    /// 328 × 28, which is 344 − 16 and 44 − 16.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            body_height: px(28.0),
        }
    }

    /// The header's own border-box height: the body plus [`HEADER_PADDING`]
    /// twice.
    ///
    /// Spelled rather than measured because the surface is asked for its height
    /// *before* a window exists — the window's size is what it decides.
    #[must_use]
    pub fn height(&self) -> Pixels {
        self.body_height + HEADER_PADDING * 2.0
    }

    /// Renders the header through `gpui_component::sidebar::SidebarHeader`.
    ///
    /// The five refinements are every place the vendor's box disagrees with
    /// `sidebar.tsx`, and each one is load-bearing — see the module docs for
    /// what is left of the wrap once they are applied.
    #[must_use]
    pub fn render(&self, anchors: &dyn AnchorSink) -> AnyElement {
        let header = anchors.root(ID_HEADER.into(), (*self).header_box());

        GpuiSidebarHeader::new()
            // `sidebar.tsx` is `flex flex-col`; the vendor is `h_flex()`, which
            // is a row **and** `items_center`.
            .flex_col()
            .items_stretch()
            // The vendor is `justify_between()`; `sidebar.tsx` authors nothing,
            // which is `flex-start`.
            .justify_start()
            // `gpui-component`'s own theme radius, on a box `sidebar.tsx`
            // leaves square. Invisible while the box paints nothing, and
            // `radius` is a field `ANCHORS.md` §3 compares.
            .rounded(px(0.0))
            // The geometry is the anchored box's. Left in place these would be
            // applied twice — the vendor's `p_2()` and `gap_2()` happen to be
            // `sidebar.tsx`'s own numbers, which is a coincidence and not a
            // seam: `Sidebar` and `SidebarFooter` write the same literals around
            // boxes that must not have them.
            .p_0()
            .gap_0()
            .child(header)
            .into_any_element()
    }

    /// `SidebarHeader`'s `<div>`: `flex flex-col gap-2 p-2 backdrop-blur-sm`.
    ///
    /// No background, no border, no radius and no colour — measured on the live
    /// element, which reports `rgba(0, 0, 0, 0)`, `border-width: 0px` and
    /// `border-radius: 0px`. `backdrop-blur-sm` is `ANCHORS.md` §6 material: it
    /// is a filter, it carries no field on either side, and gpui has no
    /// equivalent to paint.
    fn header_box(self) -> Div {
        div()
            .flex()
            .flex_col()
            .w_full()
            .gap(HEADER_GAP)
            .p(HEADER_PADDING)
            // The call site's children, as the room they take. **Unanchored on
            // purpose**: they are not part of `sidebar.tsx`, they are a
            // different sub-UI at every call site, and anchoring them would
            // invite a comparison against an element that is a different thing
            // each time. Rendered because they are what gives the header its
            // height, which is the quantity the one anchored box is built from.
            .child(div().w_full().h(self.body_height))
    }
}

/// `SidebarEmptyActionState` — the centred column a panel shows when it has
/// nothing to list.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct EmptyActionState {
    /// The `message` prop. Required in React, so required here.
    pub message: SharedString,
    /// The `description` prop, where a call site passes one.
    ///
    /// `None` by default, and not for convenience: **no live call site passes a
    /// description**, so an anchor the reference cannot produce would be a
    /// `FieldPresence` delta that forgives nothing.
    pub description: Option<SharedString>,
    /// Whether an `icon` is rendered.
    ///
    /// An **empty box**, as every icon in this port is: the glyph is a node a
    /// call site chooses, there is no native equivalent, and drawing a
    /// substitute would put a shape on screen for the oracle to converge on.
    pub icon: bool,
    /// `tone`.
    pub tone: Tone,
}

impl EmptyActionState {
    /// The live "No matching files" state, measured off the running app: a
    /// 123.94 × 96 box holding one 99.94 × 16 line at (12, 40).
    ///
    /// It is reached by typing a filter that matches nothing into the file
    /// explorer's search — which is why it is the empty state this port has a
    /// reference for and "No folder open" is not: that one needs the fixture
    /// workspace to have no folder open at all.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            message: SharedString::new_static("No matching files"),
            description: None,
            icon: false,
            tone: Tone::Neutral,
        }
    }

    /// The box's own height: `min-h-24`, or the column plus `py-6` where that is
    /// taller.
    ///
    /// Spelled rather than measured because a surface is asked for its height
    /// *before* a window exists. Every term is one line of its own run, which is
    /// what the reachable state has and what a driven cell can be relied on to
    /// produce; a message long enough to wrap is a picture this arithmetic does
    /// not describe, and the surface's own docs say so rather than this silently
    /// under-reporting.
    #[must_use]
    pub fn height(&self, theme: &Theme) -> Pixels {
        let line =
            |size: crate::theme::FontSize| px(size.value().0 * ROOT_FONT_SIZE * EMPTY_LINE_HEIGHT);
        // An empty message paints no run, so it generates no line box and its
        // `<div>` is 0 tall — CSS, not an approximation. See `render`.
        let mut column = if self.message.is_empty() {
            px(0.0)
        } else {
            line(theme.ui_text_sm)
        };
        if self.icon {
            column += EMPTY_ICON_EXTENT + EMPTY_ICON_MARGIN_BOTTOM + EMPTY_GAP;
        }
        if self.description.is_some() {
            column += EMPTY_GAP + line(theme.ui_text_xs);
        }
        let content = EMPTY_PADDING_Y * 2.0 + column;
        if content > EMPTY_MIN_HEIGHT {
            content
        } else {
            EMPTY_MIN_HEIGHT
        }
    }

    /// Renders the empty state, opting every contract anchor into `anchors`.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let mut root = Self::root(theme);

        if self.icon {
            root = root.child(anchors.boxed(ID_EMPTY_ICON.into(), Self::icon_box()));
        }
        // **The declarations follow the run, not the element.** An empty message
        // paints no text, so it has no line box to be sized by and no
        // `font.line_height` for `ANCHORS.md` v1.6 to compare against — the same
        // call `label` makes for its own empty cell, and the reason
        // `sidebar.tsx` writes `data-oracle-line-sized` conditionally.
        root = if self.message.is_empty() {
            root.child(anchors.boxed(
                AnchorId::new(ID_EMPTY_MESSAGE).content_sized(),
                self.message_box(theme),
            ))
        } else {
            root.child(anchors.boxed_text(
                AnchorId::new(ID_EMPTY_MESSAGE).content_sized().line_sized(),
                self.message_box(theme),
                self.message.clone(),
            ))
        };
        if let Some(description) = &self.description {
            root = root.child(anchors.boxed_text(
                AnchorId::new(ID_EMPTY_DESCRIPTION),
                Self::description_box(theme),
                description.clone(),
            ));
        }

        anchors.root(AnchorId::new(ID_EMPTY).content_sized(), root)
    }

    /// The outer `<div>`: `ui-font flex min-h-24 select-none flex-col
    /// items-center justify-center gap-1.5 px-3 py-6 text-center
    /// text-muted-foreground`.
    ///
    /// The font family is named explicitly for the reason `popover`'s popup is:
    /// gpui can only report the *declared* family, and an inherited
    /// `.SystemUIFont` is a string the DOM will never produce. `ui-font` is
    /// `font-family: var(--app-font-family, var(--font-sans))`, and the live
    /// element resolves it to `CalSansUI` — [`Theme::font_sans`]'s first entry.
    ///
    /// `select-none` and `text-center` carry no `ANCHORS.md` field. The centring
    /// is nonetheless applied, because it is what puts the message at y 40 in a
    /// 96px box rather than at y 24.
    fn root(theme: &Theme) -> Div {
        let family = theme.font_sans.primary().unwrap_or("sans-serif");
        div()
            .flex()
            .flex_col()
            .items_center()
            .justify_center()
            .min_h(EMPTY_MIN_HEIGHT)
            .gap(EMPTY_GAP)
            .px(EMPTY_PADDING_X)
            .py(EMPTY_PADDING_Y)
            .font_family(family)
            .text_color(theme.muted_foreground)
    }

    /// The icon `<span>`: `mb-0.5 flex size-7 items-center justify-center`, in
    /// the tone's colour.
    fn icon_box() -> Div {
        div()
            .flex()
            .items_center()
            .justify_center()
            .size(EMPTY_ICON_EXTENT)
            .mb(EMPTY_ICON_MARGIN_BOTTOM)
    }

    /// The message `<div>`: `ui-text-sm leading-[1.35]`, in the tone's colour.
    ///
    /// `ui-text-sm` is this app's own `--ui-text-sm`, `0.75rem` — measured live
    /// at `font-size: 12px`. It is [`Theme::ui_text_sm`] rather than a Tailwind
    /// step because the class **is** the token: `@utility ui-text-sm` in
    /// `index.css` compiles to `font-size: var(--ui-text-sm)` and nothing else.
    fn message_box(&self, theme: &Theme) -> Div {
        div()
            .text_size(theme.ui_text_sm.value())
            .line_height(relative(EMPTY_LINE_HEIGHT))
            .font_weight(FontWeight::NORMAL)
            .text_color(self.tone.foreground(theme))
    }

    /// The description `<div>`: `ui-text-xs max-w-[24ch] leading-[1.35]
    /// text-muted-foreground`.
    ///
    /// The clamp is [`EMPTY_DESCRIPTION_MAX_WIDTH_CH`] — recorded, not applied.
    fn description_box(theme: &Theme) -> Div {
        div()
            .text_size(theme.ui_text_xs.value())
            .line_height(relative(EMPTY_LINE_HEIGHT))
            .font_weight(FontWeight::NORMAL)
            .text_color(theme.muted_foreground)
    }
}

/// `SidebarHeaderIconButton` — a `<Button>` preset, and **not an element**.
///
/// It is `variant="ghost" compact className="size-6 rounded-md p-0"` over the
/// `button` primitive this port already has, and it authors no box of its own.
/// Modelling it as an element would mean calling `Button::render` from inside
/// another surface, which reaches [`AnchorSink::root`] — and that **clears the
/// registry**, taking the enclosing surface's anchors with it. So it is the
/// numbers, which is all it ever was.
///
/// **Zero call sites.** `grep '<SidebarHeaderIconButton'` over `web/src` finds
/// nothing, in tests or out, so there is no reference for any of this.
pub mod header_icon_button {
    use super::SPACING;
    use gpui::{Pixels, px};

    /// `size-6`.
    pub const EXTENT: Pixels = px(SPACING * 6.0);

    /// `p-0`, which overrides whatever the `compact` size would have applied.
    pub const PADDING: Pixels = px(0.0);

    /// `rounded-md`, over the `ghost` variant's own radius.
    ///
    /// A `RadiusClass` on the `button` surface, which is where a cell for this
    /// would be driven from.
    pub const RADIUS_CLASS: &str = "rounded-md";
}

/// `SidebarHeaderSearch` — an `<Input>` preset plus a leading-icon `<span>`,
/// and **not an element**, for the reason [`header_icon_button`] is not.
///
/// **Zero call sites.** The live file-explorer header hand-writes the same
/// shape — a `relative flex` span, an absolutely-positioned glyph and a padded
/// `<Input>` — rather than using this export, which is why the numbers below and
/// the ones in the reference are not the same numbers.
pub mod header_search {
    use super::SPACING;
    use gpui::{Pixels, px};

    /// `h-6` on the `<Input>`.
    pub const FIELD_HEIGHT: Pixels = px(SPACING * 6.0);

    /// `px-2`.
    pub const FIELD_PADDING_X: Pixels = px(SPACING * 2.0);

    /// `ps-7` — the inline-start padding that clears the glyph.
    pub const FIELD_PADDING_START: Pixels = px(SPACING * 7.0);

    /// `start-2` on the glyph's `<span>`.
    pub const ICON_INSET_START: Pixels = px(SPACING * 2.0);

    /// `size-3.5` on the glyph.
    pub const ICON_EXTENT: Pixels = px(SPACING * 3.5);
}

#[cfg(test)]
mod tests {
    use super::{
        ALL_TONES, CONTENT_SIZED, EMPTY_DESCRIPTION_MAX_WIDTH_CH, EMPTY_GAP, EMPTY_ICON_EXTENT,
        EMPTY_ICON_MARGIN_BOTTOM, EMPTY_LINE_HEIGHT, EMPTY_MIN_HEIGHT, EMPTY_PADDING_X,
        EMPTY_PADDING_Y, EmptyActionState, HEADER_GAP, HEADER_PADDING, Header, ID_EMPTY,
        ID_EMPTY_DESCRIPTION, ID_EMPTY_ICON, ID_EMPTY_MESSAGE, ID_HEADER, LINE_SIZED, Tone,
        header_icon_button, header_search,
    };
    use crate::theme::Theme;
    use gpui::px;

    /// Every length, against the `calc(var(--spacing) * n)` the app's own
    /// Tailwind compiles the class to.
    #[test]
    fn every_length_is_the_compiled_spacing_multiple() {
        const STEP: f32 = 4.0;

        assert_eq!(HEADER_PADDING, px(STEP * 2.0)); // p-2
        assert_eq!(HEADER_GAP, px(STEP * 2.0)); // gap-2
        assert_eq!(EMPTY_MIN_HEIGHT, px(STEP * 24.0)); // min-h-24
        assert_eq!(EMPTY_PADDING_X, px(STEP * 3.0)); // px-3
        assert_eq!(EMPTY_PADDING_Y, px(STEP * 6.0)); // py-6
        assert_eq!(EMPTY_GAP, px(STEP * 1.5)); // gap-1.5
        assert_eq!(EMPTY_ICON_EXTENT, px(STEP * 7.0)); // size-7
        assert_eq!(EMPTY_ICON_MARGIN_BOTTOM, px(STEP * 0.5)); // mb-0.5
        assert_eq!(header_icon_button::EXTENT, px(STEP * 6.0)); // size-6
        assert_eq!(header_search::FIELD_HEIGHT, px(STEP * 6.0)); // h-6
        assert_eq!(header_search::FIELD_PADDING_START, px(STEP * 7.0)); // ps-7
        assert_eq!(header_search::ICON_EXTENT, px(STEP * 3.5)); // size-3.5
    }

    /// The header's fixture is the live file-explorer header: a 28px body in a
    /// 44px box, which is 28 + 8 + 8. The 344 is the *parent's* width and not
    /// this component's, which is why there is no width here to assert.
    #[test]
    fn the_header_fixture_is_the_live_file_explorer_header() {
        let header = Header::fixture();

        assert_eq!(header.body_height, px(28.0));
        assert_eq!(header.height(), px(44.0));
        // 344 − 16 is the live child's 328, which the stretch produces rather
        // than the component declaring it.
        assert!((344.0 - f32::from(HEADER_PADDING) * 2.0 - 328.0).abs() < f32::EPSILON);
    }

    /// The padding reaches both edges at every body height — which is what makes
    /// the body a legitimate stand-in for the call site's children rather than a
    /// fudge factor.
    #[test]
    fn the_header_box_is_the_body_plus_two_paddings() {
        for h in [0.0, 5.0, 28.0, 120.0] {
            let header = Header { body_height: px(h) };
            assert_eq!(header.height(), px(h + 16.0));
        }
    }

    /// The empty state's fixture is the live "No matching files" state, and its
    /// height is `min-h-24` rather than its content: one 16.2px line inside
    /// 48px of `py-6` is 64, and 96 wins.
    ///
    /// **That is the assertion that makes `min-h-24` load-bearing.** A port that
    /// dropped it would draw a 64px box, and every bound below the message would
    /// move with it.
    #[test]
    fn the_empty_fixture_is_the_live_no_matching_files_state() {
        let empty = EmptyActionState::fixture();

        assert_eq!(empty.message, "No matching files");
        assert_eq!(empty.description, None);
        assert!(!empty.icon);
        assert_eq!(empty.tone, Tone::Neutral);

        let line = 12.0 * EMPTY_LINE_HEIGHT;
        let content = f32::from(EMPTY_PADDING_Y) * 2.0 + line;
        assert!((content - 64.2).abs() < 0.001, "{content}");
        assert!(content < f32::from(EMPTY_MIN_HEIGHT));
        // …and the reference's 96 is the minimum, to the pixel.
        assert_eq!(EMPTY_MIN_HEIGHT, px(96.0));
    }

    /// **The message is the one line-sized anchor**, and the two content-sized
    /// ones are the two boxes an `items-center` column does not stretch.
    #[test]
    fn the_declarations_are_the_two_boxes_a_centred_column_shrink_wraps() {
        assert_eq!(LINE_SIZED, [ID_EMPTY_MESSAGE]);
        assert_eq!(CONTENT_SIZED, [ID_EMPTY, ID_EMPTY_MESSAGE]);

        // The header is neither: it is stretched by its parent, and its height
        // is padding plus a body.
        assert!(!CONTENT_SIZED.contains(&ID_HEADER));
        assert!(!LINE_SIZED.contains(&ID_HEADER));
        // Nor is the description, whose `max-w-[24ch]` makes it a line box only
        // while the string fits on one line.
        assert!(!LINE_SIZED.contains(&ID_EMPTY_DESCRIPTION));
        assert!(!CONTENT_SIZED.contains(&ID_EMPTY_DESCRIPTION));
        // Nor the icon, whose box is authored by `size-7`.
        assert!(!LINE_SIZED.contains(&ID_EMPTY_ICON));
        assert!(!CONTENT_SIZED.contains(&ID_EMPTY_ICON));

        // `ceil` carries through integral padding, which is what lets the root
        // be declared alongside the run it is sized by: the reference's two
        // widths satisfy it exactly.
        let message: f32 = 99.94;
        let root: f32 = 123.94;
        let padding = f32::from(EMPTY_PADDING_X) * 2.0;
        assert!((message + padding - root).abs() < 0.001);
        assert!((message.ceil() + padding - root.ceil()).abs() < f32::EPSILON);
    }

    /// The message's line box is 16.2 and `WebKit` floors it to 16 — the pair
    /// `ANCHORS.md` v1.6 compares, and the reference carries both numbers.
    #[test]
    fn the_message_line_box_is_the_v1_6_pair() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            assert_eq!(theme.ui_text_sm.value(), gpui::rems(0.75));
        }
        let line = 0.75 * 16.0 * EMPTY_LINE_HEIGHT;
        assert!((line - 16.2).abs() < 0.001, "{line}");
        assert!((line.floor() - 16.0).abs() < f32::EPSILON);
        // And the description is a step down: `--ui-text-xs` is 0.6875rem.
        assert_eq!(Theme::DARK.ui_text_xs.value(), gpui::rems(0.6875));
        assert!(Theme::DARK.ui_text_xs.value().0 < Theme::DARK.ui_text_sm.value().0);
    }

    /// The three tones paint three different colours, so a tone cell is a
    /// different picture rather than a second spelling of the same one — and
    /// none of them is reachable, which is the fact the caption has to carry.
    #[test]
    fn the_three_tones_are_three_colours_and_none_is_live() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            let neutral = Tone::Neutral.foreground(&theme);
            let error = Tone::Error.foreground(&theme);
            let success = Tone::Success.foreground(&theme);

            assert_eq!(neutral, theme.muted_foreground);
            assert_eq!(error, theme.destructive);
            assert_eq!(success, theme.success);
            assert_ne!(neutral, error);
            assert_ne!(neutral, success);
            assert_ne!(error, success);
        }

        assert_eq!(Tone::default(), Tone::Neutral);
        assert_eq!(ALL_TONES.len(), 3);
        for tone in ALL_TONES {
            assert!(!tone.live(), "{tone:?}");
            assert!(!tone.name().is_empty());
        }
    }

    /// The ids are distinct and namespaced — a repeat would make two anchors one
    /// record, and `ANCHORS.md` v1.8 makes that an error rather than a
    /// first-wins.
    #[test]
    fn every_anchor_id_is_distinct_and_namespaced() {
        let ids = [
            ID_HEADER,
            ID_EMPTY,
            ID_EMPTY_ICON,
            ID_EMPTY_MESSAGE,
            ID_EMPTY_DESCRIPTION,
        ];
        let mut sorted = ids.to_vec();
        sorted.sort_unstable();
        let before = sorted.len();
        sorted.dedup();
        assert_eq!(sorted.len(), before, "{ids:?}");
        assert!(ids.iter().all(|id| id.starts_with("sidebar-")));

        // The two surfaces are rooted apart: neither root is a prefix the other
        // can be mistaken for.
        assert_eq!(ID_HEADER, "sidebar-header");
        assert_eq!(ID_EMPTY, "sidebar-empty");
        assert!(!ID_HEADER.starts_with(ID_EMPTY));
        assert!(!ID_EMPTY.starts_with(ID_HEADER));
    }

    /// The `24ch` clamp is recorded rather than applied, and the constant says
    /// so by existing. If someone later turns it into pixels, this is where the
    /// reasoning is.
    #[test]
    fn the_description_clamp_is_recorded_in_ch() {
        assert!((EMPTY_DESCRIPTION_MAX_WIDTH_CH - 24.0).abs() < f32::EPSILON);
    }

    /// The two presets are numbers and nothing else — `size-6 rounded-md p-0`
    /// and the search field's paddings. They exist so the values are in the
    /// system; neither authors an element, because both would have to nest a
    /// primitive whose `render` reaches `AnchorSink::root`.
    #[test]
    fn the_two_presets_are_numbers_over_primitives_this_port_already_has() {
        assert_eq!(header_icon_button::PADDING, px(0.0));
        assert_eq!(header_icon_button::RADIUS_CLASS, "rounded-md");
        assert_eq!(header_search::FIELD_PADDING_X, px(8.0));
        assert_eq!(header_search::ICON_INSET_START, px(8.0));
        // `ps-7` clears a `size-3.5` glyph inset by `start-2`: 8 + 14 = 22 < 28.
        let glyph_end =
            f32::from(header_search::ICON_INSET_START) + f32::from(header_search::ICON_EXTENT);
        assert!(
            glyph_end < f32::from(header_search::FIELD_PADDING_START),
            "{glyph_end}"
        );
    }
}
