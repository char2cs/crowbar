//! `textarea` — built, not wrapped, and **unreachable by any parity run this
//! item's dev environment could drive**.
//!
//! The native half of `web/src/components/ui/textarea.tsx`: a
//! `<span data-slot="textarea-control">` carrying every painted property,
//! wrapping a `<textarea data-slot="textarea">`, the same two-element shape
//! `input.rs`'s module docs already predicted for it: "`textarea`, `select`,
//! `checkbox` and `radio-group` are all next" for the void-element reason §2
//! below restates. Every value below came out of the app's own compiled
//! Tailwind or was read live with `getComputedStyle` on a throwaway element
//! carrying `textarea.tsx`'s own compiled class strings — see §5 for why a
//! throwaway rather than a mounted capture, and `native/mapping/textarea.md`
//! for the account in full.
//!
//! # Wrap or build: the seam test, applied
//!
//! `native/vendor/gpui-component/src/input/` has a multi-line `Input` mode
//! (`InputState::multi_line`, `input/mode.rs`). It is the same primitive
//! `input.rs` already applied the seam test to and built rather than wrapped:
//! the whole editable text element — cursor, selection, line wrapping, the
//! works — is `input/element.rs`'s 100KB+ `InputElement`, a low-level `Element`
//! impl with no `ParentElement`/`Styled` seam a caller could reach *through*
//! to supply the one box this surface needs anchored. `Input::prefix`/
//! `::suffix` exist, land *inside* that private element the same way
//! `number_input.rs`'s module docs found for its own `Input::new(&state)`
//! child, and `textarea.tsx` uses neither — it is a bare `<textarea>`, no
//! affix slots at all. **Verdict: built**, from raw `div()`s — `input.rs`'s
//! own call, confirmed independently on the multi-line half of the same
//! vendor primitive rather than assumed to carry over.
//!
//! # Reachability: measured, and it is zero
//!
//! `textarea.tsx`'s only importer is
//! `web/src/features/git/components/commit-popover.tsx`, and that popover
//! needs the sidebar's Git panel "Changes" list to hold at least one file —
//! `popover.md` §0 already reports this exact call site unreached ("needs a
//! dirty worktree"). This item's `oracle-fixture` project genuinely **has**
//! one: the backend's own `.../git/status` endpoint, queried directly,
//! returned six real changes (`src/a/a.ts` modified, `deleted-file.ts`
//! deleted, `staged-file.ts` staged, two `terminal/lib/*.ts` modified,
//! `untracked-new-file.ts` untracked) — confirmed live, not assumed from the
//! filenames. The sidebar's own "Changes" tab nonetheless rendered an empty
//! list across a full `location.reload()` **and** a real `POST .../git/stage`
//! call made through the same endpoint (confirmed to succeed, `200`, then
//! reverted) — a frontend staleness this item did not introduce and is not
//! scoped to fix. **Live count: 1 importer, 0 reachable**, the honest zero
//! `radio_group.rs` already set the precedent for reporting rather than
//! routing around.
//!
//! # 1. The primitive
//!
//! ```text
//! <span data-slot="textarea-control">   ← every painted property
//!   <textarea data-slot="textarea">     ← the box the value/caret sit in
//! </span>
//! ```
//!
//! # 2. The field has no text node — `input.md` §1, confirmed on `<textarea>`
//! independently
//!
//! `input.md` predicted this and this module confirms it directly rather
//! than by inheritance: a React-controlled `<textarea value=…>` sets the DOM
//! `.value` **property**, never a child text node. Measured on a throwaway
//! element: `textarea.childNodes.length` is **0** with `value = "Fix the
//! thing"` set. So exactly `input.rs`'s reasoning applies, word for word: no
//! `AnchorSink` text method is used, and the reference (were one reachable)
//! would carry no `text`/`fg`/`text_width`/`clipped`/`font` for either
//! anchor.
//!
//! # 3. Values — the control
//!
//! Every "Compiles to" below is a throwaway-element measurement — see §5 —
//! not a live capture.
//!
//! | React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Oracle |
//! |---|---|---|---|
//! | `w-full` | fills the parent | `.w_full()` | `bounds.w` |
//! | `rounded-lg` | **10px** | `theme.radius_lg` | `radius` = 10 |
//! | `border border-input` | `1px`, `oklch(1 0 0 / 0.08)` | `.border(BORDER_WIDTH).border_color(theme.input)` | `border.w` = 1, `border.color` compared (`w>0`) |
//! | `bg-background`, `dark:bg-input/32` | light: the bare token; dark: `oklab(1 0 0 / 0.0256)` | [`Textarea::background`] — the **identical** two-token pair `input.rs`'s `Input::background` already documents | `bg` |
//! | `text-base sm:text-sm` (unless overridden — see §4) | `16px`/`14px` | `theme.ui_text_lg`/`ui_text_base` | invisible — inherited by the field, not painted here |
//! | `text-foreground` | `oklch(0.97 0 0)` | `theme.foreground` | invisible |
//! | `shadow-xs/5` | gpui's preset, byte-identical (`input.rs`'s finding, confirmed again) | `.shadow_xs()` | §6, no field |
//! | `has-focus-visible:ring-[3px] ring-ring/24` | one box-shadow | a `BoxShadow` | §6, no field |
//! | `has-focus-visible:border-ring` | border colour → `theme.ring` | [`Textarea::border_color`] | compared, unreachable (`document.hasFocus()` false) |
//! | `has-aria-invalid:border-destructive/36`, `has-focus-visible:has-aria-invalid:border-destructive/64` | **byte-identical** rules to `input.tsx`'s | `INVALID_BORDER_ALPHA`/`INVALID_FOCUS_BORDER_ALPHA`, same numbers as `input.rs`'s own | compared, no reference |
//! | `has-focus-visible:has-aria-invalid:ring-destructive/16`, `dark:…/24` | **byte-identical** to `input.tsx`'s | `INVALID_RING_ALPHA`/`INVALID_RING_ALPHA_DARK` | §6, no field |
//! | `has-disabled:opacity-64` | opacity | `DISABLED_OPACITY` | **invisible** — v1.7 fires only at zero |
//! | `has-[:disabled,:focus-visible,[aria-invalid]]:shadow-none` | drops the shadow | [`Textarea::has_shadow`] | §6, no field either way |
//! | `not-dark:bg-clip-padding` | `background-clip` | nothing | §5, no gpui equivalent |
//!
//! **`input.rs`'s exact `dark:bg-input/32` background pair**, confirmed
//! independently rather than assumed from the shared substring: measured
//! `oklab(1 0 0 / 0.0256)` in dark, and `0.08 × 0.32 = 0.0256` — `theme.input`
//! mixed at the same 32% `input.rs`'s own `DARK_BACKGROUND_ALPHA` names.
//!
//! # 4. Values — the field
//!
//! | React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Oracle |
//! |---|---|---|---|
//! | `w-full` | fills the control's content box | `.w_full()` | `bounds.w` |
//! | `rounded-[inherit]` | the control's own **10px**, read twice | `theme.radius_lg` | `radius` = 10 |
//! | `px-[calc(--spacing(3)-1px)]` (`default`), `-(2.5)` (`sm`), unchanged (`lg`) | **11 / 9 / 11px** | [`Size::padding_x`] | `bounds` |
//! | `py-[calc(--spacing(1.5)-1px)]` (`default`), `-(1)` (`sm`), `-(2)` (`lg`) | **5 / 3 / 7px** | [`Size::padding_y`] | folds into the min-height arithmetic |
//! | `min-h-17.5`/`16.5`/`18.5` | **70 / 66 / 74px** — a **floor**, not the content's own height | [`Size::min_height`] | `bounds.h`, see §6 |
//! | `field-sizing-content` | grows with typed content | **not modelled** — see §6 | no field either way |
//! | inherited font (`text-base`/`ui-text-sm`, depending on the call site — §6) | `14px`/`20px` measured with the live call site's own `className` | [`CallSite::text_size`] | invisible — void element, §2 |
//! | `outline-none` | `outline-style: none` | nothing | no field |
//!
//! # 5. How the values were measured, given zero reachability
//!
//! Not inferred from the class name — every number above (except §0's) came
//! off `getComputedStyle` on a `<span>`/`<textarea>` pair built with
//! `textarea.tsx`'s **own compiled class strings**, injected into the live
//! app's DOM (which already carries the compiled Tailwind sheet these
//! classes resolve against — the same discipline `radio_group.rs`'s module
//! docs establish and this module follows rather than re-derives) and
//! removed immediately after. **This is not a capture of the primitive
//! mounted through React** — no `/tmp/p3-ref-textarea.json` was written, and
//! none is claimed.
//!
//! One correction the injection needed and `radio_group.rs`'s did not:
//! `size==='sm'`/`'lg'` each add a **conflicting** `min-h-*`/`px-*`/`py-*`
//! utility over the `default` arm's, and `cn()` (`tailwind-merge`) drops the
//! earlier one — a plain string concatenation does not. The first pass
//! measured `sm` and `default` at the same 70px because both `min-h-17.5`
//! and `min-h-16.5` were present in one class list and the compiled sheet's
//! *declaration order*, not the override's, won. Re-measured with the
//! conflicting base utilities dropped by hand (matching what `tailwind-merge`
//! keeps): `sm` came back **66px**, distinct from `default`'s 70 and `lg`'s
//! 74, all three whole `--spacing` multiples as `Size::min_height` expects.
//!
//! # 6. `field-sizing: content` has no gpui equivalent, and this port does
//! not need one
//!
//! `field-sizing-content` makes a `<textarea>` grow to fit its **typed**
//! content past its `min-height` floor. GPUI lays out one static instant;
//! there is no keystroke to grow from. What this surface renders is the
//! resting, **empty** cell — `commit-popover.tsx`'s own initial state
//! (`useState('')`) — where the intrinsic content height never exceeds the
//! floor (measured: an empty throwaway textarea's own height equals its
//! `min-height` exactly, at all three sizes). So [`Textarea::field_height`]
//! is the floor, unconditionally, and growth beyond it is simply not a
//! picture this port draws — the same call `input.rs` makes about the
//! caret: not approximated, just outside what a snapshot is.
//!
//! # 7. The one real call site's own override, modelled as [`CallSite`]
//!
//! `commit-popover.tsx`'s `<Textarea className="ui-font ui-text-sm min-h-20
//! resize-none" …/>` merges onto the **control**, never the field —
//! `className` is destructured out of `TextareaProps` before `...props`
//! reaches the field's own `cn()` call, so a call site can restyle the
//! control but never touch the field's class list directly. Three real
//! effects, all measured on the injected pair with this exact class string
//! appended:
//!
//! * `ui-text-sm` (12px/18px) **replaces** the control's own `text-base`
//!   (same font-size group) and, since `textarea { font: inherit }` is
//!   Tailwind's preflight rule, cascades down into the field — measured
//!   `fontSize: 12px` on the field with this className present, `14px`
//!   without it.
//! * `min-h-20` (80px) is a **second, independent** min-height on the
//!   control, on top of the field's own 70px floor. Since the control is
//!   `inline-flex` with default (`stretch`) cross-axis alignment and one
//!   child, the floor that wins is whichever is taller: measured control
//!   `80px`, field **stretched to 78px** (`80 − 2×border`) — *taller* than
//!   the field's own unstretched 70px floor. [`CallSite::CommitMessage`]
//!   carries this as `control_min_height`, and [`Textarea::field_height`]
//!   takes the max of the field's own floor and the stretched value.
//! * `resize-none` lands on the **control**, never the field — the one
//!   element a browser's native resize handle actually reads. Measured:
//!   the field's own `resize` computes `vertical` (`WebKit`'s textarea
//!   default), unchanged by the call site's class. Not a defect this port
//!   introduces; the contract has no field for it either way (§6, no
//!   resize-handle geometry).
//!
//! `ui-font` was measured too and found inert here: the app's own default
//! sans stack is already `CalSansUI, …`, so the explicit class changes
//! nothing measurable.
//!
//! # `CONTENT_SIZED` / `LINE_SIZED`
//!
//! Both empty, for `input.rs`'s exact §1/§2 reasons: the field is a void
//! `<textarea>` (§2) with no `font` group on either side, so v1.6's
//! `bounds.h`-against-`font.line_height` comparison has nothing on the other
//! side; the control's height is either an authored `min-h-*` or a stretched
//! child, never a text run's max-content extent either axis.

