//! `--surface repo-icon-popover`, laid out in a real window — plus
//! `crowbar_ui::components::repo_icon_popover::Trigger`, which carries no
//! `--surface` of its own (see that module's docs) and is driven directly
//! through the shared harness instead, the `sidebar_tab_bar.rs` shape.

use super::{a_cell, assert_px, find, ids, lay_out, measure, relative_to};
use crowbar_driver::RawAnchor;
use crowbar_ui::Theme;
use crowbar_ui::components::popover;
use crowbar_ui::components::repo_avatar::{ImageState, Kind};
use crowbar_ui::components::repo_icon_popover::{self, Trigger};
use crowbar_ui::ui_sans_font;
use gpui::{
    Bounds, Context, IntoElement, ParentElement as _, Pixels, Render, Styled as _, TestAppContext,
    Window, div, px, size,
};

use crate::driver_anchors::DriverAnchors;
use crate::row_surface::Cell;

// ─── the popup surface ────────────────────────────────────────────────────

/// A cell on this surface, with the selector already applied.
fn cell(args: &[&str]) -> Cell {
    let mut line = vec!["--surface", "repo-icon-popover"];
    line.extend_from_slice(args);
    a_cell(&line)
}

/// Bounds relative to *this* surface's own root.
fn at(records: &[RawAnchor], id: &str) -> Bounds<Pixels> {
    relative_to(records, repo_icon_popover::ID_POPUP, id)
}

/// **The default cell carries every unconditional contract anchor and
/// neither optional row.**
///
/// **Mutation:** deleting `PopupContent::render`'s `if self.show_emoji_input`
/// guard (always pushing the emoji row) turns the second assertion red —
/// `repo-icon-popover-emoji-submit` would then appear on the default cell.
#[gpui::test]
fn the_default_cell_carries_the_unconditional_anchors_and_neither_optional_row(
    cx: &mut TestAppContext,
) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&[])));

    for id in [
        repo_icon_popover::ID_POPUP,
        "avatar",
        "avatar-fallback",
        repo_icon_popover::ID_UPLOAD,
        repo_icon_popover::ID_EMOJI,
        repo_icon_popover::ID_GITHUB,
    ] {
        assert!(seen.contains(&id.to_owned()), "{id} missing from {seen:?}");
    }
    for id in [repo_icon_popover::ID_EMOJI_SUBMIT, repo_icon_popover::ID_RESET] {
        assert!(!seen.iter().any(|s| s == id), "{id} should not be on the default cell: {seen:?}");
    }
}

/// **`--emoji` mounts the emoji row's own submit button, and `--reset`
/// mounts the reset button — independently.**
#[gpui::test]
fn emoji_and_reset_each_add_their_own_button_independently(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let neither = ids(&measure(cx, cell(&[])));
    let emoji_only = ids(&measure(cx, cell(&["--emoji"])));
    let reset_only = ids(&measure(cx, cell(&["--reset"])));
    let both = ids(&measure(cx, cell(&["--emoji", "--reset"])));

    assert!(!neither.iter().any(|id| id == repo_icon_popover::ID_EMOJI_SUBMIT));
    assert!(!neither.iter().any(|id| id == repo_icon_popover::ID_RESET));

    assert!(emoji_only.iter().any(|id| id == repo_icon_popover::ID_EMOJI_SUBMIT));
    assert!(!emoji_only.iter().any(|id| id == repo_icon_popover::ID_RESET));

    assert!(!reset_only.iter().any(|id| id == repo_icon_popover::ID_EMOJI_SUBMIT));
    assert!(reset_only.iter().any(|id| id == repo_icon_popover::ID_RESET));

    assert!(both.iter().any(|id| id == repo_icon_popover::ID_EMOJI_SUBMIT));
    assert!(both.iter().any(|id| id == repo_icon_popover::ID_RESET));
}

/// **`--emoji` also mounts the shared `input.rs` primitive's own two ids**
/// (`input-control`/`input`) — the emoji field really is `<Input>` in the
/// source, hard-coded ids and all, not a bespoke box this port could leave
/// unanchored.
///
/// **Mutation:** replacing the two `anchors.boxed(AnchorId::from(super::
/// input::ID_CONTROL), …)`/`ID_FIELD` calls in `PopupContent::emoji_row`
/// with a plain, unanchored `div()` turns this red — the live React DOM
/// would then carry `input-control`/`input` on a cell this port's own
/// capture does not.
#[gpui::test]
fn emoji_mounts_the_shared_input_primitives_own_ids(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let seen = ids(&measure(cx, cell(&["--emoji"])));
    assert!(seen.iter().any(|id| id == "input-control"), "{seen:?}");
    assert!(seen.iter().any(|id| id == "input"), "{seen:?}");
}

