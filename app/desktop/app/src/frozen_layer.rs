use std::sync::Arc;

use gpui::{
    AnyElement, IntoElement as _, ObjectFit, ParentElement as _, Pixels, RenderImage, Styled,
    StyledImage as _, div, img, px,
};
use image::{Frame, RgbaImage};
use pane_host::{FrozenPane, VisiblePane};
use smallvec::SmallVec;
use webview::{Bounds, Snapshot};

#[derive(Default)]
pub(crate) struct FrozenLayer {
    holes: Vec<VisiblePane>,
    shots: Vec<(Bounds, Arc<RenderImage>)>,
}

impl FrozenLayer {
    pub(crate) fn set_holes(
        &mut self,
        holes: &[VisiblePane],
    ) {
        self.holes = holes.to_vec();
    }

    pub(crate) fn replace(
        &mut self,
        frozen: Vec<FrozenPane>,
    ) {
        self.shots = frozen
            .into_iter()
            .filter_map(|shot| {
                let hole = self.holes.iter().find(|hole| hole.pane == shot.pane)?;
                Some((hole.bounds, image_of(shot.snapshot)?))
            })
            .collect();
    }

    pub(crate) fn elements(&self) -> Vec<AnyElement> {
        self.shots
            .iter()
            .map(|(bounds, image)| {
                div()
                    .absolute()
                    .left(logical(bounds.x))
                    .top(logical(bounds.y))
                    .w(logical(bounds.width))
                    .h(logical(bounds.height))
                    .child(
                        img(Arc::clone(image))
                            .size_full()
                            .object_fit(ObjectFit::Fill),
                    )
                    .into_any_element()
            })
            .collect()
    }
}

#[expect(
    clippy::cast_possible_truncation,
    reason = "window coordinates fit an f32"
)]
fn logical(value: f64) -> Pixels {
    px(value as f32)
}

fn image_of(snapshot: Snapshot) -> Option<Arc<RenderImage>> {
    let pixels = RgbaImage::from_raw(snapshot.width, snapshot.height, snapshot.bgra)?;
    let frames: SmallVec<[Frame; 1]> = SmallVec::from_elem(Frame::new(pixels), 1);
    Some(Arc::new(RenderImage::new(frames)))
}

#[cfg(test)]
mod tests {
    use protocol::PaneId;

    use super::*;

    const BOUNDS: Bounds = Bounds {
        x: 10.0,
        y: 20.0,
        width: 30.0,
        height: 40.0,
    };

    fn hole(pane: &str) -> VisiblePane {
        VisiblePane {
            pane: PaneId::new(pane).unwrap(),
            bounds: BOUNDS,
        }
    }

    fn shot(pane: &str) -> FrozenPane {
        FrozenPane {
            pane: PaneId::new(pane).unwrap(),
            snapshot: Snapshot::new((1, 1), vec![0; 4]).unwrap(),
        }
    }

    #[test]
    fn only_panes_that_still_have_a_hole_are_drawn_and_a_release_clears_them() {
        let mut layer = FrozenLayer::default();
        layer.set_holes(&[hole("a")]);
        layer.replace(vec![shot("a"), shot("gone")]);
        assert_eq!(layer.elements().len(), 1, "the pane with a hole");
        layer.replace(Vec::new());
        assert!(layer.elements().is_empty(), "released");
    }
}
