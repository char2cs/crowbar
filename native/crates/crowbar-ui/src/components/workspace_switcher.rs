//! `workspace_switcher` — the picture inside one row of the workspace
//! switcher's command list: a leading identity glyph, a two-part label, an
//! optional change count and an optional "current" check.
//!
//! The native half of `web/src/components/layout/workspace-switcher.tsx`'s
//! own `WorkspaceSwitcherMenu`, restricted to what that file adds on top of
//! `command.tsx`/`autocomplete.tsx` — the chrome around it (the dialog
//! popup, the input row, the panel, the footer) is already
//! `crowbar_ui::components::command`'s own surface, captured live from this
//! exact call site (see that module's docs: *"the live workspace switcher,
//! reached from the Context Pill"*). What `command.rs`/`autocomplete.rs`
//! deliberately leave opaque is the **content** of one `autocomplete-item` —
//! `autocomplete::Item::content_height`'s own doc comment: *"none of that is
//! `autocomplete.tsx`'s"* — and that content is this file's whole job.
//!
//! # This module carries no anchor of its own
//!
//! `workspace-switcher.tsx`'s own outer `<div ref={rootRef}
//! className="contents">` is CSS `display: contents` — it generates no box
//! at all, so there is nothing here for a wrapper anchor to name even in
//! principle (`sidebar_tab_bar.rs`'s own "no anchor of its own" shape, for a
//! sharper reason than that file's: that wrapper *could* have carried a box
//! and chose not to anchor it, this one cannot generate a box regardless).
//! Every `<CommandItem>` this file's own logic fills already carries a real,
//! already-registered anchor — [`autocomplete::ID_ITEM`], `"autocomplete-
//! item"` — inherited from `AutocompleteItem`'s own default (`command.tsx`'s
//! `CommandItem` overrides `data-slot` but not `data-oracle-id`, confirmed
//! reading the source). [`Row::render`] reuses that exact id rather than
//! minting a `workspace-switcher-item` nobody's capture would ever produce:
//! the real DOM element these two structs describe is the same element
//! either way, so a second name for it would be the fabricated-anchor move
//! `ANCHORS.md` refuses.
//!
//! [`Row::render`] does **not** go through [`autocomplete::Item::render`] —
//! that method paints an empty, `content_height`-tall box on purpose (the
//! call site's content is not its business); this file reproduces the same
//! box (`autocomplete::ITEM_PADDING_X`, `command::ITEM_PADDING_Y`,
//! `autocomplete::ITEM_RADIUS`, all reused as public constants rather than
//! re-derived) and fills it with the real picture instead — the same "reuse
//! the measured numbers, not the anchor machinery" move `sidebar_project_
//! header.rs` makes around `button::Size::IconSm`.
//!
//! # `--surface`/`row_layout`: an argued exception, not an omission
//!
//! Registering a `--surface workspace-switcher` would need a `root` —
//! `surface.rs`'s own registry requires one — and there is no anchor to
//! offer it that is not already `command`'s own `command-dialog-popup` (a
//! collision) or a fabricated one the React side never produces (the exact
//! move this module's own docs above refuse). So this surface stays
//! un-registered, the same call `sidebar_tab_bar.rs` makes; its own
//! `row_layout/workspace_switcher.rs` drives [`Row::render`] directly
//! through the shared harness's `lay_out`, the identical instrument every
//! registered surface's own tests use, with no `Cell`/`Surface` pair to
//! select it through.
//!
//! # Composes `repo_avatar`/`workspace_branch_icon`, does not reimplement them
//!
//! `Row::Workspace`'s own `repo_avatar` field and the status glyph both render through
//! [`repo_avatar::RepoAvatar::render`]/[`workspace_branch_icon::
//! WorkspaceBranchIcon::render`] unchanged — both already opt their own
//! anchor into `anchors` via `.boxed()` (never `.root()`), so nesting them
//! here carries no collision risk the way composing `button::Button::render`
//! would. Neither composition needed an `oracleSurfaceScope` entry: the
//! nested `repo-avatar`/`workspace-branch-icon` (and, on the `working`
//! branch, `flicker-spinner` one level deeper) is exactly what this
//! composition paints, not foreign content left unpainted — see those two
//! modules' own docs for the same finding stated from their side.
//!
//! `Check`/`Library` are empty, unanchored boxes — `workspace-switcher.tsx`
//! gives neither a `data-oracle-id`, and there is no native glyph for either
//! regardless (the `git_status_row`/`avatar` "no substitute" rule). `Library`
//! is `size={14}`, compiled directly; `Check` carries no Tailwind size class
//! at all in the source, so its true rendered size has no compiled number to
//! read — modelled at 16px to match the row's other leading glyphs
//! (`repo-avatar`/`workspace-branch-icon` are both `size-4`) rather than
//! guessed at Phosphor's own unstyled default, and flagged here as an
//! assumption rather than a measurement.

