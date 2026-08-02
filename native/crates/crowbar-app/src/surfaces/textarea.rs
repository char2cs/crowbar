//! `--surface textarea` — built, not wrapped, and unreached by any parity run
//! today.
//!
//! `crowbar_ui::components::textarea` carries the seam evidence, the full
//! value tables and the reachability measurement; this file is the cell.
//! **There is no `/tmp/p3-ref-textarea.json`** — see that module's docs'
//! §5 for why (a throwaway-element measurement, not a mounted capture), and
//! `native/mapping/textarea.md` for the §6.2 row.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `focus` | real in the class list (`has-focus-visible:border-ring`) and **unreachable**: `document.hasFocus()` is `false` on this machine, `input`'s own standing finding |
//! | `hover`, `selected`, `empty` | unmodelled — `textarea.tsx` contains none of `hover`, `selected`/`data-selected`, or a rule for an empty field, at zero substring matches each |
//! | `loading`, `error` | unmodelled, as every surface must declare. `error` is real (`has-aria-invalid:border-destructive/36`, byte-identical to `input.tsx`'s) and driven by `--invalid` instead — `input`/`checkbox`/`radio-group`'s own precedent for the identical rule |

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::AnchorSink;
use crowbar_ui::components::textarea::{self, ALL_SIZES, CallSite, Size, State, Textarea};

use gpui::AnyElement;

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "textarea",
    root: textarea::ID_CONTROL,
    unmodelled: &[
        StateFlag::Hover,
        StateFlag::Selected,
        StateFlag::Empty,
        StateFlag::Loading,
        // Real (`has-aria-invalid:*`), driven by `--invalid` instead —
        // `input`/`checkbox`/`radio-group`'s precedent for the identical rule.
        StateFlag::Error,
    ],
    // The tallest fixture is the call site's own 80px control plus the
    // caption's 29 — a floor, not a ceiling: this surface drives no height.
    min_window_height: 112,
    // A textarea sits inside a popover's body — never the window itself.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Params {
    /// `--size`. **No live reference** for any arm.
    pub size: Size,
    /// `--bare`: drop the one live call site's `className`, rendering the
    /// primitive's own defaults instead.
    pub bare: bool,
    /// `--disabled`: the `disabled` prop, which `commit-popover.tsx` passes
    /// while committing (`isCommitting`). Invisible either way.
    pub disabled: bool,
    /// `--invalid`: `aria-invalid`. See the module docs for why it is here
    /// rather than on the `error` flag.
    pub invalid: bool,
}

impl Params {
    /// The textarea this cell describes: the live call site's own shape by
    /// default, driven by the cell's `--flags focus` and this bag's own
    /// options.
    #[must_use]
    pub fn textarea(self, cell: &Cell) -> Textarea {
        let mut field = Textarea::fixture();
        field.size = self.size;
        if self.bare {
            field.call_site = CallSite::Bare;
        }
        field.state = State {
            focused: cell.has(StateFlag::Focus),
            disabled: self.disabled,
            invalid: self.invalid,
        };
        field
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
            "--bare" => self.bare = true,
            "--disabled" => self.disabled = true,
            "--invalid" => self.invalid = true,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** The control's height is either an authored `min-h-*` or a
    /// stretched child — neither is an option on this command line —
    /// `input`/`radio-group`'s own answer.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(out, " · {}", self.size.name());
        let _ = write!(
            out,
            " · {}",
            if self.bare {
                "bare (no call-site className)"
            } else {
                "commit-message (commit-popover.tsx's own className)"
            },
        );

        if !self.bare {
            out.push_str(
                " · the call site's min-h-20 stretches the field past its own floor \
                 (78px against a 70px min-h-17.5) — see the module docs' §7",
            );
        }

        if self.disabled {
            out.push_str(" · disabled: opacity-64, invisible (v1.7 fires only at zero)");
        }
        if self.invalid {
            out.push_str(
                " · invalid: aria-invalid moves the control's border colour, which is \
                 compared, but no live <Textarea passes it — there is no reference either \
                 way",
            );
        }
        if cell.has(StateFlag::Focus) {
            out.push_str(
                " · focus: has-focus-visible:border-ring really does move a compared \
                 field, but document.hasFocus() is false on this machine, so there is no \
                 reference",
            );
        }

        out.push_str(" · a <textarea> has no text node — --content cannot fail");
        out.push_str(
            " · NO LIVE REFERENCE: textarea.tsx's only importer, commit-popover.tsx, sits \
             behind a git panel whose Changes list would not populate in this item's dev \
             environment even after a real git/stage API call and a full reload — see \
             native/mapping/textarea.md",
        );
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.textarea(cell).render(theme, anchors)
    }
}

