//! `--surface repo-icon-popover`, laid out in a real window — plus
//! `crowbar_ui::components::repo_icon_popover::Trigger`, which carries no
//! `--surface` of its own (see that module's docs) and is driven directly
//! through the shared harness instead, the `sidebar_tab_bar.rs` shape.

use super::{a_cell, assert_px, find, ids, lay_out, measure, relative_to};
use crowbar_driver::RawAnchor;
use crowbar_ui::Theme;
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
