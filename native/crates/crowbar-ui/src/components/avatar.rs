//! `avatar` — the port's **first image primitive**, and the first component
//! whose element set depends on something neither engine lays out: whether a
//! network image arrived.
//!
//! The native half of `web/src/components/ui/avatar.tsx`, three thin wrappers
//! over `@base-ui/react`'s `Avatar`. Every value below came out of the app's own
//! `tailwindcss` 4.3.0 with the utility as a candidate — the method
//! `native/MAPPING.md` fixes. See `native/mapping/avatar.md`.
//!
//! # The image is not hidden before it loads. It **does not exist**
//!
//! This is the finding the item exists to record, and it is a fact about
//! `base-ui` rather than about CSS. `AvatarImage` computes
//! `isVisible = imageLoadingStatus === 'loaded'`, feeds it to `useTransitionStatus`
//! and then does
//!
//! ```text
//! if (!mounted) { return null }
//! ```
//!
//! — so until the bytes arrive there is **no `<img>` in the document at all**.
//! `AvatarFallback` is the mirror image: `useRenderElement(…, { enabled:
//! imageLoadingStatus !== 'loaded' })`, so it unmounts the moment the image
//! lands. Exactly one of the two is in the tree, and which one is a runtime
//! property.
//!
//! What that means for the contract is worth stating plainly, because it is a
//! shape no earlier surface had: **the two states differ in their anchor set,
//! not in a field.** An unloaded avatar has no `avatar-image` anchor for the
//! differ to report `visible: false` on — the anchor is simply absent, which
//! `ANCHORS.md` ranks first. So a cell has to be driven to the same
//! [`ImageStatus`] on both sides or the comparison is between two different
//! documents.
//!
//! Both states were captured live, from the same review thread:
//!
//! | | anchors | fallback |
//! |---|---|---|
//! | a message from `char2cs` | `avatar`, `avatar-image` | absent |
//! | a message from an agent (`avatarUrl` is `null`) | `avatar`, `avatar-fallback` | `AG`, 24×24, `bg-muted` |
//!
//! # `rounded-full` is **not** gpui's `rounded_full()`
//!
//! Tailwind 4 compiles `rounded-full` to `border-radius: calc(infinity * 1px)`,
//! and `WebKit` resolves that to the largest single-precision float:
//! `getComputedStyle().borderTopLeftRadius` on the live avatar is
//! `"340282346638528859811704183484516925440px"`, which is exactly `f32::MAX`.
//! gpui's `Styled::rounded_full()` is `px(9999.)`. Both paint the same circle —
//! every renderer clamps a corner radius to half the box — but `radius` is a
//! **compared field**, at ±0.5, and 9999 against 3.4e38 is a delta of 3.4e38.
//!
//! So [`FULL_RADIUS`] is `px(f32::MAX)`, and that is not a fudge: `f32::MAX`
//! widened to `f64` is the same integer `WebKit` printed, both extractors' round
//! -to-a-thousandth leaves it unchanged, and the two agree **exactly**. Verified
//! against the captured reference rather than argued.
//!
//! # What is visible to the differ, per axis
//!
//! | Axis | Here |
//! |---|---|
//! | `--width` | **vacuous.** Every box on this surface is `size-*` or `size-full`; nothing is a share of anything |
//! | `--viewport-width` | **vacuous.** There is not one `sm:` variant in `avatar.tsx` — the first component in the port with none |
//! | `--theme` | **real.** `bg-background` on the root and `bg-muted` on the fallback both move |
//! | `--content` | real **only with a fallback**, whose initials are the only text on the surface |

use gpui::{
    AnyElement, Div, FontWeight, ParentElement as _, Pixels, Rems, SharedString, Styled as _, div,
    px, relative,
};

use super::anchor::{AnchorId, AnchorSink};
use crate::theme::{Color, Theme};

/// The root anchor: `Avatar`, the `[data-slot=avatar]` span. Every other bound
/// on this surface is reported relative to it (`native/oracle/ANCHORS.md` §4).
pub const ID_ROOT: &str = "avatar";