use gpui::{AnyElement, Div, ParentElement as _, Pixels, SharedString, Styled as _, div, px};

use crate::anchor::{AnchorId, AnchorSink};
use crate::theme::{Color, Theme};

/// The root anchor: the `<span data-slot="textarea-control">`. Named to
/// mirror the live `data-slot`, `input.rs`'s own convention for its
/// `ID_CONTROL`.
pub const ID_CONTROL: &str = "textarea-control";

/// The `<textarea data-slot="textarea">` — the box the value and the caret
/// sit in. The contract can see only the box; see the module docs' §2.
pub const ID_FIELD: &str = "textarea";

/// Neither anchor sizes to its own text (`ANCHORS.md` v1.5) — see the module
/// docs' closing section.
pub const CONTENT_SIZED: [&str; 0] = [];

/// Neither anchor's height is derived from a line box (`ANCHORS.md` v1.6) —
/// see the module docs' closing section.
pub const LINE_SIZED: [&str; 0] = [];

/// `--spacing`, Tailwind's stock `0.25rem` at a 16px root.
const SPACING: f32 = 4.0;

/// `border` on the control — unconditional, **1px**.
pub const BORDER_WIDTH: Pixels = px(1.0);

/// `dark:bg-input/32` — the control's background mix in the dark table, the
/// identical alpha `input.rs`'s `DARK_BACKGROUND_ALPHA` names.
pub const DARK_BACKGROUND_ALPHA: f32 = 32.0;

