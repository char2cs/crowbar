//! `search` (P3.33) — the editor/terminal find-and-replace shell, an ordinary
//! composition with no vendor widget underneath it.
//!
//! The native half of `web/src/components/ui/search.tsx`'s three exports —
//! `SearchPopover`, `SearchReplaceToggle`, `SearchReplaceRow` — which between
//! them are a `Button` + `Input` shell and nothing else: grepped for
//! `.filter(`/`score`/`fuzzy`/`match(`/`includes(`/`indexOf`, **0 hits**. The
//! real matchers (`web/src/utils/search-match.ts`,
//! `web/src/utils/fuzzy-matcher.tsx`) live outside `components/ui/` and are
//! Phase 4 work; this module ports the boxes only. See
//! `native/mapping/search.md`.
//!
//! # Built, not wrapped
//!
//! `search.tsx` renders through no `gpui-component` primitive at all —
//! `grep -c 'gpui_component\|@radix-ui\|@base-ui'` on the React file is zero.
//! Every box is `search.tsx`'s own `<div>`s, `<Button>`s and one `<Input>`,
//! positioned by the *caller* (`find-bar.tsx`'s `absolute top-9 right-2`, not
//! by this file). So the "wrap or build" test — *"a widget is
//! wrappable-and-measurable exactly when it lets the caller supply an
//! element"* — has nothing to apply to: there is no vendor `render` to reach
//! inside of, and every `Div` this module builds is already this crate's own
//! to anchor. That is also why [`SearchReplaceToggle`]'s and
//! [`SearchReplaceRow`]'s buttons are **not** rendered through
//! [`super::button::Button::render`]: that API's `Label` enum paints a closed
//! set of hard-coded strings (`"Add"`, `"Create workspace"`, …) chosen for
//! `button`'s own captured cells, and cannot paint `"Replace"`/`"All"`
//! verbatim. Each icon-shaped button below is this module's own small `Div`,
//! carrying `button`'s sealed tokens (`theme.radius_lg`, `DISABLED_OPACITY`)
//! by reference rather than by wrapping its render path — the same "own copy,
//! shared tokens" arrangement `alert_dialog` keeps with `dialog` and
//! `dropdown::trigger`/`item` keep with nothing.
//!
//! # The size-6/`sm:h-8` trap, confirmed live in both directions
//!
//! Every icon-shaped button on this surface (`searchIconButtonVariants`,
//! `searchToggleButtonVariants`) is `flex size-6 …`, merged onto a bare
//! `<Button variant="ghost" compact>` — no `size` prop, so the base
//! `buttonVariants()` list is `Size::Default`'s own `h-9 sm:h-8`.
//! `native/MAPPING.md`'s "a call site's unprefixed class can be dead above
//! 640px" trap fires here a third time (`search-toggle-icons.md` §5 already
//! recorded it once): `size-6`'s **unprefixed** height component and
//! `buttonVariants`' own **unprefixed** `h-9` are the same tailwind-merge
//! group and modifier, so `size-6` replaces `h-9` outright — but `sm:h-8`
//! carries a *different* modifier, survives the merge untouched, and its
//! media-scoped rule sits later in Tailwind's generated sheet than `size-6`'s
//! plain one. So at a `sm` viewport **`sm:h-8` wins the cascade** and the
//! button is **24×32**, not the 24×24 `size-6` alone would suggest; below
//! `sm`, nothing competes with `size-6` and the button really is **24×24**.
//!
//! Both halves were measured live, not reasoned through in one direction only
//! (`native/mapping/search.md` §2 has the numbers): the wide capture
//! (1714px) reads every icon button at `24×32`, and the same buttons at a
//! 550px window (`document.documentElement`'s own `sm:` boundary is 640px)
//! read `24×24`. [`icon_button_extent`] is that pair, taken from
//! [`super::button::Size::Default::extent`] on the `Sm` arm rather than
//! restated, so a future change to `button`'s own `sm:h-8` moves this
//! surface with it.
//!
//! # The Replace/All buttons overturn a standing finding in `button.rs`
//!
//! `button.rs`'s module docs give `CONTENT_SIZED = []` and say why: "the
//! five non-icon sizes author no width... **no live call site renders a
//! Button with a label**". `SearchReplaceRow`'s "Replace" and "All" buttons
//! are exactly that shape — `<Button variant="ghost" compact>Replace</Button>`
//! merged with `searchActionButtonVariants` (`flex h-8 items-center
//! justify-center … px-2.5`, no `w-*`) — and they **are** live, reachable by
//! expanding the editor find bar's replace row. Measured:
//! `search-replace-confirm` `75.81×32`, `text_width 53.8`; `search-replace-all`
//! `39.38×32`, `text_width 17.37`. Both are genuinely content-sized (v1.5).
//! `button.rs` is not wrong about its own captured cells — no live `Button`
//! call site anywhere else in `web/src/` passes a label — but its universal
//! claim is now contradicted by this surface, recorded here rather than
//! silently worked around.
//!
//! # `disabled:opacity-64` beats `search`'s own `disabled:opacity-50`
//!
//! `searchIconButtonVariants`' `disabled` variant is `cursor-not-allowed
//! opacity-50` — a **plain class**, unconditional once compiled in. The nav
//! buttons also carry the real HTML `disabled` attribute
//! (`disabled={!canNavigate}`), which fires `button.tsx`'s own base-class
//! `disabled:opacity-64` — an **attribute-selector pseudo-class**, one
//! specificity tier above a plain class, so it wins regardless of source
//! order. Measured live on the disabled `search-nav-previous`/`-next` cells:
//! `opacity: 0.64`, not `0.5`. This module therefore reaches for
//! `super::button::DISABLED_OPACITY` rather than defining its own 0.5 — the
//! number that is actually painted.
//!
//! # What is captured, and what is real-but-unreached
//!
//! Two references, both taken live 2026-08-02 from the editor find bar
//! (`find-bar.tsx`, the toolbar's *Find in file* button) at a 1714px
//! viewport, dark, resting (`document.hasFocus()` false throughout;
//! `.click()` reached React):
//!
//! * `/tmp/p3-ref-search.json` — root `search-popover`, replace collapsed:
//!   the leading toggle, the input, close, three option toggles and both nav
//!   buttons (disabled, empty query).
//! * `/tmp/p3-ref-search-replace-row.json` — root `search-replace-row`,
//!   replace expanded: the swap icon, the replace input and both action
//!   buttons.
//!
//! **Not captured in the same walk**, and why: expanding the replace row
//! mounts a *second* `<Input>` while `search-popover`'s own is still mounted,
//! and `web/src/components/ui/input.tsx` hard-codes `data-oracle-id`
//! (`"input-control"`/`"input"`) with no override prop — unlike
//! `button.tsx`'s `'data-oracle-id': 'button', ...props`, which a call site
//! *can* override. Rooting the second capture at `search-replace-row`
//! (a sibling subtree, not an ancestor of the first `Input`) avoids the
//! collision; a single capture spanning both mounted `Input`s cannot, without
//! an edit to `input.tsx` this item's remit does not extend to. Recorded here
//! as a genuine, load-bearing limitation of the already-verified `input`
//! primitive, not smoothed over.
//!
//! Real but not captured, for the reasons already established elsewhere in
//! this tree: `matchLabel` (no live cell had a non-empty query — `hover`'s
//! reason, `CGPreflightPostEventAccess()` false), the `active` state on a
//! toggle button (not driven live; `search-toggle-icons.md` §6 already
//! measured the one field it moves), and the clear (`×`) button inside the
//! input (only renders with `value`, and no live cell had one).

