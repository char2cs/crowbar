//! `input` — the first surface in this port that paints **editable** text, and
//! the first whose text the oracle cannot see at all.
//!
//! The native half of `web/src/components/ui/input.tsx`, which is two elements:
//! a `<span data-slot="input-control">` carrying every painted property, and an
//! `<input data-slot="input">` inside it carrying the box the caret sits in.
//! Every value below came out of the app's own `index.css` through its own
//! Tailwind 4.3.0 — see `native/mapping/input.md`.
//!
//! # An `<input>` has no text node, so the contract's whole text group is gone
//!
//! This is the headline, and it is a property of the *extractor* rather than a
//! judgement call. `web/src/lib/oracle/extract.ts` builds an anchor's
//! `text`/`fg`/`text_width`/`clipped`/`font` from `oracleOwnText(el)`, which
//! walks `el.childNodes` for `nodeType === 3`. An `<input>` is a **void
//! element**: it has no child nodes at all. Measured on the live app rather than
//! reasoned about — `input.childNodes.length` is `0`, `oracleOwnText` returns
//! `''`, and `document.createRange().selectNodeContents(input).getClientRects()`
//! returns **zero rects**, so `oracleTextAdvanceWidth` is `0` too. Even the
//! fallback clip signal is dead: with the word `Search` showing,
//! `scrollWidth === clientWidth === 224`.
//!
//! So `extractSnapshot` takes the `text.length > 0` branch **never** for this
//! element, and the reference's record for `input` carries *only* the box:
//! `bounds`, `bg`, `visible`, `radius`, `border`.
//!
//! **What follows for the port.** The string this component paints goes through
//! no [`AnchorSink`] text method at all. Routing it through
//! [`AnchorSink::text_half`] would record five fields on the native side that the
//! reference cannot produce — the `git-row-badge` shape with the sides swapped,
//! which `tabs` already met for a call site's label span. Here it is worse,
//! because it would look reasonable: the field really does paint its own string,
//! and it is still unanchorable.
//!
//! # The caret and the selection are paint with no field
//!
//! `ANCHORS.md` §6 lists what the schema deliberately cannot express. A **caret**
//! and a **selection highlight** are not on that list because they are not
//! properties of a box at all: the caret is a blinking one-pixel rule the user
//! agent draws inside the field's content box, and the selection is
//! `::selection`'s background under a run of glyphs. Neither has an element,
//! neither can carry `data-oracle-id`, and §3's pseudo-backed shortcut does not
//! reach them — it is defined for `position:absolute; inset:0` pseudos and reads
//! `getComputedStyle(el, '::before')`. No field is invented for them here. They
//! are simply outside the contract, and this component does not paint either.
//!
//! # Two elements, one of them where every trap lives
//!
//! Both of `native/MAPPING.md`'s border traps are on this one component, on two
//! different elements:
//!
//! * the **control** carries a bare `border`, so `border.w` is **1** in every
//!   cell — `button`'s trap, and skipping it because the field "has no visible
//!   border" would be wrong on every cell of a field that is compared *exactly*;
//! * `has-focus-visible:ring-[3px]` is a **box-shadow**, not a border — the
//!   `ring-1` trap `MAPPING.md` records twice — so a focused control's
//!   `border.w` is still 1, never 4.
//!
//! The **field** has no border at all: Tailwind's preflight sets `border: 0
//! solid` on `*` and `input.tsx` writes no `border-*` on it. Measured live:
//! `borderTopWidth: "0px"`.
//!
//! # `rounded-[inherit]`, which is a compared field with no number of its own
//!
//! The field's radius is `border-radius: inherit` — the control's, whatever that
//! is. Measured live: **10px on both**. It is reproduced by reading the same
//! token twice rather than by writing `px(10.0)` anywhere, so a project that
//! moved `--radius` moves the pair together.

use gpui::{
    AnyElement, BoxShadow, Div, IntoElement as _, ParentElement as _, Pixels, Rems, SharedString,
    Styled as _, div, px, relative,
};

use crate::anchor::{AnchorId, AnchorSink};
use crate::surfaces::rows::git_status_row::Breakpoint;
use crate::theme::{Color, Theme};

/// The root anchor: the `<span data-slot="input-control">` that carries the
/// border, the background and the radius.
///
/// It **has** to be the root: it is the outer of the two elements, and
/// `native/oracle/ANCHORS.md` §4 puts every other bound relative to it.
pub const ID_CONTROL: &str = "input-control";

/// The `<input data-slot="input">` itself — the box the value, the placeholder
/// and the caret are painted in, and of which the contract can see only the box.
pub const ID_FIELD: &str = "input";

/// The anchors on this surface whose boxes size to their own text
/// (`native/oracle/ANCHORS.md` v1.5).
///
/// **None**, and on this surface it is not even close. Both anchors are
/// `w-full`: the control is 100% of its parent and the field is 100% of the
/// control's content box. An `<input>`'s intrinsic width comes from its `size`
/// attribute (a *character count*), and `w-full` overrides it — so nothing here
/// is a max-content text box even in principle.
pub const CONTENT_SIZED: [&str; 0] = [];

/// The anchors on this surface whose **box height is their own line box**
/// (`native/oracle/ANCHORS.md` v1.6).
///
/// **None — and this is the closest call in the port so far**, because the two
/// numbers are *equal*. The field is authored `h-6.5` (26px) and `leading-6.5`
/// (26px), and the same coincidence holds for all three sizes at both
/// breakpoints. [`Size::extent`] and [`Size::line_height`] are separate
/// functions precisely so the equality is a measurement rather than one constant
/// read twice, and `a_fields_height_equals_its_line_height_and_it_is_still_not_line_sized`
/// pins it.
///
/// Declaring it anyway would be wrong for a **third** reason, distinct from the
/// two `ANCHORS.md` v1.6 already records:
///
/// * `git-row-badge`: the box is authored and the line box is a different
///   number, so declaring it manufactures a delta.
/// * `tabs`'s tab: the box is authored and the element contains text it does not
///   own.
/// * **here**: the reference emits **no `font` at all** for an `<input>` (see
///   the module docs), and v1.6 compares `bounds.h` against
///   `reference.font.line_height`. There is nothing on the other side of the
///   comparison. Two authored declarations that agree is not a height *derived
///   from* a line box, and the differ could not check it if it were.
pub const LINE_SIZED: [&str; 0] = [];

