use gtk::cairo::{Format, ImageSurface, Surface};

use crate::bitmap_rows::snapshot_of_rows;
use crate::row_layout::RowLayout;
use crate::snapshot::Snapshot;

pub(crate) fn snapshot_of(surface: Surface) -> Option<Snapshot> {
    let mut image = ImageSurface::try_from(surface).ok()?;
    if image.format() != Format::ARgb32 {
        return None;
    }
    let (width, height) = (
        usize::try_from(image.width()).ok()?,
        usize::try_from(image.height()).ok()?,
    );
    let stride = usize::try_from(image.stride()).ok()?;
    let data = image.data().ok()?;
    snapshot_of_rows(&data, RowLayout::new(width, height, stride))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_rows_lose_their_stride_padding_and_keep_the_pixels() {
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
}
