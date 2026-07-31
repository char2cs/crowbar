//! `--surface card` — a slotted container with **no reference**, because the
//! only thing that renders one is a caught render throw.
//!
//! See `crowbar_ui::components::card` for the reachability finding, and
//! `native/mapping/card.md`. The short form: `[data-slot=card]` has zero live
//! instances, the sole importer is `error-boundary.tsx`'s fallback, and reaching
//! that means introducing a defect in one of the three boundaries that pass no
//! `fallback` prop.
//!
//! The values are the utilities', resolved through the app's own compiled
//! `tailwindcss` 4.3.0 and read back off a probe element in the live document.
//! **No reference JSON was fabricated.**
//!
//! # `--slots` is what makes this surface a surface
//!
//! Three of the four slots carry an `in-[…]` variant keyed on what the card
//! *contains*, so the header's bottom padding and the panel's top and bottom
//! padding are functions of which siblings exist. `--slots` names the
//! combination, and it is the axis the component's own layout turns on — a cell
//! that could not vary it would only ever measure one of four arrangements.
//!
//! It is also why the surface declares **no anchor set** on the reference side:
//! the set is `header`/`title`/`panel`, which is exactly what this option moves.
//! `ANCHORS.md` v1.8 permits a declaration only where the set is a property of
//! the surface, and here it is a property of the cell — `git-status-row`'s
//! standing, for the same reason.
//!
//! # What each axis can and cannot do here
//!
//! | Axis | Here |
//! |---|---|
//! | `--width` | **real up to 384**, and then not: `max-w-sm` clamps the card, so every width above it renders the same box. That is a genuine property and not a defect, and `the_card_stops_growing_at_max_w_sm` measures it |
//! | `--content` | **vacuous.** The title is `Something went wrong`, from the boundary; nothing else on this surface paints a run this port owns |
//! | `--theme` | **real**: `--destructive`, `--card` and `--border` all differ in the two tables |
//! | `--viewport-width` | **vacuous.** No `sm:` rule anywhere in `card.tsx` |
//!
//! # The state axis
//!
//! Five of six are unmodelled. `card.tsx` has **no interaction rule of any kind**
//! — it is four class lists and four `useRender`s, and there is nothing to
//! disagree about. `empty` is the exception and is real: a `<Card>` with no
//! children is an expressible rendering that paints its border and its tint and
//! nothing else, and its snapshot is one anchor rather than four.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::AnchorSink;
use crowbar_ui::components::card::{self, ALL_CALL_SITES, ALL_SLOTS, CallSite, Card, Slots};
use gpui::AnyElement;

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "card",
    root: card::ID_CARD,
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        // `card.tsx` has no interaction rule of any kind — see the module docs.
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // The probe measures the boundary's card at 102 tall, plus
    // `CAPTION_HEIGHT`'s 29 and room for the slots a cell can add.
    min_window_height: 200,
    // A card is centred in whatever pane holds it, never the window.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--call-site`: the className bundle merged over the primitive's own.
    pub call_site: CallSite,
    /// `--slots`: which of the four slots this cell fills.
    pub slots: Slots,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            call_site: Card::fixture().call_site,
            slots: Slots::HeaderAndPanel,
        }
    }
}

impl Params {
    /// The card this cell describes.
    #[must_use]
    pub fn card(&self, cell: &Cell) -> Card {
        let slots = if cell.has(StateFlag::Empty) {
            Slots::Empty
        } else {
            self.slots
        };
        Card {
            call_site: self.call_site,
            slots,
            title: Card::fixture().title,
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
            "--call-site" => self.call_site = parse_call_site(&value(args, option)?)?,
            "--slots" => self.slots = parse_slots(&value(args, option)?)?,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** A card's height is its slots' content, and no option authors
    /// one.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(out, " · class {}", self.call_site.name());
        if cell.has(StateFlag::Empty) {
            out.push_str(
                " · empty: no slots, so the snapshot is the card's own box alone \
                 and the other three anchors are absent",
            );
            return;
        }
        let _ = write!(out, " · slots {}", self.slots.name());
        let slots = self.slots;
        let _ = write!(
            out,
            " · header pb {:?}, panel pt {:?}, panel pb {:?} — every one an \
             `in-[…]` variant keyed on a sibling",
            slots.header_padding_bottom(),
            slots.panel_padding_top(),
            slots.panel_padding_bottom(),
        );
        if self.call_site == CallSite::None {
            out.push_str(
                " · class: no live call site leaves the primitive's className \
                 alone, so there is no reference even in principle",
            );
        }
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.card(cell).render(theme, anchors)
    }
}

/// A call site's `className` bundle. **No numeric form**, the line P3.1 drew.
fn parse_call_site(raw: &str) -> Result<CallSite, ParseError> {
    ALL_CALL_SITES
        .into_iter()
        .find(|site| site.name() == raw)
        .ok_or_else(|| {
            ParseError::Rejected(format!(
                "--call-site takes one of {}, not {raw}; it names the className \
                 bundle a call site merges, never a pixel value",
                names(ALL_CALL_SITES.into_iter().map(CallSite::name)),
            ))
        })
}

/// A slot combination.
fn parse_slots(raw: &str) -> Result<Slots, ParseError> {
    ALL_SLOTS
        .into_iter()
        .find(|set| set.name() == raw)
        .ok_or_else(|| {
            ParseError::Rejected(format!(
                "--slots takes one of {}, not {raw}",
                names(ALL_SLOTS.into_iter().map(Slots::name)),
            ))
        })
}

/// A vocabulary as one line, for a usage line and for a rejection.
fn names<I: Iterator<Item = &'static str>>(words: I) -> String {
    words.collect::<Vec<_>>().join(", ")
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--call-site <name>".to_owned(),
            format!(
                "one of {} — the className bundle a call site merges, never a \
                 pixel value [{}]",
                names(ALL_CALL_SITES.into_iter().map(CallSite::name)),
                Card::fixture().call_site.name(),
            ),
        ),
        (
            "--slots <name>".to_owned(),
            format!(
                "one of {} — which slots the cell fills, which is what the three \
                 `in-[…]` padding variants key on [{}]",
                names(ALL_SLOTS.into_iter().map(Slots::name)),
                Slots::HeaderAndPanel.name(),
            ),
        ),
    ]
    .into_iter()
    .collect()
}
