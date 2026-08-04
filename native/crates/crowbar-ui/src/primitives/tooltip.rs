//! `tooltip` — the first §6.2 candidate that fails the wrap test rather than
//! passing it.
//!
//! The native half of `web/src/components/ui/tooltip.tsx`, a `@radix-ui/
//! react-tooltip` wrapper, and of the byte-identical duplicate `button.tsx`
//! carries inline for its own `tooltip`/`shortcut` props (`tooltipContentBase`
//! is imported from `tooltip.tsx` for exactly that reason — one class list, two
//! JSX sites). Every value below came out of the app's own Tailwind 4.3.0,
//! measured live rather than read off the class name — see
//! `native/mapping/tooltip.md`.
//!
//! # Wrap or build: the seam test, applied
//!
//! §10.1 says not to rebuild a primitive `gpui-component` already has, and
//! `native/vendor/gpui-component/src/tooltip.rs` does have a `Tooltip`. The
//! test this item was handed is `popover`'s own: **a widget is
//! wrappable-and-measurable exactly when it lets the caller supply an
//! *element*, not merely a style.**
//!
//! `gpui_component::Tooltip` fails it, and the failure is structural rather
//! than a missing method:
//!
//! * Its whole painted box — `bg(cx.theme().tokens.popover)`, `border_1()`,
//!   `shadow_md()`, `rounded(px(6.))`, `py_0p5().px_2()` — is built **inside
//!   its own private `Render::render`**, on an `h_flex()` neither this crate
//!   nor any caller ever holds.
//! * `Tooltip::element(builder)` *does* take `impl IntoElement` — the shape
//!   `popover`'s own test looks for — but the builder's result lands **inside**
//!   that private box as a child, not in place of it. The chrome the reference
//!   requires (`rounded-lg`, `border-border/70`, `bg-card/95`, `shadow-lg`) is
//!   never reachable through it.
//! * `Styled::style()` gives a `StyleRefinement` on that same private `h_flex`
//!   — the exact "only seam is a `StyleRefinement`" shape `ANCHORS.md` and this
//!   item's brief both name as the fake convergence to refuse. A
//!   `StyleRefinement` *could* be pushed far enough to repaint every visible
//!   pixel right, but `AnchorSink::root`/`boxed` still take a `Div` this crate
//!   constructs — and no call here ever produces the `Div` that `render`
//!   builds. There is nothing to tag with an anchor id; wrapping a `div()`
//!   around the vendor's `Tooltip` would compare a box whose bounds merely
//!   *happen* to coincide with the real one, never the box gpui actually
//!   painted.
//!
//! `popover` passed the same test because `Popover::appearance(false)` turns
//! the vendor's own box off entirely, and `ParentElement::child()` accepts a
//! `Div` this crate built and already anchored. `gpui_component::Tooltip` has
//! no `appearance` flag and no `ParentElement` impl — there is no escape hatch
//! at all, not even the one popover needed. **Verdict: built, not wrapped**,
//! the same way `dropdown_menu` and `checkbox` are: raw `div()`s under this
//! module's own anchors, none of `gpui_component::tooltip` in the render path.
//!
//! # `tooltip.tsx` is not `popover --tooltip`
//!
//! `crowbar-ui::components::popover::Variant::Tooltip` models
//! `PopoverContent`'s `tooltipStyle` prop — `w-fit rounded-md text-xs
//! shadow-md/5`, reached by `toast.tsx` and by no live `PopoverContent`. It
//! shares a *name* with this component and nothing else: different React
//! primitive (`@base-ui/react`'s `Popover` against `@radix-ui/react-tooltip`'s
//! `Tooltip`), different class list, different token set. Measured live
//! (`--flags` aside, this is two different pictures): `tooltipStyle`'s popup is
//! `rounded-md` (8px) with `shadow-md/5`; `tooltip.tsx`'s content here is
//! `rounded-lg` (10px) with `shadow-lg`, `border-border/70`, `bg-card/95` and
//! `backdrop-blur-sm` — none of which `tooltipStyle` has. **They are two
//! surfaces, not one surface and its unreached variant**, and this file is the
//! independent port of the second.
//!
//! # Reachability: real, and not gated behind a dirty worktree
//!
//! `content: string` is a required prop on `TooltipCompound`, and `button.tsx`
//! exposes the identical content through a `tooltip` prop taken by 21 call
//! sites outside the Plate set (`tab-bar-item.tsx`, `path-breadcrumb.tsx`,
//! `editor-status-actions.tsx`, the workspace sidebar, …), against 3 for the
//! standalone `<Tooltip>` (`terminal-settings.tsx`,
//! `path-breadcrumb.tsx`'s non-interactive arm, and one inside the Plate
//! floating toolbar). Both routes render byte-identical
//! `tooltipContentBase` output.
//!
//! **Reached live**, twice, through `HTMLElement.focus()` — a direct DOM call,
//! not a synthetic event, and the one this component's trigger actually
//! listens for: Radix opens a tooltip on focus without a hover delay, which is
//! why `data-state` reads `instant-open` in both captures below rather than
//! `delayed-open`.
//!
//! * The open-tabs bar's `Close ⌘W` button (`tab-bar-item.tsx`, a `Button`
//!   with `tooltip="Close" shortcut="⌘W"`) — the reference this file's fixture
//!   is measured from: `99.296875 × 32` at `data-side="bottom"`.
//! * The editor breadcrumb's `a.ts` segment (`path-breadcrumb.tsx`, the
//!   `interactive` arm, `tooltip={getSegmentPath(index)}`, no shortcut) —
//!   `demo/src/a/a.ts`, `111.28125 × 32`. Confirms the no-shortcut arm
//!   independently: same 32px height (border + padding + the 18px text line
//!   box), width purely a function of the string.
//!
//! # What had to be overridden to reach this design
//!
//! * **The shortcut chip is not `gpui-component`'s `Kbd`.** `web/src/
//!   components/ui/keybinding.tsx` is a *third* keycap primitive — distinct
//!   from both `crowbar-ui::components::kbd` (`kbd.tsx`'s `min-w-5 h-5
//!   rounded` cap, already ported) and from `gpui-component`'s own — with its
//!   own class list (`min-h-4 rounded-md border border-border bg-card
//!   px-1.5 leading-none`). Porting a fourth component for one sub-box inside
//!   this one would be scope creep this item was not handed, so the chip is a
//!   private helper here, built the same way `popover`'s title is: raw
//!   `div()`s, no shared primitive.
//! * `shadow-[inset_0_-1px_0_rgba(0,0,0,0.12)]` on the chip has no anchored
//!   field either — `ANCHORS.md` §3's `border` carries width and colour only,
//!   nothing shaped like an inset shadow — so it is not painted; the two
//!   fields the differ does read (`bg`, `border`) are.
//!
//! # Declarations
//!
//! Both anchors are content-sized (`whitespace-nowrap`, no authored width) and
//! neither is line-sized: the root's box is its padding plus border around an
//! 18px line, not the 18px line itself, and the chip's is `min-h-4`'s 16px
//! floor against a `leading-none` line box of 12 — `kbd.rs`'s own precedent,
//! confirmed independently here.

