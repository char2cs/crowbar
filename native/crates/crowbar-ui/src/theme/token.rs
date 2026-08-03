//! The sealed design-token newtypes (§4.3 rule 3, §6.1).
//!
//! A design token is a value only this module can mint. Every type here wraps a
//! private field and offers exactly one constructor, [`Color::seal`] and its
//! siblings, which are `pub(in crate::theme)` — not `pub`, not `pub(crate)`.
//! There is no `from_rgb`, no `new`, no `Default`, and no public field, so a
//! crate downstream of `crowbar-ui` **cannot** produce a `Color`: it can only
//! read one out of a [`Theme`](super::Theme). That is not a convention the
//! oracle has to police after the fact; it is a compile error.
//!
//! The seal is on **construction only**. Rendering needs the value — gpui
//! paints `Hsla`, lays out `Pixels`, sizes text in `Rems` — so every token has
//! a `value()` that hands the inner type straight back, plus `From` impls for
//! the two gpui sinks that come up in almost every element (`text_color` wants
//! an `Hsla`, `bg` wants a `Fill`). Reading is free; minting is not.
//!
//! Deriving a token from another token is minting, so it happens here too:
//! [`Color::mix`] is `color-mix(in srgb, …)` — the arithmetic lives in
//! `crowbar-core`, and this is the part that seals the answer.
//!
//! > Sealing the newtype is necessary and not sufficient. `crowbar-ui`
//! > re-exports gpui, so `crowbar_ui::gpui::rgb(0x1e1e1e)` is in scope for every
//! > view crate and `.bg(rgb(…))` would bypass `Theme` entirely. That hole is
//! > inherent — no view crate can render without gpui's colour types — so it is
//! > closed by rule 4 of `scripts/check-invariants.sh`, which bans raw colour
//! > construction anywhere outside this directory.

use std::time::Duration as StdDuration;

use crowbar_core::color::{Srgba, color_mix_remainder};
use gpui::{Fill, Hsla, Pixels, Rems, Rgba};

/// A colour token.
///
/// Wraps a `gpui::Hsla` because that is what gpui paints; the value came from a
/// `theme.css` declaration resolved through `OKLab` at generation time.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct Color(Hsla);

impl Color {
    /// Mint a colour token. `pub(in crate::theme)` — the seal.
    pub(in crate::theme) const fn seal(hsla: Hsla) -> Self {
        Self(hsla)
    }

    /// The colour gpui should paint.
    #[must_use]
    pub const fn value(self) -> Hsla {
        self.0
    }

    /// `color-mix(in srgb, self <percent>%, other)`.
    ///
    /// The single-percentage form, which is how every `color-mix()` in the
    /// Crowbar stylesheets is written — most often against
    /// [`Color::TRANSPARENT`], which is CSS's idiom for "this colour at N%
    /// opacity". The mixing itself is `crowbar_core::color`; what happens here
    /// is the round trip through sRGB and the reseal.
    #[must_use]
    pub fn mix(self, percent: f32, other: Self) -> Self {
        Self::from_srgba(color_mix_remainder(
            self.to_srgba(),
            percent,
            other.to_srgba(),
        ))
    }

    /// CSS's `transparent`: the identity element of [`Color::mix`].
    pub const TRANSPARENT: Self = Self(Hsla {
        h: 0.0,
        s: 0.0,
        l: 0.0,
        a: 0.0,
    });

    /// CSS's `white` — opaque, unconditional, and carrying no token of its own.
    ///
    /// `slider.tsx`'s thumb is `bg-white`, in **both** themes: no `dark:`
    /// variant touches its background (only its border moves,
    /// `border-input dark:border-background`, which reuses [`Theme::input`] and
    /// [`Theme::background`] directly). `theme.css` has no `--white` custom
    /// property to seal this from — Tailwind's `white` is the CSS keyword, not
    /// a design token — so this is minted here the same way
    /// [`Color::TRANSPARENT`] is: a literal that names a CSS keyword rather
    /// than a value out of `theme.css`.
    pub const WHITE: Self = Self(Hsla {
        h: 0.0,
        s: 0.0,
        l: 1.0,
        a: 1.0,
    });

