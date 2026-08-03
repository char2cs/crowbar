//! `repo-avatar` — a repo's identity mark, and the first component in the
//! port whose single element **is** its own root: unlike [`super::avatar`],
//! nothing here ever wraps a persistent parent around a child that comes and
//! goes.
//!
//! The native half of `web/src/components/layout/repo-avatar.tsx`: three
//! independent pictures — an emoji glyph, a fetched icon (with its own
//! `onError` fallback), or a coloured initial — of which the React component
//! renders exactly **one** `<span>` or `<img>` per call, never a wrapper
//! around any of them. `RepoAvatarImg`'s `errored` state is local React
//! state, not a prop, so the port models it the way [`super::avatar`] models
//! `ImageStatus`: as an input the caller drives, not as a store this crate
//! could read. Every "Compiles to" below came from running the app's own
//! `web/src/index.css` through its own `tailwindcss` 4.3.0 with the utility as
//! a candidate — the method `native/MAPPING.md` fixes. See
//! `native/mapping/repo-avatar.md`.
//!
//! # `avatar.color` is data this component cannot resolve, and the port says so
//!
//! The letter fallback's own background is `avatar.color` — a Tailwind `bg-*`
//! class **chosen per repository and stored in the workspace record**
//! (`repo.avatarColor`, threaded through unmodified by every call site:
//! `new-tab-view.tsx`, `context-pill-model.ts`, `workspace-switcher-model.ts`).
//! `avatar.md` already recorded this exact finding for `repo-icon-popover.tsx`'s
//! third fallback and ruled it **not modelled** — "a port cannot resolve a
//! class name that arrives as data, and inventing one would put a colour on
//! screen for a perceptual oracle to converge on." The difference here is that
//! this component's letter fallback is the **dominant, live, capturable** path
//! (unlike `repo-icon`'s, which sits behind an unreachable popover), so the
//! port cannot simply omit the branch — it takes the resolved colour as
//! [`RepoAvatar::background`], a caller-supplied [`Color`], and paints
//! whatever it is handed. There is no fixture reference for this one field:
//! the values observed across the test suite include real Tailwind classes
//! (`bg-indigo-700`, `bg-red-500`, `bg-sky-700`, `bg-violet-700`) *and*
//! strings that are not Tailwind classes at all (`avatar-rose`, `avatar-pink`,
//! `avatar-emerald` — no `.avatar-*` rule exists anywhere in the stylesheets,
//! so a real repo assigned one of those would render **no background at all**).
//! [`RepoAvatar::fixture`] uses `theme.primary` as a placeholder with no claim
//! to being any repo's actual colour.
//!
//! # No `data-oracle-id` exists on this file today
//!
//! Checked rather than assumed: `repo-avatar.tsx` carries no `data-oracle-id`
//! anywhere, on any element, in any branch — and neither does any call site
//! that renders it. This is a different situation from every surface ported so
//! far (`avatar.tsx`'s ids come from the primitive itself; `popover`/`select`
//! had theirs landed by a dedicated prerequisite item, P3.15, before P3.18
//! could declare their anchor sets). A live oracle capture of this surface
//! needs the same kind of prerequisite — `data-oracle-id="repo-avatar"` landed
//! on the React source — which this item does not do, since deciding *where*
//! that id goes across four call sites is exactly the kind of call P3.15 made
//! its own commit. Flagged rather than worked around.

use gpui::{AnyElement, Div, FontWeight, Pixels, SharedString, Styled as _, div, px, relative};

use super::anchor::{AnchorId, AnchorSink};
use crate::theme::{Color, Theme, ui_sans_font};

/// The single anchor this surface carries. `repo-avatar.tsx` renders exactly
/// one element per call — there is no persistent wrapper the way
/// [`super::avatar::ID_ROOT`] is one — so every [`Kind`] shares this id.
pub const ID: &str = "repo-avatar";

/// **Empty.** Every box on this surface is `h-*`/`w-*` (`sizeClasses`), so no
/// anchor's width comes from what it contains — `repo-avatar.tsx` carries no
/// `overflow-hidden` either, so a label longer than its box **overflows**
/// rather than being clipped, which is the one thing `--content` can show
/// here (see the traps table in `native/mapping/repo-avatar.md`).
pub const CONTENT_SIZED: [&str; 0] = [];

/// **Empty**, for the same reason as [`CONTENT_SIZED`]: every box is
/// authored (`h-4`/`h-5`/`h-6`), not derived from a line box.
pub const LINE_SIZED: [&str; 0] = [];

/// `--spacing`, Tailwind's stock `0.25rem` at a 16px root.
const SPACING: f32 = 4.0;