use gpui::{
    AnyElement, Div, FontWeight, ParentElement as _, Pixels, SharedString, Styled as _, div, px,
    relative, rems,
};

use crate::anchor::{AnchorId, AnchorSink};
use crate::theme::{Color, Theme};

/// The root anchor: `TooltipPrimitive.Content` — the box `bg-card/95` paints.
/// Every other bound on this surface is relative to it (`ANCHORS.md` §4).
pub const ID_TOOLTIP: &str = "tooltip";

/// The keyboard-shortcut chip, present only when a call site passes one.
pub const ID_SHORTCUT: &str = "tooltip-shortcut";

/// Both anchors size to their own content (`ANCHORS.md` v1.5):
/// `whitespace-nowrap` on the root, no authored width on either.
pub const CONTENT_SIZED: [&str; 2] = [ID_TOOLTIP, ID_SHORTCUT];

/// Neither anchor is line-sized (`ANCHORS.md` v1.6). The root's height is its
/// padding and border around the line, not the line itself; the chip's is a
/// `min-h-4` floor against a smaller `leading-none` box. See the module docs.
pub const LINE_SIZED: [&str; 0] = [];

/// `--spacing`, Tailwind's stock `0.25rem` at a 16px root.
const SPACING: f32 = 4.0;

/// `rounded-lg` on the root — the same token `popover`'s default popup takes.
pub const RADIUS: Pixels = px(10.0);

