//! The **stateful** gate surface: one row of the file explorer tree.
//!
//! The native half of
//! `web/src/features/file-explorer/file-explorer/components/file-explorer-tree-item.tsx`
//! rendered through `components/ui/tree-row.tsx` — the same `TreeRow` base
//! [`super::git_status_row`] uses, and deliberately so. What is different is the
//! only thing this surface exists for:
//!
//! > **This row actually enters the §8.3 state axis.** `SidebarTreeRow` takes an
//! > `active` prop that no live consumer passes, so `data-active` is dead on the
//! > git status row and every `selected` cell of the matrix compares a resting
//! > row against a resting row. The file explorer row sets `data-active` from
//! > `isActive` on every render, and it is inside `.file-tree-container`, so the
//! > *scoped* half of `file-explorer-tree.css` applies to it as well.
//!
//! # What the scoped rules change, and why this is not `sidebar_tree::row_button`
//!
//! [`super::sidebar_tree`]'s module docs record which of that stylesheet's rules
//! are unscoped — `.file-tree-item`'s width, the `::before` group, and
//! `.file-tree-row:hover` — and note that the rest is scoped to
//! `.file-tree-container`, **which the git status panel is not inside**. This
//! surface *is*, so four more rules fire on the button and none of them fire on
//! the git row:
//!
//! ```text
//! .file-tree-container .file-tree-item button {
//!   box-sizing: border-box !important;
//!   border: 1px solid transparent !important;   /* the git row has NO border */
//!   border-radius: 2px !important;              /* beats rounded-md's 8px    */
//!   background-color: transparent !important;
//!   width: 100% !important; min-width: 100% !important;
//!   display: flex !important; justify-content: flex-start !important;
//! }
//! .file-tree-container .file-tree-item button:focus-visible {
//!   border-color: color-mix(in srgb, var(--accent) 42%, var(--border)) !important;
//! }
//! ```
//!
//! The last one is the reason [`RowState::focused`] paints something here and
//! nothing on the git row: a 1px border whose colour moves off `transparent`.
//! `native/oracle/ANCHORS.md` v1.3 compares `border.color` only where
//! `border.w > 0`, and here it is exactly 1 — so focus is a **comparable** cell
//! on this surface rather than a vacuous one.
//!
//! # Two numbers that are *not* the git row's, and were checked rather than assumed
//!
//! * **The indent step is 16, not 14.** It is `settings.fileTreeIndentSize`,
//!   whose default is `16` (`web/src/features/settings/config/default-settings.ts`).
//!   The sidebar tree's own `SIDEBAR_TREE_INDENT_SIZE` is 14 and is a different
//!   constant; reusing [`super::sidebar_tree::INDENT_SIZE`] here would put every
//!   guide and the whole leading padding two pixels per level out.
//! * **The line height is `text-sm`'s, not `leading-[1.35]`.** `GitFileItem`
//!   authors `leading-[1.35]` on its row; this one authors nothing, so the line
//!   height is whatever `text-sm` sets. Compiling the app's own `index.css`
//!   through its own Tailwind (4.3.0) gives
//!
//!   ```text
//!   .text-sm { font-size: var(--text-sm);
//!              line-height: var(--tw-leading, var(--text-sm--line-height)) }
//!   --text-sm: 0.875rem;  --text-sm--line-height: calc(1.25 / 0.875);
//!   ```
//!
//!   and nothing in this app redefines either. So it is 14px text on a 20px line
//!   box, where the git row is 14px on 18.9.
//!
//! # The third thing that is not the git row's: **the name is coloured by git status**
//!
//! See [`GitStatus`]. `GitFileItem` pins its filename at `text-foreground` and
//! never moves it; the file explorer row paints the name — and a trailing status
//! letter — in one of six `--git-*` tokens, chosen by the file's git status.
//! This is what the oracle found on the `file-tree-row · dark · short · selected`
//! cell: one delta, `file-row-name.fg` `#f5f5f5ff` against a reference painting
//! `#fe9a00ff`, because the fixture's `a.ts` was modified and the port painted
//! every filename on the default foreground.

use gpui::{
    AnyElement, Div, FontWeight, IntoElement as _, ParentElement as _, Pixels, SharedString,
    Styled as _, div, px, relative,
};

use super::anchor::{AnchorId, AnchorSink};
use super::sidebar_tree::{self, BASE_INDENT, GUIDE_SHIFT, ICON_SIZE, ITEM_RADIUS, ROW_HEIGHT};
use super::sidebar_tree::{GuideInset, RowState, guide_inset};
use crate::theme::{Color, Theme};

/// The root anchor: the `.file-tree-item` wrapper. Every other bound is reported
/// relative to it (`native/oracle/ANCHORS.md` §4), and it is the layer whose
/// `::before` paints hover and selection.
pub const ID_ITEM: &str = "file-row-item";
/// The `<button>` — `TreeRow`, carrying the border that focus moves.
pub const ID_BUTTON: &str = "file-row-button";
/// The file-type icon's 14px box.
pub const ID_ICON: &str = "file-row-icon";
/// The file name span.
pub const ID_NAME: &str = "file-row-name";