/// `AvatarImage` — the `<img>`, which exists **only once the image has loaded**.
/// See the module docs.
pub const ID_IMAGE: &str = "avatar-image";

/// `AvatarFallback` — the initials, which exist only while it has **not**.
pub const ID_FALLBACK: &str = "avatar-fallback";

/// The anchors on this surface whose boxes size to their own text
/// (`native/oracle/ANCHORS.md` v1.5).
///
/// **Empty, and here that is a measurement rather than a caution.** The root is
/// `size-8` (or a call site's `size-6` / `size-14`) and both children are
/// `size-full`, so no box on this component takes its width from anything it
/// contains. The fallback is the one that paints text and it is pinned at 100%
/// of a square parent — measured live at 24×24 around a 17.33px run.
///
/// An empty list rather than no list, for P2.1's reason: the React side's (also
/// empty) declaration set has to be diffable against this one.
pub const CONTENT_SIZED: [&str; 0] = [];

/// The anchors on this surface whose **box height is their own line box**
/// (`native/oracle/ANCHORS.md` v1.6).
///
/// **Empty, for the same reason and one more.** `size-full` is an authored
/// height, so the fallback's box is its parent's and not its line box — measured
/// live at **24** against a **16px** line box, which is the v1.6 trap at a
/// ratio of 1.5. The root and the image paint no text at all, and v1.6 refuses
/// a `line_sized` anchor without a `font`.
pub const LINE_SIZED: [&str; 0] = [];

/// `--spacing`, Tailwind's stock `0.25rem` at a 16px root.
const SPACING: f32 = 4.0;

/// `rounded-full`, which is **not** gpui's `rounded_full()`.
///
/// `calc(infinity * 1px)` resolved by `WebKit`, which is `f32::MAX` and prints
/// as `340282346638528859811704183484516925440px`. See the module docs for why
/// the port spells it out rather than reaching for the preset.
pub const FULL_RADIUS: Pixels = px(f32::MAX);

/// `size-8` on the primitive — 32px, both axes.
pub const SIZE_DEFAULT: Pixels = px(SPACING * 8.0);

/// `size-6` — the `review-thread-item.tsx` message avatar, and the reference.
pub const SIZE_MESSAGE: Pixels = px(SPACING * 6.0);

/// `size-14` — the `repo-icon-popover.tsx` icon editor.
pub const SIZE_REPO_ICON: Pixels = px(SPACING * 14.0);

/// `mt-0.5` on the message avatar — 2px, and **outside every bound the snapshot
/// carries**: §4 reports geometry relative to the root anchor, and the root is
/// the element the margin is on.
pub const MESSAGE_MARGIN_TOP: Pixels = px(SPACING * 0.5);

/// `font-medium` on the primitive: `--font-weight-medium`, 500.
pub const WEIGHT_MEDIUM: FontWeight = FontWeight::MEDIUM;

/// `font-semibold` on the message avatar and its fallback: 600.
pub const WEIGHT_SEMIBOLD: FontWeight = FontWeight::SEMIBOLD;

/// `font-bold` on one of `repo-icon-popover`'s three fallbacks: 700.
///
/// Named although nothing here reads it, so that the one weight this port
/// **does not** model is visible rather than absent — see [`CallSite::RepoIcon`].
pub const WEIGHT_BOLD: FontWeight = FontWeight::BOLD;

/// Tailwind's line-height for `text-xs`: `calc(1 / 0.75)`.
///
/// Measured on the live avatar: a 12px face reports `lineHeight: "16px"`.
const LINE_HEIGHT_XS: f32 = 1.0 / 0.75;

/// Tailwind's line-height for `text-base`: `calc(1.5 / 1)`.
const LINE_HEIGHT_BASE: f32 = 1.5;

/// A type step: the size and the unitless line height Tailwind pairs with it.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct TypeStep {
    /// `font-size`.
    pub size: Rems,
    /// `line-height`, unitless.
    pub line_height: f32,
}

