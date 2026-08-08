//! The glyphs the app draws, and the bytes behind them.
//!
//! # Why this exists
//!
//! Until S1a **nothing in this design system drew an icon.** Every icon
//! surface rendered a correctly-sized, correctly-coloured, entirely empty box
//! — `workspace_branch_icon::glyph_box` was
//! `div().flex_shrink_0().w(SIZE_4).h(SIZE_4)` and a `text_color` for a glyph
//! that was never painted. 23 of them in the sidebar alone.
//!
//! The retired oracle could not see it. Its anchor contract compares
//! `bounds`, `bg`, `radius`, `border` and text; an empty box of the right size
//! and colour matches a reference containing an SVG on **every one of those
//! fields**. There is no field for "what artwork is inside", so a component
//! could hold a PASS verdict and still be a hole in the assembled app.
//!
//! # Where the artwork comes from
//!
//! Vendored under `native/assets/icons/`, extracted once from the two icon
//! sets the React app imports, and **committed**. Not generated at build time:
//! `native/` must not reach into `web/node_modules` (check-invariants rule 7),
//! and a generator that did would break on the day `web/` is deleted — which
//! is the whole point of that rule. `native/assets/icons/PROVENANCE.md`
//! records which upstream icon each file came from and under which licence.
//!
//! # Why an enum and not a string
//!
//! A name that does not resolve is a hole in the UI that renders as nothing —
//! which is exactly the failure this module exists to end, and a stringly
//! typed lookup would reintroduce it silently. [`IconName`] is exhaustive, its
//! bytes are `include_bytes!`-ed at compile time, and a missing file is a
//! build error rather than a blank square.

use gpui::{
    AnyElement, IntoElement as _, Pixels, SharedString, Styled as _, Transformation, radians, svg,
};

use crate::theme::Color;

/// # Phosphor's `weight` is part of the artwork, not a style
///
/// A Phosphor glyph is a *filled path* at every weight — `regular` draws an
/// outline-looking shape by filling a stroke-shaped path, and `fill` is a
/// different path entirely. So a weight is a different **file**, not a render
/// option, and the two cannot be interconverted here.
///
/// This mattered: every branch mark and the active tab's glyph are
/// `weight="fill"` at their React call sites, and this port shipped the
/// `regular` artwork for all of them. A filled lock against a hollow one is
/// the single most visible difference the pixel differ found, and no anchor
/// contract can see it — `bounds`, `bg`, `radius` and `border` are identical
/// for both.
///
/// Only the weights the app actually renders are vendored. `Lock`, `Warning`,
/// `GitFork`, `GitMerge` and `GitPullRequest` appear **only** as branch marks,
/// so only their `fill` is here; the four tab glyphs are drawn at both, since
/// `sidebar-tab-bar.tsx` picks `isActive ? 'fill' : 'regular'`.
///
/// One glyph.
///
/// The name is the port's, chosen to say what the icon *is here* rather than
/// to mirror an upstream package's naming — `RowAdd` and `RowChevron` are the
/// two the React app draws as inline paths rather than importing at all.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash)]
pub enum IconName {
    /// Back arrow, sidebar header.
    ArrowLeft,
    /// Forward arrow, sidebar header.
    ArrowRight,
    /// Settings cog, sidebar header.
    Settings,
    /// The project mark, project-home row and context pill.
    Library,
    /// Import-a-repo action.
    FolderSymlink,
    /// The project switcher's grid.
    LayoutGrid,
    /// Clone/pull action on a repo row.
    DownloadCloud,
    /// A plain branch — the inactive Git tab.
    GitBranch,
    /// A plain branch, as a branch mark.
    GitBranchFill,
    /// A branch with a parent.
    GitForkFill,
    /// A merged branch.
    GitMergeFill,
    /// A branch with an open PR.
    GitPullRequestFill,
    /// A protected branch.
    LockFill,
    /// A branch in a conflicted or errored state.
    WarningFill,
    /// Workspaces tab, inactive.
    SquaresFour,
    /// Workspaces tab, active.
    SquaresFourFill,
    /// Chats tab, inactive.
    ChatsCircle,
    /// Chats tab, active.
    ChatsCircleFill,
    /// Files tab, inactive.
    FolderOpen,
    /// Files tab, active.
    FolderOpenFill,
    /// The `+` on a row's trailing actions.
    RowAdd,
    /// The disclosure chevron on a row.
    RowChevron,
    /// The sidebar show/hide panel mark. Drawn inline by the React app rather
    /// than imported — `web/src/components/ui/sidebar-toggle-icon.tsx`.
    SidebarToggle,
}

/// Every glyph, so a gallery or a test can walk them without a hand-kept list
/// that drifts from the enum.
pub const ALL_ICONS: [IconName; 23] = [
    IconName::ArrowLeft,
    IconName::ArrowRight,
    IconName::Settings,
    IconName::Library,
    IconName::FolderSymlink,
    IconName::LayoutGrid,
    IconName::DownloadCloud,
    IconName::GitBranch,
    IconName::GitBranchFill,
    IconName::GitForkFill,
    IconName::GitMergeFill,
    IconName::GitPullRequestFill,
    IconName::LockFill,
    IconName::WarningFill,
    IconName::SquaresFour,
    IconName::SquaresFourFill,
    IconName::ChatsCircle,
    IconName::ChatsCircleFill,
    IconName::FolderOpen,
    IconName::FolderOpenFill,
    IconName::RowAdd,
    IconName::RowChevron,
    IconName::SidebarToggle,
];

