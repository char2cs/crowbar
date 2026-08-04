//! The Phase 1 gate component: one row of the git status panel.
//!
//! The native half of `web/src/features/git/components/status/git-status-file-item.tsx`
//! rendered through `SidebarTreeRow`. The chrome — wrapper, button, indent,
//! guides — is [`super::sidebar_tree`]; what is here is the content, and the
//! content is the reason this row was chosen as the gate:
//!
//! > **The filename and the directory truncate against each other through three
//! > nested flex containers.** The outcome depends on `min-width: 0` propagating
//! > correctly at every level, which is exactly where a taffy layout is most
//! > likely to disagree with Blink.
//!
//! The three levels, and the `min-width: 0` at each:
//!
//! 1. the button — `min-w-0` (it beats `TreeRow`'s `min-w-max`, same
//!    tailwind-merge group, later wins);
//! 2. the name/directory container — `flex min-w-0 flex-1 items-center gap-1.5
//!    overflow-hidden`, so `min-width: 0` *and* the automatic-minimum-size-of-0
//!    that CSS gives any scroll container;
//! 3. the two spans — both `min-w-0 truncate`, the filename additionally
//!    `shrink-0 basis-auto max-w-[45%]` and the directory `flex-1`.
//!
//! taffy 0.12 implements the automatic minimum size rule
//! (`Overflow::maybe_into_automatic_min_size` → `Some(0.0)` for a scroll
//! container), so `overflow: hidden` zeroes the automatic minimum on the gpui
//! side exactly as it does in Blink. `min_w_0()` is set explicitly as well,
//! because the class list sets it explicitly.

use gpui::{
    AnyElement, Div, FontWeight, IntoElement as _, ParentElement as _, SharedString, Styled as _,
    div, px, relative, rems,
};

use super::sidebar_tree::{self, ICON_SIZE, RowState};
use crate::anchor::{AnchorId, AnchorSink};
use crate::theme::{Color, Theme, ui_sans_font};

/// The root anchor: the `.file-tree-item` wrapper. All other geometry is
/// reported relative to it (`native/oracle/ANCHORS.md` §4).
pub const ID_ITEM: &str = "git-row-item";
/// The `<button>`.
pub const ID_BUTTON: &str = "git-row-button";
/// The file-type icon's 14px box.
pub const ID_ICON: &str = "git-row-icon";
/// The filename span.
pub const ID_NAME: &str = "git-row-name";
/// The directory span.
pub const ID_DIR: &str = "git-row-dir";
/// The `uncommitted` badge.
pub const ID_BADGE: &str = "git-row-badge";
/// The `+n` additions count.
pub const ID_ADDED: &str = "git-row-added";
/// The `-n` deletions count.
pub const ID_DELETED: &str = "git-row-deleted";

/// The anchors on this row whose boxes size to their own text
/// (`native/oracle/ANCHORS.md` v1.5).
///
/// The trailing group, and only the trailing group. `git-row-badge` is a
/// `shrink-0` box around one word, and the two counts are bare runs of `+n` /
/// `-n` — all three take their width from what they say, and `git-row-added`
/// renders `+1` or `+12` depending on the file, which is why pinning a width
/// instead was rejected: it would be wrong at one of them.
///
/// **`git-row-name` is deliberately absent.** It is the flexible sibling that
/// *absorbs* the ceil excess — `max-w-[45%]` when the directory column is on,
/// `flex-1` when it is not — and declaring it content-sized would compare it
/// against a ceiled target it has no reason to land on.
///
/// Written down as data because the same three ids have to be declared on the
/// React side, and a list one side can be diffed against is the only reason the
/// two do not drift.
pub const CONTENT_SIZED: [&str; 3] = [ID_BADGE, ID_ADDED, ID_DELETED];

