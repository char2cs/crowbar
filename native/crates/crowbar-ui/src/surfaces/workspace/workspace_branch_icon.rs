//! `workspace-branch-icon` — the sidebar row's status glyph, and the second
//! component in the port whose single element **is** its own root (see
//! [`super::repo_avatar`]'s module docs for the shape this is).
//!
//! The native half of `web/src/components/layout/workspace-branch-icon.tsx`:
//! an eight-way branch — `working` first, then `isPlaceholder`, then a
//! seven-way exhaustive `switch` on [`Status`] (the source's own
//! `_exhaustive: never` fallback proves the switch is closed) — that renders
//! exactly one `@phosphor-icons/react` glyph or one
//! [`crate::primitives::flicker_spinner::FlickerSpinner`], never a wrapper
//! around either. Every "Compiles to" below came from running the app's own
//! `web/src/index.css` through its own `tailwindcss` 4.3.0 with the utility as
//! a candidate — the method `native/MAPPING.md` fixes. See
//! `native/mapping/workspace-branch-icon.md`.
//!
//! # `working` and `isPlaceholder` are checked in order, and the order is real
//!
//! `working` is checked first and short-circuits everything else — a
//! placeholder row whose agent is mid-turn still shows the spinner, not the
//! warning glyph. `isPlaceholder` is checked second, ahead of the `locked`
//! case its own comment names: "renders the warning glyph ahead of the
//! locked→Lock case." Both are asserted as precedence, not merely as two
//! independent branches — see
//! `working_beats_everything_and_placeholder_beats_the_status_switch` below.
//!
//! # No native equivalent exists for any of the six glyphs, and that is
//! measured, not assumed
//!
//! `Lock`, `GitBranch`, `Warning`, `GitFork`, `GitPullRequest`, `GitMerge` are
//! `@phosphor-icons/react` components; `vendor/gpui-component-assets`'s
//! bundled icon set (99 SVGs, enumerated) carries none of them — no `lock`,
//! no `git-*` of any kind, and `triangle-alert.svg` is a *different* glyph
//! from Phosphor's `Warning`, not a substitute for it. So every glyph here is
//! an **empty box**, the same call `avatar.rs`'s `.image()` and
//! `git_status_row`'s `.icon()` make: there is no native equivalent, and
//! drawing a substitute would put a shape on screen for a perceptual oracle to
//! converge on. Unlike `git_status_row`'s icon, this one is **not** wrapped in
//! a centring flex span — `workspace-branch-icon.tsx` renders the
//! `<GitBranch>` etc. element directly, with `size-4 shrink-0` on the SVG
//! itself, and Tailwind's own preflight sets `svg { display: block }` — so the
//! box is a **plain block div**, not a flex container, which is
//! `dropdown-menu.md`'s "a plain div child of a div" case rather than
//! `git_status_row`'s.
//!
//! # Seven statuses, five pictures
//!
//! `deleted` and `pr-closed` both render `<GitFork ... text-red-500>` —
//! pixel-identical boxes from two different statuses, the same shape of
//! finding `file_tree_row.md` records for `GitStatus` ("six statuses, five
//! colours"). And the placeholder glyph (`text-amber-500`) is
//! pixel-identical to `pr-conflicts`'s `Warning` — same component, same
//! class list, differing only in `role`/`aria-label`, which carry no field in
//! the contract.
//!
//! # `text-red-500`, `text-green-500`, `text-violet-500` bypass the design
//! system entirely
//!
//! Every other coloured surface in this port reads a `Theme` field derived
//! from a `theme.css` custom property. This component does not: it writes raw
//! Tailwind palette utilities that no `--` token in `theme.css` ever aliases.
//! `text-amber-500` is the one exception — `--warning: var(--color-amber-500)`
//! in *both* tables, so [`Theme::warning`] carries the same number — but
//! `text-red-500` is genuinely different from [`Theme::destructive`] in dark
//! mode (`--destructive: oklch(0.65 0.22 24)` there, not `red-500`), and
//! `text-green-500`/`text-violet-500` have no token to coincide with at all
//! (`success` is `emerald-500`, and nothing aliases violet anywhere). See
//! `crowbar_ui::theme::Color::{RED_500,GREEN_500,VIOLET_500}` for the values,
//! resolved through the same `OKLab` pipeline `gen-theme.py` uses and checked
//! against Tailwind's own published hex.
//!
//! # A reference now exists — P3.54 closed the gap this file was written against
//!
//! At the time this module was first written, `workspace-branch-icon.tsx`
//! carried no `data-oracle-id` anywhere, on any branch — the same finding
//! `repo_avatar.rs`'s module docs recorded for its own file, and its own docs
//! said so rather than working around it. `native/mapping/flicker-
//! spinner.md`'s own reference *does* anchor `flicker-spinner` (that id lives
//! on `flicker-spinner.tsx`, a different file this component wraps), but the
//! wrapping `<span>` `WorkspaceAgentSpinner` builds, and every bare glyph
//! `WorkspaceBranchIcon` returns, carried none. P3.54 has since landed the
//! prerequisite this module's own [`ID`] already anticipated:
//! `workspace-branch-icon.tsx` now carries `data-oracle-id="workspace-branch-
//! icon"` on all seven glyph branches and on `WorkspaceAgentSpinner`'s own
//! wrapping `<span>`, matching [`ID`] exactly. `context-pill.tsx` and
//! `workspace-switcher.tsx` (`crowbar_ui::components::{context_pill,
//! workspace_switcher}`) both compose [`WorkspaceBranchIcon::render`]
//! directly rather than re-deriving its picture, and neither needed an
//! `oracleSurfaceScope` entry for it: the nested `workspace-branch-icon`
//! anchor (and, on the `working` branch, the `flicker-spinner` anchor nested
//! one level deeper still) is exactly what those compositions paint, not
//! foreign content left unpainted the way `sidebar-project-header`'s toggle
//! icon is.
//!
//! # This component takes no `className` and spreads no props — checked, and
//! it is the first surface with no state axis at all
//!
//! Every other "no interaction rule" surface in this port (`crowbar-mark`,
//! `kbd`, `label`, `separator`, …) still keeps one §8.3 flag real, because
//! their React primitives accept a `className`/prop passthrough a call site
//! could — hypothetically — merge down to an edge value, even where none
//! does. `workspace-branch-icon.tsx` has no such seam anywhere: its three
//! props are `status`/`working`/`isPlaceholder`, none of them forwarded, and
//! every returned element's `className` is a fixed string. Checked
//! exhaustively: none of `hover`/`focus`/`selected` has a rule either, so
//! every one of the six live pictures this component can produce paints a
//! fully-sized, fully-coloured box — there is no branch, real or
//! reachable-by-construction, where a §8.3 flag would change anything.
//!
//! This component used to carry a synthetic `empty` field anyway, purely to
//! satisfy the registry invariant that once required at least one
//! non-mandatory flag to be real on every surface. That field is gone:
//! `crowbar-app`'s `--surface workspace-branch-icon` (which is what owns the
//! §8.3 vocabulary — this crate has no `StateFlag` of its own) now declares
//! `SurfaceParams::no_state_axis` instead, "every §8.3 flag considered, none
//! applicable," rather than inventing a struct field with no seam behind it.
//! See `crowbar-app/src/surfaces/workspace_branch_icon.rs` and
//! `crowbar-app/src/surface.rs`'s `no_state_axis` doc comment for the full
//! account of why that is the honest fix and this field was not.

