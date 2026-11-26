//! Snapshots of a GTK web view: the pixels of its surface without the rows' stride padding.

use gtk::cairo::{Format, ImageSurface, Surface};

use crate::view::Snapshot;

const BYTES_PER_PIXEL: usize = 4;

pub(crate) fn snapshot_of(surface: Surface) -> Option<Snapshot> {
    let mut image = ImageSurface::try_from(surface).ok()?;
    if image.format() != Format::ARgb32 {
        return None;
    }
    let size = (
        usize::try_from(image.width()).ok()?,
        usize::try_from(image.height()).ok()?,
    );
    let stride = usize::try_from(image.stride()).ok()?;
    let data = image.data().ok()?;
    snapshot_of_rows(&data, size, stride)
}

fn snapshot_of_rows(
    data: &[u8],
    (width, height): (usize, usize),
    stride: usize,
) -> Option<Snapshot> {
    let row = width.checked_mul(BYTES_PER_PIXEL)?;
    if stride < row || data.len() < stride.checked_mul(height)? {
        return None;
    }
    let bgra = data
        .chunks(stride)
        .take(height)
        .flat_map(|line| line.iter().take(row))
        .copied()
        .collect();
    Snapshot::new(
        (u32::try_from(width).ok()?, u32::try_from(height).ok()?),
        bgra,
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    const PADDED: &[u8] = &[1, 2, 3, 4, 0, 0, 5, 6, 7, 8, 0, 0];

    #[test]
    fn padding_is_dropped_and_pixels_are_kept() {
        let snapshot = snapshot_of_rows(PADDED, (1, 2), 6).unwrap();
        assert_eq!((snapshot.width, snapshot.height), (1, 2));
        assert_eq!(snapshot.bgra, [1, 2, 3, 4, 5, 6, 7, 8]);
        let mut surface = ImageSurface::create(Format::ARgb32, 3, 2).unwrap();
        {
            let mut data = surface.data().unwrap();
            data[0] = 7;
            let second_row = data.len() / 2;
            data[second_row] = 9;
        }
        let plain: Surface = (*surface).clone();
        drop(surface);
        let snapshot = snapshot_of(plain).unwrap();
        assert_eq!((snapshot.width, snapshot.height), (3, 2), "size");
        assert_eq!(snapshot.bgra.len(), 24, "no padding");
        assert_eq!((snapshot.bgra[0], snapshot.bgra[12]), (7, 9), "rows");
    }

    #[test]
    fn data_that_cannot_hold_the_layout_is_refused() {
        type Case = (&'static str, (usize, usize), usize);
        const CASES: &[Case] = &[
            ("stride narrower than a row", (2, 2), 4),
            ("fewer rows than the height", (1, 3), 6),
            ("overflowing width", (usize::MAX, 1), 4),
        ];
        for (name, size, stride) in CASES {
            assert!(snapshot_of_rows(PADDED, *size, *stride).is_none(), "{name}");
        }
    }
}