/// **`--preview image` swaps `avatar-fallback` for `avatar-image`, and only
/// one of the two ever exists at once** — `base-ui`'s own arrangement,
/// reproduced.
///
/// **Mutation:** deleting the `Self::Image => ...` arm's own distinct id
/// (opting `ID_FALLBACK` in for every branch) turns this red — the image
/// cell would then carry `avatar-fallback` instead of `avatar-image`.
#[gpui::test]
fn exactly_one_of_fallback_or_image_exists_per_cell(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let letter = ids(&measure(cx, cell(&["--preview", "letter"])));
    assert!(letter.iter().any(|id| id == "avatar-fallback"));
    assert!(!letter.iter().any(|id| id == "avatar-image"));

    let image = ids(&measure(cx, cell(&["--preview", "image"])));
    assert!(image.iter().any(|id| id == "avatar-image"));
    assert!(!image.iter().any(|id| id == "avatar-fallback"));
}

/// **The preview avatar's own extent is `avatar::CallSite::RepoIcon`'s
/// `size-14` (56px), read through a real layout** — not re-derived by hand.
///
/// **Mutation:** swapping `avatar::CallSite::RepoIcon.extent()` for
/// `avatar::CallSite::Message.extent()` (24px) in `PreviewAvatar::render`
/// turns this red.
#[gpui::test]
fn the_preview_avatar_is_56px_square(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let avatar = at(&records, "avatar");
    assert_px(avatar.size.width, px(56.0));
    assert_px(avatar.size.height, px(56.0));
}

/// **The popup's own width is always `w-64` (256px), whatever `--width`
/// the surrounding cell drives** — `POPUP_WIDTH` is authored, so this is
/// really a check that the surface does not silently stretch to fill the
/// wrapper `--width` gives it.
///
/// **Mutation:** replacing `div().w(POPUP_WIDTH)` with `div().w_full()` in
/// `PopupContent::render` turns this red — the 500px cell would then
/// measure 500, not 256.
#[gpui::test]
fn the_popup_is_always_w_64(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    for width in [300u16, 500, 700] {
        let records = measure(cx, cell(&["--width", &width.to_string()]));
        let root = find(&records, repo_icon_popover::ID_POPUP);
        assert_px(root.bounds.size.width, px(256.0));
    }
}

/// **P3.63: the popup composes `popover`'s own chrome** — a real 1px border,
/// a real 10px radius, and a `popover-viewport` box with its own 16px
/// padding on every side. Before this fix the popup was a plain
/// `div().w(POPUP_WIDTH).bg(theme.popover)` with none of the three: no
/// border, no radius, and `inner`'s own `p-3` column sat directly against
/// it — `native/mapping/repo-icon-popover.md` §6's "one root cause behind 17
/// of the geometry deltas".
///
/// The popup's own size (256×177) and the viewport's (254×175, one border
/// in on both axes) are the two numbers `popover.md` §1 already measured
/// live on this exact popup, reused here as the target rather than
/// re-derived.
///
/// **Mutation, run and reverted:** reverting `PopupContent::render`'s tail to
/// its pre-fix shape —
/// `anchors.root(AnchorId::from(ID_POPUP), div().w(POPUP_WIDTH)
/// .bg(theme.popover).child(inner))`, dropping the border/radius/viewport
/// wrapper entirely — and running this test gives (actual output, this
/// mutation was run and reverted):
///
/// ```text
/// thread 'row_layout::repo_icon_popover::the_popup_composes_popovers_border_radius_and_viewport' (278028978) panicked at crates/crowbar-app/src/row_layout/repo_icon_popover.rs:194:5:
/// expected 1px, got 0px
/// ```
///
/// (the border assertion — the first one the mutation reaches — comparing
/// `popover::BORDER_WIDTH` against the reverted popup's `border_width` of
/// `0px`; `popover-viewport` itself is also absent from `records` on the
/// reverted shape, which the later `find` calls would have caught next.)
#[gpui::test]
fn the_popup_composes_popovers_border_radius_and_viewport(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let popup = find(&records, repo_icon_popover::ID_POPUP);
    assert_px(popup.border_width, popover::BORDER_WIDTH);
    assert_px(popup.border_width, px(1.0));
    assert_px(popup.radius, Theme::DARK.radius_lg.value());
    assert_px(popup.radius, px(10.0));
    assert_px(popup.bounds.size.width, px(256.0));
    assert_px(popup.bounds.size.height, px(177.0));

    let viewport = find(&records, popover::ID_VIEWPORT);
    let relative = at(&records, popover::ID_VIEWPORT);
    assert_px(relative.origin.x, popover::BORDER_WIDTH);
    assert_px(relative.origin.y, popover::BORDER_WIDTH);
    assert_px(viewport.bounds.size.width, px(254.0));
    assert_px(viewport.bounds.size.height, px(175.0));
}