fn parse_size(raw: &str) -> Result<Size, ParseError> {
    ALL_SIZES
        .into_iter()
        .find(|size| size.name() == raw)
        .ok_or_else(|| {
            ParseError::Rejected(format!(
                "--size takes one of {}, not {raw}",
                ALL_SIZES.map(Size::name).join(", "),
            ))
        })
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--size <name>".to_owned(),
            "sm|default|lg; no live call site asks for any of them [default]".to_owned(),
        ),
        (
            "--bare".to_owned(),
            "drop commit-popover.tsx's className, rendering the primitive's own \
             defaults [off]"
                .to_owned(),
        ),
        (
            "--disabled".to_owned(),
            "the disabled prop: invisible either way [off]".to_owned(),
        ),
        (
            "--invalid".to_owned(),
            "aria-invalid — §8.3's error state, which every surface must declare \
             unmodelled [off]"
                .to_owned(),
        ),
    ]
    .into_iter()
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{Params, SURFACE, options};
    use crate::row_surface::{Cell, ParseError, StateFlag};
    use crowbar_ui::Theme;
    use crowbar_ui::components::textarea::{CallSite, ID_CONTROL, ID_FIELD, Size, Textarea};

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "textarea"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> Params {
        *cell
            .surface_params::<Params>()
            .expect("a textarea cell carries this surface's bag")
    }

    fn built(cell: &Cell) -> Textarea {
        params_of(cell).textarea(cell)
    }

    /// The defaults are the live call site's own shape, and the bag rebuilds
    /// it rather than producing something that merely resembles it.
    #[test]
    fn the_defaults_are_the_live_call_sites_shape() {
        let bag = Params::default();
        let fixture = Textarea::fixture();

        assert_eq!(bag.size, fixture.size);
        assert!(!bag.bare);
        assert!(!bag.disabled);
        assert!(!bag.invalid);

        let built = built(&cell(&[]));
        assert_eq!(built.call_site, CallSite::CommitMessage);
        assert_eq!(built.size, Size::Default);
    }

    /// `--bare` drops the call site's own className, and the field height
    /// falls back to its own floor.
    #[test]
    fn bare_drops_the_call_sites_class_name() {
        let commit_message = built(&cell(&[]));
        let bare = built(&cell(&["--bare"]));

        assert_eq!(commit_message.call_site, CallSite::CommitMessage);
        assert_eq!(bare.call_site, CallSite::Bare);
        assert_ne!(commit_message.field_height(), bare.field_height());
        assert_eq!(bare.field_height(), Size::Default.min_height());
    }

    /// The two props reach the component, and each is captioned apart from
    /// the always-present "no live reference" line.
    #[test]
    fn the_two_props_reach_the_component() {
        let disabled = built(&cell(&["--disabled"]));
        assert!(disabled.state.disabled);
        assert!(!disabled.has_shadow());

        let invalid = built(&cell(&["--invalid"]));
        assert!(invalid.state.invalid);
        assert_ne!(
            invalid.border_color(&Theme::DARK),
            Textarea::fixture().border_color(&Theme::DARK),
        );

        assert!(SURFACE.unmodelled(StateFlag::Error));
    }

    /// `focus` reaches a compared field and still has no reference.
    #[test]
    fn focus_moves_a_compared_field_and_still_has_no_reference() {
        let focused = built(&cell(&["--flags", "focus"]));
        assert!(focused.state.focused);
        assert!(!focused.has_shadow());
        assert_ne!(
            focused.border_color(&Theme::DARK),
            Textarea::fixture().border_color(&Theme::DARK),
        );

        let described = cell(&["--flags", "focus"]).describe();
        assert!(described.contains("compared"), "{described}");
        assert!(described.contains("no reference"), "{described}");
        assert!(!SURFACE.unmodelled(StateFlag::Focus));
    }

    /// The caption always says there is no live reference.
    #[test]
    fn the_caption_always_says_there_is_no_live_reference() {
        for args in [
            vec![],
            vec!["--bare"],
            vec!["--invalid"],
            vec!["--flags", "focus"],
        ] {
            let described = cell(&args).describe();
            assert!(described.contains("NO LIVE REFERENCE"), "{described}");
        }
    }

    /// Every size reaches the component; none has a live reference.
    #[test]
    fn every_size_reaches_the_component() {
        for (word, size) in [
            ("sm", Size::Sm),
            ("default", Size::Default),
            ("lg", Size::Lg),
        ] {
            assert_eq!(built(&cell(&["--size", word])).size, size, "{word}");
        }
    }

    /// The vocabulary is closed.
    #[test]
    fn the_option_vocabulary_is_closed() {
        for line in [vec!["--size", "medium"], vec!["--size"]] {
            let mut full = vec!["--surface", "textarea"];
            full.extend_from_slice(&line);
            assert!(
                matches!(
                    Cell::parse(full.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{line:?} should have been rejected",
            );
        }
    }

    /// These options belong to this surface and to no other.
    #[test]
    fn this_surfaces_options_are_rejected_on_another_surface() {
        for option in ["--bare", "--disabled", "--invalid"] {
            let line = ["--surface", "dropdown-menu", option];
            assert!(
                matches!(
                    Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{option} should not be a dropdown-menu option",
            );
        }
    }

    /// Every option the parser accepts appears in the usage.
    #[test]
    fn the_usage_names_every_option_this_surface_takes() {
        let usage = crate::row_surface::usage();
        assert!(usage.contains("textarea"));
        for option in ["--size", "--bare", "--disabled", "--invalid"] {
            assert!(usage.contains(option), "{option} is missing from the usage");
        }
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    /// The registry entry's contract fields and the state axis this surface
    /// has.
    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "textarea");
        assert_eq!(SURFACE.root, "textarea-control");
        assert_eq!(SURFACE.root, ID_CONTROL);
        assert_ne!(ID_CONTROL, ID_FIELD);
        assert!(!SURFACE.full_bleed);

        assert!(!SURFACE.unmodelled(StateFlag::Focus));
        for flag in [
            StateFlag::Hover,
            StateFlag::Selected,
            StateFlag::Empty,
            StateFlag::Loading,
            StateFlag::Error,
        ] {
            assert!(SURFACE.unmodelled(flag), "{flag:?}");
        }
    }
}
