//! `--surface repo-icon-popover` — the repo icon editor's own popup: a
//! caption, a preview avatar, an upload/emoji/GitHub row and two
//! call-site-conditional rows.
//!
//! `crowbar_ui::components::repo_icon_popover` carries the full account of
//! this composition's own arithmetic, of why it reuses
//! `avatar::CallSite::RepoIcon`'s numbers rather than its render path, and
//! of why the trigger's own two rest states are driven without a second
//! `--surface` (`row_layout/repo_icon_popover.rs`, the `sidebar_tab_bar.rs`
//! shape). This file is the popup's own cell.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | every one of the six | **unmodelled.** `repo-icon-popover.tsx`'s own `<PopoverContent>` carries no `hover:`/`focus:`/`data-active` rule of its own — every interactive class list lives on the buttons inside it, which is `button`'s own surface's business, the identical call `sidebar-project-header.md` §5 makes about its own four buttons. `--emoji`/`--reset` are this surface's own axis instead. |

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::AnchorSink;
use crowbar_ui::components::repo_icon_popover::{self, PopupContent, PreviewAvatar};
use gpui::{AnyElement, SharedString};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "repo-icon-popover",
    root: repo_icon_popover::ID_POPUP,
    unmodelled: &[
        StateFlag::Empty,
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // p-3 (12) + caption + gap-3 (12) + avatar (56) + gap-3 (12) + the
    // button row (24) + p-3 (12), plus the caption's own text line and
    // room for the two optional rows this floor does not need to reach
    // exactly — a floor, not a ceiling, the same posture every popup-shaped
    // surface in this tree takes.
    min_window_height: 220,
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// `--preview`'s closed vocabulary.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum PreviewKind {
    /// The letter fallback.
    Letter,
    /// The emoji fallback.
    Emoji,
    /// The fetched-image case.
    Image,
}

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--preview`.
    pub preview: PreviewKind,
    /// `--emoji`: mounts the emoji entry row.
    pub show_emoji_input: bool,
    /// `--reset`: mounts the "Reset to default" button.
    pub has_avatar_url: bool,
    /// `--letter`: overrides the letter preview's own label
    /// (`repo.avatarLabel`), read only when `--preview` is `letter`.
    /// Hard-coded to `"R"` before this flag existed, which — like
    /// `repo-avatar`'s identical gap — made this preview permanently
    /// un-comparable against any live app state whose repo's own initial
    /// is not literally `"R"` (`text` is compared exactly by the differ).
    /// `None` keeps the old `"R"` placeholder.
    pub letter: Option<SharedString>,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            preview: PreviewKind::Letter,
            show_emoji_input: false,
            has_avatar_url: false,
            letter: None,
        }
    }
}

impl Params {
    /// The popup content this cell describes.
    #[must_use]
    pub fn content(&self) -> PopupContent {
        PopupContent {
            preview: match self.preview {
                PreviewKind::Letter => PreviewAvatar::Letter {
                    label: self
                        .letter
                        .clone()
                        .unwrap_or_else(|| SharedString::new_static("R")),
                    // `None`: no `--avatar-color` flag exists on this
                    // surface, and the live reference this cell is driven
                    // against has no `avatarColor` either — `avatar-
                    // fallback.bg` resolves to `theme.muted` through
                    // `PreviewAvatar::render`'s own fallback, matching
                    // `avatar.rs`'s `Avatar::fallback` (see that function's
                    // own doc comment for the fix and the live delta it
                    // closes: `#00000000` against a reference `#ffffff0a`).
                    background: None,
                },
                PreviewKind::Emoji => PreviewAvatar::Emoji(SharedString::new_static("\u{1f98a}")),
                PreviewKind::Image => PreviewAvatar::Image,
            },
            show_emoji_input: self.show_emoji_input,
            has_avatar_url: self.has_avatar_url,
        }
    }
}

impl SurfaceParams for Params {
    fn accept(
        &mut self,
        option: &str,
        args: &mut dyn Iterator<Item = String>,
    ) -> Result<bool, ParseError> {
        match option {
            "--preview" => self.preview = parse_preview(&value(args, option)?)?,
            "--emoji" => self.show_emoji_input = true,
            "--reset" => self.has_avatar_url = true,
            "--letter" => self.letter = Some(value(args, option)?.into()),
            _ => return Ok(false),
        }
        Ok(true)
    }

    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, _cell: &Cell, out: &mut String) {
        let _ = write!(out, " · preview {}", self.preview.name());
        if self.show_emoji_input {
            out.push_str(" · emoji row");
        }
        if self.has_avatar_url {
            out.push_str(" · reset button");
        }
        if let Some(letter) = &self.letter {
            let _ = write!(out, " · letter {letter:?}");
        }
    }

    fn no_state_axis(&self) -> bool {
        true
    }

    fn render(&self, _cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.content().render(theme, anchors)
    }
}

impl PreviewKind {
    /// Its word on the command line and in a caption.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Letter => "letter",
            Self::Emoji => "emoji",
            Self::Image => "image",
        }
    }
}

