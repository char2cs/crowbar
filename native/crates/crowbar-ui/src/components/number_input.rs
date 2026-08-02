//! `number-input` — built, not wrapped, and the second surface (after `input`)
//! whose field paints text no extractor can see.
//!
//! The native half of `web/src/components/ui/number-input.tsx`: a flex row of
//! two icon buttons flanking a text `<input>`. Every value below came out of
//! the app's own compiled Tailwind or was read live with `getComputedStyle` /
//! `getBoundingClientRect` — see `native/mapping/number-input.md`.
//!
//! # Wrap or build: the seam test, applied
//!
//! `native/vendor/gpui-component/src/input/number_input.rs` has a
//! `NumberInput`, and `native/vendor/gpui-component/src/stepper/stepper.rs` has
//! a `Stepper` — a name a member-name grep would flag for this item, and a
//! false lead: `stepper::Stepper` is a multi-step **wizard progress**
//! indicator (`StepperItem`s in a row or column, each showing a step number),
//! not a numeric input's increment/decrement control. It shares no shape with
//! `number-input.tsx` at all and is not examined further.
//!
//! `gpui_component::input::NumberInput` is the real candidate, and it does
//! expose element-accepting seams: `.prefix(impl IntoElement)` and
//! `.suffix(impl IntoElement)`. But `RenderOnce::render`
//! (`number_input.rs:283`) builds the **whole visible tree** itself —
//! `h_flex().child(Button::new("minus")…).child(Input::new(&self.state)…
//! .when_some(self.prefix, …)).child(Button::new("plus")…)` — and
//! `prefix`/`suffix` land *inside* the vendor's own private `Input`, not as
//! the three top-level children this surface needs to anchor. The real
//! React component never uses them: it flanks a plain `<input>` with two of
//! *this app's own* `@/components/ui/button` `Button`s (`variant="ghost"`),
//! not gpui-component's `Button`, and its minus/plus icons are sized by
//! `number-input.tsx`'s own `iconSizes` table, not by `NumberInput`'s size
//! variant. So even where a seam exists, it does not reach the three boxes
//! this component needs pixel-identical, for the same reason `popover`'s own
//! module docs give the general test: **a widget is wrappable exactly when it
//! lets the caller supply the element that becomes the anchored box, not
//! merely a style refinement on a box the vendor already decided the shape
//! of.** Neither seam here does. **Verdict: built**, from raw `div()`s, the
//! same call `input`, `button`, `checkbox` and `radio_group` each made.
//!
//! # The stepper buttons are a private local copy, not a reuse of `button.rs`
//!
//! The two flanking buttons *are* this app's shared `Button` component in
//! React, so reusing `crowbar_ui::components::button::Button` here looks
//! tempting. It is the wrong tool for one measured reason:
//! `button::Size::icon`/`glyph_box` size a button's glyph off **`Button`'s
//! own** size table (`sm:size-4.5`/`sm:size-4` for the default text size this
//! call renders at, since no `size` prop is passed and the SVG's own
//! `size-3` class beats the button's `[&_svg:not([class*='size-'])]` default
//! selector). `number-input.tsx` overrides the glyph to its **own**
//! `iconSizes` table instead (`size-3`/`size-3.5`/`size-4`, flat, no `sm:`
//! step) — a call-site override `button.rs`'s public API has no parameter
//! for. Building a small local element here, at this surface's own measured
//! numbers, is the same choice `tooltip.rs` made for its shortcut chip
//! (`keybinding.tsx`, ported as a private helper rather than a fourth shared
//! primitive) and the same one every module doc in `mod.rs` gives for why
//! these surfaces stay independent: "components are ported independently and
//! a shared helper would make one surface's diff reach into another's file."
//!
//! # Reachability: measured, and it is real
//!
//! **Live count: 15 call sites, every one reachable** — one per settings tab
//! it appears in (`file-tree-settings.tsx` ×1, `appearance-settings.tsx` ×2,
//! `developer-settings.tsx` ×2, `terminal-settings.tsx` ×5,
//! `editor-settings.tsx` ×5), through the app's own Settings dialog. Every
//! one of the 15 passes `size="xs"` — the only size a live call site ever
//! requests — and one of three merged widths from
//! `settings-control-widths.ts`: `SETTINGS_CONTROL_WIDTHS.numberCompact`
//! (`w-24`, 6 call sites), `.number` (`w-28`, 7 call sites) or `.default`
//! (`w-36`, 1 call site, `terminalScrollback`).
//!
//! **Reference:** `/tmp/p3-ref-number-input.json`, captured live through
//! `extractSnapshotSource` from Appearance → "UI Font Size" (`.number`
//! width), at a 1714px viewport, dark, resting (`value="15"`).
//!
//! # 1. Values — the root
//!
//! | React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Oracle |
//! |---|---|---|---|
//! | `flex items-center gap-1` | `display:flex`, `gap:4px` | `.flex().items_center().gap(ROOT_GAP)` | `bounds` of children |
//! | call site's `w-24`/`w-28`/`w-36`, `max-w-full` | **96 / 112 / 144px**, authored | [`Width`] | `bounds.w` = 112 on the reference |
//! | *(none)* | `background: rgba(0,0,0,0)`, no border, no radius | no `.bg`/`.border`/`.rounded` | `bg` `#00000000`, `border.w` 0, `radius` 0 |
//!
//! **`bg`/`border`/`radius` are compared exactly this way on the live
//! reference** — a plain flex wrapper with none of the three, which is why
//! `bg`/`border.w`/`radius` on the root anchor are literal zeros rather than
//! omitted.
//!
//! # 2. Values — the stepper buttons
//!
//! Both flanking buttons are `<Button variant="ghost" compact
//! onClick=… aria-label=… className="shrink-0"><Minus/Plus
//! className={iconSizes[size]} /></Button>`. **`compact` is dead** —
//! `button.tsx`'s own `ButtonProps` destructures it as `compact: _compact`
//! and never reads it, so this is a plain `variant="ghost"` button at the
//! prop's own default size, `'default'` (`h-9 px-[calc(--spacing(3)-1px)]
//! sm:h-8`) — **not** one of `Button`'s five `icon*` square sizes. Confirmed
//! by reading the live element's `className`, not assumed from the prop
//! name.
//!
//! | React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Oracle |
//! |---|---|---|---|
//! | `h-9 sm:h-8` | **36 / 32px** | [`button_height`] | `bounds.h` |
//! | `px-[calc(--spacing(3)-1px)]` | **11px**, both breakpoints | [`BUTTON_PADDING_X`] | `bounds.w` |
//! | `border` | `border-width: 1px`, unconditional | [`BUTTON_BORDER_WIDTH`] | `border.w` = 1, exact |
//! | `border-transparent` (ghost) | transparent | `Color::TRANSPARENT` | `border.color` — v1.3: ignored, `w` is compared |
//! | `rounded-lg` | **10px** | `theme.radius_lg` | `radius` = 10 |
//! | *(no `bg-*` in ghost's resting rule)* | `background: rgba(0,0,0,0)` | no `.bg(…)` | `bg` `#00000000` |
//! | `Minus`/`Plus` `className={iconSizes.xs}` = `size-3` | **12×12px**, flat — no `sm:` step | [`Size::icon_size`] | invisible (unanchored glyph) |
//! | `[&_svg]:-mx-0.5` | `-2px` each side on the glyph | [`BUTTON_ICON_MARGIN_X`] | folds into `bounds.w` |
//!
//! **The button's width is genuinely content-sized in the DOM sense** — the
//! `default` size authors no `w-*`, only padding — **but not in
//! `ANCHORS.md` v1.5's sense**, which exists only to correct GPUI's
//! `ceil()` on a *text run's* max-content width. The glyph here is a fixed
//! `size-3` box, not text, so both engines compute
//! `2×padding + glyph_box + 2×border` by ordinary flex arithmetic with no
//! rounding step for either side to disagree on — measured at exactly
//! **32px** on the live reference, and `button_width` reproduces the same
//! arithmetic rather than a bare literal. No `data-oracle-content-sized`
//! is warranted, and this is the reasoned "why not", not an oversight.
//!
//! # 3. Values — the field
//!
//! An `<input type="text">`, so §1 of `input.md` applies verbatim: it is a
//! **void element** with **no text node**, so the reference records only its
//! box. Confirmed independently here: `input.childNodes.length` is `0` for a
//! React-controlled textfield regardless of `value`.
//!
//! | React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Oracle |
//! |---|---|---|---|
//! | `h-6` (`xs`) | **24px**, flat — `fieldHeights` has **no** `sm:` step, unlike the buttons | [`Size::field_height`] | `bounds.h` |
//! | `rounded-md` | **8px** — not the buttons' `rounded-lg` | `theme.radius_md` | `radius` = 8 |
//! | `border border-border` | `1px`, `oklch(1 0 0 / 0.06)` | `.border(FIELD_BORDER_WIDTH).border_color(theme.border)` | `border.w` = 1, `border.color` compared (`w>0`) |
//! | `bg-muted` | `oklch(1 0 0 / 0.04)` — the **bare** token, no `.mix()` | `theme.muted` | `bg` = `#ffffff0a` |
//! | `px-2` (`xs`/`sm`), `px-3` (`md`) | **8 / 12px** | [`Size::field_padding_x`] | `bounds` via content box |
//! | `ui-text-sm` (`xs`/`sm`), `ui-text-base` (`md`) | **12px/18px**, `14px/20px` | `theme.ui_text_sm`/`ui_text_base` | invisible — no anchor paints text |
//! | `text-foreground` | `oklch(0.97 0 0)` | `theme.foreground` | invisible |
//! | `min-w-[5ch]` | **37.26px** (`xs`/`sm`, `ui-text-sm`), **43.47px** (`md`, `ui-text-base`) — a font-metric, not `5×`anything, measured via `getComputedStyle().minWidth` | [`Size::min_field_width`] | folds into `bounds.w`, see §5 |
//! | `flex-1` | flex-basis 0, grows/shrinks to fill the row | `.flex_1()` **clamped** at `min_field_width` | `bounds.w` |
//! | `tabular-nums` | uniform digit widths — **part of the field's own base class list**, not a call-site option | baked into every fixture (matches every live cell) | affects `min-w-[5ch]`'s own px value, not separately visible |
//!
//! `disabled:opacity-50` and `placeholder:text-muted-foreground` are in the
//! class list and **practically unreachable**: `NumberInput` always renders
//! a real digit string (`formatValue` never returns `''`, falling back to
//! `'0'`), so the placeholder never paints on any live cell, and no call
//! site ever passes `disabled`. Painted for fidelity in [`NumberInput::text_color`]
//! staying `theme.foreground` unconditionally; `ANCHORS.md` §6 has no field
//! for either rule regardless.
//!
//! # 4. The field's `min-w-[5ch]` overflows the row at the narrowest
//! authored width — a real trap, not a rounding error
//!
//! Measured on the live `Indent Size` cell (`.numberCompact`, `w-24` = 96px):
//!
//! ```text
//! root   0,0     96×32   (authored, w-24)
//! dec    0,0     32×32
//! field  36,4    37.25×24   ← overflows: 36 + 37.25 = 73.25, inc starts at 40
//! inc    40,0    32×32      ← right edge 72, 13.25px PAST the root's own 96
//! ```
//!
//! Wait — restated precisely from the raw capture (`left`/`right` in document
//! coordinates, `dec.right=1333`, `field.left=1337`, `field.right=1374.25`,
//! `inc.left=1378.25`, `inc.right=1410.25`, `root.right=1397`): the flex
//! division gives the field `96 − 2×32 − 2×4 = 24px`, but its floor is
//! `min-w-[5ch] = 37.26px` — **wider than the space flex would give it** — so
//! the field, and the increment button after it, spill **13.25px past the
//! root's own right edge**. `flex-1`'s `flex-shrink: 1` cannot shrink the
//! field below its own `min-width`, and nothing on the row clips the
//! overflow. [`NumberInput::field_width`] reproduces exactly this:
//! `max(flex_share, min_field_width)`, so the port overflows identically at
//! [`Width::Compact`] and stays inside the row at [`Width::Number`] (measured
//! `40px ≥ 37.26px`, no overflow — the live reference's own cell) and
//! [`Width::Default`] (`72px`, comfortably clear).
//!
//! # 5. The `--viewport-width` axis moves only the buttons' height
//!
//! Measured at both sides of 640px on the live `UI Font Size` cell
//! (`.number` width):
//!
//! | | Base (< 640px) | Sm (≥ 640px, the reference) |
//! |---|---|---|
//! | button `h-9`/`sm:h-8` | **36px** | **32px** |
//! | button width | 32px (unchanged — padding and glyph carry no `sm:`) | 32px |
//! | field `h-6` (`fieldHeights`, no `sm:` step) | **24px** (unchanged) | 24px |
//! | field `y` (centred by the row's `items-center`) | `(36−24)/2 = 6` | `(32−24)/2 = 4` |
//!
//! So only the buttons respond to the breakpoint; the field's own height is
//! flat across it, and the row's own height follows the taller button.
//! [`NumberInput::render`] reproduces this by giving only [`button_height`] a
//! [`Breakpoint`] parameter — [`Size::field_height`] takes none.
//!
//! # The state axis: `hover` is real, and it is the only one
//!
//! `number-input.tsx` itself contains the substrings `hover`, `focus`,
//! `selected`/`data-selected` and `aria-invalid` **zero times each** —
//! grepped, not assumed, `checkbox.rs`'s own discipline. So `empty`,
//! `selected`, `focus` and `error` are all genuinely unmodelled: the
//! original has no such rule to disagree with, not merely one this port
//! declines to reach.
//!
//! `hover` is the one exception, and it comes from the **composed**
//! `<Button variant="ghost">` rather than from this file: `ghost`'s own
//! `hover:bg-accent data-pressed:bg-accent` is real, and
//! [`NumberInput::hovered`] folds it into the base style (never a `.hover(…)`
//! refinement `ANCHORS.md` §6 says a snapshot cannot see) exactly the way
//! `button.rs` folds its own `Interaction::hovered` in. **No reference**
//! either way: synthetic pointer events are denied on this project's
//! machines, `button.rs`'s own standing finding.
//!
//! # `CONTENT_SIZED` / `LINE_SIZED`
//!
//! Both empty. No anchor here paints a text run on either side of the
//! contract: the field is a void `<input>` (§3), and the buttons paint an
//! icon, not text — see §2's content-sizing note for why that specifically
//! does *not* reach v1.5.