/// The anchors on this row whose **box height is their own line box**
/// (`native/oracle/ANCHORS.md` v1.6).
///
/// Every bare text span on the row, and only those. Each is a blockified flex
/// item holding one line of text, so its border box is exactly the line box —
/// which `WebKit` floors to a whole logical pixel and gpui snaps to the device
/// grid, landing them up to a pixel apart on the same 18.9px number.
///
/// **`git-row-badge` is deliberately absent, and this is the trap.** It is a
/// single line of text in a box, which reads like the same case, but `sm:h-4`
/// pins its border box at 16px around a 13.33px line box: the height is
/// authored, not derived. Declaring it would compare 16 against 13.33 and
/// manufacture a 2.67px delta on an anchor where both engines already agree —
/// the archived gate run has the badge at exactly 16 on both sides. The
/// evidence is in `oracle/runs/`; do not add it back on the grounds that it
/// paints text.
///
/// Written down as data for the same reason [`CONTENT_SIZED`] is: the same ids
/// have to be declared on the React side, and a list is the only thing the two
/// can be diffed against.
pub const LINE_SIZED: [&str; 4] = [ID_NAME, ID_DIR, ID_ADDED, ID_DELETED];

/// One indent guide, per level: `git-row-guide-0`, `git-row-guide-1`, …
///
/// Zero-based, matching the React `level` index that produces the `left`
/// calculation, so `git-row-guide-0` is the outermost guide on both sides.
#[must_use]
pub fn guide_id(level: u16) -> SharedString {
    SharedString::from(format!("git-row-guide-{level}"))
}

/// `--ui-text-sm`'s line height, and `--text-sm`'s: `leading-[1.35]` is
/// unitless, so it resolves against each element's own font size.
const LINE_HEIGHT: f32 = 1.35;

/// Tailwind's stock `--breakpoint-sm`, in logical px.
///
/// A **viewport** width, not a container width: the generated rule is
/// `@media (width >= 40rem)`, and `40rem` in a media query resolves against the
/// initial font size rather than the document's, so it is 640 CSS px whatever
/// the page does to `html { font-size }`.
pub const BREAKPOINT_SM: f32 = 640.0;

/// Which side of [`BREAKPOINT_SM`] the viewport is on.
///
/// gpui has no media queries, so a `sm:` variant has to be resolved by asking
/// the question CSS would have asked. Making it a type rather than a bare `f32`
/// on the row is what keeps that question from being answered twice, differently
/// — the surface width and the viewport width are separate quantities here, and
/// conflating them is how a reference captured at a 569px viewport came to be
/// compared against a native row rendering the ≥640px variant.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Breakpoint {
    /// Viewport narrower than [`BREAKPOINT_SM`]: the unprefixed utilities.
    Base,
    /// Viewport at or above [`BREAKPOINT_SM`]: the `sm:` utilities win.
    ///
    /// The default, because a real Crowbar window is wider than 640px and a
    /// caller that has not thought about the viewport should get the variant
    /// the app actually renders.
    #[default]
    Sm,
}

impl Breakpoint {
    /// The breakpoint a viewport of this width selects.
    ///
    /// `>=`, exactly as `(width >= 40rem)` says: 640 itself is `sm`.
    #[must_use]
    pub fn of(viewport_width: f32) -> Self {
        if viewport_width >= BREAKPOINT_SM {
            Self::Sm
        } else {
            Self::Base
        }
    }
}

/// The badge's `sm:text-[.625rem]` — an arbitrary Tailwind value with no token
/// behind it. `--ui-text-xs` is 0.6875rem and is a different number.
const BADGE_TEXT_SM: f32 = 0.625;

/// The badge's unprefixed `text-xs` — Tailwind's stock `--text-xs`, 0.75rem.
///
/// See [`Breakpoint`]: this is what a viewport **narrower** than 640px gets.
const BADGE_TEXT_BASE: f32 = 0.75;

