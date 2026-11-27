#[derive(Debug, Clone, Copy)]
pub(crate) struct RowLayout {
    pub(crate) width: usize,
    pub(crate) height: usize,
    pub(crate) stride: usize,
}

impl RowLayout {
    pub(crate) const fn new(
        width: usize,
        height: usize,
        stride: usize,
    ) -> Self {
        Self {
            width,
            height,
            stride,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_layout_keeps_the_size_and_the_stride_apart() {
        let layout = RowLayout::new(3, 2, 16);
        assert_eq!((layout.width, layout.height, layout.stride), (3, 2, 16));
    }
}
