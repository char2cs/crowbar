//! `repo_icon_popover` — the repo avatar turned into a popover trigger for
//! editing a repo's icon: a small trigger picture (or, while an agent turn
//! is running, the working spinner in its place) and, when open, a preview
//! avatar plus upload/emoji/GitHub/reset controls.
//!
//! The native half of `web/src/components/layout/repo-icon-popover.tsx`.
//! Every length below came out of the app's own `tailwindcss` 4.3.0 with the
//! utility as a candidate — the method `native/MAPPING.md` fixes. See
//! `native/mapping/repo-icon-popover.md`.
//!
//! # The trigger reuses `repo_avatar`'s vocabulary, not its boxes
//!
//! `repo-icon-popover.tsx` reuses `repo-avatar.tsx`'s exported
//! `RepoAvatarImg` for its own image case, so this port reuses
//! [`repo_avatar::Kind`]/[`repo_avatar::ImageState`] to describe which
//! picture is showing — the same three-way branch, genuinely shared. What is
//! **not** shared is the box: this trigger's own classes
//! (`h-5 w-5 rounded-md object-cover` on the image,
//! `rounded-md px-1 text-[11px]` on the letter fallback, `text-lg` on the
//! emoji) differ from `repo_avatar.rs`'s own `Size::Lg` (`rounded-sm
//! px-0.5`) at every case but the box extent, so [`Trigger::render`] builds
//! its own boxes from those literals rather than calling
//! [`repo_avatar::RepoAvatar::render`]. Only the **image** case carries an
//! anchor at all — `RepoAvatarImgAttempt`'s own `data-oracle-id="repo-avatar"`
//! is unconditional in the source, but this component's own hand-rolled
//! emoji and letter spans never call `RepoAvatar()` and so never reach that
//! id (checked against the source, not assumed) — [`Trigger::render`]
//! reproduces exactly that asymmetry: [`repo_avatar::ID`] is opted in only
//! for [`repo_avatar::Kind::Image`].
//!
//! # The preview avatar reuses `avatar::CallSite::RepoIcon`'s numbers, not its render path
//!
//! `avatar.rs`'s own [`super::avatar::CallSite::RepoIcon`] already models
//! this exact call site's `<Avatar className="size-14 rounded-xl
//! text-base">` — box extent, radius, weight, all pre-measured — but its
//! `render`/`root` are not reused directly, for the same reason
//! `sidebar_project_header.rs` does not call `Button::render`: `Avatar::
//! render` opts an `anchors.root(...)` of its own (`avatar.rs`'s own
//! `ID_ROOT`), and nesting a second root inside this surface's own would
//! contest which anchor `ANCHORS.md` §4 means. [`PreviewAvatar::render`]
//! reuses `CallSite::RepoIcon`'s public `extent()`/`radius()` and opts the
//! **same** id, [`super::avatar::ID_ROOT`] (`"avatar"`), in via `.boxed()`
//! instead — the real DOM never overrides it either (`repo-icon-popover.tsx`'s
//! own `<Avatar>` passes no `data-oracle-id`), so this is the id a live
//! capture would actually carry, reused rather than duplicated under a
//! second name.
//!
//! `avatar.rs`'s own docs already flag its `RepoIcon` fallback as **not
//! modelled** — "the third [fallback] writes `repo.avatarColor`... a port
//! cannot resolve a class name that arrives as data." This module resolves
//! that the same way `repo_avatar.rs` resolves the identical problem for its
//! own letter fallback: [`PreviewAvatar::Letter::background`] is a
//! caller-supplied [`Color`], not invented.
//!
//! # Two anchors, reachable from two different rest states
//!
//! [`ID_TRIGGER`] is opted in on **both** of `repo-icon-popover.tsx`'s two
//! early-return shapes — the working spinner and the popover trigger are
//! mutually exclusive (`repo.defaultWorking` short-circuits before the
//! `<Popover>` is ever reached), and this port models that the same way
//! `repo_avatar.rs` models "a leaf with a shared id across mutually-
//! exclusive pictures". Both of `repo-icon-popover.tsx`'s own returns for
//! this box carried **no** `data-oracle-id` at all before this item; both do
//! now (see the React diff) — the only asymmetry the source draws is that
//! the *image* case, one level in, additionally carries `repo-avatar`.
//!
//! [`ID_POPUP`] is namespaced away from `popover.tsx`'s own generic
//! `"popover-popup"` default — the same "call site of a shared primitive"
//! finding `detach-holder-modal-popup` and `repo-import-dialog-popup`
//! (P3.51) already made, and for the identical reason:
//! `surface.rs`'s registry requires a unique root per surface, and reusing
//! the generic id would collide in fact, not just in spelling.
//!
//! # No `oracleSurfaceScope` entry needed
//!
//! Every anchor this port's own popup composition can nest — `avatar`'s
//! (from the preview) and, on the image trigger case, `repo-avatar`'s — is
//! exactly what this composition paints, not foreign content left unpainted
//! the way `sidebar-project-header`'s toggle icon is. Every button inside
//! the popup carries its own namespaced id (added by this item; see the
//! React diff) rather than `button.tsx`'s repeated generic default, so an
//! undeclared capture rooted at [`ID_POPUP`] carries no duplicate either.
//! The `--emoji` row's `<Input>` is the one place this composition reaches a
//! **generic**, unnamespaced shared id (`input.rs`'s own `"input-control"`/
//! `"input"`, hard-coded with no override prop) — not foreign, since
//! [`PopupContent::emoji_row`] paints both, but also not unique-by-
//! construction the way every namespaced id here is. It stays undeclared
//! anyway: at most one `<Input>` is ever mounted in this popup at once (the
//! row is call-site-conditional, not repeated), so no capture this surface
//! can produce carries it twice — the `select-item`/`autocomplete-item`
//! duplication risk this declaration mechanism exists for does not arise
//! here.
//!
//! # `--surface`: the popup only; the trigger is tested without one
//!
//! One registered `--surface repo-icon-popover`, rooted at [`ID_POPUP`] —
//! the richer, multi-control picture, the same choice `popover`/
//! `detach-holder-modal` make (the floated content is the surface; a call
//! site's own trigger chrome is not separately registered anywhere in this
//! tree). [`Trigger::render`]'s own two rest states are real, tested
//! geometry too, but registering a second surface for them would need a
//! second root this file does not otherwise have a use for — they are
//! driven directly through the shared `row_layout` harness instead, the
//! `sidebar_tab_bar.rs` shape, in `row_layout/repo_icon_popover.rs`.

