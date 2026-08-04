//! `--surface button` — what taffy resolves the ten sizes and the seven
//! variants to, measured in a real window.
//!
//! The extractor sees **two** anchors on this surface and no more: the button
//! itself, and the loading indicator when `--loading` renders one. Everything
//! else the component paints — the `::before` overlay and the leading glyph —
//! is unanchorable on the *reference* side, so anchoring it here would put a
//! record in the snapshot that the DOM extractor can never produce. That is not
//! a thin surface by accident; it is what `button.tsx` can carry.

use super::{a_cell, assert_px, assert_within_tolerance, find, ids, measure, relative_to};
use crowbar_driver::{Paint, RawAnchor};
use crowbar_ui::Theme;
use crowbar_ui::primitives::button;
use crowbar_ui::primitives::button::{
    ALL_RADIUS_CLASSES, ALL_SIZES, ALL_VARIANTS, ButtonState, RadiusClass, Size,
};
use gpui::{Bounds, Pixels, TestAppContext, px};

use crate::row_surface::Cell;

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "button"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's root, which is the button itself —
/// so this is `(0, 0)` for the root and an offset inside it for anything
/// else.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, button::ID_BUTTON, id)
}

/// **The default cell is the reference's own button**, measured: 28×28 at
/// the origin of its own snapshot, with nothing else in the frame.
///
/// 28 is `sm:size-7` — `icon-sm`, not `icon` — and it is the number the live
/// element at `x 1140, y 153` reports.
#[gpui::test]
fn the_default_cell_is_a_twenty_eight_pixel_square(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let seen = ids(&records);

    assert_eq!(seen, vec!["button".to_owned()]);
    let subject = at(&records, "button");
    assert_px(subject.origin.x, px(0.0));
    assert_px(subject.origin.y, px(0.0));
    assert_px(subject.size.width, px(28.0));
    assert_px(subject.size.height, px(28.0));

    // And it is drawn at the ordinary inset, so the root-relative
    // subtraction in the snapshot is doing work on both axes.
    assert_px(find(&records, "button").bounds.origin.x, px(24.0));

    // `rounded-sm`, from the tab bar call site the default reproduces —
    // **not** the `icon-sm` variant's own `rounded-lg`. This is the field
    // the first gate run came back one delta on.
    assert_px(find(&records, "button").radius, px(6.0));
}

/// **A call site's `rounded-*` reaches the extractor**, and `none` gives the
/// size variant's own back.
///
/// Read off the recorded anchor rather than off `Button::radius`, because
/// what the differ compares is what the extractor saw: a class that never
/// reached the style would still leave the component's own arithmetic right.
#[gpui::test]
fn a_call_site_radius_class_reaches_the_extractor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let theme = Theme::DARK;

    for class in ALL_RADIUS_CLASSES {
        let records = measure(cx, cell(&["--class-radius", class.name()]));
        assert_px(find(&records, "button").radius, class.value(&theme));
    }

    // The three are three different pictures, or the option would be
    // decoration.
    let seen: Vec<f32> = ALL_RADIUS_CLASSES
        .into_iter()
        .map(|class| {
            let records = measure(cx, cell(&["--class-radius", class.name()]));
            f32::from(find(&records, "button").radius)
        })
        .collect();
    assert_eq!(seen, vec![6.0, 8.0, 10.0]);

    // `none` is the size variant's own, on both sides of the `rounded-md`
    // split: `icon-sm` keeps 10 and `icon-xs` keeps 8.
    for (size, expected) in [(Size::IconSm, 10.0), (Size::IconXs, 8.0)] {
        let records = measure(cx, cell(&["--class-radius", "none", "--size", size.name()]));
        assert_px(find(&records, "button").radius, px(expected));
        assert_px(find(&records, "button").radius, size.radius(&theme));
    }

    // And the class beats the variant in the direction that would otherwise
    // look like agreement: `icon-xs`'s own is 8, the workspace header's
    // call site writes `rounded-lg`, and 10 is what paints.
    let header = measure(cx, cell(&["--size", "icon-xs", "--class-radius", "lg"]));
    assert_px(find(&header, "button").radius, px(10.0));
    assert_ne!(find(&header, "button").radius, Size::IconXs.radius(&theme),);
    assert_eq!(RadiusClass::Lg.value(&theme), px(10.0));
}