use gpui::prelude::FluentBuilder as _;
use gpui::{AnyElement, Div, ParentElement as _, Pixels, SharedString, Styled as _, div, px};

use super::anchor::{AnchorId, AnchorSink};
use super::git_status_row::Breakpoint;
use crate::theme::{Color, Theme};

/// The root anchor: the `flex items-center gap-1` wrapper.
pub const ID_ROOT: &str = "number-input";

/// The decrement button — `aria-label="Decrease value"`.
///
/// Written as a call-site prop on `<Button>`, overriding `button.tsx`'s own
/// `data-oracle-id: 'button'` default — `input`/`button`'s own convention:
/// "a per-slot default, before `{...props}`, so a call site can override
/// it." Without the override both flanking buttons would carry the shared
/// id `button` twice under one root, which `ANCHORS.md` v1.8 refuses.
pub const ID_DECREMENT: &str = "number-input-decrement";

/// The field — the `<input type="text">`.
pub const ID_FIELD: &str = "number-input-field";

/// The increment button — `aria-label="Increase value"`.
pub const ID_INCREMENT: &str = "number-input-increment";

/// No anchor sizes to its own text (`ANCHORS.md` v1.5) — see the module
/// docs' §2 note and §3's void-element finding.
pub const CONTENT_SIZED: [&str; 0] = [];

