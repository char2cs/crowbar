//! `--surface placeholder-row-actions` — the placeholder row's reconstructed
//! reason plus Retry/Detach… pair.
//!
//! See `crowbar_ui::components::placeholder_row_actions` for the port itself
//! and `native/mapping/placeholder-row-actions.md` for the measurement.
//!
//! # What each axis can and cannot do here
//!
//! | Axis | Here |
//! |---|---|
//! | `--width` | **real** — the panel stretches, and the action row re-justifies inside it |
//! | `--content` | **real** — `placeholderReason`'s three real strings, gated on `--held` the same way the real prop is gated on `heldByPath` |
//! | `--theme` | **real**: the reason's `text-muted-foreground` and both buttons' colours differ in the two tables |
//! | `--viewport-width` | **real** — both buttons' `sm:h-7` moves at 640px, `inline-error`'s own retry control axis one door over |
//!
//! # The state axis
//!
//! `empty` has no seam: nothing on this surface is ever absent for want of
//! content the way `inline-error`'s dev-only detail line is — the reason
//! string always has one of its three real shapes, and the Detach… button's
//! presence is `--held`, not `empty`. `hover`/`focus`/`selected`/`loading` all
//! belong to `button`'s own surface, the identical call `inline-error.rs`
//! makes for its own composed retry control.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::placeholder_row_actions::{self, PlaceholderRowActions};
use crowbar_ui::components::{AnchorSink, ContentLength};
use gpui::{AnyElement, SharedString};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "placeholder-row-actions",
    root: placeholder_row_actions::ID_PANEL,
    unmodelled: &[
        StateFlag::Empty,
        StateFlag::Loading,
        StateFlag::Error,
        // Both buttons' own hover/focus/selected belong to `button`'s surface
        // — the composed-control call `inline_error.rs` makes.
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // The probe measures 72 tall (held) / 66 tall (unheld) at the real 262px
    // detail-wrapper width, plus `CAPTION_HEIGHT`'s 29. A floor, not a
    // ceiling: the panel is a plain column that takes the height its content
    // needs.
    min_window_height: 130,
    // Sits inset inside `workspace-tree-item.tsx`'s own `mx-1.5 px-2.5`
    // wrapper, not flush chrome.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--held`: whether `workspace.heldByPath` is set — gates the Detach…
    /// button, exactly as the real prop does.
    pub held: bool,
}

impl Default for Params {
    fn default() -> Self {
        Self { held: true }
    }
}

impl Params {
    /// The panel this cell describes.
    #[must_use]
    pub fn panel(&self, cell: &Cell) -> PlaceholderRowActions {
        PlaceholderRowActions {
            reason: reason_of(self.held, cell.content),
            detach_available: self.held,
            breakpoint: cell.breakpoint(),
        }
    }
}

/// One of `placeholderReason`'s three real strings — the held arm when
/// `held`, the generic (no-holder, no-error) arm otherwise. `--content` moves
/// the interpolated branch/path the same way it moves `inline-error`'s
/// detail line, never introducing a real fourth string.
fn reason_of(held: bool, content: ContentLength) -> SharedString {
    let branch = match content {
        ContentLength::Short => "ci",
        ContentLength::Normal => "fix-auth-bug",
        ContentLength::Overflow => "feature/rewrite-the-onboarding-flow-end-to-end",
    };
    if held {
        SharedString::from(format!(
            "`{branch}` is checked out at /Users/dev/crowbar-worktrees/{branch} — detach it to \
             let Crowbar manage this branch.",
        ))
    } else {
        SharedString::from(format!("Crowbar couldn't set up `{branch}`. Retry to provision it."))
    }
}

impl SurfaceParams for Params {
    fn accept(
        &mut self,
        option: &str,
        args: &mut dyn Iterator<Item = String>,
    ) -> Result<bool, ParseError> {
        match option {
            "--held" => {
                let raw = crate::surface::value(args, option)?;
                self.held = match raw.as_str() {
                    "true" => true,
                    "false" => false,
                    _ => {
                        return Err(ParseError::Rejected(format!(
                            "--held takes true or false, not {raw}"
                        )));
                    }
                };
            }
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** The panel is a plain column, not a fixed-height surface.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(out, " · held {}", self.held);
        let _ = write!(out, " · reason \"{}\"", reason_of(self.held, cell.content));
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.panel(cell).render(theme, anchors)
    }
}

fn options() -> Vec<(String, String)> {
    vec![(
        "--held <true|false>".to_owned(),
        format!(
            "whether `workspace.heldByPath` is set, gating the Detach… button and its anchor \
             [{}]",
            Params::default().held,
        ),
    )]
}

#[cfg(test)]
mod tests {
    use super::{Params, reason_of};
    use crate::row_surface::Cell;
    use crate::surface::SurfaceParams;
    use crowbar_ui::components::ContentLength;

    /// `--held` selects between `placeholderReason`'s two reachable arms —
    /// the third (`lastError`, no persisted field, spec §4/B7) is
    /// unreachable, exactly as `inline-error`'s error state is.
    #[test]
    fn held_selects_the_checked_out_reason_and_unheld_the_generic_one() {
        let held = reason_of(true, ContentLength::Normal);
        assert!(held.contains("is checked out at"));
        assert!(held.contains("detach it"));

        let unheld = reason_of(false, ContentLength::Normal);
        assert!(unheld.contains("Retry to provision it"));
        assert!(!unheld.contains("checked out at"));
    }

    /// `--content` moves the interpolated branch in both arms, strictly by
    /// length, and never introduces a fourth string shape.
    #[test]
    fn content_moves_the_interpolated_branch_in_both_arms() {
        for held in [true, false] {
            let short = reason_of(held, ContentLength::Short);
            let normal = reason_of(held, ContentLength::Normal);
            let overflow = reason_of(held, ContentLength::Overflow);
            assert!(short.len() < normal.len(), "held={held}");
            assert!(normal.len() < overflow.len(), "held={held}");
        }
    }

    /// `held` reaches `detach_available` unchanged, and the panel it builds
    /// carries the matching reason arm.
    #[test]
    fn the_panel_gates_detach_on_held() {
        let params = Params { held: true };
        let panel = params.panel(&Cell::default());
        assert!(panel.detach_available);
        assert!(panel.reason.contains("checked out at"));

        let params = Params { held: false };
        let panel = params.panel(&Cell::default());
        assert!(!panel.detach_available);
        assert!(!panel.reason.contains("checked out at"));
    }

    /// `--held` rejects anything but its two words.
    #[test]
    fn held_rejects_a_non_boolean_value() {
        let mut params = Params::default();
        let err = params.accept("--held", &mut vec!["maybe".to_owned()].into_iter());
        assert!(err.is_err());
    }
}