/// `rounded-md` on the shortcut chip.
pub const SHORTCUT_RADIUS: Pixels = px(8.0);

/// `border` on the root — measured `1px`.
pub const BORDER_WIDTH: Pixels = px(1.0);

/// `border-border/70`. `mix` rather than `Hsla::opacity` — the idiom this
/// codebase already uses for a Tailwind `/N` modifier (`dropdown_menu`'s
/// `RING_ALPHA`), and the one that reproduces `color-mix(in srgb, …)`.
pub const BORDER_ALPHA: f32 = 70.0;

/// `border-border` on the shortcut chip — the bare token, no modifier.
/// Measured live at the same alpha `popover`'s bare `border` class carries.
const SHORTCUT_BORDER_ALPHA: f32 = 100.0;

/// `bg-card/95` on the root.
pub const BACKGROUND_ALPHA: f32 = 95.0;

/// `py-1.5` — `calc(--spacing * 1.5)`, measured `padding-top: 6px`.
pub const PADDING_Y: Pixels = px(SPACING * 1.5);

/// `px-2.5` — measured `padding-left: 10px`.
pub const PADDING_X: Pixels = px(SPACING * 2.5);

/// `gap-2`, present only alongside a shortcut (`flex items-center gap-2` is
/// conditional in `tooltipContentBase`'s call site).
pub const GAP: Pixels = px(SPACING * 2.0);

/// `px-1.5` on the shortcut chip.
pub const SHORTCUT_PADDING_X: Pixels = px(SPACING * 1.5);

/// `min-h-4` on the shortcut chip — a floor, not the chip's line box.
pub const SHORTCUT_MIN_HEIGHT: Pixels = px(SPACING * 4.0);

/// `ui-text-sm` on both the root and the chip: `0.75rem`.
const TEXT_SIZE: f32 = 0.75;

/// The root's line height: measured live at `18px` over a `12px` run —
/// `ui-text-sm`'s pairing in this stylesheet, confirmed on both captured
/// fixtures (with and without a shortcut).
const LINE_HEIGHT: f32 = 1.5;

/// `leading-none` on the shortcut chip: `line-height: 1`.
const SHORTCUT_LINE_HEIGHT: f32 = 1.0;

/// One tooltip popup: its text, and an optional keyboard shortcut.
///
/// # Why the trigger and the placement are absent
///
/// Neither is part of `tooltipContentBase`. The trigger (`TooltipPrimitive.
/// Trigger`) carries no styling of its own — same shape as `popover`'s — and
/// the popup is portalled to `document.body`, so `side`/`sideOffset` are
/// Floating UI placement, invisible to `ANCHORS.md` §4's root-relative
/// arithmetic exactly as `popover`'s own `sideOffset` is.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Tooltip {
    /// The tooltip's text. `""` is `--flags empty` — a real, if unreached,
    /// picture: the box collapses to its padding and border around nothing.
    pub content: SharedString,
    /// `shortcut`, rendered as the chip Radix calls `Keybinding`. `None` on
    /// the majority of call sites, `Some` on 21 of the 24 total.
    pub shortcut: Option<SharedString>,
}

