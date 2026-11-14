use super::*;

#[test]
fn bindings_cover_the_four_actions_in_the_menu_context() {
    let bindings = bindings();
    assert_eq!(bindings.len(), 4);
    assert!(bindings.iter().all(|binding| binding.predicate().is_some()));
}