/// The letter fallback's `text-[10px]` — an arbitrary Tailwind value with no
/// token behind it, `git_status_row`'s `BADGE_TEXT_SM`'s shape.
const LETTER_TEXT_SM: f32 = 10.0;
/// The letter fallback's `text-[11px]`.
const LETTER_TEXT_LG: f32 = 11.0;
/// The letter fallback's `text-[13px]`.
const LETTER_TEXT_XL: f32 = 13.0;

/// The emoji span's `text-lg` (xl size): `1.125rem`. **No `--ui-text-*` token
/// carries this number** — the nearest neighbours are `ui_text_lg` (1rem) and
/// `ui_text_xl` (1.25rem), each 0.125rem off — so unlike `text-xs`/`text-sm`
/// below, this one cannot be read through the token trade `dropdown-menu.md`
/// establishes. Carried as a raw literal the way `git_status_row`'s
/// `BADGE_TEXT_SM` is.
const EMOJI_TEXT_LG_REM: f32 = 1.125;

/// `sm` / `lg` / `xl` — `repo-avatar.tsx`'s own `sizeClasses` keys, verbatim.
/// **There is no `md`**: the scale jumps from `sm` straight to `lg`, which is
/// a fact about the source, not an omission here.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Size {
    /// `h-4 w-4` box, `text-[10px]` letter, `text-xs` emoji.
    #[default]
    Sm,
    /// `h-5 w-5` box, `text-[11px]` letter, `text-sm` emoji.
    Lg,
    /// `h-6 w-6` box, `text-[13px]` letter, `text-lg` emoji — the New Tab
    /// headline size, per the source's own comment.
    Xl,
}

/// Every size, for `--help` and the closed-vocabulary test.
pub const ALL_SIZES: [Size; 3] = [Size::Sm, Size::Lg, Size::Xl];

impl Size {
    /// Its word on the command line and in a caption.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Sm => "sm",
            Self::Lg => "lg",
            Self::Xl => "xl",
        }
    }

    /// `h-*`/`w-*` — the box, both axes.
    #[must_use]
    pub const fn extent(self) -> Pixels {
        match self {
            Self::Sm => px(SPACING * 4.0),
            Self::Lg => px(SPACING * 5.0),
            Self::Xl => px(SPACING * 6.0),
        }
    }

    /// The letter fallback's own `text-[…px]` — an arbitrary literal, not a
    /// token.
    #[must_use]
    pub const fn letter_text(self) -> Pixels {
        match self {
            Self::Sm => px(LETTER_TEXT_SM),
            Self::Lg => px(LETTER_TEXT_LG),
            Self::Xl => px(LETTER_TEXT_XL),
        }
    }

    /// The emoji span's own `text-xs`/`text-sm`/`text-lg`, read through the
    /// token that carries the same number where one exists (`text-xs` ==
    /// `--ui-text-sm`, `text-sm` == `--ui-text-base`, the trade
    /// `dropdown-menu.md` establishes), and the raw literal where none does
    /// (`text-lg` — see [`EMOJI_TEXT_LG_REM`]).
    #[must_use]
    pub fn emoji_text(self, theme: &Theme) -> Pixels {
        match self {
            Self::Sm => theme.ui_text_sm.value().to_pixels(px(16.0)),
            Self::Lg => theme.ui_text_base.value().to_pixels(px(16.0)),
            Self::Xl => px(EMOJI_TEXT_LG_REM * 16.0),
        }
    }
}

/// Whether the fetched `<img>` loaded or errored — `RepoAvatarImgAttempt`'s
/// local `errored` state, driven here rather than computed.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum ImageState {
    /// The bytes arrived: the `<img>` is in the document.
    #[default]
    Loaded,
    /// `onError` fired: the `<img>` unmounts and the letter fallback takes
    /// its place — the same picture as [`Kind::Letter`].
    Errored,
}

impl ImageState {
    /// Its word on the command line and in a caption.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Loaded => "loaded",
            Self::Errored => "errored",
        }
    }
}

/// Every image state, for `--help` and the closed-vocabulary test.
pub const ALL_IMAGE_STATES: [ImageState; 2] = [ImageState::Loaded, ImageState::Errored];

/// Which of the three pictures this cell renders — `RepoAvatar`'s own
/// three-way branch, `avatar.url?.startsWith('emoji:')` /
/// `avatar.url` / neither.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Kind {
    /// `avatar.url` starts with `"emoji:"` — the sliced glyph, no background,
    /// no border.
    Emoji,
    /// `avatar.url` is a real URL. [`ImageState::Loaded`] renders the `<img>`;
    /// [`ImageState::Errored`] renders the same letter fallback [`Self::Letter`]
    /// does.
    Image(ImageState),
    /// No `avatar.url` at all: the letter fallback directly. Pixel-identical
    /// to `Image(ImageState::Errored)` — same box, same classes, same
    /// content — which is a fact about the source (`letterFallback` is one
    /// value both branches return), not a coincidence collapsed here.
    Letter,
}

