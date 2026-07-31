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
    Context, IntoElement, ParentElement as _, Pixels, Render, Size, Styled as _, TestAppContext,
    Window, div, px,
};

use crate::driver_anchors::{DriverAnchors, fold_text_halves};
use crate::row_surface::{Cell, RowSurface, render_row};

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
/// The window is `RowSurface::window_size`, exactly as the app makes it — on
/// **both** axes since P2.5. Sizing it to some fixed 1200px instead would let a
/// surface wider than the window pass unnoticed here and clip in the real
/// binary, and it would make the harness and the thing it measures two
/// different configurations; the height used to be a fixed 400 for no better
/// reason than that no surface had yet been tall enough for it to matter.
fn measure(cx: &mut TestAppContext, cell: Cell) -> Vec<RawAnchor> {
    let window = RowSurface::window_size(&cell);
    measure_in(cx, cell, window).1
}

/// [`measure`] in a window of a stated size, with the registry kept.
///
/// Two callers need one of the two extras. `row_layout::window` drives a window
/// the surface does **not** fit in, which is the case a harness that always
/// sized itself correctly could never reach; and `row_snapshot::emit` takes the
/// registry, so a test of the refusal has to hold it rather than only its
/// records.
fn measure_in(
    cx: &mut TestAppContext,
    cell: Cell,
    window: Size<Pixels>,
) -> (AnchorRegistry, Vec<RawAnchor>) {
    let anchors: AnchorRegistry = cx.update(crowbar_driver::install);
    let _window = cx.open_window(window, |_, _| Stage(cell));
    cx.run_until_parked();
    let records = fold_text_halves(anchors.records());
    (anchors, records)
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
    crowbar_driver::leak_checked!(cx);
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
    crowbar_driver::leak_checked!(cx);
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
    crowbar_driver::leak_checked!(cx);
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
    crowbar_driver::leak_checked!(cx);
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
    crowbar_driver::leak_checked!(cx);
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
    crowbar_driver::leak_checked!(cx);
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
    crowbar_driver::leak_checked!(cx);
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
    crowbar_driver::leak_checked!(cx);
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
    crowbar_driver::leak_checked!(cx);
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
    crowbar_driver::leak_checked!(cx);
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
    crowbar_driver::leak_checked!(cx);
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
    crowbar_driver::leak_checked!(cx);
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
    crowbar_driver::leak_checked!(cx);
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
    crowbar_driver::leak_checked!(cx);
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
    crowbar_driver::leak_checked!(cx);

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
    crowbar_driver::leak_checked!(cx);

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
    let badge_line = badge
        .text
        .expect("the badge paints its label")
        .font
        .line_height;
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
    crowbar_driver::leak_checked!(cx);
    let narrow = measure(cx, a_cell(&["--width", "294", "--viewport-width", "600"]));
    let wide = measure(cx, a_cell(&["--width", "294", "--viewport-width", "800"]));

    let below = find(&narrow, "git-row-badge");
    let above = find(&wide, "git-row-badge");

    assert_px(below.bounds.size.height, px(20.0));
    assert_px(above.bounds.size.height, px(16.0));
    assert_px(below.text.clone().expect("paints text").font.size, px(12.0));
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
    crowbar_driver::leak_checked!(cx);
    for width in ["240", "294", "420"] {
        let records = measure(cx, a_cell(&["--width", width]));
        assert_px(find(&records, "git-row-badge").bounds.size.height, px(16.0));
    }
}