/// `--spacing`, Tailwind's stock `0.25rem` at a 16px root.
///
/// Spelled once, as `dropdown_menu`, `resizable` and `tabs` spell it, so a theme
/// that moved the step would move every length below together. `theme.css` does
/// **not** redefine it.
const SPACING: f32 = 4.0;

/// `border` on the control, as an `f32` so [`Size::padding_x`] can be arithmetic
/// rather than another literal.
const BORDER_PX: f32 = 1.0;

/// `border` on the control — **a real 1px border on every cell**.
///
/// `ANCHORS.md` v1.1 compares `border.w` *exactly*, and the live control reports
/// `borderTopWidth: "1px"` at rest. The variants change only the *colour*; there
/// is no state in which this becomes zero.
pub const BORDER_WIDTH: Pixels = px(BORDER_PX);

/// `before:rounded-[calc(var(--radius-lg)-1px)]` — how much smaller the `::before`
/// overlay's radius is than the control's.
///
/// A pixel, and the same pixel [`BORDER_WIDTH`] is: the overlay covers the
/// control's padding box, which is inset by the border all round, so its radius
/// follows in. Measured live at **9px** against the control's 10.
pub const OVERLAY_RADIUS_INSET: Pixels = px(BORDER_PX);

/// `has-focus-visible:ring-[3px]` — **a box-shadow**, 3px of spread with no blur.
///
/// The `ring-1` trap `native/MAPPING.md` records twice, at three times the width.
/// A port that reached for `.border_4()` here would report `border.w: 4` against
/// the reference's `1`, on the one field the contract compares exactly.
pub const RING_SPREAD: Pixels = px(3.0);

/// `ring-ring/24` — the resting ring colour, which paints only once
/// `has-focus-visible:ring-[3px]` gives the ring a width.
pub const RING_ALPHA: f32 = 24.0;

/// `has-focus-visible:has-aria-invalid:ring-destructive/16`.
pub const INVALID_RING_ALPHA: f32 = 16.0;

/// `dark:has-aria-invalid:ring-destructive/24`.
///
/// Equal specificity to [`INVALID_RING_ALPHA`]'s rule — `:has()` takes its
/// argument's specificity, so both are `(0,3,0)` — and Tailwind emits the `dark:`
/// variant later, so **in the dark table this one wins**. Read off the compiled
/// sheet rather than assumed.
pub const INVALID_RING_ALPHA_DARK: f32 = 24.0;

/// `has-aria-invalid:border-destructive/36`.
pub const INVALID_BORDER_ALPHA: f32 = 36.0;

/// `has-focus-visible:has-aria-invalid:border-destructive/64`.
pub const INVALID_FOCUS_BORDER_ALPHA: f32 = 64.0;

/// `dark:bg-input/32` — the control's background in the dark table.
pub const DARK_BACKGROUND_ALPHA: f32 = 32.0;

/// `placeholder:text-muted-foreground/72`.
///
/// Painted, and **invisible to the differ**: it colours the `::placeholder`
/// pseudo, which has no DOM node, and the reference emits no `fg` for an
/// `<input>` anyway. `getComputedStyle(input, '::placeholder').color` on the live
/// app returns the *element's* colour rather than the pseudo's, so this is not
/// even readable by hand through that door.
pub const PLACEHOLDER_ALPHA: f32 = 72.0;

/// `has-disabled:opacity-64`.
///
/// **Invisible**: `ANCHORS.md` v1.7's `visible` term fires only at *zero*, and
/// the contract has no opacity field. Painted for fidelity, exactly as
/// `button`'s identical `disabled:opacity-64` is.
pub const DISABLED_OPACITY: f32 = 0.64;

/// `text-base`'s line height — `--text-base--line-height`, `calc(1.5 / 1)`.
const CONTROL_LINE_HEIGHT_BASE: f32 = 1.5;

/// `sm:text-sm`'s line height — `--text-sm--line-height`, `calc(1.25 / 0.875)`.
const CONTROL_LINE_HEIGHT_SM: f32 = 1.25 / 0.875;

/// How much smaller a size's `sm:` variant is, in `--spacing` steps.
///
/// **One step, uniformly**, on the height and on the line box, for all three
/// sizes: `h-7.5 sm:h-6.5`, `h-8.5 sm:h-7.5`, `h-9.5 sm:h-8.5`, and the same six
/// numbers again for `leading-*`. Read off the compiled sheet rather than
/// noticed — writing it once is what keeps a size table from being six literals
/// that could drift apart.
const SM_STEP_DROP: f32 = 1.0;

/// `1` at 640px and above, `0` below it — [`SM_STEP_DROP`]'s multiplier.
const fn breakpoint_steps(breakpoint: Breakpoint) -> f32 {
    match breakpoint {
        Breakpoint::Base => 0.0,
        Breakpoint::Sm => 1.0,
    }
}

/// Whether a `dark:` Tailwind variant is in force.
///
/// The React app switches on a `dark` class on the document element, which is
/// the same switch that selects the token table. A local copy of
/// `dropdown_menu`'s and `git_status_row`'s, deliberately: the components are
/// ported independently and a shared helper would make one surface's diff reach
/// into another's file.
fn is_dark(theme: &Theme) -> bool {
    *theme == Theme::DARK
}

