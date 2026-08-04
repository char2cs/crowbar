//! `--surface toast` — a **built, not wrapped**, content-sized surface with no
//! live producer at all.
//!
//! `crowbar_ui::primitives::toast` carries the seam evidence (§1) and the
//! reachability finding (§2: `anchoredToastManager.add` has zero call sites in
//! `web/src`, so `toast.tsx`'s only rendering export never paints); this file
//! is the cell. There is no live fixture to default to — every default below
//! is `Toast::fixture()`'s own declared, unmeasured shape.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `empty` | **real, on the same arithmetic `tooltip`'s is.** Blanks the title to `""` — a real, if unreached, picture: `Toast.Title` renders whatever `toast.title` is, and nothing in `toast.tsx` refuses an empty string. |
//! | `hover`, `focus`, `selected` | **unmodelled.** Neither branch of `toast.tsx`'s class list carries a `hover:`/`focus:`/`active:` rule. |
//! | `loading`, `error` | unmodelled, as on every surface — though `error` is coincidentally also one of [`Variant`]'s *icons* (`toast.type`), a different axis entirely; see `--type`. |
//!
//! Four of the six are declared on [`SURFACE`].

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::AnchorSink;
use crowbar_ui::primitives::toast::{self, Toast, Variant};
use gpui::{AnyElement, IntoElement as _, ParentElement as _, SharedString, Styled as _, div, px};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, pixels, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "toast",
    root: toast::ID_POPUP,
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // Big enough for the fixture (title + description, ~66px) plus the
    // caption below it, with headroom — a floor, not a ceiling: this surface
    // drives no height of its own (like `tooltip`'s, `driven_height` is
    // `None`).
    min_window_height: 110,
    // A popup floated over the page, whose width is its own content's — the
    // same declaration `tooltip`'s and `popover`'s both make.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// `--title`'s default: `Toast::fixture`'s own title.
const DEFAULT_TITLE: &str = "Saved";

/// `--description`'s default: `Toast::fixture`'s own description.
const DEFAULT_DESCRIPTION: &str = "Your changes have been saved.";

/// This surface's own options.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Params {
    /// `--variant`: `default` (icon/title/description/action) or `tooltip`
    /// (a bare title).
    pub variant: Variant,
    /// `--title`: the toast's own text.
    pub title: SharedString,
    /// `--description`: renders a `Toast.Description`. **The `default`
    /// variant only** — silently inert under `tooltip`, the same relationship
    /// `dialog`'s `--description` has to a call site that never nests one.
    pub description: Option<SharedString>,
    /// `--icon`: renders the leading type icon. **The `default` variant
    /// only.**
    pub icon: bool,
    /// `--action-height <px>`: renders the action row at this content
    /// height. **The `default` variant only.** `None` omits it.
    pub action_height: Option<u16>,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            variant: Variant::Default,
            title: SharedString::new_static(DEFAULT_TITLE),
            description: Some(SharedString::new_static(DEFAULT_DESCRIPTION)),
            icon: true,
            action_height: None,
        }
    }
}