use gpui::{
    AnyElement, Div, FontWeight, ParentElement as _, Pixels, SharedString, Styled as _, div, px,
};

use super::anchor::{AnchorId, AnchorSink};
use super::button::{self, DISABLED_OPACITY};
use super::git_status_row::Breakpoint;
use super::input;
use super::search_toggle_icons::{SearchToggleIcon, Toggle};
use crate::theme::{Color, Theme, ui_sans_font};

/// The root anchor: `SearchPopover`'s own `w-[320px] rounded-xl …` box.
pub const ID_POPOVER: &str = "search-popover";
/// `SearchReplaceToggle`'s button, used as `SearchPopover`'s `leadingControl`.
pub const ID_REPLACE_TOGGLE: &str = "search-replace-toggle";
/// The close (`×`) button.
pub const ID_CLOSE: &str = "search-close";
/// The "Match case" toggle.
///
/// `search.tsx` has no hard-coded notion of "case" — `options` is a generic
/// `SearchToggleOption[]`, and the React side spells this as a template
/// literal, `search-toggle-` concatenated with the call site's own
/// `option.id`. `find-bar.tsx`'s and `terminal-search.tsx`'s shared
/// `option.id` for this one is `"case-sensitive"`, hence the suffix here —
/// not `"case"`, which would not match anything the DOM ever emits.
pub const ID_TOGGLE_CASE: &str = "search-toggle-case-sensitive";
/// The "Match whole word" toggle — `option.id` `"whole-word"`.
pub const ID_TOGGLE_WHOLE_WORD: &str = "search-toggle-whole-word";
/// The "Use regular expression" toggle — `option.id` `"regex"`.
pub const ID_TOGGLE_REGEX: &str = "search-toggle-regex";
/// The "Preserve case" toggle — `option.id` `"preserve-case"`, `find-bar.tsx`
/// only, and only while
/// [`SearchPopover::replace`] is `Some`.
pub const ID_TOGGLE_PRESERVE_CASE: &str = "search-toggle-preserve-case";
/// The previous-match chevron.
pub const ID_NAV_PREVIOUS: &str = "search-nav-previous";
/// The next-match chevron.
pub const ID_NAV_NEXT: &str = "search-nav-next";
/// `SearchReplaceRow`'s own root — `flex items-center gap-1.5 border-t pt-1.5`.
pub const ID_REPLACE_ROW: &str = "search-replace-row";
/// The swap-icon `<span>` leading `SearchReplaceRow`.
pub const ID_REPLACE_ICON: &str = "search-replace-icon";
/// The "Replace" button.
pub const ID_REPLACE_CONFIRM: &str = "search-replace-confirm";
/// The "All" button.
pub const ID_REPLACE_ALL: &str = "search-replace-all";

/// The anchors on this surface whose boxes size to their own text
/// (`native/oracle/ANCHORS.md` v1.5).
///
/// Both labelled buttons in [`SearchReplaceRow`] — see the module docs for
/// the measurement that overturns `button.rs`'s own "no live call site"
/// claim. Neither `SearchPopover`'s nor `SearchReplaceToggle`'s anchors
/// qualify: every one of them is the fixed `size-6`-derived box the module
/// docs measure, never a run's own width.
pub const CONTENT_SIZED: [&str; 2] = [ID_REPLACE_CONFIRM, ID_REPLACE_ALL];

/// The anchors on this surface whose box height is their own line box
/// (`native/oracle/ANCHORS.md` v1.6).
///
/// **Empty.** Every button on this surface authors its own height —
/// [`icon_button_extent`] or `search.tsx`'s own `h-8` — exactly the
/// `button::LINE_SIZED` shape, for the same reason.
pub const LINE_SIZED: [&str; 0] = [];

/// `p-1.5` on the root, `gap-1.5` in the top row, and `mt-1.5` on the
/// secondary row's own wrapper: the same `--spacing(1.5)` = 6px in three
/// places, measured live and matching by construction rather than
/// coincidence — the root's own `p-1.5` is Tailwind's stock quarter-rem step.
pub const SPACING_1_5: Pixels = px(6.0);