/// `size` on `Input` — but only the three **named** ones.
///
/// The prop is `'sm' | 'default' | 'lg' | number`, and the numeric arm is a trap
/// rather than a fourth size: `input.tsx` passes it straight through to the
/// `<input size={…}>` **attribute**, which is a character count, and the class
/// list then falls into the `default` arm because `size === 'sm'` is false. With
/// `w-full` on the field the attribute changes nothing at all. So a numeric size
/// *is* [`Size::Default`] plus an inert attribute, and this enum is the whole
/// visual vocabulary.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Size {
    /// `size="sm"`: `h-7.5 sm:h-6.5`, `px-[calc(--spacing(2.5)-1px)]`.
    ///
    /// **What the live reference is.** Four call sites pass it, including
    /// `file-explorer-tree.tsx`'s filter, which is the one on screen.
    Sm,
    /// The prop's own default: `h-8.5 sm:h-7.5`, `px-[calc(--spacing(3)-1px)]`.
    ///
    /// Fifteen of the twenty `<Input` call sites pass no `size` at all, so this
    /// is the most-used arm — it is simply not the one the capture could be
    /// taken from.
    #[default]
    Default,
    /// `size="lg"`: `h-9.5 sm:h-8.5`, and the base padding.
    ///
    /// **No reference.** No `<Input` in `web/src/` passes it.
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

    /// Whether a live `<Input` call site asks for it.
    #[must_use]
    pub const fn live(self) -> bool {
        match self {
            Self::Sm | Self::Default => true,
            Self::Lg => false,
        }
    }

    /// The `h-*` step this size authors below 640px.
    ///
    /// Its own table, not shared with [`Size::leading_step`], because they are
    /// two independent declarations in the class list that happen to agree — see
    /// [`LINE_SIZED`].
    const fn height_step(self) -> f32 {
        match self {
            Self::Sm => 7.5,
            Self::Default => 8.5,
            Self::Lg => 9.5,
        }
    }

    /// The `leading-*` step this size authors below 640px.
    const fn leading_step(self) -> f32 {
        match self {
            Self::Sm => 7.5,
            Self::Default => 8.5,
            Self::Lg => 9.5,
        }
    }

    /// The field's **authored** height: `h-*`, or `sm:h-*` at 640px and above.
    ///
    /// Deliberately a different function from [`Size::line_height`] even though
    /// the two return the same number.
    #[must_use]
    pub const fn extent(self, breakpoint: Breakpoint) -> Pixels {
        px(SPACING * (self.height_step() - SM_STEP_DROP * breakpoint_steps(breakpoint)))
    }

    /// The field's **authored** line box: `leading-*`, or `sm:leading-*` at 640px
    /// and above.
    ///
    /// `leading-<n>` in Tailwind 4 is `calc(var(--spacing) * n)` — an absolute
    /// length, not a ratio — so this is a number of pixels rather than a
    /// multiplier. It equals [`Size::extent`] for every size at every
    /// breakpoint, which is how the field's single line of text comes out
    /// vertically centred without any `items-center` anywhere.
    #[must_use]
    pub const fn line_height(self, breakpoint: Breakpoint) -> Pixels {
        px(SPACING * (self.leading_step() - SM_STEP_DROP * breakpoint_steps(breakpoint)))
    }

    /// The field's horizontal padding: `px-[calc(--spacing(2.5)-1px)]` for `sm`,
    /// `px-[calc(--spacing(3)-1px)]` otherwise — **9px and 11px, not 10 and 12**.
    ///
    /// The `-1px` pays for the *control's* border out of the *field's* padding,
    /// so the visible inset from the outer edge stays at the round number. It is
    /// the same arithmetic `button` and `tabs` both write, and the same trap: a
    /// port that read `px-2.5` would be one pixel wide on each side.
    ///
    /// `lg` keeps the base value — the `lg` arm sets heights and leading only.
    #[must_use]
    pub const fn padding_x(self) -> Pixels {
        match self {
            Self::Sm => px(SPACING * 2.5 - BORDER_PX),
            Self::Default | Self::Lg => px(SPACING * 3.0 - BORDER_PX),
        }
    }

    /// The gutter a `leftIcon` clears: `ps-7` for `sm`, `ps-8` otherwise.
    ///
    /// A whole `--spacing` multiple with **no** `-1px`, unlike
    /// [`Size::padding_x`] — the class really is `ps-7`/`ps-8`. It replaces the
    /// inline-start half of `px-*` rather than adding to it: both survive
    /// tailwind-merge, and the compiled sheet emits every `padding-inline-start`
    /// rule *after* every `padding-inline` one, so `ps-*` wins on that side.
    /// Read off the compiled sheet, not assumed.
    #[must_use]
    pub const fn icon_gutter(self) -> Pixels {
        match self {
            Self::Sm => px(SPACING * 7.0),
            Self::Default | Self::Lg => px(SPACING * 8.0),
        }
    }

    /// The `leftIcon`'s own box: `size-3.5` for `sm`, `size-4` otherwise.
    #[must_use]
    pub const fn icon_size(self) -> Pixels {
        match self {
            Self::Sm => px(SPACING * 3.5),
            Self::Default | Self::Lg => px(SPACING * 4.0),
        }
    }

    /// Where the `leftIcon` sits: `start-2` for `sm`, `start-2.5` otherwise.
    #[must_use]
    pub const fn icon_inset(self) -> Pixels {
        match self {
            Self::Sm => px(SPACING * 2.0),
            Self::Default | Self::Lg => px(SPACING * 2.5),
        }
    }

    /// The control's font size, which the field inherits.
    ///
    /// `text-base` is Tailwind's 1rem and `--ui-text-lg` is the same 1rem;
    /// `sm:text-sm` is 0.875rem and `--ui-text-base` is the same 0.875rem. The
    /// `--ui-text-*` trade `native/MAPPING.md` states once for the whole port:
    /// read the crowbar token that carries Tailwind's number and say so at the
    /// call site, because there is no token for Tailwind's own scale and minting
    /// one would put a value in the design system the design system does not
    /// have.
    ///
    /// **The field inherits it rather than declaring it**, and that is a fact
    /// about Tailwind's preflight rather than about the class list: `input.tsx`
    /// writes `text-base sm:text-sm` on the *control*, and preflight's
    /// `button, input, … { font: inherit }` is what carries it down. Without
    /// that rule an `<input>` would keep the UA's own 13.33px form font.
    #[must_use]
    pub fn text_size(breakpoint: Breakpoint, theme: &Theme) -> Rems {
        match breakpoint {
            Breakpoint::Base => theme.ui_text_lg.value(),
            Breakpoint::Sm => theme.ui_text_base.value(),
        }
    }

    /// The control's own line height, as a ratio of its font size.
    ///
    /// Reaches nothing the differ can see — the control's only child is the
    /// field, which overrides it with its own `leading-*` — and reproduced
    /// because it is the declaration the field's cascade starts from.
    #[must_use]
    pub const fn control_line_height(breakpoint: Breakpoint) -> f32 {
        match breakpoint {
            Breakpoint::Base => CONTROL_LINE_HEIGHT_BASE,
            Breakpoint::Sm => CONTROL_LINE_HEIGHT_SM,
        }
    }
}

