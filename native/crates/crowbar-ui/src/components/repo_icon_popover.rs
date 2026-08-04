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
    Styled as _, div, px, relative,
};

use super::anchor::{AnchorId, AnchorSink};
use super::avatar;
use super::button;
use super::git_status_row::Breakpoint;
use super::popover;
use super::repo_avatar::{self, ImageState, Kind};
use super::workspace_branch_icon::{self, WorkspaceBranchIcon};
use crate::theme::{Color, Theme, ui_sans_font};

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

/// **The three always-visible buttons — `ID_UPLOAD`/`ID_EMOJI`/`ID_GITHUB`.**
/// Each is `flex-1` with no authored width, so its used width *is* its own
/// content's max-content width (`native/oracle/ANCHORS.md` v1.5); `ID_RESET`
/// (`w-full`, a definite width) and `ID_EMOJI_SUBMIT`/`"Set"` (not part of
/// this item's own verdict cell) stay undeclared. `ActionButton::render`'s
/// own doc comment carries the re-verdict this closes: a 0.61px `bounds.x`
/// delta on `github`, the accumulated ceil excess of the two buttons before
/// it, live on both sides only once `repo-icon-popover.tsx`'s own three
/// `<Button>`s carry `data-oracle-content-sized="true"` too — v1.5 is
/// explicit that this is an authored pair, never detected on either side.
pub const CONTENT_SIZED: [&str; 3] = [ID_UPLOAD, ID_EMOJI, ID_GITHUB];
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
/// The caption's own line height. `text-[10px]` is an arbitrary value with
/// no paired `leading-*` utility, so it inherits Tailwind's own preflight
/// default rather than a utility's companion ratio: `html { line-height:
/// 1.5 }`, giving a 15px line box at 10px. `native/mapping/repo-icon-
/// popover.md`'s own "Corroboration" section measures this live and calls
/// it P3.60's `row_base` finding reproduced independently on this component.
///
/// **Fixed alongside the popup's own chrome (P3.63).** [`PopupContent::
/// caption`] used to set no line height at all, so — the same unset-leaf
/// shape [`PreviewAvatar::LETTER_LINE_HEIGHT`]'s own doc comment
/// records — it fell back to gpui's own [`gpui::phi`] default (the golden
/// ratio) rather than 1.5, landing on a 16px line box (`10 × 1.618034`,
/// device-pixel-snapped) instead of 15px. That 1px propagated into every
/// anchor below the caption: measured before this fix, the preview avatar's
/// own `y` (relative to the popup) came out `57`, against the mapping doc's
/// own arithmetic, which assumes a 15px caption and targets `56`.
///
/// This `1.5` is **not** a transfer of `row_base::LINE_HEIGHT_RELATIVE` —
/// that constant's own docs say it is not a general law, and this is a
/// separate derivation for this leaf's own (unset) utility that happens to
/// land on the same ratio.
const CAPTION_LINE_HEIGHT: f32 = 1.5;
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
    ///
    /// # The non-working shell is rounded; the working one is not
    ///
    /// `repo-icon-popover.tsx` gives these two rest states **different**
    /// class lists on the very element this function anchors as
    /// [`ID_TRIGGER`]: the `repo.defaultWorking` early return's `<span
    /// className="pointer-events-none inline-flex h-5 w-5 shrink-0
    /// items-center justify-center">` carries no `rounded-*` utility at
    /// all, while the ordinary `<PopoverTrigger className="…inline-flex h-5
    /// w-5 shrink-0 items-center justify-center rounded-md outline-none">`
    /// does. So the working branch below stays unrounded (correct, matching
    /// the spinner span) and only the non-working `shell` gets
    /// `theme.radius_md` — `repo_icon_popover.rs`'s own `image_box`/
    /// `letter_box` already read the same token for the picture *inside*
    /// this shell; this is the outer `<PopoverTrigger>` box catching up to
    /// them. Before this fix the outer shell carried no `.rounded()` call
    /// at all, so `repo-icon-popover-trigger.radius` read `0.0` against a
    /// live `8.0` whenever this trigger renders inside a real row
    /// (`native/mapping/repo-section.md`'s own verdict, defect 3).
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
            .h(TRIGGER_SIZE)
            .rounded(theme.radius_md.value());
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
        /// `repo.avatarColor`, caller-resolved — `Some` when the repo has
        /// one assigned, `None` when it does not.
        ///
        /// **`None` is a real, modelled state, not "unknown."** `cn(...,
        /// repo.avatarColor)` drops a falsy `avatarColor` silently, and
        /// `AvatarFallback`'s own base class list is `bg-muted`
        /// (`avatar.rs`'s own [`avatar::Avatar::fallback`] paints exactly
        /// `theme.muted` for the identical reason) — so a repo with no
        /// assigned colour shows the primitive's own muted background, not
        /// nothing. [`PreviewAvatar::render`] resolves that fallback here,
        /// reusing `avatar.rs`'s own token rather than re-deriving it.
        background: Option<Color>,
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

    /// `text-sm`'s own line height — Tailwind's stock `calc(1.25 / 0.875)`,
    /// the same ratio a dozen other `text-sm` leaves in this crate already
    /// carry (`button::LINE_HEIGHT_SM`, `badge::LINE_HEIGHT_SM`,
    /// `dropdown_menu::ROW_LINE_HEIGHT`, `select::TEXT_SM_LINE_HEIGHT`, …),
    /// and the ratio `native/MAPPING.md`'s own `text-sm` row states once for
    /// the whole port: 14px text on a 20px line.
    ///
    /// **This is a fix, not a fresh derivation.** [`Self::Letter`]'s box used
    /// to set no line height at all, so an unset leaf fell back to gpui's own
    /// [`gpui::phi`] default — the golden ratio, `~1.618034` — rather than
    /// this project's `text-sm` pairing. Measured on this exact cell before
    /// the fix (`--surface repo-icon-popover`, the default `--preview
    /// letter` cell, `crowbar-app/src/row_layout/repo_icon_popover.rs`'s own
    /// harness): `avatar-fallback`'s `font.line_height` was `22.5px` —
    /// `14 × 1.618034 = 22.65`, device-pixel-snapped to the nearest half —
    /// against the live reference's `20px` (`native/mapping/repo-icon-
    /// popover.md` §6). `row_base::LINE_HEIGHT_RELATIVE` (`1.5`) is **not**
    /// this box's ratio either, and transferring it would still be wrong:
    /// `14 × 1.5 = 21`, not `20`. `text-sm`'s own companion is the only ratio
    /// that lands on the reference.
    const LETTER_LINE_HEIGHT: f32 = 1.25 / 0.875;

    /// `text-2xl`'s own line height — Tailwind's stock `calc(2rem / 1.5rem)`.
    ///
    /// The emoji fallback has the same unset-leaf shape [`Self::Letter`] had
    /// (see [`Self::LETTER_LINE_HEIGHT`]) and no live cell reaches it either
    /// (`--preview emoji` has no reference — `native/mapping/repo-icon-
    /// popover.md` §0), so this is preventive rather than measured: setting
    /// the utility's own real ratio explicitly, on the same evidence that an
    /// unset leaf here defaults to gpui's golden-ratio [`gpui::phi`] rather
    /// than to anything Tailwind ever compiled.
    const EMOJI_LINE_HEIGHT: f32 = 2.0 / 1.5;

    /// Renders the preview avatar as **two** nested boxes, matching
    /// `avatar.tsx`'s own structure: `AvatarPrimitive.Root` (`bg-background`,
    /// unconditional, [`super::avatar::ID_ROOT`]) wrapping either
    /// `AvatarImage` ([`super::avatar::ID_IMAGE`]) or `AvatarFallback`
    /// (`bg-muted`, overridden per branch below, [`super::avatar::
    /// ID_FALLBACK`]) — never both, `base-ui`'s own arrangement, the
    /// identical shape `avatar.rs`'s own `Avatar::render` documents for
    /// every other call site.
    ///
    /// # The fix: `Self::Letter`'s "no colour" default is `theme.muted`, not transparent
    ///
    /// A live check (`--surface repo-icon-popover`, the default cell) found
    /// `avatar-fallback.bg` painting nothing (`#00000000`) against a live
    /// reference of `#ffffff0a` — 4% white. That number is not an arbitrary
    /// colour: it is `bg-muted` in the dark theme, `AvatarFallback`'s own
    /// unconditional base class, showing through because the driven
    /// fixture's repo has no `avatarColor` at all, and `cn(..., '')` drops a
    /// falsy class silently — the same "no override" case
    /// `avatar::Avatar::fallback` already models for every call site that
    /// goes through it (`.bg(theme.muted)`, unconditional).
    ///
    /// **This belongs here, not in `avatar.rs`.** `avatar::Avatar::fallback`
    /// is already correct — checked directly, it paints `theme.muted`
    /// unconditionally and nothing about it needed to change, so `--surface
    /// avatar`'s own verdict is untouched by this fix. The bug was local to
    /// this file's own reimplementation (`PreviewAvatar` cannot call
    /// `Avatar::render` — see the module docs — so it reproduces the box by
    /// hand): [`PreviewAvatar::Letter::background`] used to be a plain
    /// `Color` whose fixture/driver default was `Color::TRANSPARENT`, a
    /// placeholder invented rather than read off the primitive it stands in
    /// for. It is now `Option<Color>`, `None` reads as "no `avatarColor`,"
    /// and this function resolves that the same way `avatar.rs`'s own
    /// primitive does: `background.unwrap_or(theme.muted)`.
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
                    .font(ui_sans_font(theme))
                    .line_height(relative(Self::EMOJI_LINE_HEIGHT))
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
                    .bg(background.unwrap_or(theme.muted))
                    .font(ui_sans_font(theme))
                    .font_weight(FontWeight::BOLD)
                    .line_height(relative(Self::LETTER_LINE_HEIGHT))
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

    /// One button's own box: `flex-1 gap-1`, an icon and a label, at
    /// `button::Size::Xs`'s own type step and radius.
    ///
    /// # P3.63: the label carries its own anchor fields
    ///
    /// It used to be a bare `.child(label)` — a string with no anchor of its
    /// own, so a snapshot rooted on this button's id saw its box and nothing
    /// about what it painted: no `text`, `fg`, `text_width`, `font` or
    /// `clipped` (`native/mapping/repo-icon-popover.md` §6, "15
    /// field-presence" over the three always-visible buttons).
    /// [`AnchorSink::boxed_text`] is the primitive every other labelled
    /// anchor in this file already uses (the caption, the preview fallback)
    /// — it records the string under the same id as the box, so what the
    /// differ sees is what is actually painted.
    ///
    /// # P3.63: the width follows the label, not a flat three-way split
    ///
    /// Measured before this fix, the default cell (`--surface repo-icon-
    /// popover`), with none of the box's own chrome (no border, no padding,
    /// a full-size icon): Upload, Emoji and GitHub all landed at ~73px,
    /// whatever their own text said. `.flex_1()` is CSS `flex: 1 1 0%`, and
    /// growing every item on a `0%` basis by an equal share **without first
    /// clamping any of them to their own min-content** is what this crate's
    /// layout engine did — the CSS "automatic minimum size" step a browser
    /// applies before distributing leftover space, which this engine skips.
    ///
    /// **Still `.flex_1()`, not a substitute.** A first pass reached for
    /// `.flex_auto()` (`flex: 1 1 auto`) to route around the missing clamp —
    /// plausible in isolation, but not what `repo-icon-popover.tsx` writes,
    /// and once the box carries its own real border, padding and icon
    /// margin (below), the three buttons' own combined footprint
    /// (**220px**: `72 + 64 + 72 = 208`, plus the row's own two 6px gaps)
    /// already **exceeds** the row's own 198px share of the popup, so there
    /// is no leftover space for either `.flex_1()` or `.flex_auto()` to
    /// distribute — measured identical under both, `72/64/72`, once that
    /// floor is in place. With nothing left for the flex algorithm to
    /// decide, the literal, faithful class wins on the tie: `.flex_1()`,
    /// matching `repo-icon-popover.tsx` exactly rather than a workaround the
    /// fix upstream made unnecessary.
    ///
    /// # The re-verdict round: the surviving fields, all reused from `button.rs`
    ///
    /// The field-presence fix made `text`/`font`/`border`/`bounds.w`
    /// *comparable* for the first time, which is what surfaced these four —
    /// none is a regression (`repo-icon-popover.md` §8). `bounds.w` took two
    /// separate sub-fixes (padding, icon margin); the other three are one
    /// each. Every number below is `button::Size::Xs`'s own, already
    /// measured and gate-tested in `button.rs`; none is re-derived by hand,
    /// which is the mistake this port keeps re-learning (`native/
    /// MAPPING.md`'s own repeated warning about transferring a number
    /// instead of reading the type-scale table it actually belongs to).
    ///
    /// * **`border.w` 0 → 1.** `button.tsx`'s base class list carries a bare
    ///   `border` unconditionally, on every variant — `button.rs`'s own
    ///   module docs record this as "the opposite of every earlier trap":
    ///   `ghost`'s `border-transparent` changes the *colour*, never the
    ///   *width*. [`button::BORDER_WIDTH`] (1px) plus [`button::Variant::
    ///   border`] on `Variant::Ghost` (the colour, `Color::TRANSPARENT`) are
    ///   reused directly rather than re-typed.
    /// * **No horizontal padding at all.** `size="xs"` carries `px-[calc(
    ///   --spacing(2)-1px)]` — [`button::Size::padding_x`] returns this
    ///   already computed (7px), and this box had never called it.
    /// * **`font.weight` 400 → 500.** `button.tsx`'s base class list is
    ///   `font-medium`, on every button — `button::Button`'s own private
    ///   `shell` paints `FontWeight::MEDIUM` unconditionally, reused here as
    ///   the same literal. **Not** a `Styled::font`-overwrite footgun (P3.64's
    ///   finding, checked and ruled out): this box never called `.font(…)`
    ///   at all, so there was nothing to overwrite — the weight was simply
    ///   never set. The stray `.font(ui_sans_font(theme))` this box *did*
    ///   carry is dropped for the same reason `button::Button::shell` never
    ///   calls it: the family already reaches every leaf here by ordinary
    ///   inheritance from the harness's own root (`row_surface.rs`/
    ///   `row_layout.rs`), measured correctly on this exact box before this
    ///   fix (`family: "CalSansUI"`) — an explicit call was never load-
    ///   bearing and, on this box specifically, was one keystroke from being
    ///   the exact footgun P3.64 found.
    /// * **`font.line_height` 19.5 → 16.** The coordinator's own read is the
    ///   one that matters here: `font.size: 12` is `text-xs`, a **named**
    ///   Tailwind step with a *paired* line-height (`12/16`, ratio
    ///   `1.333…`) — not the `1.5` an *arbitrary* `text-[Npx]` inherits,
    ///   which is what the caption's own fix (`CAPTION_LINE_HEIGHT`) was
    ///   right to use and this box would have been wrong to copy. The step
    ///   this box already computed (`button::Size::Xs.type_step(theme,
    ///   BP).line_height`) *carries this exact ratio* — `button::
    ///   LINE_HEIGHT_XS` is `1.0 / 0.75` — and this box had computed `step`
    ///   for its own font *size* but never read `step.line_height` at all,
    ///   so the box fell back to gpui's golden-ratio default the same way
    ///   the caption and `avatar-fallback` did (`14 × phi` there; `12 ×
    ///   phi ≈ 19.42`, device-snapped to `19.5`, here).
    /// * **The icon's own margin, folded into its box rather than
    ///   declared.** `[&_svg]:-mx-0.5` pulls every button icon's neighbours
    ///   in by 2px a side — `button.rs`'s own module docs record this as
    ///   *"the largest finding in this port"*: a negative inline margin on
    ///   an in-flow, content-sized flex item breaks taffy's main-size
    ///   resolution outright, not by a fixed offset, so it cannot be
    ///   declared as a margin at all. `button::Button::glyph` resolves it by
    ///   shrinking the glyph's own box instead and applying **no** margin —
    ///   [`button::ICON_MARGIN_X`] is the measurement. This surface's own
    ///   icon is call-site-sized (`size-3`, [`BUTTON_ICON_SIZE`]) rather
    ///   than `button.rs`'s own computed [`button::Size::icon`], so
    ///   [`button::Size::glyph_box`] does not apply directly — the same
    ///   arithmetic does: `BUTTON_ICON_SIZE + button::ICON_MARGIN_X * 2.0`
    ///   is this box's own in-flow width (8px), height unchanged (the
    ///   margin is inline-axis only).
    ///
    /// # `bounds.w`/`bounds.x` — a declaration, not a residual
    ///
    /// Every non-text term above is `button.rs`'s own already-verified
    /// number: border, padding, icon margin, gap. What is left is the
    /// label's own **shaped** advance width, which `row_layout`'s own
    /// harness cannot verify — `#[gpui::test]`'s `TestPlatform` hardcodes a
    /// `NoopTextSystem` (`vendor/gpui/src/platform/test/platform.rs`), so
    /// *no* `#[gpui::test]` in this workspace ever shapes a real glyph; a
    /// width measured there is against a synthetic stand-in font, never
    /// `CalSansUI` — the same reason `row_layout::badge`'s own default-cell
    /// test declines to assert its own width to the pixel.
    ///
    /// **A first pass over-read that limitation.** It does mean this file's
    /// own tests cannot pin a magnitude — true, and why the width-ordering
    /// test below stays directional. It does **not** mean the width itself
    /// has an unclosable residual: the live binary *does* shape a real
    /// glyph, through the same `MacTextSystem` `main.rs` registers
    /// `CalSansUI` into, and re-verdicted there the three widths land on
    /// **exactly** `ceil()` of the reference — `ceil(69.63) = 70`,
    /// `ceil(59.77) = 60`, `ceil(69.56) = 70` — which is `ANCHORS.md` v1.5's
    /// own documented GPUI behaviour (`elements/text.rs` ceils a text run's
    /// max-content width where `WebKit` keeps the fraction), not noise.
    ///
    /// v1.5 already has an allowance for exactly this — every other
    /// geometry field in the snapshot gets `Σ(ceil excess)` over the anchors
    /// declared `content_sized` — **but only once both sides author the
    /// declaration**, and neither did: `content_sized` never detects, it is
    /// an authored argument on both extractors by design (v1.5's own text:
    /// "two extractors each guessing is the silent divergence this file
    /// exists to prevent"). Undeclared, `upload`'s and `emoji`'s own ceil
    /// excess (`0.37 + 0.23`) pushed `github`'s own `x` right by it:
    /// `github.bounds.x: 171.0` against `170.39`, a `0.61px` delta — real,
    /// if small, and exactly the shape the allowance exists to forgive, but
    /// inert for want of the two attributes. `data-oracle-content-sized="true"`
    /// on `repo-icon-popover.tsx`'s three `<Button>` elements (the existing
    /// pattern `inline-error.tsx`/`search.tsx`/`placeholder-row-actions.tsx`
    /// already use per call site, *not* a change to `button.tsx` itself —
    /// see the note below) and [`AnchorId::content_sized`] here close it.
    ///
    /// **Why the declaration is per call site, not in `button.tsx`.** The
    /// first attempt reached for the shared primitive, on button.rs's own
    /// module-doc claim that "no live call site renders a Button with a
    /// label." That claim is stale: a direct audit found **74 other** live,
    /// non-icon-sized `<Button>`s with visible text across the app, none of
    /// whose own Rust ports declare `content_sized` for their own anchors.
    /// Adding the attribute inside `button.tsx` would have put it on all 74
    /// unconditionally — new one-sided declarations (React `true`, Rust
    /// absent) on dozens of otherwise-passing, unrelated surfaces, which
    /// `ANCHORS.md` v1.5 and the differ's own test suite both treat as a
    /// contract defect that forgives nothing. Five call sites already carry
    /// this exact prop by hand for the identical reason; this file's three
    /// buttons now join them the same way, scoped to what this item
    /// verified rather than what a stale doc comment assumed.
    ///
    /// **`Reset to default` and `Set` are not declared, on the same
    /// discipline.** `Reset` (`full_width: true`) authors `w-full` — a real,
    /// definite width, not content-sized — so [`Self::render`] only opts
    /// `!full_width` in. `Set` (`emoji_row`, below) is not part of this
    /// item's own verdict cell either and is left undeclared rather than
    /// assumed.
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
            .px(button::Size::Xs.padding_x())
            .border(button::BORDER_WIDTH)
            .border_color(button::Variant::Ghost.border(theme, button::ButtonState::resting()))
            .text_color(theme.muted_foreground)
            .text_size(step.size.to_pixels(px(16.0)))
            .line_height(relative(step.line_height))
            .font_weight(FontWeight::MEDIUM)
            .child(
                div()
                    .flex_shrink_0()
                    .w(BUTTON_ICON_SIZE + button::ICON_MARGIN_X * 2.0)
                    .h(BUTTON_ICON_SIZE),
            );
        let mut anchor_id = AnchorId::from(id);
        shell = if full_width {
            shell.w_full()
        } else {
            // `flex-1` with no authored width — content-sized (`ANCHORS.md`
            // v1.5). `Reset to default` (`full_width: true`, `w-full`) is a
            // real, definite width and stays undeclared.
            anchor_id = anchor_id.content_sized();
            shell.flex_1()
        };
        anchors.boxed_text(anchor_id, shell, SharedString::new_static(label))
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
    /// `avatarURL`). No `avatarColor` either — `None`, matching the live
    /// reference's own `avatar-fallback.bg: #ffffff0a` (`theme.muted`, see
    /// [`PreviewAvatar::render`]'s own doc comment).
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            preview: PreviewAvatar::Letter {
                label: SharedString::new_static("R"),
                background: None,
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
            .font(ui_sans_font(theme))
            .line_height(relative(CAPTION_LINE_HEIGHT))
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
    ///
    /// # The popup composes `popover`'s own chrome instead of hand-rolling a
    /// plain box — the P3.63 fix
    ///
    /// `repo-icon-popover.tsx`'s `<PopoverContent>` **is** a
    /// `PopoverPrimitive.Popup` wrapping a `PopoverPrimitive.Viewport`
    /// (`web/src/components/ui/popover.tsx`); the call site's own
    /// `className="w-64 p-0"` only sets the popup's width and zeroes a
    /// padding utility the popup never had in the first place. Every other
    /// class on the popup — the **border**, the **radius** — and the
    /// viewport's own **16px padding** belong to the shared primitive,
    /// unconditionally, on every call site that uses it.
    ///
    /// This box used to skip both: a plain `div().w(POPUP_WIDTH)
    /// .bg(theme.popover)` with no border, no radius, and `inner` (the `p-3`
    /// column) sitting directly against it — no viewport box, and no
    /// `popover-viewport` anchor at all. A live check of this exact popup
    /// (`native/mapping/repo-icon-popover.md` §6, the P3.63 verdict) found
    /// both box arithmetics resolve exactly once the two missing terms are
    /// put back:
    ///
    /// ```text
    /// React: 1 (border) + 16 (viewport padding) + 12 (inner p-3) + 15 (caption) + 12 (gap) = 56 = avatar y ; popup h = 177
    /// port:  0          + 0                      + 12             + 16 (caption) + 12       = 40 = avatar y ; popup h = 144
    /// ```
    ///
    /// (`native/mapping/repo-icon-popover.md` §6's own version of the port's
    /// row writes `15` for the caption there too, which is off by one against
    /// the `40` it sums to — measured independently, on the actual pre-fix
    /// code, the port's caption was **also** unset and defaulted to gpui's
    /// golden-ratio line height, the same bug [`PreviewAvatar::
    /// LETTER_LINE_HEIGHT`]'s own doc comment fixes, landing on `16px` rather
    /// than `15`; `0 + 0 + 12 + 16 + 12` is the arithmetic that actually
    /// reaches `40`. [`CAPTION_LINE_HEIGHT`] closes that one too, alongside
    /// the border/viewport fix below.)
    ///
    /// `1 + 16 = 17` is the 16px `y` shift the border and viewport contribute
    /// to every child; `2 + 32 = 34` is most of the popup's own 33px height
    /// shortfall (`177 − 144`), the last pixel of which is the caption fix
    /// above.
    ///
    /// This reuses [`popover::BORDER_WIDTH`], [`popover::VIEWPORT_PADDING`]
    /// and [`popover::ID_VIEWPORT`] rather than re-deriving them, because
    /// they are not this surface's own numbers — they are `popover.tsx`'s,
    /// already measured live in `native/mapping/popover.md` §1–2 (`border`:
    /// `border-width: 1px`; the viewport: `py-4` / `px-(--viewport-inline-
    /// padding)`, both `--spacing(4)` = 16px). `popover-viewport` in
    /// particular is the primitive's own **generic**, unnamespaced id — not
    /// `repo-icon-popover-*` — the same reuse [`Self::emoji_row`] already
    /// makes of `input.rs`'s `input-control`/`input` two rows up, for the
    /// same reason: it is the id the real DOM actually carries, not a second
    /// name for it. `theme.radius_lg` is `popover::Variant::Default::radius`'s
    /// own token (10px), and `theme.border` is the same border colour
    /// [`popover::Popover::popup`] paints.
    ///
    /// **This is composition, not a call to [`popover::Popover::render`].**
    /// That constructor roots itself at [`popover::ID_POPUP`] (`"popover-
    /// popup"`, collides with this surface's own namespaced
    /// [`ID_POPUP`]) and opens through `gpui_component::Popover`'s deferred,
    /// anchored placement — real machinery this call site does not want: per
    /// the module docs, the popup is driven directly, with no live trigger,
    /// through `row_layout`'s own harness. So the two boxes
    /// `popover::Popover::popup`/`::viewport` build are reproduced here
    /// directly, at this surface's own root id, rather than reached through
    /// the wrapped primitive — the same "sibling wraps stay independent"
    /// posture `dropdown.rs`'s module docs record for its own trigger.
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

        // `PopoverPrimitive.Viewport`: `relative size-full overflow-clip`
        // with `popover.tsx`'s own 16px padding on every side. See the
        // method's own doc comment for the measurement this closes.
        let viewport = anchors.boxed(
            AnchorId::from(popover::ID_VIEWPORT),
            div()
                .relative()
                .w_full()
                .px(popover::VIEWPORT_PADDING)
                .py(popover::VIEWPORT_PADDING)
                .overflow_hidden()
                .child(inner),
        );

        // `PopoverPrimitive.Popup`: `rounded-lg border bg-popover`, `w-64`
        // from this call site's own `className`.
        let popup = div()
            .flex()
            .w(POPUP_WIDTH)
            .rounded(theme.radius_lg.value())
            .border(popover::BORDER_WIDTH)
            .border_color(theme.border)
            .bg(theme.popover)
            .child(viewport);

        anchors.root(AnchorId::from(ID_POPUP), popup)
    }
}