use gpui::{AnyElement, Div, ParentElement as _, Pixels, Styled as _, div, px};

use crate::anchor::{AnchorId, AnchorSink};
use crate::icon::IconName;
use crate::primitives::flicker_spinner::{CallSite as SpinnerCallSite, FlickerSpinner};
use crate::theme::{Color, Theme};

/// The single anchor this surface carries — see the module docs for why
/// every branch shares one id.
pub const ID: &str = "workspace-branch-icon";

/// **Empty.** Every box is `size-4`, and none of the six glyphs (nor the
/// spinner's wrapper) paints a text run.
pub const CONTENT_SIZED: [&str; 0] = [];

/// **Empty**, for the same reason.
pub const LINE_SIZED: [&str; 0] = [];

/// `size-4` — the one box every branch of this component renders, glyph or
/// spinner alike. Unconditional: nothing on this surface collapses it.
pub const SIZE_4: Pixels = px(16.0);

/// `WorkspaceStatus`'s seven members, spelled exactly as the TypeScript union
/// does. The source's own `_exhaustive: never` default arm is what proves
/// this switch is closed — a status this enum cannot name is a status the
/// React switch cannot reach either.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Status {
    /// `'locked'`.
    #[default]
    Locked,
    /// `'new'`.
    New,
    /// `'pr-conflicts'`.
    PrConflicts,
    /// `'deleted'`.
    Deleted,
    /// `'pr-open'`.
    PrOpen,
    /// `'pr-closed'`.
    PrClosed,
    /// `'pr-merged'`.
    PrMerged,
}

