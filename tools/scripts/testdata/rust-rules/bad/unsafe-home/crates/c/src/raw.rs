//! Reads a byte through a raw pointer.

pub(crate) fn first(bytes: &[u8]) -> u8 {
    // SAFETY: the caller passes a non-empty slice.
    unsafe { *bytes.as_ptr() }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn reads_the_first_byte() {
        assert_eq!(first(&[7]), 7, "first");
    }
}