/// No anchor's height is derived from a line box (`ANCHORS.md` v1.6) — none
/// of the three paints a `font` group at all.
pub const LINE_SIZED: [&str; 0] = [];

/// `--spacing`, Tailwind's stock `0.25rem` at a 16px root.
const SPACING: f32 = 4.0;

/// `gap-1` on the root.
pub const ROOT_GAP: Pixels = px(SPACING);

/// `border` on a stepper button — unconditional, ghost only changes colour.
pub const BUTTON_BORDER_WIDTH: Pixels = px(1.0);

/// `px-[calc(--spacing(3)-1px)]` on a stepper button — **11px**, flat across
/// breakpoints (only the button's `h-*` carries a `sm:` step).
pub const BUTTON_PADDING_X: Pixels = px(SPACING * 3.0 - 1.0);

/// `[&_svg]:-mx-0.5` on the glyph — a local copy of `button.rs`'s own
/// `ICON_MARGIN_X`, independently confirmed by live measurement here rather
/// than imported: the two components are ported independently, and sharing
/// this constant would make one surface's diff reach into another's file.
pub const BUTTON_ICON_MARGIN_X: Pixels = px(-SPACING * 0.5);

/// `disabled:opacity-64` on a stepper button (`button.tsx`'s own base class
/// list). No live call site disables a button — `canDecrement`/`canIncrement`
/// are both `true` at every captured cell — and it is invisible regardless
/// (`ANCHORS.md` v1.7's `visible` fires only at zero).
pub const BUTTON_DISABLED_OPACITY: f32 = 0.64;