/// Every status, in the switch's own order, for `--help` and the
/// closed-vocabulary test.
pub const ALL_STATUSES: [Status; 7] = [
    Status::Locked,
    Status::New,
    Status::PrConflicts,
    Status::Deleted,
    Status::PrOpen,
    Status::PrClosed,
    Status::PrMerged,
];

impl Status {
    /// Its word on the command line and in a caption — the TypeScript
    /// union's own literal spelling.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Locked => "locked",
            Self::New => "new",
            Self::PrConflicts => "pr-conflicts",
            Self::Deleted => "deleted",
            Self::PrOpen => "pr-open",
            Self::PrClosed => "pr-closed",
            Self::PrMerged => "pr-merged",
        }
    }
}

/// Which of the six drawable glyphs a non-`working` cell shows. All six are
/// empty boxes — see the module docs.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Glyph {
    /// `locked`, healthy (not a placeholder).
    Lock,
    /// `new`.
    GitBranch,
    /// `pr-conflicts`, **and** any `isPlaceholder` cell regardless of status
    /// — pixel-identical boxes. See the module docs.
    Warning,
    /// `deleted` **and** `pr-closed` — pixel-identical boxes. See the module
    /// docs.
    GitFork,
    /// `pr-open`.
    GitPullRequest,
    /// `pr-merged`.
    GitMerge,
}

impl Glyph {
    /// Its word on the command line and in a caption.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Lock => "lock",
            Self::GitBranch => "git-branch",
            Self::Warning => "warning",
            Self::GitFork => "git-fork",
            Self::GitPullRequest => "git-pull-request",
            Self::GitMerge => "git-merge",
        }
    }
}

/// One `<WorkspaceBranchIcon>`.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct WorkspaceBranchIcon {
    /// `status` — read only once `working` and `isPlaceholder` are both
    /// false. See [`Self::glyph`].
    pub status: Status,
    /// `working` — checked **first**; overrides everything else.
    pub working: bool,
    /// `isPlaceholder` — checked **second**, ahead of the status switch.
    pub is_placeholder: bool,
}

impl Glyph {
    /// The vendored artwork this glyph draws.
    ///
    /// The same icon the React call site imports, so the two apps paint the
    /// same shape rather than merely the same box — see
    /// `native/assets/icons/PROVENANCE.md`.
    #[must_use]
    pub const fn icon(self) -> IconName {
        match self {
            // `weight="fill"` on every arm — `workspace-branch-icon.tsx`
            // passes it unconditionally, and Phosphor's fill is a different
            // path, not a different style. See `IconName`'s own docs.
            Self::Lock => IconName::LockFill,
            Self::GitBranch => IconName::GitBranchFill,
            Self::Warning => IconName::WarningFill,
            Self::GitFork => IconName::GitForkFill,
            Self::GitPullRequest => IconName::GitPullRequestFill,
            Self::GitMerge => IconName::GitMergeFill,
        }
    }
}

