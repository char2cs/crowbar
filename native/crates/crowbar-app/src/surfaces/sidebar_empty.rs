//! `--surface sidebar-empty` — `sidebar.tsx`'s `SidebarEmptyActionState`, the
//! centred column a sidebar panel shows when it has nothing to list.
//!
//! The **only component in `sidebar.tsx` with no `gpui-component` equivalent**,
//! so it is built from `div()`. That is not a §6.2 exception: the instruction is
//! not to rebuild what the library provides, and it provides no empty state.
//! `crowbar_ui::surfaces::sidebar::shell` carries the values.
//!
//! # How its one reachable cell is reached
//!
//! Both live call sites are the file explorer's. `No folder open` needs a
//! workspace with no folder open at all; **`No matching files` needs only a
//! filter that matches nothing**, which is a string typed into the tree search.
//! That is the cell this surface's defaults describe, and `/tmp/p3-ref-sidebar-empty.json`
//! is it — a 123.94 × 96 box holding one 99.94 × 16 line at (12, 40).
//!
//! # The action `<Button>` is recorded and not rendered
//!
//! `SidebarEmptyActionState` takes `actionLabel` + `onAction` and renders a
//! `<Button variant="ghost" compact className="ui-text-xs h-6 px-2 …">`. It is
//! **not** modelled here, for two reasons that both stand on their own:
//!
//! * `crowbar_ui::primitives::Button::render` reaches `AnchorSink::root`, and
//!   the root anchor **clears the registry** — so a button nested inside this
//!   surface would take this surface's anchors with it. The way round it is to
//!   render the nested primitive through `Unanchored`, which is a real option
//!   and a change to make deliberately rather than in passing.
//! * The call site's `h-6 px-2` is not one of `button`'s ten `Size` arms, so
//!   modelling it means inventing a size that surface does not have — on an
//!   anchor with **no live reference**, since the one call site that renders an
//!   action is the one a parity run cannot reach.
//!
//! Named in `native/mapping/sidebar.md` §6 as the item's one deferred piece,
//! rather than approximated here.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `empty` | **real**, and it is `label`'s reading of the word: the message is the empty string, so the run vanishes and the anchor drops its font group and its `line_sized` declaration with it. `min-h-24` then decides the whole box on its own. |
//! | `error` | **unmodelled**, and that is a ruling rather than an oversight. `SidebarEmptyActionState` really does have an error *appearance* — `tone="error"` paints `text-destructive` — but `StateFlag::Error` is §8.3's "the component is reporting a failure", which this port's own registry invariant holds unmodelled on every surface. The appearance is a **prop**, so it is `--tone`, and `surface.rs`'s `no_surface_declares_its_entire_state_axis_unmodelled` is what stopped the two being conflated. |
//! | `loading`, `hover`, `focus`, `selected` | unmodelled. `grep -o 'hover:[a-z-]*\|focus:[a-z-]*\|active:[a-z-]*'` over the component finds one hit — `hover:text-foreground` — and it is on the **action `<Button>`**, which this surface does not render. The box itself is `select-none` and has no interaction rule at all. |
//!
//! **No tone is live.** Neither call site passes `tone`, so `Neutral` is reached
//! by the prop's default and the other two have no reference. The caption says
//! so per cell.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::AnchorSink;
use crowbar_ui::surfaces::sidebar::shell::{self as sidebar, EmptyActionState, Tone};
use gpui::{AnyElement, IntoElement as _, ParentElement as _, SharedString, Styled as _, div};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "sidebar-empty",
    root: sidebar::ID_EMPTY,
    // Five of the six. `empty` is the one that moves a box; `error` is a
    // §8.3 *state* and this component's error is a **prop**, which is the
    // distinction the module docs spell out.
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // The live box is 96px — `min-h-24` — and the caption sits below it.
    min_window_height: 160,
    // A shrink-to-fit column centred in a panel, not a box that fills the
    // viewport.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// `--message`'s default: the live "No matching files" state, which is the one