use gpui::{
    AnyElement, Div, FontWeight, IntoElement as _, ParentElement as _, Pixels, SharedString,
    Styled as _, div, px,
};

use super::anchor::{AnchorId, AnchorSink};
use super::avatar;
use super::button;
use super::git_status_row::Breakpoint;
use super::repo_avatar::{self, ImageState, Kind};
use super::workspace_branch_icon::{self, WorkspaceBranchIcon};
use crate::theme::{Color, Theme};

/// The trigger's own anchor — carried on both of its mutually-exclusive
/// rest states. See the module docs.
pub const ID_TRIGGER: &str = "repo-icon-popover-trigger";
/// The popup's own anchor — namespaced away from `popover.tsx`'s generic
/// default. See the module docs.
pub const ID_POPUP: &str = "repo-icon-popover-popup";
/// The "Upload" button.
pub const ID_UPLOAD: &str = "repo-icon-popover-upload";
/// The "Emoji" toggle button.
pub const ID_EMOJI: &str = "repo-icon-popover-emoji";
/// The "GitHub" button.
pub const ID_GITHUB: &str = "repo-icon-popover-github";
/// The emoji row's own "Set" submit button. Reachable only when
/// [`PopupContent::show_emoji_input`] is `true`.
pub const ID_EMOJI_SUBMIT: &str = "repo-icon-popover-emoji-submit";
/// The "Reset to default" button. Reachable only when
/// [`PopupContent::has_avatar_url`] is `true`.
pub const ID_RESET: &str = "repo-icon-popover-reset";