/// Whether the `<img>` is in the document, which on this component is the whole
/// of its state.
///
/// **A runtime property of a network fetch**, not a prop and not a class — see
/// the module docs. It is driven by the surface's own `--image` option rather
/// than by §8.3's `loading` flag, for the reason `button`'s `loading` prop is:
/// the flag reaches nothing here, so driving it cannot fail, while this option
/// selects a picture the reference really has.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum ImageStatus {
    /// `imageLoadingStatus === 'loaded'`: the `<img>` is mounted and the
    /// fallback is not. The reference's own state.
    #[default]
    Loaded,
    /// `'idle' | 'loading' | 'error'`: the `<img>` is `null` and the fallback
    /// renders. All three are one picture — `AvatarFallback` asks only
    /// `!== 'loaded'` — which is why this enum has three arms and not five.
    Pending,
    /// No `<AvatarImage>` at all, because the call site rendered none.
    ///
    /// `review-thread-item.tsx` does exactly this when a message has no
    /// `avatarUrl` — `{display.avatarUrl && <AvatarImage …>}` — which is the
    /// case for every agent message. Indistinguishable from [`Self::Pending`]
    /// in the document, and different in the app: one settles and one never
    /// will.
    Absent,
}

/// Every image status, in the order the loading sequence reaches them.
pub const ALL_IMAGE_STATUSES: [ImageStatus; 3] = [
    ImageStatus::Loaded,
    ImageStatus::Pending,
    ImageStatus::Absent,
];

impl ImageStatus {
    /// Its word on the command line and in a caption.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Loaded => "loaded",
            Self::Pending => "pending",
            Self::Absent => "absent",
        }
    }

    /// Whether the `<img>` is in the document.
    #[must_use]
    pub const fn mounted(self) -> bool {
        matches!(self, Self::Loaded)
    }

    /// Whether `AvatarFallback` is enabled, which is `!== 'loaded'`.
    #[must_use]
    pub const fn shows_fallback(self) -> bool {
        !self.mounted()
    }
}

/// The `className` a **call site** merges over the primitive's own.
///
/// The line P3.1 drew for `button`'s `--class-radius`: an arm names the
/// **classes** a call site literally writes, never a resolved number, so both
/// engines still compute the pixels from the same tokens.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum CallSite {
    /// No `className` — `size-8`, `rounded-full`, `text-xs`, `font-medium`.
    ///
    /// **No live call site leaves it alone**, so the primitive's own 32px box
    /// has no reference. Rendered anyway, for `resizable`'s reason about the
    /// grip.
    None,
    /// `mt-0.5 size-6 shrink-0 text-xs font-semibold` on the root and
    /// `text-xs font-semibold text-muted-foreground` on the fallback —
    /// `review-thread-item.tsx`'s `MessageAvatar`.
    ///
    /// The reference: the only live `<Avatar>` outside a popover.
    #[default]
    Message,
    /// `size-14 rounded-xl text-base` on the root — `repo-icon-popover.tsx`.
    ///
    /// The **only live avatar with a finite radius**, and the one that cannot be
    /// captured: it lives inside a `PopoverContent`, which is a portal that
    /// exists only while the popover is open, and synthetic pointer events are
    /// denied on this project's machines.
    ///
    /// Its three fallbacks are **not modelled**. Two of them are, structurally,
    /// the primitive's; the third writes `repo.avatarColor` — a Tailwind
    /// `bg-*` class chosen per repository and stored in the workspace record —
    /// and a port cannot resolve a class name that arrives as data. Recorded
    /// rather than approximated.
    RepoIcon,
}

/// Every call-site bundle.
pub const ALL_CALL_SITES: [CallSite; 3] = [CallSite::None, CallSite::Message, CallSite::RepoIcon];