/// Where `native/assets/icons/` sits relative to this crate.
macro_rules! icon_bytes {
    ($file:literal) => {
        include_bytes!(concat!(
            env!("CARGO_MANIFEST_DIR"),
            "/../../assets/icons/",
            $file
        ))
    };
}

impl IconName {
    /// The asset path this glyph is served under.
    ///
    /// Namespaced so an `AssetSource` can route by prefix rather than by
    /// guessing which of its kinds a bare filename belongs to.
    #[must_use]
    pub const fn path(self) -> &'static str {
        match self {
            Self::ArrowLeft => "icons/arrow-left.svg",
            Self::ArrowRight => "icons/arrow-right.svg",
            Self::Settings => "icons/settings.svg",
            Self::Library => "icons/library.svg",
            Self::FolderSymlink => "icons/folder-symlink.svg",
            Self::LayoutGrid => "icons/layout-grid.svg",
            Self::DownloadCloud => "icons/download-cloud.svg",
            Self::GitBranch => "icons/git-branch.svg",
            Self::GitBranchFill => "icons/git-branch-fill.svg",
            Self::GitForkFill => "icons/git-fork-fill.svg",
            Self::GitMergeFill => "icons/git-merge-fill.svg",
            Self::GitPullRequestFill => "icons/git-pull-request-fill.svg",
            Self::LockFill => "icons/lock-fill.svg",
            Self::WarningFill => "icons/warning-fill.svg",
            Self::SquaresFour => "icons/squares-four.svg",
            Self::SquaresFourFill => "icons/squares-four-fill.svg",
            Self::ChatsCircle => "icons/chats-circle.svg",
            Self::ChatsCircleFill => "icons/chats-circle-fill.svg",
            Self::FolderOpen => "icons/folder-open.svg",
            Self::FolderOpenFill => "icons/folder-open-fill.svg",
            Self::RowAdd => "icons/row-add.svg",
            Self::RowChevron => "icons/row-chevron.svg",
            Self::SidebarToggle => "icons/sidebar-toggle.svg",
        }
    }

    /// This glyph's artwork, embedded at compile time.
    #[must_use]
    pub const fn bytes(self) -> &'static [u8] {
        match self {
            Self::ArrowLeft => icon_bytes!("arrow-left.svg"),
            Self::ArrowRight => icon_bytes!("arrow-right.svg"),
            Self::Settings => icon_bytes!("settings.svg"),
            Self::Library => icon_bytes!("library.svg"),
            Self::FolderSymlink => icon_bytes!("folder-symlink.svg"),
            Self::LayoutGrid => icon_bytes!("layout-grid.svg"),
            Self::DownloadCloud => icon_bytes!("download-cloud.svg"),
            Self::GitBranch => icon_bytes!("git-branch.svg"),
            Self::GitBranchFill => icon_bytes!("git-branch-fill.svg"),
            Self::GitForkFill => icon_bytes!("git-fork-fill.svg"),
            Self::GitMergeFill => icon_bytes!("git-merge-fill.svg"),
            Self::GitPullRequestFill => icon_bytes!("git-pull-request-fill.svg"),
            Self::LockFill => icon_bytes!("lock-fill.svg"),
            Self::WarningFill => icon_bytes!("warning-fill.svg"),
            Self::SquaresFourFill => icon_bytes!("squares-four-fill.svg"),
            Self::ChatsCircleFill => icon_bytes!("chats-circle-fill.svg"),
            Self::FolderOpenFill => icon_bytes!("folder-open-fill.svg"),
            Self::SquaresFour => icon_bytes!("squares-four.svg"),
            Self::ChatsCircle => icon_bytes!("chats-circle.svg"),
            Self::FolderOpen => icon_bytes!("folder-open.svg"),
            Self::RowAdd => icon_bytes!("row-add.svg"),
            Self::RowChevron => icon_bytes!("row-chevron.svg"),
            Self::SidebarToggle => icon_bytes!("sidebar-toggle.svg"),
        }
    }

    /// Render this glyph at `size`, inheriting whatever `text_color` the
    /// enclosing box already carries.
    ///
    /// For the call sites that already set the colour on the box the glyph
    /// sits in — `sidebar_toggle_icon` is one — so the colour is stated once
    /// rather than twice and cannot drift.
    #[must_use]
    pub fn render_uncoloured(self, size: Pixels) -> AnyElement {
        svg()
            .path(SharedString::new_static(self.path()))
            .w(size)
            .h(size)
            .flex_shrink_0()
            .into_any_element()
    }

    /// Render this glyph at `size` in `color`, rotated a quarter turn
    /// clockwise.
    ///
    /// The disclosure chevron's whole expanded state: the reference draws one
    /// right-pointing path and rotates it — `cn('size-3 transition-transform',
    /// !isCollapsed && 'rotate-90')` — rather than shipping a second glyph.
    /// Reproducing that as a rotation keeps one file for one shape, and keeps
    /// the collapsed and expanded marks from drifting apart.
    #[must_use]
    pub fn render_quarter_turned(self, size: Pixels, color: Color) -> AnyElement {
        svg()
            .path(SharedString::new_static(self.path()))
            .w(size)
            .h(size)
            .flex_shrink_0()
            .text_color(color)
            .with_transformation(Transformation::rotate(radians(std::f32::consts::FRAC_PI_2)))
            .into_any_element()
    }

    /// Render this glyph at `size`, painted in `color`.
    ///
    /// Every vendored file draws in `currentColor`, so the colour arrives
    /// through `text_color` exactly as it does in the React app's own
    /// `text-*` classes — one mechanism, not a per-icon fill.
    #[must_use]
    pub fn render(self, size: Pixels, color: Color) -> AnyElement {
        svg()
            .path(SharedString::new_static(self.path()))
            .w(size)
            .h(size)
            .flex_shrink_0()
            .text_color(color)
            .into_any_element()
    }
}