/// `has-aria-invalid:border-destructive/36` — byte-identical to
/// `input.tsx`'s own rule.
pub const INVALID_BORDER_ALPHA: f32 = 36.0;

/// `has-focus-visible:has-aria-invalid:border-destructive/64` —
/// byte-identical to `input.tsx`'s.
pub const INVALID_FOCUS_BORDER_ALPHA: f32 = 64.0;

/// `has-focus-visible:has-aria-invalid:ring-destructive/16` —
/// byte-identical to `input.tsx`'s. Invisible either way (§6, no field).
pub const INVALID_RING_ALPHA: f32 = 16.0;

/// `dark:has-aria-invalid:ring-destructive/24` — byte-identical to
/// `input.tsx`'s.
pub const INVALID_RING_ALPHA_DARK: f32 = 24.0;

/// `has-disabled:opacity-64`. **Invisible** — `ANCHORS.md` v1.7's `visible`
/// fires only at zero.
pub const DISABLED_OPACITY: f32 = 0.64;

/// `size` — `sm` | `default` | `lg`. **No live call site passes it** —
/// `commit-popover.tsx` renders the prop's own default.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Size {
    /// `min-h-16.5`, `px-[calc(--spacing(2.5)-1px)]`, `py-[calc(--spacing(1)-1px)]`.
    Sm,
    /// The prop's own default — **what `commit-popover.tsx` renders**.
    /// `min-h-17.5`, `px-[calc(--spacing(3)-1px)]`, `py-[calc(--spacing(1.5)-1px)]`.
    #[default]
    Default,
    /// `min-h-18.5`, the `default` padding-x, `py-[calc(--spacing(2)-1px)]`.
    Lg,
}