use gpui::{
    AnyElement, Div, FontWeight, ParentElement as _, Pixels, SharedString, Styled as _, div, px,
};

use super::anchor::{AnchorId, AnchorSink};
use super::autocomplete;
use super::command;
use super::repo_avatar::RepoAvatar;
use super::workspace_branch_icon::{self, WorkspaceBranchIcon};
use crate::theme::{Color, Theme};

/// `gap-2` between the leading glyph, the label and the trailing content.
pub const GAP: Pixels = px(8.0);

/// The row's own content height — the label's `text-[13px]` line box.
/// [`autocomplete::Item::fixture`]'s own documented number for this exact
/// call site, reused rather than re-measured.
pub const CONTENT_HEIGHT: Pixels = px(18.0);

/// `text-[13px]` on the label — an arbitrary literal, not a token.
pub const LABEL_TEXT: Pixels = px(13.0);

/// `Library`'s own `size={14}` on the home row.
pub const LIBRARY_SIZE: Pixels = px(14.0);

/// `Check`'s own box, as modelled here — see the module docs for why this is
/// an assumption rather than a compiled number.
pub const CHECK_SIZE: Pixels = px(16.0);

/// One `<CommandItem>`'s worth of content — `WorkspaceSwitcherItem`'s own
/// two-way union, spelled the same way.
#[derive(Clone, Debug, PartialEq)]
pub enum Row {
    /// A project's home entry: `Library` glyph, `"{project} / home"`, an
    /// optional check.
    Home {
        /// `item.projectName`.
        project_name: SharedString,
        /// `item.isCurrent`.
        is_current: bool,
    },
    /// A workspace entry: the identity glyph (avatar or status icon),
    /// `"{repo} / {branch}"`, optional change counts, an optional check.
    Workspace {
        /// `item.status` — read only when `working` (below) is `false` and
        /// no avatar is showing.
        status: workspace_branch_icon::Status,
        /// `item.working` — the spinner beats the avatar, the identical
        /// precedence `context-pill.tsx`'s own comment states.
        working: bool,
        /// `item.repoName`.
        repo_name: SharedString,
        /// `item.branch`.
        branch_name: SharedString,
        /// `item.added` — shown only when `status != locked` and this is
        /// `Some(n)` with `n > 0`.
        added: Option<u32>,
        /// `item.deleted`, the same gating.
        deleted: Option<u32>,
        /// `item.isCurrent`.
        is_current: bool,
        /// `item.repoAvatar` — only the repo-home row ever carries one.
        repo_avatar: Option<RepoAvatar>,
    },
}

impl Row {
    /// The live reachable cell: one workspace, highlighted (the one-item
    /// list's own `autoHighlight="always"`), not current, no avatar, no
    /// change counts — [`autocomplete::Item::fixture`]'s own reference,
    /// given real content.
    #[must_use]
    pub fn fixture() -> Self {
        Self::Workspace {
            status: workspace_branch_icon::Status::New,
            working: false,
            repo_name: "oracle-fixture".into(),
            branch_name: "home".into(),
            added: None,
            deleted: None,
            is_current: false,
            repo_avatar: None,
        }
    }

    /// Whether the change counts can show at all — `status !== 'locked'`.
    /// Read only on [`Self::Workspace`]; a [`Self::Home`] row has none.
    #[must_use]
    pub fn counts_eligible(&self) -> bool {
        match self {
            Self::Home { .. } => false,
            Self::Workspace { status, .. } => {
                !matches!(status, workspace_branch_icon::Status::Locked)
            }
        }
    }

    /// Renders the row's content into `autocomplete::ID_ITEM` — see the
    /// module docs for why that id, not a fabricated one.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink, highlighted: bool) -> AnyElement {
        let mut row = Self::shell(theme, highlighted);

