//! `--surface checkbox` — `selected` is real here too, on **one** field and in
//! **one** of the two themes.
//!
//! One `<Checkbox>` in one cell of the §8.3 matrix. The default cell is the live
//! commit popover's file list —
//! `web/src/features/git/components/commit-popover.tsx` — measured at `16 × 16`
//! at `innerWidth` 1714.
//!
//! # `selected` moves exactly one recorded field, and only in dark
//!
//! | field | off | on |
//! |---|---|---|
//! | `checkbox.bg` | `#ffffff07` (`dark:bg-input/32`) | `#1f1f1eff` (`bg-background`) |
//!
//! The rule that carries it is `dark:not-data-checked:bg-input/32`, a `dark:`
//! variant. **Below it the box is `bg-background` in both states**, so a
//! `--flags selected` cell driven at `--theme light` moves nothing the contract
//! records and **cannot fail**. That is said on the cell itself by
//! [`Params::describe`], because a 0-delta light run would otherwise read as
//! coverage it is not. The live app is dark, which is the only reason the
//! captured pair differs.
//!
//! # The indicator is unanchored, and that is the interesting call
//!
//! `data-unchecked:hidden` is `display: none`, and the two extractors do
//! different things with it: `extract.ts` emits the anchor (base-ui keeps the
//! element mounted) with an all-zero `getBoundingClientRect()`, while
//! `ANCHORS.md` §6 says the GPUI side simply has no anchor at all. Anchoring the
//! fill would therefore put a structural delta on this surface's **default** cell,
//! caused by the contract rather than by the port.
//!
//! The full reasoning, the two rejected repairs and the live measurement are in
//! `crowbar_ui::primitives::checkbox`'s module docs and in
//! `native/mapping/checkbox.md`. The consequence for this surface is the one
//! sentence worth repeating: **the green fill and the tick are painted and not
//! measured**, so `selected` has one field here where `switch` has two.
//!
//! # What each axis can and cannot do here
//!
//! | Axis | Here |
//! |---|---|
//! | `--flags selected` | **real in dark, vacuous in light** — see above |
//! | `--theme` | real, and it is what decides whether `selected` is real: the box's background is one token in light and a `/32` mix of another in dark-unchecked |
//! | `--viewport-width` | real: `size-4.5 sm:size-4` is an 18px box below the breakpoint and a 16px one above it |
//! | `--width` | **vacuous.** `size-*` authors both axes; nothing here is a function of the container |
//! | `--content` | **vacuous.** A checkbox paints no text — the tick is an `<svg>`, which has element children rather than text nodes, so no `text`/`fg`/`font` is emitted on either side |
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `selected` | **real** — the whole item, with the theme caveat above |
//! | `focus` | real in the class list and **invisible**: `focus-visible:ring-2` is a box-shadow. Unlike `input`, focus alone does *not* move this component's `border.color` — there is no bare `focus-visible:border-*` rule. Combined with `--invalid` it does |
//! | `hover` | **unmodelled**, counted rather than assumed: `checkbox.tsx` contains the substring `hover` **zero** times |
//! | `empty` | unmodelled. A checkbox has no content to be empty of |
//! | `loading` | unmodelled, as on every surface |
//! | `error` | unmodelled — **and that is forced rather than true**. See below |
//!
//! ## `error` is a real state here, and it is still unmodelled
//!
//! `aria-invalid` is exactly §8.3's `error`: `aria-invalid:border-destructive/36`
//! moves the box's `border.color`, which the differ compares. So the honest
//! declaration would be that `error` is modelled.
//!
//! It cannot be. `surface.rs`'s `no_surface_declares_its_entire_state_axis_unmodelled`
//! asserts `unmodelled(Loading)` **and** `unmodelled(Error)` for *every*
//! registered surface, and that assertion is not this item's to weaken. So the
//! state is driven by `--invalid`, exactly as `input`'s is — which is the surface
//! that predicted this: "`select`, `checkbox`, `radio-group` and `textarea` carry
//! the same four rules and will hit this again." They do, and it did.
//!
//! It costs nothing today: **no `<Checkbox` in `web/src/` passes `aria-invalid`**,
//! so the cell has no reference either way.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::primitives::checkbox::{ALL_CHECKED, Checkbox, Checked};
use crowbar_ui::AnchorSink;
use crowbar_ui::primitives::checkbox;
use gpui::AnyElement;

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "checkbox",
    root: checkbox::ID_CHECKBOX,
    unmodelled: &[
        // `checkbox.tsx` has no `hover:` rule at all — counted, not assumed.
        StateFlag::Hover,
        // A checkbox has no content, so there is nothing to be empty of.
        StateFlag::Empty,
        StateFlag::Loading,
        // Real (`aria-invalid`), and rendered by `--invalid` rather than by this
        // flag — see the module docs. `surface.rs`'s workspace invariant requires
        // the declaration. `input`'s precedent, second occurrence.
        StateFlag::Error,
    ],
    // The tallest box is the **base** breakpoint's 18px, plus `CAPTION_HEIGHT`'s
    // 29. 72 holds that with room, and is a floor rather than a ceiling: this
    // surface drives no height.
    min_window_height: 72,
    // A checkbox sits in a popover row — never the window itself.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Params {
    /// `--indeterminate`: base-ui's third value, which `--flags selected` has no
    /// word for.
    ///
    /// **No reference** — all four live call sites pass a plain boolean
    /// `checked`. It is here because it is a real branch of the original that
    /// changes the glyph and its colour, and leaving it out would make the port
    /// quietly smaller than the component.
    pub indeterminate: bool,
    /// `--disabled`: the `disabled` prop. Live — the commit popover passes it
    /// while committing — and invisible.
    pub disabled: bool,
    /// `--invalid`: `aria-invalid`. See the module docs for why it is here rather
    /// than on the `error` flag.
    pub invalid: bool,
}