/// The three backgrounds `.file-tree-item::before` paints, keyed off the state
/// **parameter**. A `.hover(…)` refinement would report the resting paint here,
/// which is the whole reason the state is a parameter.
#[gpui::test]
fn the_state_parameter_reaches_the_painted_background(cx: &mut TestAppContext) {
    use crowbar_driver::Paint;
    crowbar_driver::leak_checked!(cx);

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
    crowbar_driver::leak_checked!(cx);

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
    crowbar_driver::leak_checked!(cx);
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
    crowbar_driver::leak_checked!(cx);
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
    crowbar_driver::leak_checked!(cx);
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
    crowbar_driver::leak_checked!(cx);
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
        crowbar_driver::leak_checked!(cx);
        let seen = ids(&measure(cx, cell(&[])));

        for id in [
            "file-row-item",
            "file-row-button",
            "file-row-icon",
            "file-row-name",
            "file-row-guide-0",
            "file-row-guide-1",
        ] {
            assert!(
                seen.contains(&id.to_owned()),
                "{id} is missing from {seen:?}"
            );
        }
        assert!(
            !seen.contains(&"file-row-guide-2".to_owned()),
            "depth 2 has two guides, not three",
        );
        // And nothing from the other surface leaked in. Two roots in one frame
        // would make `Snapshot::build` anchor to whichever it found first.
        assert!(
            !seen.iter().any(|id| id.starts_with("git-row-")),
            "{seen:?}"
        );
    }

    /// The wrapper and the button are one full-width box, as on the git row:
    /// `.file-tree-item` is `width: 100% !important` with no padding or border,
    /// and the button is `width: 100% !important` inside it.
    #[gpui::test]
    fn the_wrapper_and_the_button_are_one_full_width_box(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
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
        crowbar_driver::leak_checked!(cx);
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
        crowbar_driver::leak_checked!(cx);
        for depth in [0_u16, 1, 4] {
            let records = measure(cx, cell(&["--depth", &depth.to_string()]));
            let icon = at(&records, "file-row-icon");
            let border = find(&records, "file-row-button").border_width;

            assert_px(
                icon.origin.x,
                border + file_tree_row::leading_padding(depth),
            );
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
        crowbar_driver::leak_checked!(cx);
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
        crowbar_driver::leak_checked!(cx);
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
        crowbar_driver::leak_checked!(cx);
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
        crowbar_driver::leak_checked!(cx);
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
        crowbar_driver::leak_checked!(cx);
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
        crowbar_driver::leak_checked!(cx);
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
        crowbar_driver::leak_checked!(cx);
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
        crowbar_driver::leak_checked!(cx);
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
        crowbar_driver::leak_checked!(cx);
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
        crowbar_driver::leak_checked!(cx);
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
        crowbar_driver::leak_checked!(cx);
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
        crowbar_driver::leak_checked!(cx);
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

/// The first Phase 2 surface: `--surface dropdown-menu`.
///
/// A module rather than a second file, for the reason the one above is: it
/// shares `measure`, and a second copy of the harness is a second thing to
/// drift. What it does not share is a single number — this surface is a *column*
/// of boxes, where both Phase 1 surfaces are one row, so every assertion here is
/// about stacking and about the one thing a row never had: a **negative margin**.
mod dropdown_menu {
    use super::{a_cell, assert_px, find, ids, measure, relative_to};
    use crowbar_driver::{Paint, RawAnchor};
    use crowbar_ui::Theme;
    use crowbar_ui::components::dropdown_menu;
    use gpui::{Bounds, Pixels, TestAppContext, px};

    use crate::row_surface::Cell;

    /// A cell on this surface, with the selector already applied.
    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "dropdown-menu"];
        line.extend_from_slice(args);
        a_cell(&line)
    }

    /// Bounds relative to *this* surface's root.
    fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
        relative_to(records, dropdown_menu::ID_POPUP, id)
    }

    /// The popup's content width at the default cell: `min-w-40` less `p-1` on
    /// both sides.
    const CONTENT_WIDTH: Pixels = px(160.0 - 8.0);

    /// `py-1` twice around a `text-sm` line box: 4 + 20 + 4.
    const ROW_HEIGHT: Pixels = px(28.0);

    /// Anchor presence is what the differ ranks first, so a missing one is the
    /// loudest possible failure.
    #[gpui::test]
    fn the_default_cell_carries_every_contract_anchor(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let seen = ids(&measure(cx, cell(&[])));

        for id in [
            "menu-popup",
            "menu-item-edit",
            "menu-item-copy",
            "menu-separator",
            "menu-item-delete",
        ] {
            assert!(
                seen.contains(&id.to_owned()),
                "{id} is missing from {seen:?}"
            );
        }
        // The comment menu has no label, no tick and no submenu, so none of
        // those anchors may appear: an anchor the reference cannot produce is a
        // `FieldPresence` delta that forgives nothing.
        for id in [
            "menu-label",
            "menu-checkbox-item",
            "menu-checkbox-indicator",
            "menu-radio-item",
            "menu-radio-indicator",
            "menu-sub-trigger",
        ] {
            assert!(!seen.contains(&id.to_owned()), "{id} should be absent");
        }
        // And nothing from another surface leaked in. Two roots in one frame
        // would make `Snapshot::build` anchor to whichever it found first.
        assert!(
            !seen.iter().any(|id| id.starts_with("git-row-")),
            "{seen:?}"
        );
    }

    /// **The popup's width is the CSS clamp, not a `max()` this port computed.**
    ///
    /// The class list declares `width: var(--anchor-width)` *and*
    /// `min-width: 10rem`, and the port declares both too — so this is taffy
    /// agreeing with `WebKit` about which one wins, at three points either side
    /// of the crossover.
    #[gpui::test]
    fn the_popup_width_is_the_anchor_clamped_by_the_minimum(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);

        // The live shape: a 24px trigger under a 160px floor.
        let narrow = measure(cx, cell(&[]));
        assert_px(at(&narrow, "menu-popup").size.width, px(160.0));

        // Exactly at the floor.
        let level = measure(cx, cell(&["--anchor-width", "160"]));
        assert_px(at(&level, "menu-popup").size.width, px(160.0));

        // And above it, where the anchor wins — the arm a hard-coded 160 would
        // have got wrong.
        let wide = measure(cx, cell(&["--anchor-width", "240", "--width", "320"]));
        assert_px(at(&wide, "menu-popup").size.width, px(240.0));

        // The floor is a call site's `className`, so it moves too.
        let raised = measure(cx, cell(&["--min-width", "200"]));
        assert_px(at(&raised, "menu-popup").size.width, px(200.0));
    }

    /// `rounded-lg` is **10px** here — this project redefines `--radius-lg` — and
    /// the popup has **no border**: `ring-1` is a box-shadow, and `border.w` is
    /// the one field `ANCHORS.md` v1.1 compares exactly.
    #[gpui::test]
    fn the_popup_is_ten_px_round_with_no_border(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let records = measure(cx, cell(&[]));
        let popup = find(&records, "menu-popup");

        assert_px(popup.radius, Theme::DARK.radius_lg.value());
        assert_px(popup.radius, px(10.0));
        assert_px(popup.border_width, px(0.0));
        assert_eq!(popup.background, Paint::Solid(Theme::DARK.popover.value()));
    }

    /// **The column, measured.** Four entries stack inside `p-1`, and the
    /// arithmetic is the one a reader can check: 4, then two 28px rows, then the
    /// separator's `my-1` + 1px + `my-1`, then the last row.
    #[gpui::test]
    fn the_entries_stack_at_the_padding_and_the_row_heights(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let records = measure(cx, cell(&[]));

        let edit = at(&records, "menu-item-edit");
        let copy = at(&records, "menu-item-copy");
        let separator = at(&records, "menu-separator");
        let delete = at(&records, "menu-item-delete");

        // `p-1` on the popup: the first row starts 4px in on both axes, and
        // every row is the popup's content width — they are block-level
        // children, which is why none of them is content-sized.
        assert_px(edit.origin.x, px(4.0));
        assert_px(edit.origin.y, px(4.0));
        assert_px(edit.size.width, CONTENT_WIDTH);
        assert_px(edit.size.height, ROW_HEIGHT);

        assert_px(copy.origin.y, edit.origin.y + ROW_HEIGHT);
        assert_px(copy.size.height, ROW_HEIGHT);

        // `my-1` above, 1px of rule, `my-1` below.
        assert_px(separator.origin.y, copy.origin.y + ROW_HEIGHT + px(4.0));
        assert_px(separator.size.height, px(1.0));
        assert_px(delete.origin.y, separator.origin.y + px(1.0) + px(4.0));
        assert_px(delete.size.height, ROW_HEIGHT);

        // And the popup is exactly as tall as its contents plus its padding.
        assert_px(
            at(&records, "menu-popup").size.height,
            delete.origin.y + ROW_HEIGHT + px(4.0),
        );
    }

    /// **The negative margin, which no Phase 1 surface had.** `-mx-1` exactly
    /// undoes the popup's `p-1`, so the rule runs from the popup's padding-box
    /// left edge to its right edge rather than stopping at the rows.
    ///
    /// Worth measuring rather than reasoning about: taffy is a separate
    /// implementation of block layout, and a negative margin that clamped at
    /// zero would leave a rule 8px short and centred, which is a difference a
    /// reader would call a rounding artefact.
    #[gpui::test]
    fn the_separator_bleeds_out_through_the_popups_padding(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let records = measure(cx, cell(&[]));

        let popup = at(&records, "menu-popup");
        let separator = at(&records, "menu-separator");

        assert_px(separator.origin.x, px(0.0));
        assert_px(separator.size.width, popup.size.width);
        // Which is 8px wider than the rows it sits between.
        assert_px(
            separator.size.width,
            at(&records, "menu-item-edit").size.width + px(8.0),
        );
        assert_eq!(
            find(&records, "menu-separator").background,
            Paint::Solid(Theme::DARK.border.value()),
        );
    }

    /// The one field the state axis moves on this surface, and it moves on
    /// **one row**: `focus:bg-accent`, from the `--focus-row` the cell names.
    #[gpui::test]
    fn focus_paints_one_row_and_leaves_the_others_alone(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let theme = Theme::DARK;
        let background = |records: &[RawAnchor], id: &str| find(records, id).background;

        let resting = measure(cx, cell(&[]));
        for id in ["menu-item-edit", "menu-item-copy", "menu-item-delete"] {
            assert_eq!(background(&resting, id), Paint::None, "{id}");
        }

        // `hover` and `focus` are the same paint here: the class list has no
        // `hover:` rule at all, and `base-ui` highlights by moving focus.
        for flag in ["hover", "focus"] {
            let records = measure(cx, cell(&["--flags", flag]));
            assert_eq!(
                background(&records, "menu-item-copy"),
                Paint::Solid(theme.accent.value()),
                "{flag}",
            );
            assert_eq!(
                background(&records, "menu-item-edit"),
                Paint::None,
                "{flag}"
            );
            assert_eq!(
                background(&records, "menu-item-delete"),
                Paint::None,
                "{flag}",
            );
        }

        // And the layout does not move: focus is paint only.
        let focused = measure(cx, cell(&["--flags", "focus"]));
        for id in ["menu-item-edit", "menu-item-copy", "menu-item-delete"] {
            assert_eq!(at(&resting, id), at(&focused, id), "{id}");
        }
    }

    /// **The destructive variant, which is the second half of the state axis.**
    /// Its text is red focused *and* at rest — the unconditional
    /// `data-[variant=destructive]:text-destructive` beats
    /// `focus:text-accent-foreground` — and its focus background is a tint of
    /// the same red rather than `--accent`.
    #[gpui::test]
    fn the_destructive_row_is_red_at_rest_and_tinted_when_focused(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let theme = Theme::DARK;

        let resting = measure(cx, cell(&[]));
        let delete = find(&resting, "menu-item-delete");
        let text = delete.text.clone().expect("the row paints its label");
        assert_eq!(text.content, "Delete");
        assert_eq!(text.color, theme.destructive.value());
        assert_eq!(delete.background, Paint::None);

        // The two default rows are on the popup's inherited colour, so the
        // variant is a real difference rather than one colour everywhere.
        assert_ne!(
            find(&resting, "menu-item-edit")
                .text
                .expect("paints text")
                .color,
            theme.destructive.value(),
        );

        let focused = measure(cx, cell(&["--flags", "focus", "--focus-row", "2"]));
        let delete = find(&focused, "menu-item-delete");
        assert_eq!(
            delete.background,
            Paint::Solid(
                theme
                    .destructive
                    .mix(20.0, crowbar_ui::Color::TRANSPARENT)
                    .value()
            ),
            "dark doubles the tint to /20",
        );
        assert_ne!(delete.background, Paint::Solid(theme.accent.value()));
        assert_eq!(
            delete.text.expect("paints text").color,
            theme.destructive.value(),
            "focus does not move a destructive row's colour",
        );

        // A focused *default* row does move, which is what makes the pair a
        // difference rather than a restatement.
        assert_eq!(
            find(&measure(cx, cell(&["--flags", "focus"])), "menu-item-copy")
                .text
                .expect("paints text")
                .color,
            theme.accent_foreground.value(),
        );
    }

    /// The typography, which has to be *declared* or gpui reports whatever the
    /// platform inherited and the DOM never produces that string.
    ///
    /// 14px on a 20px line box — Tailwind's stock `text-sm` pair, and **not** the
    /// git row's authored `leading-[1.35]` 18.9.
    #[gpui::test]
    fn every_row_is_fourteen_px_text_on_a_twenty_px_line(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let records = measure(cx, cell(&[]));

        for id in ["menu-item-edit", "menu-item-copy", "menu-item-delete"] {
            let text = find(&records, id).text.expect("the row paints its label");
            assert_eq!(text.font.family, "CalSansUI", "{id}");
            assert_px(text.font.size, px(14.0));
            assert_px(text.font.line_height, px(20.0));
        }

        // The label is the other pair: `text-xs`, 12px on 16.
        let labelled = measure(cx, cell(&["--label", "Comment"]));
        let label = find(&labelled, "menu-label")
            .text
            .expect("the label paints text");
        assert_px(label.font.size, px(12.0));
        assert_px(label.font.line_height, px(16.0));
        assert!(
            (label.font.weight - 500.0).abs() < f32::EPSILON,
            "font-medium"
        );
        assert_eq!(label.color, Theme::DARK.muted_foreground.value());
        // And its box is `py-1` around that 16px line, which is why it is not
        // declared line-sized.
        assert_px(at(&labelled, "menu-label").size.height, px(24.0));
    }

    /// **Nothing on this surface declares either v1.5 or v1.6**, and the layout
    /// bears that out: every row's box is taller than its own line box by the two
    /// `py-1`s, and every row is the popup's content width whatever its text
    /// says.
    ///
    /// This is the badge trap in a new shape. A row paints text and has a box, so
    /// it reads like the case v1.6 was written for; declaring it would compare 28
    /// against 20 and invent an 8px delta on an anchor both engines agree on.
    #[gpui::test]
    fn no_anchor_declares_content_or_line_sized_and_the_layout_agrees(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        assert!(dropdown_menu::CONTENT_SIZED.is_empty());
        assert!(dropdown_menu::LINE_SIZED.is_empty());

        let records = measure(cx, cell(&["--label", "Comment"]));
        for record in &records {
            assert!(!record.content_sized, "{}", record.id);
            assert!(!record.line_sized, "{}", record.id);
        }

        // Why not: the box is padding plus a line box, on both kinds of entry.
        for id in ["menu-item-edit", "menu-item-copy", "menu-label"] {
            let line = find(&records, id)
                .text
                .expect("paints text")
                .font
                .line_height;
            let height = at(&records, id).size.height;
            assert_px(height, line + px(8.0));
            assert!(height - line > px(0.5), "{id}");
        }

        // And the width is the container's, not the content's: two rows with
        // very different strings are exactly as wide as each other.
        assert_px(
            at(&records, "menu-item-edit").size.width,
            at(&records, "menu-item-copy").size.width,
        );
    }

    /// `text_width` and `clipped`, which is what the popup's `overflow-x: hidden`
    /// buys: a label wider than the row is cut, and the box alone cannot say so.
    ///
    /// The `overflow` fixture is one unbreakable token deliberately — a label
    /// with spaces would **wrap**, and a wrapped run is outside what the contract
    /// can compare.
    #[gpui::test]
    fn an_overlong_label_is_clipped_and_a_short_one_is_not(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);

        let short = find(
            &measure(cx, cell(&["--content", "short"])),
            "menu-item-copy",
        )
        .text
        .expect("paints text");
        assert_eq!(short.content, "Edit");
        assert!(!short.clipped);

        let long = find(
            &measure(cx, cell(&["--content", "overflow"])),
            "menu-item-copy",
        )
        .text
        .expect("paints text");
        assert_eq!(
            long.content,
            "CopyEveryOutstandingReviewCommentAsMarkdownIncludingResolvedOnes",
        );
        assert!(long.clipped);
        assert!(long.width > CONTENT_WIDTH);

        // The row's own box does not grow to fit it: the popup's width is
        // definite and the row is a block child of it.
        assert_px(
            at(
                &measure(cx, cell(&["--content", "overflow"])),
                "menu-item-copy",
            )
            .size
            .width,
            CONTENT_WIDTH,
        );
    }

    /// The tick gutter, and the anchor whose **box** carries the `selected`
    /// signal: `base-ui` unmounts the tick when unchecked, so the `<span>` stays
    /// and collapses.
    #[gpui::test]
    fn a_tick_row_reserves_its_gutter_and_the_tick_appears_only_when_checked(
        cx: &mut TestAppContext,
    ) {
        crowbar_driver::leak_checked!(cx);

        let unchecked = measure(cx, cell(&["--tick", "checkbox"]));
        let checked = measure(cx, cell(&["--tick", "checkbox", "--flags", "selected"]));

        for records in [&unchecked, &checked] {
            let row = at(records, "menu-checkbox-item");
            // `pr-8 pl-1.5` replaces `px-1.5`, so the row is the same box and
            // the gutter is inside it.
            assert_px(row.size.width, CONTENT_WIDTH);
            assert_px(row.size.height, ROW_HEIGHT);
            // `right-2` from the row's right edge, and vertically centred by the
            // flex container's `items-center`.
            let tick = at(records, "menu-checkbox-indicator");
            assert_px(
                row.origin.x + row.size.width - (tick.origin.x + tick.size.width),
                px(8.0),
            );
        }

        // Checked mounts a 16px icon; unchecked leaves an empty span.
        assert_px(at(&checked, "menu-checkbox-indicator").size.width, px(16.0));
        assert_px(
            at(&unchecked, "menu-checkbox-indicator").size.width,
            px(0.0),
        );

        // And the radio primitive is the same class list under its own anchor,
        // which is the only part of the difference the differ can see.
        let radio = ids(&measure(cx, cell(&["--tick", "radio"])));
        assert!(radio.contains(&"menu-radio-item".to_owned()));
        assert!(radio.contains(&"menu-radio-indicator".to_owned()));
        assert!(!radio.contains(&"menu-checkbox-item".to_owned()));
    }

    /// **`inset` moves nothing this surface's anchors can see, and that is a
    /// limit worth writing down rather than a test worth deleting.**
    ///
    /// `data-inset:pl-7` replaces `px-1.5`'s left half — the arithmetic is
    /// `MenuRow::padding_left`'s and is asserted there. What cannot be asserted
    /// *here* is where the padding puts anything, because the two things it
    /// moves are both unanchorable: a menu row's label is the row's own **text
    /// node** with no element around it, and the leading icon is an SVG the call
    /// site supplies. Neither can carry a `data-oracle-id`, so neither is in the
    /// anchor set, and the whole difference lands inside a box that does not
    /// change.
    ///
    /// So the assertion is the honest one: every anchored box is identical with
    /// and without it. A reader who expects the oracle to catch a wrong
    /// `data-inset` on this surface should read this and stop expecting it.
    #[gpui::test]
    fn inset_is_invisible_to_this_surfaces_anchor_set(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);

        let plain = measure(cx, cell(&["--label", "Comment"]));
        let inset = measure(cx, cell(&["--label", "Comment", "--inset"]));

        assert_eq!(ids(&plain), ids(&inset));
        for id in ids(&plain) {
            assert_eq!(at(&plain, &id), at(&inset, &id), "{id}");
        }

        // The one thing it *would* move — the padding — is spelled in the
        // component and checked there.
        assert_eq!(
            crowbar_ui::components::dropdown_menu::ROW_INSET_PADDING_LEFT,
            px(28.0),
        );
        assert_eq!(
            crowbar_ui::components::dropdown_menu::ROW_PADDING_X,
            px(6.0),
        );
    }

    /// Light and dark are the same layout and a different palette, so a geometry
    /// delta between the two themes would be a bug in the component.
    #[gpui::test]
    fn the_theme_changes_the_palette_and_not_the_layout(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let dark = measure(cx, cell(&["--theme", "dark"]));
        let light = measure(cx, cell(&["--theme", "light"]));

        for id in [
            "menu-popup",
            "menu-item-edit",
            "menu-item-copy",
            "menu-separator",
            "menu-item-delete",
        ] {
            assert_eq!(at(&dark, id), at(&light, id), "{id}");
        }
        assert_ne!(
            find(&dark, "menu-popup").background,
            find(&light, "menu-popup").background,
        );

        // And the destructive focus tint follows the `dark:` variant: /20 in
        // dark, /10 in light, which is a different colour and not a different
        // alpha of a literal.
        let tint = |records: &[RawAnchor]| find(records, "menu-item-delete").background;
        let dark_focus = measure(
            cx,
            cell(&["--theme", "dark", "--flags", "focus", "--focus-row", "2"]),
        );
        let light_focus = measure(
            cx,
            cell(&["--theme", "light", "--flags", "focus", "--focus-row", "2"]),
        );
        assert_eq!(
            tint(&dark_focus),
            Paint::Solid(
                Theme::DARK
                    .destructive
                    .mix(20.0, crowbar_ui::Color::TRANSPARENT)
                    .value()
            ),
        );
        assert_eq!(
            tint(&light_focus),
            Paint::Solid(
                Theme::LIGHT
                    .destructive
                    .mix(10.0, crowbar_ui::Color::TRANSPARENT)
                    .value()
            ),
        );
        assert_ne!(tint(&dark_focus), tint(&light_focus));
    }
}

