//! `sidebar_skeleton` — the sidebar's loading placeholder: two chat rows, a
//! divider, and two repo groups of a header row plus two workspace rows.
//!
//! The native half of `web/src/components/layout/sidebar-skeleton.tsx`, which
//! composes eighteen instances of `web/src/components/ui/skeleton.tsx`'s
//! `<Skeleton>` — already ported as [`crate::components::skeleton`] — across
//! **five** distinct box shapes, two more than that module's own
//! [`crate::components::skeleton::CallSite`] enum names. See §2 below for
//! why this file does not extend that enum, and
//! `native/mapping/sidebar-skeleton.md` for the full derivation.
//!
//! # Why there is no reference: **this composition is proven to never mount,
//! not merely unobserved**
//!
//! `skeleton.rs`'s own module docs already establish the stronger form of
//! this finding for the *primitive*: `<Skeleton>`'s only call site anywhere
//! in `web/src` is `<SidebarSkeleton>`'s own rows, and `<SidebarSkeleton>`'s
//! own only call site is a `<Suspense fallback={<SidebarSkeleton />}>`
//! wrapping `FileExplorerTree` — a **static import** with no suspending
//! source in its subtree (`grep` finds no `React.lazy`, `useSuspenseQuery` or
//! `use()` under it). A `<Suspense>` boundary shows its fallback only while a
//! descendant suspends, so the fallback never mounts: a live count of
//! `[data-slot="skeleton"]` was **0** in every state, including immediately
//! after a full reload. That finding is about the primitive; it applies with
//! exactly the same force to this file, because `<SidebarSkeleton>` **is**
//! that fallback — there is no other call site for this composition to be
//! captured from, ever, on any build. No reference was captured, attempted,
//! or fabricated; there is no `/tmp/p3-ref-sidebar-skeleton.json`.
//!
//! # 1. The composition has no parameters at all
//!
//! `sidebar-skeleton.tsx` hardcodes `[1, 2].map(...)` three times — two chat
//! rows, two repo groups, two workspace rows per group — with no prop of any
//! kind. [`SidebarSkeleton`] is therefore a unit struct: every live cell is
//! the same cell.
//!
//! # 2. Five shapes, and why this file does not touch `skeleton::CallSite`
//!
//! | Row | Bars | Shape |
//! |---|---|---|
//! | chat row (×2) | icon, title, meta | `h-5 w-5 rounded-md` · `h-3 flex-1 rounded` · `h-3 w-8 rounded` |
//! | repo header row (×2) | icon, name | `h-5 w-5 rounded-md` · **`h-3 w-24 rounded`** |
//! | workspace row (×4) | title, meta | `h-3 flex-1 rounded` · **`h-3 w-12 rounded`** |
//!
//! The two bold shapes — a fixed **96px** repo name and a fixed **48px**
//! workspace meta bar — are not in `skeleton::CallSite`'s four arms
//! (`None`/`Icon`/`Title`/`Meta`, where `Meta` is the chat row's **32px**
//! bar). `skeleton.rs`'s own count ("seven shapes... five `rounded`, two
//! `rounded-md`") already tallies all **seven DOM positions** this file
//! builds (3 in a chat row + 2 in a repo header row + 2 in a workspace row),
//! it just does not mean `CallSite` covers all of them — three of those seven
//! positions repeat a chat-row shape (`Icon`×2, `Title`×3 counting workspace
//! rows) and two are genuinely new.
//!
//! `Skeleton` carries no `id` field — every instance anchors under the fixed
//! `skeleton::ID_SKELETON`, which is right for one placeholder measured on
//! its own and wrong for eighteen under one root (the same shape of
//! collision `sidebar_project_header.rs`'s module docs record for `Button`).
//! Extending either struct risks a merge conflict with sibling P3.52 workers
//! touching the same shared file for no measurement gain — this composition
//! has no reference regardless. This file instead reuses `skeleton.rs`'s own
//! **public values** — [`crate::components::skeleton::RADIUS_CALL_SITE`] and
//! `theme.color_muted`/`theme.radius_md` — to paint the same two shapes
//! `skeleton::CallSite` already covers, and adds its own two constants for
//! the two it does not, each with its own authored anchor id.

use gpui::{
    AnyElement, Div, IntoElement as _, ParentElement as _, Pixels, SharedString, Styled as _, div,
    px,
};

