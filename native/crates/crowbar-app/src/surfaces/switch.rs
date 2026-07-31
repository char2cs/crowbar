//! `--surface switch` — **the first surface in the port where `--flags selected`
//! is unambiguously real**, on two anchors and two independent fields.
//!
//! One `<Switch>` in one cell of the §8.3 matrix. The default cell is the live
//! Git settings tab —
//! `web/src/features/settings/components/tabs/git-settings.tsx`,
//! `<Switch checked={…} size="sm" />` — measured at `30 × 18` at `innerWidth`
//! 1714.
//!
//! # The state axis, and why this surface exists
//!
//! | flag | here |
//! |---|---|
//! | `selected` | **real, on two anchors.** `data-checked:bg-primary` against `data-unchecked:bg-input` moves `switch.bg`, and `data-checked:translate-x-[…]` moves `switch-thumb.bounds.x` from 1 to 13. Both are compared fields, and both live references were captured from switches that were *simultaneously* on screen in the Git tab — one on, one off — so neither state had to be driven to produce the other |
//! | `focus` | real in the class list (`focus-visible:ring-2`) and **invisible**: a ring is a box-shadow, `ANCHORS.md` §6. Also unreachable — `document.hasFocus()` is false and immovable on this machine |
//! | `hover` | **unmodelled**, counted rather than assumed: `switch.tsx` contains the substring `hover` **zero** times |
//! | `empty` | unmodelled. A switch has no content to be empty of |
//! | `loading` | unmodelled, as on every surface |
//! | `error` | unmodelled, and genuinely so — unlike `input` and `checkbox`, `switch.tsx` has no `aria-invalid` rule at all, so this is not the forced declaration those two make |
//!
//! # What each axis can and cannot do here
//!
//! | Axis | Here |
//! |---|---|
//! | `--flags selected` | **the strongest axis on the surface** — see above |
//! | `--viewport-width` | real: `[--thumb-size:--spacing(5)] sm:[--thumb-size:--spacing(4)]` moves every length on the component, so the two breakpoints are a `38 × 22` switch and a `30 × 18` one |
//! | `--theme` | real, but **not uniformly**: `--primary` is the *same colour in both tables*, so the theme moves the **off** track (`bg-input`) and the thumb (`bg-background`) and does **not** move the on track. `switch.tsx` has no `dark:` variant, so this is the token table and nothing else |
//! | `--width` | **vacuous.** Both axes of the track are authored from `--thumb-size`; nothing on this surface is a function of its container's width |
//! | `--content` | **vacuous.** A switch paints no text at all, so neither side emits `text`, `fg`, `text_width`, `clipped` or `font` for either anchor |
//!
//! # The `size` prop is inert, and the census says so
//!
//! **All 19 live `<Switch` call sites pass `size="sm"`**, and it changes nothing:
//! `switch.tsx` spends the prop on `data-size={size}` and there is no `data-size`
//! in the class list. A sweep of every loaded stylesheet for a selector mentioning
//! `data-size` returned **0** rules. So there is no `--size` option here — adding
//! one would advertise a picture the component cannot make.
//!
//! **No live call site passes a `className` either**, which makes this the first
//! Tier B surface with no call-site merge to model at all: `button`, `input`,
//! `badge`, `avatar` and the P3.6/P3.8 leaves each needed a `CallSite` or a
//! `--class-*`, and this one does not. The primitive's class list is the whole
//! component.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::switch::Switch;
use crowbar_ui::components::{AnchorSink, switch};
use gpui::AnyElement;

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "switch",
    root: switch::ID_SWITCH,
    unmodelled: &[
        // `switch.tsx` has no `hover:` rule at all — counted, not assumed.
        StateFlag::Hover,
        // A switch has no content, so there is nothing to be empty of.
        StateFlag::Empty,
        StateFlag::Loading,
        // Genuinely unmodelled rather than forced: unlike `input` and
        // `checkbox`, this component has no `aria-invalid` rule.
        StateFlag::Error,
    ],
    // The tallest track is the **base** breakpoint's 22px, plus `CAPTION_HEIGHT`'s
    // 29. 72 holds that with room, and is a floor rather than a ceiling: this
    // surface drives no height, because a track's is decided by `--thumb-size`
    // rather than by anything on the command line the window could follow.
    min_window_height: 72,
    // A switch sits in a settings row — never the window itself. So it keeps
    // `INSET_X`, and the root-relative arithmetic stays non-trivial on both axes.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
///
/// Two booleans rather than a `Driven` struct: unlike `input`'s pair, these are
/// not one kind of thing — `--disabled` is a prop and `--focus` is §8.3's own
/// flag, which this surface models rather than renames. Only `--disabled` is
/// here; focus arrives through `--flags focus`.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Params {
    /// `--disabled`: the `disabled` prop. Live — three call sites pass it — and
    /// invisible: `opacity-64` has no field and does not reach v1.7's zero.
    pub disabled: bool,
}

