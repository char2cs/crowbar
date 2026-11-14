use super::*;

#[test]
fn settled_is_not_running() {
    assert_eq!(
        Position::settled(0.5),
        Position {
            value: 0.5,
            running: false
        }
    );
}
