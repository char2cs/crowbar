//! What a window shows on screen, read back from the X server.

use std::time::Instant;

use x11rb::protocol::xproto::{ConnectionExt as _, ImageFormat};
use x11rb::rust_connection::RustConnection;

use super::TestResult;

const ALL_PLANES: u32 = !0;
const BYTES_PER_PIXEL: usize = 4;
const LIT: u8 = 0x80;

/// Script that paints the page black and answers `dark` once the paint reached the screen.
pub(crate) const DARK_PAGE: &str = "document.documentElement.style.background = 'black';
requestAnimationFrame(() => requestAnimationFrame(() => window.ipc.postMessage('dark')))";

/// A rectangle of a window: x, y, width, height.
pub(crate) type Region = (i16, i16, u16, u16);

fn lit_pixels(
    connection: &RustConnection,
    window: u32,
    region: Region,
) -> TestResult<usize> {
    let (x, y, width, height) = region;
    let image = connection
        .get_image(
            ImageFormat::Z_PIXMAP,
            window,
            x,
            y,
            width,
            height,
            ALL_PLANES,
        )?
        .reply()?;
    let lit = image
        .data
        .as_chunks::<BYTES_PER_PIXEL>()
        .0
        .iter()
        .filter(|pixel| pixel.iter().take(3).any(|channel| *channel > LIT))
        .count();
    Ok(lit)
}

/// Whether the count of bright pixels in `region` meets `wanted` before `deadline`.
pub(crate) fn shows_before(
    connection: &RustConnection,
    (window, region): (u32, Region),
    wanted: impl Fn(usize) -> bool,
    deadline: Instant,
) -> TestResult<bool> {
    let seen = std::iter::from_fn(|| {
        (Instant::now() < deadline).then(|| lit_pixels(connection, window, region))
    })
    .find(|lit| lit.as_ref().map_or(true, |count| wanted(*count)));
    Ok(seen.transpose()?.is_some())
}
