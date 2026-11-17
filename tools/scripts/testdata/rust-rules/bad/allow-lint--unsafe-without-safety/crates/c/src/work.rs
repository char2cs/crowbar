/// Reads the byte a pointer addresses.
///
/// # Safety
///
/// `ptr` must be valid for reads.
#[allow(unsafe_code, reason = "reading through a raw pointer is the boundary")]
pub unsafe fn work(ptr: *const u8) -> u8 {
    unsafe { *ptr }
}

#[cfg(test)]
mod tests {
    #[test]
    fn a() {}
}
