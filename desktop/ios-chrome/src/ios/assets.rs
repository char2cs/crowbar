//! The app's frontend, for the Swift chrome's second webviews: wry's `tauri://` handler only
//! serves wry's own webview, so Swift serves theirs from here over a C ABI.

use std::ffi::{c_char, CStr, CString};
use std::sync::OnceLock;
use tauri::Asset;
use tauri::Url;

type Resolve = Box<dyn Fn(String) -> Option<Asset> + Send + Sync>;

struct Frontend {
    dev_url: Option<CString>,
    resolve: Resolve,
}

static FRONTEND: OnceLock<Frontend> = OnceLock::new();

pub fn install(
    dev_url: Option<Url>,
    resolve: impl Fn(String) -> Option<Asset> + Send + Sync + 'static,
) {
    let dev_url = dev_url.and_then(|url| CString::new(url.as_str().trim_end_matches('/')).ok());
    let _ = FRONTEND.set(Frontend {
        dev_url,
        resolve: Box::new(resolve),
    });
}

/// The dev server the frontend is proxied from in dev builds, or null when it is embedded.
/// Owned by Rust; valid for the life of the app.
#[no_mangle]
pub extern "C" fn crowbar_chrome_dev_url() -> *const c_char {
    FRONTEND
        .get()
        .and_then(|frontend| frontend.dev_url.as_ref())
        .map_or(std::ptr::null(), |url| url.as_ptr())
}

/// The embedded asset at `path`, or null. The caller frees the bytes and the mime string with
/// `crowbar_chrome_asset_free`.
///
/// # Safety
/// `path` is a NUL-terminated string; `len` and `mime` are writable.
#[no_mangle]
pub unsafe extern "C" fn crowbar_chrome_asset(
    path: *const c_char,
    len: *mut usize,
    mime: *mut *mut c_char,
) -> *mut u8 {
    let Some(frontend) = FRONTEND.get() else {
        return std::ptr::null_mut();
    };
    let path = CStr::from_ptr(path).to_string_lossy().into_owned();
    let Some(asset) = (frontend.resolve)(path) else {
        return std::ptr::null_mut();
    };
    let mime_type = CString::new(asset.mime_type).unwrap_or_default();
    let bytes = asset.bytes.into_boxed_slice();
    *len = bytes.len();
    *mime = mime_type.into_raw();
    Box::into_raw(bytes).cast()
}

/// # Safety
/// Exactly the values one `crowbar_chrome_asset` call returned, freed once.
#[no_mangle]
pub unsafe extern "C" fn crowbar_chrome_asset_free(bytes: *mut u8, len: usize, mime: *mut c_char) {
    drop(Box::from_raw(std::ptr::slice_from_raw_parts_mut(
        bytes, len,
    )));
    drop(CString::from_raw(mime));
}

/// The web's UI font (`web/public/fonts/CalSansUI.woff2`) and heading font (`@fontsource/
/// instrument-serif`, latin 400) as the TrueType CoreText loads: it cannot read woff2, and a Swift
/// package's resources are not carried into the app.
static FONTS: [&[u8]; 2] = [
    include_bytes!("../../assets/CalSansUI.ttf"),
    include_bytes!("../../assets/InstrumentSerif.ttf"),
];

/// Font file `index`'s bytes (0 the UI font, 1 the heading font), valid for the life of the app
/// and never freed; null past the last.
///
/// # Safety
/// `len` is writable.
#[no_mangle]
pub unsafe extern "C" fn crowbar_chrome_font(index: usize, len: *mut usize) -> *const u8 {
    let Some(font) = FONTS.get(index) else {
        return std::ptr::null();
    };
    *len = font.len();
    font.as_ptr()
}
