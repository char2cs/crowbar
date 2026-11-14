#![allow(unsafe_code, reason = "tests drive the C ABI the way Swift does")]

use super::*;
use crate::frontend_asset::FrontendAsset;
use std::sync::Once;
use tauri::Url;

static INSTALL: Once = Once::new();

fn install_fixture() {
    INSTALL.call_once(|| {
        let dev_url = Url::parse("http://localhost:1420/").expect("fixture url parses");
        Frontend::install(Some(dev_url), |path| {
            (path == "/index.html").then(|| FrontendAsset {
                bytes: b"<html>".to_vec(),
                mime_type: "text/html".to_owned(),
            })
        });
    });
}

#[test]
fn dev_url_is_served_as_a_c_string() {
    install_fixture();
    let url = crowbar_chrome_dev_url();
    assert!(!url.is_null(), "a dev frontend exposes its url");
    // SAFETY: the pointer is non-null and owned by the installed frontend for the process life.
    let url = unsafe { CStr::from_ptr(url) };
    assert_eq!(url.to_bytes(), b"http://localhost:1420", "trailing slash dropped");
}

#[test]
fn asset_round_trips_through_the_c_abi() {
    install_fixture();
    let path = CString::new("/index.html").expect("no interior NUL");
    let mut len = 0usize;
    let mut mime: *mut c_char = null_mut();
    // SAFETY: `path` is NUL-terminated and `len` and `mime` are writable.
    let bytes = unsafe { crowbar_chrome_asset(path.as_ptr(), &raw mut len, &raw mut mime) };
    assert!(!bytes.is_null(), "a known path resolves");
    // SAFETY: the call returned `len` bytes at `bytes`.
    let body = unsafe { std::slice::from_raw_parts(bytes, len) }.to_vec();
    // SAFETY: the call returned a NUL-terminated mime type.
    let mime_type = unsafe { CStr::from_ptr(mime) }.to_owned();
    assert_eq!(body, b"<html>", "body");
    assert_eq!(mime_type.to_bytes(), b"text/html", "mime");
    // SAFETY: exactly the values the call above returned, freed once.
    unsafe { crowbar_chrome_asset_free(bytes, len, mime) };
}

#[test]
fn unknown_asset_is_null() {
    install_fixture();
    let path = CString::new("/missing").expect("no interior NUL");
    let mut len = 0usize;
    let mut mime: *mut c_char = null_mut();
    // SAFETY: `path` is NUL-terminated and `len` and `mime` are writable.
    let bytes = unsafe { crowbar_chrome_asset(path.as_ptr(), &raw mut len, &raw mut mime) };
    assert!(bytes.is_null(), "unknown paths are not served");
}

#[test]
fn fonts_are_served_by_index_and_null_past_the_last() {
    let mut len = 0usize;
    // SAFETY: `len` is writable.
    let first = unsafe { crowbar_chrome_font(0, &raw mut len) };
    assert!(!first.is_null() && len > 0, "the UI font is embedded");
    // SAFETY: `len` is writable.
    let past = unsafe { crowbar_chrome_font(2, &raw mut len) };
    assert!(past.is_null(), "no font past the last");
}
