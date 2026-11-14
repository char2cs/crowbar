use super::*;

#[test]
fn flipping_twice_returns_the_start() {
    for start in [Visibility::Open, Visibility::Closed] {
        assert_ne!(start.flipped(), start, "flip changes the value");
        assert_eq!(start.flipped().flipped(), start, "flip is an involution");
    }
}