/// `gap-2` in the bottom (`options`/`nav`) row.
pub const ROW2_GAP: Pixels = px(8.0);

/// `gap-1` inside the options group and the nav group.
pub const GROUP_GAP: Pixels = px(4.0);

/// `w-[320px]` — the popover's own fixed width. Not content-driven, not
/// viewport-driven: `search.tsx` writes the pixel literal.
pub const POPOVER_WIDTH: Pixels = px(320.0);

/// `rounded-xl border border-border/70 bg-background/95` mix percentages —
/// see [`SearchPopover::shell`].
const ROOT_BG_MIX: f32 = 95.0;
const ROOT_BORDER_MIX: f32 = 70.0;

/// `search.tsx`'s own `h-8` on the `Input`'s outer `<span data-slot=
/// "input-control">`, via its `className` prop. **Not `input.rs`'s own
/// `Size::Default` height** (which the *inner* field keeps, unaffected — see
/// the module docs): `input.tsx`'s base span carries no `h-*` class of its
/// own, so `search.tsx`'s override has nothing to conflict with and applies
/// cleanly at every viewport. Measured live: `246×32` at 1714px and
/// (allowing for the narrower `Input` the replace row leaves room for)
/// `140.81×32` in `SearchReplaceRow`.
pub const INPUT_CONTROL_HEIGHT: Pixels = px(32.0);

/// `border-border/80` on `SearchPopover`'s `Input` override — a flat replace
/// of `input.tsx`'s own unprefixed `border-input`, not a `dark:` conditional,
/// because both are the same tailwind-merge group with the same (absent)
/// modifier.
const INPUT_BORDER_MIX: f32 = 80.0;

/// `border-border/70` on `SearchReplaceRow`'s leading icon `<span>`.
const REPLACE_ICON_BORDER_MIX: f32 = 70.0;

/// The icon-shaped button's box: `size-6`'s 24px width, and a height that
/// depends on the breakpoint — see the module docs' size-6/`sm:h-8` account.
///
/// At `Sm` this **is** [`super::button::Size::Default::extent`]`(Sm)` —
/// `sm:h-8`'s own 32px, read off `button`'s table rather than restated so a
/// change there moves this surface with it. At `Base` nothing survives to
/// compete with `size-6`'s own height, so the box is square: `24×24`,
/// confirmed live at a 550px window (`native/mapping/search.md` §2).
#[must_use]
pub fn icon_button_extent(breakpoint: Breakpoint) -> (Pixels, Pixels) {
    const ICON_BUTTON_WIDTH: Pixels = px(24.0);
    match breakpoint {
        Breakpoint::Base => (ICON_BUTTON_WIDTH, ICON_BUTTON_WIDTH),
        Breakpoint::Sm => (
            ICON_BUTTON_WIDTH,
            button::Size::Default.extent(Breakpoint::Sm),
        ),
    }
}

/// One live rendering of the trailing chevron/glyph a plain icon button
/// carries, as an empty box — the convention every icon in this port keeps.
fn glyph_box(breakpoint: Breakpoint) -> Div {
    let extent = search_toggle_icons_glyph_extent(breakpoint);
    div().flex_shrink_0().w(extent).h(extent)
}

/// The glyph's own box inside an icon button — `button`'s
/// `[&_svg:not([class*='size-'])]:size-4.5 sm:…size-4` rule, exactly the
/// measurement [`super::search_toggle_icons::glyph_extent`] already makes
/// for the option toggles' icons. Shared here for the leading toggle's
/// chevron, the close button's `×` and the nav chevrons — every glyph on
/// this surface sits inside the same `Size::Default` host.
fn search_toggle_icons_glyph_extent(breakpoint: Breakpoint) -> Pixels {
    button::Size::Default.icon(breakpoint)
}

/// The shared shell every plain icon button on this surface uses:
/// `flex size-6 items-center justify-center rounded-lg border
/// border-transparent text-muted-foreground`, before either
/// `searchIconButtonVariants`' or `searchToggleButtonVariants`' own state
/// arms are applied.
///
/// `border-transparent` is still a **real 1px border** — `button`'s own
/// trap, confirmed live (`leadingBtn`'s `borderTopWidth: "1px"`,
/// `borderTopColor: "rgba(0,0,0,0)"`).
fn icon_button_shell(theme: &Theme, breakpoint: Breakpoint) -> Div {
    let (width, height) = icon_button_extent(breakpoint);
    div()
        .flex_shrink_0()
        .flex()
        .items_center()
        .justify_center()
        .w(width)
        .h(height)
        .rounded(theme.radius_lg.value())
        .border_1()
        .border_color(Color::TRANSPARENT)
        .font(ui_sans_font(theme))
}

/// `SearchReplaceToggle` — the chevron button `SearchPopover` renders as its
/// `leadingControl`.
///
/// [`Self::expanded`] rotates the glyph (`rotate-90`) but moves no box: a
/// CSS transform is outside `ANCHORS.md`'s schema (§6), exactly as `dialog`'s
/// mount transition is. The field is kept anyway because it is the React
/// prop this component actually takes, and a reader should be able to see
/// that the port knows about it and why it does not change the anchor.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct SearchReplaceToggle {
    /// `isExpanded` — rotates the glyph only. See the struct docs.
    pub expanded: bool,
    /// Which side of `sm:` the viewport is on.
    pub breakpoint: Breakpoint,
}

impl SearchReplaceToggle {
    /// The captured cell: resting, replace collapsed, at the `Sm` breakpoint.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            expanded: false,
            breakpoint: Breakpoint::Sm,
        }
    }

    /// Renders the toggle, opting [`ID_REPLACE_TOGGLE`] into `anchors`.
    #[must_use]
    pub fn render(self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let shell = icon_button_shell(theme, self.breakpoint)
            .text_color(theme.muted_foreground)
            .child(glyph_box(self.breakpoint));
        anchors.boxed(AnchorId::from(ID_REPLACE_TOGGLE), shell)
    }
}