        match self {
            Self::Home {
                project_name,
                is_current,
            } => {
                row = row.child(Self::library_icon(theme));
                row = row.child(Self::home_label(theme, project_name));
                if *is_current {
                    row = row.child(Self::check_icon(theme));
                }
            }
            Self::Workspace {
                status,
                working,
                repo_name,
                branch_name,
                added,
                deleted,
                is_current,
                repo_avatar,
            } => {
                row = row.child(Self::glyph(
                    theme,
                    anchors,
                    *status,
                    *working,
                    repo_avatar.as_ref(),
                ));
                row = row.child(Self::workspace_label(theme, repo_name, branch_name));
                if self.counts_eligible() {
                    if let Some(n) = added.filter(|n| *n > 0) {
                        row = row.child(Self::count(Color::GREEN_300, '+', n));
                    }
                    if let Some(n) = deleted.filter(|n| *n > 0) {
                        row = row.child(Self::count(Color::RED_300, '-', n));
                    }
                }
                if *is_current {
                    row = row.child(Self::check_icon(theme));
                }
            }
        }

        anchors.boxed(AnchorId::from(autocomplete::ID_ITEM), row)
    }

    /// The leading glyph: the avatar beats the status icon, the spinner
    /// beats the avatar — `item.repoAvatar && !item.working ? <RepoAvatar/>
    /// : <WorkspaceBranchIcon/>`, spelled out as the two-step precedence it
    /// is.
    fn glyph(
        theme: &Theme,
        anchors: &dyn AnchorSink,
        status: workspace_branch_icon::Status,
        working: bool,
        repo_avatar: Option<&RepoAvatar>,
    ) -> AnyElement {
        match repo_avatar {
            Some(avatar) if !working => avatar.render(theme, anchors),
            _ => WorkspaceBranchIcon {
                status,
                working,
                is_placeholder: false,
            }
            .render(theme, anchors),
        }
    }

    /// `'flex items-center gap-2 font-editor py-1.5'`, `autocomplete.tsx`'s
    /// own `px-2 rounded-sm` — the same box `autocomplete::Item::render`
    /// builds, reproduced rather than reused (see the module docs).
    fn shell(theme: &Theme, highlighted: bool) -> Div {
        let mut shell = div()
            .flex()
            .items_center()
            .gap(GAP)
            .w_full()
            .px(autocomplete::ITEM_PADDING_X)
            .py(command::ITEM_PADDING_Y)
            .rounded(autocomplete::ITEM_RADIUS)
            .font_weight(FontWeight::NORMAL)
            .h(command::ITEM_PADDING_Y * 2.0 + CONTENT_HEIGHT);
        if highlighted {
            shell = shell.bg(theme.accent).text_color(theme.accent_foreground);
        }
        shell
    }

    /// `<Library size={14} className="shrink-0 text-muted-foreground" />` —
    /// unanchored, no native glyph.
    fn library_icon(theme: &Theme) -> Div {
        div()
            .flex_shrink_0()
            .w(LIBRARY_SIZE)
            .h(LIBRARY_SIZE)
            .text_color(theme.muted_foreground)
    }

    /// `<Check aria-label="current" className="shrink-0 text-muted-foreground" />`
    /// — see the module docs for [`CHECK_SIZE`]'s own caveat.
    fn check_icon(theme: &Theme) -> Div {
        div()
            .flex_shrink_0()
            .w(CHECK_SIZE)
            .h(CHECK_SIZE)
            .text_color(theme.muted_foreground)
    }

    /// `<span className="min-w-0 flex-1 truncate text-[13px]">
    /// <span className="text-muted-foreground">{project} / </span>
    /// <span className="text-foreground">home</span></span>` — unanchored
    /// call-site text, painted for fidelity, the `command-item`'s content
    /// itself never carrying an id (see the module docs).
    fn home_label(theme: &Theme, project_name: &SharedString) -> Div {
        div()
            .min_w(px(0.0))
            .flex_1()
            .overflow_hidden()
            .text_size(LABEL_TEXT)
            .child(
                div()
                    .text_color(theme.muted_foreground)
                    .child(format!("{project_name} / ")),
            )
            .child(div().text_color(theme.foreground).child("home"))
    }

    /// The workspace row's own two-line-in-one label:
    /// `{repoName} / {branchName}`, same class shape as [`Self::home_label`].
    fn workspace_label(theme: &Theme, repo_name: &SharedString, branch_name: &SharedString) -> Div {
        div()
            .min_w(px(0.0))
            .flex_1()
            .overflow_hidden()
            .text_size(LABEL_TEXT)
            .child(
                div()
                    .text_color(theme.muted_foreground)
                    .child(format!("{repo_name} / ")),
            )
            .child(
                div()
                    .text_color(theme.foreground)
                    .child(branch_name.clone()),
            )
    }

    /// `<span className="shrink-0 text-green-300|text-red-300">
    /// {sign}{formatChangeCount(n)}</span>` — unanchored, the same call-site
    /// text posture as the label.
    fn count(color: Color, sign: char, n: u32) -> Div {
        div()
            .flex_shrink_0()
            .text_color(color)
            .child(format!("{sign}{}", format_change_count(n)))
    }
}

