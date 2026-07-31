//! `--surface button` — the first Phase 3 surface, and the most-used primitive
//! in the app.
//!
//! One `<Button>` in one cell of the §8.3 matrix. The default cell is the live
//! app's own cleanest instance: `variant="ghost" size="icon-sm"`, one glyph, no
//! label, at rest — the element at `x 1140, y 153` in the fixture workspace,
//! which is the only one of the nine live Buttons whose class list is exactly
//! `buttonVariants(…)` with nothing merged over it.
//!
//! # The host is a flex row, and that is load-bearing
//!
//! `RowSurface` draws every surface inside `div().w(--width)`, which is a gpui
//! **block** container. A button drawn straight into one would be a block-level
//! flex box and would fill the width — where every live Button is a *flex item*,
//! and `shrink-0` plus an auto basis makes its used width its own max-content
//! width. So [`Params::render`] wraps the button in a flex row.
//!
//! It is also what makes the two `display`s agree without gpui having an
//! `inline-flex`: CSS blockifies a flex item's display, so the reference's
//! `inline-flex` computes to **`flex`** — measured on the live app, not assumed.
//!
//! The row is **above** the root anchor and therefore outside the snapshot
//! (`ANCHORS.md` §4), exactly as `resizable`'s shell box is.
//!
//! # What each axis can and cannot do here
//!
//! | Axis | Here |
//! |---|---|
//! | `--viewport-width` | **the strongest axis this surface has.** All ten sizes carry an `sm:` variant, and it takes exactly one `--spacing` step off the box; two of them change the type scale as well |
//! | `--width` | **weak.** Nine sizes author their own box. It matters only for `--label`, where the button is shrink-to-fit, and then only as room to be laid out in |
//! | `--theme` | real on six variants and **vacuous on `default`** — `theme.css` declares `--primary` and `--primary-foreground` to the same value in `:root` and in `.dark`, so the arm 53 of 142 call sites use cannot move |
//! | `--content` | vacuous **without `--label`**, which is off by default because all nine live Buttons are icon-only |
//!
//! # The state axis, and which flag reaches what
//!
//! | flag | here |
//! |---|---|
//! | `hover` | **real and visible** — `hover:bg-*` moves `bg` on six of seven variants, which is the first time in this port an interaction flag moves a compared field. It has **no reference**: `CGPreflightPostEventAccess()` is false on this project's machines, so no capture can be taken with a pointer over a button |
//! | `focus` | a real state with **no field**. `focus-visible:ring-2 ring-offset-1` is two box-shadows and `focus-visible:outline-hidden` is an outline; `ANCHORS.md` §6 has neither |
//! | `selected` | mapped onto the **`active` prop** (`bg-accent/20`), which is real and which **no live call site passes** — the same shape as `git-status-row`'s `selected`, whose `active` prop no live consumer passes either |
//! | `empty` | **unmodelled.** A Button with neither label nor glyph is not a state the app has: every one of the 142 call sites passes at least one child |
//! | `loading`, `error` | unmodelled, as on every surface. See the note below, because `loading` is *not* the usual case |
//!
//! ## Why `loading` is unmodelled here, spelled out
//!
//! `button.tsx` **does** have a loading branch — `data-loading:text-transparent`
//! plus a `<Spinner>` — and this port renders it. It is driven by `--loading`,
//! this surface's own option, and not by `--flags loading`.
//!
//! That is `resizable`'s `--with-handle` arrangement, and it is here for the same
//! reason: `withHandle` and `loading` are **props of the primitive that no live
//! call site passes**. Counted, not assumed — of the 142 `<Button` elements in
//! `web/src/`, zero pass `loading`. So the §8.3 `loading` *cell* is one the
//! reference cannot produce, `--flags loading` reaches nothing on this surface,
//! and [`Surface::unmodelled`]'s claim — that driving it cannot fail — is true.
//! `--loading` renders the branch, and [`Params::describe`] says per cell that it
//! has no reference.
//!
//! # `--class-radius`, and why a call site's class is a legitimate parameter
//!
//! `button.tsx` composes `cn(buttonVariants({ className, size, variant }), …)`,
//! so **the call site is half the component**: tailwind-merge resolves a call
//! site's `className` against the variant's, and all nine live Buttons use it.
//! `rounded-*` is the one merged class that reaches a *compared* field, so the
//! surface has to be able to say which one the cell has.
//!
//! The line this sits on is worth stating, because it is close to something that
//! should be refused:
//!
//! * **forbidden** — a knob that hands the port the reference's *output*. P3.2
//!   rightly refused that for `tab-indicator`, whose box **is** the answer;
//!   passing it in makes the anchor unable to fail.
//! * **correct** — a knob that supplies the same *input* both engines then
//!   resolve independently. `rounded-sm` is a class; each side still computes a
//!   radius from `--radius` through its own scale.
//!
//! So the option names the **class** (`sm|md|lg`) and never a number. There is
//! deliberately no `--class-radius 6`: that would let a cell be tuned to
//! whatever the reference happened to report.
//!
//! `none` is the **primitive's own** radius, and it is reachable — but it has no
//! visible reference, and the caption says so per cell. See
//! [`crowbar_ui::components::button::Button::fixture`] for the three groups the
//! nine live Buttons fall into.
//!
//! # Three options that are not matrix axes
//!
//! `--pressed`, `--disabled` and `--loading` are all props rather than §8.3
//! quantities, and each one is named in the caption only where it is honoured.
//! `--disabled` is the interesting one: it is the *most* live of the three (35 of
//! 142 call sites pass it) and the *least* visible — `pointer-events-none` is not
//! a visual property, `opacity-64` has no field and does not reach v1.7's zero,
//! and `shadow-none` is §6.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::button::{
    ALL_RADIUS_CLASSES, ALL_SIZES, ALL_VARIANTS, Button, ButtonState, Interaction, Label, Props,
    RadiusClass, Size, Variant,
};
use crowbar_ui::components::{AnchorSink, ContentLength, button};
use gpui::{AnyElement, IntoElement as _, ParentElement as _, Styled as _, div};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "button",
    root: button::ID_BUTTON,
    unmodelled: &[
        // Every live call site passes a child.
        StateFlag::Empty,
        // Rendered, but by `--loading` rather than by this flag — see the
        // module docs. No live call site passes the prop.
        StateFlag::Loading,
        StateFlag::Error,
    ],
    // The tallest button is `xl`/`icon-xl` below the breakpoint at 44px, plus
    // `CAPTION_HEIGHT`'s 29. 80 holds that with room and is a floor rather than
    // a ceiling: this surface drives no height (`driven_height`), because a
    // button's is decided by its own size variant and not by anything on the
    // command line that the window would have to follow.
    min_window_height: 80,
    // A button inside a toolbar, a dialog or a row — never the window itself.
    // So it keeps `INSET_X`, and the root-relative arithmetic in the snapshot
    // stays non-trivial on both axes.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// The three visual states §8.3's own vocabulary has no word for, and which