impl WorkspaceBranchIcon {
    /// A representative cell: `new`, idle — the branch glyph, no spinner, no
    /// placeholder warning.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            status: Status::New,
            working: false,
            is_placeholder: false,
        }
    }

    /// Which glyph this cell draws, or `None` for the spinner branch —
    /// `working`, then `isPlaceholder`, then the status switch, in that
    /// order.
    #[must_use]
    pub fn glyph(self) -> Option<Glyph> {
        if self.working {
            return None;
        }
        if self.is_placeholder {
            return Some(Glyph::Warning);
        }
        Some(match self.status {
            Status::Locked => Glyph::Lock,
            Status::New => Glyph::GitBranch,
            Status::PrConflicts => Glyph::Warning,
            Status::Deleted | Status::PrClosed => Glyph::GitFork,
            Status::PrOpen => Glyph::GitPullRequest,
            Status::PrMerged => Glyph::GitMerge,
        })
    }

    /// The glyph's own `text-*` colour — `theme.foreground` for the two
    /// neutral cases, `theme.warning` for the one token this component's
    /// palette happens to coincide with, and a raw Tailwind palette literal
    /// for the three that do not. See the module docs.
    #[must_use]
    pub fn color(self, theme: &Theme) -> Option<Color> {
        self.glyph().map(|glyph| match glyph {
            Glyph::Lock | Glyph::GitBranch => theme.foreground,
            Glyph::Warning => theme.warning,
            Glyph::GitFork => Color::RED_500,
            Glyph::GitPullRequest => Color::GREEN_500,
            Glyph::GitMerge => Color::VIOLET_500,
        })
    }

    /// Renders the icon, opting its one anchor into `anchors`.
    #[must_use]
    pub fn render(self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        if self.working {
            let spinner = FlickerSpinner {
                call_site: SpinnerCallSite::WorkspaceIcon,
                empty: false,
            }
            .render(anchors);
            anchors.boxed(
                AnchorId::from(ID),
                Self::spinner_wrapper(theme).child(spinner),
            )
        } else {
            anchors.boxed(AnchorId::from(ID), self.glyph_box(theme))
        }
    }

    /// `'flex size-4 shrink-0 items-center justify-center text-foreground'`
    /// around `<FlickerSpinner className="size-3.5" />` — `WorkspaceAgentSpinner`.
    ///
    /// An associated function, not a method: the wrapper is `size-4` and
    /// `theme.foreground` on every cell that reaches it — nothing on `self`
    /// (`status`, `working`, `isPlaceholder`) changes it, unlike
    /// [`Self::glyph_box`], which reads `self.color(theme)`. Taking `self`
    /// here would be `clippy::unused_self`'s finding restated as a diff
    /// rather than fixed.
    fn spinner_wrapper(theme: &Theme) -> Div {
        div()
            .flex()
            .flex_shrink_0()
            .items_center()
            .justify_center()
            .w(SIZE_4)
            .h(SIZE_4)
            .text_color(theme.foreground)
    }

    /// One glyph's `size-4 shrink-0` box, as an **empty** div.
    ///
    /// **Not `.flex()`.** The React element is the SVG itself, with no
    /// wrapping span — Tailwind's preflight sets `svg { display: block }`,
    /// so the reference's own computed display is `block`, not `flex`: the
    /// blockification `avatar.rs`/`badge.rs` measure only fires on an
    /// `inline-flex` child, and nothing here is one. `dropdown-menu.md`'s "a
    /// plain div child of a div" case, not `git_status_row`'s icon.
    fn glyph_box(self, theme: &Theme) -> Div {
        let mut node = div().flex_shrink_0().w(SIZE_4).h(SIZE_4);
        // The colour is set on the box *and* handed to the glyph: the box's
        // `text_color` is what the reference's `text-*` class does, and the
        // artwork paints in `currentColor`, so the two agree by construction.
        if let Some(color) = self.color(theme) {
            node = node.text_color(color);
            if let Some(glyph) = self.glyph() {
                node = node.child(glyph.icon().render(SIZE_4, color));
            }
        }
        node
    }
}