/// `sm:h-4` / `sm:min-w-4` on `Badge size="sm"`, in rems — the variant a
/// viewport ≥ 640px gets.
///
/// **Which variant applies is a property of the viewport, not of this
/// component, and it was contested.** Compiling the app's own `index.css`
/// through its own Tailwind gives
///
/// ```text
/// .h-5                { height: calc(var(--spacing) * 5) }
/// .sm\:h-4            { @media (width >= 40rem) { height: calc(var(--spacing) * 4) } }
/// .sm\:text-\[\.625rem\] { @media (width >= 40rem) { font-size: .625rem } }
/// ```
///
/// — a plain **viewport** media query at 640px. Nothing in this app redefines
/// `--breakpoint-sm` (it is Tailwind's stock `40rem`) or `--spacing` (0.25rem),
/// so the pair is 20px/12px below 640 and 16px/10px at or above it.
///
/// Both variants have been measured live off the *same* React row, which is
/// what settled it: one reference capture reports the badge at 74.11×16 with a
/// 10px face, another at 87.33×20 with a 12px face. The badge's `text_width`
/// between the two is 66.11 → 79.33, exactly ×1.2 — which is 12/10 to four
/// decimal places — and the filename span absorbs exactly the 13.22px the badge
/// gained. That is the breakpoint being crossed, not a component difference:
/// **the second capture was taken in a webview narrower than 640px.**
///
/// Neither is "the right one": the row renders whichever the viewport selects,
/// and it is [`GitStatusRow::breakpoint`] that carries the answer, so a
/// comparison is against a reference captured at the *same* viewport. Hard-
/// coding one and comparing it to a capture taken at the other is exactly the
/// mistake that produced four geometry deltas belonging to neither side.
const BADGE_HEIGHT_SM: f32 = 1.0;

/// The badge's unprefixed `h-5` / `min-w-5` — `calc(0.25rem * 5)`, i.e. 20px.
/// See [`BADGE_HEIGHT_SM`].
const BADGE_HEIGHT_BASE: f32 = 1.25;

/// `px-[calc(--spacing(1)-1px)]` — 4px minus the 1px border, in pixels because
/// that is how the arithmetic is written.
const BADGE_PADDING_X: f32 = 3.0;

/// `rounded-[.25rem]`, which beats the base `rounded-sm` (same tailwind-merge
/// group, written later).
const BADGE_RADIUS: f32 = 0.25;

/// `bg-warning/8` in light, `dark:bg-warning/16` in dark.
const BADGE_TINT_LIGHT: f32 = 8.0;
/// See [`BADGE_TINT_LIGHT`].
const BADGE_TINT_DARK: f32 = 16.0;

/// `text-muted-foreground/80` on the directory span.
const DIRECTORY_ALPHA: f32 = 80.0;

/// The badge's only string. Named rather than inlined because the anchor
/// records it and the element paints it, and those two have to be the same
/// `&str` or the snapshot describes something that is not on screen.
pub const BADGE_LABEL: &str = "uncommitted";

/// Whether a `dark:` Tailwind variant is in force.
///
/// The React app switches on a `dark` class on the document element, which is
/// the same switch that selects the token table — so "the dark table is in
/// force" and "the `dark:` variants apply" are the same question, and asking it
/// this way keeps the appearance out of every component's signature.
fn is_dark(theme: &Theme) -> bool {
    *theme == Theme::DARK
}

/// How the filename span is sized, which is decided by the `showDirectory`
/// **prop** and not by whether a directory string exists.
///
/// That distinction is load-bearing: a file at the repository root with
/// `showDirectory` on gets the capped sizing and no directory span at all, so
/// the filename is capped at 45% of a container it is the only occupant of.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum NameSizing {
    /// `shrink-0 basis-auto max-w-[45%]` — the filename keeps its own width up
    /// to 45% of the container and the directory takes what is left.
    Capped,
    /// `flex-1` — nothing to share with, so the filename takes the container.
    Fill,
}

/// `max-w-[45%]`.
pub const NAME_MAX_FRACTION: f32 = 0.45;

/// Which sizing the filename span gets. See [`NameSizing`].
#[must_use]
pub const fn name_sizing(show_directory: bool) -> NameSizing {
    if show_directory {
        NameSizing::Capped
    } else {
        NameSizing::Fill
    }
}

/// Whether the directory span is rendered at all: `showDirectory && directory`.
#[must_use]
pub fn shows_directory_span(show_directory: bool, directory: &str) -> bool {
    show_directory && !directory.is_empty()
}

/// Splits a path the way the React component does — and it is worth being
/// exact, because the JavaScript is not what it looks like.
///
/// `pathParts.pop() || file.path` falls back to the **whole path** when the
/// last segment is empty, which is what a trailing slash produces, while the
/// directory keeps everything before it. So `src/lib/` is the filename
/// `src/lib/` in the directory `src/lib`. Reproduced rather than tidied: a
/// divergence here is a `text` delta the oracle would report as a layout
/// problem.
#[must_use]
pub fn split_path(path: &str) -> (&str, String) {
    let mut parts: Vec<&str> = path.split('/').collect();
    let last = parts.pop().unwrap_or_default();
    let name = if last.is_empty() { path } else { last };
    (name, parts.join("/"))
}

