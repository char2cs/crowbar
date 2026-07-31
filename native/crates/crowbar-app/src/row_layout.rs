//! What the row's layout actually resolves to, measured in a real window.
//!
//! Everything else about the row can be reasoned about from the source. This
//! cannot: the whole point of the Phase 1 gate is that **taffy and Blink might
//! disagree**, and the only honest way to know what taffy does with three
//! nested flex containers and a `max-w-[45%]` is to lay it out and read the
//! bounds back. So these assertions are written against the numbers gpui
//! produced, and the CSS arithmetic they are compared to is spelled out in each
//! one — where the two agree it is stated as an equality, and where they cannot
//! it is stated as a bound with the reason.
//!
//! Reading the bounds back is `crowbar-driver`'s extractor, taken as a
//! dev-dependency so this runs under a plain `cargo test --workspace`.

#![cfg(test)]

use crowbar_driver::{AnchorRegistry, RawAnchor};
use crowbar_ui::Theme;
use crowbar_ui::components::{
    BASE_INDENT, GUIDE_END_INSET, GUIDE_WIDTH, ICON_SIZE, INDENT_SIZE, ROW_HEIGHT, guide_id,
    leading_padding,
};
use gpui::{
    Context, IntoElement, ParentElement as _, Pixels, Render, Styled as _, TestAppContext, Window,
    div, px, size,
};

use crate::driver_anchors::{DriverAnchors, fold_text_halves};
use crate::row_surface::{Cell, render_row};

struct Stage(Cell);

impl Render for Stage {
    fn render(&mut self, _window: &mut Window, _cx: &mut Context<Self>) -> impl IntoElement {
        let theme = self.0.theme();
        // Offset and width-pinned exactly as `RowSurface` draws it, and through
        // the same `render_row` dispatch, so the numbers here are the numbers
        // the oracle will read off the binary.
        div()
            .pl(px(crate::row_surface::INSET_X))
            .pt(px(crate::row_surface::INSET_Y))
            .font_family(theme.font_sans.primary().unwrap_or("sans-serif"))
            .child(
                div()
                    .w(self.0.width_px())
                    .child(render_row(&self.0, &theme, &DriverAnchors)),
            )
    }
}

/// Lays the cell out in a real window and hands back what the extractor
/// recorded — folded, so what these assertions see is what a snapshot would
/// carry.
///
/// The window is the cell's **viewport**, exactly as `RowSurface::window_size`
/// makes it in the app. Sizing it to some fixed 1200px instead would let a
/// surface wider than the window pass unnoticed here and clip in the real
/// binary, and it would make the harness and the thing it measures two
/// different configurations.
fn measure(cx: &mut TestAppContext, cell: Cell) -> Vec<RawAnchor> {
    let anchors: AnchorRegistry = cx.update(crowbar_driver::install);
    let window_width = cell.viewport_width_px();
    let _window = cx.open_window(size(window_width, px(400.0)), |_, _| Stage(cell));
    cx.run_until_parked();
    fold_text_halves(anchors.records())
}

fn find(records: &[RawAnchor], id: &str) -> RawAnchor {
    records
        .iter()
        .find(|record| record.id == id)
        .unwrap_or_else(|| panic!("{id} was not recorded; got {:?}", ids(records)))
        .clone()
}

fn ids(records: &[RawAnchor]) -> Vec<String> {
    records.iter().map(|r| r.id.to_string()).collect()
}

/// Bounds relative to a named root anchor, which is the space the contract
/// compares in (`native/oracle/ANCHORS.md` §4).
fn relative_to(records: &[RawAnchor], root: &str, id: &str) -> gpui::Bounds<Pixels> {
    let origin = find(records, root).bounds.origin;
    let mut bounds = find(records, id).bounds;
    bounds.origin -= origin;
    bounds
}

/// [`relative_to`] against the git status row's root.
fn relative(records: &[RawAnchor], id: &str) -> gpui::Bounds<Pixels> {
    relative_to(records, crowbar_ui::components::ID_ITEM, id)
}

#[track_caller]
fn assert_px(actual: Pixels, expected: Pixels) {
    assert!(
        (actual - expected).abs() < px(0.01),
        "expected {expected:?}, got {actual:?}",
    );
}

/// `native/oracle/ANCHORS.md` §5's bounds tolerance.
///
/// Used where gpui's layout is **rounded to whole pixels** and the CSS value is
/// not — see `a_percentage_length_lands_on_a_whole_pixel`. Everywhere else the
/// assertion above is exact, so this spelling marks the places where the two
/// engines genuinely cannot agree to the last decimal.
#[track_caller]
fn assert_within_tolerance(actual: Pixels, expected: Pixels) {
    assert!(
        (actual - expected).abs() <= px(0.5),
        "expected {expected:?} ± 0.5, got {actual:?}",
    );
}

fn a_cell(args: &[&str]) -> Cell {
    Cell::parse(args.iter().map(|arg| (*arg).to_owned())).expect("a well-formed cell")
}

/// All nine contract anchors, at the default cell. Anchor presence is what the
/// differ ranks first, so a missing one is the loudest possible failure.
#[gpui::test]
fn the_default_cell_carries_every_contract_anchor(cx: &mut TestAppContext) {
    let records = measure(cx, Cell::default());
    let seen = ids(&records);

    for id in [
        "git-row-item",
        "git-row-button",
        "git-row-icon",
        "git-row-name",
        "git-row-dir",
        "git-row-badge",
        "git-row-added",
        "git-row-deleted",
        "git-row-guide-0",
        "git-row-guide-1",
    ] {
        assert!(
            seen.contains(&id.to_owned()),
            "{id} is missing from {seen:?}"
        );
    }
    assert!(
        !seen.contains(&"git-row-guide-2".to_owned()),
        "depth 2 has two guides, not three",
    );
}