/// Which of the three always-live option toggles a cell is rendering, for
/// [`SearchPopover::render`]'s internal use.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum CoreToggle {
    Case,
    WholeWord,
    Regex,
}

impl CoreToggle {
    const fn id(self) -> &'static str {
        match self {
            Self::Case => ID_TOGGLE_CASE,
            Self::WholeWord => ID_TOGGLE_WHOLE_WORD,
            Self::Regex => ID_TOGGLE_REGEX,
        }
    }

    const fn glyph(self) -> Toggle {
        match self {
            Self::Case => Toggle::CaseSensitive,
            Self::WholeWord => Toggle::WholeWord,
            Self::Regex => Toggle::Regex,
        }
    }
}

/// One `<Button>` behind `searchToggleButtonVariants`/`searchIconButtonVariants`
/// — active swaps `border-transparent`/`text-muted-foreground` for
/// `border-border/70 bg-muted text-foreground`. **Not driven live**: no
/// captured cell has an active toggle (`native/mapping/search.md` §7 records
/// it as real-but-unreached, `search-toggle-icons.md` §6 already measured the
/// one field it moves on the glyph it wraps).
fn toggle_button(
    id: &'static str,
    toggle: Toggle,
    active: bool,
    breakpoint: Breakpoint,
    theme: &Theme,
    anchors: &dyn AnchorSink,
) -> AnyElement {
    let mut shell = icon_button_shell(theme, breakpoint);
    if active {
        shell = shell
            .border_color(theme.border.mix(ROOT_BORDER_MIX, Color::TRANSPARENT))
            .bg(theme.muted)
            .text_color(theme.foreground);
    } else {
        shell = shell.text_color(theme.muted_foreground);
    }
    let icon = SearchToggleIcon {
        toggle,
        breakpoint,
        active,
        empty: false,
    };
    let shell = shell.child(icon.render(theme, anchors));
    anchors.boxed(AnchorId::from(id), shell)
}

/// One `<Button disabled={!canNavigate}>` — previous/next-match chevron.
///
/// `disabled` paints [`DISABLED_OPACITY`] (0.64), **not**
/// `searchIconButtonVariants`' own `opacity-50` — see the module docs.
fn nav_button(
    id: &'static str,
    disabled: bool,
    breakpoint: Breakpoint,
    theme: &Theme,
    anchors: &dyn AnchorSink,
) -> AnyElement {
    let mut shell = icon_button_shell(theme, breakpoint)
        .text_color(theme.muted_foreground)
        .child(glyph_box(breakpoint));
    if disabled {
        shell = shell.opacity(DISABLED_OPACITY);
    }
    anchors.boxed(AnchorId::from(id), shell)
}

/// `SearchReplaceRow` — the expanded replace UI: a swap-icon span, a second
/// `Input` and two labelled action buttons.
///
/// Reachable only while [`SearchPopover::replace`] is `Some`; see the module
/// docs for why its own `Input` cannot share a capture with
/// `SearchPopover`'s.
#[derive(Clone, Debug, PartialEq)]
pub struct SearchReplaceRow {
    /// `canReplace` — disables "Replace".
    pub can_replace: bool,
    /// `canReplaceAll` — disables "All". Defaults to `canReplace` in
    /// `search.tsx`, but is its own field here because the two do
    /// legitimately diverge (`search-results-limited` clears only the
    /// second).
    pub can_replace_all: bool,
    /// Which side of `sm:` the viewport is on.
    pub breakpoint: Breakpoint,
}

