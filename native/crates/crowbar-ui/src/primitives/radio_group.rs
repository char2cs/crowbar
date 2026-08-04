//! `radio-group` — built, not wrapped, for the same structural reason
//! `tooltip` is, and unreachable by any parity run today.
//!
//! The native half of `web/src/components/ui/radio-group.tsx`, a thin
//! `@base-ui/react/radio` and `@base-ui/react/radio-group` wrapper. Every
//! value below came out of the app's own Tailwind 4.3.0. **Measured, not
//! inferred** — see the module's "How the values were measured" section for
//! how, given that no live call site can be driven to check them against a
//! running capture.
//!
//! # Wrap or build: the seam test, applied
//!
//! `native/vendor/gpui-component/src/radio.rs` has a `Radio` and a
//! `RadioGroup`. `Radio` does implement `ParentElement`, which is the shape
//! this item's brief says to look for — so it earns a closer look than
//! `tooltip` needed.
//!
//! What `ParentElement` reaches on `gpui_component::Radio` is **not** the
//! circle. `Radio::new` allocates its own `base: Div` internally — this crate
//! never constructs or holds it — and the circle
//! (`div().relative().size_4()…rounded_full().border_1().border_color(…).bg(…)`)
//! is built **inside its private `RenderOnce::render`**, unconditionally, with
//! no `appearance(false)`-style flag to turn it off. `.child()`/`.children()`
//! append to a `Vec<AnyElement>` that lands in a *second* box — the label
//! area, rendered only `when(!children.is_empty() || label.is_some())` —
//! never in place of the circle. `Styled::style()` likewise refines the
//! *row* (`self.base`, the `h_flex` wrapping circle and label), not the
//! circle. So the one box the reference needs pixel-identical
//! (`border-input`, `bg-background`/`bg-input/32`, `rounded-full`) is never a
//! `Div` this crate holds, for the same reason `tooltip`'s is not: **the seam
//! is real, but it opens onto the wrong box.**
//!
//! A second, independent tell: `gpui_component::Radio` bundles a **label** —
//! `React`'s primitive does not. `web/src/components/ui/radio-group.tsx`'s
//! `Radio` is `RadioPrimitive.Root` alone; every live label
//! (`merge-popover.tsx`'s `<label><Radio /><div>…</div></label>`) is the call
//! site's own markup, a sibling, not a child the primitive lays out. Wrapping
//! the vendor's `Radio` would reproduce a shape `radio-group.tsx` does not
//! have, on top of not being able to anchor the shape it does. **Verdict:
//! built**, the same way `checkbox` and `dropdown_menu` are: raw `div()`s
//! under this module's own anchors.
//!
//! # Reachability: measured, and it is zero
//!
//! `radio-group.tsx`'s only importer is
//! `web/src/features/git/components/merge-popover.tsx`, and that popover
//! needs a workspace that is a **child branch with an unprotected local
//! parent** — not merely a dirty worktree, which `popover`'s own
//! `commit-popover` needed and this item's dev environment already has (an
//! `oracle-fixture` project, one workspace, dirty). That environment's one
//! workspace is `home` — the repo root, not a branch — and no UI path in it
//! creates a child branch: the `Switch workspace` and `Switch project`
//! pickers list one workspace and no "new branch" affordance, and its
//! `Workspaces` panel's tree is empty. **Live count: 0 of 1** — the same
//! honest zero this item's brief asked for rather than a fabricated
//! reference.
//!
//! # How the values were measured, given zero reachability
//!
//! Not inferred from the class name — every number below was read off
//! `getComputedStyle` on `radio-group.tsx`'s **own compiled class strings**,
//! injected as a throwaway element into the live app's DOM (which already
//! carries the compiled Tailwind sheet these classes resolve against) and
//! removed immediately after. This is not a capture of the primitive
//! mounted through React — no reference JSON was written from it, and none
//! is claimed — it is the same "read the compiled rule, not the class name"
//! discipline `native/MAPPING.md` prescribes, applied without a live mount to
//! read it from. Confirmed independently: `radio-group.tsx`'s circle shares
//! `checkbox.tsx`'s exact `dark:not-data-checked:bg-input/32` substring, and
//! the injected measurement reproduces `checkbox.rs`'s own on/off pair
//! (`background`) to the token.
//!
//! # `rounded-full` is `f32::MAX`, confirmed a second time
//!
//! The injected circle reports `border-top-left-radius:
//! 340282346638528859811704183484516925440px` — `f32::MAX`, not gpui's
//! `rounded_full()` preset of `px(9999.)`. `avatar.rs` and `switch.rs`
//! establish this trap; this module is the third confirmation and the first
//! on a form control rather than a decorative shape.
//!
//! # The indicator is unanchored — `checkbox.rs`'s precedent, not re-derived
//!
//! `data-unchecked:hidden` is `display: none`, and `native/oracle/ANCHORS.md`
//! v1.11 already settles what that means for a snapshot: the DOM side keeps
//! the element mounted and emits a zero-rect record, GPUI's never exists.
//! `radio-group.tsx`'s `Indicator` carries no `data-oracle-id` for exactly
//! `checkbox.tsx`'s reason — see that file's comment, unchanged here. The
//! fill (`data-checked:bg-primary`) and the inner dot (`before:bg-primary-
//! foreground`, a `::before` — not `inset: 0`, so not pseudo-backed either
//! under `ANCHORS.md`'s own carve-out) are painted and measured by neither
//! side.
//!
//! # A group renders several radios; this surface anchors one
//!
//! Every live `RadioGroup` (`merge-popover.tsx`) holds three. `dropdown_menu`
//! already sets the precedent for this shape: `ID_ITEM` is documented "when a
//! menu has exactly one", because ids have to be unique within a snapshot and
//! the primitive file cannot invent an index a call site would have to supply.
//! With zero reachability there is no reference to check a naming scheme
//! against, so this surface takes the simpler, precedented shape rather than
//! inventing one: the group anchors its own root and **one** representative
//! [`Radio`]. Further options are real and paint, unanchored, the same way
//! `popover`'s call-site body does.