/// The root is at the origin by construction, and the wrapper and the button
/// are the same box: `.file-tree-item` is `w-full` with no padding or border,
/// and the button is `w-full` inside it.
#[gpui::test]
fn the_wrapper_and_the_button_are_one_full_width_box(cx: &mut TestAppContext) {
    let cell = Cell::default();
    let records = measure(cx, cell.clone());

    let item = relative(&records, "git-row-item");
    let button = relative(&records, "git-row-button");

    assert_px(item.origin.x, px(0.0));
    assert_px(item.origin.y, px(0.0));
    assert_px(item.size.width, cell.width_px());
    assert_px(item.size.height, ROW_HEIGHT);
    assert_px(button.origin.x, px(0.0));
    assert_px(button.size.width, cell.width_px());
    assert_px(button.size.height, ROW_HEIGHT);
}

/// `radius` and `border.w` — the two the brief got wrong and the extractor
/// then measured. `rounded-md` is 8px here because this project redefines
/// `--radius-md`, and the button's border-width is zero because `border-none`
/// and `border` are different tailwind-merge groups and a `none`-styled border
/// computes to zero width.
#[gpui::test]
fn the_button_is_eight_px_round_with_no_border(cx: &mut TestAppContext) {
    let records = measure(cx, Cell::default());
    let button = find(&records, "git-row-button");
    let item = find(&records, "git-row-item");

    assert_px(button.radius, Theme::DARK.radius_md.value());
    assert_px(button.radius, px(8.0));
    assert_px(button.border_width, px(0.0));
    // The wrapper is the layer that rounds the *painted* background, and it
    // rounds by the file-tree stylesheet's own 2px.
    assert_px(item.radius, px(2.0));
}

/// The icon sits at exactly the leading padding, which is the indent
/// arithmetic showing up in a real layout rather than in a unit test.
#[gpui::test]
fn the_icon_starts_at_the_leading_padding(cx: &mut TestAppContext) {
    for depth in [0_u16, 1, 4] {
        let records = measure(cx, a_cell(&["--depth", &depth.to_string()]));
        let icon = relative(&records, "git-row-icon");

        assert_px(icon.origin.x, leading_padding(depth));
        assert_px(icon.size.width, ICON_SIZE);
        assert_px(icon.size.height, ICON_SIZE);
        // `items-center` in a 24px row: (24 - 14) / 2.
        assert_px(icon.origin.y, (ROW_HEIGHT - ICON_SIZE) / 2.0);
    }
}

/// One guide per level, at `10 + 14n + 7 - 3`, 7px wide.
///
/// Full height, not capped: the neighbour depths default to this row's own
/// depth, and every level a row draws is *shallower* than that, so
/// `previousDepth <= level` is false at all of them. A lone row's guides run
/// edge to edge.
#[gpui::test]
fn the_guides_sit_where_the_stylesheet_puts_them(cx: &mut TestAppContext) {
    let records = measure(cx, a_cell(&["--depth", "3"]));

    for level in 0..3_u16 {
        let guide = relative(&records, &guide_id(level));
        let offset = Theme::DARK.file_tree_guide_icon_offset.value();

        assert_px(
            guide.origin.x,
            BASE_INDENT + INDENT_SIZE * f32::from(level) + offset - px(3.0),
        );
        assert_px(guide.size.width, GUIDE_WIDTH);
        assert_px(guide.origin.y, px(0.0));
        assert_px(guide.size.height, ROW_HEIGHT);
    }
}

/// The cap, driven the only way it can be: a shallower row above and below.
#[gpui::test]
fn a_guide_that_begins_and_ends_at_this_row_is_capped(cx: &mut TestAppContext) {
    let records = measure(
        cx,
        a_cell(&["--depth", "2", "--prev-depth", "0", "--next-depth", "0"]),
    );

    for level in 0..2_u16 {
        let guide = relative(&records, &guide_id(level));
        assert_px(guide.origin.y, GUIDE_END_INSET);
        assert_px(guide.size.height, ROW_HEIGHT - GUIDE_END_INSET * 2.0);
    }
}

/// A guide that runs through the row is not capped — the other half of the
/// inset rule, and the half a single-row surface would never exercise.
#[gpui::test]
fn a_guide_running_through_the_row_spans_it(cx: &mut TestAppContext) {
    let records = measure(
        cx,
        a_cell(&["--depth", "1", "--prev-depth", "3", "--next-depth", "3"]),
    );
    let guide = relative(&records, &guide_id(0));

    assert_px(guide.origin.y, px(0.0));
    assert_px(guide.size.height, ROW_HEIGHT);
}

/// **The gate's own question.** With a directory column the filename is
/// `shrink-0 basis-auto max-w-[45%]` and the directory is `flex-1`, so the two
/// share the container: the filename takes its own width up to the 45% cap and
/// the directory takes the rest, with the `gap-1.5` between them.
#[gpui::test]
fn the_name_and_the_directory_share_the_container(cx: &mut TestAppContext) {
    let records = measure(cx, a_cell(&["--content", "overflow", "--width", "320"]));

    let icon = relative(&records, "git-row-icon");
    let name = relative(&records, "git-row-name");
    let dir = relative(&records, "git-row-dir");
    let badge = relative(&records, "git-row-badge");

    // The container runs from the icon's trailing gap to the trailing group's
    // leading gap; both gaps are `gap-1.5` on the button.
    let container_left = icon.origin.x + icon.size.width + px(6.0);
    let container_right = badge.origin.x - px(6.0);
    let container_width = container_right - container_left;

    assert_px(name.origin.x, container_left);
    // The overflow fixture is far wider than 45%, so the cap is what binds —
    // to within the whole-pixel rounding gpui applies to every laid-out edge.
    assert_within_tolerance(name.size.width, container_width * 0.45);
    // `gap-1.5` between the two spans, and the directory takes everything left.
    assert_px(dir.origin.x, name.origin.x + name.size.width + px(6.0));
    assert_px(dir.origin.x + dir.size.width, container_right);
}