/// cell on this surface a parity run can reach.
pub const DEFAULT_MESSAGE: &str = "No matching files";

/// This surface's own options.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Params {
    /// `--message`: the required `message` prop.
    pub message: SharedString,
    /// `--description`: the optional second line.
    ///
    /// `None` by default, and that is not a convenience: no live call site
    /// passes a description, and an anchor the reference cannot produce is a
    /// `FieldPresence` delta that forgives nothing.
    pub description: Option<SharedString>,
    /// `--icon`: renders the `size-7` glyph box above the message.
    ///
    /// Also unreached — neither call site passes an icon.
    pub icon: bool,
    /// `--tone`.
    pub tone: Tone,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            message: SharedString::new_static(DEFAULT_MESSAGE),
            description: None,
            icon: false,
            tone: Tone::Neutral,
        }
    }
}

impl Params {
    /// The empty state this cell describes.
    ///
    /// `empty` blanks the message — and only the message. The icon and the
    /// description are props rather than content, so a cell that asked for both
    /// keeps both: what the flag removes is the run.
    #[must_use]
    pub fn empty_state(&self, cell: &Cell) -> EmptyActionState {
        EmptyActionState {
            message: if cell.has(StateFlag::Empty) {
                SharedString::new_static("")
            } else {
                self.message.clone()
            },
            description: self.description.clone(),
            icon: self.icon,
            tone: self.tone,
        }
    }

