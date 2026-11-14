struct Holder {
    _callback: Box<dyn Fn()>,
}

opaque_debug!(Holder);

#[test]
fn prints_only_the_type_name() {
    let holder = Holder {
        _callback: Box::new(|| {}),
    };
    assert_eq!(format!("{holder:?}"), "Holder { .. }");
}