/// One `<RepoAvatar>`.
#[derive(Clone, Debug, PartialEq)]
pub struct RepoAvatar {
    /// Which picture this cell renders. See [`Kind`].
    pub kind: Kind,
    /// `sizeClasses`'s key. See [`Size`].
    pub size: Size,
    /// `avatar.label`, verbatim — `repo-avatar.tsx` does not slice or
    /// transform it the way `MessageAvatar` slices `display.name`. Read only
    /// when [`Self::kind`] resolves to the letter fallback.
    pub label: SharedString,
    /// `avatar.url.slice(6)` — the emoji glyph after `"emoji:"`. Read only
    /// when [`Self::kind`] is [`Kind::Emoji`].
    pub emoji: SharedString,
    /// `avatar.color`, resolved. See the module docs: this is genuinely
    /// unresolvable data, so the caller supplies it. Read only when
    /// [`Self::kind`] resolves to the letter fallback.
    pub background: Color,
}

impl RepoAvatar {
    /// A representative cell: the letter fallback, `sm`, no captured
    /// reference for [`Self::background`] — see the module docs.
    #[must_use]
    pub fn fixture(theme: &Theme) -> Self {
        Self {
            kind: Kind::Letter,
            size: Size::Sm,
            label: SharedString::new_static("R"),
            emoji: SharedString::new_static("\u{1f98a}"), // the test suite's own 🦊
            background: theme.primary,
        }
    }

    /// Whether [`Self::kind`] renders the letter fallback — reached from two
    /// different branches of the source and one picture here.
    #[must_use]
    pub const fn shows_letter(&self) -> bool {
        matches!(self.kind, Kind::Letter | Kind::Image(ImageState::Errored))
    }

    /// Renders the avatar, opting its one anchor into `anchors`.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        match self.kind {
            Kind::Emoji => anchors.boxed_text(
                AnchorId::from(ID),
                self.emoji_box(theme),
                self.emoji.clone(),
            ),
            Kind::Image(ImageState::Loaded) => {
                anchors.boxed(AnchorId::from(ID), self.image_box(theme))
            }
            Kind::Image(ImageState::Errored) | Kind::Letter => anchors.boxed_text(
                AnchorId::from(ID),
                self.letter_box(theme),
                self.label.clone(),
            ),
        }
    }

    /// `'inline-flex shrink-0 items-center justify-center leading-none',
    /// box, emoji` — no background, no radius, no border: the emoji glyph
    /// paints its own colour and `avatar.color` is not in this branch's
    /// class list at all.
    fn emoji_box(&self, theme: &Theme) -> Div {
        let extent = self.size.extent();
        div()
            .flex()
            .flex_shrink_0()
            .items_center()
            .justify_center()
            .w(extent)
            .h(extent)
            .line_height(relative(1.0))
            .font(ui_sans_font(theme))
            .text_size(self.size.emoji_text(theme))
            .text_color(theme.foreground)
    }

    /// `'shrink-0 rounded-sm object-cover', box` — an **empty** box, the same
    /// call `avatar.rs`'s `.image()` and `git_status_row`'s `.icon()` make: a
    /// fetched photograph has no native equivalent, and drawing a substitute
    /// would put pixels on screen for a perceptual oracle to converge on.
    /// Unlike `avatar.tsx`'s image (radius 0), this one carries its own
    /// `rounded-sm` — a real, measured difference between the two ports.
    fn image_box(&self, theme: &Theme) -> Div {
        let extent = self.size.extent();
        div()
            .flex_shrink_0()
            .w(extent)
            .h(extent)
            .rounded(theme.radius_sm.value())
    }

    /// `'inline-flex shrink-0 items-center justify-center rounded-sm px-0.5
    /// font-bold text-primary-foreground', box, text, avatar.color` — the
    /// letter fallback, reached from [`Kind::Letter`] and
    /// [`Kind::Image(ImageState::Errored)`] alike.
    ///
    /// `rounded-sm`, not `rounded-full`: unlike `avatar.tsx`'s circular
    /// fallback, this one is a small rounded square — a real shape difference
    /// between the two components, not an approximation.
    fn letter_box(&self, theme: &Theme) -> Div {
        let extent = self.size.extent();
        div()
            .flex()
            .flex_shrink_0()
            .items_center()
            .justify_center()
            .w(extent)
            .h(extent)
            .rounded(theme.radius_sm.value())
            .px(px(SPACING * 0.5))
            .bg(self.background)
            .font(ui_sans_font(theme))
            .text_size(self.size.letter_text())
            .font_weight(FontWeight::BOLD)
            .text_color(theme.primary_foreground)
    }
}