/// The Phase 2 clipping surface: `--surface sidebar-carousel`.
///
/// A module rather than a second file, for the reason the two above are: it
/// shares `measure`, and a second copy of the harness is a second thing to
/// drift. What it does not share is the thing it measures — both Phase 1
/// surfaces and the menu fit inside their own boxes, and **three quarters of
/// this one is outside its own box at every instant**. So every assertion here
/// is about a track wider than the viewport it is seen through, and about the
/// one field that says so.
mod sidebar_carousel {
    use super::{a_cell, assert_px, find, ids, measure};
    use crowbar_driver::{Paint, RawAnchor};
    use crowbar_ui::components::sidebar_carousel::{ID_SCROLLPORT, SidebarTab, TABS};
    use gpui::{Bounds, Pixels, TestAppContext, px};

    use crate::row_surface::Cell;

    /// The sidebar width the live measurement was taken at: `clientWidth` 294
    /// in a 600px window, with a `scrollWidth` of 1176.
    const SURFACE_WIDTH: f32 = 294.0;

    /// `--height`'s default, and therefore every panel's height.
    const HEIGHT: f32 = 600.0;

    /// A cell on this surface, at the width the reference was measured at.
    fn cell(args: &[&str]) -> Cell {
        let mut line = vec![
            "--surface",
            "sidebar-carousel",
            "--width",
            "294",
            "--viewport-width",
            "600",
        ];
        line.extend_from_slice(args);
        a_cell(&line)
    }

