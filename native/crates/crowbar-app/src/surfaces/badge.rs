//! `--surface badge` — one `<Badge>` in one cell of the §8.3 matrix.
//!
//! The default cell is the only Badge in the live app that carries the
//! primitive's own anchor: the `agent` pill in `review-thread-item.tsx`,
//! `variant="outline"` at the default size with the call site's
//! `h-4 border-primary/30 px-1 text-xs text-primary`, measured at
//! `44.34 × 18` on a 1714px viewport.
//!
//! # Finding the reference took most of this item, and the answer is a warning
//!
//! There are five `<Badge` call sites in `web/src/` and **four of them cannot
//! produce a snapshot of this surface**:
//!
//! | call site | why not |
//! |---|---|
//! | `git-status-file-item.tsx` (×6 live) | overrides the id — those elements are `git-row-badge`, and belong to `--surface git-status-row` |
//! | `diff-review-header.tsx` | `DiffReviewHeader` is **dead code**: nothing outside its own file and unit test renders it |
//! | `review-thread-item.tsx`, two `Outdated` badges | gated on the `isOutdated` prop, which `use-review-annotations.tsx` — the component's only call site — never passes |
//! | `review-thread-item.tsx`, the `agent` badge | **this one**, on a review message whose `isAgent` is set |
//!
//! The lesson is the one P2.1 set and this item nearly fell over: **a primitive's
//! per-slot default id is not the same thing as a reference.** Six live Badges
//! is six live *elements*; whether any of them answers to `badge` is a separate
//! question, and here it came down to one branch behind a boolean on a stored
//! message.
//!
//! # What each axis can and cannot do here
//!
//! | Axis | Here |
//! |---|---|
//! | `--viewport-width` | **the strongest axis this surface has.** All three sizes carry `sm:h-*`, `sm:min-w-*` and `sm:text-*`, and on a merged call site the breakpoint decides *whose* height wins — see [`crowbar_ui::components::badge::CallSite::height`] |
//! | `--content` | **real**, and the only axis that moves `bounds.w`: the badge is content-sized, so the label's advance is the box |
//! | `--width` | **vacuous.** A badge authors no width and takes none from its parent; `--width` is room to be laid out in and nothing else |
//! | `--theme` | **real** on every variant — `bg`, `fg` and `border.color` all move, and on `outline` the background swaps token entirely (`bg-background` → `dark:bg-input/32`) |
//!
//! # The state axis, and which flag reaches what
//!
//! | flag | here |
//! |---|---|
//! | `empty` | **real and visible.** No label, so the box falls to `min-w-*` — which is the same `--spacing` step as the height, so an empty badge is a circle. No live call site renders one |
//! | `hover` | a real rule on four variants (`[button&,a&]:hover:bg-*`) that **cannot fire on a `<span>`**. It needs `--interactive`, which is `badge.tsx`'s `render` prop, which no live call site passes. And synthetic pointer events are denied on this machine either way |
//! | `focus` | a real state with **no field**: `focus-visible:ring-2 ring-offset-1` is two box-shadows and `ANCHORS.md` §6 has neither. A `<span>` is not focusable without a `tabindex` nothing writes, so it is doubly unreachable |
//! | `selected` | **unmodelled.** `badgeVariants` has no selected, active or pressed rule of any kind |
//! | `loading`, `error` | unmodelled, as on every surface |
//!
//! `--disabled` is an option rather than a flag, for `button`'s reason: it is a
//! prop, its rules are `pointer-events-none` (not a visual property) and
//! `opacity-64` (no field, and non-zero so v1.7's `visible` term does not fire),
//! and `&:disabled` cannot match a `<span>` at all.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::badge::{
    ALL_CALL_SITES, ALL_SIZES, ALL_VARIANTS, Badge, BadgeState, CallSite, Label, Size, Variant,
};
use crowbar_ui::components::{AnchorSink, ContentLength, badge};
use gpui::{AnyElement, IntoElement as _, ParentElement as _, Styled as _, div};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "badge",
    root: badge::ID_BADGE,
    unmodelled: &[
        // `badgeVariants` has no selected/active/pressed rule at all.
        StateFlag::Selected,
        StateFlag::Loading,
        StateFlag::Error,
    ],
    // The tallest badge is `lg` below the breakpoint at 26px, plus
    // `CAPTION_HEIGHT`'s 29. 60 holds that with room, and is a floor rather than
    // a ceiling: this surface drives no height (`driven_height`), because a
    // badge's is decided by its own size variant and the viewport, not by
    // anything on the command line the window would have to follow.
    min_window_height: 60,
    // A badge sits inside a row, a header or a comment — never the window
    // itself. So it keeps `INSET_X` and the root-relative arithmetic stays
    // non-trivial on both axes.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--variant`.
    pub variant: Variant,
    /// `--size`.
    pub size: Size,
    /// `--call-site`: the `className` bundle a call site merges over the
    /// variant's own. See [`CallSite`].
    pub call_site: CallSite,
    /// `--glyph`: render a leading `<svg>` as an empty box.
    ///
    /// Off by default, because no live call site passes a child other than
    /// text.
    pub glyph: bool,
    /// `--interactive`: `badge.tsx`'s `render` prop made this a `<button>` or an
    /// `<a>`, which is what every `[button&,a&]:` rule selects.
    ///
    /// Off by default and **off in every live rendering**: `useRender`'s
    /// `defaultTagName` is `'span'` and no call site overrides it.
    pub interactive: bool,
    /// `--disabled`: the `disabled` attribute. See the module docs for why it
    /// is here rather than on a flag, and why it cannot paint anything the
    /// contract can see.
    pub disabled: bool,
}

