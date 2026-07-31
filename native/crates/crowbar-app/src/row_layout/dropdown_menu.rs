//! The first Phase 2 surface: `--surface dropdown-menu`.
//!
//! A module rather than a second file, for the reason the one above is: it
//! shares `measure`, and a second copy of the harness is a second thing to
//! drift. What it does not share is a single number — this surface is a *column*
//! of boxes, where both Phase 1 surfaces are one row, so every assertion here is
//! about stacking and about the one thing a row never had: a **negative margin**.

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
fn a_tick_row_reserves_its_gutter_and_the_tick_appears_only_when_checked(cx: &mut TestAppContext) {
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
