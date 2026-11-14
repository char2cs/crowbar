use super::*;

#[test]
fn every_binding_is_scoped_to_the_input() {
    let bindings = bindings();
    assert_eq!(bindings.len(), 14);
    assert!(bindings.iter().all(|binding| binding.predicate().is_some()));
}
