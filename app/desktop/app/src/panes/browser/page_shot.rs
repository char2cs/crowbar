use std::io::Cursor;

use anyhow::Context as _;
use base64::Engine as _;
use base64::engine::general_purpose::STANDARD;
use image::imageops::FilterType;
use image::{ImageFormat, RgbaImage};
use serde_json::{Value, json};
use webview::Snapshot;

const MAX_WIDTH: u32 = 1280;
const OPAQUE: u32 = 255;
const CHANNELS: usize = 4;
const PNG: &str = "png";
const WIDTH: &str = "width";
const HEIGHT: &str = "height";

pub(crate) fn png_of(snapshot: &Snapshot) -> anyhow::Result<Value> {
    let rgba: Vec<u8> = snapshot
        .bgra
        .as_chunks::<CHANNELS>()
        .0
        .iter()
        .copied()
        .flat_map(straight_rgba)
        .collect();
    let (width, height) = (snapshot.width, snapshot.height);
    let mut image = RgbaImage::from_raw(width, height, rgba).with_context(|| {
        format!(
            "the {width}x{height} snapshot does not match its {} bytes",
            snapshot.bgra.len()
        )
    })?;
    if image.width() > MAX_WIDTH {
        let height = scaled_height(image.width(), image.height());
        image = image::imageops::resize(&image, MAX_WIDTH, height, FilterType::Triangle);
    }
    let mut png = Cursor::new(Vec::new());
    image
        .write_to(&mut png, ImageFormat::Png)
        .context("the picture could not be encoded as a PNG")?;
    Ok(json!({
        PNG: STANDARD.encode(png.into_inner()),
        WIDTH: image.width(),
        HEIGHT: image.height(),
    }))
}

fn scaled_height(
    width: u32,
    height: u32,
) -> u32 {
    let scaled = u64::from(height) * u64::from(MAX_WIDTH) / u64::from(width);
    u32::try_from(scaled).unwrap_or(height).max(1)
}

fn straight_rgba([blue, green, red, alpha]: [u8; CHANNELS]) -> [u8; CHANNELS] {
    let alpha_wide = u32::from(alpha);
    let straight = |channel: u8| {
        let lifted = (u32::from(channel) * OPAQUE).checked_div(alpha_wide);
        u8::try_from(lifted.unwrap_or(0).min(OPAQUE)).unwrap_or(u8::MAX)
    };
    [straight(red), straight(green), straight(blue), alpha]
}

#[cfg(test)]
mod tests {
    use super::*;

    fn decoded(shot: &Value) -> RgbaImage {
        let bytes = STANDARD.decode(shot[PNG].as_str().unwrap()).unwrap();
        image::load_from_memory_with_format(&bytes, ImageFormat::Png)
            .unwrap()
            .to_rgba8()
    }

    fn solid(
        width: u32,
        height: u32,
    ) -> Snapshot {
        let bgra = [10, 20, 30, 255].repeat((width * height) as usize);
        Snapshot::new((width, height), bgra).unwrap()
    }

    #[test]
    fn the_picture_keeps_its_colours_and_tells_its_size() {
        let shot = png_of(&solid(2, 1)).unwrap();
        assert_eq!(
            (shot[WIDTH].clone(), shot[HEIGHT].clone()),
            (json!(2), json!(1))
        );
        assert_eq!(
            decoded(&shot).get_pixel(0, 0).0,
            [30, 20, 10, 255],
            "blue first becomes red first"
        );
    }

    #[test]
    fn a_translucent_pixel_is_unpremultiplied_and_a_wide_page_is_scaled_to_the_limit() {
        let half = Snapshot::new((1, 1), vec![50, 100, 150, 128]).unwrap();
        let pixel = decoded(&png_of(&half).unwrap()).get_pixel(0, 0).0;
        assert_eq!(pixel, [255, 199, 99, 128]);
        let wide = png_of(&solid(2560, 400)).unwrap();
        assert_eq!(
            (wide[WIDTH].clone(), wide[HEIGHT].clone()),
            (json!(1280), json!(200))
        );
    }
}
