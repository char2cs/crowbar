//! `--surface radio-group` — built, not wrapped, and unreached by any parity
//! run today.
//!
//! `crowbar_ui::components::radio_group` carries the seam evidence and the
//! reachability measurement in full; this file is the cell. **There is no
//! `/tmp/p3-ref-radio-group.json`** — see that module's docs for why, and
//! `native/mapping/radio-group.md` for the account.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `selected` | **real**, on the circle's own `bg` — `dark:not-data-checked:bg-input/32`, moved only in the dark table, `checkbox`'s exact shape confirmed independently on this control. |
//! | `focus` | real in the class list and invisible alone (`focus-visible:ring-2` is a box-shadow); combined with `--invalid` it moves the border colour. |
//! | `hover` | unmodelled — `radio-group.tsx` contains the substring `hover` zero times. |
//! | `empty` | unmodelled. A radio has no content to be empty of. |
//! | `loading`, `error` | unmodelled, as on every surface. `error` is real (`aria-invalid:border-destructive/36`) and driven by `--invalid` instead, `checkbox`'s own precedent for the identical rule. |

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::AnchorSink;
use crowbar_ui::components::radio_group::{self, Radio, RadioGroup, Validity};

use gpui::AnyElement;

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "radio-group",
    root: radio_group::ID_GROUP,
    unmodelled: &[
        StateFlag::Hover,
        StateFlag::Empty,
        StateFlag::Loading,
        // Real (`aria-invalid`), driven by `--invalid` instead — `checkbox`'s
        // exact precedent, named by that surface for this one.
        StateFlag::Error,
    ],
    // The 18px base-breakpoint circle plus the caption, with headroom — a
    // floor, not a ceiling: this surface drives no height.
    min_window_height: 72,
    // A radio sits inside a popover's body — never the window itself.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Params {
    /// `--disabled`: the `disabled` prop. Invisible either way
    /// (`opacity-64`/`cursor-not-allowed`, neither of which the contract can
    /// see).
    pub disabled: bool,
    /// `--invalid`: `aria-invalid`. See the module docs for why it is here
    /// rather than on the `error` flag — `checkbox`'s precedent, applied.
    pub invalid: bool,
}

impl Params {
    /// The group this cell describes: one radio, checked by `--flags
    /// selected`.
    #[must_use]
    pub fn radio_group(self, cell: &Cell) -> RadioGroup {
        RadioGroup {
            radio: Radio {
                breakpoint: cell.breakpoint(),
                checked: cell.has(StateFlag::Selected),
                disabled: self.disabled,
                validity: Validity {
                    invalid: self.invalid,
                    focused: cell.has(StateFlag::Focus),
                },
            },
        }
    }
}

impl SurfaceParams for Params {
    fn accept(
        &mut self,
        option: &str,
        _args: &mut dyn Iterator<Item = String>,
    ) -> Result<bool, ParseError> {
        match option {
            "--disabled" => self.disabled = true,
            "--invalid" => self.invalid = true,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** A circle's height is its `size-*`, and no option here sets
    /// one — `checkbox`'s exact answer.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let group = self.radio_group(cell);
        let theme = cell.theme();

        let _ = write!(
            out,
            " · {}",
            if group.radio.checked {
                "checked"
            } else {
                "unchecked"
            },
        );

        if group.radio.checked && Radio::selected_moves_a_recorded_field(&theme) {
            out.push_str(
                " · selected is real in this theme: dark:not-data-checked:bg-input/32 \
                 against bg-background moves the circle's bg — ONE compared field",
            );
        } else if group.radio.checked {
            out.push_str(
                " · selected CANNOT FAIL in the light theme: below dark:not-data-checked:\
                 bg-input/32 the circle is bg-background in both states",
            );
        }

        out.push_str(
            " · the fill and its inner dot are painted and NOT measured: \
             data-unchecked:hidden is display:none, which the two extractors disagree \
             about — ANCHORS.md v1.11 — the same call checkbox.rs makes for its own tick",
        );

        if self.disabled {
            out.push_str(" · disabled: opacity-64 and cursor-not-allowed, neither compared");
        }
        if self.invalid {
            out.push_str(
                " · invalid: aria-invalid moves the circle's border colour, which is \
                 compared, but radio-group.tsx has zero live call sites, so there is no \
                 reference either way",
            );
        }

        out.push_str(" · a radio paints no text — --content cannot fail");
        out.push_str(" · --width cannot fail: size-4.5/sm:size-4 authors both axes");
        out.push_str(
            " · NO LIVE REFERENCE: radio-group.tsx's only importer is merge-popover.tsx, \
             which needs a child branch with an unprotected local parent — this item's dev \
             environment has none. See native/mapping/radio-group.md.",
        );
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.radio_group(cell).render(theme, anchors)
    }
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--disabled".to_owned(),
            "the disabled prop: opacity-64 and cursor-not-allowed, neither of which the \
             contract can see [off]"
                .to_owned(),
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
    use crowbar_ui::components::Breakpoint;
    use crowbar_ui::components::radio_group::{ID_GROUP, ID_RADIO};
    use gpui::px;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "radio-group"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a radio-group cell carries this surface's bag")
    }