/// Every size, for the tests and the usage line.
pub const ALL_SIZES: [Size; 3] = [Size::Sm, Size::Default, Size::Lg];

impl Size {
    /// Its word on the command line and in a caption.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Sm => "sm",
            Self::Default => "default",
            Self::Lg => "lg",
        }
    }

    /// The field's own `min-h-*` floor — measured, see the module docs' §5.
    #[must_use]
    pub const fn min_height(self) -> Pixels {
        match self {
            Self::Sm => px(SPACING * 16.5),
            Self::Default => px(SPACING * 17.5),
            Self::Lg => px(SPACING * 18.5),
        }
    }

    /// The field's own `px-*` — measured.
    #[must_use]
    pub const fn padding_x(self) -> Pixels {
        match self {
            Self::Sm => px(SPACING * 2.5 - 1.0),
            Self::Default | Self::Lg => px(SPACING * 3.0 - 1.0),
        }
    }

    /// The field's own `py-*` — measured.
    #[must_use]
    pub const fn padding_y(self) -> Pixels {
        match self {
            Self::Sm => px(SPACING * 1.0 - 1.0),
            Self::Default => px(SPACING * 1.5 - 1.0),
            Self::Lg => px(SPACING * 2.0 - 1.0),
        }
    }
}

/// What a call site merges onto the **control** — `input.rs`'s `LeadingPad`
/// shape, applied to `className` rather than a single utility, because this
/// primitive has exactly one live call site and its override bundle is one
/// class string rather than a member of a larger closed vocabulary.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum CallSite {
    /// The primitive's own defaults — no call site's `className`. **No live
    /// reference.**
    #[default]
    Bare,
    /// `commit-popover.tsx`'s `className="ui-font ui-text-sm min-h-20
    /// resize-none"` — the one live call site, and the shape every
    /// `CommitPopover` textarea renders. See the module docs' §7.
    CommitMessage,
}

