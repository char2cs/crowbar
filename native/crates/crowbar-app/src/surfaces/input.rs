//! `--surface input` — the first surface in the port that paints **editable**
//! text, and the first whose text no snapshot can see.
//!
//! One `<Input>` in one cell of the §8.3 matrix. The default cell is the live
//! app's tree filter —
//! `web/src/features/file-explorer/file-explorer/components/file-explorer-tree.tsx`,
//! `nativeInput size="sm" placeholder="Search" className="ps-5"`, empty and at
//! rest — measured at `246×28` in the sidebar header at `innerWidth` 1714.
//!
//! # What the anchor set can and cannot see about a text field
//!
//! The whole item, in one table. Two anchors, and between them the contract sees
//! the boxes and nothing that makes a text field a text field.
//!
//! | | seen? |
//! |---|---|
//! | the control's box, background, 10px radius, **1px border and its colour** | **yes**, all compared |
//! | the field's box, its `rounded-[inherit]` radius, its zero border | **yes** |
//! | the **value** | **no** |
//! | the **placeholder** | **no** |
//! | the **caret** | **no** |
//! | the **selection highlight** | **no** |
//!
//! The value and the placeholder are not a §6 omission — they are a property of
//! the *extractor*. `extract.ts` builds the whole text group from
//! `oracleOwnText(el)`, which reads `el.childNodes` for text nodes, and an
//! `<input>` is a **void element** with none. Measured live:
//! `childNodes.length === 0`, and a `Range` over its contents returns **zero
//! client rects**, so `text_width` would be `0` even if the branch were entered.
//! With `Search` on screen, `scrollWidth === clientWidth === 224`, so the
//! fallback clip signal is dead too.
//!
//! The caret and the selection have a different reason and it is worth keeping
//! separate: they have no *element*. A caret is a rule the user agent draws
//! inside the field, a selection is `::selection` under a run of glyphs, and
//! §3's pseudo-backed shortcut reaches neither — it is defined for
//! `position:absolute; inset:0` pseudos read through `getComputedStyle`. **No
//! field is invented for them**, the way `resizable`'s hit strip and `button`'s
//! `::before` overlay are recorded rather than modelled.
//!
//! # What each axis can and cannot do here
//!
//! | Axis | Here |
//! |---|---|
//! | `--width` | **the strongest axis.** Both anchors are `w-full`, so every horizontal number on the surface is a function of it |
//! | `--viewport-width` | real: all three sizes carry an `sm:` variant that takes one `--spacing` step off the field's height, and it moves the type scale as well |
//! | `--theme` | real, and on the *background*: `bg-background` in light against `dark:bg-input/32` in dark are two different tokens, not one token with two values |
//! | `--content` | **vacuous on every cell.** Nothing this surface paints is comparable — see above. The strings exist so the native side paints what the reference paints |
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `focus` | **real, and it moves a compared field** — `has-focus-visible:border-ring` is a border *colour*. It is also **unreachable**: measured on the live app, `document.hasFocus()` is `false` and stays false through `window.focus()` and Tauri's own `setFocus()`, so `:focus` never matches and `:focus-visible` cannot either. A programmatic `.focus()` does set `document.activeElement` — and that is all it sets |
//! | `empty` | **real, and this is the one surface where it is the interesting flag**: an empty field shows its placeholder and a filled one shows its value. It is also the **resting** cell, because the live reference is empty — so `--flags empty` moves nothing unless `--value` was given, and [`Params::describe`] says so per cell |
//! | `hover` | **unmodelled**, and counted rather than assumed: `input.tsx` contains the substring `hover` **zero** times. The original has no hover state at all |
//! | `selected` | **unmodelled.** `input.tsx` writes no `selected`, `:checked` or `data-selected` rule. A *text* selection is a document state the user agent paints, with no element to anchor and no field to compare — recorded in `native/mapping/input.md`, not modelled here |
//! | `loading` | unmodelled, as on every surface |
//! | `error` | unmodelled — **and that is forced rather than true.** See below |
//!
//! ## `error` is a real state on this component, and it is still unmodelled
//!
//! `aria-invalid` is exactly §8.3's `error`: `has-aria-invalid:border-destructive/36`
//! moves the control's `border.color`, which the differ compares. So the honest
//! declaration would be that `error` is modelled here.
//!
//! It cannot be. `surface.rs`'s `no_surface_declares_its_entire_state_axis_unmodelled`
//! asserts `unmodelled(Loading)` **and** `unmodelled(Error)` for *every*
//! registered surface, and that assertion is not this item's to weaken. So the
//! state is driven by `--invalid`, this surface's own option, exactly as
//! `button`'s `loading` branch is driven by `--loading`.
//!
//! It costs nothing today: **no `<Input` in `web/src/` passes `aria-invalid`**,
//! so the cell has no reference either way. Recorded because the next worker to
//! port `select`, `checkbox`, `radio-group` or `textarea` will meet the same
//! four rules and the same invariant.
//!
//! # `--class-ps`, and why a call site's class is a legitimate parameter
//!
//! `input.tsx` puts a call site's `className` on the **control**, so the call
//! site is half the component — `button`'s finding, in a second place. Here the
//! merged class that reaches a compared field is `ps-5`: it moves the field's
//! `bounds.x` from 1 to 21 and its `bounds.w` from 244 to 224 inside a 246px
//! control.
//!
//! The line is P3.1's and is unchanged: a knob that hands the port the
//! reference's **output** is forbidden, a knob that supplies the same **input**
//! both engines then resolve is correct. `ps-5` is a class; each side still
//! resolves `calc(var(--spacing) * 5)` through its own scale. So the option
//! spells the **class** — `--class-ps none|ps-5` — and a pixel value is a
//! rejection whose message says why.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::primitives::input::{
    ALL_LEADING_PADS, ALL_SIZES, Input, LeadingPad, Size, State, Text,
};
use crowbar_ui::AnchorSink;
use crowbar_ui::surfaces::rows::ContentLength;
use crowbar_ui::primitives::input;
use gpui::AnyElement;

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "input",
    root: input::ID_CONTROL,
    unmodelled: &[
        // `input.tsx` has no `hover:` rule at all — counted, not assumed.
        StateFlag::Hover,
        // No `selected`/`:checked`/`data-selected` rule either. A *text*
        // selection is the user agent's paint, with no element and no field.
        StateFlag::Selected,
        StateFlag::Loading,
        // Real (`aria-invalid`), and rendered by `--invalid` rather than by this
        // flag — see the module docs. `surface.rs`'s workspace invariant
        // requires the declaration.
        StateFlag::Error,
    ],
    // The tallest field is `lg` below the breakpoint at 38px, plus its two
    // border pixels and `CAPTION_HEIGHT`'s 29. 80 holds that with room, and is
    // the same floor `button` takes for the same reason: this surface drives no
    // height, because a field's is decided by its size variant rather than by
    // anything on the command line the window could follow.
    min_window_height: 80,
    // A field inside a sidebar header, a dialog or a toolbar — never the window
    // itself. So it keeps `INSET_X`, and the root-relative arithmetic in the
    // snapshot stays non-trivial on both axes.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// The two props §8.3's own vocabulary has no usable word for.