impl SearchReplaceRow {
    /// The captured cell: both actions enabled, at the `Sm` breakpoint.
    ///
    /// `can_replace`/`can_replace_all` are **not** what the live capture had
    /// — the reachable cell had an empty query, so both were actually
    /// `false` (`opacity-50`, a real class this shell does not yet model:
    /// `search.tsx`'s `searchActionButtonVariants({disabled})` is the
    /// `opacity-50` shape, not `button.tsx`'s own `disabled:opacity-64`,
    /// because neither action button carries the HTML `disabled` attribute —
    /// `search.tsx` never passes `disabled` to `<Button>` here, only merges
    /// the variant's class). The fixture defaults enabled because that is
    /// the state whose box the reference actually measures (`bg` and
    /// `border` are `transparent` either way, so the two states are
    /// geometrically identical and the capture cannot distinguish them).
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            can_replace: true,
            can_replace_all: true,
            breakpoint: Breakpoint::Sm,
        }
    }

    /// The row's own shell and its four children, built but not yet opted
    /// into `anchors` under [`ID_REPLACE_ROW`] itself — [`Self::render`] and
    /// [`Self::render_root`] each do that last step differently. Shared here
    /// so the two cannot drift: every child anchor (`ID_REPLACE_ICON`, the
    /// input, the two action buttons) is recorded identically either way,
    /// and only the row's own frame-boundary-or-not differs.
    fn shell_and_children(&self, theme: &Theme, anchors: &dyn AnchorSink) -> Div {
        let shell = div()
            .flex()
            .items_center()
            .gap(SPACING_1_5)
            .border_t(px(1.0))
            .border_color(theme.border.mix(60.0, Color::TRANSPARENT))
            .pt(SPACING_1_5)
            .font(ui_sans_font(theme));

        let icon = anchors.boxed(AnchorId::from(ID_REPLACE_ICON), self.icon_shell(theme));

        let input = input_control(theme, anchors, None);

        let confirm = action_button(
            ID_REPLACE_CONFIRM,
            "Replace",
            self.can_replace,
            theme,
            anchors,
        );
        let all = action_button(ID_REPLACE_ALL, "All", self.can_replace_all, theme, anchors);

        shell.child(icon).child(input).child(confirm).child(all)
    }

    /// Renders the row, opting [`ID_REPLACE_ROW`] and its children into
    /// `anchors`.
    ///
    /// **`AnchorSink::boxed`, never `AnchorSink::root`.** `SearchPopover`
    /// always renders this row nested inside its own tree (`search.tsx` has
    /// no call site that mounts `SearchReplaceRow` on its own), and
    /// `AnchorSink::root`'s own doc comment is explicit that reaching it
    /// "starts a new frame's worth of records" — i.e. clears whatever the
    /// enclosing `SearchPopover::render` already recorded.
    /// `button::Button::render`'s module docs name the identical trap
    /// ("`Button::render` cannot be nested — `AnchorSink::root` clears the
    /// registry"), and an early draft of this method hit it exactly: driving
    /// `--surface search --replace` recorded only this row's six anchors and
    /// silently dropped `search-popover` and everything else in it,
    /// caught by `row_layout/search.rs`'s own
    /// `replace_mounts_the_fourth_toggle` test. [`ID_REPLACE_ROW`] still
    /// works as a measurement origin either way — `relative_to` (the
    /// `row_layout` harness's own helper) locates an anchor by id and
    /// subtracts its bounds, regardless of which `AnchorSink` method
    /// produced it.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        anchors.boxed(
            AnchorId::from(ID_REPLACE_ROW),
            self.shell_and_children(theme, anchors),
        )
    }

    /// As [`Self::render`], but **`AnchorSink::root`** — the frame boundary,
    /// for the one caller allowed to reach for it: `--surface
    /// search-replace-row` (P3.37), a *second, standalone* surface over this
    /// same struct, registered because `--surface search --replace`'s own
    /// registry cannot stand in for it.
    ///
    /// `search.tsx`'s `input.tsx` hard-codes `data-oracle-id="input-control"`/
    /// `"input"` with no override prop, and the DOM reference this row is
    /// compared against (`/tmp/p3-ref-search-replace-row.json`) was captured
    /// scoped to `search-replace-row` alone *because* a document with both
    /// `Input`s mounted carries that id twice — `native/oracle/ANCHORS.md`
    /// v1.8 refuses a snapshot on exactly that shape. The driver's own
    /// registry does not refuse a duplicate id, it **replaces** the earlier
    /// record (`AnchorRegistry::record`'s own doc comment) — so `--surface
    /// search --replace` does not crash, but its recorded `"input-control"`
    /// silently becomes whichever of the two `Input`s prepainted last (this
    /// row's own, narrower one), and `Snapshot::build` copies **every**
    /// recorded anchor into any snapshot cut from that registry regardless of
    /// which id its `root` names (it re-origins, it does not filter by
    /// subtree) — so a snapshot asked for the same run with `root:
    /// "search-replace-row"` would still carry `search-popover`,
    /// `search-close` and the rest, not the six-anchor shape the reference
    /// has. Only a render pass that paints **nothing else** produces a
    /// registry [`Snapshot::build`] can turn into that shape, which is what
    /// `crate::surfaces::search_replace_row` exists to drive.
    #[must_use]
    pub fn render_root(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        anchors.root(
            AnchorId::from(ID_REPLACE_ROW),
            self.shell_and_children(theme, anchors),
        )
    }

    /// `<span className="flex h-8 w-8 shrink-0 items-center justify-center
    /// rounded-lg border border-border/70 bg-background text-muted-foreground">`.
    ///
    /// Measured live: `32×32`, `bg #1f1f1eff` (plain `theme.background`, no
    /// opacity mix — unlike the popover root's own `/95`), `border #ffffff0b`
    /// (`border-border/70`).
    fn icon_shell(&self, theme: &Theme) -> Div {
        const SIZE: Pixels = px(32.0);
        div()
            .flex_shrink_0()
            .flex()
            .items_center()
            .justify_center()
            .w(SIZE)
            .h(SIZE)
            .rounded(theme.radius_lg.value())
            .border_1()
            .border_color(
                theme
                    .border
                    .mix(REPLACE_ICON_BORDER_MIX, Color::TRANSPARENT),
            )
            .bg(theme.background)
            .text_color(theme.muted_foreground)
            .child(glyph_box(self.breakpoint))
    }
}

/// One of `search.tsx`'s two labelled action buttons —
/// `searchActionButtonVariants`: `ui-font ui-text-sm flex h-8 items-center
/// justify-center rounded-lg border border-transparent px-2.5
/// text-muted-foreground`. **Content-sized** — see the module docs' finding
/// against `button.rs`'s own claim.
fn action_button(
    id: &'static str,
    label: &'static str,
    enabled: bool,
    theme: &Theme,
    anchors: &dyn AnchorSink,
) -> AnyElement {
    let mut shell = div()
        .flex_shrink_0()
        .flex()
        .items_center()
        .justify_center()
        .h(REPLACE_ACTION_HEIGHT)
        .px(REPLACE_ACTION_PADDING_X)
        .rounded(theme.radius_lg.value())
        .border_1()
        .border_color(Color::TRANSPARENT)
        .font(ui_sans_font(theme))
        .text_size(theme.ui_text_base.value())
        .font_weight(FontWeight::MEDIUM)
        .text_color(theme.muted_foreground);
    if !enabled {
        shell = shell.opacity(DISABLED_OPACITY);
    }

    anchors.boxed_text(
        AnchorId::from(id).content_sized(),
        shell,
        SharedString::new_static(label),
    )
}

/// `searchActionButtonVariants`' `h-8` — the "Replace"/"All" buttons' own
/// authored height (unlike their *width*, which is content-sized).
pub const REPLACE_ACTION_HEIGHT: Pixels = px(32.0);

/// `searchActionButtonVariants`' `px-2.5` — `--spacing(2.5)` = 10px each
/// side. Together with [`REPLACE_ACTION_BORDER_WIDTH`], this is the padding a
/// content-sized box's width is *text advance plus*: measured live,
/// `search-replace-confirm`'s `75.81` against `text_width 53.8` is a
/// `22.01`px difference, and `2×10 + 2×1 = 22` accounts for it to within
/// gpui's own text-shaping rounding.
pub const REPLACE_ACTION_PADDING_X: Pixels = px(10.0);

