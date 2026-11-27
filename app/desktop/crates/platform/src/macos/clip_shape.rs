use objc2_core_foundation::{CFRetained, CGPoint, CGRect, CGSize};
use objc2_core_graphics::CGMutablePath;

pub(crate) const CORNERS: usize = 4;

const HOLE_RADIUS: f64 = 8.0;

#[derive(Debug, Clone, Copy, PartialEq, Default)]
pub struct ClipShape {
    pub corners: [f64; CORNERS],
    pub hole: Option<[f64; CORNERS]>,
}

impl ClipShape {
    pub(crate) fn cuts(
        self,
        point: CGPoint,
    ) -> bool {
        self.hole.is_some_and(|hole| {
            let hole = rect_of(hole);
            let across = hole.origin.x..hole.origin.x + hole.size.width;
            let down = hole.origin.y..hole.origin.y + hole.size.height;
            across.contains(&point.x) && down.contains(&point.y)
        })
    }

    pub(crate) fn outline(
        self,
        size: CGSize,
    ) -> CFRetained<CGMutablePath> {
        let path = CGMutablePath::new();
        rounded(&path, CGRect::new(CGPoint::ZERO, size), self.corners);
        if let Some(hole) = self.hole {
            let bottom = [0.0, 0.0, HOLE_RADIUS, HOLE_RADIUS];
            rounded(&path, rect_of(hole), bottom);
        }
        path
    }
}

fn rect_of(frame: [f64; CORNERS]) -> CGRect {
    let [x, y, width, height] = frame;
    CGRect::new(CGPoint::new(x, y), CGSize::new(width, height))
}

#[expect(
    unsafe_code,
    reason = "CGPathMoveToPoint takes a nullable matrix pointer"
)]
fn rounded(
    path: &CGMutablePath,
    rect: CGRect,
    radii: [f64; CORNERS],
) {
    let [top_left, top_right, bottom_right, bottom_left] = radii;
    let (x, y) = (rect.origin.x, rect.origin.y);
    let (right, bottom) = (x + rect.size.width, y + rect.size.height);
    let at = CGPoint::new;
    // SAFETY: a null matrix means no transform.
    unsafe { CGMutablePath::move_to_point(Some(path), std::ptr::null(), x + top_left, y) };
    arc(path, (at(right, y), at(right, bottom)), top_right);
    arc(path, (at(right, bottom), at(x, bottom)), bottom_right);
    arc(path, (at(x, bottom), at(x, y)), bottom_left);
    arc(path, (at(x, y), at(right, y)), top_left);
    CGMutablePath::close_subpath(Some(path));
}

#[expect(
    unsafe_code,
    reason = "CGPathAddArcToPoint takes a nullable matrix pointer"
)]
fn arc(
    path: &CGMutablePath,
    ends: (CGPoint, CGPoint),
    radius: f64,
) {
    let (corner, next) = ends;
    let none = std::ptr::null();
    // SAFETY: a null matrix means no transform.
    unsafe {
        CGMutablePath::add_arc_to_point(
            Some(path),
            none,
            corner.x,
            corner.y,
            next.x,
            next.y,
            radius,
        );
    }
}

#[cfg(test)]
mod tests {
    use objc2_core_graphics::CGPath;

    use super::*;

    #[expect(
        unsafe_code,
        reason = "CGPathContainsPoint takes a nullable matrix pointer"
    )]
    fn shows(
        path: &CGMutablePath,
        (x, y): (f64, f64),
    ) -> bool {
        // SAFETY: a null matrix means no transform.
        unsafe { CGPath::contains_point(Some(path), std::ptr::null(), CGPoint::new(x, y), true) }
    }

    #[test]
    fn the_outline_leaves_out_the_rounded_corners_and_the_hole() {
        let shape = ClipShape {
            corners: [10.0, 0.0, 0.0, 10.0],
            hole: Some([60.0, 0.0, 70.0, 20.0]),
        };
        let path = shape.outline(CGSize::new(200.0, 100.0));
        let cases = [
            ("the middle", (100.0, 50.0), true),
            ("a rounded corner", (1.0, 1.0), false),
            ("a square corner", (199.0, 1.0), true),
            ("the bottom rounded corner", (1.0, 99.0), false),
            ("the hole", (90.0, 5.0), false),
            ("the hole's rounded corner", (61.0, 19.0), true),
        ];
        for (name, point, want) in cases {
            assert_eq!(shows(&path, point), want, "{name}");
        }
        assert!(shape.cuts(CGPoint::new(90.0, 5.0)), "the hole");
        assert!(!shape.cuts(CGPoint::new(90.0, 25.0)), "below it");
    }
}
