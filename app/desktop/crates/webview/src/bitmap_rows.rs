use crate::row_layout::RowLayout;
use crate::snapshot::Snapshot;

const BYTES_PER_PIXEL: usize = 4;

pub(crate) fn snapshot_of_rows(
    data: &[u8],
    layout: RowLayout,
) -> Option<Snapshot> {
    let row = layout.width.checked_mul(BYTES_PER_PIXEL)?;
    if layout.stride < row || data.len() < layout.stride.checked_mul(layout.height)? {
        return None;
    }
    let bgra = data
        .chunks(layout.stride)
        .take(layout.height)
        .flat_map(|line| line.iter().take(row))
        .copied()
        .collect();
    Snapshot::new(
        (
            u32::try_from(layout.width).ok()?,
            u32::try_from(layout.height).ok()?,
        ),
        bgra,
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    const PADDED: &[u8] = &[1, 2, 3, 4, 0, 0, 5, 6, 7, 8, 0, 0];

    #[test]
    fn padding_is_dropped_and_pixels_are_kept() {
        let layout = RowLayout::new(1, 2, 6);
        let snapshot = snapshot_of_rows(PADDED, layout).unwrap();
        assert_eq!((snapshot.width, snapshot.height), (1, 2));
        assert_eq!(snapshot.bgra, [1, 2, 3, 4, 5, 6, 7, 8]);
    }

    #[test]
    fn data_that_cannot_hold_the_layout_is_refused() {
        const CASES: &[(&str, RowLayout)] = &[
            ("stride narrower than a row", RowLayout::new(2, 2, 4)),
            ("fewer rows than the height", RowLayout::new(1, 3, 6)),
            ("overflowing width", RowLayout::new(usize::MAX, 1, 4)),
        ];
        for (name, layout) in CASES {
            assert!(snapshot_of_rows(PADDED, *layout).is_none(), "{name}");
        }
    }
}