fn parse_preview(raw: &str) -> Result<PreviewKind, ParseError> {
    match raw {
        "letter" => Ok(PreviewKind::Letter),
        "emoji" => Ok(PreviewKind::Emoji),
        "image" => Ok(PreviewKind::Image),
        other => Err(ParseError::Rejected(format!(
            "--preview takes letter, emoji or image, not {other}",
        ))),
    }
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--preview <letter|emoji|image>".to_owned(),
            "which of the three preview-avatar pictures this cell shows [letter]".to_owned(),
        ),
        (
            "--emoji".to_owned(),
            "mounts the emoji entry row and its Set button [not mounted]".to_owned(),
        ),
        (
            "--reset".to_owned(),
            "mounts the Reset to default button [not mounted]".to_owned(),
        ),
        (
            "--letter <text>".to_owned(),
            "overrides the letter preview's own label (repo.avatarLabel), read only with \
             --preview letter — the live repo's own initial, since the fixture is a \
             hard-coded placeholder [R]"
                .to_owned(),
        ),
    ]
    .into_iter()
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{Params, PreviewKind, SURFACE, options, parse_preview};
    use crate::row_surface::{Cell, ParseError, StateFlag};
    use crate::surface::SurfaceParams;
    use crowbar_ui::components::repo_icon_popover::PreviewAvatar;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "repo-icon-popover"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a repo-icon-popover cell carries this surface's bag")
    }

    #[test]
    fn the_defaults_are_the_letter_preview_with_neither_optional_row() {
        let bag = Params::default();
        assert_eq!(bag.preview, PreviewKind::Letter);
        assert!(!bag.show_emoji_input);
        assert!(!bag.has_avatar_url);
        assert_eq!(bag.letter, None);

        let content = params_of(&cell(&[])).content();
        assert!(matches!(content.preview, PreviewAvatar::Letter { .. }));
        assert!(!content.show_emoji_input);
        assert!(!content.has_avatar_url);
        let PreviewAvatar::Letter { label, .. } = content.preview else {
            panic!("the default preview is the letter fallback");
        };
        assert_eq!(label, "R", "the old hard-coded placeholder, unchanged with no --letter");
    }

    /// **`--letter` overrides the letter preview's own label, read only
    /// with `--preview letter`** — the gap this item's own live drive hit
    /// (`repo-avatar`'s identical finding, the same day): the preview was
    /// hard-coded to `"R"` with no flag able to move it.
    #[test]
    fn letter_overrides_the_letter_previews_label_only() {
        let lettered = params_of(&cell(&["--letter", "D"])).content();
        let PreviewAvatar::Letter { label, .. } = lettered.preview else {
            panic!("--preview defaults to letter");
        };
        assert_eq!(label, "D");

        // `--letter` with a non-letter preview is accepted (nothing rejects
        // an option irrelevant to the current --preview elsewhere on this
        // surface either) but has nothing to override.
        let emoji = params_of(&cell(&["--preview", "emoji", "--letter", "D"])).content();
        assert!(matches!(emoji.preview, PreviewAvatar::Emoji(_)));
    }

    #[test]
    fn every_option_reaches_the_content_independently() {
        let content = params_of(&cell(&["--preview", "emoji", "--emoji", "--reset"])).content();
        assert!(matches!(content.preview, PreviewAvatar::Emoji(_)));
        assert!(content.show_emoji_input);
        assert!(content.has_avatar_url);

        let image_only = params_of(&cell(&["--preview", "image"])).content();
        assert!(matches!(image_only.preview, PreviewAvatar::Image));
        assert!(!image_only.show_emoji_input);
        assert!(!image_only.has_avatar_url);
    }

    #[test]
    fn preview_parses_its_closed_vocabulary() {
        assert_eq!(parse_preview("letter"), Ok(PreviewKind::Letter));
        assert_eq!(parse_preview("emoji"), Ok(PreviewKind::Emoji));
        assert_eq!(parse_preview("image"), Ok(PreviewKind::Image));
        assert!(matches!(parse_preview("bogus"), Err(ParseError::Rejected(_))));
    }

    #[test]
    fn a_non_vocabulary_preview_is_rejected_by_name() {
        let line = ["--surface", "repo-icon-popover", "--preview", "bogus"];
        assert!(matches!(
            Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
            Err(ParseError::Rejected(_)),
        ));
    }

    #[test]
    fn every_flag_is_unmodelled_and_the_surface_declares_it() {
        for flag in [
            StateFlag::Empty,
            StateFlag::Loading,
            StateFlag::Error,
            StateFlag::Hover,
            StateFlag::Focus,
            StateFlag::Selected,
        ] {
            assert!(SURFACE.unmodelled(flag), "{flag:?}");
        }
        assert!(Params::default().no_state_axis());
    }

    #[test]
    fn this_surfaces_options_are_rejected_on_another_surface() {
        for option in ["--preview", "--emoji", "--reset", "--letter"] {
            let line = ["--surface", "skeleton", option];
            assert!(
                matches!(
                    Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{option} should not be a skeleton option",
            );
        }
    }

    #[test]
    fn the_usage_names_every_option_this_surface_takes() {
        let usage = crate::row_surface::usage();
        for option in ["--preview", "--emoji", "--reset"] {
            assert!(usage.contains(option), "{option} is missing from the usage");
        }
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "repo-icon-popover");
        assert_eq!(SURFACE.root, "repo-icon-popover-popup");
        assert!(!SURFACE.full_bleed);
    }
}
