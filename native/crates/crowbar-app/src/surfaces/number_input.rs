//! `--surface number-input` — a flex row of two icon buttons and a text
//! field, and the first surface whose **`hover` state comes from a composed
//! child** rather than from the surface's own class list.
//!
//! One `<NumberInput>` in one cell of the §8.3 matrix. The default cell is
//! the live Settings dialog's "UI Font Size" field —
//! `web/src/features/settings/components/tabs/appearance-settings.tsx`,
//! `size="xs"` at `.number` width (`w-28`) — measured at `112×32` at
//! `innerWidth` 1714.
//!
//! See `crowbar_ui::components::number_input`'s module docs for the wrap-or-
//! build seam test, the full value tables and `native/mapping/number-input.md`
//! for the §6.2 row.
//!
//! # What each axis can and cannot do here
//!
//! | Axis | Here |
//! |---|---|
//! | `--width` | **vacuous on this surface's own axis** — `RowSurface`'s `--width` sizes the *column* the surface draws into, and this component's root has its own authored width (`--class-width`) inside it, unrelated to the column. Use `--class-width` to move the picture |
//! | `--viewport-width` | real, and asymmetric: moves only the two buttons' height — see `number_input.rs`'s own §5 |
//! | `--theme` | real: the field's `bg-muted`/`border-border` and (when `--flags hover`) the buttons' `bg-accent` are all theme-dependent tokens |
//! | `--content` | **vacuous on every cell** — the field is a void `<input>`, so no anchor paints text either side |
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `hover` | **real** — `hover:bg-accent` on the composed ghost `Button`s. **No reference**: synthetic pointer events are denied |
//! | `empty` | unmodelled — `number-input.tsx` contains no such rule; `formatValue` never returns an empty string |
//! | `focus` | unmodelled — `number-input.tsx` has zero `focus`-related classes, counted directly |
//! | `selected` | unmodelled — zero `selected`/`data-selected` classes |
//! | `loading` | unmodelled, as on every surface |
//! | `error` | unmodelled — zero `aria-invalid` classes; this surface has no `--invalid` escape hatch because there is no rule to drive |

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::number_input::{ALL_SIZES, ALL_WIDTHS, NumberInput, Size, Width};
use crowbar_ui::components::{AnchorSink, number_input};
use gpui::AnyElement;

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "number-input",
    root: number_input::ID_ROOT,
    unmodelled: &[
        // `number-input.tsx` has no `empty`/`focus`/`selected`/`aria-invalid`
        // rule at all — grepped, not assumed. See the module docs.
        StateFlag::Empty,
        StateFlag::Focus,
        StateFlag::Selected,
        StateFlag::Loading,
        StateFlag::Error,
    ],
    // The taller button is 36px (base breakpoint) plus `CAPTION_HEIGHT`'s 29.
    // 72 holds it with room; a floor rather than a ceiling, matching
    // `input`/`button`'s own reasoning — this surface drives no height.
    min_window_height: 72,
    // A settings row, never the window itself.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--size`.
    pub size: Size,
    /// `--class-width`: the call site's merged root width.
    pub width: Width,
    /// `--disabled`: the `disabled` prop. Live, and invisible.
    pub disabled: bool,
    /// `--at-min`: `canDecrement` is `false`.
    pub at_min: bool,
    /// `--at-max`: `canIncrement` is `false`.
    pub at_max: bool,
}

impl Default for Params {
    fn default() -> Self {
        let fixture = NumberInput::fixture();
        Self {
            size: fixture.size,
            width: fixture.width,
            disabled: fixture.disabled,
            at_min: !fixture.buttons.can_decrement,
            at_max: !fixture.buttons.can_increment,
        }
    }
}

impl Params {
    /// The number input this cell describes.
    ///
    /// Built by taking the live fixture and applying the cell, `input`'s own
    /// convention — a bare `--surface number-input` renders the field the
    /// reference actually has.
    #[must_use]
    pub fn number_input(&self, cell: &Cell) -> NumberInput {
        let mut field = NumberInput::fixture();
        field.size = self.size;
        field.width = self.width;
        // A **viewport** media query, so it follows the viewport rather than
        // `--width` — `input`'s own reasoning, restated.
        field.breakpoint = cell.breakpoint();
        field.disabled = self.disabled;
        field.buttons.can_decrement = !self.at_min && !self.disabled;
        field.buttons.can_increment = !self.at_max && !self.disabled;
        field.buttons.hovered = cell.has(StateFlag::Hover);
        field.value = value_of(cell.content).into();
        field
    }
}

