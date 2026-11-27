use std::ffi::c_char;

#[repr(C)]
pub(crate) struct AssetReply {
    pub(crate) len: usize,
    pub(crate) mime: *mut c_char,
}
