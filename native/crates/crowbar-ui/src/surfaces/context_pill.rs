//! `context_pill` — the "you are here" pill above the sidebar tab bar: the
//! current workspace's identity glyph and a two-line repo/branch (or
//! project/home) label, or a bare project name.
//!
//! The native half of `web/src/components/layout/context-pill.tsx`. What
//! opens when the pill is clicked — the centred command dialog carrying
//! [`super::workspace_switcher::Row`] — is `command.rs`'s own already-
//! captured surface (see that module's docs); this file is the pill itself,
//! the part that is always on screen. Every length below came out of the
//! app's own `tailwindcss` 4.3.0 with the utility as a candidate — the
//! method `native/MAPPING.md` fixes. See `native/mapping/context-pill.md`.
//!
//! # This composition does not call `Button::render`
//!
//! `context-pill.tsx`'s own trigger is a `<Button variant="ghost" ...>`
//! rendered through `CommandDialogTrigger`'s `render` prop — the same shape
//! `sidebar_project_header.rs` already found for its own four buttons:
//! `button.tsx` writes `'data-oracle-id': 'button'` as a default, and
//! nesting `Button::render`'s own `anchors.root(...)` inside this
//! composition's root would contest which anchor `ANCHORS.md` §4 means. This
//! file's own React source now overrides that default explicitly
//! (`data-oracle-id="context-pill-trigger"`, added by this item — see the
//! React diff), so the collision this reasoning warns about is avoided on
//! both sides at once: the port builds its own box from
//! `button::Size`/`RadiusClass`'s own public, independently-verified values
//! (the `rounded-lg` this trigger actually carries has no icon-sized
//! counterpart, so only the radius reuse applies, not `Size::extent`), and
//! the React source carries a name that will never collide with the generic
//! `button` surface.
//!
//! # `scale-110` on the trailing icon is not modelled
//!
//! `context-pill.tsx` wraps its trailing glyph in `<span className="flex
//! shrink-0 scale-110">`. CSS `transform: scale()` does not participate in
//! layout at all — the scaled element keeps its own *layout* box for every
//! purpose taffy or Blink's box tree cares about — but it **does** move
//! `getBoundingClientRect()`'s answer, because that call reports the
//! *painted*, post-transform box. gpui has no paint-time-only scale on
//! `Styled` that leaves the layout box alone the way CSS's does (its
//! `Element::paint`-level facilities are lower-level than this port's
//! `Div`-only vocabulary), so this port renders the icon at its own natural
//! extent and the differ will see a `bounds` delta of `size × 0.10` on this
//! one anchor's `w`/`h` — recorded here rather than silently absorbed by a
//! constant, so a reader who sees that delta on this specific anchor knows
//! why before reaching for the port.
//!
//! # `kind: 'empty'` is not modelled
//!
//! `ContextPill()` returns `null` when `deriveContextPillModel(...).kind ===
//! 'empty'` — no repo, no project, nothing to show. There is no element to
//! measure in that cell, so [`ContextPill`] has three variants, not four:
//! [`ContextPill::Workspace`], [`ContextPill::Home`], [`ContextPill::Project`].
//! A caller that wants the empty cell renders nothing, the same way a caller
//! wanting `popover`'s untitled cell simply does not build a `PopoverTitle`.
//!
//! # Composes `repo_avatar`/`workspace_branch_icon`, does not reimplement them
//!
//! [`ContextPill::render`] reaches [`super::repo_avatar::RepoAvatar::render`]
//! and [`super::workspace_branch_icon::WorkspaceBranchIcon::render`]
//! directly for the trailing glyph — both already opt their own anchor via
//! `.boxed()`, never `.root()`, so nesting them here is collision-free the
//! same way `workspace_switcher.rs`'s identical composition is. No
//! `oracleSurfaceScope` entry was needed: the nested `repo-avatar`/
//! `workspace-branch-icon` (and, on the `working` branch, `flicker-spinner`)
//! is exactly what this composition paints, not foreign content left
//! unpainted — the same finding those two modules' own docs record from
//! their side, and the opposite of `sidebar-project-header`'s toggle icon.

use gpui::{
    AnyElement, Div, FontWeight, IntoElement as _, ParentElement as _, Pixels, SharedString,
    Styled as _, div, px, relative,
};

