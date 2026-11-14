use super::*;

#[test]
fn reports_its_axis() {
    let drag = SashDrag {
        axis: Axis::Horizontal,
    };
    assert_eq!(drag.axis(), Axis::Horizontal);
}
