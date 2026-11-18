//! Picks the first byte.

pub(crate) fn first(bytes: &[u8]) -> Option<u8> {
    bytes.first().copied()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn reads_the_first_byte() {
        let raw = [7_u8];
        // SAFETY: the array has one element.
        let byte = unsafe { *raw.as_ptr() };
        assert_eq!(first(&raw), Some(byte), "first");
    }
}