/// `formatChangeCount` (`format-change-count.ts`), verbatim: values over 999
/// round to a `k` suffix.
#[must_use]
pub fn format_change_count(n: u32) -> String {
    if n > 999 {
        #[expect(
            clippy::cast_possible_truncation,
            clippy::cast_sign_loss,
            reason = "n / 1000.0, rounded, is never negative and never exceeds n itself"
        )]
        let thousands = (f64::from(n) / 1000.0).round() as u32;
        format!("{thousands}k")
    } else {
        n.to_string()
    }
}

#[cfg(test)]
mod tests {
    use super::{
        CHECK_SIZE, CONTENT_HEIGHT, GAP, LABEL_TEXT, LIBRARY_SIZE, Row, format_change_count,
    };
    use crate::components::autocomplete;
    use crate::components::workspace_branch_icon::Status;
    use gpui::px;

    #[test]
    fn every_length_is_the_stated_literal() {
        assert_eq!(GAP, px(8.0)); // gap-2
        assert_eq!(CONTENT_HEIGHT, px(18.0));
        assert_eq!(LABEL_TEXT, px(13.0));
        assert_eq!(LIBRARY_SIZE, px(14.0));
        assert_eq!(CHECK_SIZE, px(16.0));
    }

    #[test]
    fn format_change_count_matches_the_source_exactly() {
        assert_eq!(format_change_count(0), "0");
        assert_eq!(format_change_count(7), "7");
        assert_eq!(format_change_count(999), "999");
        assert_eq!(format_change_count(1500), "2k");
        assert_eq!(format_change_count(2499), "2k");
        assert_eq!(format_change_count(2500), "3k");
    }

    /// The fixture is the live reachable cell: one workspace, not current,
    /// no avatar, no counts.
    #[test]
    fn the_fixture_is_the_live_reachable_cell() {
        let row = Row::fixture();
        let Row::Workspace {
            status,
            working,
            added,
            deleted,
            is_current,
            repo_avatar,
            ..
        } = &row
        else {
            panic!("the fixture is a workspace row");
        };
        assert_eq!(*status, Status::New);
        assert!(!working);
        assert_eq!(*added, None);
        assert_eq!(*deleted, None);
        assert!(!is_current);
        assert!(repo_avatar.is_none());
    }

    /// A locked workspace is never eligible for change counts, whatever
    /// `added`/`deleted` say — the same gate `workspace-switcher.tsx` writes
    /// as `item.status !== 'locked' && (item.added ?? 0) > 0`.
    #[test]
    fn a_locked_row_is_never_counts_eligible() {
        let locked = Row::Workspace {
            status: Status::Locked,
            working: false,
            repo_name: "repo".into(),
            branch_name: "default".into(),
            added: Some(5),
            deleted: None,
            is_current: false,
            repo_avatar: None,
        };
        assert!(!locked.counts_eligible());

        let unlocked = Row::Workspace {
            status: Status::New,
            working: false,
            repo_name: "repo".into(),
            branch_name: "feature".into(),
            added: Some(5),
            deleted: None,
            is_current: false,
            repo_avatar: None,
        };
        assert!(unlocked.counts_eligible());
    }

    /// A home row is never counts-eligible — it has no `added`/`deleted`
    /// field at all in the source union.
    #[test]
    fn a_home_row_is_never_counts_eligible() {
        let home = Row::Home {
            project_name: "acme".into(),
            is_current: false,
        };
        assert!(!home.counts_eligible());
    }

    /// Every row reuses `autocomplete::ID_ITEM` — see the module docs for
    /// why that id and not a fabricated one.
    #[test]
    fn the_reused_anchor_is_autocompletes_own_item_id() {
        assert_eq!(autocomplete::ID_ITEM, "autocomplete-item");
    }
}