/// **A systematic difference between the two engines, recorded rather than
/// papered over.** gpui rounds laid-out edges to whole pixels; Blink keeps them
/// fractional. `max-w-[45%]` of a 129px container is 58.05px in the browser and
/// 58px here.
///
/// It fits inside `ANCHORS.md` §5's ±0.5px bounds tolerance — but it *consumes*
/// that tolerance rather than sharing it, so the budget left for a real
/// disagreement is nearer zero than half a pixel. Worth knowing before anyone
/// reads a 0.4px delta as noise.
#[gpui::test]
fn a_percentage_length_lands_on_a_whole_pixel(cx: &mut TestAppContext) {
    let records = measure(cx, a_cell(&["--content", "overflow", "--width", "320"]));
    let name = relative(&records, "git-row-name");

    let width = name.size.width;
    assert!(
        (width - width.round()).abs() < px(0.001),
        "expected a whole pixel, got {width:?}",
    );
}

/// Without the directory column the filename is `flex-1` and takes the whole
/// container — the other branch of the truncation-mode decision.
#[gpui::test]
fn without_the_directory_the_name_takes_the_container(cx: &mut TestAppContext) {
    let records = measure(cx, a_cell(&["--content", "overflow", "--no-directory"]));

    let icon = relative(&records, "git-row-icon");
    let name = relative(&records, "git-row-name");
    let badge = relative(&records, "git-row-badge");

    assert!(!ids(&records).contains(&"git-row-dir".to_owned()));
    assert_px(name.origin.x, icon.origin.x + icon.size.width + px(6.0));
    assert_px(name.origin.x + name.size.width, badge.origin.x - px(6.0));
}

/// A root-level file with the column *on* gets the capped sizing and no
/// directory span — the case a "does a directory string exist" shortcut gets
/// wrong, and the reason `name_sizing` reads the prop.
#[gpui::test]
fn a_root_level_file_is_still_capped_at_forty_five_percent(cx: &mut TestAppContext) {
    let records = measure(cx, a_cell(&["--content", "short"]));

    let name = relative(&records, "git-row-name");
    let icon = relative(&records, "git-row-icon");
    let badge = relative(&records, "git-row-badge");
    let container_width = (badge.origin.x - px(6.0)) - (icon.origin.x + icon.size.width + px(6.0));

    assert!(!ids(&records).contains(&"git-row-dir".to_owned()));
    // `a.ts` is far narrower than the cap, so `basis-auto` wins and the span is
    // its own content — which is also why it does not stretch to the container.
    assert!(name.size.width < container_width * 0.45);
    assert!(name.size.width > px(0.0));
}

/// `text_width` is the field the gate was chosen for: the box alone cannot say
/// where the ellipsis landed.
#[gpui::test]
fn the_overflowing_name_is_clipped_and_the_short_one_is_not(cx: &mut TestAppContext) {
    let overflowing = measure(cx, a_cell(&["--content", "overflow"]));
    let short = measure(cx, a_cell(&["--content", "short"]));

    let long_name = find(&overflowing, "git-row-name")
        .text
        .expect("the name paints text");
    let short_name = find(&short, "git-row-name")
        .text
        .expect("the name paints text");

    assert_eq!(
        long_name.content,
        "an-extremely-long-file-name-that-must-truncate-in-the-sidebar-row.ts",
    );
    assert!(long_name.clipped);
    assert!(long_name.width > relative(&overflowing, "git-row-name").size.width);

    assert_eq!(short_name.content, "a.ts");
    assert!(!short_name.clipped);
}

/// Narrower widths bind harder; the gate drives at least three of them.
#[gpui::test]
fn the_container_narrows_with_the_surface(cx: &mut TestAppContext) {
    let mut widths = Vec::new();
    for width in ["240", "320", "420"] {
        let records = measure(cx, a_cell(&["--width", width, "--content", "overflow"]));
        widths.push(relative(&records, "git-row-name").size.width);
    }

    assert!(widths[0] < widths[1], "{widths:?}");
    assert!(widths[1] < widths[2], "{widths:?}");
}

/// Typography, which `ANCHORS.md` ranks last and which is nonetheless the class
/// most likely to differ: the family has to be *declared*, or gpui reports
/// whatever the platform inherited and the DOM never produces that string.
#[gpui::test]
fn every_text_anchor_names_its_family_and_its_size(cx: &mut TestAppContext) {
    let records = measure(cx, Cell::default());

    for id in [
        "git-row-name",
        "git-row-dir",
        "git-row-added",
        "git-row-deleted",
    ] {
        let text = find(&records, id).text.expect("paints text");
        assert_eq!(text.font.family, "CalSansUI", "{id}");
        assert!(text.font.size > px(0.0), "{id}");
        // `leading-[1.35]` is unitless, so each element's line height is its own
        // font size times 1.35 — within gpui's device-pixel snapping.
        assert!(
            (text.font.line_height - text.font.size * 1.35).abs() <= px(0.5),
            "{id}: {:?} vs {:?}",
            text.font.line_height,
            text.font.size,
        );
    }

    // `text-sm` on the button, `ui-text-sm` on the directory and the counts.
    let name = find(&records, "git-row-name").text.expect("paints text");
    let dir = find(&records, "git-row-dir").text.expect("paints text");
    assert_px(name.font.size, px(14.0));
    assert_px(dir.font.size, px(12.0));
}

/// `content_sized` reaches the recorded row, and the ceil it models is real.
///
/// `native/oracle/ANCHORS.md` v1.5 is built on one measured claim: gpui `ceil()`s
/// a text run's max-content width, so a content-sized box is always a whole
/// number of logical pixels. That claim was taken from an archived pair of
/// snapshots; this asserts it against a live layout of the real component,
/// which is the difference between a rule that happened to fit two numbers and
/// one that holds.
///
/// It also checks the declaration survives the whole path — component →
/// [`DriverAnchors`] → the driver's elements → [`fold_text_halves`]. The badge
/// is the interesting one: its box and its run are recorded separately and
/// folded, and a declaration put on the run half would be thrown away silently.
#[gpui::test]
fn the_trailing_group_declares_itself_content_sized_and_lands_on_whole_pixels(
    cx: &mut TestAppContext,
) {
    use crowbar_ui::components::CONTENT_SIZED;

    let records = measure(cx, Cell::default());

    for id in CONTENT_SIZED {
        let record = find(&records, id);
        assert!(
            record.content_sized,
            "{id} did not reach the record declared"
        );
        let width = record.bounds.size.width;
        assert_px(width, px(f32::from(width).ceil()));
    }

    // And nothing else on the row claims it — least of all the flexible sibling
    // that absorbs the excess, which a ceiled target would be wrong for.
    for id in [
        "git-row-item",
        "git-row-button",
        "git-row-icon",
        "git-row-name",
        "git-row-dir",
        "git-row-guide-0",
    ] {
        assert!(
            !find(&records, id).content_sized,
            "{id} must not declare it"
        );
    }
}