/// this surface therefore drives with options of its own.
///
/// A struct rather than three fields on [`Params`] for the reason
/// [`crowbar_ui::components::button::Props`] is one: five booleans in one bag
/// read as five independent switches, and these three are one kind of thing —
/// props and pseudo-classes the matrix cannot name. (`struct_excessive_bools`
/// is what asked the question; the answer is the same either way.)
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Driven {
    /// `--pressed`: `data-pressed`, which base-ui writes while the pointer is
    /// down, and the `:active` every rule on this component pairs it with.
    pub pressed: bool,
    /// `--disabled`: the `disabled` prop. Live and invisible — see the module
    /// docs.
    pub disabled: bool,
    /// `--loading`: the `loading` prop. See the module docs for why it is here
    /// rather than on the `loading` flag.
    pub loading: bool,
}

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--variant`.
    pub variant: Variant,
    /// `--size`.
    pub size: Size,
    /// `--label`: render the button's text child.
    ///
    /// Off by default, because every live Button is icon-only. Which of the
    /// three strings it shows is the shared `--content` axis.
    pub label: bool,
    /// `--no-glyph`: drop the leading glyph.
    ///
    /// **Not `--no-icon`.** That word is already the shared parser's, for
    /// `git-status-row`'s `showFileIcon`, and the shared parser matches it
    /// *before* a surface's own bag is asked — so an option spelled that way
    /// here would be silently swallowed and this surface's glyph would never
    /// turn off. The one collision in this surface's vocabulary, found by
    /// reading `Cell::parse` rather than by a failing test, and pinned by
    /// `the_shared_parsers_words_are_not_this_surfaces`.
    pub icon: bool,
    /// `--pressed`, `--disabled` and `--loading`.
    pub driven: Driven,
    /// `--class-radius`: the `rounded-*` a **call site** merges over the size
    /// variant's own, or `None` for the variant's.
    ///
    /// The one option here that is not a prop, and the one that needed the most
    /// justifying — see the module docs. It names the **class**; the pixels come
    /// out of the sealed token the class names, so a cell cannot be tuned to a
    /// number the reference happened to say.
    pub class_radius: Option<RadiusClass>,
}

impl Default for Params {
    fn default() -> Self {
        let fixture = Button::fixture();
        Self {
            variant: fixture.variant,
            size: fixture.size,
            label: fixture.label.is_some(),
            icon: fixture.icon,
            driven: Driven::default(),
            class_radius: fixture.class_radius,
        }
    }
}

impl Params {
    /// The button this cell describes.
    ///
    /// Built by taking the live fixture and applying the cell — rather than by
    /// assembling one from scratch — so that a bare `--surface button` renders
    /// the button the reference actually has.
    #[must_use]
    pub fn button(&self, cell: &Cell) -> Button {
        let mut button = Button::fixture();
        button.variant = self.variant;
        button.size = self.size;
        button.breakpoint = cell.breakpoint();
        button.icon = self.icon;
        button.label = self.label.then(|| label_of(cell.content));
        button.class_radius = self.class_radius;
        button.state = ButtonState {
            interaction: Interaction {
                hovered: cell.has(StateFlag::Hover),
                pressed: self.driven.pressed,
                focused: cell.has(StateFlag::Focus),
            },
            props: Props {
                disabled: self.driven.disabled,
                loading: self.driven.loading,
                // §8.3's `selected` is this component's `active` prop.
                active: cell.has(StateFlag::Selected),
            },
        };
        button
    }
}

/// The label a content length shows.
///
/// A translation rather than a shared type: `ContentLength` is the *cell's*
/// vocabulary and [`Label`] is the component's, and the two carry different
/// strings on purpose — a button label is a verb phrase where the git row's
/// fixture is a path.
fn label_of(content: ContentLength) -> Label {
    match content {
        ContentLength::Short => Label::Short,
        ContentLength::Normal => Label::Normal,
        ContentLength::Overflow => Label::Overflow,
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
            "--size" => self.size = parse_size(&value(args, option)?)?,
            "--label" => self.label = true,
            "--no-glyph" => self.icon = false,
            "--pressed" => self.driven.pressed = true,
            "--disabled" => self.driven.disabled = true,
            "--loading" => self.driven.loading = true,
            "--class-radius" => self.class_radius = parse_class_radius(&value(args, option)?)?,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** A button's height is its size variant's, and no option here
    /// sets one — so there is nothing for the window to follow and
    /// [`Surface::min_window_height`] is the whole answer. The same call
    /// `git-status-row` makes about a row.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    /// This surface's own half of the caption — and, where it applies, the fact
    /// that a cell **cannot fail** or **has no reference**.
    ///
    /// The two are different complaints and are worded differently. "Cannot
    /// fail" means the contract has no field for what moved; "no reference"
    /// means the picture is drawable here and not on the other side. A reader
    /// who conflates them will bank the wrong cells.
    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(out, " · {} · {}", self.variant.name(), self.size.name());
        if !self.icon {
            out.push_str(" · no icon");
        }
        if self.driven.pressed {
            out.push_str(" · pressed");
        }
        if self.driven.disabled {
            out.push_str(" · disabled");
        }
        if self.driven.loading {
            out.push_str(" · loading");
        }

        match self.class_radius {
            Some(class) => {
                let _ = write!(out, " · rounded-{}", class.name());
                // The default cell reproduces a real call site, and says which.
                // A caption that only printed the class would leave a reader
                // unable to check the claim against the app.
                if self.variant == Variant::Ghost
                    && self.size == Size::IconSm
                    && class == RadiusClass::Sm
                {
                    out.push_str(
                        " (the tab bar's sidebar toggle, tab-navigation-buttons.tsx; \
                         four more Buttons write the same class)",
                    );
                }
            }
            // The primitive's own radius. Both live Buttons that keep it are
            // inside the sidebar carousel's snapped-out panels and report
            // `visible: false`, so a capture of one is not comparable at all —
            // the same shape of finding as Phase 1's `git-row-dir`.
            None => out.push_str(
                " · radius: the size variant's own, with no call-site class; the only two \
                 live Buttons that keep it are clipped out by the carousel at \
                 visible:false, so there is no visible reference",
            ),
        }

        if !self.variant.live() {
            out.push_str(" · variant: no live call site uses it, so there is no reference");
        }
        if !self.size.live() {
            out.push_str(" · size: no live call site uses it, so there is no reference");
        }
        if self.label {
            let _ = write!(out, " · label \"{}\"", label_of(cell.content).text());
        } else {
            out.push_str(" · no label, so --content cannot fail");
        }
        if self.variant == Variant::Default {
            out.push_str(
                " · default: --primary and --primary-foreground are the same in both \
                 token tables, so --theme cannot fail",
            );
        }
        if cell.has(StateFlag::Hover) {
            out.push_str(
                " · hover: it does move bg, and synthetic pointer events are denied on \
                 this machine, so there is no reference",
            );
        }
        if cell.has(StateFlag::Focus) {
            out.push_str(
                " · focus: the ring is two box-shadows, which ANCHORS.md §6 has no field \
                 for, so this cell cannot fail",
            );
        }
        if cell.has(StateFlag::Selected) {
            out.push_str(
                " · selected: the active prop is real and no live call site passes it, \
                 so there is no reference",
            );
        }
        if self.driven.disabled && !self.driven.loading {
            out.push_str(
                " · disabled: opacity-64 has no field and does not reach v1.7's zero, so \
                 this cell cannot fail",
            );
        }
        if self.driven.loading {
            out.push_str(" · loading: no live call site passes the prop, so there is no reference");
        }
        if self.driven.pressed {
            out.push_str(
                " · pressed: base-ui writes data-pressed while the pointer is down, which \
                 this machine cannot drive, so there is no reference",
            );
        }
    }

    /// The button, inside the flex row that makes it a flex item.
    ///
    /// See the module docs: the row is what gives a labelled button its
    /// shrink-to-fit width and what makes the reference's `inline-flex` and this
    /// port's `flex` the same computed `display`. It carries no anchor, so it
    /// cannot reach a snapshot.
    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        div()
            .flex()
            .flex_row()
            .items_center()
            .child(self.button(cell).render(theme, anchors))
            .into_any_element()
    }
}

/// One of `cva`'s seven variant keys.
///
/// The vocabulary is generated from [`ALL_VARIANTS`] rather than restated, so
/// the word the command line takes and the word a caption prints cannot drift
/// into two spellings of one variant.
fn parse_variant(raw: &str) -> Result<Variant, ParseError> {
    ALL_VARIANTS
        .into_iter()
        .find(|variant| variant.name() == raw)
        .ok_or_else(|| {
            ParseError::Rejected(format!(
                "--variant takes one of {}, not {raw}",
                names(ALL_VARIANTS.into_iter().map(Variant::name)),
            ))
        })
}

/// A call site's `rounded-*`, or `none` for the size variant's own.
///
/// The vocabulary is generated from [`ALL_RADIUS_CLASSES`], and `none` is the
/// only word this function adds — the same shape `--git-status none` takes, and
/// for the same reason: the absence is not a variant of the enum.
///
/// **There is deliberately no numeric form.** A `--class-radius 6` would let a
/// future cell be tuned to whatever the reference happened to report, and the
/// anchor would stop being able to fail. What a caller may say is which class
/// the call site wrote; both engines still resolve it from `--radius`.
fn parse_class_radius(raw: &str) -> Result<Option<RadiusClass>, ParseError> {
    if raw == "none" {
        return Ok(None);
    }
    ALL_RADIUS_CLASSES
        .into_iter()
        .find(|class| class.name() == raw)
        .map(Some)
        .ok_or_else(|| {
            ParseError::Rejected(format!(
                "--class-radius takes none or one of {}, not {raw}; it names the \
                 rounded-* class a call site merges, never a pixel value",
                names(ALL_RADIUS_CLASSES.into_iter().map(RadiusClass::name)),
            ))
        })
}

/// One of `cva`'s ten size keys. Generated, for [`parse_variant`]'s reason.
fn parse_size(raw: &str) -> Result<Size, ParseError> {
    ALL_SIZES
        .into_iter()
        .find(|size| size.name() == raw)
        .ok_or_else(|| {
            ParseError::Rejected(format!(
                "--size takes one of {}, not {raw}",
                names(ALL_SIZES.into_iter().map(Size::name)),
            ))
        })
}

/// A vocabulary as one line, for a usage line and for a rejection.
fn names<I: Iterator<Item = &'static str>>(words: I) -> String {
    words.collect::<Vec<_>>().join(", ")
}

fn options() -> Vec<(String, String)> {
    let dead_variants = names(
        ALL_VARIANTS
            .into_iter()
            .filter(|variant| !variant.live())
            .map(Variant::name),
    );
    let dead_sizes = names(
        ALL_SIZES
            .into_iter()
            .filter(|size| !size.live())
            .map(Size::name),
    );
    [
        (
            "--variant <name>".to_owned(),
            format!(
                "one of {}; {dead_variants} has no live call site [{}]",
                names(ALL_VARIANTS.into_iter().map(Variant::name)),
                Button::fixture().variant.name(),
            ),
        ),
        (
            "--size <name>".to_owned(),
            format!(
                "one of {}; {dead_sizes} have no live call site [{}]",
                names(ALL_SIZES.into_iter().map(Size::name)),
                Button::fixture().size.name(),
            ),
        ),
        (
            "--label".to_owned(),
            "render a text child; --content picks the string [off: every live \
             Button is icon-only]"
                .to_owned(),
        ),
        (
            "--no-glyph".to_owned(),
            "drop the leading glyph; not --no-icon, which is git-status-row's \
             [the glyph is on]"
                .to_owned(),
        ),
        (
            "--pressed".to_owned(),
            "data-pressed / :active; no reference [off]".to_owned(),
        ),
        (
            "--disabled".to_owned(),
            "the disabled prop; live, and invisible to the contract [off]".to_owned(),
        ),
        (
            "--loading".to_owned(),
            "the loading prop; no live call site passes it [off]".to_owned(),
        ),
        (
            "--class-radius <name>".to_owned(),
            format!(
                "none|{} — the rounded-* a call site merges over the variant's; \
                 `none` is the primitive's own and has no visible reference [{}]",
                ALL_RADIUS_CLASSES
                    .into_iter()
                    .map(RadiusClass::name)
                    .collect::<Vec<_>>()
                    .join("|"),
                Button::fixture()
                    .class_radius
                    .map_or("none", RadiusClass::name),
            ),
        ),
    ]
    .into_iter()
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{Params, SURFACE, options};
    use crate::row_surface::{CAPTION_HEIGHT, Cell, ParseError, StateFlag};
    use crowbar_ui::components::button;
    use crowbar_ui::components::button::{
        ALL_RADIUS_CLASSES, ALL_SIZES, ALL_VARIANTS, Button, Label, RadiusClass, Size, Variant,
    };

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "button"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a button cell carries this surface's bag")
    }

    fn button(cell: &Cell) -> Button {
        params_of(cell).button(cell)
    }

    /// The default cell is the live app's own cleanest Button, not a
    /// convenient one.
    #[test]
    fn the_default_cell_is_the_reference_button() {
        let bag = Params::default();

        assert_eq!(bag.variant, Variant::Ghost);
        assert_eq!(bag.size, Size::IconSm);
        assert!(bag.icon);
        assert!(!bag.label);
        assert!(!bag.driven.pressed && !bag.driven.disabled && !bag.driven.loading);

        let drawn = button(&cell(&[]));
        assert_eq!(drawn, Button::fixture());
        // 28×28 at the default 800px viewport, which is the `sm` side.
        assert_eq!(drawn.size.extent(drawn.breakpoint), gpui::px(28.0));
    }

    /// **`--viewport-width` is the axis that does the work**, and it reaches the
    /// button through the shared `breakpoint`, not through a private option.
    #[test]
    fn the_viewport_selects_the_sm_variant_of_every_size() {
        let wide = button(&cell(&["--viewport-width", "800"]));
        let narrow = button(&cell(&["--viewport-width", "600", "--width", "300"]));

        for size in ALL_SIZES {
            let mut a = wide.clone();
            let mut b = narrow.clone();
            a.size = size;
            b.size = size;
            assert_eq!(
                b.size.extent(b.breakpoint) - a.size.extent(a.breakpoint),
                gpui::px(4.0),
                "{}",
                size.name(),
            );
        }
        // 640 itself is `sm`, exactly as `(width >= 40rem)` says.
        let edge = button(&cell(&["--viewport-width", "640", "--width", "300"]));
        assert_eq!(edge.breakpoint, wide.breakpoint);
    }

    /// Every variant and every size parses, by `cva`'s own key, and the
    /// vocabulary is closed.
    #[test]
    fn the_whole_cva_vocabulary_parses_and_nothing_else_does() {
        for variant in ALL_VARIANTS {
            let driven = cell(&["--variant", variant.name()]);
            assert_eq!(params_of(&driven).variant, variant, "{}", variant.name());
            assert_eq!(button(&driven).variant, variant);
        }
        for size in ALL_SIZES {
            let driven = cell(&["--size", size.name()]);
            assert_eq!(params_of(&driven).size, size, "{}", size.name());
            assert_eq!(button(&driven).size, size);
        }

        for line in [
            vec!["--variant", "primary"],
            vec!["--variant", "Ghost"],
            vec!["--variant", ""],
            vec!["--variant"],
            vec!["--size", "medium"],
            vec!["--size", "icon-md"],
            vec!["--size"],
        ] {
            let mut full = vec!["--surface", "button"];
            full.extend_from_slice(&line);
            assert!(
                matches!(
                    Cell::parse(full.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{line:?} should have been rejected",
            );
        }

        // And a rejection lists what was wanted, because "invalid" is not
        // something anyone can act on.
        let Err(ParseError::Rejected(complaint)) = Cell::parse(
            ["--surface", "button", "--variant", "primary"]
                .iter()
                .map(|arg| (*arg).to_owned()),
        ) else {
            panic!("`primary` is not a variant");
        };
        for variant in ALL_VARIANTS {
            assert!(complaint.contains(variant.name()), "{complaint}");
        }
    }

    /// **`--class-radius` names a class and resolves it through the token**, and
    /// the default is the call site a reference can be captured from.
    #[test]
    fn the_class_radius_option_names_a_class_and_never_a_number() {
        let theme = crowbar_ui::Theme::DARK;

        // The default is the tab bar's `rounded-sm`, which is what makes the
        // default cell converge without flags.
        assert_eq!(params_of(&cell(&[])).class_radius, Some(RadiusClass::Sm));
        assert_eq!(button(&cell(&[])).radius(&theme), gpui::px(6.0));

        for class in ALL_RADIUS_CLASSES {
            let driven = cell(&["--class-radius", class.name()]);
            assert_eq!(
                params_of(&driven).class_radius,
                Some(class),
                "{}",
                class.name()
            );
            // Through the token, so this is the compiled value rather than a
            // literal restated here.
            assert_eq!(button(&driven).radius(&theme), class.value(&theme));
        }

        // `none` is the primitive's own, which for `icon-sm` is `rounded-lg`.
        let bare = cell(&["--class-radius", "none"]);
        assert_eq!(params_of(&bare).class_radius, None);
        assert_eq!(button(&bare).radius(&theme), Size::IconSm.radius(&theme));
        assert_eq!(button(&bare).radius(&theme), gpui::px(10.0));
        assert_ne!(
            button(&bare).radius(&theme),
            button(&cell(&[])).radius(&theme)
        );

        // **No numeric form.** A pixel value is a rejection, not a fourth arm.
        for line in [
            vec!["--class-radius", "6"],
            vec!["--class-radius", "6px"],
            vec!["--class-radius", "rounded-sm"],
            vec!["--class-radius", "xl"],
            vec!["--class-radius", ""],
            vec!["--class-radius"],
        ] {
            let mut full = vec!["--surface", "button"];
            full.extend_from_slice(&line);
            assert!(
                matches!(
                    Cell::parse(full.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{line:?} should have been rejected",
            );
        }

        let Err(ParseError::Rejected(complaint)) = Cell::parse(
            ["--surface", "button", "--class-radius", "6"]
                .iter()
                .map(|arg| (*arg).to_owned()),
        ) else {
            panic!("a pixel value is not a class");
        };
        assert!(complaint.contains("never a pixel value"), "{complaint}");
        assert!(complaint.contains("none"), "{complaint}");
        for class in ALL_RADIUS_CLASSES {
            assert!(complaint.contains(class.name()), "{complaint}");
        }
    }

    /// The default cell **names the call site it reproduces**, and `none` says
    /// that the primitive's own radius has nothing to compare against.
    #[test]
    fn the_caption_names_the_call_site_and_the_unreferenced_alternative() {
        let default = cell(&[]).describe();
        assert!(default.contains("rounded-sm"), "{default}");
        assert!(default.contains("tab-navigation-buttons.tsx"), "{default}");
        assert!(!default.contains("no visible reference"), "{default}");

        let bare = cell(&["--class-radius", "none"]).describe();
        assert!(bare.contains("no visible reference"), "{bare}");
        assert!(bare.contains("visible:false"), "{bare}");
        assert!(!bare.contains("rounded-"), "{bare}");

        // The call-site sentence is about **that** call site, so a different
        // variant or size carrying the same class does not claim it.
        let elsewhere = cell(&["--variant", "outline", "--class-radius", "sm"]).describe();
        assert!(elsewhere.contains("rounded-sm"), "{elsewhere}");
        assert!(
            !elsewhere.contains("tab-navigation-buttons.tsx"),
            "{elsewhere}"
        );

        // The workspace header's own pair, which is the other live override.
        let header = cell(&["--size", "icon-xs", "--class-radius", "lg"]).describe();
        assert!(header.contains("rounded-lg"), "{header}");
        assert!(!header.contains("tab-navigation-buttons.tsx"), "{header}");
    }

    /// `--label` turns the text child on and `--content` picks the string;
    /// without it the content axis is vacuous and the caption says so.
    #[test]
    fn the_label_is_off_by_default_and_content_picks_the_string() {
        assert_eq!(button(&cell(&[])).label, None);
        assert!(cell(&[]).describe().contains("--content cannot fail"));

        for (word, label) in [
            ("short", Label::Short),
            ("normal", Label::Normal),
            ("overflow", Label::Overflow),
        ] {
            let driven = cell(&["--label", "--content", word]);
            assert_eq!(button(&driven).label, Some(label), "{word}");
            let caption = driven.describe();
            assert!(caption.contains(label.text()), "{caption}");
            assert!(!caption.contains("--content cannot fail"), "{caption}");
        }
    }

    /// The three §8.3 flags this surface really has reach the button, and
    /// `selected` reaches the **`active` prop** rather than a state of its own.
    #[test]
    fn the_three_modelled_flags_reach_the_buttons_state() {
        let resting = button(&cell(&[]));
        assert!(
            !resting.state.interaction.hovered
                && !resting.state.interaction.focused
                && !resting.state.props.active
        );

        let hovered = button(&cell(&["--flags", "hover"]));
        assert!(
            hovered.state.interaction.hovered
                && !hovered.state.interaction.focused
                && !hovered.state.props.active
        );

        let focused = button(&cell(&["--flags", "focus"]));
        assert!(focused.state.interaction.focused && !focused.state.interaction.hovered);

        let selected = button(&cell(&["--flags", "selected"]));
        assert!(selected.state.props.active);
        assert!(!selected.state.interaction.hovered && !selected.state.interaction.focused);

        for flag in [StateFlag::Hover, StateFlag::Focus, StateFlag::Selected] {
            assert!(!SURFACE.unmodelled(flag), "{}", flag.name());
        }
    }

    /// The three prop options are not matrix flags, and each reaches exactly
    /// one field.
    #[test]
    fn the_prop_options_reach_the_button_and_loading_forces_disabled() {
        assert!(button(&cell(&["--pressed"])).state.interaction.pressed);
        assert!(button(&cell(&["--disabled"])).state.props.disabled);
        assert!(button(&cell(&["--loading"])).state.props.loading);
        assert!(!button(&cell(&["--disabled"])).state.props.loading);
        assert!(!button(&cell(&["--loading"])).state.props.disabled);

        // `isDisabled = Boolean(loading || disabledProp)`.
        assert!(button(&cell(&["--loading"])).state.is_disabled());
        assert!(button(&cell(&["--disabled"])).state.is_disabled());
        assert!(!button(&cell(&[])).state.is_disabled());

        assert!(!button(&cell(&["--no-glyph"])).icon);
        assert!(button(&cell(&[])).icon);
    }

    /// **`--flags loading` reaches nothing**, which is what makes
    /// `Surface::unmodelled` honest here — the loading picture is drawn by
    /// `--loading`, and the module docs say why.
    #[test]
    fn the_loading_flag_reaches_nothing_and_the_option_reaches_the_prop() {
        let flagged = button(&cell(&["--flags", "loading"]));
        assert!(!flagged.state.props.loading);
        assert_eq!(flagged, Button::fixture());
        assert!(SURFACE.unmodelled(StateFlag::Loading));
        assert!(SURFACE.unmodelled(StateFlag::Error));
        assert!(SURFACE.unmodelled(StateFlag::Empty));

        let optioned = button(&cell(&["--loading"]));
        assert!(optioned.state.props.loading);
        assert_ne!(optioned, Button::fixture());
    }

    /// **Every cell that cannot fail, or has no reference, says which.** The two
    /// are different complaints with different remedies and the caption keeps
    /// them apart.
    #[test]
    fn the_captions_separate_cannot_fail_from_no_reference() {
        // No field in the contract.
        let focus = cell(&["--flags", "focus"]).describe();
        assert!(focus.contains("box-shadow"), "{focus}");
        assert!(focus.contains("cannot fail"), "{focus}");

        let disabled = cell(&["--disabled"]).describe();
        assert!(disabled.contains("cannot fail"), "{disabled}");

        let default = cell(&["--variant", "default"]).describe();
        assert!(default.contains("--theme cannot fail"), "{default}");
        assert!(
            !cell(&["--variant", "ghost"])
                .describe()
                .contains("--theme cannot fail"),
        );

        // Drawable here, not on the other side.
        for line in [
            vec!["--flags", "hover"],
            vec!["--flags", "selected"],
            vec!["--loading"],
            vec!["--pressed"],
            vec!["--variant", "destructive-outline"],
            vec!["--size", "xl"],
        ] {
            let caption = cell(&line).describe();
            assert!(caption.contains("no reference"), "{line:?}: {caption}");
        }

        // And a live combination claims neither.
        let plain = cell(&[]).describe();
        assert!(!plain.contains("no reference"), "{plain}");
        assert!(!plain.contains("--theme cannot fail"), "{plain}");
        assert!(
            plain.contains("ghost") && plain.contains("icon-sm"),
            "{plain}"
        );
    }

    /// The dead arms are named as data, so a caption cannot claim a reference
    /// the app does not have.
    #[test]
    fn the_dead_arms_are_the_ones_the_component_names() {
        for variant in ALL_VARIANTS {
            let caption = cell(&["--variant", variant.name()]).describe();
            assert_eq!(
                caption.contains("variant: no live call site"),
                !variant.live(),
                "{}",
                variant.name(),
            );
        }
        for size in ALL_SIZES {
            let caption = cell(&["--size", size.name()]).describe();
            assert_eq!(
                caption.contains("size: no live call site"),
                !size.live(),
                "{}",
                size.name(),
            );
        }
    }

    /// **These options belong to this surface and to no other.**
    #[test]
    fn this_surfaces_options_are_rejected_on_another_surface() {
        for option in [
            "--variant",
            "--size",
            "--label",
            "--pressed",
            "--loading",
            "--class-radius",
        ] {
            let line = ["--surface", "resizable", option, "ghost"];
            assert!(
                matches!(
                    Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{option} should not be a resizable option",
            );
        }
        assert!(matches!(
            Cell::parse(
                ["--surface", "resizable", "--disabled"]
                    .iter()
                    .map(|arg| (*arg).to_owned())
            ),
            Err(ParseError::Rejected(_)),
        ));
    }

    /// **The shared parser's words are not this surface's.** `Cell::parse`
    /// matches every option it knows *before* it asks the selected surface's
    /// bag, so a surface option spelled the same as a shared one is swallowed
    /// with no error anywhere. `--no-icon` is the collision this surface had:
    /// it is `git-status-row`'s `showFileIcon`, so this one is `--no-glyph`.
    #[test]
    fn the_shared_parsers_words_are_not_this_surfaces() {
        // Accepted, because the shared parser owns it — and it reaches the
        // *shared* field, leaving this surface's glyph alone.
        let shared = cell(&["--no-icon"]);
        assert!(!shared.show_file_icon);
        assert!(button(&shared).icon, "--no-icon must not reach the glyph");

        // Where `--no-glyph` reaches the glyph and leaves the shared field
        // alone.
        let own = cell(&["--no-glyph"]);
        assert!(own.show_file_icon);
        assert!(!button(&own).icon);
    }

    /// Every option the parser accepts appears in the usage, or driving the
    /// matrix means reading the source.
    #[test]
    fn the_usage_names_every_option_this_surface_takes() {
        let usage = crate::row_surface::usage();

        for option in [
            "--variant",
            "--size",
            "--label",
            "--no-glyph",
            "--pressed",
            "--disabled",
            "--loading",
            "--class-radius",
        ] {
            assert!(usage.contains(option), "{option} is missing from the usage");
        }
        assert!(usage.contains("button"));
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
        // And every word each of the two vocabularies takes.
        for variant in ALL_VARIANTS {
            assert!(usage.contains(variant.name()), "{}", variant.name());
        }
        for size in ALL_SIZES {
            assert!(usage.contains(size.name()), "{}", size.name());
        }
        for class in ALL_RADIUS_CLASSES {
            assert!(usage.contains(class.name()), "{}", class.name());
        }
    }

    /// The registry entry's contract fields, which a snapshot carries verbatim.
    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "button");
        assert_eq!(SURFACE.root, "button");
        assert_eq!(SURFACE.root, button::ID_BUTTON);
        assert!(!SURFACE.full_bleed);
        // The floor holds the tallest button and its caption: `xl` below the
        // breakpoint is 44px.
        assert!(SURFACE.min_window_height >= 44 + CAPTION_HEIGHT);
    }
}