/// **Empty.** Every box on this surface is authored; nothing sizes to a text
/// run's own max-content width.
pub const CONTENT_SIZED: [&str; 0] = [];
/// **Empty**, for the same reason.
pub const LINE_SIZED: [&str; 0] = [];

/// `--spacing`, Tailwind's stock `0.25rem` at a 16px root.
const SPACING: f32 = 4.0;

/// `h-5 w-5` on the trigger.
pub const TRIGGER_SIZE: Pixels = px(SPACING * 5.0);
/// `rounded-md` on the trigger's image/letter pictures — `theme.radius_md`,
/// **not** `repo_avatar::Size::Lg`'s own `rounded-sm`. See the module docs.
/// (Read through `theme.radius_md` at render time; no separate constant.)
pub const TRIGGER_LETTER_TEXT: Pixels = px(11.0);
/// The trigger emoji's own `text-lg`.
pub const TRIGGER_EMOJI_TEXT: Pixels = px(18.0);
/// `px-1` on the trigger's letter fallback.
pub const TRIGGER_LETTER_PADDING_X: Pixels = px(SPACING);

/// `w-64` on the popup.
pub const POPUP_WIDTH: Pixels = px(256.0);
/// `p-3` on the popup's own inner wrapper.
pub const POPUP_PADDING: Pixels = px(SPACING * 3.0);
/// `gap-3` between the popup's own rows.
pub const POPUP_GAP: Pixels = px(SPACING * 3.0);
/// `gap-1.5` inside the three-button row and the emoji row.
pub const ROW_GAP: Pixels = px(SPACING * 1.5);
/// `text-[10px]` on the "Icon" caption.
pub const CAPTION_TEXT: Pixels = px(10.0);
/// `size-3` on each button's own leading glyph.
pub const BUTTON_ICON_SIZE: Pixels = px(SPACING * 3.0);
/// `h-7` on the emoji input and its "Set" button.
pub const EMOJI_ROW_HEIGHT: Pixels = px(SPACING * 7.0);

/// The breakpoint every cell on this surface renders at — the posture
/// `autocomplete.rs`'s own `BP` states: every window this port can drive is
/// at or above 640px.
const BP: Breakpoint = Breakpoint::Sm;

/// The trigger's own picture, plus the `working` flag that overrides it.
#[derive(Clone, Debug, PartialEq)]
pub struct Trigger {
    /// `repo.defaultWorking` — checked first; when `true` neither
    /// [`Self::picture`] nor the popover exists at all.
    pub working: bool,
    /// `isEmoji`/`avatarSrc`/neither — [`repo_avatar::Kind`], reused. See
    /// the module docs.
    pub picture: Kind,
    /// `repo.avatarLabel` — read only when [`Self::picture`] resolves to
    /// the letter fallback.
    pub label: SharedString,
    /// The emoji glyph — read only when [`Self::picture`] is
    /// [`Kind::Emoji`].
    pub emoji: SharedString,
    /// `repo.avatarColor`, resolved by the caller — the same posture
    /// `repo_avatar::RepoAvatar::background` takes, for the identical
    /// reason.
    pub background: Color,
}

impl Trigger {
    /// The live resting trigger: idle, the letter fallback.
    #[must_use]
    pub fn fixture(theme: &Theme) -> Self {
        Self {
            working: false,
            picture: Kind::Letter,
            label: SharedString::new_static("R"),
            emoji: SharedString::new_static("\u{1f98a}"),
            background: theme.primary,
        }
    }

    /// `'inline-flex h-5 w-5 items-center justify-center text-lg
    /// leading-none'` — no background, no radius, matching
    /// `repo_avatar.rs`'s own emoji box shape but at this trigger's own
    /// size.
    fn emoji_box(theme: &Theme) -> Div {
        div()
            .flex()
            .flex_shrink_0()
            .items_center()
            .justify_center()
            .w(TRIGGER_SIZE)
            .h(TRIGGER_SIZE)
            .text_size(TRIGGER_EMOJI_TEXT)
            .text_color(theme.foreground)
    }

