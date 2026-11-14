use super::*;

#[test]
fn presents_the_id_as_an_xlib_window() {
    let handle = XlibParent(0x42).window_handle().unwrap().as_raw();
    assert!(
        matches!(handle, RawWindowHandle::Xlib(h) if h.window == 0x42),
        "{handle:?}"
    );
}