    /// Bounds relative to *this* surface's root — the scrollport.
    fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
        super::relative_to(records, ID_SCROLLPORT, id)
    }

    /// Every anchor this surface has, root first and then the track in DOM
    /// order.
    fn every_anchor() -> Vec<&'static str> {
        let mut all = vec![ID_SCROLLPORT];
        all.extend(TABS.into_iter().map(SidebarTab::anchor));
        all
    }

    /// Anchor presence is what the differ ranks first, so a missing one is the
    /// loudest possible failure — and an *extra* one is an anchor the reference
    /// cannot produce, which forgives nothing either.
    #[gpui::test]
    fn the_default_cell_carries_every_contract_anchor(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);

        assert_eq!(ids(&measure(cx, cell(&[]))), every_anchor());
    }

    /// **The live measurement, reproduced.** At the sidebar's 294px the track is
    /// four scrollports — 1176px — wide, one of which is in view. That pair of
    /// numbers came off the running app, so this is the one assertion here that
    /// is checked against something other than gpui.
    #[gpui::test]
    fn the_track_is_four_scrollports_wide_and_shows_one(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let records = measure(cx, cell(&[]));

        let scrollport = at(&records, ID_SCROLLPORT);
        assert_px(scrollport.size.width, px(SURFACE_WIDTH));
        assert_px(scrollport.origin.x, px(0.0));

        for tab in TABS {
            let panel = at(&records, tab.anchor());
            // `min-w-full`, and it is a floor that every panel is pinned *at*:
            // four of them against one scrollport of space is negative free
            // space, so flexbox freezes each at its minimum.
            assert_px(panel.size.width, px(SURFACE_WIDTH));
            assert_px(panel.origin.x, px(f32::from(tab.index()) * SURFACE_WIDTH));
        }

        // The track's right edge: 4 × 294 = 1176 against a 294px scrollport.
        let last = at(&records, SidebarTab::Git.anchor());
        assert_px(last.origin.x + last.size.width, px(4.0 * SURFACE_WIDTH));
        assert_px(px(4.0 * SURFACE_WIDTH), px(1176.0));
    }

    /// **The field this surface exists for.** A panel scrolled out of the
    /// scrollport reports `visible: false` and keeps every pixel of its
    /// geometry, so a wrong scroll offset cannot hide behind the flag.
    ///
    /// The tangent case is asserted by name: at `scrollLeft = 2W` the fourth
    /// panel begins **exactly** at the scrollport's right edge, the intersection
    /// is zero-wide, and that has to read as invisible — the DOM side requires
    /// `r - l > 0` and the driver requires a non-empty intersection, which is
    /// the same answer arrived at twice.
    #[gpui::test]
    fn a_snapped_out_panel_is_invisible_and_keeps_its_geometry(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let records = measure(cx, cell(&["--flags", "selected"]));

        // `--active-tab` defaults to `files`, index 2.
        let expected = [
            (SidebarTab::Workspaces, -2.0, false),
            (SidebarTab::Chats, -1.0, false),
            (SidebarTab::Files, 0.0, true),
            (SidebarTab::Git, 1.0, false),
        ];
        for (tab, scrollports, visible) in expected {
            let panel = at(&records, tab.anchor());
            assert_px(panel.origin.x, px(scrollports * SURFACE_WIDTH));
            assert_px(panel.size.width, px(SURFACE_WIDTH));
            assert_eq!(
                find(&records, tab.anchor()).visible,
                visible,
                "{}",
                tab.name(),
            );
        }

        // The tangency, spelled out: the fourth panel's left edge is the
        // scrollport's right edge, so the overlap is exactly zero.
        let scrollport = at(&records, ID_SCROLLPORT);
        let git = at(&records, SidebarTab::Git.anchor());
        assert_px(git.origin.x, scrollport.origin.x + scrollport.size.width);
        assert!(!find(&records, SidebarTab::Git.anchor()).visible);

        // And the root is present, at the origin, and visible —
        // `ANCHORS.md` v1.1 §4 makes all three a load error otherwise.
        assert_px(scrollport.origin.x, px(0.0));
        assert_px(scrollport.origin.y, px(0.0));
        assert!(find(&records, ID_SCROLLPORT).visible);
    }

    /// Every tab snaps the track by **exactly one scrollport per index**, and
    /// leaves exactly one panel visible. This is `scrollLeft = index *
    /// clientWidth` measured rather than restated.
    #[gpui::test]
    fn every_tab_snaps_the_track_by_one_scrollport_per_index(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);

        for active in TABS {
            let records = measure(
                cx,
                cell(&["--flags", "selected", "--active-tab", active.name()]),
            );

            for tab in TABS {
                let offset = f32::from(tab.index()) - f32::from(active.index());
                assert_px(
                    at(&records, tab.anchor()).origin.x,
                    px(offset * SURFACE_WIDTH),
                );
                assert_eq!(
                    find(&records, tab.anchor()).visible,
                    tab == active,
                    "{} while showing {}",
                    tab.name(),
                    active.name(),
                );
            }

            let visible = TABS
                .into_iter()
                .filter(|tab| find(&records, tab.anchor()).visible)
                .count();
            assert_eq!(visible, 1, "showing {}", active.name());
        }
    }

    /// **The claim that licenses drawing the four panels empty, measured.**
    ///
    /// The port renders the panels with nothing in them because
    /// `WorkspaceTree`, `AgentChatsPanel`, `FileExplorerTree` and `GitPanel`
    /// have no native equivalent. That is only sound if a panel's box does not
    /// depend on what is inside it — and it does not, because all four are
    /// `min-w-full`, so the track always overflows and every item is frozen at
    /// its minimum whatever its flex base size was.
    ///
    /// Driving a filler several times the surface width through every panel and
    /// getting a byte-identical record set is that argument turned into a
    /// measurement.
    #[gpui::test]
    fn what_is_inside_a_panel_does_not_move_the_track(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let empty = measure(cx, cell(&["--flags", "selected"]));

        for filler in ["1", "1200", "4000"] {
            let stuffed = measure(
                cx,
                cell(&["--flags", "selected", "--panel-content", filler]),
            );

            // The whole record, not just its geometry: `bg`, `radius`,
            // `border` and `visible` all have to be untouched too, or "the
            // track ignores its contents" would be a claim about four fields
            // out of eight.
            assert_eq!(stuffed, empty, "{filler}");
            for id in ids(&empty) {
                assert_eq!(at(&stuffed, &id), at(&empty, &id), "{id} at {filler}");
                assert_eq!(
                    find(&stuffed, &id).visible,
                    find(&empty, &id).visible,
                    "{id} at {filler}",
                );
            }
        }
    }

    /// `h-full` is on the first two panels and not on the last two, and that
    /// asymmetry is **inert**: a flex item in a row stretches to the container's
    /// height, and `height: 100%` resolves against the same box. Measured here
    /// rather than argued, because it is the only place in this port where the
    /// JSX carries an inconsistency the port reproduces on purpose.
    #[gpui::test]
    fn h_full_and_the_default_stretch_give_the_same_height(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let records = measure(cx, cell(&["--flags", "selected"]));

        assert_px(at(&records, ID_SCROLLPORT).size.height, px(HEIGHT));
        for tab in TABS {
            let panel = at(&records, tab.anchor());
            assert_px(panel.size.height, px(HEIGHT));
            assert_px(panel.origin.y, px(0.0));
        }
        // The two that carry `h-full` and the two that do not, side by side.
        assert_eq!(
            TABS.map(SidebarTab::full_height),
            [true, true, false, false],
        );
    }

    /// `--height` is `NavStack`'s column, and it reaches every panel — which is
    /// what makes a parity run able to match the reference's sidebar rather than
    /// report an `h` delta on all five anchors.
    #[gpui::test]
    fn the_height_option_is_the_column_and_reaches_every_panel(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);

        for (option, expected) in [("120", 120.0), ("640", 640.0)] {
            let records = measure(cx, cell(&["--height", option]));
            for id in every_anchor() {
                assert_px(at(&records, id).size.height, px(expected));
            }
        }
    }

    /// The surface width scales the whole track: the panels are the scrollport,
    /// and the track is four of it. The §8.3 width axis is the one axis that
    /// does real work here.
    #[gpui::test]
    fn the_width_axis_scales_the_scrollport_and_the_track(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);

        for (width, expected) in [("240", 240.0), ("294", 294.0), ("420", 420.0)] {
            let records = measure(
                cx,
                a_cell(&[
                    "--surface",
                    "sidebar-carousel",
                    "--width",
                    width,
                    "--viewport-width",
                    "800",
                    "--flags",
                    "selected",
                ]),
            );

            assert_px(at(&records, ID_SCROLLPORT).size.width, px(expected));
            for tab in TABS {
                let panel = at(&records, tab.anchor());
                assert_px(panel.size.width, px(expected));
                // Two scrollports left of the origin at `--active-tab files`,
                // so the offset follows the width rather than being pinned.
                assert_px(
                    panel.origin.x,
                    px((f32::from(tab.index()) - 2.0) * expected),
                );
            }
        }
    }

    /// **This component paints nothing**, and that is a fact the differ can
    /// check: `bg` is transparent on all five anchors, `radius` is zero, and
    /// `border.w` — the one field `ANCHORS.md` v1.1 compares *exactly* — is
    /// zero. A port that reached for a background here would be told so on every
    /// cell of the matrix.
    #[gpui::test]
    fn nothing_on_this_surface_paints(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let records = measure(cx, cell(&["--flags", "selected"]));

        for id in every_anchor() {
            let record = find(&records, id);
            assert_eq!(record.background, Paint::None, "{id}");
            assert_px(record.radius, px(0.0));
            assert_px(record.border_width, px(0.0));
            assert_eq!(record.border_color, None, "{id}");
            // And no text: `text_width`, `clipped` and `font` are absent, which
            // is why both declaration lists are empty.
            assert!(record.text.is_none(), "{id}");
        }
    }

    /// **Two of §8.3's four axes cannot move an anchor on this surface**, and
    /// saying so is worth a test rather than a sentence: the component carries
    /// no colour, so `--theme` selects a token table nothing reads, and no text,
    /// so the three `--content` lengths are one picture. A future change that
    /// gave the carousel a background would fail here, which is the point.
    #[gpui::test]
    fn the_theme_and_content_axes_move_nothing(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let baseline = measure(cx, cell(&["--flags", "selected"]));

        for line in [
            vec!["--theme", "light"],
            vec!["--theme", "dark"],
            vec!["--content", "short"],
            vec!["--content", "overflow"],
        ] {
            let mut full = vec!["--flags", "selected"];
            full.extend_from_slice(&line);
            let records = measure(cx, cell(&full));

            assert_eq!(ids(&records), ids(&baseline), "{line:?}");
            for id in every_anchor() {
                assert_eq!(at(&records, id), at(&baseline, id), "{id} at {line:?}");
                assert_eq!(
                    find(&records, id).background,
                    find(&baseline, id).background,
                    "{id} at {line:?}",
                );
                assert_eq!(
                    find(&records, id).visible,
                    find(&baseline, id).visible,
                    "{id} at {line:?}",
                );
            }
        }
    }
}