/// The `ps-*` a **call site** merges onto the control.
///
/// A parameter for the reason [`super::button::RadiusClass`] is one: `input.tsx`
/// puts a call site's `className` on the *control*, so the call site is half the
/// component, and this is the one merged class that reaches a **compared** field
/// — it moves the field's `bounds.x` and `bounds.w` inside the control.
///
/// It names the **class**, never a number. `--class-ps 20` is a rejection, for
/// P3.1's reason: a numeric knob would let a future cell be tuned to whatever
/// the reference happened to report, where a class is an *input* both engines
/// then resolve through their own `--spacing`.
///
/// The vocabulary is closed at what the app actually writes. Exactly one
/// `<Input` in `web/src/` puts a `ps-*` on the control, and inventing arms for
/// steps nothing uses would be advertising pictures the reference cannot make.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum LeadingPad {
    /// No `ps-*`: the primitive's own control, which authors no padding at all.
    ///
    /// Nineteen of the twenty call sites, and the field then starts one border
    /// pixel from the control's left edge. **It has a live, visible reference** —
    /// unlike `button`'s unmerged radius, whose only two instances are clipped
    /// out by the carousel: the git review reply box reports the field at
    /// `x: 1, w: 452.42` inside a `454.42×28` control.
    /// `/tmp/p3-ref-input-unpadded.json` is the archived half of it.
    None,
    /// `ps-5` — 20px.
    ///
    /// `file-explorer-tree.tsx`'s tree filter, which clears the `start-2.5`
    /// magnifier the *call site* renders as a sibling of the control. The
    /// default here, because that call site is the live reference.
    #[default]
    Ps5,
}

/// Every leading pad class, for the tests and the usage line.
pub const ALL_LEADING_PADS: [LeadingPad; 2] = [LeadingPad::None, LeadingPad::Ps5];

impl LeadingPad {
    /// Its word on the command line and in a caption — the class, spelled out.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::None => "none",
            Self::Ps5 => "ps-5",
        }
    }

    /// The padding it puts on the control's inline-start edge.
    #[must_use]
    pub const fn value(self) -> Pixels {
        match self {
            Self::None => px(0.0),
            Self::Ps5 => px(SPACING * 5.0),
        }
    }
}

/// Which of three strings the field shows.
///
/// Two vocabularies rather than one, because a placeholder and a value are
/// different kinds of string: a placeholder is a prompt and a value is what a
/// user typed. `Normal`'s placeholder is `Search` — the live reference's own.
///
/// **Nothing here is comparable.** See the module docs: the reference emits no
/// `text` for an `<input>`, so the `--content` axis cannot fail on any cell of
/// this surface. The strings exist so that the native side paints what the
/// reference paints, not so that the differ can check it.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Text {
    /// A string comfortably inside the field.
    Short,
    /// The live reference's.
    #[default]
    Normal,
    /// Longer than any field on this surface, so that the native side clips
    /// where the reference's `text-overflow: clip` would.
    Overflow,
}

/// Every content length, for the tests.
pub const ALL_TEXTS: [Text; 3] = [Text::Short, Text::Normal, Text::Overflow];

impl Text {
    /// The `placeholder` attribute this length carries.
    #[must_use]
    pub const fn placeholder(self) -> &'static str {
        match self {
            Self::Short => "Go",
            Self::Normal => "Search",
            Self::Overflow => "Search files, folders and symbols across this whole workspace",
        }
    }

    /// The `value` this length carries.
    #[must_use]
    pub const fn value(self) -> &'static str {
        match self {
            Self::Short => "a.ts",
            Self::Normal => "resolve-terminal-connection.ts",
            Self::Overflow => "src/features/terminal/lib/resolve-terminal-connection.ts",
        }
    }
}

/// The three states the control's `has-*` variants select on.
///
/// **Parameters, not gpui refinements**: `ANCHORS.md` §6 makes runtime
/// interaction state invisible to the extractor, so a component that expressed
/// these as `.hover(…)`/`.focus(…)` would report its resting appearance in every
/// cell and converge while proving nothing.
///
/// A struct rather than three fields on [`Input`] for the reason `button`'s
/// `Interaction`/`Props` split is one: they are one kind of thing — the
/// conditions the control's variant chain reads — and `struct_excessive_bools`
/// is the lint that asks the question.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct State {
    /// `:focus-visible` on the field, which the control reads through
    /// `has-focus-visible:`.
    ///
    /// **Real, and it moves a compared field** — `has-focus-visible:border-ring`
    /// is a border *colour*, not a shadow. It is also unreachable: measured on
    /// the live app, `document.hasFocus()` is `false` and stays false through
    /// `window.focus()` and Tauri's own `setFocus()`, so `:focus` never matches
    /// and `:focus-visible` cannot either.
    pub focused: bool,
    /// The `disabled` prop, which the control reads through `has-disabled:`.
    ///
    /// Live — one call site passes it — and invisible: `opacity-64` has no field
    /// and does not reach v1.7's zero, and `shadow-none` is `ANCHORS.md` §6.
    pub disabled: bool,
    /// `aria-invalid` on the field, which the control reads through
    /// `has-aria-invalid:`.
    ///
    /// **Real, and it moves a compared field**: the control's `border.color`.
    /// **No reference** — no `<Input` in `web/src/` passes `aria-invalid`,
    /// although four sibling primitives (`select`, `checkbox`, `radio-group`,
    /// `textarea`) carry the same rules.
    pub invalid: bool,
}

/// One `Input`: the control, its optional leading icon, and the field.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Input {
    /// `size`.
    pub size: Size,
    /// Which side of `sm:` (640px) the **viewport** is on.
    pub breakpoint: Breakpoint,
    /// The `ps-*` a call site merged onto the control.
    pub leading_pad: LeadingPad,
    /// Whether the call site passes the `leftIcon` **prop**.
    ///
    /// An **empty box**, as on every surface in this port: the icon is a
    /// component a call site chooses, there is no native equivalent, and drawing
    /// a substitute would put a shape on screen for the oracle to converge on.
    ///
    /// `false` in the fixture, and that is the *live* picture rather than a
    /// simplification: the magnifier next to the tree filter is a `<Search>` the
    /// **call site** renders as a **sibling of the control**, outside the root
    /// anchor entirely, and the control's `ps-5` is what clears it. Three other
    /// call sites do pass the prop.
    pub icon: bool,
    /// The `placeholder` attribute.
    pub placeholder: SharedString,
    /// The `value`, where the field holds one.
    ///
    /// `None` is the live reference: the tree filter is captured with
    /// `value === ""`, so what is on screen is the placeholder.
    pub value: Option<SharedString>,
    /// The three `has-*` conditions.
    pub state: State,
}