/// **P3.63: the three always-visible buttons carry their own text.**
/// `text`, `font` and `text_width` all now ride on the same anchor as the
/// box, through `AnchorSink::boxed_text`. Before this fix
/// `ActionButton::render` painted `.child(label)` — a bare, unanchored
/// string — so a snapshot rooted on one of these buttons carried a box and
/// nothing about what was painted inside it (`repo-icon-popover.md` §6,
/// "15 field-presence").
///
/// **Mutation, run and reverted:** reverting `ActionButton::render` to
/// `.child(div()...).child(label)` plus `anchors.boxed(AnchorId::from(id),
/// shell)` (dropping `boxed_text`) and running this test gives (actual
/// output, this mutation was run and reverted):
///
/// ```text
/// thread 'row_layout::repo_icon_popover::the_three_action_buttons_carry_their_own_text' (278036539) panicked at crates/crowbar-app/src/row_layout/repo_icon_popover.rs:239:32:
/// repo-icon-popover-upload carries no text
/// ```
#[gpui::test]
fn the_three_action_buttons_carry_their_own_text(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    for (id, label) in [
        (repo_icon_popover::ID_UPLOAD, "Upload"),
        (repo_icon_popover::ID_EMOJI, "Emoji"),
        (repo_icon_popover::ID_GITHUB, "GitHub"),
    ] {
        let button = find(&records, id);
        let text = button
            .text
            .unwrap_or_else(|| panic!("{id} carries no text"));
        assert_eq!(text.content.as_ref(), label, "{id}");
        assert!(text.width > px(0.0), "{id}");
    }
}

/// **P3.63: the three buttons no longer share one flat width.** `Upload` and
/// `GitHub` (six letters each) come out equal, and both wider than `Emoji`
/// (five) — content-driven, not a three-way equal split that cannot see the
/// labels at all. Before this fix all three measured within half a pixel of
/// each other (`73.5`/`73`/`73.5`) regardless of their own text —
/// `.flex_1()`'s `flex-basis: 0%` grows every item on an equal share without
/// first clamping any of them to their own min-content, which is the
/// "automatic minimum size" step a browser applies and this layout engine
/// does not (`ActionButton::render`'s own doc comment carries the measured
/// before/after).
///
/// **Mutation, run and reverted:** reverting `.flex_auto()` back to
/// `.flex_1()` and running this test gives (actual output, this mutation was
/// run and reverted — the equal-share width is `62px` here rather than the
/// `73.5px` `repo-icon-popover.md` §6 measured, because this test runs
/// *after* the border/viewport fix narrowed the row's own available width;
/// the point the mutation demonstrates — equal regardless of label — is the
/// same either way):
///
/// ```text
/// thread 'row_layout::repo_icon_popover::the_three_action_buttons_size_to_their_own_label' (278041756) panicked at crates/crowbar-app/src/row_layout/repo_icon_popover.rs:278:5:
/// upload=62px emoji=62px
/// ```
#[gpui::test]
fn the_three_action_buttons_size_to_their_own_label(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let upload = find(&records, repo_icon_popover::ID_UPLOAD).bounds.size.width;
    let emoji = find(&records, repo_icon_popover::ID_EMOJI).bounds.size.width;
    let github = find(&records, repo_icon_popover::ID_GITHUB).bounds.size.width;

    assert_px(upload, github);
    assert!(upload > emoji, "upload={upload:?} emoji={emoji:?}");
    assert!(github > emoji, "github={github:?} emoji={emoji:?}");
}

/// **P3.63: `avatar-fallback`'s line height is `text-sm`'s own `1.25/0.875`
/// ratio (20px on 14px text), not gpui's golden-ratio default for a leaf
/// with no explicit line height (`~22.5px`).**
///
/// **Mutation, run and reverted:** dropping
/// `.line_height(relative(Self::LETTER_LINE_HEIGHT))` from
/// `PreviewAvatar::render`'s `Self::Letter` arm and running this test gives
/// (actual output, this mutation was run and reverted):
///
/// ```text
/// thread 'row_layout::repo_icon_popover::avatar_fallback_line_height_is_text_sm_not_the_golden_ratio_default' (278045763) panicked at crates/crowbar-app/src/row_layout/repo_icon_popover.rs:302:5:
/// expected 20px, got 22.5px
/// ```
#[gpui::test]
fn avatar_fallback_line_height_is_text_sm_not_the_golden_ratio_default(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));

    let fallback = find(&records, "avatar-fallback");
    let text = fallback.text.expect("avatar-fallback paints text");
    assert_px(text.font.line_height, px(20.0));
}