    /// `'h-5 w-5 rounded-md object-cover'` — an **empty** box, the same call
    /// `repo_avatar.rs`'s own image case makes.
    fn image_box(theme: &Theme) -> Div {
        div()
            .flex_shrink_0()
            .w(TRIGGER_SIZE)
            .h(TRIGGER_SIZE)
            .rounded(theme.radius_md.value())
    }

    /// `'inline-flex h-5 w-5 items-center justify-center rounded-md px-1
    /// text-[11px] font-bold text-primary-foreground'`.
    fn letter_box(&self, theme: &Theme) -> Div {
        div()
            .flex()
            .flex_shrink_0()
            .items_center()
            .justify_center()
            .w(TRIGGER_SIZE)
            .h(TRIGGER_SIZE)
            .rounded(theme.radius_md.value())
            .px(TRIGGER_LETTER_PADDING_X)
            .bg(self.background)
            .font_weight(FontWeight::BOLD)
            .text_size(TRIGGER_LETTER_TEXT)
            .text_color(theme.primary_foreground)
    }

    /// Renders the trigger, opting [`ID_TRIGGER`] in on both rest states —
    /// see the module docs.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        if self.working {
            let spinner = WorkspaceBranchIcon {
                status: workspace_branch_icon::Status::default(),
                working: true,
                is_placeholder: false,
            }
            .render(theme, anchors);
            let shell = div()
                .flex()
                .flex_shrink_0()
                .items_center()
                .justify_center()
                .w(TRIGGER_SIZE)
                .h(TRIGGER_SIZE)
                .child(spinner);
            return anchors.boxed(AnchorId::from(ID_TRIGGER), shell);
        }

        let shell = div()
            .relative()
            .flex_shrink_0()
            .w(TRIGGER_SIZE)
            .h(TRIGGER_SIZE);
        let shell = match self.picture {
            Kind::Emoji => shell.child(Self::emoji_box(theme).child(self.emoji.clone())),
            Kind::Image(ImageState::Loaded) => {
                shell.child(anchors.boxed(AnchorId::from(repo_avatar::ID), Self::image_box(theme)))
            }
            Kind::Image(ImageState::Errored) | Kind::Letter => {
                shell.child(self.letter_box(theme).child(self.label.clone()))
            }
        };
        anchors.boxed(AnchorId::from(ID_TRIGGER), shell)
    }
}

/// The popup's own preview picture — `isEmoji`/`avatarSrc`/neither again,
/// but a **different** three-way branch from [`Trigger`]'s: the emoji
/// fallback is `text-2xl`, the letter fallback is `text-sm font-bold` on
/// `repo.avatarColor`, and `avatar.rs`'s own docs already flag this branch
/// as not modelled there — resolved here instead.
#[derive(Clone, Debug, PartialEq)]
pub enum PreviewAvatar {
    /// `<AvatarFallback className="rounded-xl bg-transparent text-2xl">`.
    Emoji(SharedString),
    /// `<AvatarImage src={avatarSrc}>` — an empty box, no native
    /// equivalent.
    Image,
    /// `<AvatarFallback className="rounded-xl text-sm font-bold
    /// text-primary-foreground" style={repo.avatarColor}>`.
    Letter {
        /// `repo.avatarLabel`.
        label: SharedString,
        /// `repo.avatarColor`, caller-resolved.
        background: Color,
    },
}

impl PreviewAvatar {
    /// `text-2xl` — Tailwind's `1.5rem` = 24px.
    const EMOJI_TEXT: Pixels = px(24.0);
    /// `text-sm` — read through the `--ui-text-*` trade `native/MAPPING.md`
    /// states once (`text-sm` == `--ui-text-base`).
    fn letter_text(theme: &Theme) -> Pixels {
        theme.ui_text_base.value().to_pixels(px(16.0))
    }