impl Default for Params {
    fn default() -> Self {
        let fixture = Badge::fixture();
        Self {
            variant: fixture.variant,
            size: fixture.size,
            call_site: fixture.call_site,
            glyph: fixture.glyph,
            interactive: fixture.interactive,
            disabled: false,
        }
    }
}

impl Params {
    /// The badge this cell describes.
    ///
    /// Built by taking the live fixture and applying the cell — rather than by
    /// assembling one from scratch — so that a bare `--surface badge` renders
    /// the badge the reference actually has.
    #[must_use]
    pub fn badge(&self, cell: &Cell) -> Badge {
        let mut badge = Badge::fixture();
        badge.variant = self.variant;
        badge.size = self.size;
        badge.breakpoint = cell.breakpoint();
        badge.call_site = self.call_site;
        badge.glyph = self.glyph;
        badge.interactive = self.interactive;
        // §8.3's `empty` is a badge with no children to measure.
        badge.label = (!cell.has(StateFlag::Empty)).then(|| label_of(cell.content));
        badge.state = BadgeState {
            hovered: cell.has(StateFlag::Hover),
            focused: cell.has(StateFlag::Focus),
            disabled: self.disabled,
        };
        badge
    }
}

/// The label a content length shows.
///
/// A translation rather than a shared type, for `button`'s reason:
/// `ContentLength` is the *cell's* vocabulary and [`Label`] is the component's,
/// and the two carry different strings on purpose — the git row's fixture is a
/// path and a badge's label is a word.
fn label_of(content: ContentLength) -> Label {
    match content {
        ContentLength::Short => Label::Short,
        ContentLength::Normal => Label::Normal,
        ContentLength::Overflow => Label::Overflow,
    }
}

