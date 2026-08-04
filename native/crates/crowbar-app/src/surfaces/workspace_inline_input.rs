//! `--surface workspace-inline-input` — the sidebar's rename/create field.
//!
//! See `crowbar_ui::surfaces::workspace::workspace_inline_input` for the port itself
//! and `native/mapping/workspace-inline-input.md` for the measurement.
//!
//! # What each axis can and cannot do here
//!
//! | Axis | Here |
//! |---|---|
//! | `--width` | **real** — the root and both children stretch to it |
//! | `--content` | **weak on the field** (`ANCHORS.md`'s own "an `<input>` has no text node" finding, `input.rs`'s precedent) — **real on the hint**, whose wrapped height moves with the typed value's length |
//! | `--theme` | **real** on the hint (`text-muted-foreground/70`); invisible on the field (`fg` is not a field this surface's `input` anchor carries at all) |
//! | `--viewport-width` | **vacuous** — neither element carries an `sm:` rule |
//!
//! # The state axis
//!
//! `empty` is real: it is what `defaultValue=''` (every live call site's own
//! starting state, and the only state a fresh create-child row is ever in)
//! renders — the placeholder instead of a value. `--hint` is this surface's
//! own option rather than a §8.3 flag: no word in that vocabulary names "a
//! collision was found", and driving it directly is the `nav-stack`/
//! `sidebar-peek` precedent for a boolean a real store/pointer action would
//! otherwise compute.
//!
//! `hover`, `focus`, `selected` and `loading` are unmodelled: this component's
//! own React source carries no `hover:`/`focus-visible:`/`data-active` rule of
//! its own on either element (the hint's `hover:text-foreground` has no
//! reference — synthetic pointer events are denied on this project's
//! machines, `button.rs`'s standing finding), and there is nothing here that
//! loads.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::surfaces::workspace::workspace_inline_input::{self, Kind, WorkspaceInlineInput};
use crowbar_ui::AnchorSink;
use crowbar_ui::surfaces::rows::ContentLength;
use gpui::{AnyElement, SharedString};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "workspace-inline-input",
    root: workspace_inline_input::ID_ROOT,
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // The probed root is 19.5 tall with no hint and 53.5 with one (294px
    // width) — plus `CAPTION_HEIGHT`'s 29. A floor, not a ceiling: the root is
    // `flex-1` and takes whatever column it is given.
    min_window_height: 120,
    // An inline row leaf, not viewport-filling chrome — `inline-error`'s and
    // `detach-holder-modal`'s own posture.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--kind`.
    pub kind: Kind,
    /// `--hint`: whether `resolveExisting(value)` matched.
    pub hint: bool,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            kind: Kind::Identifier,
            hint: false,
        }
    }
}

impl Params {
    /// The field this cell describes.
    #[must_use]
    pub fn field(&self, cell: &Cell) -> WorkspaceInlineInput {
        let placeholder = match self.kind {
            Kind::Identifier => "branch-name",
            Kind::Prose => "chat title",
        };
        WorkspaceInlineInput {
            value: if cell.has(StateFlag::Empty) {
                None
            } else {
                Some(value_of(cell.content))
            },
            placeholder: SharedString::new_static(placeholder),
            kind: self.kind,
            hint: self.hint,
        }
    }
}

/// The typed value a content length drives — always a plausible branch name,
/// since `--hint` reuses this same string for the hint's own text and a
/// non-identifier string there would misdescribe what the hint claims to
/// have matched.
fn value_of(content: ContentLength) -> SharedString {
    match content {
        ContentLength::Short => SharedString::new_static("main"),
        ContentLength::Normal => SharedString::new_static("fix-auth-bug"),
        ContentLength::Overflow => {
            SharedString::new_static("feature/rewrite-the-onboarding-flow-end-to-end")
        }
    }
}