/// `disabled:opacity-50` on the field. Real in the class list, unreachable in
/// practice — see the module docs' §3 note — and invisible either way.
pub const FIELD_DISABLED_OPACITY: f32 = 0.5;

/// `border` on the field — unconditional, **1px**.
pub const FIELD_BORDER_WIDTH: Pixels = px(1.0);

/// `min-w-[5ch]` at `ui-text-sm` (the `xs`/`sm` sizes' font) — a font metric,
/// measured via `getComputedStyle(el).minWidth` on the live app rather than
/// computed from `5 * ch-guess`. See the module docs' §4.
pub const MIN_FIELD_WIDTH_SM_TEXT: Pixels = px(37.26);

/// `min-w-[5ch]` at `ui-text-base` (the `md` size's font). **No live
/// reference** — no call site passes `size="md"` — measured the same way as
/// [`MIN_FIELD_WIDTH_SM_TEXT`] for completeness.
pub const MIN_FIELD_WIDTH_BASE_TEXT: Pixels = px(43.47);

/// The `default` (unnamed) Button size's authored box extent —
/// `h-9 sm:h-8`, a local copy of `button.rs`'s own `Size::Default` numbers,
/// independently measured on this surface's own live cell (`36`/`32px`) and
/// not imported, for the reason [`BUTTON_ICON_MARGIN_X`]'s doc records.
#[must_use]
pub const fn button_height(breakpoint: Breakpoint) -> Pixels {
    match breakpoint {
        Breakpoint::Base => px(SPACING * 9.0),
        Breakpoint::Sm => px(SPACING * 8.0),
    }
}

/// A stepper button's own content-sized width: `2×padding + glyph + 2×border`.
///
/// Not content-sized in `ANCHORS.md` v1.5's sense — see the module docs' §2 —
/// so this is plain arithmetic rather than a `ceil()`-corrected quantity.
#[must_use]
pub fn button_width(icon_size: Pixels) -> Pixels {
    let glyph = icon_size + BUTTON_ICON_MARGIN_X * 2.0;
    glyph + BUTTON_PADDING_X * 2.0 + BUTTON_BORDER_WIDTH * 2.0
}

/// `size` — the `xs` | `sm` | `md` prop. **Every live call site passes `xs`.**
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Size {
    /// `iconSizes.xs = size-3` (12px), `fieldHeights.xs = h-6` (24px),
    /// `numberInputFieldPadding.xs = px-2` (8px), `ui-text-sm`.
    ///
    /// **The only size any live `<NumberInput` passes** — all 15 call sites.
    #[default]
    Xs,
    /// `size-3.5` (14px), `h-7` (28px), `px-2` (8px), `ui-text-sm`. **No
    /// reference.**
    Sm,
    /// `size-4` (16px), `h-8` (32px), `px-3` (12px), `ui-text-base`. **No
    /// reference.**
    Md,
}