impl SurfaceParams for Params {
    fn accept(
        &mut self,
        option: &str,
        args: &mut dyn Iterator<Item = String>,
    ) -> Result<bool, ParseError> {
        match option {
            "--variant" => self.variant = parse_variant(&value(args, option)?)?,
            "--size" => self.size = parse_size(&value(args, option)?)?,
            "--call-site" => self.call_site = parse_call_site(&value(args, option)?)?,
            "--glyph" => self.glyph = true,
            "--interactive" => self.interactive = true,
            "--disabled" => self.disabled = true,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** A badge's height is its size variant's and its viewport's, and
    /// no option here sets one — so there is nothing for the window to follow
    /// and [`Surface::min_window_height`] is the whole answer.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    /// This surface's own half of the caption — and, where it applies, the fact
    /// that a cell **cannot fail** or **has no reference**.
    ///
    /// The two are different complaints and are worded differently. "Cannot
    /// fail" means the contract has no field for what moved; "no reference"
    /// means the picture is drawable here and not on the other side.
    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(
            out,
            " · {} · {} · class {}",
            self.variant.name(),
            self.size.name(),
            self.call_site.name(),
        );
        if self.glyph {
            out.push_str(" · glyph");
        }
        if self.interactive {
            out.push_str(" · render=button");
        }
        if self.disabled {
            out.push_str(" · disabled");
        }

        // The default cell reproduces a real call site, and says which. A
        // caption that only printed the classes would leave a reader unable to
        // check the claim against the app.
        if self.call_site == CallSite::Agent
            && self.variant == Variant::Outline
            && self.size == Size::Default
        {
            out.push_str(
                " (the review thread's agent badge, review-thread-item.tsx; the only live \
                 Badge that keeps the primitive's own data-oracle-id)",
            );
        }
        if !self.call_site.live() {
            out.push_str(
                " · class: isOutdated is never passed by use-review-annotations.tsx, so \
                 there is no reference",
            );
        }
        if !self.variant.live() {
            out.push_str(" · variant: no live call site uses it, so there is no reference");
        }
        if self.variant == Variant::Secondary {
            out.push_str(
                " · secondary: its only call site is DiffReviewHeader, which is dead code, \
                 so there is no reference",
            );
        }
        if !self.size.live() {
            out.push_str(" · size: no live call site uses it, so there is no reference");
        }
        if self.glyph {
            out.push_str(
                " · glyph: no live call site passes a non-text child, so there is no \
                 reference",
            );
        }
        if self.interactive {
            out.push_str(
                " · render: no live call site passes it, so every live Badge is a <span> and \
                 there is no reference",
            );
        }
        if self.disabled {
            out.push_str(
                " · disabled: opacity-64 has no field and does not reach v1.7's zero, and \
                 &:disabled cannot match a <span> — so this cell cannot fail",
            );
        }

        if cell.has(StateFlag::Empty) {
            out.push_str(
                " · empty: no label, so the box falls to min-w-*; no live call site renders \
                 one, so there is no reference",
            );
        } else {
            let _ = write!(out, " · label \"{}\"", label_of(cell.content).text());
        }
        if cell.has(StateFlag::Hover) {
            if self.interactive {
                out.push_str(
                    " · hover: it does move bg, and synthetic pointer events are denied on \
                     this machine, so there is no reference",
                );
            } else {
                out.push_str(
                    " · hover: every [button&,a&]: rule needs --interactive, so on this \
                     cell's <span> it moves nothing and cannot fail",
                );
            }
        }
        if cell.has(StateFlag::Focus) {
            out.push_str(
                " · focus: the ring is two box-shadows, which ANCHORS.md §6 has no field \
                 for, so this cell cannot fail",
            );
        }
    }

    /// The badge, inside the flex row that makes it a flex item.
    ///
    /// `RowSurface` draws every surface inside `div().w(--width)`, which is a
    /// gpui **block** container; a badge drawn straight into one would be a
    /// block-level flex box and would fill the width, where every live Badge is
    /// a flex item whose used width is its own max-content width. The row is
    /// also what makes the two `display`s agree without gpui having an
    /// `inline-flex`: CSS blockifies a flex item's display, so the reference's
    /// `inline-flex` computes to **`flex`** — measured live.
    ///
    /// It is **above** the root anchor and therefore outside the snapshot
    /// (`ANCHORS.md` §4), exactly as `button`'s row is.
    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        div()
            .flex()
            .flex_row()
            .items_center()
            .child(self.badge(cell).render(theme, anchors))
            .into_any_element()
    }
}

/// One of `cva`'s eight variant keys.
///
/// The vocabulary is generated from [`ALL_VARIANTS`] rather than restated, so
/// the word the command line takes and the word a caption prints cannot drift
/// into two spellings of one variant.
fn parse_variant(raw: &str) -> Result<Variant, ParseError> {
    ALL_VARIANTS
        .into_iter()
        .find(|variant| variant.name() == raw)
        .ok_or_else(|| {
            ParseError::Rejected(format!(
                "--variant takes one of {}, not {raw}",
                names(ALL_VARIANTS.into_iter().map(Variant::name)),
            ))
        })
}

/// One of `cva`'s three size keys. Generated, for [`parse_variant`]'s reason.
fn parse_size(raw: &str) -> Result<Size, ParseError> {
    ALL_SIZES
        .into_iter()
        .find(|size| size.name() == raw)
        .ok_or_else(|| {
            ParseError::Rejected(format!(
                "--size takes one of {}, not {raw}",
                names(ALL_SIZES.into_iter().map(Size::name)),
            ))
        })
}