use gpui::{AnyElement, Div, ParentElement as _, Pixels, Styled as _, div, px};

use crate::anchor::{AnchorId, AnchorSink};
use crate::surfaces::rows::git_status_row::Breakpoint;
use crate::theme::{Color, Theme};

/// The group's root anchor: `RadioGroupPrimitive.Root`, `flex flex-col gap-3`.
pub const ID_GROUP: &str = "radio-group";

/// The one radio this surface measures — see the module docs' final section.
pub const ID_RADIO: &str = "radio";

/// Neither anchor paints text (`ANCHORS.md` v1.5) — `checkbox`'s own finding,
/// confirmed here on the same shape of control.
pub const CONTENT_SIZED: [&str; 0] = [];

/// Neither anchor is line-sized (`ANCHORS.md` v1.6) — no text, nothing for
/// v1.6 to compare on either side.
pub const LINE_SIZED: [&str; 0] = [];

/// `--spacing`, Tailwind's stock `0.25rem` at a 16px root.
const SPACING: f32 = 4.0;

/// `gap-3` on the group — `calc(--spacing * 3)`.
pub const GROUP_GAP: Pixels = px(SPACING * 3.0);

/// `border` on the circle — measured `1px`, unconditional (checked does not
/// remove it, unlike the shadow).
pub const BORDER_WIDTH: Pixels = px(1.0);

/// `-inset-px` on the indicator: the same single pixel [`BORDER_WIDTH`] is,
/// which is why the fill lands exactly on the circle's border box —
/// `checkbox.rs`'s `INDICATOR_INSET` shape, confirmed here independently.
pub const INDICATOR_INSET: Pixels = BORDER_WIDTH;

/// `dark:not-data-checked:bg-input/32` — the one rule that carries `selected`,
/// and the same token `checkbox.rs`'s `DARK_UNCHECKED_ALPHA` names. Confirmed
/// by injected measurement rather than assumed from the shared substring.
pub const DARK_UNCHECKED_ALPHA: f32 = 32.0;

/// `aria-invalid:border-destructive/36` — a **byte-identical substring** of
/// `checkbox.tsx`'s own rule. `checkbox.rs`'s module docs name this exactly:
/// "`select`, `checkbox`, `radio-group` and `textarea` carry the same four
/// rules and will hit this again." They do.
pub const INVALID_BORDER_ALPHA: f32 = 36.0;

/// `focus-visible:aria-invalid:border-destructive/64` — likewise identical to
/// `checkbox.tsx`'s.
pub const INVALID_FOCUS_BORDER_ALPHA: f32 = 64.0;

/// `rounded-full`, measured on the injected circle at
/// `340282346638528859811704183484516925440px` — **`f32::MAX`**, not gpui's
/// `rounded_full()` preset of `px(9999.)`. See the module docs.
pub const RADIUS: Pixels = px(f32::MAX);