#[cfg(test)]
mod tests {
    use super::{
        ALL_STATUSES, CONTENT_SIZED, Glyph, ID, LINE_SIZED, SIZE_4, Status, WorkspaceBranchIcon,
    };
    use crate::theme::{Color, Theme};
    use gpui::px;

    /// Every box on this surface is authored `size-4`, so neither
    /// declaration list has an entry.
    #[test]
    fn neither_declaration_list_has_an_entry() {
        assert!(CONTENT_SIZED.is_empty());
        assert!(LINE_SIZED.is_empty());
        assert_eq!(SIZE_4, px(16.0));
    }

    /// `working` beats `isPlaceholder`, which beats the status switch — the
    /// exact order `workspace-branch-icon.tsx` checks them in.
    #[test]
    fn working_beats_everything_and_placeholder_beats_the_status_switch() {
        let mut icon = WorkspaceBranchIcon {
            status: Status::Locked,
            working: true,
            is_placeholder: true,
        };
        assert_eq!(icon.glyph(), None, "working wins over both");

        icon.working = false;
        assert_eq!(
            icon.glyph(),
            Some(Glyph::Warning),
            "isPlaceholder wins over the locked status",
        );

        icon.is_placeholder = false;
        assert_eq!(
            icon.glyph(),
            Some(Glyph::Lock),
            "with neither flag set, the status switch runs",
        );
    }

    /// The seven-way switch, read off the source verbatim — `deleted` and
    /// `pr-closed` share `GitFork`, which is asserted rather than collapsed
    /// away.
    #[test]
    fn the_status_switch_matches_the_source_exactly() {
        let expect = [
            (Status::Locked, Glyph::Lock),
            (Status::New, Glyph::GitBranch),
            (Status::PrConflicts, Glyph::Warning),
            (Status::Deleted, Glyph::GitFork),
            (Status::PrOpen, Glyph::GitPullRequest),
            (Status::PrClosed, Glyph::GitFork),
            (Status::PrMerged, Glyph::GitMerge),
        ];
        for (status, glyph) in expect {
            let icon = WorkspaceBranchIcon {
                status,
                working: false,
                is_placeholder: false,
            };
            assert_eq!(icon.glyph(), Some(glyph), "{}", status.name());
        }
    }

    /// The placeholder glyph is pixel-identical to `pr-conflicts`'s —
    /// same component, same colour, from two different inputs.
    #[test]
    fn the_placeholder_glyph_is_the_same_picture_as_pr_conflicts() {
        let theme = Theme::DARK;
        let placeholder = WorkspaceBranchIcon {
            status: Status::New,
            working: false,
            is_placeholder: true,
        };
        let conflicts = WorkspaceBranchIcon {
            status: Status::PrConflicts,
            working: false,
            is_placeholder: false,
        };
        assert_eq!(placeholder.glyph(), conflicts.glyph());
        assert_eq!(placeholder.color(&theme), conflicts.color(&theme));
    }

    /// `deleted` and `pr-closed` are the same picture too — `GitFork` at the
    /// same colour, from two different statuses.
    #[test]
    fn deleted_and_pr_closed_are_the_same_picture() {
        let theme = Theme::DARK;
        let deleted = WorkspaceBranchIcon {
            status: Status::Deleted,
            working: false,
            is_placeholder: false,
        };
        let pr_closed = WorkspaceBranchIcon {
            status: Status::PrClosed,
            working: false,
            is_placeholder: false,
        };
        assert_eq!(deleted.glyph(), pr_closed.glyph());
        assert_eq!(deleted.color(&theme), pr_closed.color(&theme));
    }