/// A call site's `className` bundle.
///
/// **There is deliberately no numeric form.** A `--padding 4` would let a cell
/// be tuned to whatever the reference happened to report, and the anchor would
/// stop being able to fail. What a caller may say is which bundle a call site
/// wrote; both engines still resolve it from the same tokens. The line P3.1 drew
/// for `--class-radius`.
fn parse_call_site(raw: &str) -> Result<CallSite, ParseError> {
    ALL_CALL_SITES
        .into_iter()
        .find(|site| site.name() == raw)
        .ok_or_else(|| {
            ParseError::Rejected(format!(
                "--call-site takes one of {}, not {raw}; it names the className bundle a \
                 call site merges, never a pixel value",
                names(ALL_CALL_SITES.into_iter().map(CallSite::name)),
            ))
        })
}

/// A vocabulary as one line, for a usage line and for a rejection.
fn names<I: Iterator<Item = &'static str>>(words: I) -> String {
    words.collect::<Vec<_>>().join(", ")
}

fn options() -> Vec<(String, String)> {
    let dead_variants = names(
        ALL_VARIANTS
            .into_iter()
            .filter(|variant| !variant.live())
            .map(Variant::name),
    );
    [
        (
            "--variant <name>".to_owned(),
            format!(
                "one of {}; {dead_variants} have no live call site [{}]",
                names(ALL_VARIANTS.into_iter().map(Variant::name)),
                Badge::fixture().variant.name(),
            ),
        ),
        (
            "--size <name>".to_owned(),
            format!(
                "one of {}; lg has no live call site [{}]",
                names(ALL_SIZES.into_iter().map(Size::name)),
                Badge::fixture().size.name(),
            ),
        ),
        (
            "--call-site <name>".to_owned(),
            format!(
                "one of {} — the className bundle a call site merges, never a pixel \
                 value; outdated has no live rendering [{}]",
                names(ALL_CALL_SITES.into_iter().map(CallSite::name)),
                Badge::fixture().call_site.name(),
            ),
        ),
        (
            "--glyph".to_owned(),
            "render a leading svg as an empty box; no live call site passes one [off]".to_owned(),
        ),
        (
            "--interactive".to_owned(),
            "render=button|a, which is what every [button&,a&]: rule needs; no live \
             call site passes it [off]"
                .to_owned(),
        ),
        (
            "--disabled".to_owned(),
            "the disabled attribute; dead on a <span> and invisible to the contract [off]"
                .to_owned(),
        ),
    ]
    .into_iter()
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{Params, SURFACE};
    use crate::row_surface::{Cell, ParseError, StateFlag};
    use crowbar_ui::components::badge::{
        ALL_CALL_SITES, ALL_SIZES, ALL_VARIANTS, CallSite, Size, Variant,
    };
    use crowbar_ui::components::{Breakpoint, badge};

    fn a_cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "badge"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("a well-formed cell")
    }

    fn params(cell: &Cell) -> Params {
        cell.surface_params::<Params>()
            .expect("badge's own bag")
            .clone()
    }

    /// The surface's root is the primitive's per-slot default, which is what
    /// the reference's `root` field says. A capture of `git-row-badge` is a
    /// different surface's, however similar the element is.
    #[test]
    fn the_root_anchor_is_the_primitives_own_id() {
        assert_eq!(SURFACE.root, "badge");
        assert_eq!(badge::ID_BADGE, "badge");
        assert_ne!(SURFACE.root, crowbar_ui::components::ID_BADGE);
    }

    /// A bare `--surface badge` is the live `agent` badge.
    #[test]
    fn the_default_cell_is_the_live_agent_badge() {
        let cell = a_cell(&[]);
        let badge = params(&cell).badge(&cell);

        assert_eq!(badge.variant, Variant::Outline);
        assert_eq!(badge.size, Size::Default);
        assert_eq!(badge.call_site, CallSite::Agent);
        assert_eq!(badge.breakpoint, Breakpoint::Sm);
        assert!(!badge.interactive);
        assert!(badge.label.is_some());
        assert_eq!(badge.id, "badge");
    }

    /// Every word of every vocabulary parses, and nothing outside them does.
    #[test]
    fn the_vocabularies_are_closed() {
        for variant in ALL_VARIANTS {
            let cell = a_cell(&["--variant", variant.name()]);
            assert_eq!(params(&cell).badge(&cell).variant, variant);
        }
        for size in ALL_SIZES {
            let cell = a_cell(&["--size", size.name()]);
            assert_eq!(params(&cell).badge(&cell).size, size);
        }
        for site in ALL_CALL_SITES {
            let cell = a_cell(&["--call-site", site.name()]);
            assert_eq!(params(&cell).badge(&cell).call_site, site);
        }

        for line in [
            vec!["--surface", "badge", "--variant", "ghost"],
            vec!["--surface", "badge", "--size", "xs"],
            vec!["--surface", "badge", "--call-site", "review"],
            vec!["--surface", "badge", "--call-site", "4"],
            vec!["--surface", "badge", "--variant"],
        ] {
            assert!(
                matches!(
                    Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{line:?} should have been rejected",
            );
        }
    }

    /// **No numeric form.** A `--padding 4` or a `--height 18` would hand the
    /// port the reference's own answer and the anchor would stop being able to
    /// fail. The rejection says so rather than only listing the words.
    #[test]
    fn a_call_site_can_be_named_but_never_measured() {
        let Err(ParseError::Rejected(complaint)) = Cell::parse(
            ["--surface", "badge", "--call-site", "4"]
                .iter()
                .map(|arg| (*arg).to_owned()),
        ) else {
            panic!("4 is not a className bundle");
        };
        assert!(complaint.contains("never a pixel value"), "{complaint}");

        for option in ["--padding", "--height", "--radius"] {
            assert!(
                matches!(
                    Cell::parse(
                        ["--surface", "badge", option, "4"]
                            .iter()
                            .map(|arg| (*arg).to_owned())
                    ),
                    Err(ParseError::Rejected(_)),
                ),
                "{option} must not exist",
            );
        }
    }

    /// `empty` is the one flag with a real, visible original: no label, so the
    /// box falls to `min-w-*`.
    #[test]
    fn the_empty_flag_drops_the_label() {
        let cell = a_cell(&["--flags", "empty"]);
        assert!(params(&cell).badge(&cell).label.is_none());

        let cell = a_cell(&[]);
        assert!(params(&cell).badge(&cell).label.is_some());
    }

    /// The two flags that reach the component, and the three that do not.
    #[test]
    fn the_interaction_flags_reach_the_state_and_selected_is_unmodelled() {
        let cell = a_cell(&["--flags", "hover,focus"]);
        let badge = params(&cell).badge(&cell);
        assert!(badge.state.hovered);
        assert!(badge.state.focused);
        assert!(!badge.state.disabled);

        assert!(SURFACE.unmodelled(StateFlag::Selected));
        assert!(SURFACE.unmodelled(StateFlag::Loading));
        assert!(SURFACE.unmodelled(StateFlag::Error));
        for flag in [StateFlag::Empty, StateFlag::Hover, StateFlag::Focus] {
            assert!(!SURFACE.unmodelled(flag), "{}", flag.name());
        }
    }

    /// The caption names the live call site the default cell reproduces, and
    /// says per cell where a picture has no reference on the other side.
    #[test]
    fn the_caption_says_which_cells_have_no_reference() {
        assert!(a_cell(&[]).describe().contains("review-thread-item.tsx"));

        let outdated = a_cell(&["--call-site", "outdated"]);
        assert!(
            outdated.describe().contains("isOutdated"),
            "{}",
            outdated.describe()
        );

        let secondary = a_cell(&["--variant", "secondary"]);
        assert!(
            secondary.describe().contains("dead code"),
            "{}",
            secondary.describe()
        );

        let lg = a_cell(&["--size", "lg"]);
        assert!(
            lg.describe().contains("no live call site"),
            "{}",
            lg.describe()
        );

        // A hover cell on a span cannot fail; on a button it has no reference.
        let span = a_cell(&["--flags", "hover"]);
        assert!(
            span.describe().contains("cannot fail"),
            "{}",
            span.describe()
        );
        let button = a_cell(&["--flags", "hover", "--interactive"]);
        assert!(
            button.describe().contains("no reference"),
            "{}",
            button.describe(),
        );
    }

    /// The viewport axis reaches the component, which is where the call-site
    /// height trap lives.
    #[test]
    fn the_viewport_selects_the_breakpoint() {
        let wide = a_cell(&["--viewport-width", "800"]);
        assert_eq!(params(&wide).badge(&wide).breakpoint, Breakpoint::Sm);
        assert_eq!(params(&wide).badge(&wide).height(), gpui::px(18.0));

        let narrow = a_cell(&["--viewport-width", "600"]);
        assert_eq!(params(&narrow).badge(&narrow).breakpoint, Breakpoint::Base);
        assert_eq!(params(&narrow).badge(&narrow).height(), gpui::px(16.0));
    }
}