/// The three content lengths of the §8.3 matrix, as the live fixture spells
/// them.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum ContentLength {
    /// A short name at the repository root, so there is no directory to show.
    Short,
    /// A typical name in a typical directory.
    #[default]
    Normal,
    /// A name long enough that it has to truncate at every width in the matrix.
    Overflow,
}

impl ContentLength {
    /// The path this content length stands for.
    #[must_use]
    pub const fn path(self) -> &'static str {
        match self {
            Self::Short => "a.ts",
            Self::Normal => "src/features/terminal/lib/resolve-terminal-connection.ts",
            Self::Overflow => {
                "src/features/terminal/lib/\
                 an-extremely-long-file-name-that-must-truncate-in-the-sidebar-row.ts"
            }
        }
    }
}

/// What the row shows on its trailing edge.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct TrailingContent {
    /// Render the `uncommitted` badge.
    pub uncommitted: bool,
    /// `+n`, rendered only when non-zero.
    pub additions: u32,
    /// `-n`, rendered only when non-zero.
    pub deletions: u32,
    /// `compactGitStatusBadges` — `gap-0.5` instead of `gap-1`.
    pub compact: bool,
}

impl TrailingContent {
    /// Nothing at all: no badge, no counts. The `empty` matrix flag.
    #[must_use]
    pub const fn empty() -> Self {
        Self {
            uncommitted: false,
            additions: 0,
            deletions: 0,
            compact: false,
        }
    }

    /// `hasDiffStats` — the counts group renders only when one of them is
    /// non-zero.
    #[must_use]
    pub const fn has_counts(self) -> bool {
        self.additions > 0 || self.deletions > 0
    }
}

/// One row of the git status panel.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct GitStatusRow {
    /// The repo-relative path the row stands for.
    pub path: SharedString,
    /// `indentLevel`.
    pub depth: u16,
    /// The depth of the row above, which caps the guides at the top of a run.
    pub previous_depth: u16,
    /// The depth of the row below, which caps them at the bottom.
    pub next_depth: u16,
    /// `showDirectory`.
    pub show_directory: bool,
    /// `showFileIcon`.
    pub show_file_icon: bool,
    /// The trailing badge and counts.
    pub trailing: TrailingContent,
    /// The visual state, as a parameter — see [`RowState`].
    pub state: RowState,
    /// Which side of [`BREAKPOINT_SM`] the **viewport** is on.
    ///
    /// A parameter for the same reason [`RowState`] is: gpui resolves no media
    /// queries, so the answer has to arrive from whatever knows the viewport.
    /// It is not the surface width — a 294px sidebar in a 1200px window is
    /// [`Breakpoint::Sm`].
    pub breakpoint: Breakpoint,
}

impl GitStatusRow {
    /// A row for one of the matrix's content lengths, in the live fixture's
    /// shape: icon on, directory column on, `uncommitted` badge, `+12 / -3`.
    #[must_use]
    pub fn fixture(content: ContentLength) -> Self {
        Self {
            path: SharedString::new_static(content.path()),
            depth: 0,
            previous_depth: 0,
            next_depth: 0,
            show_directory: true,
            show_file_icon: true,
            trailing: TrailingContent {
                uncommitted: true,
                additions: 12,
                deletions: 3,
                compact: false,
            },
            state: RowState::resting(),
            breakpoint: Breakpoint::default(),
        }
    }

    /// The filename and the directory, split the way the React component
    /// splits them.
    #[must_use]
    pub fn parts(&self) -> (&str, String) {
        split_path(&self.path)
    }

    /// Renders the row, opting every contract anchor into `anchors`.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let (name, directory) = self.parts();
        let mut content: Vec<AnyElement> = Vec::with_capacity(3);

        if self.show_file_icon {
            content.push(anchors.boxed(ID_ICON.into(), Self::icon(theme)));
        }
        content.push(
            self.name_and_directory(theme, anchors, name, &directory)
                .into_any_element(),
        );
        content.push(self.trailing(theme, anchors).into_any_element());

        let button = anchors.boxed(ID_BUTTON.into(), self.button(theme).children(content));

