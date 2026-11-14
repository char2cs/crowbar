#![allow(unsafe_code, reason = "the C ABI Swift calls into is the plugin's one unsafe boundary")]

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
    let path = unsafe { CStr::from_ptr(path) }.to_string_lossy().into_owned();
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
pub(crate) unsafe extern "C" fn crowbar_chrome_font(index: usize, len: *mut usize) -> *const u8 {
    let Some(bytes) = font(index) else {
        return null();
    };
    // SAFETY: the caller passes a writable `usize`.
    unsafe { *len = bytes.len() };
    bytes.as_ptr()
}

#[cfg(test)]
#[path = "ffi_tests.rs"]
mod tests;
