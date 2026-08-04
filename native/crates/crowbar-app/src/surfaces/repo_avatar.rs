//! `--surface repo-avatar` — a repo's identity mark, one of two P3.50
//! foundation leaves. See `crowbar_ui::surfaces::repo::repo_avatar`'s module docs
//! for the shape (single swapped element, no persistent wrapper) and for the
//! `avatar.color` finding this surface's `--background` cannot resolve.
//!
//! # What each axis can and cannot do here
//!
//! | Axis | Here |
//! |---|---|
//! | `--size` | **real** — `sm`/`lg`/`xl`, 16/20/24px |
//! | `--kind` / `--image` | **real** — which of the three pictures renders, and (for the image kind) whether it loaded |
//! | `--content` | real **only for the letter fallback** — drives [`Self::label`] through the three fixture strings |
//! | `--theme` | **vacuous on every field but the letter's own background**, and even that one is a placeholder — `avatar.color` is unresolvable data, see the module docs |
//! | `--width` / `--viewport-width` | **vacuous.** Every box is `h-*`/`w-*`; there is no `sm:` variant anywhere in `repo-avatar.tsx` |
//!
//! # The state axis
//!
//! `empty` is the one real flag, reached through the **real** `label` prop at
//! its empty-string edge (`avatar.label` is a caller-supplied string with no
//! non-emptiness rule): the letter box still paints its background and size,
//! with no text run inside it. `hover`/`focus`/`selected` are unmodelled —
//! `repo-avatar.tsx` carries no interaction rule of any kind, and neither it
//! nor `RepoAvatarImg` accepts a `className`/prop passthrough that could stand
//! in for one.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::surfaces::repo::repo_avatar::{
    ALL_IMAGE_STATES, ALL_SIZES, ImageState, Kind, RepoAvatar, Size,
};
use crowbar_ui::AnchorSink;
use crowbar_ui::surfaces::rows::ContentLength;
use gpui::{AnyElement, IntoElement as _, ParentElement as _, SharedString, Styled as _, div};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "repo-avatar",
    root: crowbar_ui::surfaces::repo::repo_avatar::ID,
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        // `repo-avatar.tsx` has no interaction rule of any kind, and takes no
        // `className`/prop passthrough anywhere — see the module docs.
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // The tallest box is `xl`'s 24px, plus `CAPTION_HEIGHT`'s 29. A floor, not
    // a ceiling: this surface drives no height.
    min_window_height: 72,
    // An avatar sits beside a repo/workspace name — never the window.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
#[derive(Clone, Debug, Default, PartialEq)]
pub struct Params {
    /// `--size`: `sizeClasses`'s key.
    pub size: Size,
    /// `--kind`: which of the three pictures renders.
    pub kind: KindWord,
    /// `--image`: `RepoAvatarImgAttempt`'s `errored` state, read only when
    /// `--kind` is `image`.
    pub image: ImageState,
    /// `--letter`: overrides `avatar.label` outright, beating `--content`.
    /// `label_of` only ever produces the three fixture strings
    /// `"R"`/`"RE"`/`"REPOSITORY"` — none of them any real repo's own
    /// initial — so no drive of a bare `--content` could ever match a live
    /// app showing a different repo (this item's own live run hit exactly
    /// that: the fixture read `"RE"`, the live repo's letter was `"D"`,
    /// and `text` is compared exactly). `None` leaves `--content` in
    /// charge, unchanged.
    pub letter: Option<SharedString>,
}

/// `--kind`'s vocabulary — `Kind` minus the state [`ImageState`] carries, so
/// the two options compose rather than one repeating the other's cases.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum KindWord {
    /// `avatar.url` starts with `"emoji:"`.
    Emoji,
    /// `avatar.url` is a real URL — see `--image`.
    Image,
    /// No `avatar.url` at all — the letter fallback, `RepoAvatar::fixture`'s
    /// own kind and the one `--surface repo-avatar`'s bare cell has to
    /// produce (module docs, this file's own tests). `#[default]` belongs
    /// here rather than on `Image`: `Params` derives `Default` field-wise
    /// (there is no `Theme` at `Default::default()`'s call site to hand
    /// `RepoAvatar::fixture` the way `workspace-branch-icon.rs`'s `Params`
    /// hands its own fixture one), so whichever `KindWord` variant carries
    /// this attribute *is* the bare cell's kind — it was `Image` before this
    /// fix, which is what the four failing tests were reporting.
    #[default]
    Letter,
}

/// Every `--kind` word, for `--help` and the closed-vocabulary test.
pub const ALL_KIND_WORDS: [KindWord; 3] = [KindWord::Emoji, KindWord::Image, KindWord::Letter];

impl KindWord {
    /// Its word on the command line and in a caption.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Emoji => "emoji",
            Self::Image => "image",
            Self::Letter => "letter",
        }
    }
}

/// The three fixture strings a real `avatar.label` could be — `--content`'s
/// vocabulary applied to this surface's one text-painting anchor.
fn label_of(content: ContentLength) -> &'static str {
    match content {
        ContentLength::Short => "R",
        ContentLength::Normal => "RE",
        ContentLength::Overflow => "REPOSITORY",
    }
}