impl CallSite {
    /// Its word on the command line and in a caption.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Bare => "bare",
            Self::CommitMessage => "commit-message",
        }
    }

    /// The field's font size — `ui-text-sm` under [`Self::CommitMessage`]
    /// (cascaded down from the control via `font: inherit`), the size's own
    /// `text-base`/`sm:text-sm` otherwise.
    #[must_use]
    pub fn text_size(self, theme: &Theme) -> gpui::Rems {
        match self {
            Self::Bare => theme.ui_text_lg.value(),
            Self::CommitMessage => theme.ui_text_sm.value(),
        }
    }

    /// The **control's** own extra `min-h-*`, on top of the field's floor —
    /// `min-h-20` (80px) under [`Self::CommitMessage`], none otherwise. See
    /// the module docs' §7.
    #[must_use]
    pub const fn control_min_height(self) -> Option<Pixels> {
        match self {
            Self::Bare => None,
            Self::CommitMessage => Some(px(SPACING * 20.0)),
        }
    }
}

/// The three `has-*` conditions this component's class list reads — a local
/// copy of `input.rs`'s own `State`, deliberately: independently ported
/// surfaces do not share this struct.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct State {
    /// `:focus-visible`, through `has-focus-visible:border-ring`. **Real,
    /// and unreachable** — `document.hasFocus()` is `false` on this machine,
    /// `input.rs`'s own finding, unchanged here.
    pub focused: bool,
    /// The `disabled` prop, which `commit-popover.tsx` passes while
    /// committing (`isCommitting`). Invisible — `DISABLED_OPACITY` never
    /// reaches v1.7's zero.
    pub disabled: bool,
    /// `aria-invalid`, through `has-aria-invalid:*`. **No live call site
    /// passes it** — the same absence `input.rs`'s own `State::invalid`
    /// documents, and every sibling primitive `input.md` predicted
    /// (`select`, `checkbox`, `radio-group`) has now hit it too.
    pub invalid: bool,
}

/// One `Textarea`: the control and the field.
#[derive(Clone, Debug, PartialEq)]
pub struct Textarea {
    /// `size` — see [`Size`]. **No live reference** for any arm; the sole
    /// call site renders the prop's own default.
    pub size: Size,
    /// The call site's merged `className` — see [`CallSite`].
    pub call_site: CallSite,
    /// The three `has-*` conditions — see [`State`].
    pub state: State,
    /// The value the field paints. Unanchored — see the module docs' §2.
    /// `commit-popover.tsx`'s own initial state is the empty string.
    pub value: SharedString,
}

