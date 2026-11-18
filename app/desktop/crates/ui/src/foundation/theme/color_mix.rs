//! Colour interpolation as CSS transitions do it: in sRGB with premultiplied alpha.

use gpui::{Hsla, Rgba};

pub(crate) fn mix(
    from: Hsla,
    to: Hsla,
    t: f32,
) -> Hsla {
    let (from, to) = (Rgba::from(from), Rgba::from(to));
    let alpha = from.a + (to.a - from.a) * t;
    if alpha <= f32::EPSILON {
        return Hsla::from(Rgba { a: 0.0, ..to });
    }
    let channel = |a: f32, b: f32| (a * from.a + (b * to.a - a * from.a) * t) / alpha;
    Hsla::from(Rgba {
        r: channel(from.r, to.r),
        g: channel(from.g, to.g),
        b: channel(from.b, to.b),
        a: alpha,
    })
}

#[cfg(test)]
mod tests {
    use gpui::rgba;

    use super::*;
    use crate::test_support::channels::channel_byte;

    fn bytes(color: Hsla) -> [u8; 4] {
        let c = Rgba::from(color);
        [c.r, c.g, c.b, c.a].map(channel_byte)
    }

    type Case = (&'static str, u32, u32, f32, [u8; 4]);

    const CASES: &[Case] = &[
        (
            "start is exact",
            0x1122_33ff,
            0xaabb_ccff,
            0.0,
            [0x11, 0x22, 0x33, 0xff],
        ),
        (
            "end is exact",
            0x1122_33ff,
            0xaabb_ccff,
            1.0,
            [0xaa, 0xbb, 0xcc, 0xff],
        ),
        (
            "opaque midpoint is the average",
            0x0000_00ff,
            0xffff_ffff,
            0.5,
            [128, 128, 128, 255],
        ),
        (
            "transparent to colour only fades alpha",
            0x0000_0000,
            0xffff_ff1a,
            0.5,
            [255, 255, 255, 13],
        ),
    ];

    #[test]
    fn mixing_matches_the_expected_bytes() {
        for &(name, from, to, amount, want) in CASES {
            let mixed = mix(Hsla::from(rgba(from)), Hsla::from(rgba(to)), amount);
            assert_eq!(bytes(mixed), want, "{name}");
        }
    }
}