/// The second Phase 2 surface: `--surface resizable`.
///
/// A module rather than a second file, for the reason the two above are: it shares
/// `measure`, and a second copy of the harness is a second thing to drift. What it
/// does not share is what it measures. Both Phase 1 surfaces and the menu are
/// *stacks of authored lengths*; this one is a **division**. `flex-basis: 0` with
/// fractional `flex-grow` around a fixed 1px sibling means every panel's width is
/// one arithmetic operation on the group's, and the only honest way to know
/// whether taffy performs it the way `WebKit` does is to lay it out and read the
/// bounds back.
///
/// It is also the surface with nothing else to measure — no text, and no colour
/// unless `--with-handle` is passed — which is asserted here rather than left as a
/// claim in the module docs.
mod resizable {
    use super::{a_cell, assert_px, assert_within_tolerance, find, ids, measure, relative_to};
    use crowbar_driver::{Paint, RawAnchor};
    use crowbar_ui::Theme;
    use crowbar_ui::components::resizable;
    use gpui::{Bounds, Pixels, TestAppContext, px};

    use crate::row_surface::Cell;

    /// A cell on this surface, with the selector already applied.
    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "resizable"];
        line.extend_from_slice(args);
        a_cell(&line)
    }

    /// Bounds relative to *this* surface's root.
    fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
        relative_to(records, resizable::ID_GROUP, id)
    }

    /// The three ids the live group carries, in DOM order.
    const SIDEBAR: &str = "resize-panel-sidebar";
    const HANDLE: &str = "resize-handle";
    const CONTENT: &str = "resize-panel-content";

    /// Anchor presence is what the differ ranks first, so a missing one is the
    /// loudest possible failure.
    #[gpui::test]
    fn the_default_cell_carries_every_contract_anchor(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let seen = ids(&measure(cx, cell(&[])));

        for id in ["resize-group", SIDEBAR, HANDLE, CONTENT] {
            assert!(
                seen.contains(&id.to_owned()),
                "{id} is missing from {seen:?}"
            );
        }
        // The live shell passes no `withHandle`, so the grip must not appear: an
        // anchor the reference cannot produce is a `FieldPresence` delta that
        // forgives nothing. Nor may the primitive's own default panel id, which
        // the two call-site names replace.
        for id in ["resize-handle-grip", "resize-panel"] {
            assert!(!seen.contains(&id.to_owned()), "{id} should be absent");
        }
        // And nothing from another surface leaked in. Two roots in one frame
        // would make `Snapshot::build` anchor to whichever it found first.
        assert!(
            !seen.iter().any(|id| id.starts_with("git-row-")
                || id.starts_with("file-row-")
                || id.starts_with("menu-")),
            "{seen:?}",
        );
    }

    /// **The group is the surface, and the shell box above it is not in the
    /// snapshot.** `width: 100%` resolves against the cell's surface width and
    /// `height: 100%` against `--shell-height`, which is the whole reason that
    /// parameter exists — a percentage against an auto-height parent computes to
    /// `auto`, and every bound here would be zero.
    #[gpui::test]
    fn the_group_fills_the_surface_and_the_shell_box(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let subject = cell(&["--width", "600", "--shell-height", "120"]);
        let records = measure(cx, subject.clone());
        let group = at(&records, "resize-group");

        assert_px(group.origin.x, px(0.0));
        assert_px(group.origin.y, px(0.0));
        assert_px(group.size.width, subject.width_px());
        assert_px(group.size.height, px(120.0));

        // The shell box carries no anchor, so it cannot reach a snapshot at all.
        assert!(!ids(&records).iter().any(|id| id.contains("shell")));
    }

    /// **The division, measured.** Three boxes tile the group's main axis exactly:
    /// the sidebar from the origin, the separator's single pixel, then the content
    /// panel to the far edge. No gap and no overlap, at a width where the share is
    /// *not* a whole number — which is the case a port that rounded each panel
    /// independently would get wrong.
    #[gpui::test]
    fn the_panels_and_the_separator_tile_the_group_exactly(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);

        for width in ["320", "600", "1100"] {
            let records = measure(cx, cell(&["--width", width, "--viewport-width", "1200"]));
            let group = at(&records, "resize-group");
            let sidebar = at(&records, SIDEBAR);
            let handle = at(&records, HANDLE);
            let content = at(&records, CONTENT);

            assert_px(sidebar.origin.x, px(0.0));
            assert_px(handle.origin.x, sidebar.size.width);
            assert_px(handle.size.width, resizable::HANDLE_THICKNESS);
            assert_px(
                content.origin.x,
                handle.origin.x + resizable::HANDLE_THICKNESS,
            );
            assert_px(content.origin.x + content.size.width, group.size.width);

            // The separator takes its pixel out of the free space before the
            // division, because `flex: 0 0 auto` keeps it out of the growth.
            assert_px(
                sidebar.size.width + content.size.width + resizable::HANDLE_THICKNESS,
                group.size.width,
            );

            // Every child stretches to the group's cross axis: the group sets no
            // `align-items`, so CSS's `stretch` is in force on both sides.
            for id in [SIDEBAR, HANDLE, CONTENT] {
                let child = at(&records, id);
                assert_px(child.origin.y, px(0.0));
                assert_px(child.size.height, group.size.height);
            }
        }
    }

    /// **The share, against the CSS arithmetic.** `flex-basis: 0` makes each
    /// panel's used width `grow / Σgrow × free`, where `free` is the group less the
    /// separator's pixel. Spelled as the division rather than as a literal, so a
    /// change to the fixture's factors cannot pass by leaving a number alone.
    ///
    /// Exact where the arithmetic lands on a whole pixel, and inside
    /// `native/oracle/ANCHORS.md` §5's ±0.5 where it does not — which is the
    /// honest statement, because the two engines are entitled to quantise it
    /// differently and the contract says so.
    #[gpui::test]
    fn each_panels_width_is_its_share_of_the_free_space(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);

        // An even split of a whole number of pixels: 321 − 1 = 320, half each.
        let even = measure(cx, cell(&["--width", "321", "--grow", "50,50"]));
        assert_px(at(&even, SIDEBAR).size.width, px(160.0));
        assert_px(at(&even, CONTENT).size.width, px(160.0));

        // The live pair, at a width where the share is fractional.
        let width = 600.0_f32;
        let free = width - f32::from(resizable::HANDLE_THICKNESS);
        let total = resizable::SIDEBAR_GROW + resizable::CONTENT_GROW;
        let records = measure(cx, cell(&["--width", "600", "--viewport-width", "700"]));

        assert_within_tolerance(
            at(&records, SIDEBAR).size.width,
            px(free * resizable::SIDEBAR_GROW / total),
        );
        assert_within_tolerance(
            at(&records, CONTENT).size.width,
            px(free * resizable::CONTENT_GROW / total),
        );

        // And a zero factor collapses its panel outright, because the basis is
        // zero and there is nothing else to size it from. **This is a state the
        // reference really has**: the shell's sidebar is `collapsible` with
        // `collapsedSize={0}`, so a collapsed sidebar is `--grow 0,100`.
        let collapsed = measure(cx, cell(&["--width", "600", "--grow", "0,100"]));
        assert_px(at(&collapsed, SIDEBAR).size.width, px(0.0));
        assert_px(at(&collapsed, CONTENT).size.width, px(free));
        // Both extractors call a zero-area box invisible — `oracleIsVisible`
        // requires `width > 0 && height > 0` and `crowbar-driver`'s `is_visible`
        // requires a non-empty intersection with the clip — so this cell compares
        // rather than diverging on a field neither side agreed the meaning of.
        assert!(!find(&collapsed, SIDEBAR).visible);
        assert!(find(&collapsed, CONTENT).visible);
    }

    /// **taffy rounds a panel's width to a whole logical pixel; `WebKit` does
    /// not.** Recorded as a measurement rather than as prose, because it is the
    /// systematic difference every parity run on this surface will meet.
    ///
    /// At 600px with the live factors the CSS arithmetic is **146.8748**, and taffy
    /// lays the panel out at exactly **147** — a *round*, so the error is bounded
    /// by 0.5 in both directions and `native/oracle/ANCHORS.md` §5's ±0.5 already
    /// covers it. It is not the `content_sized` case: that is a one-directional
    /// `ceil` on a text run, which is why v1.5 had to correct for it rather than
    /// tolerate it.
    ///
    /// **The caution it does buy**, said here so a later reader does not have to
    /// re-derive it: a share that lands on a half pixel is Δ 0.5 exactly, right at
    /// the tolerance edge, and would read as a defect. Picking `--width` and
    /// `--grow` so the arithmetic lands near a whole pixel is how a matrix cell
    /// avoids reporting quantisation.
    #[gpui::test]
    fn taffy_rounds_a_panels_width_where_webkit_would_keep_the_fraction(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let records = measure(cx, cell(&["--width", "600", "--viewport-width", "700"]));

        let free = 600.0 - f32::from(resizable::HANDLE_THICKNESS);
        let total = resizable::SIDEBAR_GROW + resizable::CONTENT_GROW;
        let exact = free * resizable::SIDEBAR_GROW / total;

        // The CSS value is fractional…
        assert!((exact - 146.8748).abs() < 0.001, "{exact}");
        assert!(exact.fract() > 0.001, "{exact}");
        // …and taffy's is not.
        let measured = at(&records, SIDEBAR).size.width;
        assert_px(measured, px(147.0));
        assert!(f32::from(measured).fract() < f32::EPSILON, "{measured:?}");
        // Inside the contract's tolerance, and a round rather than a ceil: the
        // native side came out *above* here and must be free to come out below.
        assert!((f32::from(measured) - exact).abs() <= 0.5, "{measured:?}");
    }

    /// **The separator does not grow and does not shrink**, which is what keeps it
    /// one pixel at every width — and is the property `flex: 0 0 auto` buys.
    #[gpui::test]
    fn the_separator_stays_one_pixel_at_every_width(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);

        for width in ["320", "600", "1100"] {
            let records = measure(cx, cell(&["--width", width, "--viewport-width", "1200"]));
            let handle = find(&records, HANDLE);

            assert_px(handle.bounds.size.width, px(1.0));
            // It paints nothing: the visible divider a user sees is the `::after`
            // hit strip on hover, and at rest the separator is a transparent
            // 1px column. `border.w` is the field `ANCHORS.md` v1.1 compares
            // exactly, and `focus-visible:ring-1` is a shadow rather than a
            // border, so it is zero in every cell.
            assert_eq!(handle.background, Paint::None);
            assert_px(handle.border_width, px(0.0));
            assert_px(handle.radius, px(0.0));
        }
    }

    /// **A vertical group swaps the axes**, including the separator's: its
    /// `aria-orientation` is the *opposite* of the group's, which is why the
    /// `aria-[orientation=horizontal]:h-px w-full` rules are the ones a vertical
    /// group gets.
    ///
    /// No live call site asks for it, so this measures the port against its own
    /// arithmetic rather than against a reference — said here so nobody reads it
    /// as a parity result.
    #[gpui::test]
    fn a_vertical_group_divides_the_height_instead(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let records = measure(
            cx,
            cell(&[
                "--orientation",
                "vertical",
                "--width",
                "600",
                "--shell-height",
                "121",
                "--grow",
                "50,50",
            ]),
        );

        let group = at(&records, "resize-group");
        let sidebar = at(&records, SIDEBAR);
        let handle = at(&records, HANDLE);
        let content = at(&records, CONTENT);

        // 121 − 1 = 120, half each.
        assert_px(group.size.height, px(121.0));
        assert_px(sidebar.size.height, px(60.0));
        assert_px(content.size.height, px(60.0));
        assert_px(handle.size.height, px(1.0));

        // The separator is now a full-width rule, and the panels stack.
        assert_px(handle.size.width, group.size.width);
        assert_px(sidebar.origin.y, px(0.0));
        assert_px(handle.origin.y, sidebar.size.height);
        assert_px(content.origin.y, handle.origin.y + px(1.0));
        for id in [SIDEBAR, HANDLE, CONTENT] {
            assert_px(at(&records, id).origin.x, px(0.0));
            assert_px(at(&records, id).size.width, group.size.width);
        }
    }

    /// **The grip is the only paint on this surface**, and it is the only reason
    /// the theme axis is not entirely vacuous. `rounded-lg` is 10px here because
    /// this project redefines `--radius-lg`, on a box 4px wide.
    ///
    /// It has **no reference**: no live call site passes `withHandle`, so this
    /// measures the port against its own arithmetic. Kept for the same reason
    /// Phase 1 kept `git-row-dir` — the day a call site passes the prop, the cell
    /// is already drawable.
    #[gpui::test]
    fn the_grip_is_a_four_by_twenty_four_pill_in_the_border_colour(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let records = measure(cx, cell(&["--with-handle", "--shell-height", "120"]));
        let grip = find(&records, "resize-handle-grip");
        let handle = at(&records, HANDLE);
        let grip_bounds = at(&records, "resize-handle-grip");

        assert_px(grip.bounds.size.width, resizable::GRIP_THICKNESS);
        assert_px(grip.bounds.size.height, resizable::GRIP_LENGTH);
        assert_px(grip.bounds.size.width, px(4.0));
        assert_px(grip.bounds.size.height, px(24.0));
        assert_px(grip.radius, Theme::DARK.radius_lg.value());
        assert_px(grip.radius, px(10.0));
        assert_eq!(grip.background, Paint::Solid(Theme::DARK.border.value()));

        // `items-center justify-center` on a 1px-wide separator: the grip
        // overflows it symmetrically, 1.5px either side, and is centred on the
        // group's cross axis.
        assert_px(grip_bounds.origin.x, handle.origin.x + px(0.5) - px(2.0));
        assert_px(
            grip_bounds.origin.y + grip_bounds.size.height / 2.0,
            handle.origin.y + handle.size.height / 2.0,
        );
    }

    /// **Nothing on this surface paints anything, unless the grip is asked for.**
    /// Measured rather than asserted from the source, because it is the claim the
    /// whole `--theme` axis rests on: three anchors, both themes, no background,
    /// no border, no radius and no text.
    #[gpui::test]
    fn without_the_grip_the_surface_has_no_colour_at_all(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);

        for theme in ["dark", "light"] {
            let records = measure(cx, cell(&["--theme", theme]));
            for id in ["resize-group", SIDEBAR, HANDLE, CONTENT] {
                let anchor = find(&records, id);
                assert_eq!(anchor.background, Paint::None, "{id} in {theme}");
                assert_px(anchor.border_width, px(0.0));
                assert_px(anchor.radius, px(0.0));
                assert!(anchor.text.is_none(), "{id} in {theme}");
                assert!(anchor.visible, "{id} in {theme}");
                // Which is also why both declaration lists are empty: v1.6
                // requires a `font` on any anchor that declares it.
                assert!(!anchor.content_sized && !anchor.line_sized, "{id}");
            }
        }

        assert!(resizable::CONTENT_SIZED.is_empty());
        assert!(resizable::LINE_SIZED.is_empty());
    }

    /// **The two interaction flags move no compared field**, which is this
    /// surface's headline and is measured rather than argued. `hover` paints the
    /// `::after`, which carries no anchor; `focus` paints a ring, which is a
    /// box-shadow `ANCHORS.md` §6 has no field for. Every anchored record is
    /// byte-identical to the resting one in both cells.
    #[gpui::test]
    fn neither_interaction_flag_moves_a_field_the_differ_can_see(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let resting = measure(cx, cell(&[]));

        for flags in ["hover", "focus", "hover,focus"] {
            let driven = measure(cx, cell(&["--flags", flags]));
            for id in ["resize-group", SIDEBAR, HANDLE, CONTENT] {
                let before = find(&resting, id);
                let after = find(&driven, id);
                assert_eq!(before.bounds, after.bounds, "{id} under {flags}");
                assert_eq!(before.background, after.background, "{id} under {flags}");
                assert_eq!(before.radius, after.radius, "{id} under {flags}");
                assert_eq!(
                    before.border_width, after.border_width,
                    "{id} under {flags}",
                );
                assert_eq!(before.visible, after.visible, "{id} under {flags}");
            }
        }
    }

    /// Light and dark are the same layout, and — without the grip — the same
    /// palette too, because there is no palette. A geometry delta between the two
    /// themes would be a bug in the component; a *colour* delta only exists once
    /// `--with-handle` puts one on screen.
    #[gpui::test]
    fn the_theme_changes_only_the_grip(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let dark = measure(cx, cell(&["--theme", "dark", "--with-handle"]));
        let light = measure(cx, cell(&["--theme", "light", "--with-handle"]));

        for id in [
            "resize-group",
            SIDEBAR,
            HANDLE,
            CONTENT,
            "resize-handle-grip",
        ] {
            assert_eq!(at(&dark, id), at(&light, id), "{id}");
        }
        assert_eq!(
            find(&dark, "resize-handle-grip").background,
            Paint::Solid(Theme::DARK.border.value()),
        );
        assert_eq!(
            find(&light, "resize-handle-grip").background,
            Paint::Solid(Theme::LIGHT.border.value()),
        );
        assert_ne!(
            find(&dark, "resize-handle-grip").background,
            find(&light, "resize-handle-grip").background,
        );
    }
}

