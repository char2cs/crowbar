//! `--surface label` — one `<Label>`, and the second P3.6 leaf with a captured
//! reference.
//!
//! The default cell is the settings dialog's **Typography** section header,
//! measured live at a 1714px viewport: `80.89 × 16`, `fg #f5f5f5ff`,
//! `CalSansUI` 14/16 at weight 500, `content_sized` and `line_sized`. See
//! `native/mapping/label.md` and `/tmp/p3-ref-label.json`.
//!
//! # `--viewport-width` is the axis that matters here
//!
//! `label.tsx` carries `text-base/4.5 sm:text-sm/4`, so the type step moves at
//! 640px and this is the first P3.6 surface where `--viewport-width` is real
//! rather than vacuous. The trap is documented at
//! `crowbar_ui::primitives::label`: at every real window width the primitive's
//! `sm:` step **beats the call site's `ui-text-sm`**, so the rendered size is
//! 14px and not the 12px the call site's class names.
//!
//! # What each axis can and cannot do here
//!
//! | Axis | Here |
//! |---|---|
//! | `--viewport-width` | **real** — `sm:text-sm/4` against `text-base/4.5`, 14/16 against 16/18 |
//! | `--content` | **real** — the label's width is its run's advance width |
//! | `--theme` | **real**: `text-foreground` differs in the two tables |
//! | `--width` | **vacuous.** The box is `inline-flex` with no authored width and no stretch |
//!
//! # The state axis
//!
//! Five of the six are unmodelled. `label.tsx` has **no interaction rule of any
//! kind** — no `hover:`, no `focus`, no `peer-disabled:`, none of the shadcn
//! label's usual group rules survived into this one. It is a class list and a
//! `useRender`, and there is nothing to disagree about.
//!
//! `empty` is the exception and is a real branch: `<Label>` takes its children
//! from the call site, and one with none paints **no run at all**. That cell
//! also drops the `line_sized` declaration, because `ANCHORS.md` v1.6 makes it
//! valid only on an anchor that carries a `font` — declaring it on a box with no
//! text is a refusal, not a delta.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::primitives::label::{ALL_CALL_SITES, CallSite, Label};
use crowbar_ui::AnchorSink;
use crowbar_ui::surfaces::rows::ContentLength;
use crowbar_ui::primitives::label;
use gpui::{AnyElement, IntoElement as _, ParentElement as _, SharedString, Styled as _, div};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "label",
    root: label::ID_LABEL,
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        // `label.tsx` has no interaction rule of any kind — see the module docs.
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // A 16px line box and `CAPTION_HEIGHT`'s 29, with room for the 18px step
    // below the breakpoint. A floor rather than a ceiling: this surface drives
    // no height, a label's being its own line box.
    min_window_height: 72,
    // A label sits in a settings row — never the window.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--call-site`: the `className` bundle a call site merges over the
    /// primitive's own. See [`CallSite`] — it moves the weight and nothing else.
    pub call_site: CallSite,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            call_site: Label::fixture().call_site,
        }
    }
}

impl Params {
    /// The label this cell describes, built from the live fixture so a bare
    /// `--surface label` renders the label the reference actually has.
    #[must_use]
    pub fn label(&self, cell: &Cell) -> Label {
        Label {
            text: if cell.has(StateFlag::Empty) {
                SharedString::new_static("")
            } else {
                text_of(cell.content)
            },
            call_site: self.call_site,
            breakpoint: cell.breakpoint(),
        }
    }
}

/// The string a content length shows.
///
/// `Typography` is the captured one. The other two are live section headers
/// from the same dialog, so `--content` moves the run's width without inventing
/// a string the product never paints.
fn text_of(content: ContentLength) -> SharedString {
    match content {
        ContentLength::Short => SharedString::new_static("Icons"),
        ContentLength::Normal => SharedString::new_static("Typography"),
        ContentLength::Overflow => SharedString::new_static("Keep Workspaces in Memory"),
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
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** A label's height is its own line box, and no option here
    /// authors one.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        if cell.has(StateFlag::Empty) {
            let _ = write!(
                out,
                " · class {} · empty: no run, so the anchor drops its font group and its \
                 line_sized declaration with it",
                self.call_site.name(),
            );
            return;
        }
        let _ = write!(
            out,
            " · class {} · \"{}\"",
            self.call_site.name(),
            text_of(cell.content),
        );
        match self.call_site {
            CallSite::None => out.push_str(
                " · class: no live call site leaves the primitive's className alone, so \
                 there is no reference",
            ),
            CallSite::SectionHeader => {
                out.push_str(" (settings-section.tsx's header — the captured cell)");
            }
            CallSite::Row => out.push_str(
                " · font-normal: the only visual property any live call site takes off \
                 the primitive",
            ),
        }
    }

    /// The label, inside the flex row that makes it a flex item.
    ///
    /// `RowSurface` draws every surface into a gpui **block** container, and a
    /// label drawn straight into one would be a block-level flex box. Every
    /// live `<Label>` is a flex item — which is also why the reference's
    /// computed `display` is `flex` rather than `inline-flex`, CSS having
    /// blockified it. The row carries no anchor, so it cannot reach a snapshot.
    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        div()
            .flex()
            .flex_row()
            .items_center()
            .child(self.label(cell).render(theme, anchors))
            .into_any_element()
    }
}

/// A call site's `className` bundle.
///
/// **There is deliberately no numeric form**, the line P3.1 drew for
/// `--class-radius`: a knob may supply the same *input* both engines resolve,
/// never the reference's *output*.
fn parse_call_site(raw: &str) -> Result<CallSite, ParseError> {
    ALL_CALL_SITES
        .into_iter()
        .find(|site| site.name() == raw)
        .ok_or_else(|| {
            ParseError::Rejected(format!(
                "--call-site takes one of {}, not {raw}; it names the className bundle a \
                 call site merges, never a pixel value",
                names(ALL_CALL_SITES.into_iter().map(CallSite::name)),
            ))
        })
}

/// A vocabulary as one line, for a usage line and for a rejection.
fn names<I: Iterator<Item = &'static str>>(words: I) -> String {
    words.collect::<Vec<_>>().join(", ")
}

fn options() -> Vec<(String, String)> {
    [(
        "--call-site <name>".to_owned(),
        format!(
            "one of {} — the className bundle a call site merges, never a pixel value; \
             it moves the weight and nothing else [{}]",
            names(ALL_CALL_SITES.into_iter().map(CallSite::name)),
            Label::fixture().call_site.name(),
        ),
    )]
    .into_iter()
    .collect()
}
