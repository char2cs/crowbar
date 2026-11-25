//! Gaussian blur of a captured image, as CSS `blur()` draws it.

use image::{Rgba, RgbaImage};

const CHANNELS: usize = 4;
const TAP_RADIUS: usize = 2;
const PASSES: usize = 2;
const SIGMA_PER_PIXEL: usize = 2;

pub(crate) fn blurred(
    image: &RgbaImage,
    sigma: usize,
) -> RgbaImage {
    let factor = u32::try_from((sigma / SIGMA_PER_PIXEL).max(1)).unwrap_or(1);
    let small = shrunk(image, factor);
    let (width, height) = small.dimensions();
    let stride = usize::try_from(width).unwrap_or(0) * CHANNELS;
    let across = (0..PASSES).fold(small.as_raw().clone(), |pixels, _| {
        blur_rows(&pixels, stride)
    });
    let down = (0..PASSES).fold(across, |pixels, _| blur_columns(&pixels, stride));
    RgbaImage::from_raw(width, height, down).unwrap_or(small)
}

fn blur_rows(
    pixels: &[u8],
    stride: usize,
) -> Vec<u8> {
    pixels
        .chunks_exact(stride.max(CHANNELS))
        .flat_map(blur_row)
        .collect()
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
    let row = |offset: usize| {
        let at = (index + offset).saturating_sub(TAP_RADIUS).min(last);
        rows.get(at).copied().unwrap_or(&[])
    };
    let width = row(0).len();
    let sums = (0..=2 * TAP_RADIUS)
        .map(row)
        .fold(vec![0_u32; width], |sums, taps| {
            sums.iter()
                .zip(taps)
                .map(|(sum, tap)| sum + u32::from(*tap))
                .collect()
        });
    sums.into_iter().map(|sum| mean(sum, tap_count())).collect()
}

#[cfg(test)]
mod tests {
    use image::{Rgba, RgbaImage};

    use super::*;

    #[test]
    fn a_flat_image_stays_flat_at_a_smaller_size() {
        let flat = RgbaImage::from_pixel(9, 7, Rgba([40, 80, 120, 255]));
        let expected = RgbaImage::from_pixel(5, 4, Rgba([40, 80, 120, 255]));
        assert_eq!(blurred(&flat, 4), expected);
    }

    const WIDTH: u32 = 400;
    const STEP: f64 = 200.0;
    const CASES: [u32; 3] = [4, 8, 16];

    fn erf(x: f64) -> f64 {
        let t = 1.0 / (1.0 + 0.327_591_1 * x.abs());
        let poly = [
            0.254_829_592,
            -0.284_496_736,
            1.421_413_741,
            -1.453_152_027,
            1.061_405_429,
        ];
        let sum = poly.iter().rev().fold(0.0, |acc, c| acc * t + c) * t;
        (1.0 - sum * (-x * x).exp()).copysign(x)
    }

    fn drawn(
        soft: &RgbaImage,
        x: u32,
    ) -> f64 {
        let scale = f64::from(WIDTH) / f64::from(soft.width());
        let at = ((f64::from(x) + 0.5) / scale - 0.5).max(0.0);
        let last = soft.width() - 1;
        let left = (0..=last).rev().find(|i| f64::from(*i) <= at).unwrap_or(0);
        let right = (left + 1).min(last);
        let red = |x: u32| f64::from(soft.get_pixel(x, 0)[0]);
        let t = (at - f64::from(left)).clamp(0.0, 1.0);
        red(left) * (1.0 - t) + red(right) * t
    }

    fn worst_error(
        soft: &RgbaImage,
        sigma: u32,
    ) -> f64 {
        let middle = f64::from(WIDTH / 2);
        let css = |x: u32| {
            let z = (f64::from(x) + 0.5 - middle) / f64::from(sigma);
            STEP * 0.5 * (1.0 + erf(z / std::f64::consts::SQRT_2))
        };
        (0..WIDTH)
            .map(|x| (drawn(soft, x) - css(x)).abs())
            .fold(0.0, f64::max)
    }

    #[test]
    fn an_edge_drawn_from_the_blur_follows_the_gaussian_css_blur_draws() {
        for sigma in CASES {
            let middle = WIDTH / 2;
            let edge = RgbaImage::from_fn(WIDTH, 4, |x, _| Rgba([200 * u8::from(x >= middle); 4]));
            let soft = blurred(&edge, usize::try_from(sigma).unwrap_or(1));
            let error = worst_error(&soft, sigma);
            assert!(error <= STEP * 0.04, "sigma {sigma}: off by {error}");
        }
    }

    #[test]
    fn a_hard_edge_softens_symmetrically_around_it() {
        let edge = RgbaImage::from_fn(80, 8, |x, _| Rgba([200 * u8::from(x >= 40); 4]));
        let soft = blurred(&edge, 4);
        let red = |x: u32| soft.get_pixel(x, 1)[0];
        let (middle, last) = (soft.width() / 2, soft.width() - 1);
        let (before, after) = (red(middle - 1), red(middle));
        assert!(
            before > 0 && after < 200,
            "the edge blends: {before} {after}"
        );
        let sum = u32::from(before) + u32::from(after);
        assert!(sum.abs_diff(200) <= 2, "around the middle: {sum}");
        assert_eq!((red(0), red(last)), (0, 200), "far from it nothing changes");
    }
}