    /// Renders the preview avatar as **two** nested boxes, matching
    /// `avatar.tsx`'s own structure: `AvatarPrimitive.Root` (`bg-background`,
    /// unconditional, [`super::avatar::ID_ROOT`]) wrapping either
    /// `AvatarImage` ([`super::avatar::ID_IMAGE`]) or `AvatarFallback`
    /// (`bg-muted`, overridden per branch below, [`super::avatar::
    /// ID_FALLBACK`]) — never both, `base-ui`'s own arrangement, the
    /// identical shape `avatar.rs`'s own `Avatar::render` documents for
    /// every other call site.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let extent = avatar::CallSite::RepoIcon.extent();
        let radius = avatar::CallSite::RepoIcon.radius(theme);
        let root = div()
            .flex()
            .flex_shrink_0()
            .items_center()
            .justify_center()
            .overflow_hidden()
            .w(extent)
            .h(extent)
            .rounded(radius)
            .bg(theme.background)
            .font_weight(avatar::CallSite::RepoIcon.weight());

        let content = match self {
            Self::Emoji(emoji) => anchors.boxed_text(
                AnchorId::from(avatar::ID_FALLBACK),
                div()
                    .size_full()
                    .flex()
                    .items_center()
                    .justify_center()
                    .rounded(radius)
                    .text_size(Self::EMOJI_TEXT),
                emoji.clone(),
            ),
            Self::Image => anchors.boxed(AnchorId::from(avatar::ID_IMAGE), div().size_full()),
            Self::Letter { label, background } => anchors.boxed_text(
                AnchorId::from(avatar::ID_FALLBACK),
                div()
                    .size_full()
                    .flex()
                    .items_center()
                    .justify_center()
                    .rounded(radius)
                    .bg(*background)
                    .font_weight(FontWeight::BOLD)
                    .text_size(Self::letter_text(theme))
                    .text_color(theme.primary_foreground),
                label.clone(),
            ),
        };

        anchors.boxed(AnchorId::from(avatar::ID_ROOT), root.child(content))
    }
}

/// One `size="xs"` icon-and-label button in the three-button row, or the
/// full-width reset button.
struct ActionButton;

impl ActionButton {
    /// `size="xs"`'s own height at [`BP`] — `button::Size::Xs.extent(BP)`,
    /// reused rather than re-derived, the `sidebar_project_header.rs`
    /// move.
    fn height() -> Pixels {
        button::Size::Xs.extent(BP)
    }

    /// One button's own box: `flex-1 gap-1`, an empty leading glyph and a
    /// label, at `button::Size::Xs`'s own type step and radius.
    fn render(
        theme: &Theme,
        anchors: &dyn AnchorSink,
        id: &'static str,
        label: &'static str,
        full_width: bool,
    ) -> AnyElement {
        let step = button::Size::Xs.type_step(theme, BP);
        let mut shell = div()
            .flex()
            .items_center()
            .justify_center()
            .gap(button::Size::Xs.gap())
            .h(Self::height())
            .rounded(button::Size::Xs.radius(theme))
            .text_color(theme.muted_foreground)
            .text_size(step.size.to_pixels(px(16.0)))
            .child(
                div()
                    .flex_shrink_0()
                    .w(BUTTON_ICON_SIZE)
                    .h(BUTTON_ICON_SIZE),
            )
            .child(label);
        shell = if full_width {
            shell.w_full()
        } else {
            shell.flex_1()
        };
        anchors.boxed(AnchorId::from(id), shell)
    }
}

/// The popup's own state: which preview picture shows, and the two
/// call-site-conditional rows.
#[derive(Clone, Debug, PartialEq)]
pub struct PopupContent {
    /// The preview avatar. See [`PreviewAvatar`].
    pub preview: PreviewAvatar,
    /// `showEmojiInput` — mounts the emoji text field and its "Set" button.
    pub show_emoji_input: bool,
    /// `repo.avatarURL` truthy — mounts the "Reset to default" button.
    pub has_avatar_url: bool,
}