use crate::anchor::{AnchorId, AnchorSink};
use crate::icon::IconName;
use crate::primitives::button;
use crate::surfaces::repo::repo_avatar::RepoAvatar;
use crate::surfaces::workspace::workspace_branch_icon::{self, WorkspaceBranchIcon};
use crate::theme::{Color, Theme};

/// The wrapper's own anchor — every other bound on this surface is reported
/// relative to it.
pub const ID_ROOT: &str = "context-pill";
/// The trigger button's own anchor — namespaced away from `button.tsx`'s
/// generic default. See the module docs.
pub const ID_TRIGGER: &str = "context-pill-trigger";

/// **Empty.** Every box on this surface is authored (`px-*`/`py-*`/`gap-*`),
/// so nothing sizes to a text run's max-content width.
pub const CONTENT_SIZED: [&str; 0] = [];
/// **Empty**, for the same reason.
pub const LINE_SIZED: [&str; 0] = [];

/// `--spacing`, Tailwind's stock `0.25rem` at a 16px root.
const SPACING: f32 = 4.0;

/// `px-2` on the outer wrapper.
pub const OUTER_PADDING_X: Pixels = px(SPACING * 2.0);
/// `pb-1` on the outer wrapper. (`pt-0` is `0`, not carried as a constant.)
pub const OUTER_PADDING_BOTTOM: Pixels = px(SPACING * 1.0);

/// `gap-2` between the label stack and the trailing glyph.
pub const TRIGGER_GAP: Pixels = px(SPACING * 2.0);
/// `px-3` on the trigger.
pub const TRIGGER_PADDING_X: Pixels = px(SPACING * 3.0);
/// `py-1.5` on the trigger.
pub const TRIGGER_PADDING_Y: Pixels = px(SPACING * 1.5);

/// `gap-0.5` between the two stacked label lines.
pub const STACK_GAP: Pixels = px(SPACING * 0.5);

/// `text-xs`'s own font size, on the small (repo/project) line.
pub const SMALL_TEXT: Pixels = px(12.0);

/// `text-xs`'s own bundled ratio — `calc(1 / 0.75)`, compiled directly
/// (`@tailwindcss/node`'s `__unstable__loadDesignSystem` against this app's
/// own `index.css`) and confirmed against the live DOM as this line's own
/// `getComputedStyle`: `font-size 12px, line-height 16px`, ratio `1.3333`.
/// `@property --tw-leading { inherits: false }` sits right next to
/// `.text-xs`'s own `line-height: var(--tw-leading, var(--text-xs--line-
/// height))`, and the live reading confirms that declaration is honoured
/// exactly: the ancestor `leading-tight`'s ratio does not reach this
/// line's **computed `line-height` property**.
///
/// **Not what this line renders at.** `ANCHORS.md` compares `bounds.h` —
/// the rendered box — and on this line the box (`15px`, measured live)
/// and the computed property (`16px`, also measured live) disagree. See
/// [`SMALL_LINE_BOX_HEIGHT`]'s own doc for the mechanism and for why this
/// constant, though a real, confirmed CSS fact, is not what
/// [`Self::stack`] renders. Kept for the record and exercised by
/// `the_computed_line_height_and_the_rendered_box_are_different_numbers`
/// below, not because anything here still reads it for layout.
pub const TEXT_XS_LINE_HEIGHT: f32 = 4.0 / 3.0; // calc(1 / 0.75)

