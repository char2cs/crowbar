//! `--surface avatar` — one `<Avatar>` in one cell of the §8.3 matrix, and the
//! first surface in the port whose **anchor set** is a state rather than a
//! constant.
//!
//! The default cell is the live review-thread message avatar with its image
//! loaded: `mt-0.5 size-6 shrink-0 text-xs font-semibold` over the primitive's
//! `rounded-full bg-background`, measured at `24 × 24` on a 1714px viewport.
//!
//! # `--image` is not a style. It is which elements exist
//!
//! `base-ui`'s `AvatarImage` returns `null` until the bytes arrive and
//! `AvatarFallback` unmounts the moment they do, so the two states are
//! *different documents*: one carries `avatar-image`, the other carries
//! `avatar-fallback`, and neither carries both. `ANCHORS.md` ranks a missing
//! anchor first, so a cell driven to one status on one side and the other on the
//! other reports the loudest failure it has for a reason that is not a port bug.
//!
//! Both were captured live from the same review thread, which is what makes the
//! claim a measurement:
//!
//! ```text
//! char2cs's message   anchors: avatar, avatar-image      (the image loaded)
//! the agent's message anchors: avatar, avatar-fallback   ("AG", 24×24, bg-muted)
//! ```
//!
//! ## Why it is an option and not the `loading` flag
//!
//! This is the one place in the port where the §8.3 word and the surface's own
//! option describe the *same real state*, and the flag is still inert.
//! `--flags loading` reaches nothing here — nothing on this surface reads it —
//! so [`Surface::unmodelled`]'s claim, that driving it cannot fail, is true; and
//! `--image pending` renders the branch. `button` made the same arrangement for
//! its `loading` prop and `resizable` for `--with-handle`. The difference worth
//! saying out loud is that unlike either of those, **this branch is reachable in
//! the live app** and its reference has been taken.
//!
//! # What each axis can and cannot do here
//!
//! | Axis | Here |
//! |---|---|
//! | `--theme` | **real**, and the only axis that is. `bg-background` on the root and `bg-muted` on the fallback are different tokens in the two tables |
//! | `--content` | real **only with a fallback** — the initials are the sole text on the surface. Vacuous on the default cell, which shows the image |
//! | `--width` | **vacuous.** Every box here is `size-*` or `size-full` |
//! | `--viewport-width` | **vacuous.** `avatar.tsx` contains no `sm:` variant at all — the first component in the port with none |
//!
//! # The state axis
//!
//! `empty` is the only flag with an original, and it is a real branch of the
//! primitive rather than a contrivance: `<Avatar>` takes its children from the
//! call site, so a root with no image and no fallback is an expressible — if
//! unused — rendering, and it is the one picture where the root's own
//! `bg-background` circle is all there is.
//!
//! Everything else is unmodelled and the reason is the same for all four:
//! **`avatar.tsx` has no interaction rule of any kind.** No `hover:`, no
//! `focus`, no `data-[selected]`, no `disabled:`. There is nothing to disagree
//! about.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::primitives::avatar::{
    ALL_CALL_SITES, ALL_IMAGE_STATUSES, Avatar, CallSite, ImageStatus, Initials,
};
use crowbar_ui::AnchorSink;
use crowbar_ui::surfaces::rows::ContentLength;
use crowbar_ui::primitives::avatar;
use gpui::{AnyElement, IntoElement as _, ParentElement as _, Styled as _, div};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "avatar",
    root: avatar::ID_ROOT,
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        // `avatar.tsx` has no interaction rule of any kind — see the module
        // docs. These three are not "not got to yet"; there is nothing there.
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // The tallest avatar is `repo-icon`'s `size-14` at 56px, plus
    // `CAPTION_HEIGHT`'s 29. 90 holds that with room and is a floor rather than
    // a ceiling: this surface drives no height, because an avatar's is decided
    // by its call site's `size-*`.
    min_window_height: 90,
    // An avatar sits beside a comment or inside a popover — never the window.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--call-site`: the `className` bundle a call site merges over the
    /// primitive's own. See [`CallSite`].
    pub call_site: CallSite,
    /// `--image`: whether the `<img>` is in the document. See [`ImageStatus`]
    /// and the module docs — this option decides the **anchor set**, not a
    /// style.
    pub image: ImageStatus,
    /// `--no-fallback`: the call site renders no `<AvatarFallback>`.
    ///
    /// Both live call sites always pass one, so this is off by default. With
    /// `--image pending` it is the `empty` cell arrived at from the other
    /// direction, and the two agree — see `an_avatar_with_neither_child_is_the_empty_cell`.
    pub fallback: bool,
}