impl CallSite {
    /// Its word on the command line and in a caption.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::None => "none",
            Self::Message => "message",
            Self::RepoIcon => "repo-icon",
        }
    }

    /// Whether this bundle has a live rendering the oracle can reach.
    ///
    /// `repo-icon` renders live and is **not capturable**, which is a different
    /// complaint from `none`, which does not render at all. Both answer `false`
    /// and the caption says which.
    #[must_use]
    pub const fn capturable(self) -> bool {
        matches!(self, Self::Message)
    }

    /// `size-*` — the root's box, both axes.
    #[must_use]
    pub const fn extent(self) -> Pixels {
        match self {
            Self::None => SIZE_DEFAULT,
            Self::Message => SIZE_MESSAGE,
            Self::RepoIcon => SIZE_REPO_ICON,
        }
    }

    /// `rounded-full`, or `repo-icon`'s `rounded-xl`.
    ///
    /// `rounded-xl` is `calc(var(--radius) * 1.4)` = **14px**, not Tailwind's
    /// stock 12: `theme.css` redefines the whole scale off one
    /// `--radius: 0.625rem`. Read through the token, never as a literal.
    #[must_use]
    pub fn radius(self, theme: &Theme) -> Pixels {
        match self {
            Self::None | Self::Message => FULL_RADIUS,
            Self::RepoIcon => theme.radius_xl.value(),
        }
    }

    /// `mt-0.5`, which only the message avatar writes.
    #[must_use]
    pub const fn margin_top(self) -> Pixels {
        match self {
            Self::Message => MESSAGE_MARGIN_TOP,
            Self::None | Self::RepoIcon => px(0.0),
        }
    }

    /// The type step the root sets and the fallback inherits.
    ///
    /// The token each step reads is the one carrying its number — the
    /// `--ui-text-*` trade `native/MAPPING.md` states once. `text-xs` is
    /// 0.75rem and so is `--ui-text-sm`; `text-base` is 1rem and so is
    /// `--ui-text-lg`.
    #[must_use]
    pub fn type_step(self, theme: &Theme) -> TypeStep {
        match self {
            Self::None | Self::Message => TypeStep {
                size: theme.ui_text_sm.value(),
                line_height: LINE_HEIGHT_XS,
            },
            Self::RepoIcon => TypeStep {
                size: theme.ui_text_lg.value(),
                line_height: LINE_HEIGHT_BASE,
            },
        }
    }

    /// `font-medium` on the primitive, `font-semibold` where the call site says
    /// so.
    #[must_use]
    pub const fn weight(self) -> FontWeight {
        match self {
            Self::None | Self::RepoIcon => WEIGHT_MEDIUM,
            Self::Message => WEIGHT_SEMIBOLD,
        }
    }

    /// The fallback's own text colour, where the call site names one.
    ///
    /// `review-thread-item.tsx` writes `text-muted-foreground`; the primitive
    /// names none, so the fallback inherits the root's, which inherits the
    /// document's.
    #[must_use]
    pub fn fallback_foreground(self, theme: &Theme) -> Color {
        match self {
            Self::Message => theme.muted_foreground,
            Self::None | Self::RepoIcon => theme.foreground,
        }
    }
}

/// Which of the three §8.3 content lengths the fallback shows.
///
/// `MessageAvatar` computes `(display.name || 'U').slice(0, 2).toUpperCase()`,
/// so the live string is always one or two characters. The overflow arm is
/// longer than the app can produce and is here because the box is authored:
/// `size-full` means a long run **overflows** the circle rather than resizing
/// it, which is the one thing on this surface `--content` can show.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Initials {
    /// One character — `'U'`, the fallback of the fallback.
    Short,
    /// Two characters, which is what `slice(0, 2)` gives every real name.
    /// The reference's own: an agent message renders `AG`.
    #[default]
    Normal,
    /// Longer than the circle. Not reachable from `MessageAvatar`, and
    /// reachable from `repo-icon-popover`, whose fallback renders
    /// `repo.avatarLabel` unsliced.
    Overflow,
}

impl Initials {
    /// The string this length shows.
    #[must_use]
    pub const fn text(self) -> &'static str {
        match self {
            Self::Short => "U",
            Self::Normal => "AG",
            Self::Overflow => "CROWBAR",
        }
    }
}