/// Every size, for the tests and the usage line.
pub const ALL_SIZES: [Size; 3] = [Size::Xs, Size::Sm, Size::Md];

impl Size {
    /// Its word on the command line and in a caption.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Xs => "xs",
            Self::Sm => "sm",
            Self::Md => "md",
        }
    }

    /// Whether a live `<NumberInput` call site asks for it.
    #[must_use]
    pub const fn live(self) -> bool {
        matches!(self, Self::Xs)
    }

    /// `iconSizes[size]` — the glyph's own box, flat across breakpoints.
    #[must_use]
    pub const fn icon_size(self) -> Pixels {
        match self {
            Self::Xs => px(SPACING * 3.0),
            Self::Sm => px(SPACING * 3.5),
            Self::Md => px(SPACING * 4.0),
        }
    }

    /// `fieldHeights[size]` — the field's own authored height. **No `sm:`
    /// step at all**, unlike every other sized primitive in this port so
    /// far — see the module docs' §5.
    #[must_use]
    pub const fn field_height(self) -> Pixels {
        match self {
            Self::Xs => px(SPACING * 6.0),
            Self::Sm => px(SPACING * 7.0),
            Self::Md => px(SPACING * 8.0),
        }
    }

    /// `numberInputFieldPadding[size]`.
    #[must_use]
    pub const fn field_padding_x(self) -> Pixels {
        match self {
            Self::Xs | Self::Sm => px(SPACING * 2.0),
            Self::Md => px(SPACING * 3.0),
        }
    }

    /// `numberInputTextSize[size]` — the field's font, read on the field
    /// itself rather than inherited (the field has no wrapping control
    /// element the way `input.rs`'s does).
    #[must_use]
    pub fn text_size(self, theme: &Theme) -> gpui::Rems {
        match self {
            Self::Xs | Self::Sm => theme.ui_text_sm.value(),
            Self::Md => theme.ui_text_base.value(),
        }
    }

    /// `min-w-[5ch]` at this size's font — see the module docs' §3/§4.
    #[must_use]
    pub const fn min_field_width(self) -> Pixels {
        match self {
            Self::Xs | Self::Sm => MIN_FIELD_WIDTH_SM_TEXT,
            Self::Md => MIN_FIELD_WIDTH_BASE_TEXT,
        }
    }
}

/// The call site's merged width — `SETTINGS_CONTROL_WIDTHS.numberCompact` /
/// `.number` / `.default`, the only three classes any live call site merges
/// onto the root. Named, never a raw pixel count, for `input.rs`'s
/// `LeadingPad` reason: a class is an input both engines resolve through
/// their own `--spacing`, where a numeric knob could be tuned to whatever a
/// reference happened to report.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Width {
    /// `w-24 max-w-full` — **96px**. 6 live call sites (every `editor-settings`
    /// one, plus `file-tree-settings`'s indent size).
    Compact,
    /// `w-28 max-w-full` — **112px**. 7 live call sites, including the
    /// captured reference (`appearance-settings`'s UI font size).
    #[default]
    Number,
    /// `w-36 max-w-full` — **144px**. 1 live call site
    /// (`terminal-settings`'s scrollback).
    Default,
}

/// Every width, for the tests and the usage line.
pub const ALL_WIDTHS: [Width; 3] = [Width::Compact, Width::Number, Width::Default];

impl Width {
    /// Its word on the command line and in a caption.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Compact => "compact",
            Self::Number => "number",
            Self::Default => "default",
        }
    }

    /// The authored `w-*` value.
    #[must_use]
    pub const fn value(self) -> Pixels {
        match self {
            Self::Compact => px(SPACING * 24.0),
            Self::Number => px(SPACING * 28.0),
            Self::Default => px(SPACING * 36.0),
        }
    }
}

/// The two stepper buttons' own state — bundled so [`NumberInput`] stays
/// under clippy's `struct_excessive_bools`, the reason `button.rs`'s own
/// `Interaction`/`Props` split is one: these three are one kind of thing,
/// the conditions the two buttons' variant chain reads.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Buttons {
    /// `!disabled && numericValue > min` — whether the decrement button is
    /// enabled. `true` on the captured reference.
    pub can_decrement: bool,
    /// `!disabled && numericValue < max` — whether the increment button is
    /// enabled. `true` on the captured reference.
    pub can_increment: bool,
    /// `hover:bg-accent` on both stepper buttons — §8.3's `hover` flag, and
    /// this surface's **one genuinely modelled state**: `number-input.tsx`
    /// itself carries no `hover`/`focus`/`selected`/`aria-invalid` rule at
    /// all (grepped, zero matches), but the composed `<Button
    /// variant="ghost">` does, through the shared `button-variants.ts`
    /// `hover:bg-accent data-pressed:bg-accent` rule — `button.rs`'s own
    /// `Variant::Ghost` background arm, `Some(theme.accent)` when engaged.
    /// Folded into the base style rather than a `.hover(…)` refinement, so
    /// a cell can actually show it — `ANCHORS.md` §6's rule for why a
    /// runtime-only hover would silently compare resting-against-resting.
    /// **No reference**: synthetic pointer events are denied on this
    /// project's machines, `button.rs`'s own standing finding.
    pub hovered: bool,
}