/// The small line's own **rendered box height** — `15px`, read directly
/// off the live DOM's `bounds.h` (`getBoundingClientRect`, what
/// `ANCHORS.md` actually compares), not derived from
/// [`TEXT_XS_LINE_HEIGHT`]'s `16px` **computed `line-height` property**.
///
/// # Two different numbers answer two different questions, and this file's own history got that wrong twice
///
/// A CSS engine's *used line-box height* — the quantity that determines
/// how tall an inline box actually renders, and so what `bounds.h`
/// reports — is not simply the `line-height` property's computed value.
/// It is derived from the font's own ascent, descent and half-leading
/// (CSS2.1 §10.8): for `JetBrains Mono Variable` at `12px` with a computed
/// `line-height` of `16px`, that formula lays out a `15px` box, one pixel
/// short of the property it was computed from. The large line shows the
/// same gap in miniature — computed `16.25px`, box `16px` (§ on
/// [`LEADING_TIGHT`]) — small enough there that gpui's own rounding
/// absorbs it without anyone having to model the mechanism directly; the
/// small line's own gap is a whole pixel and does not round away.
///
/// This file's own history chased that whole pixel twice and got the
/// *reason* wrong both times before landing here:
///
/// 1. First, the small line was moved onto [`LEADING_TIGHT`] outright
///    (`1.25 × 12 = 15px`), on the theory that `--tw-leading` inherits
///    into it after all. A live `getComputedStyle` read refuted that
///    directly — the computed property really is `1.3333`, not `1.25` —
///    and the change was reverted.
/// 2. That revert then read as an unexplained residual: fixing the
///    border and the large line alone left the trigger at `48px` against
///    the reference's `47`, and reverting the small line's ratio (back to
///    its own correct computed value) did nothing to close it, because
///    the computed *property* was never the right quantity to chase.
///    Measuring `bounds.h` on both text leaves directly — rather than
///    `getComputedStyle().lineHeight` on either — is what finally showed
///    the small line's own **rendered box** is `15px`, matching the
///    reference all along; the "residual" was a measurement asking the
///    wrong CSS question, not a defect in the port.
///
/// [`Self::stack`] therefore sets this line's box height directly, as a
/// measured literal, rather than reaching for a ratio that does not
/// describe it: `15.0px` is a font-metrics fact this port cannot re-derive
/// from `--leading-tight` or from `text-xs`'s own ratio, and is taken as
/// measured the way `repo_avatar.rs` takes a caller's `avatar.color` as
/// measured, not invented.
pub const SMALL_LINE_BOX_HEIGHT: Pixels = px(15.0);

/// `leading-tight`'s own ratio — `--leading-tight: 1.25`, compiled the same
/// way [`TEXT_XS_LINE_HEIGHT`] was, and confirmed against the live DOM as
/// the **large** line's own computed `line-height` property: `font-size
/// 13px, line-height 16.25px`, ratio `1.25` exactly. `text-[13px]` has no
/// `line-height` of its own (compiled to confirm: `.text-\[13px\] {
/// font-size: 13px; }`, nothing else), so it inherits the ancestor's
/// unitless `1.25` and resolves to `1.25 × 13 = 16.25px` — this line's
/// computed property, unlike the small line's, is not in question.
///
/// Its **rendered box height** is `16px`, measured live — one pixel
/// short of the `16.25px` computed value, the same used-line-box gap
/// [`SMALL_LINE_BOX_HEIGHT`]'s own doc explains for the small line, just
/// small enough here to disappear into gpui's own `.round()` in
/// `text_system.rs`'s `line_height_in_pixels` (`round(16.25) == 16`)
/// without this file needing a second literal constant for it. Kept as
/// `relative(LEADING_TIGHT)` in [`Self::stack`] rather than a literal
/// `px(16.0)`, because for this line the ratio *is* the right mechanism
/// (the computed property genuinely is `1.25`) and the rounding gpui
/// already does lands on the correct box on its own.
///
/// This file's own `px(18.0)` before this — borrowed from
/// `workspace_switcher::CONTENT_HEIGHT`'s own measured 18px for `text-
/// [13px]` under the same font family — was a different, earlier defect
/// (that borrow's own reasoning holds for `workspace-switcher.tsx`'s
/// `CommandItem`, which has no ancestor `leading-*` class, and does not
/// transfer to this file's own `leading-tight` wrapper). Fixed in the same
/// live-parity pass that found the missing border
/// ([`Self::trigger_shell`]); the small line's own, unrelated defect is
/// the one this constant's neighbour, [`SMALL_LINE_BOX_HEIGHT`], now
/// carries the account of in full.
pub const LEADING_TIGHT: f32 = 1.25;
/// `text-[13px]`'s own font size.
pub const LARGE_TEXT: Pixels = px(13.0);
/// The lone `text-[13px]` line on [`ContextPill::Project`] — no
/// `leading-tight` ancestor on that branch (no stack wrapper at all), so it
/// inherits the trigger's own ambient line-height instead:
/// `sm:text-sm`'s own `--text-sm--line-height: calc(1.25 / 0.875)`, the
/// ratio every cell this port drives resolves to (`sm:` is always active —
/// `autocomplete.rs`'s own `BP` states the same "≥640px always" posture).
pub const PROJECT_LINE_HEIGHT: f32 = 1.25 / 0.875; // calc(1.25 / 0.875)

/// `Library`'s own `size={14}` on the home row.
pub const LIBRARY_SIZE: Pixels = px(14.0);