impl Textarea {
    /// The shape every `CommitPopover` textarea would render, at rest:
    /// `size="default"` (unset), the one live call site's `className`, no
    /// `has-*` state, empty value. **Not a captured reference** — see the
    /// module docs' §5 — but the picture this port's fixture stands for.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            size: Size::Default,
            call_site: CallSite::CommitMessage,
            state: State::default(),
            value: SharedString::default(),
        }
    }

    /// The control's background — `bg-background`, or `dark:bg-input/32` —
    /// `input.rs`'s `Input::background` exactly, confirmed independently
    /// here (module docs' §3).
    #[must_use]
    pub fn background(theme: &Theme) -> Color {
        if *theme == Theme::DARK {
            theme.input.mix(DARK_BACKGROUND_ALPHA, Color::TRANSPARENT)
        } else {
            theme.background
        }
    }

    /// The control's border colour — `border-input`, with
    /// `has-aria-invalid:border-destructive/36` over it,
    /// `has-focus-visible:has-aria-invalid:border-destructive/64` over that,
    /// and `has-focus-visible:border-ring` alone. `input.rs`'s own chain,
    /// confirmed independently on the byte-identical rules.
    #[must_use]
    pub fn border_color(&self, theme: &Theme) -> Color {
        match (self.state.invalid, self.state.focused) {
            (true, true) => theme
                .destructive
                .mix(INVALID_FOCUS_BORDER_ALPHA, Color::TRANSPARENT),
            (true, false) => theme
                .destructive
                .mix(INVALID_BORDER_ALPHA, Color::TRANSPARENT),
            (false, true) => theme.ring,
            (false, false) => theme.input,
        }
    }

    /// The ring's colour — invisible on every cell (§6, no field), kept
    /// correct so a reader comparing this file against the class list does
    /// not have to wonder, `input.rs`'s own reasoning for keeping it.
    #[must_use]
    pub fn ring_color(&self, theme: &Theme) -> Color {
        if !self.state.invalid {
            return theme.ring;
        }
        let alpha = if *theme == Theme::DARK {
            INVALID_RING_ALPHA_DARK
        } else {
            INVALID_RING_ALPHA
        };
        theme.destructive.mix(alpha, Color::TRANSPARENT)
    }

    /// Whether `shadow-xs/5` survives —
    /// `has-[:disabled,:focus-visible,[aria-invalid]]:shadow-none` drops it
    /// in all three modelled states, `input.rs`'s `has_shadow` exactly.
    #[must_use]
    pub const fn has_shadow(&self) -> bool {
        !(self.state.disabled || self.state.focused || self.state.invalid)
    }

    /// The field's rendered height: its own floor, or the control's extra
    /// `min-h-*` **stretched down** through the border when that is taller
    /// — see the module docs' §7.
    #[must_use]
    pub fn field_height(&self) -> Pixels {
        let own_floor = self.size.min_height();
        let Some(control_floor) = self.call_site.control_min_height() else {
            return own_floor;
        };
        let stretched = control_floor - BORDER_WIDTH * 2.0;
        if stretched > own_floor {
            stretched
        } else {
            own_floor
        }
    }

    /// The control's own height: the field's rendered height plus its
    /// border, or the call site's own `min-h-*` when that is taller.
    #[must_use]
    pub fn control_height(&self) -> Pixels {
        let natural = self.field_height() + BORDER_WIDTH * 2.0;
        match self.call_site.control_min_height() {
            Some(floor) if floor > natural => floor,
            _ => natural,
        }
    }

    /// Renders the control and the field, opting both into `anchors`.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let field = anchors.boxed(AnchorId::from(ID_FIELD), self.field(theme));
        anchors.root(AnchorId::from(ID_CONTROL), self.control(theme).child(field))
    }

    /// The control's own box.
    fn control(&self, theme: &Theme) -> Div {
        let mut element = div()
            .relative()
            .flex()
            .w_full()
            .h(self.control_height())
            .rounded(theme.radius_lg.value())
            .border(BORDER_WIDTH)
            .border_color(self.border_color(theme))
            .bg(Self::background(theme));

        if self.state.disabled {
            element = element.opacity(DISABLED_OPACITY);
        }
        element
    }

    /// The field's own box, and the string it paints — unanchored, see the
    /// module docs' §2.
    fn field(&self, theme: &Theme) -> Div {
        div()
            .w_full()
            .h(self.field_height())
            .flex_shrink_0()
            .rounded(theme.radius_lg.value())
            .px(self.size.padding_x())
            .py(self.size.padding_y())
            .text_size(self.call_site.text_size(theme))
            .text_color(theme.foreground)
            .child(self.value.clone())
    }
}

#[cfg(test)]
mod tests {
    use super::{
        ALL_SIZES, BORDER_WIDTH, CONTENT_SIZED, CallSite, DARK_BACKGROUND_ALPHA, DISABLED_OPACITY,
        ID_CONTROL, ID_FIELD, INVALID_BORDER_ALPHA, INVALID_FOCUS_BORDER_ALPHA, INVALID_RING_ALPHA,
        INVALID_RING_ALPHA_DARK, LINE_SIZED, Size, State, Textarea,
    };
    use crate::theme::{Color, Theme};
    use gpui::px;