/// **P3.63: the preview avatar's own `y`, relative to the popup, is `56`** —
/// one border, the viewport's 16px padding, the inner column's own 12px
/// padding, the caption's 15px line box and one 12px gap:
/// `1 + 16 + 12 + 15 + 12 = 56`, the same arithmetic `native/mapping/
/// repo-icon-popover.md` §6 checks against the live reference. This is the
/// integration check that the border/viewport fix and the caption's own
/// line-height fix (`PopupContent::CAPTION_LINE_HEIGHT`) land together: the
/// border/viewport fix alone left this at `57`, one pixel short of the
/// target, because the caption — sized off gpui's golden-ratio default
/// before its own fix — was 16px tall rather than 15.
///
/// **Mutation, run and reverted:** dropping
/// `.line_height(relative(CAPTION_LINE_HEIGHT))` from `PopupContent::caption`
/// and running this test gives (actual output, this mutation was run and
/// reverted):
///
/// ```text
/// thread 'row_layout::repo_icon_popover::the_preview_avatar_sits_56px_below_the_popups_own_top' (278050071) panicked at crates/crowbar-app/src/row_layout/repo_icon_popover.rs:330:5:
/// expected 56px, got 57px
/// ```
#[gpui::test]
fn the_preview_avatar_sits_56px_below_the_popups_own_top(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure(cx, cell(&[]));
    let avatar = at(&records, "avatar");
    assert_px(avatar.origin.y, px(56.0));
}

// ─── the trigger, driven without a `--surface` ────────────────────────────

/// A one-view stage around a bare [`Trigger`] — the same shape
/// `row_layout::sidebar_tab_bar`'s own `Stage` wraps a bare
/// `SidebarTabBar` in.
struct TriggerStage {
    trigger: Trigger,
    theme: Theme,
}

impl Render for TriggerStage {
    fn render(&mut self, _window: &mut Window, _cx: &mut Context<Self>) -> impl IntoElement {
        div()
            .font(ui_sans_font(&self.theme))
            .child(self.trigger.render(&self.theme, &DriverAnchors))
    }
}

fn measure_trigger(cx: &mut TestAppContext, trigger: Trigger) -> Vec<RawAnchor> {
    let theme = Theme::DARK;
    let (_anchors, records) = lay_out(cx, size(px(200.0), px(120.0)), |_, _| TriggerStage {
        trigger,
        theme,
    });
    records
}

/// **The idle trigger carries its own anchor and, on the loaded-image
/// picture only, `repo-avatar` nested inside it.**
///
/// **Mutation:** opting `repo_avatar::ID` in for every [`Kind`] arm (not
/// only `Image(Loaded)`) in `Trigger::render` turns the second assertion
/// red — the letter-fallback cell would then carry `repo-avatar` too,
/// which `repo-icon-popover.tsx`'s own hand-rolled letter span never does.
#[gpui::test]
fn only_the_loaded_image_picture_carries_repo_avatar(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let theme = Theme::DARK;
    let letter = measure_trigger(cx, Trigger::fixture(&theme));
    let letter_ids = ids(&letter);
    assert!(letter_ids.contains(&repo_icon_popover::ID_TRIGGER.to_owned()));
    assert!(!letter_ids.iter().any(|id| id == "repo-avatar"));

    let mut image_trigger = Trigger::fixture(&theme);
    image_trigger.picture = Kind::Image(ImageState::Loaded);
    let image = measure_trigger(cx, image_trigger);
    let image_ids = ids(&image);
    assert!(image_ids.iter().any(|id| id == "repo-avatar"), "{image_ids:?}");
}

/// **`working: true` replaces the whole trigger with the spinner — no
/// `repo-avatar`, whatever `picture` says — and the trigger's own anchor is
/// still the single, shared id both rest states carry.**
#[gpui::test]
fn a_working_trigger_shows_the_spinner_regardless_of_picture(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let theme = Theme::DARK;
    let mut working = Trigger::fixture(&theme);
    working.working = true;
    working.picture = Kind::Image(ImageState::Loaded);

    let seen = ids(&measure_trigger(cx, working));
    assert!(seen.contains(&repo_icon_popover::ID_TRIGGER.to_owned()), "{seen:?}");
    assert!(seen.iter().any(|id| id == "workspace-branch-icon"), "{seen:?}");
    assert!(!seen.iter().any(|id| id == "repo-avatar"), "{seen:?}");
}

/// **The trigger's own box is a fixed 20px square, whichever picture it
/// shows** — `h-5 w-5` is unconditional in the source.
#[gpui::test]
fn the_trigger_is_always_20px_square(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let theme = Theme::DARK;
    let records = measure_trigger(cx, Trigger::fixture(&theme));
    let trigger = find(&records, repo_icon_popover::ID_TRIGGER);
    assert_px(trigger.bounds.size.width, px(20.0));
    assert_px(trigger.bounds.size.height, px(20.0));
}