impl Params {
    /// The switch this cell describes.
    ///
    /// Built by taking the live fixture and applying the cell — rather than by
    /// assembling one from scratch — so that a bare `--surface switch` renders
    /// the switch the reference actually has.
    #[must_use]
    pub fn switch(self, cell: &Cell) -> Switch {
        let mut switch = Switch::fixture();
        // The `sm:` variants are a **viewport** media query, so they follow the
        // viewport rather than `--width` — the same quantity `git-status-row`
        // reads, and conflating the two is how a reference captured below 640px
        // came to be compared against the `sm:` arm.
        switch.breakpoint = cell.breakpoint();
        switch.checked = cell.has(StateFlag::Selected);
        switch.focused = cell.has(StateFlag::Focus);
        switch.disabled = self.disabled;
        switch
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
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** A track's height is `--thumb-size + 2px`, and no option here sets
    /// one — so there is nothing for the window to follow and
    /// [`Surface::min_window_height`] is the whole answer.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    /// This surface's own half of the caption — including, on every cell, which
    /// fields `selected` moves and which axes cannot fail.
    fn describe(&self, cell: &Cell, out: &mut String) {
        let switch = self.switch(cell);

        let _ = write!(
            out,
            " · {}",
            if switch.checked {
                "checked"
            } else {
                "unchecked"
            },
        );

        if switch.checked {
            let _ = write!(
                out,
                " · selected is REAL here: bg-primary over bg-input on the track, and the \
                 thumb slides {}px (translate-x-[calc(--thumb-size-4px)]) — two compared \
                 fields on two anchors",
                f32::from(switch.thumb_offset()),
            );
            out.push_str(
                " · the on track is bg-primary, which is the SAME colour in both token \
                 tables, so --theme moves the thumb on this cell and not the track",
            );
        } else {
            out.push_str(
                " · the resting cell, and the one the live reference was captured from; \
                 --flags selected is what moves it",
            );
        }

        if self.disabled {
            out.push_str(
                " · disabled: opacity-64 has no field and does not reach v1.7's zero, so \
                 this cell cannot fail",
            );
        }
        if cell.has(StateFlag::Focus) {
            out.push_str(
                " · focus: focus-visible:ring-2 is a box-shadow, which ANCHORS.md §6 has no \
                 field for, so this cell cannot fail — and document.hasFocus() is false on \
                 this machine, so there is no reference either",
            );
        }

        // Said unconditionally, because they are vacuous on every cell of this
        // surface rather than only where something was driven.
        out.push_str(
            " · a switch paints no text, so the reference emits no text/fg/font for either \
             anchor and --content cannot fail",
        );
        out.push_str(
            " · --width cannot fail: both track axes are authored from --thumb-size, not \
             from the container",
        );
        out.push_str(
            " · size: all 19 live call sites pass size=\"sm\" and it is inert — it emits \
             data-size and no rule in any stylesheet selects on it (measured, 0 hits)",
        );
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.switch(cell).render(theme, anchors)
    }
}

fn options() -> Vec<(String, String)> {
    [(
        "--disabled".to_owned(),
        "the disabled prop: opacity-64, which the contract cannot see [off]".to_owned(),
    )]
    .into_iter()
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{Params, SURFACE, options};
    use crate::row_surface::{Cell, ParseError, StateFlag};
    use crowbar_ui::Theme;
    use crowbar_ui::components::Breakpoint;
    use crowbar_ui::components::switch::{ID_SWITCH, ID_THUMB, Switch};
    use gpui::px;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "switch"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a switch cell carries this surface's bag")
    }

    fn built(cell: &Cell) -> Switch {
        params_of(cell).switch(cell)
    }

    /// The defaults are the live Git-settings switch, and the bag **rebuilds** it
    /// rather than producing something that merely resembles it.
    #[test]
    fn the_defaults_are_the_live_git_settings_switch() {
        let bag = Params::default();
        assert!(!bag.disabled);
        assert_eq!(built(&cell(&[])), Switch::fixture());
    }

    /// **`--flags selected` moves two compared fields on two anchors**, which is
    /// the claim this whole surface exists to make.
    #[test]
    fn selected_moves_the_track_colour_and_the_thumbs_position() {
        let off = built(&cell(&[]));
        let on = built(&cell(&["--flags", "selected"]));

        assert!(!off.checked);
        assert!(on.checked);

        // 1. the track's background.
        assert_ne!(
            off.track_background(&Theme::DARK),
            on.track_background(&Theme::DARK),
        );
        // 2. the thumb's offset — the live 1 → 13, which is 0 → 12 root-relative
        //    plus the track's one padding pixel.
        assert_eq!(off.thumb_offset(), px(0.0));
        assert_eq!(on.thumb_offset(), px(12.0));

        // And the flag is *not* declared unmodelled — the point of the surface.
        assert!(!SURFACE.unmodelled(StateFlag::Selected));
    }

