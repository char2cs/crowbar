//! Places the shell's pop-up window (tooltips, toasts) above its web views: over the platform's
//! own X connection on Linux, as a child window of its owner that lets clicks through on macOS.

use raw_window_handle::RawWindowHandle;

struct Float(platform::FloatPlacer);

pub(crate) fn install(cx: &mut gpui::App) {
    match platform::FloatPlacer::connect() {
        Ok(placer) => ui::install_float_host(Float(placer), cx),
        Err(error) => tracing::warn!(%error, "floating elements paint under the web views"),
    }
}

#[cfg(target_os = "linux")]
impl ui::FloatHost for Float {
    fn cover(
        &self,
        float: RawWindowHandle,
        owner: RawWindowHandle,
    ) {
        logged(self.0.cover(float, owner));
    }

    fn wake(
        &self,
        float: RawWindowHandle,
    ) {
        logged(self.0.wake(float));
    }

    fn shape(
        &self,
        float: RawWindowHandle,
        seen: &[ui::FloatRegion],
        hit: &[ui::FloatRegion],
    ) {
        let (seen, hit) = (cards(seen), cards(hit));
        logged(self.0.shape(float, &seen, &hit));
    }
}

// PLATFORM: macos AppKit draws the pop-up window whatever it shows and it lets every click through, so it needs no waking and no shape.
#[cfg(target_os = "macos")]
impl ui::FloatHost for Float {
    fn cover(
        &self,
        float: RawWindowHandle,
        owner: RawWindowHandle,
    ) {
        logged(self.0.cover(float, owner));
    }

    fn wake(
        &self,
        _: RawWindowHandle,
    ) {
    }

    fn shape(
        &self,
        _: RawWindowHandle,
        _: &[ui::FloatRegion],
        _: &[ui::FloatRegion],
    ) {
    }
}

#[cfg(target_os = "linux")]
fn cards(regions: &[ui::FloatRegion]) -> Vec<platform::Card> {
    regions.iter().map(card).collect()
}

#[cfg(target_os = "linux")]
fn card(region: &ui::FloatRegion) -> platform::Card {
    let bounds = region.bounds;
    platform::Card {
        x: offset(bounds.origin.x.0),
        y: offset(bounds.origin.y.0),
        width: extent(bounds.size.width.0),
        height: extent(bounds.size.height.0),
        radius: extent(region.radius.0),
    }
}

#[cfg(target_os = "linux")]
fn offset(value: i32) -> i16 {
    let limit = i32::from(i16::MAX);
    i16::try_from(value.clamp(-limit, limit)).unwrap_or_default()
}

#[cfg(target_os = "linux")]
fn extent(value: i32) -> u16 {
    u16::try_from(value.clamp(0, i32::from(u16::MAX))).unwrap_or_default()
}

fn logged(done: Result<(), platform::Error>) {
    if let Err(error) = done {
        tracing::warn!(%error, "the pop-up window could not be placed");
    }
}

#[cfg(test)]
mod tests {
    #![cfg(target_os = "linux")]

    use gpui::{Bounds, DevicePixels, point, size};

    use super::*;

    type Rect = (i32, i32, i32, i32, i32);

    const fn want(
        at: (i16, i16),
        extent: (u16, u16, u16),
    ) -> platform::Card {
        let ((x, y), (width, height, radius)) = (at, extent);
        platform::Card {
            x,
            y,
            width,
            height,
            radius,
        }
    }

    fn region(given: Rect) -> ui::FloatRegion {
        let (x, y, width, height, radius) = given;
        let origin = point(DevicePixels(x), DevicePixels(y));
        let extent = size(DevicePixels(width), DevicePixels(height));
        ui::FloatRegion {
            bounds: Bounds::new(origin, extent),
            radius: DevicePixels(radius),
        }
    }

    #[test]
    fn a_region_becomes_the_card_x_can_take() {
        const CASES: [(Rect, platform::Card); 3] = [
            ((4, 6, 30, 20, 8), want((4, 6), (30, 20, 8))),
            ((-5, 0, -3, 70_000, 0), want((-5, 0), (0, u16::MAX, 0))),
            (
                (40_000, -40_000, 1, 1, -2),
                want((i16::MAX, -i16::MAX), (1, 1, 0)),
            ),
        ];
        for (given, want) in CASES {
            let region = region(given);
            let got = card(&region);
            assert_eq!(got, want, "{region:?}");
        }
    }
}
