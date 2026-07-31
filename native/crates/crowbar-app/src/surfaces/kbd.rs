//! `--surface kbd` — one keycap, and the P3.6 leaf with a captured reference.
//!
//! The default cell is the workspace switcher's `Esc` cap, measured live at a
//! 1714px viewport: `27.61 × 20`, `bg #ffffff0a`, `fg #a4a4a4ff`, `radius 4`,
//! `border.w 0`, `CalSansUI` 12/16 at weight 500. See
//! `native/mapping/kbd.md` and `/tmp/p3-ref-kbd.json`.
//!
//! # The root is the **cap**, never the group
//!
//! `KbdGroup` is modelled — `--group` renders one — but it is not a root and
//! carries no anchor. `data-oracle-id` lives on the primitive, so every cap
//! inside a group carries the id `kbd`; a snapshot rooted at the group would
//! hold that id twice, which `ANCHORS.md` v1.8 ranks a **refusal** rather than a
//! delta. `--group` therefore changes the cap's *layout context* and leaves the
//! anchor set at one, which is the only arrangement that can be compared.
//!
//! # What each axis can and cannot do here
//!
//! | Axis | Here |
//! |---|---|
//! | `--content` | **real** — the cap's width is its label's advance width above a 20px floor, and the three lengths straddle it |
//! | `--theme` | **real**: `bg-muted` and `text-muted-foreground` are different tokens in the two tables |
//! | `--width` | **vacuous.** Nothing here is a percentage or a stretch |
//! | `--viewport-width` | **vacuous.** `kbd.tsx` contains no `sm:` variant at all — the second component in the port with none, after `avatar` |
//!
//! # The state axis
//!
//! Five of the six are unmodelled, and the reason is the one `avatar` gave:
//! **`kbd.tsx` has no interaction rule of any kind.** No `hover:`, no `focus`,
//! no `data-[…]`, no `disabled:`. It carries `pointer-events-none`, which is
//! the class saying so out loud.
//!
//! `empty` is the exception, and it is a real branch rather than a contrivance:
//! `<Kbd>` takes its content from the call site, so a cap with neither a glyph
//! nor a legend is an expressible rendering, and it is the one picture where the
//! anchor carries **no text half at all** — no `text`, no `fg`, no `font`.
//! `ANCHORS.md` ranks a missing field group above a wrong number, so this is the
//! cell most likely to catch a port that paints a run where the DOM has none.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::kbd::{Cap, Kbd, KbdGroup};
use crowbar_ui::components::{AnchorSink, ContentLength, kbd};
use gpui::{AnyElement, IntoElement as _, ParentElement as _, SharedString, Styled as _, div};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "kbd",
    root: kbd::ID_KBD,
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        // `kbd.tsx` has no interaction rule of any kind — see the module docs.
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // A 20px cap and `CAPTION_HEIGHT`'s 29. 72 holds both with room, and is a
    // floor rather than a ceiling: this surface drives no height, a keycap's
    // being authored by `h-5`.
    min_window_height: 72,
    // A keycap sits in a command palette's footer — never the window.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--glyph`: the cap holds an icon rather than a string. Three of the four
    /// live caps do, and they sit exactly on `min-w-5`.
    pub glyph: bool,
    /// `--group`: wrap the cap in a `KbdGroup` beside a second one, which is the
    /// live ↑/↓ arrangement. Changes the cap's layout context only — the group
    /// carries no anchor, see the module docs.
    pub group: bool,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            glyph: matches!(Kbd::fixture().cap, Cap::Glyph),
            group: false,
        }
    }
}

impl Params {
    /// The cap this cell describes, built from the live fixture so a bare
    /// `--surface kbd` renders the cap the reference actually has.
    #[must_use]
    pub fn kbd(&self, cell: &Cell) -> Kbd {
        Kbd {
            cap: if cell.has(StateFlag::Empty) {
                // `empty` overrides the glyph too: §8.3's word is "a surface
                // with nothing in it", and a cap holding an icon is not that.
                Cap::Empty
            } else if self.glyph {
                Cap::Glyph
            } else {
                Cap::Text(legend_of(cell.content))
            },
        }
    }
}

/// The legend a content length shows.
///
/// A translation rather than a shared type, for `button`'s reason: the cell's
/// vocabulary and the component's carry different strings on purpose. `Esc` is
/// the captured one; the other two straddle the `min-w-5` floor, which is the
/// only thing `--content` can move on this surface.
fn legend_of(content: ContentLength) -> SharedString {
    match content {
        // One glyph — narrower than the 20px floor, so the floor binds.
        ContentLength::Short => SharedString::new_static("K"),
        // The captured cell.
        ContentLength::Normal => SharedString::new_static("Esc"),
        // Wider than anything the live footer shows.
        ContentLength::Overflow => SharedString::new_static("Backspace"),
    }
}

impl SurfaceParams for Params {
    fn accept(
        &mut self,
        option: &str,
        _args: &mut dyn Iterator<Item = String>,
    ) -> Result<bool, ParseError> {
        match option {
            "--glyph" => self.glyph = true,
            "--group" => self.group = true,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** A keycap's height is `h-5`, and no option here moves it.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        if cell.has(StateFlag::Empty) {
            out.push_str(
                " · empty: a cap with no child, which falls to the 20px min-w-5 floor \
                 and carries no text half; no live call site renders it",
            );
        } else if self.glyph {
            out.push_str(" · glyph: an icon cap, which sits on the 20px min-w-5 floor");
        } else {
            let _ = write!(out, " · legend \"{}\"", legend_of(cell.content));
        }
        if self.group {
            out.push_str(
                " · in a KbdGroup beside a second cap; the group carries no anchor \
                 (ANCHORS.md v1.8 — two `kbd` ids under one root is a refusal)",
            );
        }
    }

    /// The cap, inside the flex row that makes it a flex item.
    ///
    /// `RowSurface` draws every surface into a gpui **block** container, and a
    /// cap drawn straight into one would be a block-level flex box. Every live
    /// `<Kbd>` is a flex item — which is also why the reference's computed
    /// `display` is `flex` rather than `inline-flex`, CSS having blockified it.
    /// The row carries no anchor, so it cannot reach a snapshot.
    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let row = div().flex().flex_row().items_center();
        if self.group {
            // The live group is the ↑/↓ pair: the measured cap first, then a
            // second one so the group's `gap-1` is exercised rather than
            // asserted. Only the first carries the anchor.
            let group = KbdGroup {
                caps: vec![self.kbd(cell), Kbd { cap: Cap::Glyph }],
            };
            row.child(group.render(theme, anchors)).into_any_element()
        } else {
            row.child(self.kbd(cell).render(theme, anchors))
                .into_any_element()
        }
    }
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--glyph".to_owned(),
            format!(
                "the cap holds an icon rather than a string; three of the four live caps \
                 do, and they sit on the 20px min-w-5 floor [{}]",
                if Params::default().glyph { "on" } else { "off" },
            ),
        ),
        (
            "--group".to_owned(),
            "wrap the cap in a KbdGroup beside a second one, the live ↑/↓ arrangement; \
             the group carries no anchor of its own [off]"
                .to_owned(),
        ),
    ]
    .into_iter()
    .collect()
}