impl Params {
    /// The avatar this cell describes.
    ///
    /// `empty` overrides the label to an empty string regardless of
    /// `--content` or `--letter` — the one real state this surface has,
    /// reached through the real `label` prop at its edge value rather than
    /// through an invented mechanism. See the module docs. `--letter`, when
    /// set and the cell is not `empty`, beats `--content`'s own fixture
    /// strings — see [`Params::letter`]'s own doc comment.
    #[must_use]
    pub fn repo_avatar(&self, cell: &Cell, theme: &Theme) -> RepoAvatar {
        let mut avatar = RepoAvatar::fixture(theme);
        avatar.size = self.size;
        avatar.kind = match self.kind {
            KindWord::Emoji => Kind::Emoji,
            KindWord::Image => Kind::Image(self.image),
            KindWord::Letter => Kind::Letter,
        };
        avatar.label = if cell.has(StateFlag::Empty) {
            String::new().into()
        } else if let Some(letter) = &self.letter {
            letter.clone()
        } else {
            label_of(cell.content).into()
        };
        avatar
    }
}

impl SurfaceParams for Params {
    fn accept(
        &mut self,
        option: &str,
        args: &mut dyn Iterator<Item = String>,
    ) -> Result<bool, ParseError> {
        match option {
            "--size" => self.size = parse_size(&value(args, option)?)?,
            "--kind" => self.kind = parse_kind(&value(args, option)?)?,
            "--image" => self.image = parse_image(&value(args, option)?)?,
            "--letter" => self.letter = Some(value(args, option)?.into()),
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** The box is authored by `--size`, and no option here sets a
    /// height independently of it.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(
            out,
            " · size {} · kind {}",
            self.size.name(),
            self.kind.name(),
        );
        if self.kind == KindWord::Image {
            let _ = write!(out, " ({})", self.image.name());
        }
        if let Some(letter) = &self.letter {
            let _ = write!(out, " · letter {letter:?} (beats --content)");
        }
        if cell.has(StateFlag::Empty) {
            out.push_str(
                " · empty: the letter box paints its background and size with no text \
                 run inside it — a real prop (`avatar.label`) at its edge value, not an \
                 invented one",
            );
        }
        out.push_str(
            " · avatar.color has no reference — it is a Tailwind class chosen per \
             repository and stored as data; this cell's letter background is a \
             placeholder, not a captured value",
        );
    }

    /// The avatar, inside the flex row that makes it a flex item — `avatar`'s
    /// own convention, and true here for the same reason: every live
    /// `<RepoAvatar>` call site renders it as a sibling of a name, inside a
    /// flex row.
    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        div()
            .flex()
            .flex_row()
            .items_start()
            .child(self.repo_avatar(cell, theme).render(theme, anchors))
            .into_any_element()
    }
}

fn parse_size(raw: &str) -> Result<Size, ParseError> {
    ALL_SIZES
        .into_iter()
        .find(|size| size.name() == raw)
        .ok_or_else(|| {
            ParseError::Rejected(format!(
                "--size takes one of {}, not {raw}",
                names(ALL_SIZES.into_iter().map(Size::name)),
            ))
        })
}

fn parse_kind(raw: &str) -> Result<KindWord, ParseError> {
    ALL_KIND_WORDS
        .into_iter()
        .find(|kind| kind.name() == raw)
        .ok_or_else(|| {
            ParseError::Rejected(format!(
                "--kind takes one of {}, not {raw}",
                names(ALL_KIND_WORDS.into_iter().map(KindWord::name)),
            ))
        })
}