        let mut wrapper = sidebar_tree::item(theme, self.state).child(self.guides(theme, anchors));
        wrapper = wrapper.child(button);
        anchors.root(ID_ITEM.into(), wrapper)
    }

    /// The `<button>`, with the typography the whole subtree inherits.
    ///
    /// The font family is named explicitly rather than left to inherit: gpui
    /// can only report the *declared* family, and an inherited
    /// `.SystemUIFont` is a string the DOM will never produce.
    /// [`ui_sans_font`] also carries the fallback chain that lets a glyph
    /// missing from it resolve the way `WebKit` resolves it (P3.24).
    fn button(&self, theme: &Theme) -> Div {
        sidebar_tree::row_button(theme, self.depth)
            .font(ui_sans_font(theme))
            .font_weight(FontWeight::NORMAL)
            .text_size(theme.ui_text_base.value())
            .line_height(relative(LINE_HEIGHT))
    }

    /// `.file-tree-guides` — an `absolute inset-0` layer holding one guide per
    /// level. Empty at depth 0, exactly as `TreeGuides` returns `null` there.
    fn guides(&self, theme: &Theme, anchors: &dyn AnchorSink) -> Div {
        let guides = (0..self.depth).map(|level| {
            anchors.boxed(
                guide_id(level).into(),
                sidebar_tree::guide(theme, level, self.previous_depth, self.next_depth),
            )
        });
        div().absolute().inset_0().children(guides)
    }

    /// The 14px icon box.
    ///
    /// **Empty.** The React icon is an SVG chosen by the icon-theme registry
    /// from the file extension; there is no native equivalent and drawing a
    /// substitute would put a shape on screen that the oracle would then
    /// converge on. The box is what the contract measures.
    fn icon(theme: &Theme) -> Div {
        div()
            .flex()
            .items_center()
            .flex_shrink_0()
            .w(ICON_SIZE)
            .h(ICON_SIZE)
            .text_color(theme.muted_foreground)
    }

    /// The middle container and its one or two spans — the part the gate exists
    /// to measure.
    fn name_and_directory(
        &self,
        theme: &Theme,
        anchors: &dyn AnchorSink,
        name: &str,
        directory: &str,
    ) -> Div {
        // Each span is a *block* container holding one text run, which is what
        // the DOM has too — a flex item is blockified and its text is inline
        // content. taffy stretches a block-level in-flow child to the
        // container's inner width, so the run's box is the span's box and one
        // text anchor reports both. Wrapping the span in a box anchor under the
        // same id would record twice and the second record would win.
        let mut container = div()
            .flex()
            .items_center()
            .gap_1p5()
            .flex_1()
            .min_w_0()
            .overflow_hidden()
            .child(self.name_span(theme).child(anchors.text(
                AnchorId::new(ID_NAME).line_sized(),
                SharedString::from(name.to_owned()),
            )));

        if shows_directory_span(self.show_directory, directory) {
            container = container.child(Self::directory_span(theme).child(anchors.text(
                AnchorId::new(ID_DIR).line_sized(),
                SharedString::from(directory.to_owned()),
            )));
        }

        container
    }

    /// `min-w-0 truncate leading-[1.35] text-foreground` plus the sizing the
    /// `showDirectory` prop selects.
    fn name_span(&self, theme: &Theme) -> Div {
        let span = div().min_w_0().truncate().text_color(theme.foreground);
        match name_sizing(self.show_directory) {
            // `shrink-0 basis-auto max-w-[45%]`: grow 0 and shrink 0 are gpui's
            // defaults for grow and are set explicitly for shrink, basis auto is
            // the default, and the cap is a percentage of the container.
            NameSizing::Capped => span.flex_shrink_0().max_w(relative(NAME_MAX_FRACTION)),
            NameSizing::Fill => span.flex_1(),
        }
    }

    /// `ui-text-sm min-w-0 flex-1 truncate leading-[1.35] text-muted-foreground/80`.
    ///
    /// The alpha modifier is `color-mix(… 80%, transparent)`, which is the same
    /// value in sRGB and `OKLab`: mixing against `transparent` contributes nothing
    /// to the premultiplied colour, so only the alpha moves.
    fn directory_span(theme: &Theme) -> Div {
        div()
            .min_w_0()
            .flex_1()
            .truncate()
            .text_size(theme.ui_text_sm.value())
            .text_color(
                theme
                    .muted_foreground
                    .mix(DIRECTORY_ALPHA, Color::TRANSPARENT),
            )
    }

    /// `ml-auto flex shrink-0 items-center gap-1.5` — the badge and the counts.
    fn trailing(&self, theme: &Theme, anchors: &dyn AnchorSink) -> Div {
        let mut group = div()
            .flex()
            .items_center()
            .gap_1p5()
            .flex_shrink_0()
            .ml_auto();

        if self.trailing.uncommitted {
            group = group.child(anchors.boxed_text(
                // Content-sized and **not** line-sized: the width comes from
                // the word, the height from `h-4`/`h-5`. See [`LINE_SIZED`].
                AnchorId::new(ID_BADGE).content_sized(),
                self.badge(theme),
                SharedString::new_static(BADGE_LABEL),
            ));
        }
        if self.trailing.has_counts() {
            group = group.child(self.counts(theme, anchors));
        }
        group
    }

    /// `<Badge variant="warning" size="sm">uncommitted</Badge>`, without its
    /// label: the caller hands the string to [`AnchorSink::boxed_text`] so that
    /// the box and the run it contains reach the snapshot as one anchor.
    ///
    /// The height and the face come from [`GitStatusRow::breakpoint`] — see
    /// [`BADGE_HEIGHT_SM`] for the measurement behind both pairs. Everything
    /// else about the badge is the same at either viewport: the padding, the
    /// radius and the tint are all unprefixed.
    fn badge(&self, theme: &Theme) -> Div {
        let tint = if is_dark(theme) {
            BADGE_TINT_DARK
        } else {
            BADGE_TINT_LIGHT
        };
        let (height, text) = match self.breakpoint {
            Breakpoint::Sm => (BADGE_HEIGHT_SM, BADGE_TEXT_SM),
            Breakpoint::Base => (BADGE_HEIGHT_BASE, BADGE_TEXT_BASE),
        };
        div()
            .flex()
            .items_center()
            .justify_center()
            .flex_shrink_0()
            .h(rems(height))
            .min_w(rems(height))
            .px(px(BADGE_PADDING_X))
            .border_1()
            .border_color(Color::TRANSPARENT)
            .rounded(rems(BADGE_RADIUS))
            .bg(theme.warning.mix(tint, Color::TRANSPARENT))
            .text_size(rems(text))
            .text_color(theme.warning_foreground)
            .font_weight(FontWeight::MEDIUM)
            .whitespace_nowrap()
    }

    /// `+n` and `-n`, each rendered only when it is non-zero.
    fn counts(&self, theme: &Theme, anchors: &dyn AnchorSink) -> Div {
        let mut group = div()
            .flex()
            .items_center()
            .text_size(theme.ui_text_sm.value());
        group = if self.trailing.compact {
            group.gap_0p5()
        } else {
            group.gap_1()
        };

        if self.trailing.additions > 0 {
            let label = SharedString::from(format!("+{}", self.trailing.additions));
            group =
                group.child(div().text_color(theme.git_added).child(
                    anchors.text(AnchorId::new(ID_ADDED).content_sized().line_sized(), label),
                ));
        }
        if self.trailing.deletions > 0 {
            let label = SharedString::from(format!("-{}", self.trailing.deletions));
            group = group.child(div().text_color(theme.git_deleted).child(anchors.text(
                AnchorId::new(ID_DELETED).content_sized().line_sized(),
                label,
            )));
        }
        group
    }
}