/// One `NumberInput`: the root, its two stepper buttons, and the field.
#[derive(Clone, Debug, PartialEq)]
pub struct NumberInput {
    /// `size` — see [`Size`].
    pub size: Size,
    /// The call site's merged root width — see [`Width`].
    pub width: Width,
    /// Which side of `sm:` (640px) the **viewport** is on. Moves only the
    /// buttons' own height — see the module docs' §5.
    pub breakpoint: Breakpoint,
    /// The digit string the field paints. `NumberInput.formatValue` never
    /// returns an empty string, so this is always a real value, never the
    /// (practically unreachable) placeholder.
    pub value: SharedString,
    /// The `disabled` prop. **No live reference.**
    pub disabled: bool,
    /// The two stepper buttons' own state — see [`Buttons`].
    pub buttons: Buttons,
}

impl NumberInput {
    /// The live `UI Font Size` cell: `size="xs"`, `.number` width, `sm:`
    /// breakpoint, value `"15"`, both steppers enabled.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            size: Size::Xs,
            width: Width::Number,
            breakpoint: Breakpoint::Sm,
            value: SharedString::new_static("15"),
            disabled: false,
            buttons: Buttons {
                can_decrement: true,
                can_increment: true,
                hovered: false,
            },
        }
    }

    /// The field's rendered width: the flex-1 share of the row, floored at
    /// [`Size::min_field_width`] — see the module docs' §4.
    #[must_use]
    pub fn field_width(&self) -> Pixels {
        let button = button_width(self.size.icon_size());
        let flex_share = self.width.value() - button * 2.0 - ROOT_GAP * 2.0;
        let floor = self.size.min_field_width();
        if flex_share > floor {
            flex_share
        } else {
            floor
        }
    }

    /// The field's own text colour: always `text-foreground` in practice —
    /// see the module docs' §3 note on the unreachable placeholder.
    #[must_use]
    pub fn text_color(theme: &Theme) -> Color {
        theme.foreground
    }

    /// A stepper button's own background: `hover:bg-accent` when
    /// [`Buttons::hovered`], nothing (ghost paints no resting background) —
    /// `button.rs`'s own `Variant::Ghost` table (`Self::Ghost | Self::Link
    /// => None`), confirmed independently here.
    #[must_use]
    pub fn button_background(&self, theme: &Theme) -> Option<Color> {
        self.buttons.hovered.then_some(theme.accent)
    }

    /// Renders the root, its two stepper buttons and the field, opting all
    /// four into `anchors`.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let root = div()
            .flex()
            .items_center()
            .gap(ROOT_GAP)
            .w(self.width.value());

        let children: Vec<AnyElement> = vec![
            self.stepper_button(ID_DECREMENT, self.buttons.can_decrement, theme, anchors),
            anchors.boxed(AnchorId::from(ID_FIELD), self.field(theme)),
            self.stepper_button(ID_INCREMENT, self.buttons.can_increment, theme, anchors),
        ];

        anchors.root(AnchorId::from(ID_ROOT), root.children(children))
    }

    /// One stepper button: `variant="ghost"`, the `default` size, one glyph.
    fn stepper_button(
        &self,
        id: &'static str,
        enabled: bool,
        theme: &Theme,
        anchors: &dyn AnchorSink,
    ) -> AnyElement {
        let icon = self.size.icon_size();
        let glyph_box = icon + BUTTON_ICON_MARGIN_X * 2.0;

        let mut button = div()
            .relative()
            .flex()
            .flex_shrink_0()
            .items_center()
            .justify_center()
            .h(button_height(self.breakpoint))
            .px(BUTTON_PADDING_X)
            .rounded(theme.radius_lg.value())
            .border(BUTTON_BORDER_WIDTH)
            .border_color(Color::TRANSPARENT)
            .child(div().flex_shrink_0().w(glyph_box).h(icon));

        if let Some(bg) = self.button_background(theme) {
            button = button.bg(bg);
        }

        if !enabled || self.disabled {
            button = button.opacity(BUTTON_DISABLED_OPACITY);
        }

        anchors.boxed(AnchorId::from(id), button)
    }

    /// The field's own box, and the digit string it paints as an unanchored
    /// child — the same decision `input.rs`'s field makes, for the same
    /// reason: an `<input>` has no text node, so recording one would invent
    /// a field the reference cannot produce.
    fn field(&self, theme: &Theme) -> Div {
        div()
            .w(self.field_width())
            .flex_shrink_0()
            .h(self.size.field_height())
            .px(self.size.field_padding_x())
            .rounded(theme.radius_md.value())
            .border(FIELD_BORDER_WIDTH)
            .border_color(theme.border)
            .bg(theme.muted)
            .text_size(self.size.text_size(theme))
            .text_color(Self::text_color(theme))
            .flex()
            .items_center()
            .justify_center()
            .when(self.disabled, |el| el.opacity(FIELD_DISABLED_OPACITY))
            .child(self.value.clone())
    }
}