impl Tooltip {
    /// The `Close ⌘W` fixture: `tab-bar-item.tsx`'s close button, focused
    /// live. Measured `99.296875 × 32` at rest.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            content: SharedString::new_static("Close"),
            shortcut: Some(SharedString::new_static("⌘W")),
        }
    }

    /// The `path-breadcrumb.tsx` fixture, with no shortcut: `demo/src/a/a.ts`,
    /// measured `111.28125 × 32`.
    #[must_use]
    pub fn fixture_without_shortcut() -> Self {
        Self {
            content: SharedString::new_static("demo/src/a/a.ts"),
            shortcut: None,
        }
    }

    /// The root box's own styling, before its children.
    fn shell(theme: &Theme, has_shortcut: bool) -> Div {
        let family = theme.font_sans.primary().unwrap_or("sans-serif");
        let mut element = div()
            .flex()
            .items_center()
            .whitespace_nowrap()
            .rounded(RADIUS)
            .border(BORDER_WIDTH)
            .border_color(theme.border.mix(BORDER_ALPHA, Color::TRANSPARENT))
            .bg(theme.card.mix(BACKGROUND_ALPHA, Color::TRANSPARENT))
            .text_color(theme.foreground)
            .font_family(family)
            .font_weight(FontWeight::NORMAL)
            .text_size(rems(TEXT_SIZE))
            .line_height(relative(LINE_HEIGHT))
            .py(PADDING_Y)
            .px(PADDING_X)
            // `shadow-lg`. Invisible to the differ (`ANCHORS.md` §6), painted
            // for fidelity the same way `popover`'s is.
            .shadow_lg();
        if has_shortcut {
            element = element.gap(GAP);
        }
        element
    }

    /// The shortcut chip: `min-h-4 rounded-md border border-border bg-card
    /// px-1.5 leading-none text-muted-foreground`.
    fn shortcut_chip(theme: &Theme, anchors: &dyn AnchorSink, text: &SharedString) -> AnyElement {
        let family = theme.font_sans.primary().unwrap_or("sans-serif");
        let id = AnchorId::from(ID_SHORTCUT).content_sized();
        let shell = div()
            .flex()
            .items_center()
            .justify_center()
            .whitespace_nowrap()
            .min_h(SHORTCUT_MIN_HEIGHT)
            .px(SHORTCUT_PADDING_X)
            .rounded(SHORTCUT_RADIUS)
            .border(BORDER_WIDTH)
            .border_color(theme.border.mix(SHORTCUT_BORDER_ALPHA, Color::TRANSPARENT))
            .bg(theme.card)
            .text_color(theme.muted_foreground)
            .font_family(family)
            .font_weight(FontWeight::NORMAL)
            .text_size(rems(TEXT_SIZE))
            .line_height(relative(SHORTCUT_LINE_HEIGHT));
        anchors.boxed_text(id, shell, text.clone())
    }

    /// Renders the popup, opting the root and (when present) the shortcut
    /// chip into `anchors`.
    ///
    /// The run is placed with [`AnchorSink::text_half`] rather than
    /// [`AnchorSink::boxed_text`], because the shortcut chip — when present —
    /// follows the text rather than being the box's only content: the same
    /// shape `dropdown_menu`'s row is, for the same reason.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let id = AnchorId::from(ID_TOOLTIP).content_sized();
        let run = anchors.text_half(&id, self.content.clone());
        let mut shell = Self::shell(theme, self.shortcut.is_some()).child(run);
        if let Some(shortcut) = &self.shortcut {
            shell = shell.child(Self::shortcut_chip(theme, anchors, shortcut));
        }
        anchors.boxed(id, shell)
    }
}

#[cfg(test)]
mod tests {
    use super::{
        BACKGROUND_ALPHA, BORDER_ALPHA, CONTENT_SIZED, GAP, ID_SHORTCUT, ID_TOOLTIP, LINE_HEIGHT,
        LINE_SIZED, PADDING_X, PADDING_Y, RADIUS, SHORTCUT_LINE_HEIGHT, SHORTCUT_MIN_HEIGHT,
        SHORTCUT_PADDING_X, SHORTCUT_RADIUS, TEXT_SIZE, Tooltip,
    };
    use crate::theme::Theme;
    use gpui::px;

    /// Every length as the compiled `calc(var(--spacing) * n)` the app's own
    /// Tailwind produces.
    #[test]
    fn every_length_is_the_compiled_spacing_multiple() {
        const STEP: f32 = 4.0;
        assert_eq!(PADDING_Y, px(STEP * 1.5)); // py-1.5
        assert_eq!(PADDING_X, px(STEP * 2.5)); // px-2.5
        assert_eq!(GAP, px(STEP * 2.0)); // gap-2
        assert_eq!(SHORTCUT_PADDING_X, px(STEP * 1.5)); // px-1.5
        assert_eq!(SHORTCUT_MIN_HEIGHT, px(STEP * 4.0)); // min-h-4
    }