/// The anchors on this row whose boxes size to their own text
/// (`native/oracle/ANCHORS.md` v1.5).
///
/// One, and it is the name. The span is a flex item with `flex: 0 1 auto` and
/// no basis, so while the row has room its used width **is** its max-content
/// width — the fractional advance in `WebKit`, `ceil()`ed by gpui. That is
/// exactly the one-directional error v1.5 exists for.
///
/// **It stays correct in the `overflow` cell**, which is the question worth
/// asking before declaring a shrinkable box content-sized: there the span is
/// clamped to the space left by the icon and the paddings, all of which are
/// whole pixels, so the reference width is an integer and `ceil` of it is
/// itself. The declaration therefore never moves the target away from a number
/// the engine can hit.
///
/// The corresponding React declaration is `data-oracle-content-sized` in
/// `file-explorer-tree-item.tsx`, and the two lists have to stay identical: a
/// declaration on one side only is a `FieldPresence` delta that forgives
/// nothing.
pub const CONTENT_SIZED: [&str; 1] = [ID_NAME];

/// The anchors on this row whose **box height is their own line box**
/// (`native/oracle/ANCHORS.md` v1.6).
///
/// The name span, and only it. It is a blockified flex item holding one line of
/// text with no authored height, so its border box is the line box.
///
/// **The button is deliberately absent, and it is this surface's version of the
/// badge trap.** It paints text — it is the element the font is declared on —
/// but `h-6` pins its border box at 24px around a 20px line box. Declaring it
/// would compare 24 against 20 and manufacture a 4px delta on an anchor where
/// both engines agree exactly.
pub const LINE_SIZED: [&str; 1] = [ID_NAME];

/// One indent guide, per level: `file-row-guide-0`, `file-row-guide-1`, …
///
/// Zero-based, matching the `level` index the React `left` calculation uses, so
/// `file-row-guide-0` is the outermost guide on both sides.
#[must_use]
pub fn guide_id(level: u16) -> SharedString {
    SharedString::from(format!("file-row-guide-{level}"))
}

/// `settings.fileTreeIndentSize` — one nesting level's worth of leading padding.
///
/// **16, not the sidebar tree's 14.** The file explorer reads this off the
/// settings store (`FileExplorerTree` passes `indentSize={settings.fileTreeIndentSize}`)
/// and the default is 16. A row measured against a reference at the default
/// setting has to use the same number, and the two constants being neighbours
/// in the same crate is exactly how they would come to be confused.
pub const INDENT_SIZE: Pixels = px(16.0);

/// Tailwind's stock `--text-sm--line-height`, `calc(1.25 / 0.875)`.
///
/// Written as the division rather than as `1.4285714` because that is how the
/// stylesheet writes it, and because the pair it is derived from is the pair
/// this row's font size comes from.
const LINE_HEIGHT: f32 = 1.25 / 0.875;

/// `color-mix(in srgb, var(--accent) 42%, var(--border))` — the `:focus-visible`
/// border.
const FOCUS_BORDER_ACCENT: f32 = 42.0;

/// `.file-tree-container .file-tree-item button { border: 1px solid transparent }`.
pub const BUTTON_BORDER: Pixels = px(1.0);

/// The row's leading padding at `depth` — `FILE_TREE_BASE_INDENT + depth × 16`.
///
/// Padding rather than a spacer, for the reason `TreeRow` gives: it is what lets
/// the wrapper's background span the full width.
#[must_use]
pub fn leading_padding(depth: u16) -> Pixels {
    BASE_INDENT + INDENT_SIZE * f32::from(depth)
}

/// Where a guide's 7px host box starts, with `translateX(-3px)` folded in.
///
/// The same shape as [`super::sidebar_tree::guide_left`] over this surface's
/// indent step. It is a separate function rather than a parameter on that one
/// because the two surfaces genuinely have two different steps, and a shared
/// helper taking the step would read as though either value were a choice.
#[must_use]
pub fn guide_left(level: u16, icon_offset: Pixels) -> Pixels {
    BASE_INDENT + INDENT_SIZE * f32::from(level) + icon_offset - GUIDE_SHIFT
}

/// The button's border colour, which is the whole of what focus paints here.
///
/// `transparent` at rest — the base rule's own colour, not an absence: the
/// border is 1px in every state, so `native/oracle/ANCHORS.md` v1.3 compares its
/// colour in every state too.
#[must_use]
pub fn button_border_color(theme: &Theme, state: RowState) -> Color {
    if state.focused {
        theme.accent.mix(FOCUS_BORDER_ACCENT, theme.border)
    } else {
        Color::TRANSPARENT
    }
}

/// `text-[10px]` on the trailing status letter.
///
/// A Tailwind arbitrary value, so it is a **pixel** length and not one of the
/// `rem`-based type tokens: `text-[10px]` does not scale with `--app-ui-scale`
/// the way `ui-text-xs` does, and spelling it in rems here would make it do so.
const STATUS_LETTER_SIZE: f32 = 10.0;

/// `leading-none` on the status letter — `line-height: 1`, not the row's
/// inherited `calc(1.25 / 0.875)`.
const STATUS_LETTER_LINE_HEIGHT: f32 = 1.0;