#[cfg(test)]
mod tests {
    use super::{
        CAPTION_LINE_HEIGHT, CAPTION_TEXT, CONTENT_SIZED, EMOJI_ROW_HEIGHT, ID_EMOJI,
        ID_EMOJI_SUBMIT, ID_GITHUB, ID_POPUP, ID_RESET, ID_TRIGGER, ID_UPLOAD, LINE_SIZED,
        POPUP_GAP, POPUP_PADDING, POPUP_WIDTH, PopupContent, PreviewAvatar, ROW_GAP,
        TRIGGER_EMOJI_TEXT, TRIGGER_LETTER_TEXT, TRIGGER_SIZE, Trigger,
    };
    use crate::components::popover;
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

    /// `CONTENT_SIZED` carries exactly the three always-visible buttons —
    /// `Reset`/`Set` stay out, on the same discipline `ActionButton::
    /// render`'s own doc comment states. `LINE_SIZED` stays empty: no box
    /// here is a leading-none line box (`ANCHORS.md` v1.6).
    #[test]
    fn content_sized_names_the_three_flex_1_buttons_only() {
        assert_eq!(CONTENT_SIZED, [ID_UPLOAD, ID_EMOJI, ID_GITHUB]);
        assert!(!CONTENT_SIZED.contains(&ID_RESET));
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

    /// **The three P3.63 line-height fixes each land on their own utility's
    /// real ratio** — `text-sm` (14×20), `text-2xl` (24×32), and the
    /// caption's own Tailwind-preflight default (10×15) — and none of them
    /// is gpui's golden-ratio default (`~1.618034`) for a leaf that never set
    /// one, which is the bug each of the three closes.
    #[test]
    fn the_three_line_height_fixes_land_on_their_own_utilitys_ratio() {
        // gpui's own unset-leaf default (`gpui::phi()`) is the golden ratio,
        // `~1.618034` — measured, on this exact surface, to produce a
        // `22.5px` line box at `avatar-fallback`'s 14px against a live
        // reference of `20px` (`native/mapping/repo-icon-popover.md` §6).
        const GOLDEN_RATIO: f32 = 1.618_034;

        assert!((PreviewAvatar::LETTER_LINE_HEIGHT - 1.25 / 0.875).abs() < f32::EPSILON);
        assert!((PreviewAvatar::EMOJI_LINE_HEIGHT - 2.0 / 1.5).abs() < f32::EPSILON);
        assert!((CAPTION_LINE_HEIGHT - 1.5).abs() < f32::EPSILON);

        let letter_box = 14.0 * PreviewAvatar::LETTER_LINE_HEIGHT;
        assert!((letter_box - 20.0).abs() < 0.001, "{letter_box}");
        let emoji_box = 24.0 * PreviewAvatar::EMOJI_LINE_HEIGHT;
        assert!((emoji_box - 32.0).abs() < 0.001, "{emoji_box}");
        let caption_box = 10.0 * CAPTION_LINE_HEIGHT;
        assert!((caption_box - 15.0).abs() < 0.001, "{caption_box}");

        // None of the three fixed ratios is anywhere near the golden ratio.
        for ratio in [
            PreviewAvatar::LETTER_LINE_HEIGHT,
            PreviewAvatar::EMOJI_LINE_HEIGHT,
            CAPTION_LINE_HEIGHT,
        ] {
            assert!((ratio - GOLDEN_RATIO).abs() > 0.05, "{ratio}");
        }
    }

    /// **The popup composition reuses `popover`'s own constants rather than
    /// re-deriving them** — `BORDER_WIDTH` (1px) and `VIEWPORT_PADDING`
    /// (16px) are the values `popover.md` §1–2 already measured live on this
    /// exact popup.
    #[test]
    fn the_popup_reuses_popovers_own_border_and_viewport_constants() {
        assert_eq!(popover::BORDER_WIDTH, px(1.0));
        assert_eq!(popover::VIEWPORT_PADDING, px(16.0));
        // `popover-viewport` is the primitive's own **generic** id — not
        // namespaced under this surface's own prefix, unlike every id
        // `every_anchor_id_is_distinct_and_namespaced` checks above, because
        // it is the id the real DOM actually carries on this call site.
        assert!(!popover::ID_VIEWPORT.starts_with("repo-icon-popover"));
        assert_ne!(popover::ID_VIEWPORT, ID_POPUP);
    }
}