use super::anchor::{AnchorId, AnchorSink};
use super::skeleton::RADIUS_CALL_SITE;
use crate::theme::Theme;

/// The single anchor the outer column carries. Every skeleton bar inside it
/// carries its own — see [`SidebarSkeleton::bars`].
pub const ID_SIDEBAR_SKELETON: &str = "sidebar-skeleton";

/// **Nothing.** Every bar in this composition pins both axes or takes
/// `flex-1`; none is a content width, the same finding `skeleton.rs` already
/// makes about the primitive.
pub const CONTENT_SIZED: [&str; 0] = [];

/// **Nothing.** No anchor here paints text.
pub const LINE_SIZED: [&str; 0] = [];

/// `--spacing`, Tailwind's stock `0.25rem` at a 16px root.
const SPACING: f32 = 4.0;

/// `py-1` on the outer column.
pub const OUTER_PADDING_Y: Pixels = px(SPACING);
/// `px-2` on the outer column.
pub const OUTER_PADDING_X: Pixels = px(SPACING * 2.0);
/// `space-y-0.5` — the gap between the outer column's own top-level children
/// (two chat rows, the divider, two repo groups), reproduced as a flex `gap`
/// rather than the `margin-top`-on-siblings CSS actually uses; the two
/// produce the same picture for a column with no `flex-wrap`.
pub const OUTER_GAP: Pixels = px(SPACING * 0.5);
/// `space-y-0.5` inside a repo group, between its header row and its
/// workspace rows.
pub const GROUP_GAP: Pixels = px(SPACING * 0.5);

/// `h-9` — every row's own height.
pub const ROW_HEIGHT: Pixels = px(SPACING * 9.0);
/// `gap-2` — between a row's own bars.
pub const ROW_GAP: Pixels = px(SPACING * 2.0);
/// `mx-1.5` — a row's own horizontal margin, inside the column's `px-2`.
pub const ROW_MARGIN_X: Pixels = px(SPACING * 1.5);
/// `px-2` — a row's own internal padding.
pub const ROW_PADDING_X: Pixels = px(SPACING * 2.0);
/// `pl-6` on a workspace row — **replaces** [`ROW_PADDING_X`]'s left half
/// rather than adding to it; `tailwind-merge` drops the earlier `px-2`'s
/// `padding-left` in favour of the later, more specific `pl-6`. The right
/// side keeps [`ROW_PADDING_X`].
pub const WORKSPACE_ROW_PADDING_LEFT: Pixels = px(SPACING * 6.0);

/// `h-5 w-5` — an icon bar (chat row, repo header row).
pub const ICON_EXTENT: Pixels = px(SPACING * 5.0);
/// `h-3` — every text-shaped bar's height (title, meta, repo name,
/// workspace meta).
pub const TEXT_BAR_HEIGHT: Pixels = px(SPACING * 3.0);
/// `w-8` — the chat row's meta bar.
pub const CHAT_META_WIDTH: Pixels = px(SPACING * 8.0);
/// `w-24` — the repo header row's name bar. **Not** in `skeleton::CallSite` —
/// see the module docs.
pub const REPO_NAME_WIDTH: Pixels = px(SPACING * 24.0);
/// `w-12` — a workspace row's meta bar. **Not** in `skeleton::CallSite` —
/// see the module docs.
pub const WORKSPACE_META_WIDTH: Pixels = px(SPACING * 12.0);

/// `my-1 mx-3 h-px` — the divider between the chat rows and the repo groups.
pub const DIVIDER_MARGIN_Y: Pixels = px(SPACING);
pub const DIVIDER_MARGIN_X: Pixels = px(SPACING * 3.0);
pub const DIVIDER_HEIGHT: Pixels = px(1.0);

/// How many chat rows the source hardcodes.
pub const CHAT_ROWS: usize = 2;
/// How many repo groups the source hardcodes.
pub const REPO_GROUPS: usize = 2;
/// How many workspace rows each repo group hardcodes.
pub const WORKSPACE_ROWS_PER_GROUP: usize = 2;

/// The whole placeholder. No fields: every live cell is the same cell — see
/// the module docs.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct SidebarSkeleton;