/// One `<Avatar>`.
#[derive(Clone, Debug, PartialEq)]
pub struct Avatar {
    /// The anchor the root span carries.
    ///
    /// Authored rather than fixed, because `avatar.tsx` writes [`ID_ROOT`]
    /// *before* `{...props}` so a call site can override it — P2.1's convention.
    /// No live call site does.
    pub id: SharedString,
    /// A call site's own `className`. See [`CallSite`].
    pub call_site: CallSite,
    /// Whether the `<img>` is mounted. See [`ImageStatus`].
    pub image: ImageStatus,
    /// The fallback's initials, or `None` when the call site renders no
    /// `<AvatarFallback>` at all.
    ///
    /// `None` is the `empty` cell: a root with no image and no fallback, which
    /// is a real branch of the primitive — `<Avatar>` takes its children from
    /// the call site and both live ones happen always to pass a fallback.
    pub initials: Option<Initials>,
}

impl Avatar {
    /// The avatar a reference can actually be taken from: `review-thread-item`'s
    /// message avatar with its image loaded.
    ///
    /// Read off the live app rather than chosen. There are two `<Avatar>` call
    /// sites in `web/src/` and only one of them is capturable:
    ///
    /// | call site | `className` | capturable |
    /// |---|---|---|
    /// | `review-thread-item.tsx` | `mt-0.5 size-6 shrink-0 text-xs font-semibold` | **yes** |
    /// | `repo-icon-popover.tsx` | `size-14 rounded-xl text-base` | no — it is inside a `PopoverContent` |
    ///
    /// Measured on the live one at a 1714px viewport: root `24×24`,
    /// `bg #1f1f1eff`, `radius 3.4028234663852886e38`, `border.w 0`; image
    /// `24×24`, `bg #00000000`, `radius 0`. The image's `src` resolves through a
    /// GitHub redirect to `avatars.githubusercontent.com`, so **the reference's
    /// state depends on the network** — see [`ImageStatus`].
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            id: SharedString::new_static(ID_ROOT),
            call_site: CallSite::Message,
            image: ImageStatus::Loaded,
            initials: Some(Initials::Normal),
        }
    }

    /// Renders the avatar, opting every contract anchor into `anchors`.
    ///
    /// Exactly one of the image and the fallback reaches the tree, which is
    /// `base-ui`'s own arrangement rather than a simplification — see the module
    /// docs. A call site that renders no fallback and whose image has not
    /// loaded produces a root with no children, and that is the `empty` cell.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let mut root = self.root(theme);

        if self.image.mounted() {
            root = root.child(anchors.boxed(ID_IMAGE.into(), Self::image()));
        } else if let Some(initials) = self.initials {
            root = root.child(anchors.boxed_text(
                ID_FALLBACK.into(),
                self.fallback(theme),
                SharedString::new_static(initials.text()),
            ));
        }

        anchors.root(AnchorId::new(self.id.clone()), root)
    }

    /// `inline-flex size-8 shrink-0 select-none items-center justify-center
    /// overflow-hidden rounded-full bg-background align-middle font-medium
    /// text-xs`, plus whatever the call site merged over it.
    ///
    /// `inline-flex` becomes `.flex()`: gpui has no inline flow, and the
    /// reference's own computed `display` is **`flex`** — measured live, because
    /// CSS blockifies the display of a flex item and the message avatar is one.
    ///
    /// `select-none` and `align-middle` are not visual properties in any sense
    /// the contract can see; `overflow-hidden` is, and it is what clips an
    /// overflowing fallback.
    ///
    /// The font family is named explicitly rather than left to inherit: gpui can
    /// only report the *declared* family, and an inherited `.SystemUIFont` is a
    /// string the DOM will never produce (`ANCHORS.md` v1.2).
    fn root(&self, theme: &Theme) -> Div {
        let extent = self.call_site.extent();
        let step = self.call_site.type_step(theme);
        let family = theme.font_sans.primary().unwrap_or("sans-serif");

        div()
            .flex()
            .flex_shrink_0()
            .items_center()
            .justify_center()
            .overflow_hidden()
            .mt(self.call_site.margin_top())
            .w(extent)
            .h(extent)
            .rounded(self.call_site.radius(theme))
            .bg(theme.background)
            .font_family(family)
            .text_size(step.size)
            .line_height(relative(step.line_height))
            .font_weight(self.call_site.weight())
            .text_color(theme.foreground)
    }

    /// `<AvatarImage className="size-full object-cover">`, as an **empty box**.
    ///
    /// The same call every component since `git_status_row` has made about
    /// icons, one step further out: this is a *photograph* fetched over the
    /// network, there is no native equivalent the port could draw, and drawing a
    /// substitute would put pixels on screen for a perceptual oracle to converge
    /// on. The box is what the contract measures — and the reference agrees, in
    /// the strongest form it has: `bg #00000000`, `radius 0`, `border.w 0`.
    ///
    /// `object-fit: cover` has no field and no effect on the box; it decides
    /// which crop of the bitmap fills it.
    fn image() -> Div {
        div().w_full().h_full()
    }

    /// `<AvatarFallback className="flex size-full items-center justify-center
    /// rounded-full bg-muted">`, plus the call site's own.
    ///
    /// `size-full` rather than a content-sized box, which is why neither
    /// declaration list has an entry for it — see [`CONTENT_SIZED`] and
    /// [`LINE_SIZED`].
    fn fallback(&self, theme: &Theme) -> Div {
        div()
            .flex()
            .items_center()
            .justify_center()
            .w_full()
            .h_full()
            .rounded(FULL_RADIUS)
            .bg(theme.muted)
            .font_weight(self.call_site.weight())
            .text_color(self.call_site.fallback_foreground(theme))
    }
}