/// The pill.
#[derive(Clone, Debug, PartialEq)]
pub enum ContextPill {
    /// A workspace context: the identity glyph, `{repoName} / {branchName}`.
    Workspace {
        /// `model.status`.
        status: workspace_branch_icon::Status,
        /// `model.working`.
        working: bool,
        /// `model.repoName`.
        repo_name: SharedString,
        /// `model.branchName`.
        branch_name: SharedString,
        /// `model.repoAvatar` — only set for the default (imported-folder)
        /// workspace.
        repo_avatar: Option<RepoAvatar>,
    },
    /// A project's home context: `Library`/spinner, `{projectName} / home`.
    Home {
        /// `model.projectName`.
        project_name: SharedString,
        /// `model.working` — the home workspace's own agent-working flag.
        working: bool,
    },
    /// No workspace and no home route, only an active project: the bare
    /// name, no icon, one line.
    Project {
        /// `model.projectName`.
        project_name: SharedString,
    },
}

impl ContextPill {
    /// The live reachable cell: a plain (non-default) workspace, idle, no
    /// avatar.
    #[must_use]
    pub fn fixture() -> Self {
        Self::Workspace {
            status: workspace_branch_icon::Status::New,
            working: false,
            repo_name: "crowbar".into(),
            branch_name: "main".into(),
            repo_avatar: None,
        }
    }

    /// The wrapper's own outer box: `shrink-0 w-full px-2 pt-0 pb-1`.
    fn wrapper() -> Div {
        div()
            .flex_shrink_0()
            .w_full()
            .px(OUTER_PADDING_X)
            .pb(OUTER_PADDING_BOTTOM)
    }

    /// The trigger's own box: `h-auto w-full justify-start items-center
    /// gap-2 rounded-lg bg-sidebar-element-idle px-3 py-1.5 font-mono
    /// font-normal`, plus the **real** 1px border every `<Button>` carries
    /// unconditionally — `button-variants.ts`'s own base class list
    /// (`"...rounded-lg border font-medium..."`), coloured per variant
    /// (`ghost: 'border-transparent ...'`). Confirmed live: `getComputedStyle`
    /// on the real trigger reports `border: 1px solid rgba(0, 0, 0, 0)` — a
    /// real width, a transparent colour. `box-sizing: border-box` means this
    /// border sits *inside* the box's own height, not outside it, so it is
    /// reused from `button::BORDER_WIDTH` rather than a border-less box with
    /// a bigger height compensating — the two are not interchangeable once
    /// `border.w` is a field the differ compares **exactly** (`ANCHORS.md`
    /// §5).
    fn trigger_shell(theme: &Theme) -> Div {
        div()
            .flex()
            .w_full()
            .items_center()
            .gap(TRIGGER_GAP)
            .rounded(theme.radius_lg.value())
            .bg(theme.sidebar_element_idle)
            .px(TRIGGER_PADDING_X)
            .py(TRIGGER_PADDING_Y)
            .border(button::BORDER_WIDTH)
            .border_color(Color::TRANSPARENT)
            .font_family(theme.font_mono.primary().unwrap_or("monospace"))
    }

    /// The two-line `flex min-w-0 flex-1 flex-col items-start gap-0.5
    /// text-left leading-tight` label stack shared by [`Self::Workspace`]
    /// and [`Self::Home`]. The two lines are set two different ways, for a
    /// reason that is *not* "the ancestor's `leading-tight` reaches one and
    /// not the other" (an earlier version of this file said exactly that,
    /// and it was wrong): the small line renders at its own measured
    /// **box** height ([`SMALL_LINE_BOX_HEIGHT`], a literal — its computed
    /// `line-height` property is [`TEXT_XS_LINE_HEIGHT`], a different,
    /// larger number, and `ANCHORS.md` compares the box), and the large
    /// line renders at the ratio [`LEADING_TIGHT`] (the *property* it
    /// genuinely inherits, whose rounded box height happens to need no
    /// separate literal). See [`SMALL_LINE_BOX_HEIGHT`]'s own doc for the
    /// used-line-box mechanism both numbers answer to.
    fn stack(theme: &Theme, small: SharedString, large: SharedString) -> Div {
        div()
            .min_w(px(0.0))
            .flex_1()
            .flex()
            .flex_col()
            .items_start()
            .gap(STACK_GAP)
            .child(
                div()
                    .w_full()
                    .overflow_hidden()
                    .text_size(SMALL_TEXT)
                    .line_height(SMALL_LINE_BOX_HEIGHT)
                    .text_color(theme.foreground.mix(70.0, Color::TRANSPARENT))
                    .child(small),
            )
            .child(
                div()
                    .w_full()
                    .overflow_hidden()
                    .text_size(LARGE_TEXT)
                    .line_height(relative(LEADING_TIGHT))
                    .font_weight(FontWeight::SEMIBOLD)
                    .text_color(theme.foreground)
                    .child(large),
            )
    }