/// Whether a `dark:` Tailwind variant is in force — a local copy of
/// `checkbox.rs`'s, deliberately: independently ported surfaces do not share
/// this helper, so one surface's diff cannot reach into another's file.
fn is_dark(theme: &Theme) -> bool {
    *theme == Theme::DARK
}

/// `size-4.5 sm:size-4` on the circle **and** the indicator (`-inset-px`
/// around the same extent lands the fill exactly on the circle's border box).
#[must_use]
pub const fn extent(breakpoint: Breakpoint) -> Pixels {
    px(SPACING
        * match breakpoint {
            Breakpoint::Base => 4.5,
            Breakpoint::Sm => 4.0,
        })
}

/// `aria-invalid` and `:focus-visible`, bundled so [`Radio`] stays under
/// clippy's `struct_excessive_bools` — the two only ever matter as a pair,
/// through `focus-visible:aria-invalid:border-destructive/64`.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Validity {
    /// `aria-invalid`. **Real, and it moves a compared field** — the circle's
    /// `border.color`, through the identical rule `checkbox.rs` carries.
    /// **No reference**: `radio-group.tsx` has zero call sites at all, let
    /// alone one passing it.
    pub invalid: bool,
    /// `:focus-visible`, combined with `invalid` through
    /// `focus-visible:aria-invalid:border-destructive/64`. Alone it moves
    /// nothing recorded — `radio-group.tsx` has no bare
    /// `focus-visible:border-*` rule, the same absence `checkbox.tsx` has.
    pub focused: bool,
}

/// One radio button.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct Radio {
    /// Which side of `sm:` (640px) the **viewport** is on.
    pub breakpoint: Breakpoint,
    /// `data-checked` — §8.3's `selected`, and the one real state this
    /// surface has: measured to move the circle's own `bg` in the dark
    /// table, through the same rule `checkbox.rs` documents.
    pub checked: bool,
    /// `data-disabled`. Real in the class list — `cursor-not-allowed
    /// opacity-64` — and invisible either way (`ANCHORS.md` has no opacity
    /// field, and `checkbox.rs` already establishes cursor as unrecorded).
    pub disabled: bool,
    /// `aria-invalid` and `:focus-visible` — see [`Validity`].
    pub validity: Validity,
}

impl Radio {
    /// A resting, unchecked radio at the `sm:` breakpoint — the shape every
    /// unselected `merge-popover.tsx` option renders.
    #[must_use]
    pub const fn fixture() -> Self {
        Self {
            breakpoint: Breakpoint::Sm,
            checked: false,
            disabled: false,
            validity: Validity {
                invalid: false,
                focused: false,
            },
        }
    }

    /// The circle's extent at this cell's breakpoint.
    #[must_use]
    pub const fn extent(self) -> Pixels {
        extent(self.breakpoint)
    }

    /// The circle's own background.
    ///
    /// `bg-background`, with `dark:not-data-checked:bg-input/32` overriding
    /// it for an unchecked circle in dark — `checkbox.rs`'s `background()`
    /// exactly, confirmed independently on this control by injected
    /// measurement:
    ///
    /// | | light | dark |
    /// |---|---|---|
    /// | off | `background` | `input/32` |
    /// | on | `background` | `background` |
    #[must_use]
    pub fn background(self, theme: &Theme) -> Color {
        if is_dark(theme) && !self.checked {
            theme.input.mix(DARK_UNCHECKED_ALPHA, Color::TRANSPARENT)
        } else {
            theme.background
        }
    }

    /// `selected` moves a recorded field only in the dark table — the light
    /// column measures the same colour twice, exactly as `checkbox`'s does.
    #[must_use]
    pub fn selected_moves_a_recorded_field(theme: &Theme) -> bool {
        is_dark(theme)
    }

    /// The circle's border colour, resolved through the same variant chain
    /// `checkbox.rs`'s `border_color` uses — `border-input`, with
    /// `aria-invalid:border-destructive/36` over it and
    /// `focus-visible:aria-invalid:border-destructive/64` over that.
    #[must_use]
    pub fn border_color(self, theme: &Theme) -> Color {
        match (self.validity.invalid, self.validity.focused) {
            (true, true) => theme
                .destructive
                .mix(INVALID_FOCUS_BORDER_ALPHA, Color::TRANSPARENT),
            (true, false) => theme
                .destructive
                .mix(INVALID_BORDER_ALPHA, Color::TRANSPARENT),
            (false, _) => theme.input,
        }
    }