#[cfg(test)]
mod tests {
    use super::{
        ALL_CALL_SITES, ALL_IMAGE_STATUSES, Avatar, CONTENT_SIZED, CallSite, FULL_RADIUS, ID_ROOT,
        Initials, LINE_SIZED, MESSAGE_MARGIN_TOP, SIZE_DEFAULT, SIZE_MESSAGE, SIZE_REPO_ICON,
        WEIGHT_BOLD, WEIGHT_SEMIBOLD,
    };
    use crate::theme::Theme;
    use gpui::{FontWeight, px};

    /// `rounded-full` is `calc(infinity * 1px)`, which `WebKit` resolves to the
    /// largest `f32`. gpui's own `rounded_full()` is `px(9999.)` and would be a
    /// 3.4e38 delta on a field compared at ±0.5.
    ///
    /// The literal here is the number the live capture printed, digit for digit.
    #[test]
    fn rounded_full_is_f32_max_and_not_gpuis_preset() {
        assert_eq!(FULL_RADIUS, px(f32::MAX));
        assert!(
            (f32::from(FULL_RADIUS) - 340_282_346_638_528_859_811_704_183_484_516_925_440.0).abs()
                < f32::EPSILON,
            "the reference prints borderTopLeftRadius: 340282346638528859811704183484516925440px",
        );
        assert!(
            f32::from(FULL_RADIUS) > 9999.0,
            "gpui's rounded_full() is px(9999.), which is not what WebKit computes",
        );
    }

    /// The two declaration lists, and the measurement behind each. The fallback
    /// is the one that reads like a candidate for both and is neither.
    #[test]
    fn neither_declaration_list_has_an_entry_and_the_fallback_is_why() {
        assert!(CONTENT_SIZED.is_empty());
        assert!(LINE_SIZED.is_empty());

        // `size-full` on a `size-6` parent: the box is 24 in both axes, and the
        // live fallback's line box is 16 (a 12px face at calc(1 / 0.75)).
        let theme = Theme::DARK;
        let step = CallSite::Message.type_step(&theme);
        let line = f32::from(step.size.to_pixels(px(16.0))) * step.line_height;
        assert!((line - 16.0).abs() < 0.01, "{line}");
        assert!(
            (f32::from(SIZE_MESSAGE) - line).abs() > 0.5,
            "24 against 16 — declaring line_sized would invent an 8px delta",
        );
    }