/// The action buttons' own `border` — real, like every other button on this
/// surface, and like `button`'s own base class list.
pub const REPLACE_ACTION_BORDER_WIDTH: Pixels = px(1.0);

/// `SearchPopover` — the shell `find-bar.tsx` and `terminal-search.tsx` both
/// render.
///
/// The struct models the **reachable** configuration: a leading
/// [`SearchReplaceToggle`], the search `Input`, the close button, the three
/// always-live option toggles plus (only while [`Self::replace`] is `Some`)
/// `preserve-case`, both nav chevrons, and — again only while `replace` is
/// `Some` — a [`SearchReplaceRow`]. `matchLabel`/`extraActions` are real
/// `search.tsx` props with no live cell (see the module docs) and are not
/// fields here for the same reason `Dialog::description` defaults `None`
/// rather than being reproduced from nothing: an anchor the reference cannot
/// produce is a `FieldPresence` delta that forgives nothing.
#[derive(Clone, Debug, PartialEq)]
pub struct SearchPopover {
    /// Which side of `sm:` the viewport is on — real on every box on this
    /// surface, see [`icon_button_extent`].
    pub breakpoint: Breakpoint,
    /// `searchOptions`' three always-live toggles. A sub-struct rather than
    /// three flat fields so this struct stays under clippy's
    /// `struct_excessive_bools` — `button::Props`/`Interaction` split for the
    /// same reason.
    pub toggles: ToggleState,
    /// `searchOptions.preserveCase` — only painted while [`Self::replace`] is
    /// `Some`, exactly as `find-bar.tsx`'s own conditional spread is.
    pub preserve_case_active: bool,
    /// `canNavigate` — disables both nav chevrons. `false` on the captured
    /// cell (an empty query has no matches to navigate).
    pub can_navigate: bool,
    /// `secondaryRow` — `Some` renders a [`SearchReplaceRow`] beneath the
    /// options/nav row **and** adds the `preserve-case` toggle, matching
    /// `find-bar.tsx`'s single `isReplaceVisible` gating both at once.
    pub replace: Option<SearchReplaceRow>,
}

/// `searchOptions.caseSensitive`/`.wholeWord`/`.useRegex` — the three toggles
/// live in every importer. Split from [`SearchPopover`]'s own bool fields for
/// the reason its doc comment gives.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct ToggleState {
    /// "Match case".
    pub case: bool,
    /// "Match whole word".
    pub whole_word: bool,
    /// "Use regular expression".
    pub regex: bool,
}

impl SearchPopover {
    /// The captured cell: replace collapsed, every toggle resting, nav
    /// disabled (no query), at the `Sm` breakpoint — `/tmp/p3-ref-search.json`.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            breakpoint: Breakpoint::Sm,
            toggles: ToggleState::default(),
            preserve_case_active: false,
            can_navigate: false,
            replace: None,
        }
    }

    /// [`Self::fixture`] with replace expanded — the shape
    /// `/tmp/p3-ref-search-replace-row.json` was captured from (as a
    /// separate, sibling-rooted snapshot; see the module docs).
    #[must_use]
    pub fn fixture_with_replace() -> Self {
        Self {
            replace: Some(SearchReplaceRow::fixture()),
            ..Self::fixture()
        }
    }

    /// Renders the popover, opting [`ID_POPOVER`] and every reachable child
    /// into `anchors`.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let row1 = div()
            .flex()
            .items_center()
            .gap(SPACING_1_5)
            .child(
                SearchReplaceToggle {
                    expanded: self.replace.is_some(),
                    breakpoint: self.breakpoint,
                }
                .render(theme, anchors),
            )
            .child(input_control(theme, anchors, None))
            .child(self.close_button(theme, anchors));

        let row2 = div()
            .mt(SPACING_1_5)
            .flex()
            .items_center()
            .justify_between()
            .gap(ROW2_GAP)
            .child(self.options_group(theme, anchors))
            .child(self.nav_group(theme, anchors));

        let mut root = Self::shell(theme).child(row1).child(row2);
        if let Some(replace) = &self.replace {
            root = root.child(div().mt(SPACING_1_5).child(replace.render(theme, anchors)));
        }

        anchors.root(AnchorId::from(ID_POPOVER), root)
    }

    /// `w-[320px] rounded-xl border border-border/70 bg-background/95 p-1.5
    /// shadow-[…] backdrop-blur-sm`.
    ///
    /// `shadow-[0_16px_36px_-28px_rgba(0,0,0,0.55)]` and `backdrop-blur-sm`
    /// are painted for fidelity and invisible to the differ (`ANCHORS.md`
    /// §6, `dropdown`'s and `popover`'s own shadows are the same trade).
    fn shell(theme: &Theme) -> Div {
        div()
            .w(POPOVER_WIDTH)
            .rounded(theme.radius_xl.value())
            .border_1()
            .border_color(theme.border.mix(ROOT_BORDER_MIX, Color::TRANSPARENT))
            .bg(theme.background.mix(ROOT_BG_MIX, Color::TRANSPARENT))
            .p(SPACING_1_5)
            .font(ui_sans_font(theme))
            .shadow_lg()
    }

    fn close_button(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let shell = icon_button_shell(theme, self.breakpoint)
            .text_color(theme.muted_foreground)
            .child(glyph_box(self.breakpoint));
        anchors.boxed(AnchorId::from(ID_CLOSE), shell)
    }

    fn options_group(&self, theme: &Theme, anchors: &dyn AnchorSink) -> Div {
        let mut group = div().flex().items_center().gap(GROUP_GAP);
        for (toggle, active) in [
            (CoreToggle::Case, self.toggles.case),
            (CoreToggle::WholeWord, self.toggles.whole_word),
            (CoreToggle::Regex, self.toggles.regex),
        ] {
            group = group.child(toggle_button(
                toggle.id(),
                toggle.glyph(),
                active,
                self.breakpoint,
                theme,
                anchors,
            ));
        }
        if self.replace.is_some() {
            group = group.child(toggle_button(
                ID_TOGGLE_PRESERVE_CASE,
                Toggle::PreserveCase,
                self.preserve_case_active,
                self.breakpoint,
                theme,
                anchors,
            ));
        }
        group
    }

    fn nav_group(&self, theme: &Theme, anchors: &dyn AnchorSink) -> Div {
        div()
            .flex()
            .items_center()
            .gap(GROUP_GAP)
            .child(nav_button(
                ID_NAV_PREVIOUS,
                !self.can_navigate,
                self.breakpoint,
                theme,
                anchors,
            ))
            .child(nav_button(
                ID_NAV_NEXT,
                !self.can_navigate,
                self.breakpoint,
                theme,
                anchors,
            ))
    }
}