#[cfg(test)]
mod tests {
    use super::{
        AnchorId, BADGE_HEIGHT_BASE, BADGE_HEIGHT_SM, BADGE_TEXT_BASE, BADGE_TEXT_SM,
        BREAKPOINT_SM, Breakpoint, CONTENT_SIZED, ContentLength, GitStatusRow, ID_ADDED, ID_BADGE,
        ID_BUTTON, ID_DELETED, ID_DIR, ID_ICON, ID_ITEM, ID_NAME, LINE_SIZED, NameSizing,
        TrailingContent, guide_id, name_sizing, shows_directory_span, split_path,
    };

    #[test]
    fn a_path_splits_into_its_last_segment_and_the_rest() {
        assert_eq!(
            split_path("src/features/terminal/lib/resolve-terminal-connection.ts"),
            (
                "resolve-terminal-connection.ts",
                "src/features/terminal/lib".to_owned()
            ),
        );
    }

    #[test]
    fn a_root_level_path_has_no_directory() {
        assert_eq!(split_path("a.ts"), ("a.ts", String::new()));
    }

    /// `pathParts.pop() || file.path` — the empty last segment is falsy, so the
    /// filename becomes the whole path while the directory keeps the rest.
    #[test]
    fn a_trailing_slash_falls_back_to_the_whole_path() {
        assert_eq!(split_path("src/lib/"), ("src/lib/", "src/lib".to_owned()));
    }