impl PopupContent {
    /// The live reachable cell: the letter fallback, neither optional row
    /// mounted (`showEmojiInput` starts `false`; the fixture repo has no
    /// `avatarURL`).
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            preview: PreviewAvatar::Letter {
                label: SharedString::new_static("R"),
                background: Color::TRANSPARENT,
            },
            show_emoji_input: false,
            has_avatar_url: false,
        }
    }

    /// The "Icon" caption: `text-[10px] font-semibold uppercase
    /// tracking-wider text-muted-foreground`. `tracking-wider` is not
    /// modelled — gpui has no letter-spacing at all, the identical
    /// limitation `native/MAPPING.md`'s `dropdown-menu` row already states.
    fn caption(theme: &Theme) -> Div {
        div()
            .text_size(CAPTION_TEXT)
            .font_weight(FontWeight::SEMIBOLD)
            .text_color(theme.muted_foreground)
            .child("ICON")
    }

    /// The three-button row: Upload, Emoji, GitHub.
    fn actions(theme: &Theme, anchors: &dyn AnchorSink) -> Div {
        div()
            .flex()
            .gap(ROW_GAP)
            .child(ActionButton::render(
                theme, anchors, ID_UPLOAD, "Upload", false,
            ))
            .child(ActionButton::render(
                theme, anchors, ID_EMOJI, "Emoji", false,
            ))
            .child(ActionButton::render(
                theme, anchors, ID_GITHUB, "GitHub", false,
            ))
    }

    /// The emoji entry row: `<Input h-7 flex-1 text-center text-base
    /// maxLength={4}>` + `<Button size="sm" className="h-7">Set</Button>`.
    /// Reachable only when [`Self::show_emoji_input`] is `true`.
    ///
    /// `<Input>` is the already-ported, already-registered shared primitive
    /// — `input.rs`'s own `data-oracle-id="input-control"`/`"input"` are
    /// hard-coded with no override prop (unlike `button.tsx`'s), so this
    /// row's own field really does carry those two generic ids in the live
    /// DOM. [`input::Input::render`] itself is not called, for the reason
    /// every other composition in this file avoids calling a component that
    /// opts its own `anchors.root(...)` in: nesting a second root inside
    /// this surface's own would contest which anchor `ANCHORS.md` §4 means.
    /// This box reproduces [`super::input::ID_CONTROL`]/[`super::input::
    /// ID_FIELD`] as nested `.boxed()` anchors instead — the two real ids,
    /// on a simplified box (not every one of `input.rs`'s own painted
    /// properties), since this row has no live reference to measure against
    /// regardless (`showEmojiInput` starts `false`, and no capture technique
    /// this port can use opens the popover to type into it).
    fn emoji_row(theme: &Theme, anchors: &dyn AnchorSink) -> Div {
        let field = anchors.boxed(
            AnchorId::from(super::input::ID_FIELD),
            div().w_full().h_full(),
        );
        let input = anchors.boxed(
            AnchorId::from(super::input::ID_CONTROL),
            div()
                .relative()
                .flex_1()
                .h(EMOJI_ROW_HEIGHT)
                .rounded(theme.radius_lg.value())
                .border_1()
                .border_color(theme.input)
                .child(field),
        );
        let submit = anchors.boxed(
            AnchorId::from(ID_EMOJI_SUBMIT),
            div()
                .flex()
                .items_center()
                .justify_center()
                .h(EMOJI_ROW_HEIGHT)
                .px(button::Size::Sm.padding_x())
                .rounded(button::Size::Sm.radius(theme))
                .bg(theme.primary)
                .text_color(theme.primary_foreground)
                .child("Set"),
        );
        div().flex().gap(ROW_GAP).child(input).child(submit)
    }

    /// Renders the popup's own inner column, opting every button's contract
    /// anchor into `anchors`.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let mut children: Vec<AnyElement> = vec![
            Self::caption(theme).into_any_element(),
            div()
                .flex()
                .justify_center()
                .child(self.preview.render(theme, anchors))
                .into_any_element(),
            Self::actions(theme, anchors).into_any_element(),
        ];
        if self.show_emoji_input {
            children.push(Self::emoji_row(theme, anchors).into_any_element());
        }
        if self.has_avatar_url {
            children.push(ActionButton::render(
                theme,
                anchors,
                ID_RESET,
                "Reset to default",
                true,
            ));
        }

        let inner = div()
            .flex()
            .flex_col()
            .gap(POPUP_GAP)
            .p(POPUP_PADDING)
            .children(children);

        anchors.root(
            AnchorId::from(ID_POPUP),
            div().w(POPUP_WIDTH).bg(theme.popover).child(inner),
        )
    }
}