/// The `Input`'s two boxes — `<span data-slot="input-control">` and the
/// `<input data-slot="input">` inside it — built by hand rather than through
/// [`super::input::Input::render`].
///
/// **Why not reuse `Input::render`.** `search.tsx`'s `className` prop
/// (`h-8 rounded-lg border-border/80 bg-background py-1 pr-8 pl-8`) lands on
/// the **outer span only** (`input.tsx`'s own
/// `cn(!unstyled && baseClasses, className)`), while the **inner field**
/// keeps `input.tsx`'s own unmodified `h-8.5 w-full … sm:h-7.5` — a genuine
/// split between control-height (32, this override) and field-height (30,
/// `input`'s own `Size::Default` at `Sm`), confirmed live
/// (`input-control` `246×32`, `input` `180×30`). `input.rs`'s own `Size` enum
/// couples the two together (they are equal on every call site `input.md`'s
/// reference covers), so there is no parameter on `Input` that reproduces
/// this decoupling — the same "own copy" call `dropdown::trigger`/`item` and
/// `alert_dialog` already make, applied to a second already-verified
/// primitive rather than to a sibling wrap.
///
/// Both `input::ID_CONTROL`/`input::ID_FIELD` are reused verbatim (the same
/// ids `input.tsx` already hard-codes onto any `<Input>`, this call site
/// included) rather than renamed, because they *are* that anchor — a reader
/// comparing this surface's reference against `input.md`'s own is comparing
/// the same two ids on purpose.
///
/// No text is painted in either box: `search.tsx`'s captured cell has an
/// empty query, and even a populated one would not reach `AnchorSink` —
/// `input.rs`'s own module docs establish that an `<input>` has no text
/// node, so the reference's record for either id carries no text group
/// regardless of value.
fn input_control(theme: &Theme, anchors: &dyn AnchorSink, width: Option<Pixels>) -> AnyElement {
    let field = anchors.boxed(
        AnchorId::from(input::ID_FIELD),
        div().flex_1().h(px(30.0)).rounded(theme.radius_lg.value()),
    );

    let mut control = div()
        .relative()
        .flex_1()
        .min_w(px(0.0))
        .h(INPUT_CONTROL_HEIGHT)
        .flex()
        .items_center()
        .rounded(theme.radius_lg.value())
        .border_1()
        .border_color(theme.border.mix(INPUT_BORDER_MIX, Color::TRANSPARENT))
        .bg(input_background(theme))
        .px(px(11.0))
        .child(field);
    if let Some(width) = width {
        control = control.flex_none().w(width);
    }

    anchors.boxed(AnchorId::from(input::ID_CONTROL), control)
}

/// `bg-background` (light) / `dark:bg-input/32` (dark) — `input.tsx`'s own
/// unmodified background, since `search.tsx`'s `className` repeats
/// `bg-background` rather than overriding it (a no-op merge: same class,
/// same group). `button.rs`'s `Variant::Outline`/`DestructiveOutline`
/// background already reaches for this exact class through the same
/// `theme.input.mix(32.0, …)` expression — the second surface in the tree
/// this Tailwind rule appears on.
fn input_background(theme: &Theme) -> Color {
    if *theme == Theme::DARK {
        theme.input.mix(32.0, Color::TRANSPARENT)
    } else {
        theme.background
    }
}

#[cfg(test)]
mod tests {
    use super::{
        CONTENT_SIZED, GROUP_GAP, ID_CLOSE, ID_NAV_NEXT, ID_NAV_PREVIOUS, ID_POPOVER,
        ID_REPLACE_ALL, ID_REPLACE_CONFIRM, ID_REPLACE_ICON, ID_REPLACE_ROW, ID_REPLACE_TOGGLE,
        ID_TOGGLE_CASE, ID_TOGGLE_PRESERVE_CASE, ID_TOGGLE_REGEX, ID_TOGGLE_WHOLE_WORD,
        INPUT_CONTROL_HEIGHT, LINE_SIZED, POPOVER_WIDTH, SPACING_1_5, SearchPopover,
        SearchReplaceRow, SearchReplaceToggle, icon_button_extent,
    };
    use crate::components::git_status_row::Breakpoint;
    use crate::theme::Theme;
    use gpui::{Styled as _, div, px};

    /// The two declarations name exactly the content-sized buttons and
    /// nothing else — the finding against `button.rs`'s own claim.
    #[test]
    fn only_the_two_labelled_replace_buttons_are_content_sized() {
        assert_eq!(CONTENT_SIZED, [ID_REPLACE_CONFIRM, ID_REPLACE_ALL]);
        assert_eq!(LINE_SIZED, [] as [&str; 0]);
    }