#[cfg(test)]
mod tests {
    use super::{
        ALL_IMAGE_STATES, ALL_SIZES, CONTENT_SIZED, ID, ImageState, Kind, LINE_SIZED, RepoAvatar,
        Size,
    };
    use crate::theme::Theme;
    use gpui::{FontWeight, SharedString, px};

    /// Every box on this surface is authored, so neither declaration list has
    /// an entry.
    #[test]
    fn neither_declaration_list_has_an_entry() {
        assert!(CONTENT_SIZED.is_empty());
        assert!(LINE_SIZED.is_empty());
    }

    /// The three sizes' boxes, read off `sizeClasses` directly — and there is
    /// no `md` between `sm` and `lg`, which is a fact about the source.
    #[test]
    fn the_three_sizes_are_16_20_and_24_with_no_md_between() {
        assert_eq!(Size::Sm.extent(), px(16.0));
        assert_eq!(Size::Lg.extent(), px(20.0));
        assert_eq!(Size::Xl.extent(), px(24.0));
        let names: Vec<&str> = ALL_SIZES.iter().map(|s| s.name()).collect();
        assert_eq!(names, ["sm", "lg", "xl"]);
    }

    /// The letter fallback's own `text-[…px]` is arbitrary, not a token.
    #[test]
    fn the_letter_sizes_are_the_arbitrary_pixel_values() {
        assert_eq!(Size::Sm.letter_text(), px(10.0));
        assert_eq!(Size::Lg.letter_text(), px(11.0));
        assert_eq!(Size::Xl.letter_text(), px(13.0));
    }

    /// The emoji span's `text-xs`/`text-sm` read the tokens that carry the
    /// same number; `text-lg` has none and falls back to the raw literal.
    #[test]
    fn the_emoji_sizes_trade_a_token_where_one_exists() {
        let theme = Theme::DARK;
        assert_eq!(Size::Sm.emoji_text(&theme), px(12.0), "text-xs");
        assert_eq!(Size::Lg.emoji_text(&theme), px(14.0), "text-sm");
        assert_eq!(Size::Xl.emoji_text(&theme), px(18.0), "text-lg, no token");
    }

    /// `Kind::Letter` and `Kind::Image(Errored)` are the same picture — a
    /// fact about the source (`letterFallback` is one value both branches
    /// return), asserted rather than assumed.
    #[test]
    fn the_letter_fallback_is_reached_from_two_kinds() {
        let mut avatar = RepoAvatar::fixture(&Theme::DARK);
        avatar.kind = Kind::Letter;
        assert!(avatar.shows_letter());
        avatar.kind = Kind::Image(ImageState::Errored);
        assert!(avatar.shows_letter());
        avatar.kind = Kind::Image(ImageState::Loaded);
        assert!(!avatar.shows_letter());
        avatar.kind = Kind::Emoji;
        assert!(!avatar.shows_letter());
    }

    /// The fixture is the letter fallback at `sm`, with the test suite's own
    /// glyph carried for the emoji field even though this cell does not show
    /// it.
    #[test]
    fn the_fixture_is_the_letter_fallback() {
        let theme = Theme::DARK;
        let avatar = RepoAvatar::fixture(&theme);
        assert_eq!(avatar.kind, Kind::Letter);
        assert_eq!(avatar.size, Size::Sm);
        assert_eq!(avatar.label, SharedString::new_static("R"));
        assert_eq!(avatar.background, theme.primary);
    }

    /// The vocabularies are closed and every word round-trips.
    #[test]
    fn the_vocabularies_are_closed() {
        assert_eq!(ALL_SIZES.len(), 3);
        assert_eq!(ALL_IMAGE_STATES.len(), 2);
        assert_eq!(ImageState::Loaded.name(), "loaded");
        assert_eq!(ImageState::Errored.name(), "errored");
    }

    /// `font-bold` is Tailwind's 700 — the primitive's own weight, distinct
    /// from `avatar.tsx`'s message-avatar semibold (600).
    #[test]
    fn the_letter_weight_is_bold_not_semibold() {
        assert_eq!(FontWeight::BOLD, FontWeight(700.0));
        assert_ne!(FontWeight::BOLD, FontWeight(600.0));
    }

    /// The single shared anchor id.
    #[test]
    fn the_id_is_repo_avatar() {
        assert_eq!(ID, "repo-avatar");
    }
}