/// **The driver's window follows the surface** — P2.5, measured.
///
/// Every other module here measures a component. This one measures the *driver*:
/// whether the window it opens is big enough for the picture it was asked to
/// draw, and what happens when it is not.
///
/// The item exists because it was not. `crowbar-app` capped `--shell-height` at
/// `1..=160` on `resizable` and `--height` at `1..=640` on `sidebar-carousel`,
/// on grounds that were correct — a surface cut by the driver's own window makes
/// every `visible` in the snapshot an artefact of the window size, which is
/// worse than refusing — but applied to the wrong side of the equation. The live
/// IDE shell's `ResizablePanelGroup` is **1119px** tall, so the cap made the
/// only real reference unreachable and `resizable` could not be parity-tested at
/// all.
///
/// So the window moves. What the caps protected is kept twice over, and both
/// halves are measured below: the surface is **cut and never squashed** when a
/// window comes back short, and a frame with a cut anchor in it is **refused
/// rather than emitted**.
mod window {
    use super::{a_cell, assert_px, find, ids};
    use crowbar_driver::{AnchorRegistry, RawAnchor};
    use crowbar_ui::components::resizable;
    use crowbar_ui::components::sidebar_carousel::{ID_SCROLLPORT, SidebarTab, TABS};
    use gpui::{Pixels, Size, TestAppContext, px, size};