    /// **The size-6/`sm:h-8` trap, both directions.** `Sm` reads `button`'s
    /// own `sm:h-8`; `Base` is a bare `size-6` square, because nothing
    /// competes with it below the breakpoint.
    #[test]
    fn the_icon_button_is_24x32_at_sm_and_24x24_below_it() {
        assert_eq!(icon_button_extent(Breakpoint::Sm), (px(24.0), px(32.0)));
        assert_eq!(icon_button_extent(Breakpoint::Base), (px(24.0), px(24.0)));
    }

    /// The captured cell's own fixed geometry, spelled as constants a reader
    /// can check against `/tmp/p3-ref-search.json` directly.
    #[test]
    fn the_fixed_geometry_matches_the_captured_reference() {
        assert_eq!(POPOVER_WIDTH, px(320.0));
        assert_eq!(SPACING_1_5, px(6.0));
        assert_eq!(INPUT_CONTROL_HEIGHT, px(32.0));
    }

    /// `SearchPopover::fixture` is the collapsed, resting, no-query cell —
    /// exactly what `/tmp/p3-ref-search.json` was captured from.
    #[test]
    fn the_fixture_is_the_collapsed_resting_cell() {
        let fixture = SearchPopover::fixture();
        assert_eq!(fixture.breakpoint, Breakpoint::Sm);
        assert!(!fixture.toggles.case);
        assert!(!fixture.toggles.whole_word);
        assert!(!fixture.toggles.regex);
        assert!(!fixture.preserve_case_active);
        assert!(!fixture.can_navigate);
        assert!(fixture.replace.is_none());
    }

    /// `fixture_with_replace` is `fixture` plus a replace row — the shape
    /// `/tmp/p3-ref-search-replace-row.json` was captured from as its own
    /// sibling-rooted snapshot.
    #[test]
    fn fixture_with_replace_adds_only_the_replace_row() {
        let expanded = SearchPopover::fixture_with_replace();
        let collapsed = SearchPopover::fixture();
        assert!(expanded.replace.is_some());
        assert_eq!(expanded.breakpoint, collapsed.breakpoint);
        assert_eq!(expanded.can_navigate, collapsed.can_navigate);
    }

    /// `SearchReplaceToggle::expanded` moves no field this port paints — a
    /// CSS transform, `ANCHORS.md` §6. Recorded as a property rather than
    /// left implicit: nothing in [`SearchReplaceToggle::render`] reads it.
    #[test]
    fn replace_toggle_expanded_is_a_field_the_render_path_does_not_use() {
        let resting = SearchReplaceToggle::fixture();
        let expanded = SearchReplaceToggle {
            expanded: true,
            ..resting
        };
        assert_ne!(resting, expanded);
        assert_eq!(resting.breakpoint, expanded.breakpoint);
    }

    /// Every anchor id this module declares is distinct — no two boxes on
    /// this surface share an id, unlike `search-toggle-icons`' deliberate
    /// four-renderings-one-id arrangement (this surface's siblings are
    /// genuinely different controls, not interchangeable renderings of one).
    #[test]
    fn every_anchor_id_on_this_surface_is_distinct() {
        let ids = [
            ID_POPOVER,
            ID_REPLACE_TOGGLE,
            ID_CLOSE,
            ID_TOGGLE_CASE,
            ID_TOGGLE_WHOLE_WORD,
            ID_TOGGLE_REGEX,
            ID_TOGGLE_PRESERVE_CASE,
            ID_NAV_PREVIOUS,
            ID_NAV_NEXT,
            ID_REPLACE_ROW,
            ID_REPLACE_ICON,
            ID_REPLACE_CONFIRM,
            ID_REPLACE_ALL,
        ];
        let mut sorted = ids.to_vec();
        sorted.sort_unstable();
        sorted.dedup();
        assert_eq!(sorted.len(), ids.len(), "{ids:?}");
    }

    /// `SearchReplaceRow::fixture`'s two independent `can_replace*` fields.
    #[test]
    fn replace_row_actions_move_independently() {
        let both_enabled = SearchReplaceRow::fixture();
        assert!(both_enabled.can_replace);
        assert!(both_enabled.can_replace_all);

        let one_disabled = SearchReplaceRow {
            can_replace_all: false,
            ..both_enabled
        };
        assert!(one_disabled.can_replace);
        assert!(!one_disabled.can_replace_all);
    }

    /// `GROUP_GAP` is `gap-1` (4px), distinct from [`SPACING_1_5`]'s 6px —
    /// two different Tailwind spacing steps on the same surface, not one
    /// constant read twice.
    #[test]
    fn the_two_row_gaps_are_different_spacing_steps() {
        assert_eq!(GROUP_GAP, px(4.0));
        assert_ne!(GROUP_GAP, SPACING_1_5);
    }

    /// The shell takes this surface's own tokens (`radius_xl`, a `/95` mix on
    /// `background`), not another surface's — `dropdown`'s and `popover`'s
    /// module docs both warn against exactly this `ANCHORS.md` v1.6 mistake.
    #[test]
    fn the_shell_takes_radius_xl_and_a_ninety_five_percent_background_mix() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            assert_eq!(theme.radius_xl.value(), px(14.0));
            let mixed = theme.background.mix(95.0, crate::theme::Color::TRANSPARENT);
            assert!(mixed.value().a < 1.0);
            assert_ne!(theme.radius_xl, theme.radius_lg);
        }
    }

    /// A render call does not panic on either fixture, in either theme — the
    /// smoke test every render-path-bearing module in this tree keeps.
    #[test]
    fn both_fixtures_render_in_both_themes() {
        use crate::components::anchor::Unanchored;

        for theme in [Theme::LIGHT, Theme::DARK] {
            let _ = SearchPopover::fixture().render(&theme, &Unanchored);
            let _ = SearchPopover::fixture_with_replace().render(&theme, &Unanchored);
        }
        // Exercises the `div()` builder path without a window; a real layout
        // is `row_layout/search.rs`'s job.
        let _ = div().w(px(1.0));
    }
}