fn parse_kind(raw: &str) -> Result<Kind, ParseError> {
    match raw {
        "identifier" => Ok(Kind::Identifier),
        "prose" => Ok(Kind::Prose),
        _ => Err(ParseError::Rejected(format!(
            "--kind takes identifier or prose, not {raw}"
        ))),
    }
}

impl SurfaceParams for Params {
    fn accept(
        &mut self,
        option: &str,
        args: &mut dyn Iterator<Item = String>,
    ) -> Result<bool, ParseError> {
        match option {
            "--kind" => {
                let raw = crate::surface::value(args, option)?;
                self.kind = parse_kind(&raw)?;
            }
            "--hint" => self.hint = true,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** The root is `flex-1` and takes the column it is given.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(
            out,
            " · kind {}",
            match self.kind {
                Kind::Identifier => "identifier",
                Kind::Prose => "prose",
            },
        );
        if cell.has(StateFlag::Empty) {
            out.push_str(" · empty: placeholder shown");
        } else {
            let _ = write!(out, " · value \"{}\"", value_of(cell.content));
        }
        if self.hint {
            out.push_str(" · hint: resolveExisting matched");
        }
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.field(cell).render(theme, anchors)
    }
}

fn options() -> Vec<(String, String)> {
    vec![
        (
            "--kind <identifier|prose>".to_owned(),
            "which face a typed value takes; identifier is font-mono, prose the ambient sans"
                .to_owned(),
        ),
        (
            "--hint".to_owned(),
            "renders the `resolveExisting` collision hint, naming the same value the field \
             paints"
                .to_owned(),
        ),
    ]
}

#[cfg(test)]
mod tests {
    use super::{Params, parse_kind, value_of};
    use crate::row_surface::Cell;
    use crowbar_ui::surfaces::rows::ContentLength;
    use crowbar_ui::surfaces::workspace::workspace_inline_input::Kind;

    /// A cell on this surface, via the same CLI-parsing constructor the
    /// binary itself uses — `Cell::params` is private to `row_surface`, so a
    /// struct literal cannot build one outside it (`detach_holder_modal.rs`'s
    /// own test idiom).
    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "workspace-inline-input"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    /// `--kind` accepts exactly its two words.
    #[test]
    fn kind_parses_its_two_words_and_rejects_everything_else() {
        assert_eq!(parse_kind("identifier"), Ok(Kind::Identifier));
        assert_eq!(parse_kind("prose"), Ok(Kind::Prose));
        assert!(parse_kind("mono").is_err());
        assert!(parse_kind("").is_err());
    }

    /// The three content lengths are three distinct, plausible branch names.
    #[test]
    fn every_content_length_is_a_distinct_branch_name() {
        let short = value_of(ContentLength::Short);
        let normal = value_of(ContentLength::Normal);
        let overflow = value_of(ContentLength::Overflow);
        assert_ne!(short, normal);
        assert_ne!(normal, overflow);
        assert!(short.len() < normal.len());
        assert!(normal.len() < overflow.len());
    }

    /// `empty` drops the value in favour of the placeholder, and `--hint`
    /// carries into the field's own `hint` flag unchanged.
    #[test]
    fn empty_shows_the_placeholder_and_hint_passes_through() {
        let mut params = Params {
            kind: Kind::Identifier,
            hint: true,
        };
        let field = params.field(&cell(&["--flags", "empty"]));
        assert!(field.value.is_none());
        assert!(field.hint);

        params.hint = false;
        let field = params.field(&cell(&[]));
        assert!(field.value.is_some());
        assert!(!field.hint);
    }

    /// `--kind prose` reaches the field's own placeholder default.
    #[test]
    fn prose_kind_takes_the_chat_title_placeholder() {
        let params = Params {
            kind: Kind::Prose,
            hint: false,
        };
        let field = params.field(&cell(&[]));
        assert_eq!(field.placeholder, "chat title");
        assert_eq!(field.kind, Kind::Prose);
    }
}