    /// The colour table: two neutral glyphs read `theme.foreground`, one
    /// reads `theme.warning` (the token this palette happens to coincide
    /// with), and three read raw Tailwind palette literals no token carries.
    #[test]
    fn the_colour_table_matches_the_tokens_and_the_raw_literals() {
        let theme = Theme::DARK;

        let of = |status, placeholder| {
            WorkspaceBranchIcon {
                status,
                working: false,
                is_placeholder: placeholder,
            }
            .color(&theme)
            .expect("a non-working cell always draws a glyph")
        };

        assert_eq!(of(Status::Locked, false), theme.foreground);
        assert_eq!(of(Status::New, false), theme.foreground);
        assert_eq!(of(Status::PrConflicts, false), theme.warning);
        assert_eq!(of(Status::Locked, true), theme.warning, "placeholder");
        assert_eq!(of(Status::Deleted, false), Color::RED_500);
        assert_eq!(of(Status::PrClosed, false), Color::RED_500);
        assert_eq!(of(Status::PrOpen, false), Color::GREEN_500);
        assert_eq!(of(Status::PrMerged, false), Color::VIOLET_500);

        // None of the three raw literals is reachable through a `Theme`
        // field by accident — `success` is emerald, not green, and
        // `destructive` is not `red-500` in the dark table.
        assert_ne!(Color::GREEN_500, theme.success);
        assert_ne!(Color::RED_500, theme.destructive);
    }

    /// `working` reaches no colour of its own — the spinner paints
    /// `theme.foreground` directly on its wrapper, not through this table.
    #[test]
    fn a_working_cell_has_no_glyph_colour() {
        let icon = WorkspaceBranchIcon {
            status: Status::New,
            working: true,
            is_placeholder: false,
        };
        assert_eq!(icon.color(&Theme::DARK), None);
    }

    /// The fixture is the idle branch glyph.
    #[test]
    fn the_fixture_is_the_idle_branch_glyph() {
        let icon = WorkspaceBranchIcon::fixture();
        assert_eq!(icon.status, Status::New);
        assert!(!icon.working);
        assert!(!icon.is_placeholder);
        assert_eq!(icon.glyph(), Some(Glyph::GitBranch));
    }

    /// The vocabulary is closed and its words are the TypeScript union's own
    /// spelling.
    #[test]
    fn the_status_vocabulary_is_the_unions_own_spelling() {
        let names: Vec<&str> = ALL_STATUSES.iter().map(|s| s.name()).collect();
        assert_eq!(
            names,
            [
                "locked",
                "new",
                "pr-conflicts",
                "deleted",
                "pr-open",
                "pr-closed",
                "pr-merged",
            ],
        );
    }

    /// The single shared anchor id.
    #[test]
    fn the_id_is_workspace_branch_icon() {
        assert_eq!(ID, "workspace-branch-icon");
    }

    /// Every field on this struct is a `bool`/`Status` read straight off the
    /// eight-way branch — there is no fourth field left to collapse a box,
    /// and every value this struct can hold draws a fully-sized picture.
    /// `no_state_axis` (declared on this surface's `Params`, in
    /// `crowbar-app`) is where that fact is now recorded, not a field here.
    #[test]
    fn every_value_this_struct_can_hold_draws_a_fully_sized_box() {
        for working in [false, true] {
            for is_placeholder in [false, true] {
                for status in ALL_STATUSES {
                    let icon = WorkspaceBranchIcon {
                        status,
                        working,
                        is_placeholder,
                    };
                    if working {
                        assert_eq!(icon.glyph(), None);
                    } else {
                        assert!(icon.glyph().is_some());
                    }
                }
            }
        }
    }
}