impl Params {
    /// The toast this cell describes.
    ///
    /// `empty` blanks the title — the one flag this surface models, the same
    /// role it plays on `tooltip`.
    #[must_use]
    pub fn toast(&self, cell: &Cell) -> Toast {
        Toast {
            variant: self.variant,
            title: if cell.has(StateFlag::Empty) {
                SharedString::new_static("")
            } else {
                self.title.clone()
            },
            description: if self.variant == Variant::Tooltip {
                None
            } else {
                self.description.clone()
            },
            icon: self.variant == Variant::Default && self.icon,
            action_height: if self.variant == Variant::Tooltip {
                None
            } else {
                self.action_height.map(|h| px(f32::from(h)))
            },
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
            "--variant" => self.variant = parse_variant(&value(args, option)?)?,
            "--title" => self.title = SharedString::from(value(args, option)?),
            "--description" => self.description = Some(SharedString::from(value(args, option)?)),
            "--no-description" => self.description = None,
            // Not `--no-icon`: that word is already claimed globally, by
            // `git-status-row`/`file-tree-row`'s own `show_file_icon`
            // (`row_surface.rs`'s shared parser matches it *before* falling
            // through to any surface's own `accept`, so this arm would
            // silently never run under that name — found empirically, by a
            // test that set `--no-icon` and then asserted `!toast.icon`).
            "--no-toast-icon" => self.icon = false,
            "--action-height" => {
                self.action_height = Some(pixels(&value(args, option)?, option)?);
            }
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** Like `tooltip`'s, the box's height is its own content's — no
    /// option here sets one.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let toast = self.toast(cell);
        let variant = match toast.variant {
            Variant::Default => "default",
            Variant::Tooltip => "tooltip",
        };
        let _ = write!(out, " · {variant} · title {:?}", toast.title.as_ref());
        if let Some(description) = &toast.description {
            let _ = write!(out, " · description {description:?}");
        }
        if cell.has(StateFlag::Empty) {
            out.push_str(" · empty: title blanked to \"\"");
        }
        out.push_str(" · no live pixel reference — see toast.rs's module docs §2");
    }

    /// The toast, inside the flex row that makes it a flex item.
    ///
    /// `RowSurface` draws every surface into a gpui **block** container; a
    /// content-sized box drawn straight into one stretches to the container's
    /// width rather than hugging its own content. `tooltip`'s identical fix,
    /// applied here — see that surface's own comment. The row carries no
    /// anchor, so it cannot reach a snapshot.
    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        div()
            .flex()
            .flex_row()
            .child(self.toast(cell).render(theme, anchors))
            .into_any_element()
    }
}