impl Default for Params {
    fn default() -> Self {
        let fixture = Avatar::fixture();
        Self {
            call_site: fixture.call_site,
            image: fixture.image,
            fallback: fixture.initials.is_some(),
        }
    }
}

impl Params {
    /// The avatar this cell describes.
    ///
    /// Built by taking the live fixture and applying the cell, so that a bare
    /// `--surface avatar` renders the avatar the reference actually has.
    ///
    /// **`empty` overrides both children**, which is what the word means: §8.3's
    /// `empty` is "a surface with nothing in it", and an avatar whose image is
    /// mounted is not that however its fallback is configured.
    #[must_use]
    pub fn avatar(&self, cell: &Cell) -> Avatar {
        let mut avatar = Avatar::fixture();
        avatar.call_site = self.call_site;
        let empty = cell.has(StateFlag::Empty);
        avatar.image = if empty {
            ImageStatus::Absent
        } else {
            self.image
        };
        avatar.initials = (self.fallback && !empty).then(|| initials_of(cell.content));
        avatar
    }
}

/// The initials a content length shows.
///
/// A translation rather than a shared type, for `button`'s reason: the cell's
/// vocabulary and the component's carry different strings on purpose. Here the
/// component's are what `MessageAvatar` computes —
/// `(display.name || 'U').slice(0, 2).toUpperCase()` — plus one longer than
/// anything that expression can produce, because a run wider than the circle is
/// the only thing `--content` can show on this surface.
fn initials_of(content: ContentLength) -> Initials {
    match content {
        ContentLength::Short => Initials::Short,
        ContentLength::Normal => Initials::Normal,
        ContentLength::Overflow => Initials::Overflow,
    }
}

impl SurfaceParams for Params {
    fn accept(
        &mut self,
        option: &str,
        args: &mut dyn Iterator<Item = String>,
    ) -> Result<bool, ParseError> {
        match option {
            "--call-site" => self.call_site = parse_call_site(&value(args, option)?)?,
            "--image" => self.image = parse_image(&value(args, option)?)?,
            "--no-fallback" => self.fallback = false,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** An avatar's height is its call site's `size-*`, and no option
    /// here sets one.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    /// This surface's own half of the caption.
    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(
            out,
            " · class {} · image {}",
            self.call_site.name(),
            self.image.name(),
        );

        if self.call_site == CallSite::Message {
            out.push_str(" (review-thread-item.tsx's MessageAvatar)");
        }
        match self.call_site {
            CallSite::None => out.push_str(
                " · class: no live call site leaves the primitive's className alone, so \
                 there is no reference",
            ),
            CallSite::RepoIcon => out.push_str(
                " · class: it renders live, inside a PopoverContent this machine cannot \
                 open, so there is no reference; and its data-driven fallback colour is \
                 unmodelled",
            ),
            CallSite::Message => {}
        }

        if cell.has(StateFlag::Empty) {
            out.push_str(
                " · empty: no image and no fallback, which is a real branch of the \
                 primitive that no live call site renders, so there is no reference",
            );
        } else if self.image.shows_fallback() {
            let _ = write!(out, " · fallback \"{}\"", initials_of(cell.content).text());
            if self.image == ImageStatus::Pending {
                out.push_str(
                    " · image pending: base-ui asks only `!== 'loaded'`, so this is the \
                     same picture as absent",
                );
            }
        } else if self.fallback {
            out.push_str(
                " · the image is mounted, so the fallback is unmounted and --content \
                 cannot fail",
            );
        }

        if !self.fallback && !cell.has(StateFlag::Empty) {
            out.push_str(
                " · no fallback: both live call sites always pass one, so there is no \
                 reference",
            );
        }
    }

    /// The avatar, inside the flex row that makes it a flex item.
    ///
    /// `RowSurface` draws every surface inside a gpui **block** container, and
    /// an avatar drawn straight into one would be a block-level flex box.
    /// Every live `<Avatar>` is a flex item — which is also why the reference's
    /// computed `display` is `flex` rather than `inline-flex`, CSS having
    /// blockified it. The row carries no anchor, so it cannot reach a snapshot.
    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        div()
            .flex()
            .flex_row()
            .items_start()
            .child(self.avatar(cell).render(theme, anchors))
            .into_any_element()
    }
}