    /// The defaults are a resting, unchecked circle at `sm:`.
    #[test]
    fn the_defaults_are_resting_and_unchecked() {
        let group = params_of(&cell(&[])).radio_group(&cell(&[]));
        assert!(!group.radio.checked);
        assert!(!group.radio.disabled);
        assert!(!group.radio.validity.invalid);
        assert_eq!(group.radio.extent(), px(16.0));
    }

    /// `selected` reaches the component and moves the background only in
    /// dark — `checkbox`'s shape, confirmed here.
    #[test]
    fn selected_moves_the_background_in_dark_only() {
        let off = params_of(&cell(&[])).radio_group(&cell(&[])).radio;
        let on = params_of(&cell(&["--flags", "selected"]))
            .radio_group(&cell(&["--flags", "selected"]))
            .radio;

        assert!(!SURFACE.unmodelled(StateFlag::Selected));
        assert_ne!(off.background(&Theme::DARK), on.background(&Theme::DARK));
        assert_eq!(off.background(&Theme::LIGHT), on.background(&Theme::LIGHT));
    }

    /// The two props reach the component.
    #[test]
    fn the_two_props_reach_the_component() {
        let disabled = params_of(&cell(&["--disabled"]))
            .radio_group(&cell(&["--disabled"]))
            .radio;
        assert!(disabled.disabled);

        let invalid = params_of(&cell(&["--invalid"]))
            .radio_group(&cell(&["--invalid"]))
            .radio;
        assert!(invalid.validity.invalid);
        assert_ne!(
            invalid.border_color(&Theme::DARK),
            crowbar_ui::components::radio_group::Radio::fixture().border_color(&Theme::DARK),
        );

        // `--invalid` is §8.3's `error` under another name.
        assert!(SURFACE.unmodelled(StateFlag::Error));
    }

    /// The caption always says there is no live reference.
    #[test]
    fn the_caption_always_says_there_is_no_live_reference() {
        for args in [vec![], vec!["--flags", "selected"], vec!["--invalid"]] {
            let described = cell(&args).describe();
            assert!(described.contains("NO LIVE REFERENCE"), "{described}");
        }
    }

    /// The viewport axis reaches the component.
    #[test]
    fn the_viewport_axis_reaches_the_component() {
        let sm = params_of(&cell(&[])).radio_group(&cell(&[])).radio;
        let wide_args = ["--viewport-width", "500"];
        let base = params_of(&cell(&wide_args))
            .radio_group(&cell(&wide_args))
            .radio;
        assert_eq!(sm.breakpoint, Breakpoint::Sm);
        assert_eq!(base.breakpoint, Breakpoint::Base);
        assert_ne!(sm.extent(), base.extent());
    }

    /// Every option the parser accepts appears in the usage.
    #[test]
    fn the_usage_names_every_option_this_surface_takes() {
        let usage = crate::row_surface::usage();
        assert!(usage.contains("radio-group"));
        for option in ["--disabled", "--invalid"] {
            assert!(usage.contains(option), "{option} is missing from the usage");
        }
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    /// This surface's options belong to it and to no other.
    #[test]
    fn this_surfaces_options_are_rejected_on_another_surface() {
        for option in ["--disabled", "--invalid"] {
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

    /// The registry entry's contract fields and the state axis this surface
    /// has.
    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "radio-group");
        assert_eq!(SURFACE.root, "radio-group");
        assert_eq!(SURFACE.root, ID_GROUP);
        assert_ne!(ID_GROUP, ID_RADIO);
        assert!(!SURFACE.full_bleed);

        assert!(!SURFACE.unmodelled(StateFlag::Selected));
        for flag in [
            StateFlag::Hover,
            StateFlag::Empty,
            StateFlag::Loading,
            StateFlag::Error,
        ] {
            assert!(SURFACE.unmodelled(flag), "{flag:?}");
        }
    }
}