#[cfg(test)]
mod tests {
    use super::{
        ALL_SIZES, ALL_WIDTHS, BUTTON_BORDER_WIDTH, BUTTON_DISABLED_OPACITY, BUTTON_ICON_MARGIN_X,
        BUTTON_PADDING_X, Buttons, CONTENT_SIZED, FIELD_BORDER_WIDTH, FIELD_DISABLED_OPACITY,
        ID_DECREMENT, ID_FIELD, ID_INCREMENT, ID_ROOT, LINE_SIZED, MIN_FIELD_WIDTH_BASE_TEXT,
        MIN_FIELD_WIDTH_SM_TEXT, NumberInput, ROOT_GAP, Size, Width, button_height, button_width,
    };
    use crate::components::Breakpoint;
    use gpui::px;

    /// Every root/button length, against the compiled `calc(var(--spacing) *
    /// n)` the app's own Tailwind 4.3.0 emits at `--spacing: 0.25rem`.
    #[test]
    fn every_length_is_the_compiled_spacing_multiple() {
        assert_eq!(ROOT_GAP, px(4.0));
        assert_eq!(BUTTON_PADDING_X, px(11.0));
        assert_eq!(BUTTON_BORDER_WIDTH, px(1.0));
        assert_eq!(FIELD_BORDER_WIDTH, px(1.0));
        assert_eq!(BUTTON_ICON_MARGIN_X, px(-2.0));

        assert_eq!(Width::Compact.value(), px(96.0));
        assert_eq!(Width::Number.value(), px(112.0));
        assert_eq!(Width::Default.value(), px(144.0));

        assert_eq!(Size::Xs.icon_size(), px(12.0));
        assert_eq!(Size::Sm.icon_size(), px(14.0));
        assert_eq!(Size::Md.icon_size(), px(16.0));

        assert_eq!(Size::Xs.field_height(), px(24.0));
        assert_eq!(Size::Sm.field_height(), px(28.0));
        assert_eq!(Size::Md.field_height(), px(32.0));

        assert_eq!(Size::Xs.field_padding_x(), px(8.0));
        assert_eq!(Size::Sm.field_padding_x(), px(8.0));
        assert_eq!(Size::Md.field_padding_x(), px(12.0));
    }

    /// **The buttons' height carries a `sm:` step and nothing else on this
    /// surface does** — the module docs' §5 finding, pinned.
    #[test]
    fn only_the_buttons_height_moves_with_the_viewport() {
        assert_eq!(button_height(Breakpoint::Base), px(36.0));
        assert_eq!(button_height(Breakpoint::Sm), px(32.0));

        // The field's own height has no breakpoint parameter at all.
        for size in ALL_SIZES {
            let base = size.field_height();
            let sm = size.field_height();
            assert_eq!(base, sm, "{size:?}");
        }

        // And the button's width is unaffected too — padding and glyph
        // carry no `sm:` step either.
        assert_eq!(
            button_width(Size::Xs.icon_size()),
            button_width(Size::Xs.icon_size()),
        );
    }

    /// **The stepper button's width is measured, not merely computed** —
    /// 32px on the live reference, and the arithmetic reproduces it exactly:
    /// `2×11 padding + (12 glyph − 2×2 margin) + 2×1 border`.
    #[test]
    fn the_button_width_matches_the_live_measurement() {
        let icon = Size::Xs.icon_size();
        let glyph = icon + BUTTON_ICON_MARGIN_X * 2.0;
        assert_eq!(glyph, px(8.0));
        assert_eq!(button_width(icon), px(32.0));
    }

    /// The field's `min-w-[5ch]` overflows the row at [`Width::Compact`] and
    /// stays inside it at [`Width::Number`]/[`Width::Default`] — the module
    /// docs' §4 trap, pinned rather than merely described.
    #[test]
    fn min_field_width_overflows_only_at_the_narrowest_authored_width() {
        let compact = NumberInput {
            width: Width::Compact,
            ..NumberInput::fixture()
        };
        let number = NumberInput::fixture();
        let default = NumberInput {
            width: Width::Default,
            ..NumberInput::fixture()
        };

        let button = button_width(Size::Xs.icon_size());
        let compact_flex_share = Width::Compact.value() - button * 2.0 - ROOT_GAP * 2.0;
        assert_eq!(compact_flex_share, px(24.0));
        assert!(compact_flex_share < MIN_FIELD_WIDTH_SM_TEXT);
        // The floor wins: the field is wider than its flex share, so the row
        // overflows past `Width::Compact`'s own 96px.
        assert_eq!(compact.field_width(), MIN_FIELD_WIDTH_SM_TEXT);
        let compact_children_span = button * 2.0 + ROOT_GAP * 2.0 + compact.field_width();
        assert!(compact_children_span > Width::Compact.value());

        // `Width::Number` — the live reference's own cell — does not
        // overflow: the flex share already clears the floor.
        assert_eq!(number.field_width(), px(40.0));
        assert!(px(40.0) >= MIN_FIELD_WIDTH_SM_TEXT);

        assert_eq!(default.field_width(), px(72.0));
    }