/// **`border.w` is 1 on every variant** — the field `ANCHORS.md` v1.1
/// compares exactly, and the one this component is most likely to be ported
/// without.
///
/// Read off the extractor rather than off the constant, because what the
/// differ sees is what the extractor recorded: a `.border_1()` that never
/// reached the style would still leave `BORDER_WIDTH` equal to 1.
#[gpui::test]
fn every_variant_reports_a_one_pixel_border_to_the_extractor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for variant in ALL_VARIANTS {
        let records = measure(cx, cell(&["--variant", variant.name()]));
        let subject = find(&records, "button");

        assert_px(subject.border_width, px(1.0));
        // A colourless border serialises as `#00000000`, which is what
        // `border-transparent` gives on the DOM side — so the three
        // transparent variants agree on both fields rather than on neither.
        assert!(subject.border_color.is_some(), "{}", variant.name());

        // The pixel is not free: it comes out of the box, so the padding box
        // is two pixels smaller than the border box on both axes.
        assert_px(subject.bounds.size.width, px(28.0));
    }
}

/// Every size's box, at both breakpoints, against the compiled
/// `calc(var(--spacing) * n)`.
///
/// This is the surface's strongest axis: the `sm:` variant takes exactly one
/// 4px step off every one of the ten sizes, and a media query is the one
/// thing gpui has no equivalent for at all.
#[gpui::test]
fn every_size_resolves_to_its_compiled_box(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let theme = Theme::DARK;

    for size in ALL_SIZES {
        for (viewport, breakpoint) in [
            ("800", crowbar_ui::surfaces::rows::Breakpoint::Sm),
            ("600", crowbar_ui::surfaces::rows::Breakpoint::Base),
        ] {
            // `--class-radius none` because this test is about the size
            // *variant's* own `rounded-*`; the default cell carries the tab
            // bar's `rounded-sm` over the top of it.
            let subject = cell(&[
                "--size",
                size.name(),
                "--viewport-width",
                viewport,
                "--width",
                "300",
                "--class-radius",
                "none",
            ]);
            let records = measure(cx, subject);
            let box_ = at(&records, "button");
            let expected = size.extent(breakpoint);

            assert_px(box_.size.height, expected);
            if size.square() {
                assert_px(box_.size.width, expected);
            } else {
                // A text size with no label and one glyph: the width is the
                // glyph's margin box — the icon less its two `-mx-0.5`
                // margins — plus the horizontal padding on each side plus
                // the two border pixels. Nothing here is authored, which is
                // what makes it a division taffy performs rather than a
                // number the port wrote down.
                assert_px(
                    box_.size.width,
                    size.glyph_box(breakpoint) + size.padding_x() * 2.0 + px(2.0),
                );
            }
            assert_px(find(&records, "button").radius, size.radius(&theme));
        }
    }
}

/// A labelled button is **shrink-to-fit**, which is what the flex-row host
/// above the root anchor exists for — and the arithmetic is the flex line's,
/// not the port's.
///
/// `width = 2 border + 2 padding + the glyph's 12px outer box + the gap +
/// the shaped run`. The run's own box is `ceil`ed by gpui
/// (`ANCHORS.md` v1.5), so the comparison is against `ceil(text_width)`.
#[gpui::test]
fn a_labelled_button_is_its_contents_wide(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for (word, size) in [
        ("short", Size::Default),
        ("normal", Size::Default),
        ("overflow", Size::Default),
        ("normal", Size::Sm),
        ("normal", Size::Xs),
    ] {
        let records = measure(
            cx,
            cell(&["--label", "--content", word, "--size", size.name()]),
        );
        let subject = find(&records, "button");
        let text = subject.text.clone().expect("a labelled button paints text");

        // Never clipped: `whitespace-nowrap` with no `overflow-hidden` and
        // `shrink-0` means a long label widens the button instead.
        assert!(!text.clipped, "{word} {}", size.name());

        let run = f32::from(text.width).ceil();
        assert_within_tolerance(
            subject.bounds.size.width,
            px(2.0)
                + size.padding_x() * 2.0
                + size.glyph_box(crowbar_ui::surfaces::rows::Breakpoint::Sm)
                + size.gap()
                + px(run),
        );
    }

    // And the three content lengths are three different pictures, which is
    // what makes the content axis real once `--label` is on.
    let widths: Vec<f32> = ["short", "normal", "overflow"]
        .into_iter()
        .map(|word| {
            let records = measure(
                cx,
                cell(&["--label", "--size", "default", "--content", word]),
            );
            f32::from(find(&records, "button").bounds.size.width)
        })
        .collect();
    assert!(widths[0] < widths[1] && widths[1] < widths[2], "{widths:?}");
}