    /// The fixture is the live message avatar, and the numbers are the captured
    /// reference's.
    #[test]
    fn the_fixture_is_the_live_message_avatar() {
        let theme = Theme::DARK;
        let avatar = Avatar::fixture();

        assert_eq!(avatar.id, ID_ROOT);
        assert_eq!(avatar.call_site, CallSite::Message);
        assert!(avatar.image.mounted());
        assert!(!avatar.image.shows_fallback());

        assert_eq!(avatar.call_site.extent(), px(24.0), "size-6");
        assert_eq!(avatar.call_site.radius(&theme), FULL_RADIUS);
        assert_eq!(avatar.call_site.margin_top(), MESSAGE_MARGIN_TOP);
        assert_eq!(avatar.call_site.weight(), WEIGHT_SEMIBOLD);
        assert_eq!(
            avatar.call_site.type_step(&theme).size.to_pixels(px(16.0)),
            px(12.0),
        );
    }

    /// The state that differs in its **anchor set** rather than in a field.
    /// Exactly one of the image and the fallback is ever in the tree.
    #[test]
    fn exactly_one_of_the_image_and_the_fallback_is_mounted() {
        for status in ALL_IMAGE_STATUSES {
            assert_ne!(
                status.mounted(),
                status.shows_fallback(),
                "{}",
                status.name(),
            );
        }
        assert!(ALL_IMAGE_STATUSES[0].mounted());
        // `pending` and `absent` are one picture in the document and two facts
        // about the app: base-ui asks only `!== 'loaded'`.
        assert_eq!(
            super::ImageStatus::Pending.shows_fallback(),
            super::ImageStatus::Absent.shows_fallback(),
        );
    }

    /// The three call-site bundles, their sizes, and which of them a capture can
    /// reach. `repo-icon` renders live and is behind a popover; `none` does not
    /// render at all.
    #[test]
    fn only_the_message_bundle_is_capturable() {
        assert_eq!(ALL_CALL_SITES.len(), 3);
        assert_eq!(CallSite::None.extent(), SIZE_DEFAULT);
        assert_eq!(CallSite::Message.extent(), SIZE_MESSAGE);
        assert_eq!(CallSite::RepoIcon.extent(), SIZE_REPO_ICON);
        assert_eq!(SIZE_DEFAULT, px(32.0));
        assert_eq!(SIZE_REPO_ICON, px(56.0));

        let capturable: Vec<&str> = ALL_CALL_SITES
            .into_iter()
            .filter(|site| site.capturable())
            .map(CallSite::name)
            .collect();
        assert_eq!(capturable, ["message"]);
    }

    /// `rounded-xl` is 14px here, not Tailwind's stock 12 — `theme.css` builds
    /// the scale off `--radius: 0.625rem`. Read through the token so a project
    /// that moved the base moves this with it.
    #[test]
    fn the_repo_icons_radius_is_the_tokens_and_not_tailwinds_stock() {
        let theme = Theme::DARK;
        assert_eq!(CallSite::RepoIcon.radius(&theme), theme.radius_xl.value());
        assert_eq!(theme.radius_xl.value(), px(14.0));
    }

    /// The initials are what `MessageAvatar` computes, and the overflow arm is
    /// deliberately longer than anything it can produce: `slice(0, 2)` caps the
    /// live string at two characters.
    #[test]
    fn the_live_initials_are_at_most_two_characters() {
        assert_eq!(Initials::Short.text().len(), 1);
        assert_eq!(Initials::Normal.text(), "AG");
        assert!(Initials::Overflow.text().len() > 2);
        assert_eq!(Avatar::fixture().initials, Some(Initials::Normal));
    }

    /// The three weights the two call sites reach, as `--font-weight-*` numbers.
    /// `font-bold` belongs to a `repo-icon-popover` fallback this port does not
    /// model, and is named so the omission is visible rather than absent.
    #[test]
    fn the_weights_are_tailwinds_own_numbers() {
        assert_eq!(super::WEIGHT_MEDIUM, FontWeight(500.0));
        assert_eq!(WEIGHT_SEMIBOLD, FontWeight(600.0));
        assert_eq!(WEIGHT_BOLD, FontWeight(700.0));
    }
}