/// `opacity-80` on the status letter.
///
/// A real `opacity`, not a colour mixed down to 80% alpha: the two are the same
/// composite over an opaque backdrop but different things to read back, and the
/// letter's *declared* colour is the same token the name gets. Keeping them one
/// value is what makes `the_letter_takes_the_same_colour_as_the_name` a
/// meaningful assertion rather than a restatement of an alpha.
const STATUS_LETTER_OPACITY: f32 = 0.8;

/// The git statuses the file explorer distinguishes, and **the complete list**.
///
/// Read off `web/src/features/file-explorer/file-explorer/lib/file-tree-git-status.ts`
/// rather than inferred from a sample, because two of the six differ from what
/// two measured colours would have suggested. `GitFile['status']` is a closed
/// union of five words — `modified | added | deleted | untracked | renamed` —
/// and `staged` is not one of them: it is a **separate boolean** on the same
/// file, and `getFileTreeGitStatusDecoration` consults it in exactly one arm.
/// So the vocabulary is five statuses and six decorations.
///
/// There is deliberately **no `conflicted` and no `ignored`**. The tree has no
/// decoration for either: an ignored entry is dimmed by `file.ignored &&
/// 'opacity-50'` on the *row*, which is not a colour and not this mapping, and
/// a conflicted file has no representation in `GitFile` at all. Inventing a
/// token for either would be this port adding a state the thing it is porting
/// does not have.
///
/// # The mapping, from the source
///
/// | status | `colorClassName` | token | letter |
/// |---|---|---|---|
/// | `modified`, unstaged | `text-git-modified` | `--git-modified` | `M` |
/// | `modified`, staged | `text-git-modified-staged` | `--git-modified-staged` | `M` |
/// | `added` | `text-git-added` | `--git-added` | `A` |
/// | `deleted` | `text-git-deleted` | `--git-deleted` | `D` |
/// | `untracked` | `text-git-untracked` | `--git-untracked` | `U` |
/// | `renamed` | `text-git-renamed` | `--git-renamed` | `R` |
///
/// A `text-git-*` class resolves through `@theme inline`'s
/// `--color-git-modified: var(--git-modified)`, so the class and the token are
/// one value; [`GitStatus::color`] reads the `--git-*` half, which is the half
/// the git status row already reads for its `+n` / `-n`.
///
/// # Where the colour lands, and where it does not
///
/// On the **name** and on the **status letter**, and on nothing else. The icon
/// keeps `text-muted-foreground` — it is a sibling of the label group and the
/// class is on neither of them — and the row's background, border and radius
/// are untouched. A status therefore moves exactly one compared field,
/// `file-row-name.fg`, plus the layout the letter takes up.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash)]
pub enum GitStatus {
    /// `modified` with `staged: false`.
    Modified,
    /// `modified` with `staged: true` — the one arm the boolean reaches.
    ModifiedStaged,
    /// `added`.
    Added,
    /// `deleted`.
    Deleted,
    /// `untracked`.
    Untracked,
    /// `renamed`.
    Renamed,
}

/// Every [`GitStatus`], in decreasing [`GitStatus::priority`].
///
/// Written down so that adding a seventh without deciding its token, its letter
/// and its place in the folder ordering is a compile error in
/// [`GitStatus::color`] rather than a quietly missing row.
pub const ALL_GIT_STATUSES: [GitStatus; 6] = [
    GitStatus::Deleted,
    GitStatus::ModifiedStaged,
    GitStatus::Modified,
    GitStatus::Renamed,
    GitStatus::Added,
    GitStatus::Untracked,
];

impl GitStatus {
    /// The status's name in `--git-status` and in this crate's tests.
    ///
    /// `modified-staged` rather than a bare `staged`, because a bare `staged` is
    /// not a status in the thing being ported: it is `modified` with a boolean
    /// set, and every other spelling invites a seventh row in the table that
    /// the React source does not have.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Modified => "modified",
            Self::ModifiedStaged => "modified-staged",
            Self::Added => "added",
            Self::Deleted => "deleted",
            Self::Untracked => "untracked",
            Self::Renamed => "renamed",
        }
    }

    /// The single letter the row paints after the filename.
    ///
    /// Both `modified` arms are `M`: the staged one differs in colour only,
    /// which is exactly why the letter cannot stand in for the status.
    #[must_use]
    pub const fn letter(self) -> &'static str {
        match self {
            Self::Modified | Self::ModifiedStaged => "M",
            Self::Added => "A",
            Self::Deleted => "D",
            Self::Untracked => "U",
            Self::Renamed => "R",
        }
    }

    /// `gitStatusPriority`, verbatim — including the `+ 1` for a staged
    /// modification.
    ///
    /// The numbers are the React table's own (50/40/30/20/10) rather than a
    /// renumbering, so the two can be read against each other. The gaps are what
    /// leave room for `getGitStatusPriority`'s `priority + 1`, which is the only
    /// reason a staged modification outranks an unstaged one at all.
    #[must_use]
    pub const fn priority(self) -> u8 {
        match self {
            Self::Deleted => 50,
            Self::ModifiedStaged => 41,
            Self::Modified => 40,
            Self::Renamed => 30,
            Self::Added => 20,
            Self::Untracked => 10,
        }
    }

    /// The token the name and the letter are painted in.
    #[must_use]
    pub const fn color(self, theme: &Theme) -> Color {
        match self {
            Self::Modified => theme.git_modified,
            Self::ModifiedStaged => theme.git_modified_staged,
            Self::Added => theme.git_added,
            Self::Deleted => theme.git_deleted,
            Self::Untracked => theme.git_untracked,
            Self::Renamed => theme.git_renamed,
        }
    }
}