impl Params {
    /// Which of the three checked states this cell is in.
    ///
    /// `--indeterminate` wins over `--flags selected`, and that is what keeps it
    /// from being decoration: the two describe the same slot, and base-ui's
    /// `indeterminate` genuinely overrides `checked` in the original's rendering.
    #[must_use]
    pub fn checked(self, cell: &Cell) -> Checked {
        if self.indeterminate {
            Checked::Indeterminate
        } else if cell.has(StateFlag::Selected) {
            Checked::On
        } else {
            Checked::Off
        }
    }

    /// The checkbox this cell describes.
    #[must_use]
    pub fn checkbox(self, cell: &Cell) -> Checkbox {
        let mut checkbox = Checkbox::fixture();
        // A **viewport** media query, so it follows the viewport rather than
        // `--width`.
        checkbox.breakpoint = cell.breakpoint();
        checkbox.checked = self.checked(cell);
        checkbox.focused = cell.has(StateFlag::Focus);
        checkbox.disabled = self.disabled;
        checkbox.invalid = self.invalid;
        checkbox
    }
}

impl SurfaceParams for Params {
    fn accept(
        &mut self,
        option: &str,
        args: &mut dyn Iterator<Item = String>,
    ) -> Result<bool, ParseError> {
        match option {
            "--indeterminate" => self.indeterminate = true,
            "--disabled" => self.disabled = true,
            "--invalid" => self.invalid = true,
            "--checked" => return Err(rejected_checked(&value(args, option)?)),
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** A box's height is its `size-*`, and no option here sets one.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    /// This surface's own half of the caption — including, on every cell, whether
    /// `selected` can fail in *this* theme.
    fn describe(&self, cell: &Cell, out: &mut String) {
        let checked = self.checked(cell);
        let theme = cell.theme();

        let _ = write!(out, " · {}", checked.name());

        if checked.shows_indicator() {
            if Checkbox::selected_moves_a_recorded_field(&theme) {
                out.push_str(
                    " · selected is real in this theme: dark:not-data-checked:bg-input/32 \
                     against bg-background moves the box's bg — ONE compared field",
                );
            } else {
                out.push_str(
                    " · selected CANNOT FAIL in the light theme: the rule that carries it \
                     is dark:not-data-checked:bg-input/32, so below the dark variant the \
                     box is bg-background in both states and every recorded field is \
                     identical",
                );
            }
        } else {
            out.push_str(" · the resting cell, and the one the live reference was captured from");
        }

        // Said on every cell, because it is the surface's defining omission and a
        // reader must not have to find it in the mapping.
        out.push_str(
            " · the green fill and the tick are painted and NOT measured: \
             data-unchecked:hidden is display:none, which extract.ts emits as a zero-box \
             anchor and the driver omits entirely, so anchoring the indicator would put a \
             structural delta on the resting cell",
        );

        if self.indeterminate {
            out.push_str(
                " · indeterminate: it takes the same bg-primary fill as checked and differs \
                 only in the glyph's colour, which is on the unanchored element — so this \
                 cell cannot fail, and no live call site passes it either",
            );
        }
        if self.disabled {
            out.push_str(
                " · disabled: opacity-64 and shadow-none, neither of which the contract can \
                 see, so this cell cannot fail",
            );
        }
        if self.invalid {
            out.push_str(
                " · invalid: aria-invalid does move the box's border colour, which is \
                 compared, but no live <Checkbox passes it, so there is no reference",
            );
        }
        if cell.has(StateFlag::Focus) {
            out.push_str(
                " · focus: the ring is a box-shadow (ANCHORS.md §6) and there is no bare \
                 focus-visible:border-* rule on this component, so focus alone moves \
                 nothing — and document.hasFocus() is false on this machine",
            );
        }

        out.push_str(
            " · a checkbox paints no text — the tick is an <svg>, which has element children \
             rather than text nodes — so --content cannot fail",
        );
        out.push_str(
            " · --width cannot fail: size-4.5/sm:size-4 authors both axes, not the container",
        );
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.checkbox(cell).render(theme, anchors)
    }
}

/// `--checked` is a rejection, and the message says which axis owns the state.
///
/// A worker reading `checkbox.tsx` sees a `checked` prop and reaches for it. The
/// state is §8.3's `selected`, which is a *shared* axis, and giving this surface
/// a private spelling of it would let one cell be driven two ways and compared as
/// though it were one.
fn rejected_checked(raw: &str) -> ParseError {
    ParseError::Rejected(format!(
        "--checked is not an option on this surface: the checked state is §8.3's \
         `selected` flag, so use --flags selected. A private spelling of a shared axis \
         would let the same cell be driven two ways. Got {raw}. \
         (--indeterminate is separate, because the matrix has no word for it.)",
    ))
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--indeterminate".to_owned(),
            format!(
                "base-ui's third checked state, which §8.3 has no word for; it takes the \
                 checked fill and no live call site passes it — one of {} [off]",
                ALL_CHECKED
                    .into_iter()
                    .map(Checked::name)
                    .collect::<Vec<_>>()
                    .join(", "),
            ),
        ),
        (
            "--disabled".to_owned(),
            "the disabled prop: opacity-64 and shadow-none, neither of which the contract \
             can see [off]"
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
    use crowbar_ui::surfaces::rows::Breakpoint;
    use crowbar_ui::primitives::checkbox::{Checkbox, Checked, ID_CHECKBOX};
    use gpui::px;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "checkbox"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a checkbox cell carries this surface's bag")
    }

    fn built(cell: &Cell) -> Checkbox {
        params_of(cell).checkbox(cell)
    }

    /// The defaults are the live commit-popover checkbox.
    #[test]
    fn the_defaults_are_the_live_commit_popover_checkbox() {
        let bag = Params::default();
        assert!(!bag.indeterminate);
        assert!(!bag.disabled);
        assert!(!bag.invalid);
        assert_eq!(built(&cell(&[])), Checkbox::fixture());
        assert_eq!(built(&cell(&[])).extent(), px(16.0));
    }

    /// **`selected` reaches the component, and moves a recorded field only in
    /// dark.**
    #[test]
    fn selected_moves_the_background_in_dark_only() {
        let off = built(&cell(&[]));
        let on = built(&cell(&["--flags", "selected"]));

        assert_eq!(off.checked, Checked::Off);
        assert_eq!(on.checked, Checked::On);
        assert!(!SURFACE.unmodelled(StateFlag::Selected));

        assert_ne!(off.background(&Theme::DARK), on.background(&Theme::DARK));
        assert_eq!(off.background(&Theme::LIGHT), on.background(&Theme::LIGHT));
    }

    /// **The caption tells the truth per theme** — which is the half a green
    /// light-theme run would otherwise be read as coverage for.
    #[test]
    fn the_caption_says_the_light_selected_cell_cannot_fail() {
        let dark = cell(&["--flags", "selected", "--theme", "dark"]).describe();
        assert!(dark.contains("selected is real in this theme"), "{dark}");
        assert!(dark.contains("ONE compared field"), "{dark}");

        let light = cell(&["--flags", "selected", "--theme", "light"]).describe();
        assert!(light.contains("CANNOT FAIL in the light theme"), "{light}");
        assert!(!light.contains("selected is real in this theme"), "{light}");
    }

    /// **Every cell says the indicator is unanchored**, because it is the
    /// surface's defining omission.
    #[test]
    fn every_cell_declares_the_unanchored_indicator() {
        for args in [
            vec![],
            vec!["--flags", "selected"],
            vec!["--indeterminate"],
            vec!["--theme", "light"],
            vec!["--width", "400"],
        ] {
            let described = cell(&args).describe();
            assert!(described.contains("NOT measured"), "{described}");
            assert!(described.contains("display:none"), "{described}");
            assert!(described.contains("--content cannot fail"), "{described}");
            assert!(described.contains("--width cannot fail"), "{described}");
        }
    }

    /// `--indeterminate` wins over `--flags selected`, and says it cannot fail.
    #[test]
    fn indeterminate_beats_selected_and_cannot_fail() {
        let mixed = built(&cell(&["--indeterminate"]));
        assert_eq!(mixed.checked, Checked::Indeterminate);
        // It overrides the flag rather than sitting beside it.
        assert_eq!(
            built(&cell(&["--indeterminate", "--flags", "selected"])).checked,
            Checked::Indeterminate,
        );

        let described = cell(&["--indeterminate"]).describe();
        assert!(described.contains("cannot fail"), "{described}");
        assert!(described.contains("no live call site"), "{described}");
    }

    /// The two props reach the component and are captioned apart — one cannot
    /// fail, the other merely has no reference.
    #[test]
    fn the_two_props_reach_the_component_and_are_captioned_apart() {
        let disabled = built(&cell(&["--disabled"]));
        assert!(disabled.disabled);
        assert!(!disabled.has_shadow());
        let caption = cell(&["--disabled"]).describe();
        assert!(caption.contains("cannot fail"), "{caption}");

        let invalid = built(&cell(&["--invalid"]));
        assert!(invalid.invalid);
        assert_ne!(
            invalid.border_color(&Theme::DARK),
            Checkbox::fixture().border_color(&Theme::DARK),
        );
        let caption = cell(&["--invalid"]).describe();
        assert!(caption.contains("no reference"), "{caption}");
        assert!(caption.contains("aria-invalid"), "{caption}");

        // `--invalid` is §8.3's `error` under another name.
        assert!(SURFACE.unmodelled(StateFlag::Error));
        assert_eq!(built(&cell(&["--flags", "error"])), Checkbox::fixture());
    }

    /// **`--checked` is rejected, and the rejection names the axis that owns the
    /// state** — because "invalid" is not something anyone can act on.
    #[test]
    fn a_private_spelling_of_a_shared_axis_is_rejected() {
        let Err(ParseError::Rejected(complaint)) = Cell::parse(
            ["--surface", "checkbox", "--checked", "true"]
                .iter()
                .map(|arg| (*arg).to_owned()),
        ) else {
            panic!("--checked must be rejected");
        };
        assert!(complaint.contains("--flags selected"), "{complaint}");
        assert!(complaint.contains("shared axis"), "{complaint}");
        // And `--indeterminate` is *not* swept up in the same refusal.
        assert!(
            Cell::parse(
                ["--surface", "checkbox", "--indeterminate"]
                    .iter()
                    .map(|arg| (*arg).to_owned())
            )
            .is_ok()
        );
    }

    /// The viewport axis reaches both boxes and is a different picture.
    #[test]
    fn the_viewport_axis_reaches_the_component() {
        let sm = built(&cell(&[]));
        let base = built(&cell(&["--viewport-width", "500"]));

        assert_eq!(sm.breakpoint, Breakpoint::Sm);
        assert_eq!(base.breakpoint, Breakpoint::Base);
        assert_eq!(sm.extent(), px(16.0));
        assert_eq!(base.extent(), px(18.0));
        assert_ne!(sm.glyph_extent(), base.glyph_extent());
    }

    /// Every option the parser accepts appears in the usage.
    #[test]
    fn the_usage_names_every_option_this_surface_takes() {
        let usage = crate::row_surface::usage();
        assert!(usage.contains("checkbox"));
        for option in ["--indeterminate", "--disabled", "--invalid"] {
            assert!(usage.contains(option), "{option} is missing from the usage");
        }
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    /// This surface's options belong to it and to no other.
    #[test]
    fn this_surfaces_options_are_rejected_on_another_surface() {
        for line in [
            vec!["--surface", "dropdown-menu", "--indeterminate"],
            vec!["--surface", "skeleton", "--invalid"],
            vec!["--surface", "switch", "--indeterminate"],
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

    /// The registry entry's contract fields and the state axis this surface has.
    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "checkbox");
        assert_eq!(SURFACE.root, "checkbox");
        assert_eq!(SURFACE.root, ID_CHECKBOX);
        assert!(!SURFACE.full_bleed);

        assert!(!SURFACE.unmodelled(StateFlag::Selected));
        assert!(!SURFACE.unmodelled(StateFlag::Focus));
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
