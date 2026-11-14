use super::*;

#[test]
fn edges_map_to_axis_and_order() {
    let cases = [
        (Edge::Left, Axis::Horizontal, true),
        (Edge::Right, Axis::Horizontal, false),
        (Edge::Top, Axis::Vertical, true),
        (Edge::Bottom, Axis::Vertical, false),
    ];
    for (edge, axis, first) in cases {
        assert_eq!(edge.axis(), axis, "{edge:?} axis");
        assert_eq!(edge.new_pane_first(), first, "{edge:?} order");
    }
}