/// The artwork behind `path`, or `None` when it names no glyph this crate has.
///
/// The seam an `AssetSource` in the binary delegates to: the design system
/// owns the artwork, the binary owns the platform wiring, and neither has to
/// know the other's file layout.
#[must_use]
pub fn asset_bytes(path: &str) -> Option<&'static [u8]> {
    ALL_ICONS
        .into_iter()
        .find(|icon| icon.path() == path)
        .map(IconName::bytes)
}

/// Every asset path this crate serves.
#[must_use]
pub fn asset_paths() -> Vec<SharedString> {
    ALL_ICONS
        .into_iter()
        .map(|icon| SharedString::new_static(icon.path()))
        .collect()
}

#[cfg(test)]
mod tests {

    /// Every vendored file is reachable from a variant, and every variant
    /// resolves to a file that exists.
    ///
    /// The five `regular`-weight Phosphor files this port shipped by mistake
    /// sat in `assets/` unreferenced after the fix, which nothing would have
    /// noticed: an orphan asset is invisible to the compiler, and a variant
    /// pointing at a missing file is a runtime blank box, which is the exact
    /// failure `crate::icon` exists to end.
    #[test]
    fn every_vendored_file_is_reachable_and_every_variant_resolves() {
        let dir = std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("../../assets/icons");
        let mut on_disk: Vec<String> = std::fs::read_dir(&dir)
            .expect("the icon directory is readable")
            .filter_map(std::result::Result::ok)
            .map(|entry| entry.file_name().to_string_lossy().into_owned())
            .filter(|name| {
                std::path::Path::new(name)
                    .extension()
                    .is_some_and(|extension| extension.eq_ignore_ascii_case("svg"))
            })
            .collect();
        on_disk.sort();

        let mut referenced: Vec<String> = super::ALL_ICONS
            .iter()
            .map(|icon| {
                icon.path()
                    .strip_prefix("icons/")
                    .unwrap_or(icon.path())
                    .to_owned()
            })
            .collect();
        referenced.sort();
        referenced.dedup();

        assert_eq!(on_disk, referenced);
        for icon in super::ALL_ICONS {
            assert!(!icon.bytes().is_empty(), "{icon:?} has no artwork");
        }
    }
    use super::{ALL_ICONS, IconName, asset_bytes, asset_paths};

    /// A glyph whose file is missing is a hole in the UI that renders as
    /// nothing — the exact failure this module exists to end. `include_bytes!`
    /// makes it a build error, and this makes it an assertion too.
    #[test]
    fn every_glyph_carries_real_artwork() {
        for icon in ALL_ICONS {
            let bytes = icon.bytes();
            assert!(!bytes.is_empty(), "{icon:?} has no artwork");
            let text = std::str::from_utf8(bytes).expect("an svg is utf-8");
            assert!(text.starts_with("<svg"), "{icon:?} is not an svg");
            assert!(
                text.contains("currentColor"),
                "{icon:?} does not paint in currentColor, so it cannot be themed"
            );
        }
    }

    #[test]
    fn all_icons_lists_every_variant_exactly_once() {
        let mut paths: Vec<&str> = ALL_ICONS.into_iter().map(IconName::path).collect();
        paths.sort_unstable();
        let count = paths.len();
        paths.dedup();
        assert_eq!(paths.len(), count, "ALL_ICONS repeats a glyph");
    }

    #[test]
    fn asset_lookup_round_trips() {
        for icon in ALL_ICONS {
            assert_eq!(
                asset_bytes(icon.path()),
                Some(icon.bytes()),
                "{icon:?} does not resolve through its own path"
            );
        }
        assert_eq!(asset_bytes("icons/not-a-glyph.svg"), None);
        assert_eq!(asset_paths().len(), ALL_ICONS.len());
    }
}