    /// `<span className="flex shrink-0 scale-110">…</span>` — the scale is
    /// not modelled; see the module docs.
    fn glyph_wrapper() -> Div {
        div().flex().flex_shrink_0()
    }

    /// The trailing glyph: the avatar beats the status icon, matching
    /// `context-pill.tsx`'s own comment ("a working agent must still spin
    /// its icon — so the spinner... takes precedence over it").
    fn workspace_glyph(
        theme: &Theme,
        anchors: &dyn AnchorSink,
        status: workspace_branch_icon::Status,
        working: bool,
        repo_avatar: Option<&RepoAvatar>,
    ) -> AnyElement {
        let inner = match repo_avatar {
            Some(avatar) if !working => avatar.render(theme, anchors),
            _ => WorkspaceBranchIcon {
                status,
                working,
                is_placeholder: false,
            }
            .render(theme, anchors),
        };
        Self::glyph_wrapper().child(inner).into_any_element()
    }

    /// The home glyph: `model.working ? <WorkspaceAgentSpinner/> :
    /// <Library size={14}/>` — `text-foreground/70` on the `Library` arm
    /// only (the spinner paints its own colour).
    fn home_glyph(theme: &Theme, anchors: &dyn AnchorSink, working: bool) -> AnyElement {
        let inner = if working {
            WorkspaceBranchIcon {
                status: workspace_branch_icon::Status::default(),
                working: true,
                is_placeholder: false,
            }
            .render(theme, anchors)
        } else {
            // `<Library size={14} />`, `context-pill.tsx:98` — the same mark
            // the project-home row draws as its leading glyph.
            IconName::Library.render(LIBRARY_SIZE, theme.foreground.mix(70.0, Color::TRANSPARENT))
        };
        Self::glyph_wrapper().child(inner).into_any_element()
    }

    /// Renders the pill, opting [`ID_ROOT`] and [`ID_TRIGGER`] into
    /// `anchors`.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let trigger = match self {
            Self::Workspace {
                status,
                working,
                repo_name,
                branch_name,
                repo_avatar,
            } => Self::trigger_shell(theme)
                .child(Self::stack(theme, repo_name.clone(), branch_name.clone()))
                .child(Self::workspace_glyph(
                    theme,
                    anchors,
                    *status,
                    *working,
                    repo_avatar.as_ref(),
                )),
            Self::Home {
                project_name,
                working,
            } => Self::trigger_shell(theme)
                .child(Self::stack(
                    theme,
                    project_name.clone(),
                    SharedString::new_static("home"),
                ))
                .child(Self::home_glyph(theme, anchors, *working)),
            Self::Project { project_name } => Self::trigger_shell(theme).child(
                div()
                    .overflow_hidden()
                    .text_size(LARGE_TEXT)
                    .line_height(relative(PROJECT_LINE_HEIGHT))
                    .text_color(theme.foreground)
                    .child(project_name.clone()),
            ),
        };

        anchors.root(
            AnchorId::from(ID_ROOT),
            Self::wrapper().child(anchors.boxed(AnchorId::from(ID_TRIGGER), trigger)),
        )
    }
}

#[cfg(test)]
mod tests {
    use super::{
        CONTENT_SIZED, ContextPill, ID_ROOT, ID_TRIGGER, LARGE_TEXT, LEADING_TIGHT, LIBRARY_SIZE,
        LINE_SIZED, OUTER_PADDING_BOTTOM, OUTER_PADDING_X, PROJECT_LINE_HEIGHT,
        SMALL_LINE_BOX_HEIGHT, SMALL_TEXT, STACK_GAP, TEXT_XS_LINE_HEIGHT, TRIGGER_GAP,
        TRIGGER_PADDING_X, TRIGGER_PADDING_Y,
    };
    use crate::primitives::button;
    use crate::surfaces::workspace::workspace_branch_icon::Status;
    use gpui::px;