impl Input {
    /// The live tree filter, as
    /// `web/src/features/file-explorer/file-explorer/components/file-explorer-tree.tsx`
    /// renders it: `nativeInput size="sm" placeholder="Search" className="ps-5"`,
    /// empty, at rest.
    ///
    /// **This call site rather than one of the other nineteen**, for the reason
    /// `tabs` picked the sidebar tab bar: it is the one on screen in the fixture
    /// workspace, and its root's subtree holds its own two anchors and nothing
    /// else — the control's only children are the field and, at three other call
    /// sites, an icon that carries no anchor. `ANCHORS.md` v1.8 is satisfied
    /// without a declaration.
    ///
    /// `nativeInput` versus the base-ui `Input` makes no difference to any of
    /// this: both branches render an `<input>` with the same class list and the
    /// same `data-slot`, and both spread `{...props}` last.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            size: Size::Sm,
            breakpoint: Breakpoint::Sm,
            leading_pad: LeadingPad::Ps5,
            icon: false,
            placeholder: SharedString::new_static(Text::Normal.placeholder()),
            value: None,
            state: State::default(),
        }
    }

    /// The string the field paints — the value where it has one, the placeholder
    /// otherwise.
    ///
    /// **Deliberately not routed through [`AnchorSink`]**, on either branch. See
    /// the module docs: an `<input>` has no text node, so the reference emits no
    /// `text`, `fg`, `text_width`, `clipped` or `font` for it, and recording any
    /// of them here would be five `FieldPresence` deltas per cell.
    #[must_use]
    pub fn painted(&self) -> &SharedString {
        self.value.as_ref().unwrap_or(&self.placeholder)
    }

    /// Whether the field is showing its placeholder — §8.3's `empty`.
    #[must_use]
    pub const fn is_empty(&self) -> bool {
        self.value.is_none()
    }

    /// The control's background.
    ///
    /// `bg-background`, or `dark:bg-input/32` in the dark table. Not the same
    /// token with a different value — two *different* tokens, one of them mixed
    /// down, so a port that read only `theme.background` would be wrong on every
    /// dark cell and the live app is dark.
    ///
    /// `has-autofill:bg-foreground/4` and its `dark:` twin are **absent**: gpui
    /// has no autofill, `WebKit` paints it from the platform, and no cell can
    /// reach it.
    #[must_use]
    pub fn background(&self, theme: &Theme) -> Color {
        if is_dark(theme) {
            theme.input.mix(DARK_BACKGROUND_ALPHA, Color::TRANSPARENT)
        } else {
            theme.background
        }
    }

    /// The control's border colour, resolved through the variant chain.
    ///
    /// The order is **compiled and read**, not assumed. `has-aria-invalid:` and
    /// `has-focus-visible:` are both `(0,2,0)` — `:has()` takes its argument's
    /// specificity — so source order decides, and the compiled sheet emits
    /// `border-color: var(--ring)` *before* `has-aria-invalid`'s. So invalid
    /// beats focus, and the doubly-qualified `/64` rule at `(0,3,0)` beats both.
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

    /// The ring's colour, which paints only while [`State::focused`].
    ///
    /// **Invisible on every cell** — it is a box-shadow and `ANCHORS.md` §6 has
    /// no field for one. Kept correct anyway, because a reader comparing this
    /// file against the class list should not have to wonder.
    #[must_use]
    pub fn ring_color(&self, theme: &Theme) -> Color {
        if !self.state.invalid {
            return theme.ring.mix(RING_ALPHA, Color::TRANSPARENT);
        }
        let alpha = if is_dark(theme) {
            INVALID_RING_ALPHA_DARK
        } else {
            INVALID_RING_ALPHA
        };
        theme.destructive.mix(alpha, Color::TRANSPARENT)
    }

    /// Whether `shadow-xs/5` survives.
    ///
    /// `has-[:disabled,:focus-visible,[aria-invalid]]:shadow-none` drops it in
    /// all three of the states this component models, so the resting cell is the
    /// only one that carries a shadow at all — and none of it is visible to the
    /// differ either way.
    #[must_use]
    pub const fn has_shadow(&self) -> bool {
        !(self.state.disabled || self.state.focused || self.state.invalid)
    }

    /// The colour the painted string takes.
    ///
    /// `text-foreground` for a value — inherited from the control — and
    /// `placeholder:text-muted-foreground/72` for a placeholder. gpui has no
    /// `::placeholder`, so the port resolves the pseudo into the run it is
    /// actually painting, which is the only expression available and costs
    /// nothing: no anchor on this surface reports an `fg`.
    #[must_use]
    pub fn text_color(&self, theme: &Theme) -> Color {
        if self.is_empty() {
            theme
                .muted_foreground
                .mix(PLACEHOLDER_ALPHA, Color::TRANSPARENT)
        } else {
            theme.foreground
        }
    }

    /// The `::before` overlay's radius: the control's less one pixel.
    ///
    /// Derived from the same token the control reads rather than written as
    /// `px(9.0)`, so a project that moved `--radius` moves the pair together.
    #[must_use]
    pub fn overlay_radius(theme: &Theme) -> Pixels {
        theme.radius_lg.value() - OVERLAY_RADIUS_INSET
    }

    /// Renders the control and the field, opting both into `anchors`.
    ///
    /// The root **is** the outermost element: `RowSurface` already draws the
    /// surface inside a `--width` box, and `w-full` is what the control does with
    /// it. Nothing above the root is needed, unlike `button`'s flex row — the
    /// control's `inline-flex` blockifies to `flex` in the reference because
    /// every live control is a flex item, and gpui's `.flex()` reaches the same
    /// computed value without inline flow existing at all.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let mut children: Vec<AnyElement> = vec![Self::overlay(theme).into_any_element()];
        if self.icon {
            children.push(self.icon_box(theme).into_any_element());
        }
        children.push(anchors.boxed(AnchorId::from(ID_FIELD), self.field(theme)));

        anchors.root(ID_CONTROL.into(), self.control(theme).children(children))
    }

    /// The control's own box — every painted property on this component.
    fn control(&self, theme: &Theme) -> Div {
        let mut element = div()
            .relative()
            .flex()
            .w(relative(1.0))
            .pl(self.leading_pad.value())
            .rounded(theme.radius_lg.value())
            .border(BORDER_WIDTH)
            .border_color(self.border_color(theme).value())
            .bg(self.background(theme))
            .text_size(Size::text_size(self.breakpoint, theme))
            .line_height(relative(Size::control_line_height(self.breakpoint)))
            .text_color(theme.foreground);

        if self.has_shadow() {
            // `shadow-xs/5`, and gpui's preset is **byte-identical** to what the
            // app compiles: `0 1px 2px 0` at `rgb(0 0 0 / .05)`, measured live as
            // `oklab(0 0 0 / 0.05) 0px 1px 2px 0px`. Unlike `tabs`, whose
            // `shadow-black/10` needed an alpha no preset carries, this one lands
            // exactly — which also settles rule 4, since the literal lives inside
            // gpui rather than in a component.
            element = element.shadow_xs();
        }
        if self.state.focused {
            element = ring(element, self.ring_color(theme));
        }
        if self.state.disabled {
            element = element.opacity(DISABLED_OPACITY);
        }
        element
    }

    /// The `::before` overlay.
    ///
    /// **Deliberately unanchored**, and the reason is `button`'s rather than
    /// `resizable`'s: §3's pseudo-backed shortcut is *legal* here — the pseudo
    /// really is `position:absolute; inset:0` — and taking it would still be
    /// wrong, because a pseudo-backed anchor **replaces** the host's record.
    /// Anchoring it would throw away the control's own background, its 1px border
    /// and its 10px radius, which are the whole surface.
    ///
    /// Its two inset shadows are **not painted**:
    /// `not-has-disabled:…:before:shadow-[0_1px_--theme(--color-black/4%)]` and
    /// the dark `[0_-1px_--theme(--color-white/6%)]` need Tailwind's own black
    /// and white, which `check-invariants.sh` rule 4 will not let a component
    /// mint — `Theme::LIGHT.card` is the one door open for white, and there is
    /// none for black. Measured live, the overlay's computed `box-shadow` in dark
    /// is `oklab(1 0 0 / 0.06) 0 -1px 0 0`.
    ///
    /// Measured live, so that the numbers exist somewhere no gate would notice
    /// them going wrong: control `246×28` radius **10**, overlay `244×26` at
    /// `0,0` radius **9**.
    fn overlay(theme: &Theme) -> Div {
        div()
            .absolute()
            .top(px(0.0))
            .right(px(0.0))
            .bottom(px(0.0))
            .left(px(0.0))
            .rounded(Self::overlay_radius(theme))
    }

    /// The `leftIcon`'s empty box: `absolute inset-y-0 my-auto`, `start-2`/`-2.5`.
    ///
    /// Unanchored on both sides — it is a component a call site passes — and out
    /// of flow either way, so it can move nothing the differ compares.
    fn icon_box(&self, theme: &Theme) -> Div {
        div()
            .absolute()
            .top(px(0.0))
            .bottom(px(0.0))
            .left(self.size.icon_inset())
            .my_auto()
            .flex_shrink_0()
            .w(self.size.icon_size())
            .h(self.size.icon_size())
            .text_color(theme.muted_foreground)
    }

    /// The `<input>`'s own box, and the string it paints.
    ///
    /// The string is a plain unanchored child. See the module docs — this is the
    /// decision the whole item turns on.
    fn field(&self, theme: &Theme) -> Div {
        let mut element = div()
            .w(relative(1.0))
            .min_w(px(0.0))
            .h(self.size.extent(self.breakpoint))
            // `rounded-[inherit]`: the control's radius, read from the same token
            // rather than repeated as a number.
            .rounded(theme.radius_lg.value())
            .px(self.size.padding_x())
            .line_height(self.size.line_height(self.breakpoint))
            .text_color(self.text_color(theme))
            .overflow_hidden();

        if self.icon {
            // `ps-7`/`ps-8` replaces the inline-start half of `px-*`. Written
            // after `.px(…)` for the same reason the compiled sheet emits it
            // after: later wins.
            element = element.pl(self.size.icon_gutter());
        }
        element.child(self.painted().clone())
    }
}

