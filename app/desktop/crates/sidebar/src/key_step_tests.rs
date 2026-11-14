use super::*;

#[test]
fn shift_quadruples_the_step() {
    assert!((KeyStep::Fine.pixels() - 16.0).abs() < f32::EPSILON, "fine");
    assert!(
        (KeyStep::Coarse.pixels() - 64.0).abs() < f32::EPSILON,
        "coarse"
    );
}
