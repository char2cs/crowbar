//! The driver-backed [`AnchorSink`], and the one thing it cannot express
//! directly: an anchor that is both a painted box and a text run.
//!
//! Compiled for a driver build **and** for `cargo test`, because both need it
//! and a second copy is a second thing to drift. `row_layout.rs` measures the
//! row through this sink under a plain `cargo test --workspace`;
//! `row_snapshot.rs` emits through it behind `--features driver`.

use crowbar_driver::RawAnchor;
use crowbar_ui::components::{AnchorId, AnchorSink};
use gpui::{AnyElement, Div, IntoElement as _, ParentElement as _, SharedString};

/// What [`DriverAnchors::boxed_text`] appends to an id to record the run inside
/// the box under an id of its own.
///
/// The driver's registry **replaces** a record when a second one arrives under
/// the same id — deliberately, so that a subtree prepainted twice in one frame
/// cannot make the snapshot depend on which record the differ happened to read.
/// A box and the run inside it are two records, so they cannot share an id on
/// the way in; [`fold_text_halves`] puts them back together on the way out.
///
/// Not a character an anchor id would ever contain, so a half that escaped the
/// fold would reach the snapshot as an unknown anchor and be rejected by name
/// rather than quietly compared against nothing.
const TEXT_HALF: &str = "\u{a7}text";

/// Wraps a component's elements in the driver's anchor elements.
///
/// The wrappers contribute no taffy node and return `None` from
/// `Element::id()`, so the layout tree with them is the layout tree without
/// them — see `crowbar-driver`'s `src/element.rs`.
pub struct DriverAnchors;

impl AnchorSink for DriverAnchors {
    fn root(&self, id: AnchorId, element: Div) -> AnyElement {
        crowbar_driver::anchor_root(id.id, element).into_any_element()
    }

    fn boxed(&self, id: AnchorId, element: Div) -> AnyElement {
        if id.content_sized {
            crowbar_driver::anchor_content_sized(id.id, element).into_any_element()
        } else {
            crowbar_driver::anchor(id.id, element).into_any_element()
        }
    }

    fn text(&self, id: AnchorId, content: SharedString) -> AnyElement {
        if id.content_sized {
            crowbar_driver::anchor_text_content_sized(id.id, content).into_any_element()
        } else {
            crowbar_driver::anchor_text(id.id, content).into_any_element()
        }
    }

    /// The declaration goes on the **box** half, never the run.
    ///
    /// [`fold_text_halves`] keeps the box's geometry and takes only the run's
    /// `text`, so the box is the record whose `bounds.w` the differ compares —
    /// and it is the box the DOM extractor anchors too. A flag on the run would
    /// be folded away and the anchor would reach the snapshot undeclared, which
    /// is a blind spot that reports nothing.
    fn boxed_text(&self, id: AnchorId, element: Div, content: SharedString) -> AnyElement {
        let run = crowbar_driver::anchor_text(text_half(&id.id), content);
        self.boxed(id, element.child(run))
    }
}

/// The id the run inside a `boxed_text` anchor records itself under.
pub fn text_half(id: &str) -> SharedString {
    SharedString::from(format!("{id}{TEXT_HALF}"))
}

/// Folds every `boxed_text` anchor's two records back into the one anchor
/// `native/oracle/ANCHORS.md` §3 describes.
///
/// The box half keeps its geometry and its paint — `bounds`, `bg`, `radius`,
/// `border` — and gains the run's `text`, `text_width`, `clipped`, `fg` and
/// `font`. That split is not arbitrary: the box is the element the DOM
/// extractor anchors, so its border box is the one the reference reports, while
/// the run is the only thing that knows what was shaped.
///
/// A half whose box never arrived is **promoted**, not dropped. It means the
/// box anchor was not recorded at all, which is a real finding; losing the text
/// with it would turn a loud missing-anchor delta into a quiet one.
pub fn fold_text_halves(records: Vec<RawAnchor>) -> Vec<RawAnchor> {
    let mut folded: Vec<RawAnchor> = Vec::with_capacity(records.len());

    for mut record in records {
        let Some(box_id) = record.id.strip_suffix(TEXT_HALF).map(str::to_owned) else {
            folded.push(record);
            continue;
        };
        let host = folded
            .iter_mut()
            .find(|candidate| *candidate.id == *box_id.as_str());
        if let Some(host) = host {
            host.text = record.text;
        } else {
            record.id = SharedString::from(box_id);
            folded.push(record);
        }
    }

    folded
}