/// Adds `has-focus-visible:ring-[3px]`'s single shadow layer in front of whatever
/// shadow `element` already carries.
///
/// A local copy of the one `dropdown_menu`, `resizable`, `tabs` and `button` each
/// carry, for their reason: the components are ported independently and a shared
/// helper would make one surface's diff reach into another's file.
///
/// There is **no offset layer** here, unlike `button`'s. `input.tsx` writes no
/// `ring-offset-*`, so `--tw-ring-offset-width` stays at its `0px` initial and
/// the offset layer keeps its `0 0 #0000` initial — the same reading `resizable`
/// made of `ring-offset-background` with no width beside it.
fn ring(mut element: Div, color: Color) -> Div {
    let halo = BoxShadow::new(px(0.0), px(0.0), color.value()).spread_radius(RING_SPREAD);
    element
        .style()
        .box_shadow
        .get_or_insert_with(Vec::new)
        .insert(0, halo);
    element
}

#[cfg(test)]
mod tests {
    use super::{
        ALL_LEADING_PADS, ALL_SIZES, ALL_TEXTS, BORDER_WIDTH, CONTENT_SIZED, DARK_BACKGROUND_ALPHA,
        DISABLED_OPACITY, ID_CONTROL, ID_FIELD, INVALID_BORDER_ALPHA, INVALID_FOCUS_BORDER_ALPHA,
        INVALID_RING_ALPHA, INVALID_RING_ALPHA_DARK, Input, LINE_SIZED, LeadingPad,
        OVERLAY_RADIUS_INSET, PLACEHOLDER_ALPHA, RING_ALPHA, RING_SPREAD, Size, State, Text,
    };
    use crate::surfaces::rows::git_status_row::Breakpoint;
    use crate::theme::{Color, Theme};
    use gpui::px;

