/// The pixels of a web view at one moment, as premultiplied BGRA rows without padding.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Snapshot {
    /// Width in device pixels.
    pub width: u32,
    /// Height in device pixels.
    pub height: u32,
    /// `width * height * 4` bytes, blue first.
    pub bgra: Vec<u8>,
}

const BYTES_PER_PIXEL: u64 = 4;

impl Snapshot {
    /// A snapshot of `bgra` rows; `None` when the byte count does not match the size.
    #[must_use]
    pub fn new(
        (width, height): (u32, u32),
        bgra: Vec<u8>,
    ) -> Option<Self> {
        let expected = u64::from(width) * u64::from(height) * BYTES_PER_PIXEL;
        (bgra.len() as u64 == expected).then_some(Self {
            width,
            height,
            bgra,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_snapshot_needs_exactly_four_bytes_per_pixel() {
        assert!(Snapshot::new((2, 1), vec![0; 8]).is_some(), "exact");
        assert!(Snapshot::new((2, 1), vec![0; 7]).is_none(), "short");
        assert!(Snapshot::new((2, 1), vec![0; 12]).is_none(), "long");
    }
}
