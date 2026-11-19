//! Gaussian blur of a captured image, as CSS `blur()` draws it.

use image::{Rgba, RgbaImage};

const CHANNELS: usize = 4;
const TAP_RADIUS: usize = 1;

pub(crate) fn blurred(
    image: &RgbaImage,
    sigma: usize,
) -> RgbaImage {
    let factor = u32::try_from(sigma.max(1)).unwrap_or(1);
    let small = shrunk(image, factor);
    let (width, height) = small.dimensions();
    let stride = usize::try_from(width).unwrap_or(0) * CHANNELS;
    let across: Vec<u8> = small
        .as_raw()
        .chunks_exact(stride.max(CHANNELS))
        .flat_map(blur_row)
        .collect();
    let down = blur_columns(&across, stride);
    RgbaImage::from_raw(width, height, down).unwrap_or(small)
}

fn shrunk(
    image: &RgbaImage,
    factor: u32,
) -> RgbaImage {
    let (width, height) = image.dimensions();
    let size = (
        width.div_ceil(factor).max(1),
        height.div_ceil(factor).max(1),
    );
    RgbaImage::from_fn(size.0, size.1, |x, y| block_mean(image, (x, y), factor))
}

fn block_mean(
    image: &RgbaImage,
    (x, y): (u32, u32),
    factor: u32,
) -> Rgba<u8> {
    let (width, height) = image.dimensions();
    let columns = (x * factor)..((x + 1) * factor).min(width);
    let rows = (y * factor)..((y + 1) * factor).min(height);
    let block = rows.flat_map(|row| columns.clone().map(move |column| (column, row)));
    let (sums, count) = block.fold(([0_u32; CHANNELS], 0_u32), |(sums, count), (x, y)| {
        (added(sums, image.get_pixel(x, y).0), count + 1)
    });
    Rgba(sums.map(|sum| mean(sum, count)))
}

fn added(
    sums: [u32; CHANNELS],
    pixel: [u8; CHANNELS],
) -> [u32; CHANNELS] {
    std::array::from_fn(|channel| {
        let sum = sums.get(channel).copied().unwrap_or(0);
        sum + u32::from(pixel.get(channel).copied().unwrap_or(0))
    })
}

fn mean(
    sum: u32,
    count: u32,
) -> u8 {
    u8::try_from((sum + count / 2) / count.max(1)).unwrap_or(u8::MAX)
}

fn tap_count() -> u32 {
    u32::try_from(2 * TAP_RADIUS + 1).unwrap_or(1)
}

fn blur_row(row: &[u8]) -> Vec<u8> {
    let (pixels, _) = row.as_chunks::<CHANNELS>();
    (0..pixels.len())
        .flat_map(|index| row_taps(pixels, index))
        .collect()
}

fn row_taps(
    pixels: &[[u8; CHANNELS]],
    index: usize,
) -> [u8; CHANNELS] {
    let last = pixels.len().saturating_sub(1);
    let at = |offset: usize| {
        let at = (index + offset).saturating_sub(TAP_RADIUS).min(last);
        pixels.get(at).copied().unwrap_or([0; CHANNELS])
    };
    (0..=2 * TAP_RADIUS)
        .map(at)
        .fold([0; CHANNELS], added)
        .map(|sum| mean(sum, tap_count()))
}

fn blur_columns(
    pixels: &[u8],
    stride: usize,
) -> Vec<u8> {
    let rows: Vec<&[u8]> = pixels.chunks_exact(stride.max(CHANNELS)).collect();
    (0..rows.len())
        .flat_map(|index| column_taps(&rows, index))
        .collect()
}

fn column_taps(
    rows: &[&[u8]],
    index: usize,
) -> Vec<u8> {
    let last = rows.len().saturating_sub(1);
    let row = |index: usize| rows.get(index.min(last)).copied().unwrap_or(&[]);
    let (above, here) = (row(index.saturating_sub(TAP_RADIUS)), row(index));
    let below = row(index + TAP_RADIUS);
    above
        .iter()
        .zip(here)
        .zip(below)
        .map(|((up, at), down)| {
            mean(
                u32::from(*up) + u32::from(*at) + u32::from(*down),
                tap_count(),
            )
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use image::{Rgba, RgbaImage};

    use super::*;

    #[test]
    fn a_flat_image_stays_flat_at_a_smaller_size() {
        let flat = RgbaImage::from_pixel(9, 7, Rgba([40, 80, 120, 255]));
        let expected = RgbaImage::from_pixel(3, 2, Rgba([40, 80, 120, 255]));
        assert_eq!(blurred(&flat, 4), expected);
    }

    #[test]
    fn a_hard_edge_softens_symmetrically_around_it() {
        let edge = RgbaImage::from_fn(80, 8, |x, _| Rgba([200 * u8::from(x >= 40); 4]));
        let soft = blurred(&edge, 4);
        let red = |x: u32| soft.get_pixel(x, 1)[0];
        assert!(
            red(9) > 0 && red(10) < 200,
            "the edge blends: {} {}",
            red(9),
            red(10)
        );
        let sum = u32::from(red(9)) + u32::from(red(10));
        assert!(sum.abs_diff(200) <= 2, "around the middle: {sum}");
        assert_eq!((red(0), red(19)), (0, 200), "far from it nothing changes");
    }
}
