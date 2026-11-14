use gpui::rgba;

use super::*;

fn bytes(color: Hsla) -> [u8; 4] {
    let c = Rgba::from(color);
    [c.r, c.g, c.b, c.a].map(|v| (v * 255.0).round() as u8)
}

#[test]
fn endpoints_are_exact() {
    let (a, b) = (Hsla::from(rgba(0x112233ff)), Hsla::from(rgba(0xaabbccff)));
    assert_eq!(bytes(mix(a, b, 0.0)), [0x11, 0x22, 0x33, 0xff]);
    assert_eq!(bytes(mix(a, b, 1.0)), [0xaa, 0xbb, 0xcc, 0xff]);
}

#[test]
fn opaque_midpoint_is_the_average() {
    let mid = mix(rgba(0x000000ff).into(), rgba(0xffffffff).into(), 0.5);
    assert_eq!(bytes(mid), [128, 128, 128, 255]);
}

#[test]
fn transparent_to_colour_only_fades_alpha() {
    let hot = Hsla::from(rgba(0xffffff1a));
    let rest = Hsla::from(rgba(0x00000000));
    let mid = bytes(mix(rest, hot, 0.5));
    assert_eq!(&mid[..3], &[255, 255, 255]);
    assert_eq!(mid[3], 13);
}
