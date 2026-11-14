use super::*;

#[test]
fn keys_are_lowercased_and_modifiers_queryable() {
    let pressed = Pressed::new("ArrowLeft", &[Modifier::Alt]);
    assert_eq!(pressed.key, "arrowleft");
    assert!(pressed.has(Modifier::Alt));
    assert!(!pressed.has(Modifier::Shift));
}