/// `line_sized` reaches the recorded row, and every anchor that claims it is
/// telling the truth (`native/oracle/ANCHORS.md` v1.6).
///
/// The claim is checkable here in a way it is not on paper: a line-sized box's
/// height **is** its own line height, and the assertion below reads both off
/// the same record. That is what makes the badge's absence from the list a
/// measurement rather than an opinion — its 16px box sits around a 13.5px line
/// box, two and a half pixels apart, so declaring it would have compared 16
/// against the reference's 13.33 and invented a delta on an anchor where the
/// archived gate run has both sides at exactly 16.
#[gpui::test]
fn every_bare_text_span_declares_line_sized_and_its_box_is_its_line_box(cx: &mut TestAppContext) {
    use crowbar_ui::components::LINE_SIZED;

    let records = measure(cx, Cell::default());

    for id in LINE_SIZED {
        let record = find(&records, id);
        assert!(record.line_sized, "{id} did not reach the record declared");
        let line_height = record.text.expect("paints text").font.line_height;
        assert_px(relative(&records, id).size.height, line_height);
    }

    for id in [
        "git-row-item",
        "git-row-button",
        "git-row-icon",
        "git-row-badge",
        "git-row-guide-0",
    ] {
        assert!(!find(&records, id).line_sized, "{id} must not declare it");
    }

    // The badge, measured: its height is authored, not derived.
    let badge = find(&records, "git-row-badge");
    let badge_line = badge.text.expect("the badge paints its label").font.line_height;
    assert_px(badge.bounds.size.height, px(16.0));
    assert!(
        (badge.bounds.size.height - badge_line).abs() > px(0.5),
        "the badge's box is `sm:h-4`, not its line box: {:?} vs {badge_line:?}",
        badge.bounds.size.height,
    );
}

/// **`--viewport-width` selects the badge's `sm:` variant**, which is the whole
/// reason the option exists.
///
/// Measured off the live reference at both viewports: 16px tall with a 10px
/// face at or above 640, 20px tall with a 12px face below it. Everything else
/// about the badge is unprefixed and must not move — a variant switch that also
/// changed the radius or the border would be this component inventing a
/// difference rather than reproducing one.
#[gpui::test]
fn the_viewport_width_selects_the_badges_breakpoint_variant(cx: &mut TestAppContext) {
    let narrow = measure(cx, a_cell(&["--width", "294", "--viewport-width", "600"]));
    let wide = measure(cx, a_cell(&["--width", "294", "--viewport-width", "800"]));

    let below = find(&narrow, "git-row-badge");
    let above = find(&wide, "git-row-badge");

    assert_px(below.bounds.size.height, px(20.0));
    assert_px(above.bounds.size.height, px(16.0));
    assert_px(
        below.text.clone().expect("paints text").font.size,
        px(12.0),
    );
    assert_px(above.text.clone().expect("paints text").font.size, px(10.0));

    // The unprefixed half of the badge is the same at both viewports.
    assert_eq!(below.radius, above.radius);
    assert_eq!(below.border_width, above.border_width);
    assert_eq!(below.background, above.background);
    assert_eq!(
        below.text.expect("paints text").content,
        above.text.expect("paints text").content,
    );

    // 640 itself is already `sm` — `(width >= 40rem)`, not `>`.
    let at = measure(cx, a_cell(&["--width", "294", "--viewport-width", "640"]));
    assert_px(find(&at, "git-row-badge").bounds.size.height, px(16.0));
}

/// The surface width does **not** select the variant. A 294px sidebar in an
/// 800px window is the wide badge, exactly as the React original is — and
/// conflating the two is what produced four geometry deltas belonging to
/// neither side.
#[gpui::test]
fn the_surface_width_does_not_move_the_breakpoint(cx: &mut TestAppContext) {
    for width in ["240", "294", "420"] {
        let records = measure(cx, a_cell(&["--width", width]));
        assert_px(
            find(&records, "git-row-badge").bounds.size.height,
            px(16.0),
            );
    }
}

/// The three backgrounds `.file-tree-item::before` paints, keyed off the state
/// **parameter**. A `.hover(…)` refinement would report the resting paint here,
/// which is the whole reason the state is a parameter.
#[gpui::test]
fn the_state_parameter_reaches_the_painted_background(cx: &mut TestAppContext) {
    use crowbar_driver::Paint;

    let resting = find(&measure(cx, Cell::default()), "git-row-item");
    let hovered = find(&measure(cx, a_cell(&["--flags", "hover"])), "git-row-item");
    let selected = find(
        &measure(cx, a_cell(&["--flags", "selected"])),
        "git-row-item",
    );
    let focused = find(&measure(cx, a_cell(&["--flags", "focus"])), "git-row-item");

    assert_eq!(resting.background, Paint::None);
    assert_eq!(
        hovered.background,
        Paint::Solid(Theme::DARK.file_tree_hover_bg.value()),
    );
    assert_eq!(
        selected.background,
        Paint::Solid(Theme::DARK.accent.value())
    );
    // Focus paints nothing on this surface: the `:focus-visible` border rule is
    // scoped to `.file-tree-container`, which the git status panel is not in.
    assert_eq!(focused.background, Paint::None);
    assert_px(
        find(
            &measure(cx, a_cell(&["--flags", "focus"])),
            "git-row-button",
        )
        .border_width,
        px(0.0),
    );
}

