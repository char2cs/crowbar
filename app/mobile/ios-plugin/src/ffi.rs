#![allow(
    unsafe_code,
    reason = "the C ABI Swift calls into is the plugin's one unsafe boundary"
)]

use crate::font::font;
use crate::frontend::Frontend;
use std::ffi::{c_char, CStr, CString};
use std::ptr::{null, null_mut};

#[no_mangle]
pub(crate) extern "C" fn crowbar_chrome_dev_url() -> *const c_char {
    Frontend::installed()
        .and_then(Frontend::dev_url)
        .map_or(null(), CStr::as_ptr)
}

#[no_mangle]
pub(crate) unsafe extern "C" fn crowbar_chrome_asset(
    path: *const c_char,
    len: *mut usize,
    mime: *mut *mut c_char,
) -> *mut u8 {
    let Some(frontend) = Frontend::installed() else {
        return null_mut();
    };
    // SAFETY: the caller passes a NUL-terminated string that outlives this call.
    let path = unsafe { CStr::from_ptr(path) }
        .to_string_lossy()
        .into_owned();
    let Some(asset) = frontend.resolve(path) else {
        return null_mut();
    };
    let Ok(mime_type) = CString::new(asset.mime_type) else {
        return null_mut();
    };
    let bytes = asset.bytes.into_boxed_slice();
    // SAFETY: the caller passes a writable `usize`.
    unsafe { *len = bytes.len() };
    // SAFETY: the caller passes a writable pointer slot.
    unsafe { *mime = mime_type.into_raw() };
    Box::into_raw(bytes).cast()
}

#[no_mangle]
pub(crate) unsafe extern "C" fn crowbar_chrome_asset_free(
    bytes: *mut u8,
    len: usize,
    mime: *mut c_char,
) {
    // SAFETY: `bytes` and `len` are exactly what one `crowbar_chrome_asset` call returned, freed once.
    drop(unsafe { Box::from_raw(std::ptr::slice_from_raw_parts_mut(bytes, len)) });
    // SAFETY: `mime` is the pointer that same call returned, freed once.
    drop(unsafe { CString::from_raw(mime) });
}

#[no_mangle]
pub(crate) unsafe extern "C" fn crowbar_chrome_font(
    index: usize,
    len: *mut usize,
) -> *const u8 {
    let Some(bytes) = font(index) else {
        return null();
    };
    // SAFETY: the caller passes a writable `usize`.
    unsafe { *len = bytes.len() };
    bytes.as_ptr()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::frontend_asset::FrontendAsset;
    use std::sync::Once;
    use tauri::Url;

    static INSTALL: Once = Once::new();

    fn fixture_asset(path: &str) -> Option<FrontendAsset> {
        (path == "/index.html").then(|| FrontendAsset {
            bytes: b"<html>".to_vec(),
            mime_type: "text/html".to_owned(),
        })
    }

    fn install_fixture() {
        INSTALL.call_once(|| {
            let dev_url = Url::parse("http://localhost:1420/").expect("fixture url parses");
            Frontend::install(Some(dev_url), |path| fixture_asset(&path));
        });
    }

    type Served = (Vec<u8>, Vec<u8>);
    type Expected = Option<(&'static [u8], &'static [u8])>;

    fn fetch(path: &str) -> Option<Served> {
        install_fixture();
        let path = CString::new(path).expect("no interior NUL");
        let mut len = 0usize;
        let mut mime: *mut c_char = null_mut();
        // SAFETY: `path` is NUL-terminated and `len` and `mime` are writable.
        let bytes = unsafe { crowbar_chrome_asset(path.as_ptr(), &raw mut len, &raw mut mime) };
        if bytes.is_null() {
            return None;
        }
        // SAFETY: the call returned `len` bytes at `bytes`.
        let body = unsafe { std::slice::from_raw_parts(bytes, len) }.to_vec();
        // SAFETY: the call returned a NUL-terminated mime type.
        let mime_type = unsafe { CStr::from_ptr(mime) }.to_bytes().to_vec();
        // SAFETY: exactly the values the call above returned, freed once.
        unsafe { crowbar_chrome_asset_free(bytes, len, mime) };
        Some((body, mime_type))
    }

    #[test]
    fn dev_url_is_served_as_a_c_string() {
        install_fixture();
        let url = crowbar_chrome_dev_url();
        assert!(!url.is_null(), "a dev frontend exposes its url");
        // SAFETY: the pointer is non-null and owned by the installed frontend for the process life.
        let url = unsafe { CStr::from_ptr(url) };
        assert_eq!(
            url.to_bytes(),
            b"http://localhost:1420",
            "trailing slash dropped"
        );
    }

    #[test]
    fn assets_round_trip_through_the_c_abi_and_unknown_paths_are_null() {
        const CASES: &[(&str, Expected)] = &[
            ("/index.html", Some((b"<html>", b"text/html"))),
            ("/missing", None),
        ];
        for (path, expected) in CASES {
            let served = fetch(path);
            let served = served
                .as_ref()
                .map(|(body, mime)| (body.as_slice(), mime.as_slice()));
            assert_eq!(served, *expected, "asset for {path}");
        }
    }

    #[test]
    fn fonts_are_served_by_index_and_null_past_the_last() {
        const CASES: &[(usize, bool)] = &[(0, true), (1, true), (2, false)];
        for (index, served) in CASES {
            let mut len = 0usize;
            // SAFETY: `len` is writable.
            let bytes = unsafe { crowbar_chrome_font(*index, &raw mut len) };
            assert_eq!(!bytes.is_null() && len > 0, *served, "font {index}");
        }
    }
}