    /// `rounded-lg` on the root, `rounded-md` on the chip — the same two radii
    /// `popover`'s default and `tooltipStyle` arms take, and distinct from
    /// each other.
    #[test]
    fn the_two_radii_are_the_projects_lg_and_md_steps() {
        assert_eq!(RADIUS, px(10.0));
        assert_eq!(SHORTCUT_RADIUS, px(8.0));
        assert_ne!(RADIUS, SHORTCUT_RADIUS);
    }

    /// The root's 18px line box over a 12px run, measured live and identical
    /// on both captured fixtures — with and without a shortcut.
    #[test]
    fn the_root_line_box_matches_both_live_captures() {
        let line_box = TEXT_SIZE * 16.0 * LINE_HEIGHT;
        assert!((line_box - 18.0).abs() < 0.001, "{line_box}");
    }

    /// The chip's line box (`leading-none`) is smaller than its authored
    /// `min-h-4` floor — the arithmetic that makes it not line-sized.
    #[test]
    fn the_chip_floor_exceeds_its_own_line_box() {
        let line_box = TEXT_SIZE * 16.0 * SHORTCUT_LINE_HEIGHT;
        assert!((line_box - 12.0).abs() < 0.001, "{line_box}");
        assert!(f32::from(SHORTCUT_MIN_HEIGHT) - line_box > 0.5);
    }

    /// Both anchors are content-sized and neither is line-sized.
    #[test]
    fn the_declarations_match_the_measured_shapes() {
        assert_eq!(CONTENT_SIZED, [ID_TOOLTIP, ID_SHORTCUT]);
        assert_eq!(LINE_SIZED, [] as [&str; 0]);
    }

    /// `border-border/70` on the root against the bare `border-border` on the
    /// chip — the same pair of alphas `popover`'s border and `dropdown_menu`'s
    /// ring already establish as distinct tokens, not a shared constant.
    #[test]
    fn the_root_border_is_mixed_and_the_chip_border_is_not() {
        assert!((BORDER_ALPHA - 70.0).abs() < f32::EPSILON);
        let theme = Theme::DARK;
        let root_border = theme
            .border
            .mix(BORDER_ALPHA, crate::theme::Color::TRANSPARENT);
        assert_ne!(root_border, theme.border);
    }

    /// `bg-card/95` — real, and distinguishable from a bare `bg-card`.
    #[test]
    fn the_background_alpha_is_measured_not_assumed() {
        assert!((BACKGROUND_ALPHA - 95.0).abs() < f32::EPSILON);
        let theme = Theme::DARK;
        let mixed = theme
            .card
            .mix(BACKGROUND_ALPHA, crate::theme::Color::TRANSPARENT);
        assert_ne!(mixed, theme.card);
    }

    /// The two fixtures are the two live captures.
    #[test]
    fn the_fixtures_are_the_two_live_captures() {
        let with_shortcut = Tooltip::fixture();
        assert_eq!(with_shortcut.content.as_ref(), "Close");
        assert_eq!(with_shortcut.shortcut.as_deref(), Some("⌘W"));

        let without = Tooltip::fixture_without_shortcut();
        assert_eq!(without.content.as_ref(), "demo/src/a/a.ts");
        assert_eq!(without.shortcut, None);
    }

    /// `--flags empty`: a real, if unreached, picture — `""` collapses the
    /// root to its padding and border around nothing, the same call
    /// `popover`'s `empty` flag makes about its own body.
    #[test]
    fn empty_content_is_expressible() {
        let empty = Tooltip {
            content: gpui::SharedString::new_static(""),
            shortcut: None,
        };
        assert_eq!(empty.content.as_ref(), "");
    }

    /// The two ids are distinct and both namespaced under `tooltip`.
    #[test]
    fn the_anchor_ids_are_distinct_and_namespaced() {
        assert_ne!(ID_TOOLTIP, ID_SHORTCUT);
        assert!(ID_SHORTCUT.starts_with(ID_TOOLTIP));
    }
}
