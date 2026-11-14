use gpui::rgba;

use super::*;
use crate::test_support::channel_byte;

fn bytes(color: Hsla) -> [u8; 4] {
    let c = Rgba::from(color);
    [c.r, c.g, c.b, c.a].map(channel_byte)
}

#[test]
fn endpoints_are_exact() {
    let (a, b) = (Hsla::from(rgba(0x1122_33ff)), Hsla::from(rgba(0xaabb_ccff)));
    assert_eq!(bytes(mix(a, b, 0.0)), [0x11, 0x22, 0x33, 0xff]);
    assert_eq!(bytes(mix(a, b, 1.0)), [0xaa, 0xbb, 0xcc, 0xff]);
}

#[test]
fn opaque_midpoint_is_the_average() {
    let mid = mix(rgba(0x0000_00ff).into(), rgba(0xffff_ffff).into(), 0.5);
    assert_eq!(bytes(mid), [128, 128, 128, 255]);
}

#[test]
fn transparent_to_colour_only_fades_alpha() {
    let hot = Hsla::from(rgba(0xffff_ff1a));
    let rest = Hsla::from(rgba(0x0000_0000));
    let mid = bytes(mix(rest, hot, 0.5));
    assert_eq!(&mid[..3], &[255, 255, 255]);
    assert_eq!(mid[3], 13);
}