    /// The indicator's fill: `data-checked:bg-primary`. Painted, never
    /// anchored — see the module docs.
    #[must_use]
    pub fn indicator_background(theme: &Theme) -> Color {
        theme.primary
    }

    /// The inner dot's colour: `before:bg-primary-foreground`. A `::before`
    /// that is not `inset: 0`, so not pseudo-backed under `ANCHORS.md`'s own
    /// carve-out — painted, never anchored.
    #[must_use]
    pub fn dot_color(theme: &Theme) -> Color {
        theme.primary_foreground
    }

    /// The circle, opting itself into `anchors`. The indicator and its dot
    /// are painted as its children when checked and omitted entirely when
    /// not — `data-unchecked:hidden` is `display: none`, which this port
    /// reproduces by not rendering the element at all, the same choice
    /// `checkbox.rs` documents for its own tick.
    #[must_use]
    pub fn render(self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let extent = self.extent();
        let mut circle = div()
            .relative()
            .flex_shrink_0()
            .w(extent)
            .h(extent)
            .rounded(RADIUS)
            .border(BORDER_WIDTH)
            .border_color(self.border_color(theme))
            .bg(self.background(theme));
        if self.checked {
            circle = circle.child(self.indicator(theme, extent));
        }
        anchors.boxed(AnchorId::from(ID_RADIO), circle)
    }

    /// The fill span and its dot — painted, never anchored.
    fn indicator(self, theme: &Theme, extent: Pixels) -> Div {
        div()
            .absolute()
            .inset(-INDICATOR_INSET)
            .flex()
            .items_center()
            .justify_center()
            .w(extent)
            .h(extent)
            .rounded(RADIUS)
            .bg(Self::indicator_background(theme))
            .child(self.dot(theme))
    }

    /// `before:size-2 sm:before:size-1.5` — the inner dot, as a real box
    /// standing in for the `::before` `radio-group.tsx` paints it as.
    fn dot(self, theme: &Theme) -> Div {
        let side = match self.breakpoint {
            Breakpoint::Base => px(SPACING * 2.0),
            Breakpoint::Sm => px(SPACING * 1.5),
        };
        div()
            .w(side)
            .h(side)
            .rounded(RADIUS)
            .bg(Self::dot_color(theme))
    }
}

/// A group of radios. Every live one (`merge-popover.tsx`) holds three; this
/// surface anchors its root and one representative [`Radio`] — see the module
/// docs' final section for why.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct RadioGroup {
    /// The one radio this surface measures.
    pub radio: Radio,
}

impl RadioGroup {
    /// The resting group: one unchecked radio at the `sm:` breakpoint.
    #[must_use]
    pub const fn fixture() -> Self {
        Self {
            radio: Radio::fixture(),
        }
    }

    /// `flex flex-col gap-3` around the one anchored radio.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let group = div()
            .flex()
            .flex_col()
            .gap(GROUP_GAP)
            .child(self.radio.render(theme, anchors));
        anchors.root(AnchorId::from(ID_GROUP), group)
    }
}

#[cfg(test)]
mod tests {
    use super::{
        BORDER_WIDTH, CONTENT_SIZED, DARK_UNCHECKED_ALPHA, GROUP_GAP, ID_GROUP, ID_RADIO,
        INDICATOR_INSET, INVALID_BORDER_ALPHA, INVALID_FOCUS_BORDER_ALPHA, LINE_SIZED, RADIUS,
        Radio, RadioGroup, Validity, extent,
    };
    use crate::surfaces::rows::git_status_row::Breakpoint;
    use crate::theme::{Color, Theme};
    use gpui::px;

    /// `rounded-full` is `f32::MAX`, not gpui's `rounded_full()` preset of
    /// `px(9999.)` — the trap `avatar.rs` and `switch.rs` already name,
    /// confirmed here by injected `getComputedStyle` measurement rather than
    /// assumed from the class name.
    #[test]
    fn rounded_full_is_f32_max_and_not_gpuis_preset() {
        assert_eq!(RADIUS, px(f32::MAX));
        assert!(f32::from(RADIUS) > 9999.0);
    }

