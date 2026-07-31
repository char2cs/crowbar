//! The **state** gate: `--surface file-tree-row`.
//!
//! A module rather than a second file, because it shares the harness above and a
//! second copy of `measure` is a second thing to drift. What it does not share is
//! two numbers: this row's indent step is **16** and its line box is **20**,
//! where the git row's are 14 and 18.9.

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