/// **How a folder's colour is derived**: the highest-priority status anywhere
/// beneath it wins, and ties go to the first one seen.
///
/// `createFileTreeGitStatusLookup` builds two maps. Files get their own
/// decoration. Directories get one per *ancestor* of every changed path — for
/// `src/lib/a.ts` it credits `src` and `src/lib`, never the file itself — and
/// keeps the decoration whose [`GitStatus::priority`] is highest:
///
/// ```text
/// if (nextPriority > currentPriority) { directories.set(currentPath, decoration) }
/// ```
///
/// Two details in that line are load-bearing and are why this is a function
/// rather than a `max_by_key`:
///
/// * **The comparison is strict**, so among equal priorities the *first* file in
///   `gitStatus.files` wins. `Iterator::max_by_key` returns the **last**
///   maximum, which is the opposite tie-break and would silently disagree with
///   the reference on any folder holding two files of the same status.
/// * **The aggregate is inherited, not summarised.** A folder holding one
///   deletion and nine additions paints `--git-deleted` — which is the measured
///   `#f94047ff` on `src` that first showed this was a mapping and not a single
///   colour.
///
/// The path walking itself is not here: this port has no file tree yet, and a
/// path-to-ancestors fold with nothing to fold over would be a shape with no
/// caller. What is here is the rule that shape would apply.
#[must_use]
pub fn aggregate<I: IntoIterator<Item = GitStatus>>(statuses: I) -> Option<GitStatus> {
    let mut worst: Option<GitStatus> = None;
    for status in statuses {
        if worst.is_none_or(|best| status.priority() > best.priority()) {
            worst = Some(status);
        }
    }
    worst
}

/// The filename's colour: its status's token, or the inherited foreground.
///
/// `None` is the row with no decoration at all — `getFileTreeEntryGitStatusDecoration`
/// returning `null`, which is every unchanged file and every folder with nothing
/// changed under it. The React name span then carries no colour class and
/// inherits the button's `text-foreground`, which is the `#f5f5f5ff` the port
/// was already painting and must go on painting.
#[must_use]
pub const fn name_color(theme: &Theme, status: Option<GitStatus>) -> Color {
    match status {
        Some(status) => status.color(theme),
        None => theme.foreground,
    }
}

/// One row of the file explorer tree.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct FileTreeRow {
    /// The file's display name — the row paints this and nothing else.
    pub name: SharedString,
    /// `depth`.
    pub depth: u16,
    /// The depth of the row above, which caps the guides at the top of a run.
    pub previous_depth: u16,
    /// The depth of the row below, which caps them at the bottom.
    pub next_depth: u16,
    /// The visual state, as a parameter — see [`RowState`]. gpui resolves a
    /// `.hover(…)` refinement from runtime interaction state the extractor
    /// cannot see, so a row that expressed its states that way would report its
    /// resting paint in every cell of the state axis.
    pub state: RowState,
    /// The file's git status, which colours the name — see [`GitStatus`].
    ///
    /// `None` is an unchanged file, and it is the default: every invocation
    /// written before this existed renders the row it rendered.
    ///
    /// A parameter for the same reason [`RowState`] is one. It is *also* a
    /// parameter because a row has no way to know it: in the React app the
    /// status arrives from a lookup built over the whole `GitStatus` payload
    /// (see [`aggregate`]), which is a level of the tree this surface is one row
    /// of.
    pub git_status: Option<GitStatus>,
}

impl FileTreeRow {
    /// A row for one of the matrix's content lengths.
    ///
    /// The strings are [`super::ContentLength`]'s own, taken through
    /// [`super::split_path`] so that the two gate surfaces show the **same**
    /// name at the same content length. That is not decoration: a difference in
    /// truncation between the two surfaces is then a difference in the row and
    /// not in the fixture.
    #[must_use]
    pub fn fixture(content: super::ContentLength) -> Self {
        let (name, _) = super::split_path(content.path());
        Self {
            name: SharedString::from(name.to_owned()),
            depth: 0,
            previous_depth: 0,
            next_depth: 0,
            state: RowState::resting(),
            git_status: None,
        }
    }