impl SidebarSkeleton {
    /// One bar: `bg-muted`, a corner, a fixed extent on one or both axes, or
    /// `flex-1` where the source's does. Mirrors `skeleton::Skeleton::shell`'s
    /// own paint without going through its fixed `id` — see the module docs.
    fn bar(
        theme: &Theme,
        radius: Pixels,
        height: Pixels,
        width: Option<Pixels>,
        grows: bool,
    ) -> Div {
        let mut element = div().bg(theme.color_muted).rounded(radius).h(height);
        if let Some(width) = width {
            element = element.w(width);
        }
        if grows {
            element = element.flex_grow_1();
        }
        element
    }

    fn icon_bar(theme: &Theme) -> Div {
        Self::bar(
            theme,
            theme.radius_md.value(),
            ICON_EXTENT,
            Some(ICON_EXTENT),
            false,
        )
        .flex_shrink_0()
    }

    fn title_bar(theme: &Theme) -> Div {
        Self::bar(theme, RADIUS_CALL_SITE, TEXT_BAR_HEIGHT, None, true)
    }

    fn meta_bar(theme: &Theme, width: Pixels) -> Div {
        Self::bar(theme, RADIUS_CALL_SITE, TEXT_BAR_HEIGHT, Some(width), false)
    }

    /// A `flex h-9 items-center gap-2 mx-1.5 px-2` row, with `pl-6` swapped in
    /// on the left where `extra_left_padding` asks for it.
    fn row_shell(extra_left_padding: bool) -> Div {
        let element = div()
            .flex()
            .flex_row()
            .items_center()
            .h(ROW_HEIGHT)
            .gap(ROW_GAP)
            .mx(ROW_MARGIN_X)
            .px(ROW_PADDING_X);
        if extra_left_padding {
            element.pl(WORKSPACE_ROW_PADDING_LEFT)
        } else {
            element
        }
    }

    /// A chat row: icon, title, meta — `sidebar-skeleton-chat-{i}-{slot}`.
    fn chat_row(theme: &Theme, anchors: &dyn AnchorSink, i: usize) -> AnyElement {
        Self::row_shell(false)
            .child(anchors.boxed(bar_id("chat", i, "icon"), Self::icon_bar(theme)))
            .child(anchors.boxed(bar_id("chat", i, "title"), Self::title_bar(theme)))
            .child(anchors.boxed(
                bar_id("chat", i, "meta"),
                Self::meta_bar(theme, CHAT_META_WIDTH),
            ))
            .into_any_element()
    }

    /// The divider between the chat rows and the repo groups.
    fn divider(theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        anchors.boxed(
            AnchorId::new(ID_DIVIDER),
            div()
                .bg(theme.border)
                .h(DIVIDER_HEIGHT)
                .mx(DIVIDER_MARGIN_X)
                .my(DIVIDER_MARGIN_Y),
        )
    }

    /// One repo group: a header row (icon, name) plus
    /// [`WORKSPACE_ROWS_PER_GROUP`] workspace rows (title, meta).
    fn repo_group(theme: &Theme, anchors: &dyn AnchorSink, i: usize) -> AnyElement {
        let mut group = div().flex().flex_col().gap(GROUP_GAP).child(
            Self::row_shell(false)
                .child(anchors.boxed(bar_id("repo", i, "icon"), Self::icon_bar(theme)))
                .child(anchors.boxed(
                    bar_id("repo", i, "name"),
                    Self::meta_bar(theme, REPO_NAME_WIDTH),
                )),
        );
        for j in 0..WORKSPACE_ROWS_PER_GROUP {
            let workspace_id = format!("repo-{i}-workspace-{j}");
            group = group.child(
                Self::row_shell(true)
                    .child(anchors.boxed(
                        AnchorId::new(format!("sidebar-skeleton-{workspace_id}-title")),
                        Self::title_bar(theme),
                    ))
                    .child(anchors.boxed(
                        AnchorId::new(format!("sidebar-skeleton-{workspace_id}-meta")),
                        Self::meta_bar(theme, WORKSPACE_META_WIDTH),
                    )),
            );
        }
        group.into_any_element()
    }

    /// The outer column's own box.
    fn shell() -> Div {
        div()
            .flex()
            .flex_col()
            .gap(OUTER_GAP)
            .py(OUTER_PADDING_Y)
            .px(OUTER_PADDING_X)
    }