    /// The box's own height, for the window to follow.
    #[must_use]
    pub fn box_height(&self, cell: &Cell) -> u16 {
        let height = f32::from(self.empty_state(cell).height(&cell.theme()));
        #[expect(
            clippy::cast_possible_truncation,
            clippy::cast_sign_loss,
            reason = "every term is a small non-negative whole or one-decimal \
                      number of px; `Cell` needs a `u16` to stay `Eq`"
        )]
        {
            height.ceil() as u16
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
            "--message" => self.message = SharedString::from(value(args, option)?),
            "--description" => self.description = Some(SharedString::from(value(args, option)?)),
            "--icon" => self.icon = true,
            "--tone" => {
                let wanted = value(args, option)?;
                self.tone = sidebar::ALL_TONES
                    .into_iter()
                    .find(|tone| tone.name() == wanted)
                    .ok_or_else(|| {
                        ParseError::Rejected(format!(
                            "--tone takes one of {}, not {wanted:?}",
                            sidebar::ALL_TONES.map(Tone::name).join(", "),
                        ))
                    })?;
            }
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **The box's own height**, which `min-h-24` decides at every reachable
    /// cell — so the window follows it rather than capping it.
    fn driven_height(&self, cell: &Cell) -> Option<u16> {
        Some(self.box_height(cell))
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let state = self.empty_state(cell);
        let _ = write!(out, " · tone {}", state.tone.name());
        if cell.has(StateFlag::Empty) {
            out.push_str(
                " · empty: no run, so the anchor drops its font group and its line_sized \
                 declaration with it — and no call site renders a messageless empty state, \
                 so this cell has no reference",
            );
        }
        if state.tone != Tone::Neutral {
            out.push_str(": no call site passes a tone, so this cell has no reference");
        }
        if state.icon {
            out.push_str(" · icon: no call site passes one, so this cell has no reference");
        }
        if state.description.is_some() {
            out.push_str(" · description: no call site passes one, so this cell has no reference");
        }
    }

    /// The element tree, inside the row flex parent the live call site puts it
    /// in.
    ///
    /// `file-explorer-tree.tsx` wraps every `SidebarEmptyActionState` in
    /// `<div className="file-tree-empty-state flex flex-1 items-center
    /// justify-center">`, and **that wrapper is what makes the box
    /// shrink-to-fit**: a row flex item is sized by its content, and
    /// `items-center` is what stops it being stretched to the panel's height.
    /// Reproduced here rather than left out, because without it the box would be
    /// `--width` wide and `CONTENT_SIZED` would be a declaration about a
    /// stretched box. It is not anchored — it is the call site's, not the
    /// surface's (`ANCHORS.md` v1.8).
    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        div()
            .flex()
            .items_center()
            .child(self.empty_state(cell).render(theme, anchors))
            .into_any_element()
    }
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--message <text>".to_owned(),
            format!("the required `message` prop [{DEFAULT_MESSAGE:?}]"),
        ),
        (
            "--description <text>".to_owned(),
            "the optional second line; no live call site passes one".to_owned(),
        ),
        (
            "--icon".to_owned(),
            "render the size-7 glyph box; no live call site passes one".to_owned(),
        ),
        (
            "--tone <name>".to_owned(),
            format!(
                "one of {}; no live call site passes one [neutral]",
                sidebar::ALL_TONES.map(Tone::name).join(", "),
            ),
        ),
    ]
    .into_iter()
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{DEFAULT_MESSAGE, Params, SURFACE, options};
    use crate::row_surface::{Cell, ParseError, StateFlag};
    use crowbar_ui::surfaces::sidebar::shell::Tone;

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "sidebar-empty"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a sidebar-empty cell carries this surface's bag")
    }

    /// The defaults are the live "No matching files" state, measured off the
    /// running app: a 96px box, which is `min-h-24` rather than its 64px column.
    #[test]
    fn the_defaults_are_the_live_no_matching_files_state() {
        let bag = Params::default();
        let default_cell = cell(&[]);

        assert_eq!(bag.message, DEFAULT_MESSAGE);
        assert_eq!(bag.description, None);
        assert!(!bag.icon);
        assert_eq!(bag.tone, Tone::Neutral);
        assert_eq!(bag.box_height(&default_cell), 96);
    }

    /// **`empty` is the flag this surface models**, and it removes the run and
    /// nothing else: the icon and the description are props rather than
    /// content, so a cell that asked for them keeps them.
    #[test]
    fn empty_removes_the_run_and_leaves_the_props() {
        let empty = cell(&["--flags", "empty"]);
        assert!(params_of(&empty).empty_state(&empty).message.is_empty());
        assert!(!SURFACE.unmodelled(StateFlag::Empty));

        // The box is `min-h-24` on its own once the 16.2px line is gone.
        assert_eq!(params_of(&empty).box_height(&empty), 96);

        let dressed = cell(&["--flags", "empty", "--icon", "--description", "x"]);
        let state = params_of(&dressed).empty_state(&dressed);
        assert!(state.message.is_empty());
        assert!(state.icon);
        assert_eq!(state.description.as_deref(), Some("x"));
    }

    /// **`--tone error` is a prop and `--flags error` is a state**, and this
    /// surface keeps them apart.
    ///
    /// `SidebarEmptyActionState` genuinely has an error *appearance*, which is
    /// the reason the two are easy to conflate — and the registry invariant
    /// `no_surface_declares_its_entire_state_axis_unmodelled` is what caught it
    /// when they were.
    #[test]
    fn the_error_tone_is_a_prop_and_the_error_flag_stays_unmodelled() {
        let toned = cell(&["--tone", "error"]);
        assert_eq!(params_of(&toned).empty_state(&toned).tone, Tone::Error);

        // The flag reaches nothing: driving it renders the resting picture, and
        // the binary says so on stderr rather than drawing a cell that cannot
        // fail.
        let flagged = cell(&["--flags", "error"]);
        assert_eq!(
            params_of(&flagged).empty_state(&flagged).tone,
            Tone::Neutral,
        );
        assert!(SURFACE.unmodelled(StateFlag::Error));
    }

    /// The other five are declared unmodelled.
    #[test]
    fn the_five_remaining_flags_are_declared_unmodelled() {
        for flag in [
            StateFlag::Error,
            StateFlag::Loading,
            StateFlag::Hover,
            StateFlag::Focus,
            StateFlag::Selected,
        ] {
            assert!(SURFACE.unmodelled(flag), "{flag:?}");
            assert!(
                cell(&["--flags", flag.name()])
                    .unmodelled_flags()
                    .contains(&flag),
                "{flag:?} should be reported on stderr",
            );
        }
    }

    /// The icon and the description each add their own row **and** a gap, and
    /// only once past 96 does the box grow at all — which is the whole point of
    /// `min-h-24` and the reason the reference is 96 and not 64.
    #[test]
    fn the_optional_rows_are_absorbed_by_the_minimum_until_they_are_not() {
        let plain = cell(&[]);
        let icon = cell(&["--icon"]);
        let described = cell(&["--description", "Try a different search."]);
        let both = cell(&["--icon", "--description", "Try a different search."]);

        // 48 + 16.2 = 64.2, under the 96 minimum.
        assert_eq!(params_of(&plain).box_height(&plain), 96);
        // …plus a 14.85 line and a 6 gap is 85.05, still under it.
        assert_eq!(params_of(&described).box_height(&described), 96);
        // …plus 28 + 2 + 6 for the icon is 100.2, which is over.
        assert_eq!(params_of(&icon).box_height(&icon), 101);
        // …and both together is 121.05, which ceils to 122.
        assert_eq!(params_of(&both).box_height(&both), 122);
    }

    /// The vocabulary is closed and every rejection names what was wanted.
    #[test]
    fn the_option_vocabulary_is_closed() {
        for line in [
            vec!["--tone", "warning"],
            vec!["--tone"],
            vec!["--message"],
            vec!["--description"],
            vec!["--body-height", "28"],
            vec!["--action-label", "Open Folder"],
        ] {
            let mut full = vec!["--surface", "sidebar-empty"];
            full.extend_from_slice(&line);
            assert!(
                matches!(
                    Cell::parse(full.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{line:?} should have been rejected",
            );
        }

        // And the rejection lists the vocabulary rather than only refusing.
        let Err(ParseError::Rejected(complaint)) =
            Cell::parse(["--surface", "sidebar-empty", "--tone", "warning"].map(str::to_owned))
        else {
            panic!("`warning` is not a tone");
        };
        for tone in ["neutral", "error", "success"] {
            assert!(complaint.contains(tone), "{complaint}");
        }
    }

    /// **These options belong to this surface and to no other**, which is the
    /// property the registry exists for.
    #[test]
    fn this_surfaces_options_are_rejected_on_another_surface() {
        for option in ["--message", "--description", "--tone"] {
            let line = ["--surface", "git-status-row", option, "x"];
            assert!(
                matches!(
                    Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{option} should not be a git-status-row option",
            );
        }
        let icon = ["--surface", "git-status-row", "--icon"];
        assert!(matches!(
            Cell::parse(icon.iter().map(|arg| (*arg).to_owned())),
            Err(ParseError::Rejected(_)),
        ));
    }

    /// Every option the parser accepts appears in the usage.
    #[test]
    fn the_usage_names_every_option_this_surface_takes() {
        let usage = crate::row_surface::usage();

        for option in ["--message", "--description", "--icon", "--tone"] {
            assert!(usage.contains(option), "{option} is missing from the usage");
        }
        assert!(usage.contains("sidebar-empty"));
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    /// The caption says which cells have **no reference to compare against**,
    /// which is per-cell where `Surface::unmodelled` is per-surface.
    #[test]
    fn the_caption_names_every_cell_that_has_no_reference() {
        assert!(
            cell(&["--tone", "error"])
                .describe()
                .contains("no reference")
        );
        assert!(cell(&["--icon"]).describe().contains("no reference"));
        assert!(
            cell(&["--description", "x"])
                .describe()
                .contains("no reference")
        );
        assert!(
            cell(&["--flags", "empty"])
                .describe()
                .contains("no reference")
        );
        // And says nothing of the sort at the reachable default.
        assert!(!cell(&[]).describe().contains("no reference"));
        assert!(cell(&[]).describe().contains("tone neutral"));
    }

    /// The registry entry's two contract fields, which a snapshot carries
    /// verbatim.
    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "sidebar-empty");
        assert_eq!(SURFACE.root, "sidebar-empty");
    }
}