/// The label's text facts, which are the half of the contract an icon-only
/// button cannot exercise at all.
///
/// `font.family` has to be named explicitly on the native side
/// (`ANCHORS.md` v1.2 #5), and the name that reaches the record is the
/// surface's own `font_family`, so this is where a theme whose stack changed
/// would show up.
#[gpui::test]
fn the_label_carries_the_type_the_class_list_compiles_to(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let theme = Theme::DARK;

    let records = measure(cx, cell(&["--label"]));
    let text = find(&records, "button")
        .text
        .expect("a labelled button paints text");

    assert_eq!(text.content, "Create workspace");
    // `sm:text-sm` — 14px on a 20px line, at the default 800px viewport.
    assert_px(text.font.size, px(14.0));
    assert_within_tolerance(text.font.line_height, px(20.0));
    // `font-medium` is 500, and `font.weight` is compared *exactly*.
    assert!((text.font.weight - 500.0).abs() < f32::EPSILON);
    assert_eq!(text.font.family, "CalSansUI");
    // `ghost` is `text-foreground`.
    assert_eq!(text.color, theme.foreground.value());

    // Below the breakpoint the base list's own `text-base` wins: 16 on 24.
    let base = measure(
        cx,
        cell(&["--label", "--viewport-width", "600", "--width", "300"]),
    );
    let wide = find(&base, "button")
        .text
        .expect("a labelled button paints text");
    assert_px(wide.font.size, px(16.0));
    assert_within_tolerance(wide.font.line_height, px(24.0));
}

/// **`data-loading:text-transparent` is the only state the differ can see**,
/// and the spinner is the only anchor a state adds.
#[gpui::test]
fn loading_adds_the_indicator_and_empties_the_labels_colour(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let records = measure(cx, cell(&["--label", "--loading"]));
    let seen = ids(&records);
    assert!(
        seen.contains(&"button-loading-indicator".to_owned()),
        "{seen:?}"
    );

    let text = find(&records, "button")
        .text
        .expect("a labelled button paints text");
    // The string is still there — only its colour went.
    assert_eq!(text.content, "Create workspace");
    assert!(
        (text.color.a - 0.0).abs() < f32::EPSILON,
        "{:?}",
        text.color
    );

    // And a resting button of the same shape paints an opaque label, so the
    // cell is a difference rather than a coincidence.
    let resting = measure(cx, cell(&["--label"]));
    let opaque = find(&resting, "button")
        .text
        .expect("a labelled button paints text");
    assert!((opaque.color.a - 1.0).abs() < f32::EPSILON);
    assert!(!ids(&resting).contains(&"button-loading-indicator".to_owned()));
}

/// The spinner is `position: absolute` with **no inset**, so it takes its
/// static position from the flex container's own alignment — centred, by
/// `items-center` and `justify-center`.
///
/// Measured rather than asserted from CSS: gpui and `WebKit` each have to
/// apply the container's alignment to an auto inset, and `dropdown-menu`
/// made the same measurement for its `right-2` tick on the other axis.
#[gpui::test]
fn the_spinner_is_centred_by_the_containers_alignment(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let records = measure(cx, cell(&["--loading"]));
    let host = at(&records, "button");
    let spinner = at(&records, "button-loading-indicator");

    // A 16px glyph — `sm:size-4` on an `icon-sm` button.
    assert_px(spinner.size.width, px(16.0));
    assert_px(spinner.size.height, px(16.0));

    // The margins are symmetric, so the border box's centre is the margin
    // box's centre and both land on the host's. The live reference puts its
    // `<svg>` at `x 6` in a 28px button, which is this arithmetic exactly:
    // 1px border + a centred 12px margin box in 26px of content is 8, less
    // the 2px margin.
    let host_centre_x = f32::from(host.size.width) / 2.0;
    let spinner_centre_x = f32::from(spinner.origin.x) + f32::from(spinner.size.width) / 2.0;
    assert!(
        (spinner_centre_x - host_centre_x).abs() <= 0.5,
        "{spinner_centre_x} against {host_centre_x}",
    );
    assert_px(spinner.origin.x, px(6.0));
    assert_px(spinner.origin.y, px(6.0));
    let host_centre_y = f32::from(host.size.height) / 2.0;
    let spinner_centre_y = f32::from(spinner.origin.y) + f32::from(spinner.size.height) / 2.0;
    assert!(
        (spinner_centre_y - host_centre_y).abs() <= 0.5,
        "{spinner_centre_y} against {host_centre_y}",
    );
}

