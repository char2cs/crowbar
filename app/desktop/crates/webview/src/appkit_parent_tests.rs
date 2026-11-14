use super::*;

#[test]
fn presents_the_view_as_an_appkit_window() {
    let view = NonNull::<u8>::dangling().cast();
    let handle = AppKitParent(view).window_handle().unwrap().as_raw();
    assert!(
        matches!(handle, RawWindowHandle::AppKit(h) if h.ns_view == view),
        "{handle:?}"
    );
}