    #[test]
    fn an_empty_path_is_its_own_filename() {
        assert_eq!(split_path(""), ("", String::new()));
    }

    /// The sizing follows the **prop**, not the string. A root-level file with
    /// the directory column enabled still gets the capped filename, which is
    /// the case a "does the directory exist" shortcut would get wrong.
    #[test]
    fn the_name_sizing_follows_the_prop_not_the_string() {
        assert_eq!(name_sizing(true), NameSizing::Capped);
        assert_eq!(name_sizing(false), NameSizing::Fill);

        assert!(!shows_directory_span(true, ""));
        assert!(shows_directory_span(true, "src/lib"));
        assert!(!shows_directory_span(false, "src/lib"));
    }

    #[test]
    fn a_root_level_file_with_the_column_on_is_capped_and_has_no_directory_span() {
        let row = GitStatusRow::fixture(ContentLength::Short);
        let (name, directory) = row.parts();

        assert_eq!(name, "a.ts");
        assert_eq!(directory, "");
        assert_eq!(name_sizing(row.show_directory), NameSizing::Capped);
        assert!(!shows_directory_span(row.show_directory, &directory));
    }

    #[test]
    fn the_two_long_fixtures_share_a_directory() {
        let normal = GitStatusRow::fixture(ContentLength::Normal);
        let overflow = GitStatusRow::fixture(ContentLength::Overflow);

        assert_eq!(normal.parts().1, "src/features/terminal/lib");
        assert_eq!(overflow.parts().1, "src/features/terminal/lib");
        assert_eq!(normal.parts().0, "resolve-terminal-connection.ts");
        assert_eq!(
            overflow.parts().0,
            "an-extremely-long-file-name-that-must-truncate-in-the-sidebar-row.ts",
        );
    }

    /// The overflow fixture has to be longer than the normal one at every
    /// width, or the matrix cell measures nothing.
    #[test]
    fn the_overflow_fixture_is_the_longest_name() {
        let normal = GitStatusRow::fixture(ContentLength::Normal);
        let overflow = GitStatusRow::fixture(ContentLength::Overflow);

        assert!(overflow.parts().0.len() > normal.parts().0.len() * 2);
    }

    #[test]
    fn the_counts_group_renders_only_when_a_count_is_non_zero() {
        assert!(!TrailingContent::empty().has_counts());
        assert!(
            TrailingContent {
                additions: 1,
                ..TrailingContent::empty()
            }
            .has_counts()
        );
        assert!(
            TrailingContent {
                deletions: 1,
                ..TrailingContent::empty()
            }
            .has_counts()
        );
        assert!(
            GitStatusRow::fixture(ContentLength::Normal)
                .trailing
                .has_counts()
        );
    }

    /// Zero-based, so `git-row-guide-0` is the outermost guide on both sides of
    /// the differ. An off-by-one here is an anchor-presence delta, which the
    /// differ ranks first.
    #[test]
    fn guide_ids_are_zero_based() {
        assert_eq!(guide_id(0), "git-row-guide-0");
        assert_eq!(guide_id(3), "git-row-guide-3");
    }