    /// Every length, against the `calc(var(--spacing) * n)` the app's own
    /// Tailwind 4.3.0 compiles the class to, at `--spacing: 0.25rem` and a 16px
    /// root. Spelled as the arithmetic rather than as more literals, so a change
    /// to the step cannot pass by leaving the literals alone.
    #[test]
    fn every_length_is_the_compiled_spacing_multiple() {
        const STEP: f32 = 4.0;

        // `h-7.5 sm:h-6.5` / `h-8.5 sm:h-7.5` / `h-9.5 sm:h-8.5`.
        assert_eq!(Size::Sm.extent(Breakpoint::Base), px(STEP * 7.5));
        assert_eq!(Size::Sm.extent(Breakpoint::Sm), px(STEP * 6.5));
        assert_eq!(Size::Default.extent(Breakpoint::Base), px(STEP * 8.5));
        assert_eq!(Size::Default.extent(Breakpoint::Sm), px(STEP * 7.5));
        assert_eq!(Size::Lg.extent(Breakpoint::Base), px(STEP * 9.5));
        assert_eq!(Size::Lg.extent(Breakpoint::Sm), px(STEP * 8.5));

        // `ps-7` / `ps-8`, `size-3.5` / `size-4`, `start-2` / `start-2.5`.
        assert_eq!(Size::Sm.icon_gutter(), px(STEP * 7.0));
        assert_eq!(Size::Default.icon_gutter(), px(STEP * 8.0));
        assert_eq!(Size::Sm.icon_size(), px(STEP * 3.5));
        assert_eq!(Size::Default.icon_size(), px(STEP * 4.0));
        assert_eq!(Size::Sm.icon_inset(), px(STEP * 2.0));
        assert_eq!(Size::Default.icon_inset(), px(STEP * 2.5));

        // `ps-5`, the call site's.
        assert_eq!(LeadingPad::Ps5.value(), px(STEP * 5.0));
        assert_eq!(LeadingPad::None.value(), px(0.0));

        // The two that are not spacing multiples and must not become them:
        // `border` and the overlay's inset are literal pixels in Tailwind's own
        // scale, and `ring-[3px]` is an arbitrary value.
        assert_eq!(BORDER_WIDTH, px(1.0));
        assert_eq!(OVERLAY_RADIUS_INSET, px(1.0));
        assert_eq!(RING_SPREAD, px(3.0));

        // `lg` shares the base padding: its arm sets heights and leading only.
        assert_eq!(Size::Lg.icon_gutter(), Size::Default.icon_gutter());
        assert_eq!(Size::Lg.padding_x(), Size::Default.padding_x());
    }

    /// **The field's horizontal padding has the control's border subtracted out**
    /// — 9 and 11, not 10 and 12 — which is the whole point of writing the class
    /// as `px-[calc(--spacing(2.5)-1px)]` rather than as `px-2.5`.
    ///
    /// The pixel comes off the *field's* padding to pay for a border on the
    /// *control*, which is a different element. That is what makes it worth
    /// asserting rather than assuming.
    #[test]
    fn the_fields_padding_has_the_controls_border_subtracted_out() {
        assert_eq!(Size::Sm.padding_x(), px(9.0));
        assert_eq!(Size::Default.padding_x(), px(11.0));
        assert_eq!(Size::Sm.padding_x() + BORDER_WIDTH, px(4.0 * 2.5));
        assert_eq!(Size::Default.padding_x() + BORDER_WIDTH, px(4.0 * 3.0));

        // And the icon gutter is a *whole* step, with no such subtraction —
        // `ps-7`/`ps-8` are plain utilities, not arbitrary `calc`s.
        assert_eq!(Size::Sm.icon_gutter(), px(28.0));
        assert_eq!(Size::Default.icon_gutter(), px(32.0));
    }

    /// **A field's authored height equals its authored line height — and it is
    /// still not `line_sized`.**
    ///
    /// The near-miss `LINE_SIZED` exists to record. Both numbers are authored,
    /// separately, in the same class list (`h-6.5` and `leading-6.5`), and their
    /// agreement is what centres the single line of text without any
    /// `items-center`. It is *not* a height derived from a line box, and the
    /// reference could not check it if it were: an `<input>` has no text node, so
    /// the DOM extractor emits no `font` for it at all and v1.6's comparison
    /// would have nothing on the other side.
    #[test]
    fn a_fields_height_equals_its_line_height_and_it_is_still_not_line_sized() {
        for size in ALL_SIZES {
            for breakpoint in [Breakpoint::Base, Breakpoint::Sm] {
                assert_eq!(
                    size.extent(breakpoint),
                    size.line_height(breakpoint),
                    "{size:?}/{breakpoint:?}",
                );
            }
        }

        assert_eq!(LINE_SIZED, [] as [&str; 0]);
        assert_eq!(CONTENT_SIZED, [] as [&str; 0]);
    }

    /// The fixture is the live tree filter: `size="sm"`, `ps-5`, `placeholder`
    /// showing, no `leftIcon` prop, at rest.
    #[test]
    fn the_fixture_is_the_live_tree_filter() {
        let input = Input::fixture();

        assert_eq!(input.size, Size::Sm);
        assert_eq!(input.breakpoint, Breakpoint::Sm);
        assert_eq!(input.leading_pad, LeadingPad::Ps5);
        assert_eq!(input.placeholder, "Search");
        assert_eq!(input.state, State::default());

        // **The magnifier next to the live field is not this component's.** The
        // call site renders it as a *sibling* of the control, outside the root
        // anchor, and `ps-5` is what clears it — so the `leftIcon` prop is off.
        assert!(!input.icon);

        // Empty, so what is on screen is the placeholder rather than a value.
        assert!(input.is_empty());
        assert!(input.value.is_none());
        assert_eq!(input.painted(), "Search");

        // At `sm:`, which is the only side of 640px a real window is on, the
        // field is 26px and the control is that plus its two border pixels — the
        // 28px the live app reports.
        assert_eq!(input.size.extent(Breakpoint::Sm), px(26.0));
        assert_eq!(
            input.size.extent(Breakpoint::Sm) + BORDER_WIDTH * 2.0,
            px(28.0)
        );
    }

    /// A value replaces the placeholder in **both** the string and its colour,
    /// and the resting field is the empty one.
    #[test]
    fn a_value_replaces_the_placeholder_and_recolours_the_run() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            let empty = Input::fixture();
            assert!(empty.is_empty());
            assert_eq!(
                empty.text_color(&theme),
                theme
                    .muted_foreground
                    .mix(PLACEHOLDER_ALPHA, Color::TRANSPARENT),
            );

            let filled = Input {
                value: Some("a.ts".into()),
                ..Input::fixture()
            };
            assert!(!filled.is_empty());
            assert_eq!(filled.painted(), "a.ts");
            assert_eq!(filled.text_color(&theme), theme.foreground);

