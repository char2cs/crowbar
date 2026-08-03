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
    Styled as _, div, px,
};

use super::anchor::{AnchorId, AnchorSink};
use super::repo_avatar::RepoAvatar;
use super::workspace_branch_icon::{self, WorkspaceBranchIcon};
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

/// `text-xs` on the small (repo/project) line. Tailwind pairs `text-xs`
/// (`0.75rem` = 12px) with `line-height: calc(1 / 0.75)`, a **CSS** value
/// independent of the font's own metrics — 16px regardless of family.
pub const SMALL_LINE_HEIGHT: Pixels = px(16.0);
/// `text-xs`'s own font size.
pub const SMALL_TEXT: Pixels = px(12.0);

/// `text-[13px]` on the large (branch/home/project) line — an arbitrary
/// literal with no paired `line-height` utility, so its box is `normal`,
/// which resolves through the font's own metrics. `command.tsx`'s own
/// `font-editor` and this trigger's `font-mono` are the *same* custom
/// property read through two names (`command.rs`'s own module docs: both
/// are `var(--editor-font-family)`), so `workspace_switcher::CONTENT_HEIGHT`'s
/// measured 18px for the identical `text-[13px]` combination transfers here
/// unchanged rather than being re-derived.
pub const LARGE_LINE_HEIGHT: Pixels = px(18.0);
/// `text-[13px]`'s own font size.
pub const LARGE_TEXT: Pixels = px(13.0);

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
    /// font-normal`.
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
            .font_family(theme.font_mono.primary().unwrap_or("monospace"))
    }

    /// The two-line `flex min-w-0 flex-1 flex-col items-start gap-0.5
    /// text-left leading-tight` label stack shared by [`Self::Workspace`]
    /// and [`Self::Home`].
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
                    .line_height(SMALL_LINE_HEIGHT)
                    .text_color(theme.foreground.mix(70.0, Color::TRANSPARENT))
                    .child(small),
            )
            .child(
                div()
                    .w_full()
                    .overflow_hidden()
                    .text_size(LARGE_TEXT)
                    .line_height(LARGE_LINE_HEIGHT)
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
            div()
                .flex_shrink_0()
                .w(LIBRARY_SIZE)
                .h(LIBRARY_SIZE)
                .text_color(theme.foreground.mix(70.0, Color::TRANSPARENT))
                .into_any_element()
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
        CONTENT_SIZED, ContextPill, ID_ROOT, ID_TRIGGER, LARGE_LINE_HEIGHT, LARGE_TEXT,
        LIBRARY_SIZE, LINE_SIZED, OUTER_PADDING_BOTTOM, OUTER_PADDING_X, SMALL_LINE_HEIGHT,
        SMALL_TEXT, STACK_GAP, TRIGGER_GAP, TRIGGER_PADDING_X, TRIGGER_PADDING_Y,
    };
    use crate::components::workspace_branch_icon::Status;
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
        // The three literals, not spacing multiples.
        assert_eq!(SMALL_TEXT, px(12.0));
        assert_eq!(SMALL_LINE_HEIGHT, px(16.0));
        assert_eq!(LARGE_TEXT, px(13.0));
        assert_eq!(LARGE_LINE_HEIGHT, px(18.0));
        assert_eq!(LIBRARY_SIZE, px(14.0));
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