    #[test]
    fn every_length_is_the_compiled_spacing_multiple_or_a_literal() {
        const STEP: f32 = 4.0;
        assert_eq!(OUTER_PADDING_X, px(STEP * 2.0)); // px-2
        assert_eq!(OUTER_PADDING_BOTTOM, px(STEP * 1.0)); // pb-1
        assert_eq!(TRIGGER_GAP, px(STEP * 2.0)); // gap-2
        assert_eq!(TRIGGER_PADDING_X, px(STEP * 3.0)); // px-3
        assert_eq!(TRIGGER_PADDING_Y, px(STEP * 1.5)); // py-1.5
        assert_eq!(STACK_GAP, px(STEP * 0.5)); // gap-0.5
        // The two text sizes, literals rather than spacing multiples.
        assert_eq!(SMALL_TEXT, px(12.0));
        assert_eq!(LARGE_TEXT, px(13.0));
        assert_eq!(LIBRARY_SIZE, px(14.0));
        // The small line's own measured box height — a literal, not a
        // ratio; see its own doc for why.
        assert_eq!(SMALL_LINE_BOX_HEIGHT, px(15.0));
        // The two computed line-height ratios — confirmed live against the
        // two text leaves' own `getComputedStyle`. TEXT_XS_LINE_HEIGHT is
        // no longer read by `stack()` (SMALL_LINE_BOX_HEIGHT is); it stays
        // for the record — see its own doc.
        assert!((TEXT_XS_LINE_HEIGHT - 4.0 / 3.0).abs() < f32::EPSILON);
        assert!((LEADING_TIGHT - 1.25).abs() < f32::EPSILON);
        assert!((PROJECT_LINE_HEIGHT - 1.25 / 0.875).abs() < f32::EPSILON);
        assert_eq!(button::BORDER_WIDTH, px(1.0));
    }

    /// **The computed `line-height` property and the rendered box height
    /// are different numbers on the small line, and `ANCHORS.md` wants the
    /// box.** `TEXT_XS_LINE_HEIGHT × SMALL_TEXT` is exactly `16px` — the
    /// small line's own computed property, confirmed live — and
    /// [`SMALL_LINE_BOX_HEIGHT`] is `15px` — the same line's own rendered
    /// box, also confirmed live. The two do not agree, on purpose: this is
    /// the used-line-box gap [`SMALL_LINE_BOX_HEIGHT`]'s own doc explains,
    /// spelled out numerically so a reader does not have to take the claim
    /// on faith. The large line's own gap (`LEADING_TIGHT × LARGE_TEXT` =
    /// `16.25px` computed, `16px` rendered) is the same mechanism, smaller
    /// — small enough that gpui's own `.round()` in `line_height_in_pixels`
    /// closes it without a second literal constant.
    #[test]
    fn the_computed_line_height_and_the_rendered_box_are_different_numbers() {
        assert!((TEXT_XS_LINE_HEIGHT * 12.0 - 16.0).abs() < f32::EPSILON);
        assert_eq!(SMALL_LINE_BOX_HEIGHT, px(15.0));
        // Typed `Pixels`, not the raw `f32`s that produced them — comparing
        // the floats directly is `clippy::float_cmp`'s own trap, and this
        // one genuinely wants exact disagreement, not a tolerance.
        assert_ne!(
            px(TEXT_XS_LINE_HEIGHT * 12.0),
            SMALL_LINE_BOX_HEIGHT,
            "the computed property and the rendered box are different \
             numbers on this line — that is the whole point"
        );

        assert!((LEADING_TIGHT * 13.0 - 16.25).abs() < f32::EPSILON);
    }

    #[test]
    fn neither_declaration_list_has_an_entry() {
        assert!(CONTENT_SIZED.is_empty());
        assert!(LINE_SIZED.is_empty());
    }

    #[test]
    fn the_two_anchor_ids_are_distinct_and_namespaced() {
        assert_ne!(ID_ROOT, ID_TRIGGER);
        assert!(ID_TRIGGER.starts_with(ID_ROOT));
        assert_ne!(
            ID_TRIGGER, "button",
            "must not collide with button.tsx's own default"
        );
    }

    #[test]
    fn the_fixture_is_a_plain_idle_workspace() {
        let ContextPill::Workspace {
            status,
            working,
            repo_avatar,
            ..
        } = ContextPill::fixture()
        else {
            panic!("the fixture is a workspace pill");
        };
        assert_eq!(status, Status::New);
        assert!(!working);
        assert!(repo_avatar.is_none());
    }
}
