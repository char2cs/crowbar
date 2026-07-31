//! `--surface callout-node` — a Plate callout block, and the only P3.10 surface
//! with a captured reference.
//!
//! The default cell is the one measured live at a 1714px viewport in a 720px
//! editor column: `720 × 68.58`, `bg #f5f5f50d`, `radius 8`, `border 0`, over
//! four anchors. See `crowbar_ui::components::callout_node`,
//! `native/mapping/callout-node.md` and `/tmp/p3-ref-callout-node.json`.
//!
//! # The fixture is data, and deleting it removes the only capturable callout
//!
//! A callout renders from a markdown file whose blockquote opens with a GitHub
//! alert marker, so the live count was **0** until one was written:
//!
//! ```text
//! <fixture workspace>/callout-fixture.md
//!
//!   > [!NOTE]
//!   > Reachability
//! ```
//!
//! Opening that file in the app renders `CalloutElement` through
//! `markdown-callout-rules.ts`'s `blockquote` deserializer. That is a **user
//! path** and not a harness: the file is ordinary data, exactly as P3.3's agent
//! reply and Phase 1's dirty git fixture are, and no app state was reached into.
//! **Deleting the file removes the only capturable Callout**, which is P3.3's
//! standing for its Badge.
//!
//! Editor **focus is not required**, which is worth stating because the brief
//! expected it to be: `document.hasFocus()` is `false` and immovable, and it
//! gates Plate's `FloatingToolbar` (which is what blocked `separator`) — not node
//! rendering. A Plate element renders from the document, and the document is on
//! disk.
//!
//! # What each axis can and cannot do here
//!
//! | Axis | Here |
//! |---|---|
//! | `--width` | **real** — the callout is `w-full` in the editor's measured column, so every anchor's width moves with it |
//! | `--content` | **real** — it moves the body run, which is what the `callout-content` box is sized around |
//! | `--theme` | **real**: `--md-wash` is derived from `--foreground`, which differs in the two tables |
//! | `--viewport-width` | **vacuous.** Nothing on this component carries an `sm:` rule of its own. The emoji control's `sm:h-8` is already resolved into `EMOJI_HEIGHT` because the class list that loses to it is the *call site's*, not a breakpoint the surface can move |
//!
//! # The state axis
//!
//! `hover` is **real and is the only one**: `hover:bg-muted-foreground/15` on the
//! emoji control is the single interaction rule anywhere in `callout-node.tsx`.
//! It is taken as a parameter rather than as a `.hover(…)` refinement, per
//! `ANCHORS.md` §6.
//!
//! `empty` is real too and is a genuine branch — a callout whose paragraph has no
//! text is what Plate leaves behind when the body is deleted, and it collapses
//! `callout-content` onto an empty line box rather than removing it.
//!
//! `selected` looks plausible and is **not** modelled: Plate's selection paints
//! through `slate-selectable`, which resolves against editor state this port has
//! no equivalent for and which the reference cannot be driven into either
//! (`document.hasFocus()` again).

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::callout_node::{self, CalloutNode};
use crowbar_ui::components::{AnchorSink, ContentLength};
use gpui::{AnyElement, SharedString};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "callout-node",
    root: callout_node::ID_CALLOUT,
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        // Plate's selection is editor state, not a class this component carries.
        StateFlag::Selected,
        // No `focus-visible:` rule anywhere on the callout or its control.
        StateFlag::Focus,
    ],
    // The reference callout is 68.58 tall plus `CAPTION_HEIGHT`'s 29, and the
    // `1.2em` margins the surface does not draw would add 38 more. A floor, not
    // a ceiling: the height follows the body, which `--content` moves.
    min_window_height: 140,
    // A callout sits in the editor's measured column, never the window.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--icon`: `props.element.icon`, the emoji a user picks from the popover.
    ///
    /// A **string the call site supplies**, not a number the port is handed: it
    /// is the same input both engines shape independently, which is the line
    /// P3.1 drew for `--class-radius`.
    pub icon: SharedString,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            icon: CalloutNode::fixture().icon,
        }
    }
}

impl Params {
    /// The callout this cell describes.
    #[must_use]
    pub fn callout(&self, cell: &Cell) -> CalloutNode {
        CalloutNode {
            icon: self.icon.clone(),
            body: if cell.has(StateFlag::Empty) {
                SharedString::new_static("")
            } else {
                body_of(cell.content)
            },
            emoji_hovered: cell.has(StateFlag::Hover),
        }
    }
}

/// The body a content length shows.
///
/// **Every one is a single unbreakable token.** The body is the only run on this
/// surface, and a run that wraps is outside what the contract compares at all —
/// the DOM sums client rects where gpui shapes one line. `--content` therefore
/// moves the run's *width* without ever introducing a break opportunity, which is
/// what keeps the overflow cell comparable rather than merely long.
fn body_of(content: ContentLength) -> SharedString {
    match content {
        ContentLength::Short => SharedString::new_static("Note"),
        ContentLength::Normal => SharedString::new_static("Reachability"),
        ContentLength::Overflow => {
            SharedString::new_static("Reachability-was-measured-before-anything-was-written")
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
            "--icon" => self.icon = parse_icon(&value(args, option)?)?,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** A callout's height is its padding plus its body's line boxes,
    /// and no option here authors one.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(out, " · icon {}", self.icon);
        if cell.has(StateFlag::Empty) {
            out.push_str(
                " · empty: the paragraph keeps its line box, so `callout-content` \
                 collapses onto one empty line rather than vanishing",
            );
        } else {
            let _ = write!(out, " · \"{}\"", body_of(cell.content));
        }
        if cell.has(StateFlag::Hover) {
            out.push_str(
                " · hover: `bg-muted-foreground/15` on the emoji control — the only \
                 interaction rule this component has",
            );
        }
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.callout(cell).render(theme, anchors)
    }
}

/// One emoji.
///
/// Rejecting a multi-character string is not pedantry: the control is a 24px box
/// with 4px of padding, so a second glyph changes `text_width` and `clipped` on
/// the surface's only text anchor while looking like the same cell in the
/// caption.
fn parse_icon(raw: &str) -> Result<SharedString, ParseError> {
    let mut chars = raw.chars();
    match (chars.next(), chars.next()) {
        (Some(_), None) => Ok(SharedString::from(raw.to_owned())),
        _ => Err(ParseError::Rejected(format!(
            "--icon takes exactly one character, not {raw:?}; the control is a \
             24px box and a second glyph moves text_width and clipped",
        ))),
    }
}

fn options() -> Vec<(String, String)> {
    [(
        "--icon <char>".to_owned(),
        format!(
            "the emoji `props.element.icon` supplies, one character [{}]",
            CalloutNode::fixture().icon,
        ),
    )]
    .into_iter()
    .collect()
}