    /// Every length, against the compiled `calc(var(--spacing) * n)` at
    /// `--spacing: 0.25rem`.
    #[test]
    fn every_length_is_the_compiled_spacing_multiple() {
        assert_eq!(BORDER_WIDTH, px(1.0));

        assert_eq!(Size::Sm.min_height(), px(66.0));
        assert_eq!(Size::Default.min_height(), px(70.0));
        assert_eq!(Size::Lg.min_height(), px(74.0));

        assert_eq!(Size::Sm.padding_x(), px(9.0));
        assert_eq!(Size::Default.padding_x(), px(11.0));
        assert_eq!(Size::Lg.padding_x(), px(11.0));

        assert_eq!(Size::Sm.padding_y(), px(3.0));
        assert_eq!(Size::Default.padding_y(), px(5.0));
        assert_eq!(Size::Lg.padding_y(), px(7.0));

        assert_eq!(CallSite::CommitMessage.control_min_height(), Some(px(80.0)));
        assert_eq!(CallSite::Bare.control_min_height(), None);
    }

    /// **The `sm`/`default`/`lg` floors are three distinct numbers** — the
    /// module docs' §5 correction, pinned: a naive string-concatenation
    /// measurement (not deduplicating the conflicting `min-h-*` utilities
    /// the way `tailwind-merge` does) reported `sm` and `default` as the
    /// same 70px. They are not.
    #[test]
    fn the_three_size_floors_are_distinct() {
        assert_ne!(Size::Sm.min_height(), Size::Default.min_height());
        assert_ne!(Size::Default.min_height(), Size::Lg.min_height());
        assert!(Size::Sm.min_height() < Size::Default.min_height());
        assert!(Size::Default.min_height() < Size::Lg.min_height());
    }

    /// **The call site's `min-h-20` stretches the field past its own
    /// floor** — the module docs' §7 finding, pinned.
    #[test]
    fn the_call_sites_min_height_stretches_the_field_past_its_own_floor() {
        let bare = Textarea {
            call_site: CallSite::Bare,
            ..Textarea::fixture()
        };
        let commit_message = Textarea::fixture();

        assert_eq!(bare.field_height(), Size::Default.min_height());
        assert_eq!(
            bare.control_height(),
            Size::Default.min_height() + BORDER_WIDTH * 2.0
        );

        // 80 authored on the control; the field stretches to 80 - 2*1 = 78,
        // taller than its own 70px floor.
        assert_eq!(commit_message.field_height(), px(78.0));
        assert!(commit_message.field_height() > Size::Default.min_height());
        assert_eq!(commit_message.control_height(), px(80.0));
    }

    /// The field's own font is `ui-text-sm` under the live call site's
    /// className, cascaded down via `font: inherit` — measured, not assumed
    /// from the class name.
    #[test]
    fn the_call_sites_class_name_changes_the_fields_font() {
        let theme = Theme::DARK;
        assert_eq!(
            CallSite::CommitMessage.text_size(&theme),
            theme.ui_text_sm.value(),
        );
        assert_eq!(CallSite::Bare.text_size(&theme), theme.ui_text_lg.value());
        assert_ne!(
            CallSite::CommitMessage.text_size(&theme),
            CallSite::Bare.text_size(&theme),
        );
    }

    /// The control's background is two different tokens, not one token with
    /// two values — `input.rs`'s own finding, confirmed independently here.
    #[test]
    fn the_controls_background_is_a_different_token_in_each_theme() {
        assert_eq!(Textarea::background(&Theme::LIGHT), Theme::LIGHT.background);
        assert_eq!(
            Textarea::background(&Theme::DARK),
            Theme::DARK
                .input
                .mix(DARK_BACKGROUND_ALPHA, Color::TRANSPARENT),
        );
        assert_ne!(Textarea::background(&Theme::DARK), Theme::DARK.input);
        assert_ne!(Textarea::background(&Theme::DARK), Theme::DARK.background);
    }