    /// Renders the whole placeholder, opting every bar into `anchors`.
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let mut children: Vec<AnyElement> = Vec::new();
        for i in 0..CHAT_ROWS {
            children.push(Self::chat_row(theme, anchors, i));
        }
        children.push(Self::divider(theme, anchors));
        for i in 0..REPO_GROUPS {
            children.push(Self::repo_group(theme, anchors, i));
        }
        anchors.root(ID_SIDEBAR_SKELETON.into(), Self::shell().children(children))
    }
}

/// The divider's own anchor id.
pub const ID_DIVIDER: &str = "sidebar-skeleton-divider";

/// Builds a bar's anchor id: `sidebar-skeleton-{row}-{i}-{slot}`.
fn bar_id(row: &str, i: usize, slot: &str) -> AnchorId {
    AnchorId::new(SharedString::from(format!(
        "sidebar-skeleton-{row}-{i}-{slot}"
    )))
}

#[cfg(test)]
mod tests {
    use super::{
        CHAT_META_WIDTH, CHAT_ROWS, DIVIDER_HEIGHT, DIVIDER_MARGIN_X, DIVIDER_MARGIN_Y, GROUP_GAP,
        ICON_EXTENT, OUTER_GAP, OUTER_PADDING_X, OUTER_PADDING_Y, REPO_GROUPS, REPO_NAME_WIDTH,
        ROW_GAP, ROW_HEIGHT, ROW_MARGIN_X, ROW_PADDING_X, TEXT_BAR_HEIGHT, WORKSPACE_META_WIDTH,
        WORKSPACE_ROW_PADDING_LEFT, WORKSPACE_ROWS_PER_GROUP,
    };
    use gpui::px;

    #[test]
    fn every_length_is_the_compiled_spacing_multiple() {
        const STEP: f32 = 4.0;
        assert_eq!(OUTER_PADDING_Y, px(STEP)); // py-1
        assert_eq!(OUTER_PADDING_X, px(STEP * 2.0)); // px-2
        assert_eq!(OUTER_GAP, px(STEP * 0.5)); // space-y-0.5
        assert_eq!(GROUP_GAP, px(STEP * 0.5)); // space-y-0.5
        assert_eq!(ROW_HEIGHT, px(STEP * 9.0)); // h-9
        assert_eq!(ROW_GAP, px(STEP * 2.0)); // gap-2
        assert_eq!(ROW_MARGIN_X, px(STEP * 1.5)); // mx-1.5
        assert_eq!(ROW_PADDING_X, px(STEP * 2.0)); // px-2
        assert_eq!(WORKSPACE_ROW_PADDING_LEFT, px(STEP * 6.0)); // pl-6
        assert_eq!(ICON_EXTENT, px(STEP * 5.0)); // h-5 w-5
        assert_eq!(TEXT_BAR_HEIGHT, px(STEP * 3.0)); // h-3
        assert_eq!(CHAT_META_WIDTH, px(STEP * 8.0)); // w-8
        assert_eq!(REPO_NAME_WIDTH, px(STEP * 24.0)); // w-24
        assert_eq!(WORKSPACE_META_WIDTH, px(STEP * 12.0)); // w-12
        assert_eq!(DIVIDER_MARGIN_Y, px(STEP)); // my-1
        assert_eq!(DIVIDER_MARGIN_X, px(STEP * 3.0)); // mx-3
        assert_eq!(DIVIDER_HEIGHT, px(1.0)); // h-px, a literal
    }

    /// The two shapes `skeleton::CallSite` does not carry are genuinely
    /// different numbers from the ones it does — or "two more shapes" would
    /// be decoration.
    #[test]
    fn the_two_new_shapes_are_not_the_chat_rows_meta_width() {
        assert_ne!(REPO_NAME_WIDTH, CHAT_META_WIDTH);
        assert_ne!(WORKSPACE_META_WIDTH, CHAT_META_WIDTH);
        assert_ne!(REPO_NAME_WIDTH, WORKSPACE_META_WIDTH);
    }

    #[test]
    fn the_hardcoded_counts_match_the_sources_two_maps() {
        assert_eq!(CHAT_ROWS, 2);
        assert_eq!(REPO_GROUPS, 2);
        assert_eq!(WORKSPACE_ROWS_PER_GROUP, 2);
    }
}