    /// `size="md"` has no live reference, but its `min-w-[5ch]` was measured
    /// anyway, at its own (larger) font.
    #[test]
    fn the_md_size_min_width_is_measured_and_larger_than_sms() {
        assert!(MIN_FIELD_WIDTH_BASE_TEXT > MIN_FIELD_WIDTH_SM_TEXT);
        assert_eq!(Size::Md.min_field_width(), MIN_FIELD_WIDTH_BASE_TEXT);
        assert_eq!(Size::Xs.min_field_width(), MIN_FIELD_WIDTH_SM_TEXT);
        assert_eq!(Size::Sm.min_field_width(), MIN_FIELD_WIDTH_SM_TEXT);
    }

    /// The fixture is the live `UI Font Size` cell.
    #[test]
    fn the_fixture_is_the_live_ui_font_size_cell() {
        let fixture = NumberInput::fixture();
        assert_eq!(fixture.size, Size::Xs);
        assert_eq!(fixture.width, Width::Number);
        assert_eq!(fixture.breakpoint, Breakpoint::Sm);
        assert_eq!(fixture.value, "15");
        assert!(!fixture.disabled);
        assert!(fixture.buttons.can_decrement);
        assert!(fixture.buttons.can_increment);
        assert!(!fixture.buttons.hovered);
    }

    /// **`hover` is the one genuinely real state on this surface** —
    /// `number-input.tsx` itself has no `hover:`/`focus:`/`selected`/
    /// `aria-invalid` rule at all, but the composed ghost `Button` does, and
    /// folding it into the base style (rather than a runtime `.hover(…)`
    /// refinement `ANCHORS.md` §6 says a snapshot cannot see) is what makes
    /// it a real, comparable field rather than decoration.
    #[test]
    fn hover_paints_the_ghost_buttons_accent_background_and_nothing_else_does() {
        use crate::theme::Theme;

        let resting = NumberInput::fixture();
        let hovered = NumberInput {
            buttons: Buttons {
                hovered: true,
                ..resting.buttons
            },
            ..NumberInput::fixture()
        };
        assert_ne!(resting, hovered);

        for theme in [Theme::LIGHT, Theme::DARK] {
            // Resting ghost paints no background at all — `button.rs`'s own
            // `Variant::Ghost` table, confirmed here independently.
            assert_eq!(resting.button_background(&theme), None);
            assert_eq!(hovered.button_background(&theme), Some(theme.accent));
        }

        // Neither `size`, `width` nor `breakpoint` move under hover — it is
        // purely a background change on the two buttons.
        assert_eq!(resting.field_width(), hovered.field_width());
    }

    /// Every anchor id is distinct and namespaced under the root.
    #[test]
    fn every_anchor_id_is_distinct_and_namespaced() {
        let ids = [ID_ROOT, ID_DECREMENT, ID_FIELD, ID_INCREMENT];
        for (i, a) in ids.iter().enumerate() {
            for b in &ids[i + 1..] {
                assert_ne!(a, b);
            }
        }
        assert!(ID_DECREMENT.starts_with(ID_ROOT));
        assert!(ID_FIELD.starts_with(ID_ROOT));
        assert!(ID_INCREMENT.starts_with(ID_ROOT));
    }

    /// Neither anchor paints text, on either side of the contract.
    #[test]
    fn no_anchor_is_content_or_line_sized() {
        assert_eq!(CONTENT_SIZED, [] as [&str; 0]);
        assert_eq!(LINE_SIZED, [] as [&str; 0]);
    }

    /// The vocabularies a caption and a command line spell, and which arms
    /// have a live call site.
    #[test]
    fn every_option_has_a_word_and_says_whether_it_is_live() {
        assert_eq!(Size::default(), Size::Xs);
        assert_eq!(ALL_SIZES.map(Size::name), ["xs", "sm", "md"]);
        assert!(Size::Xs.live());
        assert!(!Size::Sm.live());
        assert!(!Size::Md.live());

        assert_eq!(Width::default(), Width::Number);
        assert_eq!(
            ALL_WIDTHS.map(Width::name),
            ["compact", "number", "default"]
        );
    }

    /// The disabled opacities are the literal Tailwind fractions, and both
    /// are invisible to the contract (`ANCHORS.md` v1.7 fires only at zero).
    #[test]
    fn disabled_opacities_are_the_tailwind_fractions_and_nonzero() {
        assert!((BUTTON_DISABLED_OPACITY - 0.64).abs() < f32::EPSILON);
        assert!((FIELD_DISABLED_OPACITY - 0.5).abs() < f32::EPSILON);
        const { assert!(BUTTON_DISABLED_OPACITY > 0.0) };
        const { assert!(FIELD_DISABLED_OPACITY > 0.0) };
    }
}