    use crate::driver_anchors::{DriverAnchors, fold_text_halves};
    use crate::row_snapshot::{Destination, emit};
    use crate::row_surface::{Cell, INSET_Y, RowSurface};

    /// The live IDE shell's own measured height, in logical px.
    ///
    /// The number this whole item is about. It came off the running app —
    /// `web/src/components/layout/ide-shell.tsx`'s `ResizablePanelGroup` in its
    /// `h-screen` box — which is why it is a constant here rather than a round
    /// number chosen to be large.
    const LIVE_SHELL_HEIGHT: u16 = 1119;

    /// Lays a cell out through **`RowSurface` itself** — the view the binary
    /// opens, caption and `size_full` root and all — in a window of the given
    /// size, and hands back the registry as well as the records.
    ///
    /// Not `super::measure`, and the difference is the point of this module.
    /// `Stage` is an auto-sized root, so nothing about a window too short for
    /// its content is visible through it. `RowSurface`'s root is `size_full`,
    /// which makes it a flex container with the window's *definite* height — and
    /// that is where a surface can be quietly compressed instead of cut.
    fn draw(
        cx: &mut TestAppContext,
        cell: &Cell,
        window: Size<Pixels>,
    ) -> (AnchorRegistry, Vec<RawAnchor>) {
        let anchors: AnchorRegistry = cx.update(crowbar_driver::install);
        let caption = cell.describe();
        let subject = cell.clone();
        let _window = cx.open_window(window, |_, _| {
            RowSurface::new(subject, Box::new(DriverAnchors), caption)
        });
        cx.run_until_parked();
        let records = fold_text_halves(anchors.records());
        (anchors, records)
    }

    /// The window the app would open for this cell.
    fn window_for(cell: &Cell) -> Size<Pixels> {
        RowSurface::window_size(cell)
    }

