use super::*;

#[test]
fn chat_is_registered_once() {
    let mut registry = PaneRegistry::default();
    register_core_panes(&mut registry, "chat/index.html").unwrap();
    assert!(
        register_core_panes(&mut registry, "chat/index.html").is_err(),
        "second registration is refused"
    );
    assert!(registry.get(&chat_type().unwrap()).is_some(), "registered");
}

#[test]
fn a_chat_is_identified_by_its_thread() {
    let params = chat_params("t1").unwrap();
    assert_eq!(params.get("threadId"), Some("t1"), "thread id");
}