    /// v1.5. The declaration is the *trailing group*, and `git-row-name` — the
    /// flexible sibling that absorbs the ceil excess — is not in it. Declaring
    /// it would compare it against a ceiled target it has no reason to land on,
    /// and would double-count the excess it is already absorbing.
    #[test]
    fn only_the_trailing_group_is_declared_content_sized() {
        assert_eq!(
            CONTENT_SIZED,
            ["git-row-badge", "git-row-added", "git-row-deleted"]
        );
        assert!(!CONTENT_SIZED.contains(&ID_NAME));
        assert!(!CONTENT_SIZED.contains(&ID_ITEM));
        assert!(!CONTENT_SIZED.contains(&ID_BUTTON));
        assert!(!CONTENT_SIZED.contains(&ID_DIR));
        assert!(!CONTENT_SIZED.contains(&ID_ICON));

        for id in CONTENT_SIZED {
            assert!(AnchorId::new(id).content_sized().content_sized, "{id}");
        }
    }

    /// v1.6. Every bare text span, and **not the badge**.
    ///
    /// The badge is the trap this test exists to hold shut: it is one line of
    /// text in a box, which reads like the same case, but its height is
    /// authored (`sm:h-4` → 16px) rather than derived from its 13.33px line
    /// box. Declaring it would compare 16 against 13.33 and invent a 2.67px
    /// delta on an anchor where the archived gate run has both sides at
    /// exactly 16.
    #[test]
    fn every_bare_text_span_is_line_sized_and_the_badge_is_not() {
        assert_eq!(
            LINE_SIZED,
            [
                "git-row-name",
                "git-row-dir",
                "git-row-added",
                "git-row-deleted"
            ]
        );
        assert!(
            !LINE_SIZED.contains(&ID_BADGE),
            "the badge's height is h-4/h-5, not its line box"
        );
        for id in [ID_ITEM, ID_BUTTON, ID_ICON] {
            assert!(!LINE_SIZED.contains(&id), "{id} paints no text");
        }

        for id in LINE_SIZED {
            assert!(AnchorId::new(id).line_sized().line_sized, "{id}");
        }

        // The two counts are the overlap: their width is their advance width
        // and their height is their line box, so they declare both.
        for id in [ID_ADDED, ID_DELETED] {
            assert!(
                CONTENT_SIZED.contains(&id) && LINE_SIZED.contains(&id),
                "{id}"
            );
        }
        // And `git-row-name` is line-sized only: it is the flexible sibling
        // that absorbs the ceil excess along the inline axis.
        assert!(LINE_SIZED.contains(&ID_NAME) && !CONTENT_SIZED.contains(&ID_NAME));
    }

    /// The `sm:` pair is a **viewport** media query at Tailwind's stock
    /// `40rem`, and `>=` means 640 itself is already `sm`.
    #[test]
    fn the_breakpoint_is_the_viewports_and_is_inclusive() {
        assert!((BREAKPOINT_SM - 640.0).abs() < f32::EPSILON);
        assert_eq!(Breakpoint::of(639.9), Breakpoint::Base);
        assert_eq!(Breakpoint::of(640.0), Breakpoint::Sm);
        assert_eq!(Breakpoint::of(1200.0), Breakpoint::Sm);
        assert_eq!(Breakpoint::of(0.0), Breakpoint::Base);
        // A real Crowbar window is wider than 640, so the default is the
        // variant the app actually renders.
        assert_eq!(Breakpoint::default(), Breakpoint::Sm);
        assert_eq!(
            GitStatusRow::fixture(ContentLength::Normal).breakpoint,
            Breakpoint::Sm
        );
    }

    /// The two variants, in the units the row states them in: 16px/10px above
    /// the breakpoint and 20px/12px below it, against a 16px root.
    ///
    /// Both pairs were measured off the live React row — one capture at
    /// 74.11×16 with a 10px face, one at 87.33×20 with a 12px face — and the
    /// arithmetic here is the class list agreeing with the measurement rather
    /// than substituting for it.
    #[test]
    fn the_badge_variants_are_the_measured_pairs() {
        const ROOT: f32 = 16.0;

        assert!(
            (BADGE_HEIGHT_SM * ROOT - 16.0).abs() < f32::EPSILON,
            "sm:h-4"
        );
        assert!(
            (BADGE_TEXT_SM * ROOT - 10.0).abs() < f32::EPSILON,
            "sm:text"
        );
        assert!(
            (BADGE_HEIGHT_BASE * ROOT - 20.0).abs() < f32::EPSILON,
            "h-5"
        );
        assert!(
            (BADGE_TEXT_BASE * ROOT - 12.0).abs() < f32::EPSILON,
            "text-xs"
        );
    }
}
