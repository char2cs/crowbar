/// Reads the byte a pointer addresses.
///
/// # Safety
///
/// `ptr` must be valid for reads of one byte.
#[allow(
    unsafe_code,
    reason = "reading through a raw pointer is the FFI boundary of this fixture"
)]
pub unsafe fn first_byte(ptr: *const u8) -> u8 {
    // SAFETY: the caller guarantees the pointer is valid for reads of one byte.
    unsafe { *ptr }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn reads_the_byte() {
        let byte = 7_u8;
        // SAFETY: the pointer comes from a live local.
        assert_eq!(unsafe { first_byte(&raw const byte) }, 7);
    }
}