fn parse_image(raw: &str) -> Result<ImageState, ParseError> {
    ALL_IMAGE_STATES
        .into_iter()
        .find(|status| status.name() == raw)
        .ok_or_else(|| {
            ParseError::Rejected(format!(
                "--image takes one of {}, not {raw}",
                names(ALL_IMAGE_STATES.into_iter().map(ImageState::name)),
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
            "--size <name>".to_owned(),
            format!(
                "one of {} — `sizeClasses`'s key, no `md` between `sm` and `lg` [{}]",
                names(ALL_SIZES.into_iter().map(Size::name)),
                Size::default().name(),
            ),
        ),
        (
            "--kind <name>".to_owned(),
            format!(
                "one of {} — which of the three pictures renders [{}]",
                names(ALL_KIND_WORDS.into_iter().map(KindWord::name)),
                KindWord::default().name(),
            ),
        ),
        (
            "--image <status>".to_owned(),
            format!(
                "one of {} — read only with --kind image [{}]",
                names(ALL_IMAGE_STATES.into_iter().map(ImageState::name)),
                ImageState::default().name(),
            ),
        ),
        (
            "--letter <text>".to_owned(),
            "overrides the letter fallback's own label (avatar.label), beating --content \
             — the live repo's own initial, since --content only ever produces the fixture \
             strings R / RE / REPOSITORY [none]"
                .to_owned(),
        ),
    ]
    .into_iter()
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{ALL_KIND_WORDS, Params, SURFACE};
    use crate::row_surface::{Cell, ParseError, StateFlag};
    use crowbar_ui::Theme;
    use crowbar_ui::surfaces::repo::repo_avatar::{self, ImageState, Kind};

    fn a_cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "repo-avatar"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("a well-formed cell")
    }

    fn params(cell: &Cell) -> Params {
        cell.surface_params::<Params>()
            .expect("repo-avatar's own bag")
            .clone()
    }

    /// The surface's root is the primitive's own single anchor.
    #[test]
    fn the_root_anchor_is_the_primitives_own_id() {
        assert_eq!(SURFACE.root, "repo-avatar");
        assert_eq!(repo_avatar::ID, "repo-avatar");
    }

    /// A bare `--surface repo-avatar` is the letter fallback at `sm`.
    #[test]
    fn the_default_cell_is_the_letter_fallback() {
        let theme = Theme::DARK;
        let cell = a_cell(&[]);
        let avatar = params(&cell).repo_avatar(&cell, &theme);
        assert_eq!(avatar.kind, Kind::Letter);
        assert_eq!(avatar.size, repo_avatar::Size::Sm);
    }

    /// `--kind image --image errored` is the same picture as `--kind letter`
    /// — both reach the letter box.
    #[test]
    fn an_errored_image_shows_the_letter_fallback() {
        let theme = Theme::DARK;
        let cell = a_cell(&["--kind", "image", "--image", "errored"]);
        let avatar = params(&cell).repo_avatar(&cell, &theme);
        assert!(avatar.shows_letter());
        assert_eq!(avatar.kind, Kind::Image(ImageState::Errored));
    }

    /// `empty` blanks the label regardless of `--content` — driven through
    /// the real `label` prop, not an invented mechanism.
    #[test]
    fn empty_blanks_the_label_and_beats_content() {
        let theme = Theme::DARK;
        let flagged = a_cell(&["--flags", "empty"]);
        assert_eq!(params(&flagged).repo_avatar(&flagged, &theme).label, "");

        let contradicted = a_cell(&["--flags", "empty", "--content", "overflow"]);
        assert_eq!(
            params(&contradicted).repo_avatar(&contradicted, &theme).label,
            "",
            "empty wins over --content",
        );

        let not_empty = a_cell(&["--content", "overflow"]);
        assert_eq!(
            params(&not_empty).repo_avatar(&not_empty, &theme).label,
            "REPOSITORY",
        );
    }

    /// **`--letter` overrides the label outright, beating `--content`** —
    /// the gap this item's own live drive hit: the fixture always read
    /// `"RE"`, the live repo's own letter was `"D"`, and no drive of
    /// `--content` could ever reach it (`label_of` has a closed
    /// three-string vocabulary, none of them any real repo's own initial).
    /// `empty` still beats `--letter` — it is the surface's one real state
    /// flag, reached through the same `avatar.label` prop at its edge
    /// value, and `--letter` is not a second, competing state.
    #[test]
    fn letter_overrides_content_and_empty_still_wins() {
        let theme = Theme::DARK;

        let lettered = a_cell(&["--letter", "D"]);
        assert_eq!(params(&lettered).repo_avatar(&lettered, &theme).label, "D");

        let contradicted = a_cell(&["--letter", "D", "--content", "overflow"]);
        assert_eq!(
            params(&contradicted).repo_avatar(&contradicted, &theme).label,
            "D",
            "--letter beats --content",
        );

        let emptied = a_cell(&["--letter", "D", "--flags", "empty"]);
        assert_eq!(
            params(&emptied).repo_avatar(&emptied, &theme).label,
            "",
            "empty still beats --letter",
        );

        let unset = a_cell(&[]);
        assert_eq!(
            params(&unset).repo_avatar(&unset, &theme).label,
            "RE",
            "no --letter leaves --content in charge, unchanged",
        );
    }

    /// Five of the six flags are unmodelled; `empty` is the one real one.
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

    /// The vocabularies are closed and every word round-trips.
    #[test]
    fn the_vocabularies_are_closed() {
        for kind in ALL_KIND_WORDS {
            let cell = a_cell(&["--kind", kind.name()]);
            assert_eq!(params(&cell).kind, kind);
        }

        let Err(ParseError::Rejected(_)) = Cell::parse(
            ["--surface", "repo-avatar", "--size", "md"]
                .iter()
                .map(|arg| (*arg).to_owned()),
        ) else {
            panic!("md is not a size — there is no md between sm and lg");
        };
    }

    /// Every size resolves to its own box, and the vocabulary is `sm`/`lg`/`xl`
    /// — no `md`.
    #[test]
    fn every_size_resolves_to_its_own_box() {
        use gpui::px;
        let theme = Theme::DARK;
        let sizes = [
            ("sm", px(16.0)),
            ("lg", px(20.0)),
            ("xl", px(24.0)),
        ];
        for (word, extent) in sizes {
            let cell = a_cell(&["--size", word]);
            assert_eq!(
                params(&cell).repo_avatar(&cell, &theme).size.extent(),
                extent,
                "{word}",
            );
        }
    }
}