fn parse_variant(raw: &str) -> Result<Variant, ParseError> {
    match raw {
        "default" => Ok(Variant::Default),
        "tooltip" => Ok(Variant::Tooltip),
        other => Err(ParseError::Rejected(format!(
            "--variant takes default or tooltip, not {other}"
        ))),
    }
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--variant default|tooltip",
            "which of toast.tsx's two appearances [default]".to_owned(),
        ),
        (
            "--title <text>",
            format!("the toast's own text [{DEFAULT_TITLE:?}]"),
        ),
        (
            "--description <text>",
            format!("default variant only [{DEFAULT_DESCRIPTION:?}]"),
        ),
        ("--no-description", "default variant only".to_owned()),
        ("--no-toast-icon", "default variant only".to_owned()),
        (
            "--action-height <px>",
            "renders the action row at this content height; default variant only".to_owned(),
        ),
    ]
    .into_iter()
    .map(|(option, description): (&str, String)| (option.to_owned(), description))
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{DEFAULT_DESCRIPTION, DEFAULT_TITLE, Params, SURFACE, options};
    use crate::row_surface::{Cell, ParseError, StateFlag};
    use crowbar_ui::primitives::toast::Variant;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "toast"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a toast cell carries this surface's bag")
    }

    /// The defaults are `Toast::fixture`'s own declared shape — no live
    /// pixels, and this test says so by construction rather than measurement.
    #[test]
    fn the_defaults_are_the_declared_fixture_shape() {
        let bag = Params::default();
        assert_eq!(bag.variant, Variant::Default);
        assert_eq!(bag.title.as_ref(), DEFAULT_TITLE);
        assert_eq!(bag.description.as_deref(), Some(DEFAULT_DESCRIPTION));
        assert!(bag.icon);
        assert_eq!(bag.action_height, None);

        let toast = params_of(&cell(&[])).toast(&cell(&[]));
        assert_eq!(toast.title.as_ref(), "Saved");
        assert_eq!(toast.description.as_deref(), Some(DEFAULT_DESCRIPTION));
        assert!(toast.icon);
    }

    /// `--variant tooltip` silences the description, the icon and the action
    /// regardless of what the other options say.
    #[test]
    fn the_tooltip_variant_silences_every_default_only_field() {
        let tooltip = cell(&[
            "--variant",
            "tooltip",
            "--action-height",
            "24",
            "--title",
            "Copied",
        ]);
        let toast = params_of(&tooltip).toast(&tooltip);

        assert_eq!(toast.variant, Variant::Tooltip);
        assert_eq!(toast.title.as_ref(), "Copied");
        assert_eq!(toast.description, None);
        assert!(!toast.icon);
        assert_eq!(toast.action_height, None);
    }

    /// `--no-description`/`--no-toast-icon`/`--action-height` each move exactly
    /// their own field, under the `default` variant.
    #[test]
    fn each_default_variant_option_moves_its_own_field() {
        let bare = cell(&["--no-description", "--no-toast-icon"]);
        let toast = params_of(&bare).toast(&bare);
        assert_eq!(toast.description, None);
        assert!(!toast.icon);
        assert_eq!(toast.action_height, None);

        let with_action = cell(&["--action-height", "24"]);
        assert_eq!(
            params_of(&with_action).toast(&with_action).action_height,
            Some(gpui::px(24.0)),
        );
    }

    /// **`empty` is the one flag this surface models**, and it blanks the
    /// title regardless of variant.
    #[test]
    fn empty_blanks_the_title_on_both_variants() {
        for variant in ["default", "tooltip"] {
            let empty_cell = cell(&["--flags", "empty", "--variant", variant, "--title", "x"]);
            let toast = params_of(&empty_cell).toast(&empty_cell);
            assert_eq!(toast.title.as_ref(), "", "{variant}");
        }
        assert!(!SURFACE.unmodelled(StateFlag::Empty));
    }

    /// The other four are declared unmodelled.
    #[test]
    fn the_four_interaction_flags_are_declared_unmodelled() {
        for flag in [
            StateFlag::Loading,
            StateFlag::Error,
            StateFlag::Hover,
            StateFlag::Focus,
            StateFlag::Selected,
        ] {
            assert!(SURFACE.unmodelled(flag), "{flag:?}");
        }
    }

    /// The caption always names the absence of a reference — every cell on
    /// this surface has none.
    #[test]
    fn the_caption_always_names_the_missing_reference() {
        assert!(cell(&[]).describe().contains("no live pixel reference"));
    }

    /// The vocabulary is closed and every rejection names what was wanted.
    #[test]
    fn the_variant_vocabulary_is_closed() {
        let Err(ParseError::Rejected(complaint)) =
            Cell::parse(["--surface", "toast", "--variant", "rich"].map(str::to_owned))
        else {
            panic!("`rich` is not a variant");
        };
        assert!(complaint.contains("default"), "{complaint}");
        assert!(complaint.contains("tooltip"), "{complaint}");
    }

    /// Every option the parser accepts appears in the usage.
    #[test]
    fn the_usage_names_every_option_this_surface_takes() {
        let usage = crate::row_surface::usage();
        for option in [
            "--variant",
            "--title",
            "--description",
            "--no-description",
            "--no-toast-icon",
            "--action-height",
        ] {
            assert!(usage.contains(option), "{option} is missing from the usage");
        }
        assert!(usage.contains("toast"));
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    /// This surface's options belong to it and to no other.
    #[test]
    fn this_surfaces_options_are_rejected_on_another_surface() {
        for option in [
            "--variant",
            "--title",
            "--no-description",
            "--no-toast-icon",
        ] {
            let line = ["--surface", "checkbox", option, "x"];
            assert!(
                matches!(
                    Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{option} should not be a checkbox option",
            );
        }
    }

    /// The registry entry's two contract fields.
    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "toast");
        assert_eq!(SURFACE.root, "toast-popup");
        assert!(!SURFACE.full_bleed);
    }
}