/// A call site's `className` bundle.
///
/// **There is deliberately no numeric form.** The line P3.1 drew for
/// `--class-radius`: a knob may supply the same *input* both engines resolve,
/// never the reference's *output*.
fn parse_call_site(raw: &str) -> Result<CallSite, ParseError> {
    ALL_CALL_SITES
        .into_iter()
        .find(|site| site.name() == raw)
        .ok_or_else(|| {
            ParseError::Rejected(format!(
                "--call-site takes one of {}, not {raw}; it names the className bundle a \
                 call site merges, never a pixel value",
                names(ALL_CALL_SITES.into_iter().map(CallSite::name)),
            ))
        })
}

/// Whether the `<img>` is in the document.
fn parse_image(raw: &str) -> Result<ImageStatus, ParseError> {
    ALL_IMAGE_STATUSES
        .into_iter()
        .find(|status| status.name() == raw)
        .ok_or_else(|| {
            ParseError::Rejected(format!(
                "--image takes one of {}, not {raw}; it decides which anchors exist, not \
                 how they are painted",
                names(ALL_IMAGE_STATUSES.into_iter().map(ImageStatus::name)),
            ))
        })
}

/// A vocabulary as one line, for a usage line and for a rejection.
fn names<I: Iterator<Item = &'static str>>(words: I) -> String {
    words.collect::<Vec<_>>().join(", ")
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--call-site <name>".to_owned(),
            format!(
                "one of {} — the className bundle a call site merges, never a pixel \
                 value; only `message` is capturable [{}]",
                names(ALL_CALL_SITES.into_iter().map(CallSite::name)),
                Avatar::fixture().call_site.name(),
            ),
        ),
        (
            "--image <status>".to_owned(),
            format!(
                "one of {} — which of avatar-image / avatar-fallback exists at all [{}]",
                names(ALL_IMAGE_STATUSES.into_iter().map(ImageStatus::name)),
                Avatar::fixture().image.name(),
            ),
        ),
        (
            "--no-fallback".to_owned(),
            "render no AvatarFallback; both live call sites always pass one [off]".to_owned(),
        ),
    ]
    .into_iter()
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{Params, SURFACE};
    use crate::row_surface::{Cell, ParseError, StateFlag};
    use crowbar_ui::primitives::avatar;
    use crowbar_ui::primitives::avatar::{
        ALL_CALL_SITES, ALL_IMAGE_STATUSES, CallSite, ImageStatus, Initials,
    };

    fn a_cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "avatar"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("a well-formed cell")
    }

    fn params(cell: &Cell) -> Params {
        cell.surface_params::<Params>()
            .expect("avatar's own bag")
            .clone()
    }

    /// The surface's root is the primitive's own id, which is what the
    /// reference's `root` field says.
    #[test]
    fn the_root_anchor_is_the_primitives_own_id() {
        assert_eq!(SURFACE.root, "avatar");
        assert_eq!(avatar::ID_ROOT, "avatar");
    }

    /// A bare `--surface avatar` is the live message avatar with its image
    /// loaded — the state the captured reference was taken in.
    #[test]
    fn the_default_cell_is_the_live_message_avatar() {
        let cell = a_cell(&[]);
        let avatar = params(&cell).avatar(&cell);

        assert_eq!(avatar.call_site, CallSite::Message);
        assert_eq!(avatar.image, ImageStatus::Loaded);
        assert!(avatar.image.mounted());
        assert_eq!(avatar.initials, Some(Initials::Normal));
        assert_eq!(avatar.id, "avatar");
    }

    /// The option that decides the anchor set. Every status parses, and the two
    /// non-loaded ones are one picture.
    #[test]
    fn the_image_status_decides_which_child_exists() {
        for status in ALL_IMAGE_STATUSES {
            let cell = a_cell(&["--image", status.name()]);
            let avatar = params(&cell).avatar(&cell);
            assert_eq!(avatar.image, status);
            assert_eq!(avatar.image.mounted(), status == ImageStatus::Loaded);
            // The fallback is configured either way; whether it *renders* is the
            // status's answer, and `render` is where the two meet.
            assert!(avatar.initials.is_some());
        }
    }

    /// `empty` means "nothing in it", so it overrides both children whatever the
    /// other options say — and the same picture is reachable from `--image
    /// pending --no-fallback`, which is the check that the two spellings agree.
    #[test]
    fn an_avatar_with_neither_child_is_the_empty_cell() {
        let flagged = a_cell(&["--flags", "empty"]);
        let one = params(&flagged).avatar(&flagged);
        assert!(!one.image.mounted());
        assert!(one.initials.is_none());

        let driven = a_cell(&["--image", "pending", "--no-fallback"]);
        let two = params(&driven).avatar(&driven);
        assert!(!two.image.mounted());
        assert!(two.initials.is_none());

        // `empty` beats a loaded image, which is the whole point of the word.
        let contradicted = a_cell(&["--flags", "empty", "--image", "loaded"]);
        assert!(!params(&contradicted).avatar(&contradicted).image.mounted());
    }

    /// Four of the six flags are unmodelled, and the reason is that
    /// `avatar.tsx` has no interaction rule at all.
    #[test]
    fn only_empty_is_modelled() {
        for flag in [
            StateFlag::Loading,
            StateFlag::Error,
            StateFlag::Hover,
            StateFlag::Focus,
            StateFlag::Selected,
        ] {
            assert!(SURFACE.unmodelled(flag), "{}", flag.name());
        }
        assert!(!SURFACE.unmodelled(StateFlag::Empty));
    }

    /// The vocabularies are closed, and neither takes a number.
    #[test]
    fn the_vocabularies_are_closed_and_take_no_measurements() {
        for site in ALL_CALL_SITES {
            let cell = a_cell(&["--call-site", site.name()]);
            assert_eq!(params(&cell).avatar(&cell).call_site, site);
        }

        for line in [
            vec!["--surface", "avatar", "--call-site", "review"],
            vec!["--surface", "avatar", "--call-site", "24"],
            vec!["--surface", "avatar", "--image", "error"],
            vec!["--surface", "avatar", "--size", "6"],
            vec!["--surface", "avatar", "--radius", "9999"],
        ] {
            assert!(
                matches!(
                    Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{line:?} should have been rejected",
            );
        }

        let Err(ParseError::Rejected(complaint)) = Cell::parse(
            ["--surface", "avatar", "--call-site", "24"]
                .iter()
                .map(|arg| (*arg).to_owned()),
        ) else {
            panic!("24 is not a className bundle");
        };
        assert!(complaint.contains("never a pixel value"), "{complaint}");
    }

    /// The caption says, per cell, which pictures have no reference — and the
    /// two reasons are worded differently, because "it does not render" and "it
    /// renders behind a popover this machine cannot open" send a reader to
    /// different places.
    #[test]
    fn the_caption_separates_does_not_render_from_cannot_be_captured() {
        assert!(a_cell(&[]).describe().contains("MessageAvatar"));

        let none = a_cell(&["--call-site", "none"]);
        assert!(
            none.describe().contains("no live call site leaves"),
            "{}",
            none.describe(),
        );

        let repo = a_cell(&["--call-site", "repo-icon"]);
        assert!(
            repo.describe().contains("PopoverContent"),
            "{}",
            repo.describe()
        );

        // With the image mounted the fallback is gone, so --content is inert.
        let loaded = a_cell(&[]);
        assert!(
            loaded.describe().contains("--content cannot fail"),
            "{}",
            loaded.describe()
        );

        // And with it pending the initials are named.
        let pending = a_cell(&["--image", "pending", "--content", "short"]);
        assert!(
            pending.describe().contains("fallback \"U\""),
            "{}",
            pending.describe()
        );
    }
}