    /// The caption says which fields moved, on the cell where they moved — and
    /// says the resting cell is the reference's on the cell where they did not.
    #[test]
    fn the_caption_names_the_two_fields_selected_moves() {
        let on = cell(&["--flags", "selected"]).describe();
        assert!(on.contains("selected is REAL"), "{on}");
        assert!(on.contains("bg-primary"), "{on}");
        assert!(on.contains("thumb slides 12px"), "{on}");
        assert!(on.contains("two anchors"), "{on}");

        let off = cell(&[]).describe();
        assert!(off.contains("resting cell"), "{off}");
        assert!(!off.contains("selected is REAL"), "{off}");
    }

    /// **Every cell's caption says that `--content` and `--width` cannot fail**,
    /// because they are vacuous on all of them — the claim a 0-delta run would
    /// otherwise be read as evidence for.
    #[test]
    fn the_two_vacuous_axes_are_declared_on_every_cell() {
        for args in [
            vec![],
            vec!["--flags", "selected"],
            vec!["--content", "overflow"],
            vec!["--width", "400"],
            vec!["--disabled"],
        ] {
            let described = cell(&args).describe();
            assert!(described.contains("--content cannot fail"), "{described}");
            assert!(described.contains("--width cannot fail"), "{described}");
            // And the inert prop is named on every cell too, since every live
            // call site passes it.
            assert!(described.contains("it is inert"), "{described}");
        }
    }

    /// The viewport axis reaches the component and is a genuinely different
    /// picture — the control that stops `--viewport-width` being decoration.
    #[test]
    fn the_viewport_axis_reaches_the_component() {
        let sm = built(&cell(&[]));
        let base = built(&cell(&["--viewport-width", "500"]));

        assert_eq!(sm.breakpoint, Breakpoint::Sm);
        assert_eq!(base.breakpoint, Breakpoint::Base);
        assert_eq!(sm.track_width(), px(30.0));
        assert_eq!(base.track_width(), px(38.0));
        assert_ne!(sm.thumb_extent(), base.thumb_extent());
    }

    /// The two invisible states reach the component and each says in the caption
    /// that it cannot fail.
    #[test]
    fn the_invisible_states_reach_the_component_and_say_they_cannot_fail() {
        let disabled = built(&cell(&["--disabled"]));
        assert!(disabled.disabled);
        let caption = cell(&["--disabled"]).describe();
        assert!(caption.contains("disabled:"), "{caption}");
        assert!(caption.contains("cannot fail"), "{caption}");

        let focused = built(&cell(&["--flags", "focus"]));
        assert!(focused.focused);
        let caption = cell(&["--flags", "focus"]).describe();
        assert!(caption.contains("box-shadow"), "{caption}");
        assert!(caption.contains("no reference"), "{caption}");
        // Focus is a state the original genuinely has, so it is not unmodelled.
        assert!(!SURFACE.unmodelled(StateFlag::Focus));
    }

    /// **There is no `--size`**, because the prop is inert — and no `--class-*`,
    /// because no live call site merges one. Both are rejections rather than
    /// silently-ignored words.
    #[test]
    fn the_inert_prop_and_the_absent_call_site_are_not_options() {
        for line in [
            vec!["--size", "sm"],
            vec!["--size", "default"],
            vec!["--class-radius", "sm"],
            vec!["--call-site", "none"],
        ] {
            let mut full = vec!["--surface", "switch"];
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

    /// This surface's option belongs to it, and `--disabled` on another surface
    /// is that surface's own — the check that the shared parser is not swallowing
    /// it somewhere else.
    #[test]
    fn the_disabled_option_is_this_surfaces_own() {
        assert!(built(&cell(&["--disabled"])).disabled);
        // `input` spells it too, and means its own picture by it.
        assert!(
            Cell::parse(
                ["--surface", "input", "--disabled"]
                    .iter()
                    .map(|arg| (*arg).to_owned())
            )
            .is_ok()
        );
        // A surface that does not spell it rejects it.
        assert!(
            Cell::parse(
                ["--surface", "skeleton", "--disabled"]
                    .iter()
                    .map(|arg| (*arg).to_owned())
            )
            .is_err()
        );
    }

    /// Every option the parser accepts appears in the usage.
    #[test]
    fn the_usage_names_every_option_this_surface_takes() {
        let usage = crate::row_surface::usage();
        assert!(usage.contains("switch"));
        assert!(usage.contains("--disabled"));
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    /// The registry entry's contract fields, which a snapshot carries verbatim,
    /// and the state axis this surface really has.
    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "switch");
        assert_eq!(SURFACE.root, "switch");
        assert_eq!(SURFACE.root, ID_SWITCH);
        assert!(!SURFACE.full_bleed);
        assert_ne!(SURFACE.root, ID_THUMB);

        // The two real flags.
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