///
/// A struct rather than two fields on [`Params`] for the reason
/// [`crowbar_ui::primitives::input::State`] is one: they are one kind of thing —
/// conditions the control's `has-*` chain reads — and keeping them together is
/// what makes it obvious that `--invalid` is the `error` flag under another name.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Driven {
    /// `--disabled`: the `disabled` prop. Live and invisible.
    pub disabled: bool,
    /// `--invalid`: `aria-invalid`. See the module docs for why it is here
    /// rather than on the `error` flag.
    pub invalid: bool,
}

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--size`.
    pub size: Size,
    /// `--class-ps`: the `ps-*` a **call site** merges onto the control.
    pub leading_pad: LeadingPad,
    /// `--icon`: render the `leftIcon` **prop**'s box and its gutter.
    ///
    /// Off by default, and that is the live picture: the magnifier beside the
    /// tree filter is a `<Search>` the *call site* renders as a sibling of the
    /// control, outside the root anchor entirely.
    pub icon: bool,
    /// `--value`: put a value in the field instead of showing the placeholder.
    ///
    /// Which of the three strings it shows is the shared `--content` axis.
    pub value: bool,
    /// `--disabled` and `--invalid`.
    pub driven: Driven,
}

impl Default for Params {
    fn default() -> Self {
        let fixture = Input::fixture();
        Self {
            size: fixture.size,
            leading_pad: fixture.leading_pad,
            icon: fixture.icon,
            value: fixture.value.is_some(),
            driven: Driven::default(),
        }
    }
}

impl Params {
    /// Whether this cell's field holds a value.
    ///
    /// `--flags empty` is the authority and wins over `--value`, which is what
    /// keeps the flag from being decoration: `--value --flags empty` empties a
    /// field that was asked to hold something. Without the flag the answer is
    /// `--value`'s, and the resting cell has none — the live reference is empty.
    #[must_use]
    pub fn filled(&self, cell: &Cell) -> bool {
        self.value && !cell.has(StateFlag::Empty)
    }

    /// The input this cell describes.
    ///
    /// Built by taking the live fixture and applying the cell — rather than by
    /// assembling one from scratch — so that a bare `--surface input` renders the
    /// field the reference actually has.
    #[must_use]
    pub fn input(&self, cell: &Cell) -> Input {
        let text = text_of(cell.content);
        let mut input = Input::fixture();
        input.size = self.size;
        // The `sm:` variants are a **viewport** media query, so they follow the
        // viewport rather than `--width` — the same quantity `git-status-row`
        // reads, and conflating the two is how a reference captured below 640px
        // came to be compared against the `sm:` arm.
        input.breakpoint = cell.breakpoint();
        input.leading_pad = self.leading_pad;
        input.icon = self.icon;
        input.placeholder = text.placeholder().into();
        input.value = self.filled(cell).then(|| text.value().into());
        input.state = State {
            focused: cell.has(StateFlag::Focus),
            disabled: self.driven.disabled,
            invalid: self.driven.invalid,
        };
        input
    }
}

/// The strings a content length carries.
///
/// A translation rather than a shared type: `ContentLength` is the *cell's*
/// vocabulary and [`Text`] is the component's, and the two carry different
/// strings on purpose — a field's placeholder is a prompt where the git row's
/// fixture is a path.
fn text_of(content: ContentLength) -> Text {
    match content {
        ContentLength::Short => Text::Short,
        ContentLength::Normal => Text::Normal,
        ContentLength::Overflow => Text::Overflow,
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
            "--class-ps" => self.leading_pad = parse_leading_pad(&value(args, option)?)?,
            "--icon" => self.icon = true,
            "--value" => self.value = true,
            "--disabled" => self.driven.disabled = true,
            "--invalid" => self.driven.invalid = true,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** A field's height is its size variant's, and no option here sets
    /// one — so there is nothing for the window to follow and
    /// [`Surface::min_window_height`] is the whole answer. The same call
    /// `button` and `git-status-row` make.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    /// This surface's own half of the caption — and, where it applies, the fact
    /// that a cell **cannot fail** or **has no reference**.
    ///
    /// The two are different complaints and are worded differently, as
    /// `button`'s are. "Cannot fail" means the contract has no field for what
    /// moved; "no reference" means the picture is drawable here and not on the
    /// other side.
    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(out, " · {}", self.size.name());
        let _ = write!(
            out,
            " · {}",
            if self.filled(cell) {
                "value"
            } else {
                "placeholder"
            },
        );
        match self.leading_pad {
            LeadingPad::Ps5 => out.push_str(
                " · ps-5 (the tree filter's, file-explorer-tree.tsx; it clears the \
                 magnifier the call site renders outside the control)",
            ),
            LeadingPad::None => out.push_str(
                " · no call-site ps-*: the primitive's own control, which nineteen of \
                 the twenty call sites get, and which the git review reply box \
                 (review-thread-item.tsx) renders visibly",
            ),
        }
        if self.icon {
            out.push_str(" · leftIcon");
        }
        if self.driven.disabled {
            out.push_str(
                " · disabled: opacity-64 has no field and does not reach v1.7's zero, \
                 and shadow-none is ANCHORS.md §6, so this cell cannot fail",
            );
        }
        if self.driven.invalid {
            out.push_str(
                " · invalid: aria-invalid does move the control's border colour, which \
                 is compared, but no live <Input passes it, so there is no reference",
            );
        }

        if !self.size.live() {
            out.push_str(" · size: no live call site asks for it, so there is no reference");
        }

        // The content axis is vacuous on every cell of this surface, so it is
        // said unconditionally rather than only where a string was driven.
        out.push_str(
            " · an <input> has no text node, so the reference emits no text/fg/font for \
             either anchor and --content cannot fail",
        );

        if cell.has(StateFlag::Empty) && !self.value {
            out.push_str(
                " · empty: the field is already empty without --value, so this cell is \
                 the resting one and cannot fail",
            );
        }
        if cell.has(StateFlag::Focus) {
            out.push_str(
                " · focus: has-focus-visible:border-ring really does move a compared \
                 field, but document.hasFocus() is false on this machine and neither \
                 window.focus() nor Tauri's setFocus() changes it, so :focus-visible is \
                 unreachable and there is no reference",
            );
        }
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.input(cell).render(theme, anchors)
    }
}

fn parse_size(raw: &str) -> Result<Size, ParseError> {
    ALL_SIZES
        .into_iter()
        .find(|size| size.name() == raw)
        .ok_or_else(|| {
            // A numeric `size` is a real prop and is *not* a fourth size — it is
            // the `<input size>` attribute, a character count that `w-full`
            // overrides, on a control that renders the `default` class arm. The
            // rejection says so rather than saying "invalid", because a reader
            // who tried it was reading `input.tsx` and deserves the answer.
            ParseError::Rejected(format!(
                "--size takes one of {}, not {raw}. A numeric size is the HTML \
                 <input size> attribute — a character count that w-full overrides — \
                 and renders the default arm, so it is not a fourth visual size",
                ALL_SIZES.map(Size::name).join(", "),
            ))
        })
}

fn parse_leading_pad(raw: &str) -> Result<LeadingPad, ParseError> {
    ALL_LEADING_PADS
        .into_iter()
        .find(|pad| pad.name() == raw)
        .ok_or_else(|| {
            ParseError::Rejected(format!(
                "--class-ps names the class a call site merges onto the control — one \
                 of {} — and never a pixel value. A numeric knob would let a cell be \
                 tuned to whatever the reference happened to report; a class is an \
                 input both engines resolve through their own --spacing. Got {raw}",
                ALL_LEADING_PADS.map(LeadingPad::name).join(", "),
            ))
        })
}

fn options() -> Vec<(String, String)> {
    let fixture = Params::default();
    [
        (
            "--size <name>".to_owned(),
            format!(
                "sm|default|lg; lg has no live call site [{}]",
                fixture.size.name(),
            ),
        ),
        (
            "--class-ps <class>".to_owned(),
            format!(
                "none|ps-5; the class a call site merges onto the control, never a \
                 pixel value [{}]",
                fixture.leading_pad.name(),
            ),
        ),
        (
            "--icon".to_owned(),
            "render the leftIcon prop's box and its ps-7/ps-8 gutter [off]".to_owned(),
        ),
        (
            "--value".to_owned(),
            "put a value in the field instead of the placeholder; --flags empty wins \
             over it [off]"
                .to_owned(),
        ),
        (
            "--disabled".to_owned(),
            "the disabled prop: opacity-64 and shadow-none, neither of which the \
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
    use crowbar_ui::primitives::input::{ID_CONTROL, ID_FIELD, Input, LeadingPad, Size, Text};

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "input"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("an input cell carries this surface's bag")
    }

    fn built(cell: &Cell) -> Input {
        params_of(cell).input(cell)
    }

    /// The defaults are the live tree filter, and the bag **rebuilds** it rather
    /// than producing something that merely resembles it.
    #[test]
    fn the_defaults_are_the_live_tree_filter() {
        let bag = Params::default();
        let fixture = Input::fixture();

        assert_eq!(bag.size, fixture.size);
        assert_eq!(bag.leading_pad, fixture.leading_pad);
        assert_eq!(bag.icon, fixture.icon);
        assert!(!bag.value);
        assert!(!bag.driven.disabled);
        assert!(!bag.driven.invalid);

        assert_eq!(built(&cell(&[])), fixture);
    }

    /// **`empty` is the flag that means something here**, and `--value` is what
    /// it has something to say about.
    #[test]
    fn empty_beats_value_and_the_resting_field_is_already_empty() {
        // The resting cell is the live reference: no value, placeholder showing.
        let resting = built(&cell(&[]));
        assert!(resting.is_empty());
        assert_eq!(resting.painted(), Text::Normal.placeholder());

        // `--value` fills it, from the `--content` axis.
        let filled = built(&cell(&["--value"]));
        assert!(!filled.is_empty());
        assert_eq!(filled.painted(), Text::Normal.value());
        assert_eq!(
            built(&cell(&["--value", "--content", "short"])).painted(),
            Text::Short.value(),
        );

        // And the flag wins over the option, which is what stops it being
        // decoration.
        let emptied = built(&cell(&["--value", "--flags", "empty"]));
        assert!(emptied.is_empty());
        assert_eq!(emptied.painted(), Text::Normal.placeholder());

        // The flag alone changes nothing, because the resting cell is already
        // empty — and the caption says so rather than letting a reader bank it.
        assert_eq!(built(&cell(&["--flags", "empty"])), Input::fixture());
        let described = cell(&["--flags", "empty"]).describe();
        assert!(described.contains("already empty"), "{described}");
        assert!(described.contains("cannot fail"), "{described}");
        // With `--value` it *did* move, so the caption must not claim otherwise.
        let moved = cell(&["--value", "--flags", "empty"]).describe();
        assert!(!moved.contains("already empty"), "{moved}");
    }

    /// The placeholder tracks `--content` even with no value, so the axis reaches
    /// the paint on every cell — it is only the *comparison* that cannot fail.
    #[test]
    fn the_content_axis_reaches_the_paint_and_still_cannot_fail() {
        for (word, text) in [
            ("short", Text::Short),
            ("normal", Text::Normal),
            ("overflow", Text::Overflow),
        ] {
            let driven = cell(&["--content", word]);
            assert_eq!(built(&driven).painted(), text.placeholder(), "{word}");

            let described = driven.describe();
            assert!(described.contains("--content cannot fail"), "{described}");
            assert!(described.contains("no text node"), "{described}");
        }

        // Said on every cell, including one that drove a value.
        let valued = cell(&["--value"]).describe();
        assert!(valued.contains("--content cannot fail"), "{valued}");
    }

    /// `--focus` reaches a **compared** field and still has no reference, and the
    /// caption has to say both halves — the two claims a reader must not merge.
    #[test]
    fn focus_moves_a_compared_field_and_still_has_no_reference() {
        let focused = built(&cell(&["--flags", "focus"]));
        assert!(focused.state.focused);
        assert!(!focused.has_shadow());
        assert_ne!(
            focused.border_color(&crowbar_ui::Theme::DARK),
            Input::fixture().border_color(&crowbar_ui::Theme::DARK),
        );

        let described = cell(&["--flags", "focus"]).describe();
        assert!(described.contains("compared field"), "{described}");
        assert!(described.contains("no reference"), "{described}");
        // And it is *not* declared unmodelled: the original has the state.
        assert!(!SURFACE.unmodelled(StateFlag::Focus));
    }

    /// The two props reach the component, and each one says in the caption
    /// whether it cannot fail or merely has no reference.
    #[test]
    fn the_two_props_reach_the_component_and_are_captioned_apart() {
        let disabled = built(&cell(&["--disabled"]));
        assert!(disabled.state.disabled);
        assert!(!disabled.has_shadow());
        let caption = cell(&["--disabled"]).describe();
        assert!(caption.contains("cannot fail"), "{caption}");
        assert!(!caption.contains("disabled: aria-invalid"), "{caption}");

        let invalid = built(&cell(&["--invalid"]));
        assert!(invalid.state.invalid);
        let caption = cell(&["--invalid"]).describe();
        assert!(caption.contains("no reference"), "{caption}");
        assert!(caption.contains("aria-invalid"), "{caption}");

        // `--invalid` is §8.3's `error` under another name, and the flag itself
        // is declared unmodelled because the workspace invariant requires it.
        assert!(SURFACE.unmodelled(StateFlag::Error));
        assert_eq!(built(&cell(&["--flags", "error"])), Input::fixture());
    }

    /// The call site's class is a parameter, it reaches the component, and the
    /// caption names the call site so a reader can check the claim.
    #[test]
    fn the_call_sites_leading_pad_is_a_parameter() {
        assert_eq!(built(&cell(&[])).leading_pad, LeadingPad::Ps5);
        assert_eq!(
            built(&cell(&["--class-ps", "none"])).leading_pad,
            LeadingPad::None,
        );

        let default_caption = cell(&[]).describe();
        assert!(default_caption.contains("ps-5"), "{default_caption}");
        assert!(
            default_caption.contains("file-explorer-tree.tsx"),
            "{default_caption}",
        );

        let bare = cell(&["--class-ps", "none"]).describe();
        assert!(bare.contains("primitive's own control"), "{bare}");
    }

    /// The three sizes reach the component, and the dead one says so.
    #[test]
    fn the_sizes_reach_the_component_and_the_dead_one_says_so() {
        for (word, size) in [
            ("sm", Size::Sm),
            ("default", Size::Default),
            ("lg", Size::Lg),
        ] {
            assert_eq!(built(&cell(&["--size", word])).size, size, "{word}");
        }

        let dead = cell(&["--size", "lg"]).describe();
        assert!(dead.contains("no reference"), "{dead}");
        for live in ["sm", "default"] {
            let described = cell(&["--size", live]).describe();
            assert!(!described.contains("size: no live"), "{described}");
        }
    }

    /// `--icon` renders the **prop**, which the live reference does not pass.
    #[test]
    fn the_left_icon_prop_is_off_by_default() {
        assert!(!built(&cell(&[])).icon);
        assert!(built(&cell(&["--icon"])).icon);

        let described = cell(&["--icon"]).describe();
        assert!(described.contains("leftIcon"), "{described}");
    }

    /// The vocabulary is closed, and **a pixel value for `--class-ps` is a
    /// rejection that says why** — the P3.1 line, restated where it applies.
    #[test]
    fn the_option_vocabulary_is_closed() {
        for line in [
            vec!["--size", "medium"],
            vec!["--size", "8"],
            vec!["--size"],
            vec!["--class-ps", "20"],
            vec!["--class-ps", "5"],
            vec!["--class-ps", "ps-7"],
            vec!["--class-ps"],
        ] {
            let mut full = vec!["--surface", "input"];
            full.extend_from_slice(&line);
            assert!(
                matches!(
                    Cell::parse(full.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{line:?} should have been rejected",
            );
        }

        // Both rejections name the rule they are enforcing, because "invalid" is
        // not something anyone can act on.
        let Err(ParseError::Rejected(pixels)) = Cell::parse(
            ["--surface", "input", "--class-ps", "20"]
                .iter()
                .map(|arg| (*arg).to_owned()),
        ) else {
            panic!("--class-ps takes a class");
        };
        assert!(pixels.contains("never a pixel value"), "{pixels}");

        let Err(ParseError::Rejected(numeric)) = Cell::parse(
            ["--surface", "input", "--size", "8"]
                .iter()
                .map(|arg| (*arg).to_owned()),
        ) else {
            panic!("--size takes a name");
        };
        assert!(numeric.contains("character count"), "{numeric}");
    }

    /// **These options belong to this surface and to no other.**
    ///
    /// `--size` is the sharp one: `button` spells it too, and the shared parser
    /// matches its own words *before* delegating — so this checks that
    /// `--class-ps`, `--icon`, `--value` and `--invalid` are not swallowed
    /// somewhere else.
    #[test]
    fn this_surfaces_options_are_rejected_on_another_surface() {
        for line in [
            vec!["--surface", "dropdown-menu", "--class-ps", "ps-5"],
            vec!["--surface", "dropdown-menu", "--icon"],
            vec!["--surface", "dropdown-menu", "--value"],
            vec!["--surface", "dropdown-menu", "--invalid"],
        ] {
            assert!(
                matches!(
                    Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{line:?} should have been rejected",
            );
        }

        // And `--size lg` on `button` is *button's* vocabulary, not this one's —
        // both accept the word and they mean different pictures.
        assert!(
            Cell::parse(
                ["--surface", "button", "--size", "icon-sm"]
                    .iter()
                    .map(|arg| (*arg).to_owned())
            )
            .is_ok()
        );
        assert!(
            Cell::parse(
                ["--surface", "input", "--size", "icon-sm"]
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

        for option in [
            "--size",
            "--class-ps",
            "--icon",
            "--value",
            "--disabled",
            "--invalid",
        ] {
            assert!(usage.contains(option), "{option} is missing from the usage");
        }
        assert!(usage.contains("input"));
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    /// The registry entry's contract fields, which a snapshot carries verbatim,
    /// and the state axis this surface really has.
    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "input");
        assert_eq!(SURFACE.root, "input-control");
        assert_eq!(SURFACE.root, ID_CONTROL);
        assert!(!SURFACE.full_bleed);
        assert_ne!(SURFACE.root, ID_FIELD);

        // Two real flags, and both of them are states `input.tsx` genuinely has.
        assert!(!SURFACE.unmodelled(StateFlag::Focus));
        assert!(!SURFACE.unmodelled(StateFlag::Empty));
        for flag in [
            StateFlag::Hover,
            StateFlag::Selected,
            StateFlag::Loading,
            StateFlag::Error,
        ] {
            assert!(SURFACE.unmodelled(flag), "{flag:?}");
        }
    }
}