    /// CSS's `black` — the same trade [`Color::WHITE`] is, one door over.
    ///
    /// `fps-overlay.tsx` writes its badge's background as an inline
    /// `style={{ background: 'rgba(0,0,0,0.72)' }}` — the one paint in the whole
    /// `components/layout` cluster P3.52 covers that bypasses `theme.css`
    /// entirely rather than naming a Tailwind utility. `rgba(0,0,0,0.72)` is
    /// exactly `color-mix(in srgb, black 72%, transparent)`: opaque black
    /// mixed 72/28 with [`Color::TRANSPARENT`], the same idiom
    /// `text-muted-foreground/72` compiles to elsewhere in this tree
    /// (`native/mapping/tabs.md` §1). `theme.css` has no `--black` custom
    /// property to seal this from, so it is minted here the same way `WHITE`
    /// is: a literal naming a CSS keyword, not a value read out of the
    /// stylesheet. See `native/mapping/fps-overlay.md`.
    pub const BLACK: Self = Self(Hsla {
        h: 0.0,
        s: 0.0,
        l: 0.0,
    /// Tailwind's `red-500`, unconditional and carrying no token of its own.
    ///
    /// `workspace-branch-icon.tsx`'s `deleted`/`pr-closed` glyphs write
    /// `text-red-500` directly — `theme.css` has no `--` custom property this
    /// reaches, and it is **not** interchangeable with [`Theme`](super::Theme)'s
    /// `destructive`: the light table's `--destructive` is `var(--color-red-500)`,
    /// the same colour, but the dark table redefines it to
    /// `oklch(0.65 0.22 24)`, a different number. Minted here the way
    /// [`Color::WHITE`] is: a literal out of Tailwind's own default palette, not
    /// a value out of `theme.css`.
    ///
    /// `oklch(63.7% 0.237 25.331)`, Tailwind's own hex `#fb2c36` —
    /// `crowbar-ui/tools/gen-theme.py`'s `TAILWIND` table already carries this
    /// entry (to resolve `--destructive`'s light-mode value) and its
    /// `check_palette()` proves it against Tailwind's published hex.
    pub const RED_500: Self = Self(Hsla {
        h: 0.991_516_65,
        s: 0.958_988_25,
        l: 0.577_229_26,
        a: 1.0,
    });

    /// Tailwind's `green-500` — **not** the same family as
    /// [`Theme`](super::Theme)'s `success`, which is `var(--color-emerald-500)`
    /// in both tables. `workspace-branch-icon.tsx`'s `pr-open` glyph writes
    /// `text-green-500` directly, and no `--` custom property in `theme.css`
    /// ever aliases Tailwind's green family, so there is no token to read this
    /// number from. Minted here the way [`Color::WHITE`] is.
    ///
    /// `oklch(72.3% 0.219 149.579)`, resolved through the same `OKLab` pipeline
    /// `gen-theme.py` uses, and checked against Tailwind's own published hex
    /// `#00c950`.
    pub const GREEN_500: Self = Self(Hsla {
        h: 0.400_160_82,
        s: 1.0,
        l: 0.393_577_8,
        a: 1.0,
    });

    /// Tailwind's `violet-500`. `workspace-branch-icon.tsx`'s `pr-merged`
    /// glyph writes `text-violet-500` directly; `theme.css` has no violet
    /// token at all — not even one this could coincide with by accident, the
    /// way `warning` coincides with `amber-500`. Minted here the way
    /// [`Color::WHITE`] is.
    ///
    /// `oklch(60.6% 0.25 292.717)`, resolved the same way, checked against
    /// Tailwind's own published hex `#8e51ff`.
    pub const VIOLET_500: Self = Self(Hsla {
        h: 0.724_784_85,
        s: 1.0,
        l: 0.659_081_9,
        a: 1.0,
    });

    fn to_srgba(self) -> Srgba {
        let rgba = Rgba::from(self.0);
        Srgba::new(rgba.r, rgba.g, rgba.b, rgba.a)
    }

    fn from_srgba(srgba: Srgba) -> Self {
        Self(Hsla::from(Rgba {
            r: srgba.r,
            g: srgba.g,
            b: srgba.b,
            a: srgba.a,
        }))
    }
}

impl From<Color> for Hsla {
    fn from(color: Color) -> Self {
        color.0
    }
}

impl From<Color> for Fill {
    fn from(color: Color) -> Self {
        color.0.into()
    }
}

/// A spacing token: a gap, an inset, an offset.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct Space(Pixels);

impl Space {
    /// Mint a spacing token. `pub(in crate::theme)` — the seal.
    pub(in crate::theme) const fn seal(pixels: Pixels) -> Self {
        Self(pixels)
    }

    /// The distance to lay out.
    #[must_use]
    pub const fn value(self) -> Pixels {
        self.0
    }
}

impl From<Space> for Pixels {
    fn from(space: Space) -> Self {
        space.0
    }
}

/// A corner-radius token.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct Radius(Pixels);

impl Radius {
    /// Mint a radius token. `pub(in crate::theme)` — the seal.
    pub(in crate::theme) const fn seal(pixels: Pixels) -> Self {
        Self(pixels)
    }

    /// The radius to round by.
    #[must_use]
    pub const fn value(self) -> Pixels {
        self.0
    }
}

impl From<Radius> for Pixels {
    fn from(radius: Radius) -> Self {
        radius.0
    }
}

/// A type-size token.
///
/// Held in `Rems` rather than `Pixels` on purpose: the React tokens are
/// `calc(<n>rem * var(--app-ui-scale))`, and gpui's rem size is the same knob,
/// so the UI-scale setting stays one number instead of a rescale of every
/// token.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct FontSize(Rems);

impl FontSize {
    /// Mint a type-size token. `pub(in crate::theme)` — the seal.
    pub(in crate::theme) const fn seal(rems: Rems) -> Self {
        Self(rems)
    }

    /// The size to set text at.
    #[must_use]
    pub const fn value(self) -> Rems {
        self.0
    }
}

impl From<FontSize> for Rems {
    fn from(size: FontSize) -> Self {
        size.0
    }
}

/// An animation-duration token.
///
/// The React `--animate-*` tokens are full `animation` shorthands
/// (`toast-error-odd 0.28s cubic-bezier(0.5, 1, 0.89, 1)`). Only the duration
/// survives the port as a token: the keyframes are geometry that gpui expresses
/// as code rather than as CSS, and the easing goes with them. What is here is
/// the number the two have to agree on.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct Duration(StdDuration);

impl Duration {
    /// Mint a duration token. `pub(in crate::theme)` — the seal.
    pub(in crate::theme) const fn seal(duration: StdDuration) -> Self {
        Self(duration)
    }

    /// How long the animation runs.
    #[must_use]
    pub const fn value(self) -> StdDuration {
        self.0
    }
}

impl From<Duration> for StdDuration {
    fn from(duration: Duration) -> Self {
        duration.0
    }
}

/// A font-stack token: family names in CSS fallback order.
///
/// A stack rather than a single family because that is what the CSS declares,
/// and because gpui's font resolution takes fallbacks. The stack may be empty:
/// `--font-editor` is `var(--editor-font-family)` with no fallback at all, so
/// its only value comes from Settings at runtime and there is nothing static to
/// record.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct FontFamily(&'static [&'static str]);

impl FontFamily {
    /// Mint a font-stack token. `pub(in crate::theme)` — the seal.
    pub(in crate::theme) const fn seal(stack: &'static [&'static str]) -> Self {
        Self(stack)
    }

    /// The family names, in fallback order.
    #[must_use]
    pub const fn value(self) -> &'static [&'static str] {
        self.0
    }

    /// The first family in the stack, if the stack is not empty.
    #[must_use]
    pub const fn primary(self) -> Option<&'static str> {
        match self.0 {
            [first, ..] => Some(*first),
            [] => None,
        }
    }
}

/// A unitless scale factor.
///
/// One token uses it — `--app-ui-scale`, the multiplier the type-size tokens
/// are defined in terms of.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct Scale(f32);

impl Scale {
    /// Mint a scale token. `pub(in crate::theme)` — the seal.
    pub(in crate::theme) const fn seal(factor: f32) -> Self {
        Self(factor)
    }

    /// The multiplier.
    #[must_use]
    pub const fn value(self) -> f32 {
        self.0
    }
}

impl From<Scale> for f32 {
    fn from(scale: Scale) -> Self {
        scale.0
    }
}