    /// Renders the row, opting every contract anchor into `anchors`.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let content: Vec<AnyElement> = vec![
            anchors.boxed(ID_ICON.into(), Self::icon(theme)),
            self.label(theme, anchors).into_any_element(),
        ];

        let button = anchors.boxed(ID_BUTTON.into(), self.button(theme).children(content));
        let wrapper = sidebar_tree::item(theme, self.state)
            .child(self.guides(theme, anchors))
            .child(button);
        anchors.root(ID_ITEM.into(), wrapper)
    }

    /// The `<button>`: `TreeRow`'s classes as the scoped stylesheet leaves them.
    ///
    /// The font family is named explicitly rather than inherited, because gpui
    /// can only report the *declared* family and an inherited `.SystemUIFont` is
    /// a string the DOM will never produce.
    fn button(&self, theme: &Theme) -> Div {
        let family = theme.font_sans.primary().unwrap_or("sans-serif");
        div()
            .flex()
            .items_center()
            .justify_start()
            .w_full()
            .min_w_full()
            .h(ROW_HEIGHT)
            .gap_1p5()
            .py_1()
            .pr_1p5()
            .pl(leading_padding(self.depth))
            .border(BUTTON_BORDER)
            .border_color(button_border_color(theme, self.state))
            // `border-radius: 2px !important` from the container-scoped rule,
            // which beats `rounded-md`'s 8. The same 2px the wrapper rounds by,
            // and the same constant.
            .rounded(ITEM_RADIUS)
            .whitespace_nowrap()
            .font_family(family)
            .font_weight(FontWeight::NORMAL)
            // `text-sm` — Tailwind's stock `--text-sm`, 0.875rem. `--ui-text-base`
            // is the same 0.875rem and is the token this design system carries;
            // there is no token for Tailwind's own scale, and inventing one would
            // put a value in the system the system does not have.
            .text_size(theme.ui_text_base.value())
            .line_height(relative(LINE_HEIGHT))
            .text_color(theme.foreground)
    }

    /// `.file-tree-guides` — an `absolute inset-0` layer holding one guide per
    /// level. Empty at depth 0, exactly as the React `guideLevels` array is.
    fn guides(&self, theme: &Theme, anchors: &dyn AnchorSink) -> Div {
        let offset = theme.file_tree_guide_icon_offset.value();
        let guides = (0..self.depth).map(|level| {
            let inset: GuideInset = guide_inset(level, self.previous_depth, self.next_depth);
            anchors.boxed(
                guide_id(level).into(),
                sidebar_tree::guide_at(theme, guide_left(level, offset), inset),
            )
        });
        div().absolute().inset_0().children(guides)
    }

    /// The 14px icon box.
    ///
    /// **Empty**, for the reason [`super::GitStatusRow`]'s is: the React icon is
    /// an SVG the icon-theme registry picks from the file name, there is no
    /// native equivalent, and drawing a substitute would put a shape on screen
    /// for the oracle to converge on. The box is what the contract measures.
    fn icon(theme: &Theme) -> Div {
        div()
            .flex()
            .items_center()
            .flex_shrink_0()
            .w(ICON_SIZE)
            .h(ICON_SIZE)
            .text_color(theme.muted_foreground)
    }

    /// `<span class="relative z-1 flex min-w-0 items-baseline gap-1.5">`, the
    /// name span inside it, and — when the file has a git status — the status
    /// letter beside it.
    ///
    /// The wrapper is kept rather than flattened away: it is the box that
    /// carries `min-w-0`, and it is what lets the name shrink while the icon
    /// does not. Anchoring the run inside the span — rather than wrapping the
    /// span in a box anchor — is the same arrangement the git row uses: taffy
    /// stretches a block-level in-flow child to its container's inner width, so
    /// the run's box **is** the span's box and one text anchor reports both.
    ///
    /// The colour goes on the **name's own box**, not on this wrapper, because
    /// that is where the React class is: `cn('select-none truncate
    /// whitespace-nowrap', gitStatusDecoration?.colorClassName)` is on the name
    /// span alone. Putting it on the wrapper would give the same `fg` today and
    /// paint the wrong thing the moment a second uncoloured child is added.
    fn label(&self, theme: &Theme, anchors: &dyn AnchorSink) -> Div {
        let mut group = div()
            .flex()
            .items_baseline()
            .gap_1p5()
            .min_w_0()
            .text_color(theme.foreground)
            .child(
                div()
                    .min_w_0()
                    .truncate()
                    .text_color(name_color(theme, self.git_status))
                    .child(anchors.text(
                        AnchorId::new(ID_NAME).content_sized().line_sized(),
                        self.name.clone(),
                    )),
            );
        if let Some(status) = self.git_status {
            group = group.child(Self::status_letter(theme, status));
        }
        group
    }

    /// `<span class="shrink-0 select-none font-mono text-[10px] leading-none
    /// opacity-80">M</span>` — the trailing status letter.
    ///
    /// **Unanchored, on purpose.** The React span carries no `data-oracle-id`,
    /// and giving the native one an anchor the reference cannot produce is a
    /// `FieldPresence` delta that forgives nothing — the same reason the row's
    /// anchor list has stayed at six.
    ///
    /// It is nonetheless *rendered*, because it is a flex sibling of the name
    /// inside a `gap-1.5` container: leaving it out does not leave the
    /// comparison alone, it hands the name six pixels of gap and the letter's
    /// whole advance that the reference does not have. That is invisible in the
    /// `short` and `normal` cells, where the name is content-sized and takes its
    /// max-content width regardless — and it is a direct width delta on
    /// `file-row-name` in the `overflow` cell, which is the cell truncation is
    /// measured in.
    ///
    /// One honest limit: the family is [`Theme::font_mono`]'s first stack entry,
    /// and the reference's `--font-mono` begins `var(--editor-font-family, …)` —
    /// a *runtime* Settings value. There is no static answer to what the DOM
    /// shapes this letter with, so its advance is close rather than exact. The
    /// letter is one glyph at 10px; being approximately right about it is
    /// strictly nearer than omitting it, and it never reaches a compared field
    /// on its own.
    fn status_letter(theme: &Theme, status: GitStatus) -> Div {
        div()
            .flex_shrink_0()
            .font_family(theme.font_mono.primary().unwrap_or("monospace"))
            .text_size(px(STATUS_LETTER_SIZE))
            .line_height(relative(STATUS_LETTER_LINE_HEIGHT))
            .opacity(STATUS_LETTER_OPACITY)
            .text_color(status.color(theme))
            .child(SharedString::new_static(status.letter()))
    }
}