/// The button paints nothing in any state — `bg-transparent`, and
/// `.file-tree-row:hover` pins it transparent with `!important`.
#[gpui::test]
fn the_button_never_paints_a_background(cx: &mut TestAppContext) {
    use crowbar_driver::Paint;

    for flags in ["hover", "selected", "hover,selected"] {
        let records = measure(cx, a_cell(&["--flags", flags]));
        assert_eq!(
            find(&records, "git-row-button").background,
            Paint::None,
            "{flags}",
        );
    }
}

/// The `empty` flag drops the trailing group, which is the one flag of the
/// three list-shaped ones that the original actually has.
#[gpui::test]
fn the_empty_flag_drops_the_badge_and_the_counts(cx: &mut TestAppContext) {
    let records = measure(cx, a_cell(&["--flags", "empty"]));
    let seen = ids(&records);

    for id in ["git-row-badge", "git-row-added", "git-row-deleted"] {
        assert!(!seen.contains(&id.to_owned()), "{id} should be gone");
    }
    assert!(seen.contains(&"git-row-name".to_owned()));
}

/// Light and dark are the same layout and a different palette — so a geometry
/// delta between the two themes would be a bug in the row, not in the tokens.
#[gpui::test]
fn the_theme_changes_the_palette_and_not_the_layout(cx: &mut TestAppContext) {
    let dark = measure(cx, a_cell(&["--theme", "dark"]));
    let light = measure(cx, a_cell(&["--theme", "light"]));

    for id in [
        "git-row-button",
        "git-row-icon",
        "git-row-name",
        "git-row-dir",
    ] {
        assert_eq!(relative(&dark, id), relative(&light, id), "{id}");
    }
    assert_ne!(
        find(&dark, "git-row-name").text.expect("paints text").color,
        find(&light, "git-row-name")
            .text
            .expect("paints text")
            .color,
    );
}

/// **The badge is a painted box *containing* text, and the snapshot has to say
/// both.** `ANCHORS.md` §3 puts the text group alongside `bg`/`radius`/`border`
/// rather than instead of them, and the DOM extractor emits both halves for it.
/// Emitting only the box cost five `FieldPresence` deltas on one anchor in the
/// first gate run.
#[gpui::test]
fn the_badge_anchor_carries_its_box_and_its_text(cx: &mut TestAppContext) {
    let badge = find(&measure(cx, Cell::default()), "git-row-badge");

    // The box half.
    assert!(matches!(badge.background, crowbar_driver::Paint::Solid(_)));
    assert_eq!(badge.radius, px(4.0));
    assert_eq!(badge.border_width, px(1.0));
    assert!(badge.visible);

    // The text half, on the same record.
    let text = badge.text.expect("the badge paints its label");
    assert_eq!(text.content, crowbar_ui::components::BADGE_LABEL);
    assert!(text.width > px(0.0));
    assert!(!text.clipped, "the badge is shrink-to-fit and nowrap");
    assert_eq!(text.font.family, "CalSansUI");
    assert!((text.font.weight - 500.0).abs() < f32::EPSILON);
}

/// The counts are content, and content is what the two sides have to share.
/// `--deleted 0` omits the anchor exactly as `deletions > 0` gates the span in
/// the React original — which is the shape the live reference row is in.
#[gpui::test]
fn the_counts_flags_drive_the_trailing_anchors(cx: &mut TestAppContext) {
    let records = measure(cx, a_cell(&["--added", "1", "--deleted", "0"]));
    let seen = ids(&records);

    assert!(!seen.contains(&"git-row-deleted".to_owned()));
    assert_eq!(
        find(&records, "git-row-added")
            .text
            .expect("paints text")
            .content,
        "+1",
    );
    assert!(seen.contains(&"git-row-badge".to_owned()));
}

/// The **state** gate: `--surface file-tree-row`.
///
/// A module rather than a second file, because it shares the harness above and a
/// second copy of `measure` is a second thing to drift. What it does not share is
/// two numbers: this row's indent step is **16** and its line box is **20**,
/// where the git row's are 14 and 18.9.
mod file_tree_row {
    use super::{a_cell, assert_px, find, ids, measure, relative_to};
    use crowbar_driver::{Paint, RawAnchor};
    use crowbar_ui::Theme;
    use crowbar_ui::components::{
        ALL_GIT_STATUSES, GUIDE_END_INSET, GUIDE_WIDTH, GitStatus, ICON_SIZE, ROW_HEIGHT, RowState,
        file_tree_row,
    };
    use gpui::{Bounds, Pixels, TestAppContext, px};

    use crate::row_surface::Cell;