            // A 72% mix is a different colour from the token, or the `/72` would
            // be decoration — and different from the value's colour, or the
            // placeholder would be invisible in a second sense.
            assert_ne!(empty.text_color(&theme), theme.muted_foreground);
            assert_ne!(empty.text_color(&theme), filled.text_color(&theme));
        }

        // The two vocabularies are distinct strings, so a cell cannot show a
        // placeholder that reads like a value.
        for text in ALL_TEXTS {
            assert_ne!(text.placeholder(), text.value(), "{text:?}");
        }
        assert_eq!(Text::default(), Text::Normal);
        assert_eq!(Text::Normal.placeholder(), "Search");
    }

    /// **The control's background is two different tokens, not one token with
    /// two values** — and the live app is on the dark one.
    #[test]
    fn the_controls_background_is_a_different_token_in_each_theme() {
        let input = Input::fixture();

        assert_eq!(input.background(&Theme::LIGHT), Theme::LIGHT.background);
        assert_eq!(
            input.background(&Theme::DARK),
            Theme::DARK
                .input
                .mix(DARK_BACKGROUND_ALPHA, Color::TRANSPARENT),
        );
        // The mix is not the bare token, or `dark:bg-input/32` would be
        // `dark:bg-input`.
        assert_ne!(input.background(&Theme::DARK), Theme::DARK.input);
        // And it is not the light answer either, or `--theme` would be vacuous
        // on the one property this surface's whole background rests on.
        assert_ne!(input.background(&Theme::DARK), Theme::DARK.background);
    }

    /// **The border chain, in the order the compiled sheet emits it**: invalid
    /// beats focus at equal specificity, and both-at-once beats either.
    #[test]
    fn the_border_colour_resolves_invalid_over_focus() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            let resting = Input::fixture();
            assert_eq!(resting.border_color(&theme), theme.input);

            let focused = Input {
                state: State {
                    focused: true,
                    ..State::default()
                },
                ..Input::fixture()
            };
            assert_eq!(focused.border_color(&theme), theme.ring);

            let invalid = Input {
                state: State {
                    invalid: true,
                    ..State::default()
                },
                ..Input::fixture()
            };
            assert_eq!(
                invalid.border_color(&theme),
                theme
                    .destructive
                    .mix(INVALID_BORDER_ALPHA, Color::TRANSPARENT),
            );

            // Invalid wins over focus at equal specificity — the compiled sheet
            // emits `has-aria-invalid:` after `has-focus-visible:`.
            let both = Input {
                state: State {
                    focused: true,
                    invalid: true,
                    ..State::default()
                },
                ..Input::fixture()
            };
            assert_eq!(
                both.border_color(&theme),
                theme
                    .destructive
                    .mix(INVALID_FOCUS_BORDER_ALPHA, Color::TRANSPARENT),
            );
            assert_ne!(both.border_color(&theme), invalid.border_color(&theme));
            assert_ne!(both.border_color(&theme), theme.ring);

            // Every arm is a genuinely different colour, or the chain would be
            // decoration.
            assert_ne!(resting.border_color(&theme), focused.border_color(&theme));
            assert_ne!(resting.border_color(&theme), invalid.border_color(&theme));
        }
    }

    /// The ring's colour, including the one place a `dark:` variant **outranks**
    /// a rule with more qualifiers on it.
    #[test]
    fn the_dark_invalid_ring_beats_the_focused_invalid_one() {
        let invalid_focus = Input {
            state: State {
                focused: true,
                invalid: true,
                ..State::default()
            },
            ..Input::fixture()
        };

        assert_eq!(
            invalid_focus.ring_color(&Theme::LIGHT),
            Theme::LIGHT
                .destructive
                .mix(INVALID_RING_ALPHA, Color::TRANSPARENT),
        );
        assert_eq!(
            invalid_focus.ring_color(&Theme::DARK),
            Theme::DARK
                .destructive
                .mix(INVALID_RING_ALPHA_DARK, Color::TRANSPARENT),
        );
        assert!((INVALID_RING_ALPHA - INVALID_RING_ALPHA_DARK).abs() > f32::EPSILON);

        // A valid field's ring is the plain one in both tables.
        for theme in [Theme::LIGHT, Theme::DARK] {
            let focused = Input {
                state: State {
                    focused: true,
                    ..State::default()
                },
                ..Input::fixture()
            };
            assert_eq!(
                focused.ring_color(&theme),
                theme.ring.mix(RING_ALPHA, Color::TRANSPARENT),
            );
        }
    }

    /// **All three modelled states drop the shadow**, and only the resting cell
    /// keeps it — `has-[:disabled,:focus-visible,[aria-invalid]]:shadow-none`.
    #[test]
    fn every_modelled_state_drops_the_shadow() {
        assert!(Input::fixture().has_shadow());

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
            let driven = Input {
                state,
                ..Input::fixture()
            };
            assert!(!driven.has_shadow(), "{state:?}");
        }

        // The opacity is the one `button` carries too, and it does not reach
        // v1.7's zero — so `visible` stays `true` on both sides.
        assert!((DISABLED_OPACITY - 0.64).abs() < f32::EPSILON);
        const { assert!(DISABLED_OPACITY > 0.0) };
    }

    /// **The field's radius is the control's**, through `rounded-[inherit]`, and
    /// the overlay's is one pixel less. Both read the same token.
    #[test]
    fn the_overlay_is_one_pixel_rounder_in_than_the_control() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            assert_eq!(theme.radius_lg.value(), px(10.0));
            assert_eq!(Input::overlay_radius(&theme), px(9.0));
            assert_eq!(
                theme.radius_lg.value() - Input::overlay_radius(&theme),
                OVERLAY_RADIUS_INSET,
            );
        }
    }

    /// The two anchor ids are distinct and mirror the two `data-slot`s.
    #[test]
    fn the_two_anchor_ids_are_the_two_data_slots() {
        assert_eq!(ID_CONTROL, "input-control");
        assert_eq!(ID_FIELD, "input");
        assert_ne!(ID_CONTROL, ID_FIELD);
        // The root is the *outer* element: every other bound is relative to it.
        assert!(ID_CONTROL.starts_with(ID_FIELD));
    }

    /// The vocabularies a caption and a command line spell, and which arms have
    /// a live call site.
    #[test]
    fn every_option_has_a_word_and_says_whether_it_is_live() {
        assert_eq!(Size::default(), Size::Default);
        assert_eq!(ALL_SIZES.map(Size::name), ["sm", "default", "lg"]);
        assert!(Size::Sm.live());
        assert!(Size::Default.live());
        assert!(!Size::Lg.live());

        assert_eq!(LeadingPad::default(), LeadingPad::Ps5);
        assert_eq!(ALL_LEADING_PADS.map(LeadingPad::name), ["none", "ps-5"]);

        // The two sizes that share their padding still differ in height, or
        // `lg` would be `default` under another name.
        assert_ne!(
            Size::Lg.extent(Breakpoint::Sm),
            Size::Default.extent(Breakpoint::Sm),
        );
    }
}
