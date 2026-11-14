use super::*;

#[test]
fn shares_are_capped_and_leave_the_gutter() {
    type Viewport = (f32, f32);
    const CASES: &[(Viewport, Viewport)] = &[
        ((1200.0, 800.0), (1080.0, 592.0)),
        ((3000.0, 2000.0), (1120.0, 820.0)),
        ((400.0, 300.0), (360.0, 222.0)),
    ];
    let extent = DialogExtent::new(0.9, 0.74, px(1120.0), px(820.0));
    for &((width, height), (expected_width, expected_height)) in CASES {
        let got = extent.size(size(px(width), px(height)));
        assert_eq!(
            (f32::from(got.width), f32::from(got.height)),
            (expected_width, expected_height),
            "{width}x{height}"
        );
    }
}