/// **A disabled button is still `visible`.** `opacity-64` is 0.64, and
/// v1.7's opacity term fires only at zero — so the two extractors agree, and
/// the 36% they cannot report is a difference neither side can see.
#[gpui::test]
fn a_disabled_button_is_still_visible_to_both_extractors(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for line in [vec!["--disabled"], vec!["--loading"], vec![]] {
        let records = measure(cx, cell(&line));
        assert!(find(&records, "button").visible, "{line:?}");
    }
}

/// Every variant's resting background, as the extractor records it — and the
/// two that paint none.
#[gpui::test]
fn the_variants_paint_the_backgrounds_their_class_lists_name(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let theme = Theme::DARK;

    for variant in ALL_VARIANTS {
        let records = measure(cx, cell(&["--variant", variant.name()]));
        let painted = find(&records, "button").background;

        match variant.background(&theme, ButtonState::default()) {
            Some(colour) => {
                assert_eq!(painted, Paint::Solid(colour.value()), "{}", variant.name());
            }
            // `ghost` and `link` at rest: no background at all, which the
            // DOM reports as `rgba(0, 0, 0, 0)` and the driver as `None`,
            // and both serialise to `#00000000`.
            None => assert_eq!(painted, Paint::None, "{}", variant.name()),
        }
    }
}

/// A bare flex row with one fixed-size item carrying an inline margin, and
/// nothing else — the smallest tree that exhibits the taffy defect
/// `crowbar_ui::primitives::button::ICON_MARGIN_X` describes.
///
/// At module scope rather than inside its test, because the two gates
/// disagree about where an item may go: `check-invariants.sh` rule 6 wants
/// `leak_checked!` to be the test's **first statement**, and clippy's
/// `items_after_statements` will not have a `struct` after one.
struct Margin(Pixels);

impl gpui::Render for Margin {
    fn render(
        &mut self,
        _window: &mut gpui::Window,
        _cx: &mut gpui::Context<Self>,
    ) -> impl gpui::IntoElement {
        use gpui::{ParentElement as _, Styled as _, div};
        div().child(
            div().flex().flex_row().child(crowbar_driver::anchor_root(
                "probe",
                div()
                    .flex()
                    .flex_shrink_0()
                    .border_1()
                    .child(div().flex_shrink_0().w(px(16.0)).h(px(16.0)).mx(self.0)),
            )),
        )
    }
}

/// **The defect that made `[&_svg]:-mx-0.5` unportable, pinned.**
///
/// A negative inline margin on an in-flow flex item breaks taffy's
/// content-based main-size resolution: the container collapses to its
/// padding box. This is the control for `Size::glyph_box`, built by hand so
/// that it measures gpui rather than this component — and so that a gpui
/// bump which *fixes* it fails here rather than being noticed years later.
///
/// A positive margin of the same size is exact, which is what makes it a
/// defect rather than a policy.
#[gpui::test]
fn a_negative_margin_still_collapses_a_content_sized_flex_container(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let measure_margin = |cx: &mut TestAppContext, margin: Pixels| {
        let anchors = cx.update(crowbar_driver::install);
        let _window = cx.open_window(gpui::size(px(400.0), px(120.0)), |_, _| Margin(margin));
        cx.run_until_parked();
        crowbar_driver::AnchorRegistry::records(&anchors)
            .into_iter()
            .find(|record| record.id == "probe")
            .expect("the probe is anchored")
            .bounds
            .size
            .width
    };

    // No margin: 16 of content inside two border pixels. CSS agrees.
    assert_px(measure_margin(cx, px(0.0)), px(18.0));
    // A positive margin is exact: 16 + 2 + 2 + 2 border.
    assert_px(measure_margin(cx, px(2.0)), px(22.0));
    // A negative one should be 16 - 2 - 2 + 2 = 14. It is **2** — the two
    // border pixels, with the whole content contribution gone.
    let negative = measure_margin(cx, px(-2.0));
    assert_px(negative, px(2.0));
    assert!(
        negative < px(14.0),
        "gpui now lays a negative margin out correctly ({negative:?}); \
         Size::glyph_box can go back to declaring it",
    );
}

/// **Nothing on this surface leaks another surface's anchors**, at any cell
/// this surface can be driven to. Two roots in one frame would make
/// `Snapshot::build` anchor to whichever it found first.
#[gpui::test]
fn no_cell_of_this_surface_records_a_foreign_anchor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for line in [
        vec!["--label"],
        vec!["--loading", "--label"],
        vec!["--variant", "outline", "--size", "xl"],
        vec!["--flags", "hover,focus,selected"],
    ] {
        let seen = ids(&measure(cx, cell(&line)));
        assert!(
            seen.iter()
                .all(|id| id == "button" || id == "button-loading-indicator"),
            "{line:?}: {seen:?}",
        );
    }
}