#[cfg(test)]
mod tests {
    use super::{
        ALL_GIT_STATUSES, AnchorId, BUTTON_BORDER, CONTENT_SIZED, FileTreeRow, GitStatus,
        ID_BUTTON, ID_ICON, ID_ITEM, ID_NAME, INDENT_SIZE, LINE_HEIGHT, LINE_SIZED, aggregate,
        button_border_color, guide_id, guide_left, leading_padding, name_color,
    };
    use crate::components::{ContentLength, RowState};
    use crate::theme::{Color, Theme};
    use gpui::px;

    /// **The indent step is the file explorer's own, not the sidebar tree's.**
    /// They differ by two pixels a level, and the two constants sit next to each
    /// other in this crate, so this is the assertion that keeps one from being
    /// swapped for the other.
    #[test]
    fn the_indent_step_is_sixteen_not_the_sidebar_trees_fourteen() {
        assert_eq!(INDENT_SIZE, px(16.0));
        assert_ne!(INDENT_SIZE, crate::components::INDENT_SIZE);

        assert_eq!(leading_padding(0), px(10.0));
        assert_eq!(leading_padding(1), px(26.0));
        assert_eq!(leading_padding(2), px(42.0));
    }

    /// `left: calc(10px + level × 16px + var(--file-tree-guide-icon-offset))`
    /// with `transform: translateX(-3px)` folded in.
    #[test]
    fn a_guide_sits_at_the_icons_centre_with_the_translate_folded_in() {
        let offset = Theme::DARK.file_tree_guide_icon_offset.value();

        assert_eq!(offset, px(7.0));
        assert_eq!(guide_left(0, offset), px(14.0));
        assert_eq!(guide_left(1, offset), px(30.0));
        assert_eq!(guide_left(2, offset), px(46.0));
    }

    #[test]
    fn guide_ids_are_zero_based_and_namespaced_to_this_surface() {
        assert_eq!(guide_id(0), "file-row-guide-0");
        assert_eq!(guide_id(3), "file-row-guide-3");
    }

    /// The one state the git row cannot reach. `focused` is the *only* thing
    /// that moves the border, and it moves it off `transparent` — which is a
    /// comparable delta because the border is 1px wide in every state and
    /// `ANCHORS.md` v1.3 compares `border.color` wherever `w > 0`.
    #[test]
    fn focus_is_the_only_state_that_moves_the_buttons_border() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            assert_eq!(
                button_border_color(&theme, RowState::resting()),
                Color::TRANSPARENT,
            );
            for state in [
                RowState {
                    hovered: true,
                    ..RowState::resting()
                },
                RowState {
                    selected: true,
                    ..RowState::resting()
                },
            ] {
                assert_eq!(button_border_color(&theme, state), Color::TRANSPARENT);
            }