    /// The two breakpoints match `checkbox`'s own numbers on this shape of
    /// control — coincidence of the design system's scale, not a shared
    /// constant (mod.rs keeps every surface's independent).
    #[test]
    fn the_extent_matches_the_compiled_spacing_multiple() {
        assert_eq!(extent(Breakpoint::Base), px(4.0 * 4.5));
        assert_eq!(extent(Breakpoint::Sm), px(4.0 * 4.0));
        assert_eq!(GROUP_GAP, px(4.0 * 3.0));
        assert_eq!(BORDER_WIDTH, px(1.0));
        assert_eq!(INDICATOR_INSET, BORDER_WIDTH);
    }

    /// `selected` moves the circle's own `bg` only in the dark table — the
    /// same asymmetry `checkbox.rs` documents, confirmed here by injected
    /// measurement of `radio-group.tsx`'s own compiled classes.
    #[test]
    fn selected_moves_a_field_only_in_dark() {
        assert!(Radio::selected_moves_a_recorded_field(&Theme::DARK));
        assert!(!Radio::selected_moves_a_recorded_field(&Theme::LIGHT));

        let off = Radio {
            checked: false,
            ..Radio::fixture()
        };
        let on = Radio {
            checked: true,
            ..Radio::fixture()
        };
        assert_ne!(off.background(&Theme::DARK), on.background(&Theme::DARK));
        assert_eq!(
            off.background(&Theme::LIGHT),
            on.background(&Theme::LIGHT),
            "the light table measures the same colour twice",
        );
    }

    /// The dark "off" background really is the mixed `input/32`, and "on"
    /// really is the bare `background` — not merely "different from each
    /// other", which a bug that swapped the two conditions would also
    /// satisfy.
    #[test]
    fn the_dark_backgrounds_are_the_named_tokens_and_not_merely_distinct() {
        let theme = Theme::DARK;
        let off = Radio {
            checked: false,
            ..Radio::fixture()
        };
        let on = Radio {
            checked: true,
            ..Radio::fixture()
        };
        assert_eq!(
            off.background(&theme),
            theme.input.mix(DARK_UNCHECKED_ALPHA, Color::TRANSPARENT),
        );
        assert_eq!(on.background(&theme), theme.background);
    }

    /// Neither anchor paints text, matching `checkbox`'s own finding on the
    /// same shape of control.
    #[test]
    fn neither_anchor_is_content_or_line_sized() {
        assert_eq!(CONTENT_SIZED, [] as [&str; 0]);
        assert_eq!(LINE_SIZED, [] as [&str; 0]);
    }

    /// The two ids are distinct, and the group's contains the radio's own —
    /// `tooltip`'s namespacing convention, applied here too.
    #[test]
    fn the_anchor_ids_are_distinct_and_namespaced() {
        assert_ne!(ID_GROUP, ID_RADIO);
        assert!(ID_GROUP.starts_with(ID_RADIO));
    }

    /// `aria-invalid:border-destructive/36`, and
    /// `focus-visible:aria-invalid:border-destructive/64` over it —
    /// `checkbox.rs`'s own chain, reached here on the byte-identical rule.
    #[test]
    fn invalid_moves_the_border_and_focus_deepens_it() {
        let resting = Radio::fixture();
        let invalid = Radio {
            validity: Validity {
                invalid: true,
                focused: false,
            },
            ..Radio::fixture()
        };
        let invalid_focused = Radio {
            validity: Validity {
                invalid: true,
                focused: true,
            },
            ..Radio::fixture()
        };

        let theme = Theme::DARK;
        assert_eq!(resting.border_color(&theme), theme.input);
        assert_eq!(
            invalid.border_color(&theme),
            theme
                .destructive
                .mix(INVALID_BORDER_ALPHA, Color::TRANSPARENT),
        );
        assert_eq!(
            invalid_focused.border_color(&theme),
            theme
                .destructive
                .mix(INVALID_FOCUS_BORDER_ALPHA, Color::TRANSPARENT),
        );
        assert_ne!(
            invalid.border_color(&theme),
            invalid_focused.border_color(&theme)
        );

        // Focus alone — no bare `focus-visible:border-*` rule — moves nothing.
        let focused_only = Radio {
            validity: Validity {
                invalid: false,
                focused: true,
            },
            ..Radio::fixture()
        };
        assert_eq!(focused_only.border_color(&theme), theme.input);
    }

    /// The fixture is the resting, unchecked cell at `sm:`.
    #[test]
    fn the_fixture_is_resting_and_unchecked() {
        let group = RadioGroup::fixture();
        assert!(!group.radio.checked);
        assert!(!group.radio.disabled);
        assert_eq!(group.radio.breakpoint, Breakpoint::Sm);
    }
}