/// The digit string a content length stands for — this surface's own
/// vocabulary, since a field's number and a git row's path are different
/// kinds of string. **Vacuous**: no anchor here paints text, so the choice
/// changes nothing the differ compares.
fn value_of(content: crowbar_ui::components::ContentLength) -> &'static str {
    use crowbar_ui::components::ContentLength;
    match content {
        ContentLength::Short => "5",
        ContentLength::Normal => "15",
        ContentLength::Overflow => "123456",
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
            "--class-width" => self.width = parse_width(&value(args, option)?)?,
            "--disabled" => self.disabled = true,
            "--at-min" => self.at_min = true,
            "--at-max" => self.at_max = true,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** A field's height is its size variant's and the taller
    /// button's, and no option here sets either — `input`'s own call.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(out, " · {}", self.size.name());
        let _ = write!(out, " · {}", self.width.name());

        if !self.size.live() {
            out.push_str(" · size: no live call site asks for it, so there is no reference");
        }

        if self.disabled {
            out.push_str(
                " · disabled: no live call site passes it, and it is invisible either way \
                 (opacity-64 never reaches v1.7's zero)",
            );
        }
        if self.at_min || self.at_max {
            out.push_str(
                " · a stepper button at its bound: disabled:opacity-64 has no field \
                 either, so this cell cannot fail",
            );
        }

        out.push_str(
            " · an <input> has no text node, so the reference emits no text/fg/font for \
             the field and --content cannot fail",
        );

        if cell.has(StateFlag::Hover) {
            out.push_str(
                " · hover: hover:bg-accent on the composed ghost Button really does move \
                 a field the contract compares, but synthetic pointer events are denied \
                 on this project's machines, so there is no reference",
            );
        }
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.number_input(cell).render(theme, anchors)
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

fn parse_width(raw: &str) -> Result<Width, ParseError> {
    ALL_WIDTHS
        .into_iter()
        .find(|width| width.name() == raw)
        .ok_or_else(|| {
            ParseError::Rejected(format!(
                "--class-width names the class a call site merges onto the root — one of \
                 {} — and never a pixel value. A numeric knob would let a cell be tuned to \
                 whatever the reference happened to report; a class is an input both \
                 engines resolve through their own --spacing. Got {raw}",
                ALL_WIDTHS.map(Width::name).join(", "),
            ))
        })
}

fn options() -> Vec<(String, String)> {
    let fixture = Params::default();
    [
        (
            "--size <name>".to_owned(),
            format!(
                "xs|sm|md; only xs has a live call site [{}]",
                fixture.size.name(),
            ),
        ),
        (
            "--class-width <class>".to_owned(),
            format!(
                "compact|number|default; the class a call site merges onto the root, \
                 never a pixel value [{}]",
                fixture.width.name(),
            ),
        ),
        (
            "--disabled".to_owned(),
            "the disabled prop: invisible either way [off]".to_owned(),
        ),
        (
            "--at-min".to_owned(),
            "canDecrement is false: opacity-64 on the decrement button, invisible [off]".to_owned(),
        ),
        (
            "--at-max".to_owned(),
            "canIncrement is false: opacity-64 on the increment button, invisible [off]".to_owned(),
        ),
    ]
    .into_iter()
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{Params, SURFACE, options};
    use crate::row_surface::{Cell, ParseError, StateFlag};
    use crowbar_ui::components::number_input::{
        ID_DECREMENT, ID_FIELD, ID_INCREMENT, ID_ROOT, NumberInput, Size, Width,
    };

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "number-input"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a number-input cell carries this surface's bag")
    }

    fn built(cell: &Cell) -> NumberInput {
        params_of(cell).number_input(cell)
    }

    /// The defaults are the live "UI Font Size" cell, and the bag rebuilds it
    /// rather than producing something that merely resembles it.
    #[test]
    fn the_defaults_are_the_live_ui_font_size_cell() {
        let bag = Params::default();
        let fixture = NumberInput::fixture();

        assert_eq!(bag.size, fixture.size);
        assert_eq!(bag.width, fixture.width);
        assert!(!bag.disabled);
        assert!(!bag.at_min);
        assert!(!bag.at_max);

        let built = built(&cell(&[]));
        assert_eq!(built.size, fixture.size);
        assert_eq!(built.width, fixture.width);
        assert_eq!(built.value, fixture.value);
        assert!(built.buttons.can_decrement);
        assert!(built.buttons.can_increment);
        assert!(!built.buttons.hovered);
    }

    /// **`hover` reaches the component and moves a real field** — the module
    /// docs' own state-axis finding, pinned.
    #[test]
    fn hover_reaches_the_component() {
        let resting = built(&cell(&[]));
        let hovered = built(&cell(&["--flags", "hover"]));
        assert!(!resting.buttons.hovered);
        assert!(hovered.buttons.hovered);
        assert!(!SURFACE.unmodelled(StateFlag::Hover));

        let described = cell(&["--flags", "hover"]).describe();
        assert!(described.contains("hover:bg-accent"), "{described}");
        assert!(described.contains("no reference"), "{described}");
    }

    /// `--at-min`/`--at-max` reach the two buttons independently, and
    /// `--disabled` overrides both.
    #[test]
    fn the_bound_flags_reach_the_two_buttons_independently() {
        assert!(!built(&cell(&["--at-min"])).buttons.can_decrement);
        assert!(built(&cell(&["--at-min"])).buttons.can_increment);
        assert!(!built(&cell(&["--at-max"])).buttons.can_increment);
        assert!(built(&cell(&["--at-max"])).buttons.can_decrement);

        let disabled = built(&cell(&["--disabled"]));
        assert!(!disabled.buttons.can_decrement);
        assert!(!disabled.buttons.can_increment);
        assert!(disabled.disabled);
    }

    /// Every size and width reach the component, and the dead ones say so.
    #[test]
    fn every_size_and_width_reach_the_component() {
        for (word, size) in [("xs", Size::Xs), ("sm", Size::Sm), ("md", Size::Md)] {
            assert_eq!(built(&cell(&["--size", word])).size, size, "{word}");
        }
        for (word, width) in [
            ("compact", Width::Compact),
            ("number", Width::Number),
            ("default", Width::Default),
        ] {
            assert_eq!(
                built(&cell(&["--class-width", word])).width,
                width,
                "{word}"
            );
        }

        for dead in ["sm", "md"] {
            let described = cell(&["--size", dead]).describe();
            assert!(described.contains("no live"), "{described}");
        }
        let live = cell(&["--size", "xs"]).describe();
        assert!(!live.contains("size: no live"), "{live}");
    }

    /// The vocabulary is closed, and a pixel value for `--class-width` is a
    /// rejection that says why.
    #[test]
    fn the_option_vocabulary_is_closed() {
        for line in [
            vec!["--size", "large"],
            vec!["--size"],
            vec!["--class-width", "96"],
            vec!["--class-width", "w-28"],
            vec!["--class-width"],
        ] {
            let mut full = vec!["--surface", "number-input"];
            full.extend_from_slice(&line);
            assert!(
                matches!(
                    Cell::parse(full.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{line:?} should have been rejected",
            );
        }

        let Err(ParseError::Rejected(pixels)) = Cell::parse(
            ["--surface", "number-input", "--class-width", "96"]
                .iter()
                .map(|arg| (*arg).to_owned()),
        ) else {
            panic!("--class-width takes a class");
        };
        assert!(pixels.contains("never a pixel value"), "{pixels}");
    }

    /// These options belong to this surface and to no other.
    #[test]
    fn this_surfaces_options_are_rejected_on_another_surface() {
        for line in [
            vec!["--surface", "dropdown-menu", "--class-width", "number"],
            vec!["--surface", "dropdown-menu", "--at-min"],
            vec!["--surface", "dropdown-menu", "--at-max"],
        ] {
            assert!(
                matches!(
                    Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{line:?} should have been rejected",
            );
        }
    }

    /// Every option the parser accepts appears in the usage.
    #[test]
    fn the_usage_names_every_option_this_surface_takes() {
        let usage = crate::row_surface::usage();
        assert!(usage.contains("number-input"));
        for option in [
            "--size",
            "--class-width",
            "--disabled",
            "--at-min",
            "--at-max",
        ] {
            assert!(usage.contains(option), "{option} is missing from the usage");
        }
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    /// The registry entry's contract fields and the state axis this surface
    /// really has.
    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "number-input");
        assert_eq!(SURFACE.root, "number-input");
        assert_eq!(SURFACE.root, ID_ROOT);
        assert!(!SURFACE.full_bleed);

        assert!(!SURFACE.unmodelled(StateFlag::Hover));
        for flag in [
            StateFlag::Empty,
            StateFlag::Focus,
            StateFlag::Selected,
            StateFlag::Loading,
            StateFlag::Error,
        ] {
            assert!(SURFACE.unmodelled(flag), "{flag:?}");
        }

        // The three anchor ids beneath the root, for a reader cross-checking
        // against the component's own module.
        assert_ne!(ID_DECREMENT, ID_FIELD);
        assert_ne!(ID_FIELD, ID_INCREMENT);
    }
}