    /// The border chain resolves invalid over focus, byte-identical to
    /// `input.rs`'s own chain.
    #[test]
    fn the_border_colour_resolves_invalid_over_focus() {
        let theme = Theme::DARK;
        let resting = Textarea::fixture();
        assert_eq!(resting.border_color(&theme), theme.input);

        let focused = Textarea {
            state: State {
                focused: true,
                ..State::default()
            },
            ..Textarea::fixture()
        };
        assert_eq!(focused.border_color(&theme), theme.ring);

        let invalid = Textarea {
            state: State {
                invalid: true,
                ..State::default()
            },
            ..Textarea::fixture()
        };
        assert_eq!(
            invalid.border_color(&theme),
            theme
                .destructive
                .mix(INVALID_BORDER_ALPHA, Color::TRANSPARENT),
        );

        let both = Textarea {
            state: State {
                focused: true,
                invalid: true,
                ..State::default()
            },
            ..Textarea::fixture()
        };
        assert_eq!(
            both.border_color(&theme),
            theme
                .destructive
                .mix(INVALID_FOCUS_BORDER_ALPHA, Color::TRANSPARENT),
        );
        assert_ne!(both.border_color(&theme), invalid.border_color(&theme));
    }

    /// The dark invalid ring beats the focused invalid one — the same
    /// `dark:` precedence `input.rs` documents.
    #[test]
    fn the_dark_invalid_ring_beats_the_focused_invalid_one() {
        let invalid = Textarea {
            state: State {
                invalid: true,
                ..State::default()
            },
            ..Textarea::fixture()
        };
        assert_eq!(
            invalid.ring_color(&Theme::LIGHT),
            Theme::LIGHT
                .destructive
                .mix(INVALID_RING_ALPHA, Color::TRANSPARENT),
        );
        assert_eq!(
            invalid.ring_color(&Theme::DARK),
            Theme::DARK
                .destructive
                .mix(INVALID_RING_ALPHA_DARK, Color::TRANSPARENT),
        );
        assert!((INVALID_RING_ALPHA - INVALID_RING_ALPHA_DARK).abs() > f32::EPSILON);
    }

    /// All three modelled states drop the shadow.
    #[test]
    fn every_modelled_state_drops_the_shadow() {
        assert!(Textarea::fixture().has_shadow());
        for state in [
            State {
                focused: true,
                ..State::default()
            },
            State {
                disabled: true,
                ..State::default()
            },
            State {
                invalid: true,
                ..State::default()
            },
        ] {
            let driven = Textarea {
                state,
                ..Textarea::fixture()
            };
            assert!(!driven.has_shadow(), "{state:?}");
        }
        assert!((DISABLED_OPACITY - 0.64).abs() < f32::EPSILON);
        const { assert!(DISABLED_OPACITY > 0.0) };
    }

    /// The fixture is the one live call site's own shape, at rest.
    #[test]
    fn the_fixture_is_the_live_call_sites_shape() {
        let fixture = Textarea::fixture();
        assert_eq!(fixture.size, Size::Default);
        assert_eq!(fixture.call_site, CallSite::CommitMessage);
        assert_eq!(fixture.state, State::default());
        assert_eq!(fixture.value, "");
    }

    /// The two anchor ids mirror the two `data-slot`s and the root is the
    /// outer element.
    #[test]
    fn the_two_anchor_ids_mirror_the_data_slots() {
        assert_eq!(ID_CONTROL, "textarea-control");
        assert_eq!(ID_FIELD, "textarea");
        assert_ne!(ID_CONTROL, ID_FIELD);
        assert!(ID_CONTROL.starts_with(ID_FIELD));
    }

    /// Neither anchor paints text, and the vocabularies are named.
    #[test]
    fn no_anchor_is_content_or_line_sized_and_every_option_is_named() {
        assert_eq!(CONTENT_SIZED, [] as [&str; 0]);
        assert_eq!(LINE_SIZED, [] as [&str; 0]);

        assert_eq!(Size::default(), Size::Default);
        assert_eq!(ALL_SIZES.map(Size::name), ["sm", "default", "lg"]);
        assert_eq!(CallSite::default(), CallSite::Bare);
        assert_eq!(CallSite::Bare.name(), "bare");
        assert_eq!(CallSite::CommitMessage.name(), "commit-message");
    }
}
