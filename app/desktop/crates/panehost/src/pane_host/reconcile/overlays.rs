use webview::Bounds;

use crate::pane_host::reconcile::covered::{Covered, meet};

#[derive(Debug, Clone, Default, PartialEq)]
pub struct Overlays {
    pub covered: Covered,
    pub cutout: Option<Bounds>,
}

impl Overlays {
    pub(crate) fn cut_for(
        &self,
        pane: &Bounds,
    ) -> Option<Bounds> {
        let hole = self.cutout.filter(|hole| meet(pane, hole))?;
        let (left, top) = (hole.x.max(pane.x), hole.y.max(pane.y));
        let right = (hole.x + hole.width).min(pane.x + pane.width);
        let bottom = (hole.y + hole.height).min(pane.y + pane.height);
        Some(Bounds {
            x: left - pane.x,
            y: top - pane.y,
            width: right - left,
            height: bottom - top,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const PANE: Bounds = Bounds {
        x: 100.0,
        y: 40.0,
        width: 200.0,
        height: 300.0,
    };

    fn rect(
        (x, y): (f64, f64),
        (width, height): (f64, f64),
    ) -> Bounds {
        Bounds {
            x,
            y,
            width,
            height,
        }
    }

    fn handle(at: (f64, f64)) -> Overlays {
        Overlays {
            cutout: Some(rect(at, (69.0, 21.0))),
            ..Overlays::default()
        }
    }

    #[test]
    fn the_handle_is_cut_where_it_meets_a_pane_relative_to_the_pane() {
        let cases = [
            (
                "hanging from the top edge",
                handle((165.0, 30.0)),
                Some(rect((65.0, 0.0), (69.0, 11.0))),
            ),
            (
                "past the right edge",
                handle((260.0, 40.0)),
                Some(rect((160.0, 0.0), (40.0, 21.0))),
            ),
            ("beside the pane", handle((400.0, 40.0)), None),
            ("no handle", Overlays::default(), None),
        ];
        for (what, overlays, expected) in cases {
            assert_eq!(overlays.cut_for(&PANE), expected, "{what}");
        }
    }
}