            let focused = button_border_color(
                &theme,
                RowState {
                    focused: true,
                    ..RowState::resting()
                },
            );
            assert_eq!(focused, theme.accent.mix(42.0, theme.border));
            assert_ne!(focused, Color::TRANSPARENT);
        }
        assert_eq!(BUTTON_BORDER, px(1.0));
    }

    /// The three backgrounds are the wrapper's, and they are the sidebar tree's
    /// — the `::before` group is *unscoped*, so both surfaces get it. This
    /// asserts the reuse rather than restating the colours, because a second
    /// spelling of them is a second thing to drift.
    #[test]
    fn the_wrapper_background_is_the_shared_unscoped_rule() {
        let theme = Theme::DARK;

        assert_eq!(
            crate::components::item_background(&theme, RowState::resting()),
            None,
        );
        assert_eq!(
            crate::components::item_background(
                &theme,
                RowState {
                    selected: true,
                    ..RowState::resting()
                }
            ),
            Some(theme.accent),
        );
    }

    /// The fixture shares the git row's strings, so the same content length
    /// means the same name on both surfaces.
    #[test]
    fn the_fixture_shares_the_git_rows_names() {
        assert_eq!(FileTreeRow::fixture(ContentLength::Short).name, "a.ts");
        assert_eq!(
            FileTreeRow::fixture(ContentLength::Normal).name,
            "resolve-terminal-connection.ts",
        );
        assert_eq!(
            FileTreeRow::fixture(ContentLength::Overflow).name,
            "an-extremely-long-file-name-that-must-truncate-in-the-sidebar-row.ts",
        );
        assert_eq!(
            FileTreeRow::fixture(ContentLength::Normal).state,
            RowState::resting()
        );
        // And no git status, so every command line written before `--git-status`
        // existed renders the row it rendered.
        assert_eq!(FileTreeRow::fixture(ContentLength::Short).git_status, None);
    }

    /// v1.5 and v1.6, both on the name and on nothing else.
    ///
    /// The button is the trap here: it paints text — it is where the font is
    /// declared — but `h-6` authors its height at 24px around a 20px line box,
    /// so declaring it would invent a 4px delta on an anchor both engines agree
    /// on. The same reasoning that keeps the git row's badge off its list.
    #[test]
    fn only_the_name_is_declared_and_the_button_is_not() {
        assert_eq!(CONTENT_SIZED, [ID_NAME]);
        assert_eq!(LINE_SIZED, [ID_NAME]);

        for id in [ID_ITEM, ID_BUTTON, ID_ICON] {
            assert!(!CONTENT_SIZED.contains(&id), "{id}");
            assert!(!LINE_SIZED.contains(&id), "{id}");
        }
        assert!(!LINE_SIZED.contains(&"file-row-guide-0"));

        let declared = AnchorId::new(ID_NAME).content_sized().line_sized();
        assert!(declared.content_sized && declared.line_sized);
    }

    /// `text-sm`'s line height, spelled as the two numbers the stylesheet
    /// divides — 14px text on a 20px line box, where the git row is 14 on 18.9.
    #[test]
    fn the_line_height_is_text_sms_and_lands_on_twenty_pixels() {
        assert!((LINE_HEIGHT - 1.25 / 0.875).abs() < f32::EPSILON);
        assert!((LINE_HEIGHT * 14.0 - 20.0).abs() < 0.001, "{LINE_HEIGHT}");
        // And it is *not* the git row's authored `leading-[1.35]`.
        assert!((LINE_HEIGHT - 1.35).abs() > 0.05);
    }

    /// **The mapping: status in, token out**, in both tables.
    ///
    /// One arm per status rather than a loop, because the point of the test is
    /// the *pairing* — a loop over `ALL_GIT_STATUSES` calling `color` could only
    /// restate `color`'s own `match`.
    #[test]
    fn every_status_reads_the_token_its_class_names() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            assert_eq!(GitStatus::Modified.color(&theme), theme.git_modified);
            assert_eq!(
                GitStatus::ModifiedStaged.color(&theme),
                theme.git_modified_staged,
            );
            assert_eq!(GitStatus::Added.color(&theme), theme.git_added);
            assert_eq!(GitStatus::Deleted.color(&theme), theme.git_deleted);
            assert_eq!(GitStatus::Untracked.color(&theme), theme.git_untracked);
            assert_eq!(GitStatus::Renamed.color(&theme), theme.git_renamed);
        }
    }

    /// Six statuses, **five** colours — and the collision is real rather than a
    /// port mistake.
    ///
    /// `--git-modified-staged` and `--git-added` are both `var(--success)` in
    /// `theme.css`, in both tables. So a staged modification and an addition are
    /// indistinguishable on screen and distinguishable only by the letter, and a
    /// test asserting all six were distinct would be asserting something the
    /// design system does not say. Everything else is pairwise different, which
    /// is what makes `--git-status` a real axis.
    #[test]
    fn the_six_statuses_are_five_colours_and_the_pair_that_shares_is_named() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            assert_eq!(
                GitStatus::ModifiedStaged.color(&theme),
                GitStatus::Added.color(&theme),
            );

            let distinct = [
                GitStatus::Modified,
                GitStatus::Added,
                GitStatus::Deleted,
                GitStatus::Untracked,
                GitStatus::Renamed,
            ];
            for (index, one) in distinct.iter().enumerate() {
                for other in &distinct[index + 1..] {
                    assert_ne!(
                        one.color(&theme),
                        other.color(&theme),
                        "{} and {}",
                        one.name(),
                        other.name(),
                    );
                }
            }
        }
    }

    /// The two colours the oracle measured off the live reference, pinned to the
    /// tokens they are supposed to be.
    ///
    /// `#fe9a00` is Tailwind's `amber-500` reached through
    /// `--git-modified: var(--warning)`, and `#f94047` is the dark table's
    /// `--destructive: oklch(0.65 0.22 24)` reached through `--git-deleted`.
    /// Both were read off the running React app on a row this port got wrong, so
    /// they are evidence rather than arithmetic — which is exactly why they are
    /// written down here as well as derived there.
    #[test]
    fn the_measured_reference_colours_are_these_tokens() {
        assert_hex(GitStatus::Modified.color(&Theme::DARK), 0x00fe_9a00);
        assert_hex(GitStatus::Deleted.color(&Theme::DARK), 0x00f9_4047);
    }

    /// The same `assert_hex` the theme's own tests use: RGB to within half a
    /// channel step, because the tokens came through `OKLab`.
    #[track_caller]
    fn assert_hex(got: Color, want: u32) {
        let got = gpui::Rgba::from(got.value());
        let want: gpui::Rgba = gpui::rgb(want);
        let tolerance = 0.6 / 255.0;
        for (channel, g, w) in [
            ("r", got.r, want.r),
            ("g", got.g, want.g),
            ("b", got.b, want.b),
        ] {
            assert!(
                (g - w).abs() < tolerance,
                "{channel}: got {g}, want {w} ({got:?} vs {want:?})"
            );
        }
    }

    /// `gitStatusPriority` verbatim, `+ 1` and all.
    #[test]
    fn the_priorities_are_the_react_tables() {
        assert_eq!(GitStatus::Deleted.priority(), 50);
        assert_eq!(GitStatus::Modified.priority(), 40);
        assert_eq!(GitStatus::Renamed.priority(), 30);
        assert_eq!(GitStatus::Added.priority(), 20);
        assert_eq!(GitStatus::Untracked.priority(), 10);
        // `getGitStatusPriority`: `status === 'modified' && staged` is the only
        // thing that ever adds to the table, and it adds exactly one.
        assert_eq!(
            GitStatus::ModifiedStaged.priority(),
            GitStatus::Modified.priority() + 1,
        );
        // Which still leaves a deletion the loudest thing in a folder.
        assert!(GitStatus::ModifiedStaged.priority() < GitStatus::Deleted.priority());

        // And `ALL_GIT_STATUSES` is in decreasing priority, so reading it top to
        // bottom is reading the folder ordering.
        for pair in ALL_GIT_STATUSES.windows(2) {
            assert!(pair[0].priority() > pair[1].priority(), "{pair:?}");
        }
    }

    /// Every priority is distinct — which is what makes the tie-break in
    /// [`aggregate`] unobservable *between statuses*, and therefore why the
    /// assertion below is about the ordering rather than about the tie.
    ///
    /// The strict `>` is kept regardless, because the React line is strict and
    /// this is a port of it: a folder holding two files of the same status keeps
    /// the first, and a later change that gave two statuses one priority would
    /// inherit the reference's answer instead of inventing one.
    #[test]
    fn no_two_statuses_share_a_priority() {
        let mut priorities: Vec<u8> = ALL_GIT_STATUSES
            .iter()
            .map(|status| status.priority())
            .collect();
        priorities.sort_unstable();
        let before = priorities.len();
        priorities.dedup();
        assert_eq!(priorities.len(), before, "{priorities:?}");
    }

    /// **How a folder is coloured**: the highest priority beneath it wins, and
    /// nothing is summed or averaged.
    #[test]
    fn a_folder_takes_the_loudest_status_beneath_it() {
        assert_eq!(aggregate([]), None);
        assert_eq!(aggregate([GitStatus::Added]), Some(GitStatus::Added));

        // The measured case: `src` held one deleted file and painted
        // `#f94047ff`, whatever else was under it.
        assert_eq!(
            aggregate([
                GitStatus::Added,
                GitStatus::Untracked,
                GitStatus::Deleted,
                GitStatus::Modified,
            ]),
            Some(GitStatus::Deleted),
        );

        // The order of the children does not change the answer.
        assert_eq!(
            aggregate([GitStatus::Modified, GitStatus::ModifiedStaged]),
            Some(GitStatus::ModifiedStaged),
        );
        assert_eq!(
            aggregate([GitStatus::ModifiedStaged, GitStatus::Modified]),
            Some(GitStatus::ModifiedStaged),
        );

        // And the whole ordering, one pair at a time.
        for pair in ALL_GIT_STATUSES.windows(2) {
            assert_eq!(aggregate([pair[0], pair[1]]), Some(pair[0]));
            assert_eq!(aggregate([pair[1], pair[0]]), Some(pair[0]));
        }
    }

    /// No status is the row this port was already painting: the inherited
    /// `text-foreground`, which is the `#f5f5f5ff` the oracle read. Every status
    /// moves it.
    #[test]
    fn no_status_leaves_the_name_on_the_inherited_foreground() {
        for theme in [Theme::LIGHT, Theme::DARK] {
            assert_eq!(name_color(&theme, None), theme.foreground);
            for status in ALL_GIT_STATUSES {
                assert_eq!(name_color(&theme, Some(status)), status.color(&theme));
                assert_ne!(
                    name_color(&theme, Some(status)),
                    theme.foreground,
                    "{}",
                    status.name(),
                );
            }
        }
    }

    /// The letters and the vocabulary, both from the React source.
    ///
    /// Both `modified` arms are `M`, which is the reason the letter cannot stand
    /// in for the status: a staged and an unstaged modification paint the same
    /// character in two different colours.
    #[test]
    fn the_letters_and_the_names_are_the_react_sources() {
        assert_eq!(GitStatus::Modified.letter(), "M");
        assert_eq!(GitStatus::ModifiedStaged.letter(), "M");
        assert_eq!(GitStatus::Added.letter(), "A");
        assert_eq!(GitStatus::Deleted.letter(), "D");
        assert_eq!(GitStatus::Untracked.letter(), "U");
        assert_eq!(GitStatus::Renamed.letter(), "R");

        assert_eq!(GitStatus::ModifiedStaged.name(), "modified-staged");
        let mut names: Vec<&str> = ALL_GIT_STATUSES
            .iter()
            .map(|status| status.name())
            .collect();
        assert!(
            names.iter().all(|name| {
                !name.is_empty()
                    && *name != "none"
                    && name.chars().all(|c| c.is_ascii_lowercase() || c == '-')
            }),
            "{names:?}",
        );
        names.sort_unstable();
        let before = names.len();
        names.dedup();
        assert_eq!(names.len(), before, "a status name is spelled twice");
    }
}