#[cfg(test)]
mod tests {
    use crowbar_driver::{FontFacts, Paint, RawAnchor, TextFacts};
    use gpui::{Hsla, Pixels, SharedString, bounds, point, px, size};

    use super::{fold_text_halves, text_half};

    fn a_box(id: &str, width: f32) -> RawAnchor {
        RawAnchor {
            id: SharedString::from(id.to_owned()),
            bounds: bounds(point(px(10.0), px(20.0)), size(px(width), px(16.0))),
            background: Paint::Solid(Hsla::default()),
            text: None,
            visible: true,
            radius: px(4.0),
            border_width: px(1.0),
            border_color: Some(Hsla::default()),
            // The badge is content-sized on the real row, and the box half is
            // where the declaration lives — see `boxed_text`.
            content_sized: true,
        }
    }

    fn a_run(id: SharedString, width: Pixels) -> RawAnchor {
        RawAnchor {
            id,
            // Deliberately a different box from the host's, so that a fold
            // taking the run's geometry instead of the box's would be visible.
            bounds: bounds(point(px(13.0), px(22.0)), size(px(60.0), px(12.0))),
            background: Paint::None,
            text: Some(TextFacts {
                content: "uncommitted".into(),
                width,
                clipped: false,
                color: Hsla::default(),
                font: FontFacts {
                    size: px(10.0),
                    weight: 500.0,
                    family: "CalSansUI".into(),
                    line_height: px(14.0),
                },
            }),
            visible: true,
            radius: px(0.0),
            border_width: px(0.0),
            border_color: None,
            // Deliberately *not* declared on the run: the fold keeps the box's
            // record, so a declaration here would be thrown away.
            content_sized: false,
        }
    }

    /// The whole point of `boxed_text`: one anchor carrying both halves.
    #[test]
    fn a_boxed_text_anchor_folds_into_one_record_with_both_halves() {
        let folded = fold_text_halves(vec![
            a_box("git-row-badge", 74.11),
            a_run(text_half("git-row-badge"), px(66.11)),
        ]);

        assert_eq!(folded.len(), 1);
        let record = &folded[0];
        assert_eq!(record.id, "git-row-badge");
        // The box half's geometry and paint survive …
        assert_eq!(record.bounds.size.width, px(74.11));
        assert_eq!(record.radius, px(4.0));
        assert_eq!(record.border_width, px(1.0));
        assert_eq!(record.background, Paint::Solid(Hsla::default()));
        // … and the run half's text is now on the same record.
        let text = record.text.as_ref().expect("the badge paints text");
        assert_eq!(text.content, "uncommitted");
        assert_eq!(text.width, px(66.11));
        assert_eq!(text.font.family, "CalSansUI");
        // v1.5: the declaration is the box's and survives the fold. Losing it
        // here would send an undeclared anchor to the differ, which would then
        // compare 75.0 against 74.11 and report a delta nobody can act on.
        assert!(record.content_sized);
    }

    /// The suffix must not reach the snapshot, or the differ rejects the
    /// document for an anchor id that is not in the contract.
    #[test]
    fn no_folded_record_keeps_the_text_half_suffix() {
        let folded = fold_text_halves(vec![
            a_box("git-row-badge", 74.11),
            a_run(text_half("git-row-badge"), px(66.11)),
        ]);

        assert!(!folded.iter().any(|record| record.id.contains("text")));
    }

    /// Order and every other anchor are untouched — the fold must not reshuffle
    /// the pre-order the contract calls for.
    #[test]
    fn anchors_without_a_text_half_pass_through_in_order() {
        let folded = fold_text_halves(vec![
            a_box("git-row-item", 294.0),
            a_box("git-row-badge", 74.11),
            a_run(text_half("git-row-badge"), px(66.11)),
            a_box("git-row-added", 21.0),
        ]);

        let ids: Vec<&str> = folded.iter().map(|record| record.id.as_ref()).collect();
        assert_eq!(ids, ["git-row-item", "git-row-badge", "git-row-added"]);
    }

    /// A missing box half is a real finding. Promoting the run keeps the anchor
    /// present, so the delta reads "this anchor lost its box" rather than "this
    /// anchor vanished".
    #[test]
    fn an_orphaned_text_half_is_promoted_rather_than_dropped() {
        let folded = fold_text_halves(vec![a_run(text_half("git-row-badge"), px(66.11))]);

        assert_eq!(folded.len(), 1);
        assert_eq!(folded[0].id, "git-row-badge");
        assert!(folded[0].text.is_some());
    }
}
