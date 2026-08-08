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
    /// A plain branch.
    GitBranch,
    /// A branch with a parent.
    GitFork,
    /// A merged branch.
    GitMerge,
    /// A branch with an open PR.
    GitPullRequest,
    /// A protected branch.
    Lock,
    /// A branch in a conflicted or errored state.
    Warning,
    /// Workspaces tab.
    SquaresFour,
    /// Chats tab.
    ChatsCircle,
    /// Files tab.
    FolderOpen,
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
pub const ALL_ICONS: [IconName; 19] = [
    IconName::ArrowLeft,
    IconName::ArrowRight,
    IconName::Settings,
    IconName::Library,
    IconName::FolderSymlink,
    IconName::LayoutGrid,
    IconName::DownloadCloud,
    IconName::GitBranch,
    IconName::GitFork,
    IconName::GitMerge,
    IconName::GitPullRequest,
    IconName::Lock,
    IconName::Warning,
    IconName::SquaresFour,
    IconName::ChatsCircle,
    IconName::FolderOpen,
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
            Self::GitFork => "icons/git-fork.svg",
            Self::GitMerge => "icons/git-merge.svg",
            Self::GitPullRequest => "icons/git-pull-request.svg",
            Self::Lock => "icons/lock.svg",
            Self::Warning => "icons/warning.svg",
            Self::SquaresFour => "icons/squares-four.svg",
            Self::ChatsCircle => "icons/chats-circle.svg",
            Self::FolderOpen => "icons/folder-open.svg",
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
            Self::GitFork => icon_bytes!("git-fork.svg"),
            Self::GitMerge => icon_bytes!("git-merge.svg"),
            Self::GitPullRequest => icon_bytes!("git-pull-request.svg"),
            Self::Lock => icon_bytes!("lock.svg"),
            Self::Warning => icon_bytes!("warning.svg"),
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