    /// A cell on this surface, with the selector already applied.
    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "file-tree-row"];
        line.extend_from_slice(args);
        a_cell(&line)
    }

    /// Bounds relative to *this* surface's root.
    fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
        relative_to(records, file_tree_row::ID_ITEM, id)
    }

    /// Anchor presence is what the differ ranks first, so a missing one is the
    /// loudest possible failure.
    #[gpui::test]
    fn the_default_cell_carries_every_contract_anchor(cx: &mut TestAppContext) {
        let seen = ids(&measure(cx, cell(&[])));

        for id in [
            "file-row-item",
            "file-row-button",
            "file-row-icon",
            "file-row-name",
            "file-row-guide-0",
            "file-row-guide-1",
        ] {
            assert!(seen.contains(&id.to_owned()), "{id} is missing from {seen:?}");
        }
        assert!(
            !seen.contains(&"file-row-guide-2".to_owned()),
            "depth 2 has two guides, not three",
        );
        // And nothing from the other surface leaked in. Two roots in one frame
        // would make `Snapshot::build` anchor to whichever it found first.
        assert!(!seen.iter().any(|id| id.starts_with("git-row-")), "{seen:?}");
    }

    /// The wrapper and the button are one full-width box, as on the git row:
    /// `.file-tree-item` is `width: 100% !important` with no padding or border,
    /// and the button is `width: 100% !important` inside it.
    #[gpui::test]
    fn the_wrapper_and_the_button_are_one_full_width_box(cx: &mut TestAppContext) {
        let one = cell(&[]);
        let records = measure(cx, one.clone());

        let item = at(&records, "file-row-item");
        let button = at(&records, "file-row-button");

        assert_px(item.origin.x, px(0.0));
        assert_px(item.origin.y, px(0.0));
        assert_px(item.size.width, one.width_px());
        assert_px(item.size.height, ROW_HEIGHT);
        assert_px(button.origin.x, px(0.0));
        assert_px(button.size.width, one.width_px());
        assert_px(button.size.height, ROW_HEIGHT);
    }

    /// **The scoped stylesheet, measured.** This is the pair that separates the
    /// two surfaces' chrome: the git row's button has an 8px radius and *no*
    /// border, because `border-none` computes a `none`-styled border's width to
    /// zero. Inside `.file-tree-container` the container rule pins
    /// `border: 1px solid transparent !important` and `border-radius: 2px
    /// !important` over both.
    #[gpui::test]
    fn the_button_is_two_px_round_with_a_one_px_border(cx: &mut TestAppContext) {
        let records = measure(cx, cell(&[]));
        let button = find(&records, "file-row-button");

        assert_px(button.radius, px(2.0));
        assert_px(button.border_width, px(1.0));
        assert_eq!(
            button.border_color,
            Some(file_tree_row::button_border_color(&Theme::DARK, RowState::resting()).value()),
        );
        // The wrapper rounds the painted background by the same 2px.
        assert_px(find(&records, "file-row-item").radius, px(2.0));
        // And the button paints no background: `background-color: transparent
        // !important`, with `.file-tree-row:hover` pinned transparent too.
        assert_eq!(button.background, Paint::None);
    }

    /// The icon starts at the leading padding **plus the border**, and the
    /// leading padding steps by **16** — `settings.fileTreeIndentSize`, whose
    /// default is 16. The sidebar tree's own step is 14, and reusing it would
    /// put every level two pixels out.
    ///
    /// The extra pixel is not a rounding artefact and was measured rather than
    /// predicted: `TreeRow` writes `padding-left` as an inline style, and the
    /// container rule adds `border: 1px solid transparent` outside it, so the
    /// content box begins at `1 + padding`. The git row's button has **no**
    /// border — `border-none` computes a `none`-styled border's width to zero —
    /// so its icon sits at the padding exactly. Two surfaces, two answers, and
    /// this is the assertion that keeps one from being copied onto the other.
    #[gpui::test]
    fn the_icon_starts_at_a_sixteen_pixel_indent_past_the_border(cx: &mut TestAppContext) {
        for depth in [0_u16, 1, 4] {
            let records = measure(cx, cell(&["--depth", &depth.to_string()]));
            let icon = at(&records, "file-row-icon");
            let border = find(&records, "file-row-button").border_width;

            assert_px(icon.origin.x, border + file_tree_row::leading_padding(depth));
            assert_px(icon.origin.x, px(1.0 + 10.0 + 16.0 * f32::from(depth)));
            assert_px(icon.size.width, ICON_SIZE);
            assert_px(icon.size.height, ICON_SIZE);
            // The content box is the 24px row less its 1px borders and 4px
            // paddings — 14px, which the 14px icon fills exactly, so it sits at
            // `border + padding` on the block axis too.
            assert_px(icon.origin.y, px(5.0));
            assert_px(
                icon.origin.y + icon.size.height,
                ROW_HEIGHT - px(1.0) - px(4.0),
            );
        }
    }

    /// One guide per level at `10 + 16n + 7 - 3`, 7px wide, full height: the
    /// neighbours default to this row's own depth, so nothing is capped.
    #[gpui::test]
    fn the_guides_step_by_sixteen_too(cx: &mut TestAppContext) {
        let records = measure(cx, cell(&["--depth", "3"]));
        let offset = Theme::DARK.file_tree_guide_icon_offset.value();

        for level in 0..3_u16 {
            let guide = at(&records, &file_tree_row::guide_id(level));

            assert_px(guide.origin.x, file_tree_row::guide_left(level, offset));
            assert_px(guide.origin.x, px(14.0 + 16.0 * f32::from(level)));
            assert_px(guide.size.width, GUIDE_WIDTH);
            assert_px(guide.origin.y, px(0.0));
            assert_px(guide.size.height, ROW_HEIGHT);
        }
    }

    /// The cap, driven the only way it can be: a shallower row above and below.
    #[gpui::test]
    fn a_guide_that_begins_and_ends_at_this_row_is_capped(cx: &mut TestAppContext) {
        let records = measure(
            cx,
            cell(&["--depth", "2", "--prev-depth", "0", "--next-depth", "0"]),
        );

        for level in 0..2_u16 {
            let guide = at(&records, &file_tree_row::guide_id(level));
            assert_px(guide.origin.y, GUIDE_END_INSET);
            assert_px(guide.size.height, ROW_HEIGHT - GUIDE_END_INSET * 2.0);
        }
    }

    /// **The one this surface exists for.** All three of the row's real states
    /// paint, and each paints something *different* — which is what makes the
    /// state axis comparable here and vacuous on the git row.
    ///
    /// Note where each lands: hover and selection are the wrapper's
    /// (`.file-tree-item::before`), focus is the **button's border**
    /// (`:focus-visible`, scoped to `.file-tree-container`). Reading focus off
    /// the background would report "no change" and conclude, wrongly, that this
    /// surface has no focus state either.
    #[gpui::test]
    fn every_state_flag_paints_something_different(cx: &mut TestAppContext) {
        let theme = Theme::DARK;

        let resting = measure(cx, cell(&[]));
        let hovered = measure(cx, cell(&["--flags", "hover"]));
        let selected = measure(cx, cell(&["--flags", "selected"]));
        let focused = measure(cx, cell(&["--flags", "focus"]));

        let background = |records: &[RawAnchor]| find(records, "file-row-item").background;
        let border = |records: &[RawAnchor]| find(records, "file-row-button").border_color;

        assert_eq!(background(&resting), Paint::None);
        assert_eq!(
            background(&hovered),
            Paint::Solid(theme.file_tree_hover_bg.value()),
        );
        assert_eq!(background(&selected), Paint::Solid(theme.accent.value()));
        assert_ne!(background(&hovered), background(&selected));

        // Focus leaves the background alone and moves the border off transparent.
        assert_eq!(background(&focused), Paint::None);
        assert_eq!(border(&resting), border(&hovered));
        assert_eq!(border(&resting), border(&selected));
        assert_ne!(border(&focused), border(&resting));
        assert_eq!(
            border(&focused),
            Some(theme.accent.mix(42.0, theme.border).value()),
        );
        // The border is 1px wide in every state, which is what makes its colour
        // a *comparable* field at all: ANCHORS.md v1.3 ignores `border.color`
        // where `w == 0`, because a zero-width border reports inherited junk.
        for records in [&resting, &hovered, &selected, &focused] {
            assert_px(find(records, "file-row-button").border_width, px(1.0));
        }
    }

    /// `data-active` beats `:hover` — the attribute selector and the
    /// pseudo-class have the same specificity, and the active rule is second.
    #[gpui::test]
    fn selected_wins_over_hover(cx: &mut TestAppContext) {
        let both = measure(cx, cell(&["--flags", "hover,selected"]));

        assert_eq!(
            find(&both, "file-row-item").background,
            Paint::Solid(Theme::DARK.accent.value()),
        );
    }

    /// `text_width` is the field the gate was chosen for: the box alone cannot
    /// say where the ellipsis landed.
    #[gpui::test]
    fn the_overflowing_name_is_clipped_and_the_short_one_is_not(cx: &mut TestAppContext) {
        let long = find(
            &measure(cx, cell(&["--content", "overflow"])),
            "file-row-name",
        )
        .text
        .expect("the name paints text");
        let short = find(&measure(cx, cell(&["--content", "short"])), "file-row-name")
            .text
            .expect("the name paints text");

        assert_eq!(
            long.content,
            "an-extremely-long-file-name-that-must-truncate-in-the-sidebar-row.ts",
        );
        assert!(long.clipped);
        assert_eq!(short.content, "a.ts");
        assert!(!short.clipped);
    }

    /// Narrower widths bind harder, which is what makes the width axis worth
    /// driving on this surface as well as on the geometry one.
    #[gpui::test]
    fn the_name_narrows_with_the_surface(cx: &mut TestAppContext) {
        let mut widths = Vec::new();
        for width in ["240", "320", "420"] {
            let records = measure(cx, cell(&["--width", width, "--content", "overflow"]));
            widths.push(at(&records, "file-row-name").size.width);
        }

        assert!(widths[0] < widths[1], "{widths:?}");
        assert!(widths[1] < widths[2], "{widths:?}");
    }

    /// **The line box is 20px, not the git row's 18.9.** `GitFileItem` authors
    /// `leading-[1.35]`; this row authors nothing and inherits `.text-sm`'s
    /// `calc(1.25 / 0.875)`. Declaring `line_sized` against the wrong line
    /// height is precisely how a delta gets manufactured, so it is measured here
    /// rather than reasoned about.
    #[gpui::test]
    fn the_name_is_fourteen_px_text_on_a_twenty_px_line(cx: &mut TestAppContext) {
        let records = measure(cx, cell(&[]));
        let text = find(&records, "file-row-name")
            .text
            .expect("the name paints text");

        assert_eq!(text.font.family, "CalSansUI");
        assert_px(text.font.size, px(14.0));
        assert_px(text.font.line_height, px(20.0));
        // And the box *is* the line box, which is the claim `line_sized` makes.
        assert_px(at(&records, "file-row-name").size.height, px(20.0));
    }

    /// v1.5 and v1.6 reach the record, and both claims hold of the layout they
    /// describe: the width is a whole pixel (gpui `ceil()`s a text run's
    /// max-content width) and the height is the run's own line height.
    #[gpui::test]
    fn the_name_declares_both_and_the_layout_bears_them_out(cx: &mut TestAppContext) {
        let records = measure(cx, cell(&[]));

        for id in file_tree_row::CONTENT_SIZED {
            let record = find(&records, id);
            assert!(
                record.content_sized,
                "{id} did not reach the record declared"
            );
            let width = record.bounds.size.width;
            assert_px(width, px(f32::from(width).ceil()));
        }
        for id in file_tree_row::LINE_SIZED {
            let record = find(&records, id);
            assert!(record.line_sized, "{id} did not reach the record declared");
            let line = record
                .text
                .expect("a line-sized anchor paints text")
                .font
                .line_height;
            assert_px(at(&records, id).size.height, line);
        }

        // Nothing else claims either — least of all the button, which *does*
        // paint text (it is where the font is declared) but authors its 24px box
        // with `h-6` around that same 20px line. Declaring it would compare 24
        // against 20 and invent a 4px delta on an anchor both engines agree on.
        for id in [
            "file-row-item",
            "file-row-button",
            "file-row-icon",
            "file-row-guide-0",
        ] {
            let record = find(&records, id);
            assert!(!record.content_sized, "{id} must not declare content_sized");
            assert!(!record.line_sized, "{id} must not declare line_sized");
        }
        assert_px(
            find(&records, "file-row-button").bounds.size.height,
            ROW_HEIGHT,
        );
    }

    /// Light and dark are the same layout and a different palette, so a geometry
    /// delta between the two themes would be a bug in the row.
    #[gpui::test]
    fn the_theme_changes_the_palette_and_not_the_layout(cx: &mut TestAppContext) {
        let dark = measure(cx, cell(&["--theme", "dark"]));
        let light = measure(cx, cell(&["--theme", "light"]));

        for id in [
            "file-row-button",
            "file-row-icon",
            "file-row-name",
            "file-row-guide-0",
        ] {
            assert_eq!(at(&dark, id), at(&light, id), "{id}");
        }
        assert_ne!(
            find(&dark, "file-row-name")
                .text
                .expect("paints text")
                .color,
            find(&light, "file-row-name")
                .text
                .expect("paints text")
                .color,
        );

        // And the selection colour follows the token table rather than a literal
        // — the same flag in the other theme is a different paint.
        let dark_selected = measure(cx, cell(&["--theme", "dark", "--flags", "selected"]));
        let light_selected = measure(cx, cell(&["--theme", "light", "--flags", "selected"]));
        assert_eq!(
            find(&dark_selected, "file-row-item").background,
            Paint::Solid(Theme::DARK.accent.value()),
        );
        assert_eq!(
            find(&light_selected, "file-row-item").background,
            Paint::Solid(Theme::LIGHT.accent.value()),
        );
    }

    /// **The delta this item exists for, measured.**
    ///
    /// Driving both apps into `file-tree-row · dark · short · selected` left one
    /// field disagreeing: `file-row-name.fg`, `#f5f5f5ff` against the
    /// reference's `#fe9a00ff`. The reference fixture's `a.ts` is modified and
    /// the React name span carries `text-git-modified`; this port painted every
    /// filename on the inherited foreground. Six statuses, six tokens, and the
    /// resting row unchanged.
    #[gpui::test]
    fn every_git_status_paints_the_name_its_own_token(cx: &mut TestAppContext) {
        let theme = Theme::DARK;
        let fg = |records: &[RawAnchor]| {
            find(records, "file-row-name")
                .text
                .expect("the name paints text")
                .color
        };

        // The default is untouched, and so is the word for it.
        assert_eq!(fg(&measure(cx, cell(&[]))), theme.foreground.value());
        assert_eq!(
            fg(&measure(cx, cell(&["--git-status", "none"]))),
            theme.foreground.value(),
        );

        for status in ALL_GIT_STATUSES {
            let records = measure(cx, cell(&["--git-status", status.name()]));
            assert_eq!(
                fg(&records),
                status.color(&theme).value(),
                "{}",
                status.name(),
            );
            assert_ne!(fg(&records), theme.foreground.value(), "{}", status.name());
        }

        // Two statuses, two colours: the axis is real, not one flag with one
        // paint. `modified` is the amber the oracle measured.
        assert_ne!(
            fg(&measure(cx, cell(&["--git-status", "modified"]))),
            fg(&measure(cx, cell(&["--git-status", "deleted"]))),
        );
        assert_eq!(
            fg(&measure(cx, cell(&["--git-status", "modified"]))),
            GitStatus::Modified.color(&theme).value(),
        );

        // And it follows the token table, so the same status in the other theme
        // is a different paint rather than a literal baked into the row.
        assert_ne!(
            fg(&measure(
                cx,
                cell(&["--theme", "light", "--git-status", "untracked"])
            )),
            fg(&measure(
                cx,
                cell(&["--theme", "dark", "--git-status", "untracked"])
            )),
        );
    }

    /// The status moves the name's colour and **nothing else**.
    ///
    /// Not the icon — the React icon keeps `text-muted-foreground`, and the
    /// class is on neither it nor the label wrapper — not the wrapper's
    /// background, not the button's border, and not the anchor set: the status
    /// letter is rendered but deliberately unanchored, because the React span
    /// carries no `data-oracle-id` and an anchor the reference cannot produce is
    /// a `FieldPresence` delta.
    #[gpui::test]
    fn the_status_leaves_the_icon_the_chrome_and_the_anchor_set_alone(cx: &mut TestAppContext) {
        let plain = measure(cx, cell(&[]));
        let modified = measure(cx, cell(&["--git-status", "modified"]));

        assert_eq!(ids(&plain), ids(&modified));
        for id in [
            "file-row-item",
            "file-row-button",
            "file-row-icon",
            "file-row-guide-0",
        ] {
            assert_eq!(at(&plain, id), at(&modified, id), "{id}");
        }
        // The name starts in the same place. Its *width* is allowed to move and
        // does at this cell's content length — see
        // `the_status_letter_takes_room_from_a_clamped_name`, which is the whole
        // reason the letter is rendered rather than skipped as "unanchored".
        assert_eq!(
            at(&plain, "file-row-name").origin,
            at(&modified, "file-row-name").origin,
        );
        assert_eq!(
            find(&plain, "file-row-item").background,
            find(&modified, "file-row-item").background,
        );
        assert_eq!(
            find(&plain, "file-row-button").border_color,
            find(&modified, "file-row-button").border_color,
        );
        // The icon is an empty box on this side by design (see `icon()`), so
        // "the icon is not coloured" is a claim about the React class, which is
        // on the name span alone. What is checkable here is that the icon anchor
        // paints no text of its own to have been recoloured.
        assert!(find(&modified, "file-row-icon").text.is_none());
    }

    /// The status letter is unanchored but **not weightless**.
    ///
    /// It is a flex sibling of the name inside the `gap-1.5` label group, so
    /// wherever the name is clamped rather than content-sized it gets six pixels
    /// of gap and one glyph less room. Leaving it out — which is what "unanchored
    /// so it does not matter" would have meant — hands the native name all of
    /// that back and puts a width delta on `file-row-name` in the one cell
    /// truncation is measured in.
    #[gpui::test]
    fn the_status_letter_takes_room_from_a_clamped_name(cx: &mut TestAppContext) {
        let width = |records: &[RawAnchor]| at(records, "file-row-name").size.width;

        let clamped = width(&measure(cx, cell(&["--content", "overflow"])));
        let clamped_with_letter = width(&measure(
            cx,
            cell(&["--content", "overflow", "--git-status", "modified"]),
        ));
        assert!(
            clamped_with_letter < clamped,
            "{clamped_with_letter:?} should be narrower than {clamped:?}",
        );

        // And where the name is content-sized it is unmoved, which is why the
        // `short` cell converged on everything except the colour.
        let content_sized = width(&measure(cx, cell(&["--content", "short"])));
        let content_sized_with_letter = width(&measure(
            cx,
            cell(&["--content", "short", "--git-status", "modified"]),
        ));
        assert_px(content_sized_with_letter, content_sized);
    }
}