    /// **The live IDE shell's height, drawn unclipped.**
    ///
    /// The headline assertion of P2.5: at `--shell-height 1119` — the height the
    /// old cap of 160 refused — every anchor on the surface reports
    /// `visible: true` and carries the geometry the CSS arithmetic asks for. Not
    /// "the parser permitted it": permitted-but-clipped is precisely the outcome
    /// the cap existed to prevent, and it would show up here as a `visible` that
    /// went false or a height that came back short.
    #[gpui::test]
    fn the_live_shell_height_is_drawn_with_every_anchor_visible(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let cell = a_cell(&[
            "--surface",
            "resizable",
            "--width",
            "600",
            "--viewport-width",
            "700",
            "--shell-height",
            "1119",
        ]);

        // The window grew to hold it, rather than the surface being held down to
        // the window: 16px inset, the group, the caption, 16px inset.
        let window = window_for(&cell);
        assert_px(window.height, px(INSET_Y * 2.0 + 1119.0 + 29.0));
        assert!(window.height > px(f32::from(LIVE_SHELL_HEIGHT)));

        let (_anchors, records) = draw(cx, &cell, window);
        let group = find(&records, resizable::ID_GROUP);

        // Every anchor visible — the field the cap was protecting.
        for id in ids(&records) {
            assert!(find(&records, &id).visible, "{id} is not visible");
        }
        assert_eq!(ids(&records).len(), 4, "{:?}", ids(&records));

        // …and genuinely 1119 tall, top to bottom, inside the window.
        assert_px(group.bounds.size.height, px(1119.0));
        assert_px(group.bounds.origin.y, px(INSET_Y));
        assert_px(group.bounds.origin.y + group.bounds.size.height, px(1135.0));
        assert!(px(1135.0) <= window.height);

        // The two panels and the separator stretch the group's full height, so
        // the height reaches every anchor and not only the root.
        for id in [
            "resize-panel-sidebar",
            "resize-handle",
            "resize-panel-content",
        ] {
            let anchor = find(&records, id);
            assert_px(anchor.bounds.size.height, px(1119.0));
            assert_px(anchor.bounds.origin.y, px(INSET_Y));
            assert!(anchor.visible, "{id}");
        }
    }

    /// **The other surface, at the same height.** `sidebar-carousel` was capped
    /// at 640 for the same reason and is freed by the same change — including
    /// the one anchor that is legitimately invisible here, so this is not "every
    /// anchor is visible" passing because the driver forgot how to say false.
    #[gpui::test]
    fn the_carousel_is_drawn_at_the_same_height_unclipped(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let cell = a_cell(&[
            "--surface",
            "sidebar-carousel",
            "--width",
            "294",
            "--viewport-width",
            "600",
            "--height",
            "1119",
        ]);

        let window = window_for(&cell);
        assert_px(window.height, px(INSET_Y * 2.0 + 1119.0 + 29.0));

        let (_anchors, records) = draw(cx, &cell, window);

        // The scrollport and all four panels are the full 1119 tall, none of
        // them cut, and — at the resting track — every one of them visible.
        for id in [ID_SCROLLPORT]
            .into_iter()
            .chain(TABS.into_iter().map(SidebarTab::anchor))
        {
            let anchor = find(&records, id);
            assert_px(anchor.bounds.size.height, px(1119.0));
            assert_px(anchor.bounds.origin.y, px(INSET_Y));
        }
        assert!(find(&records, ID_SCROLLPORT).visible);
        assert!(find(&records, SidebarTab::Workspaces.anchor()).visible);

        // …and the three scrolled out of the scrollport horizontally are still
        // reported invisible, which is what proves the assertion above is
        // reading a live field rather than a constant.
        for tab in [SidebarTab::Chats, SidebarTab::Files, SidebarTab::Git] {
            assert!(!find(&records, tab.anchor()).visible, "{}", tab.name());
        }
    }

    /// **A cut frame is refused, not written.**
    ///
    /// The property the old caps were protecting, kept where it can actually be
    /// observed. `emit` takes the drawable area the platform *granted* and
    /// checks the frame it drew against it, so a snapshot whose `visible` fields
    /// would be artefacts of the window size is never produced — and the
    /// refusal names the anchor, both numbers and what it would have cost.
    ///
    /// The first two assertions are the **precondition**, and they are stated
    /// rather than assumed: the refusal works by finding an anchor outside the
    /// window, so it has teeth only if a short window *cuts* the surface instead
    /// of compressing it into itself. A squashed surface would report a full set
    /// of plausible bounds that are the wrong picture and leave nothing outside
    /// the window at all. They are here rather than in a test of their own
    /// because no mutation of this crate can make them fail — see the note in
    /// `RowSurface::render` — and a test nothing can break is not a test.
    #[gpui::test]
    fn a_cut_surface_is_refused_and_nothing_is_written(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let cell = a_cell(&[
            "--surface",
            "resizable",
            "--width",
            "600",
            "--viewport-width",
            "700",
            "--shell-height",
            "1119",
        ]);

        // A window a third of what the cell asked for — which is what macOS
        // hands back on a display that cannot hold it.
        let short = size(window_for(&cell).width, px(400.0));
        let (anchors, records) = draw(cx, &cell, short);

        let group = find(&records, resizable::ID_GROUP);
        assert_px(group.bounds.size.height, px(1119.0));
        assert!(
            group.bounds.origin.y + group.bounds.size.height > short.height,
            "the group must reach past the window, not be squeezed inside it",
        );

        // A path that does not exist, so "nothing was written" is checkable
        // without a temporary file to clean up.
        let refused = std::env::temp_dir().join("crowbar-p25-refused-must-not-exist.json");
        let _ = std::fs::remove_file(&refused);

        let outcome = emit(&anchors, &cell, &Destination::File(refused.clone()), short);

        let Err(complaint) = outcome else {
            panic!("a surface cut by the window must not be emitted");
        };
        assert!(complaint.contains("resize-group"), "{complaint}");
        assert!(complaint.contains("artefact"), "{complaint}");
        assert!(complaint.contains("400"), "{complaint}");
        assert!(complaint.contains("1135"), "{complaint}");
        // And the number to act on: the tallest surface this window *would*
        // have held, which is the window less the inset the surface starts at.
        // Without it a reader has to bisect `--shell-height` by hand.
        assert!(complaint.contains("384"), "{complaint}");
        assert!(!refused.exists(), "a refused emit wrote {refused:?}");
    }

    /// **The control the refusal above needs.** Without it, "the emit failed"
    /// would also pass on an `emit` that had stopped working altogether — which
    /// is the shape of vacuous guard that has already cost this project a
    /// batch of tests. The identical cell in the window it asked for emits, and
    /// the file is there.
    #[gpui::test]
    fn the_same_cell_in_the_window_it_asked_for_emits(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let cell = a_cell(&[
            "--surface",
            "resizable",
            "--width",
            "600",
            "--viewport-width",
            "700",
            "--shell-height",
            "1119",
        ]);

        let window = window_for(&cell);
        let (anchors, _records) = draw(cx, &cell, window);

        let written = std::env::temp_dir().join("crowbar-p25-emitted.json");
        let _ = std::fs::remove_file(&written);

        let path = emit(&anchors, &cell, &Destination::File(written.clone()), window)
            .expect("a surface the window holds is emitted");
        assert_eq!(path, written);

        let json = std::fs::read_to_string(&written).expect("the snapshot is on disk");
        assert!(json.contains("\"resize-group\""), "{json}");
        // And every anchor in it says `visible`, which is the whole reason the
        // window had to grow.
        assert!(!json.contains("\"visible\": false"), "{json}");
        let _ = std::fs::remove_file(&written);
    }

    /// **The two Phase 1 surfaces' window is the one their archived runs were
    /// taken at**, to the pixel — because neither drives a height, so the floor
    /// is the whole arithmetic and `window_extent` cannot move it.
    ///
    /// 24 snapshot pairs in `native/oracle/runs/` were measured at this
    /// geometry. A change here is a re-baselining, and re-baselining archived
    /// evidence is not a thing this port does.
    #[gpui::test]
    fn the_phase_one_surfaces_keep_the_window_they_were_measured_in(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);

        for surface in ["git-status-row", "file-tree-row"] {
            let cell = a_cell(&["--surface", surface]);
            assert_eq!(cell.window_extent(), 72, "{surface}");
            assert_px(window_for(&cell).height, px(104.0));

            // And what they emit is untouched by the window having become a
            // computed quantity: the same records, anchor for anchor, as in the
            // 400px window the harness used before P2.5.
            let (_, now) = draw(cx, &cell, window_for(&cell));
            let (_, before) = draw(cx, &cell, size(cell.viewport_width_px(), px(400.0)));
            assert_eq!(now, before, "{surface}");
        }
    }
}