#[cfg(test)]
mod tests {
    use super::{
        CAPTION_TEXT, CONTENT_SIZED, EMOJI_ROW_HEIGHT, ID_EMOJI, ID_EMOJI_SUBMIT, ID_GITHUB,
        ID_POPUP, ID_RESET, ID_TRIGGER, ID_UPLOAD, LINE_SIZED, POPUP_GAP, POPUP_PADDING,
        POPUP_WIDTH, PopupContent, ROW_GAP, TRIGGER_EMOJI_TEXT, TRIGGER_LETTER_TEXT, TRIGGER_SIZE,
        Trigger,
    };
    use crate::components::repo_avatar::Kind;
    use crate::theme::Theme;
    use gpui::px;

    #[test]
    fn every_length_is_the_compiled_spacing_multiple_or_a_literal() {
        const STEP: f32 = 4.0;
        assert_eq!(TRIGGER_SIZE, px(STEP * 5.0)); // h-5 w-5
        assert_eq!(POPUP_WIDTH, px(256.0)); // w-64
        assert_eq!(POPUP_PADDING, px(STEP * 3.0)); // p-3
        assert_eq!(POPUP_GAP, px(STEP * 3.0)); // gap-3
        assert_eq!(ROW_GAP, px(STEP * 1.5)); // gap-1.5
        assert_eq!(EMOJI_ROW_HEIGHT, px(STEP * 7.0)); // h-7
        assert_eq!(TRIGGER_LETTER_TEXT, px(11.0));
        assert_eq!(TRIGGER_EMOJI_TEXT, px(18.0));
        assert_eq!(CAPTION_TEXT, px(10.0));
    }

    #[test]
    fn neither_declaration_list_has_an_entry() {
        assert!(CONTENT_SIZED.is_empty());
        assert!(LINE_SIZED.is_empty());
    }

    /// Every id this module mints is distinct and namespaced under this
    /// component's own prefix — the `command.rs` convention.
    #[test]
    fn every_anchor_id_is_distinct_and_namespaced() {
        let ids = [
            ID_TRIGGER,
            ID_POPUP,
            ID_UPLOAD,
            ID_EMOJI,
            ID_GITHUB,
            ID_EMOJI_SUBMIT,
            ID_RESET,
        ];
        let mut sorted = ids.to_vec();
        sorted.sort_unstable();
        let before = sorted.len();
        sorted.dedup();
        assert_eq!(sorted.len(), before, "{ids:?}");
        assert!(ids.iter().all(|id| id.starts_with("repo-icon-popover")));
        assert_ne!(
            ID_POPUP, "popover-popup",
            "must not collide with popover.tsx's own default"
        );
    }

    #[test]
    fn the_trigger_fixture_is_idle_with_the_letter_fallback() {
        let theme = Theme::DARK;
        let trigger = Trigger::fixture(&theme);
        assert!(!trigger.working);
        assert_eq!(trigger.picture, Kind::Letter);
    }

    #[test]
    fn the_popup_fixture_mounts_neither_optional_row() {
        let content = PopupContent::fixture();
        assert!(!content.show_emoji_input);
        assert!(!content.has_avatar_url);
    }
}
