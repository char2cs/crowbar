//! Colour construction from the literals of the spec.

use gpui::Rgba;

const BYTE_MASK: u32 = 0xff;
const BYTE_MAX: f32 = 255.0;
const RED_SHIFT: u32 = 24;
const GREEN_SHIFT: u32 = 16;
const BLUE_SHIFT: u32 = 8;

const fn channel(
    value: u32,
    shift: u32,
) -> f32 {
    (((value >> shift) & BYTE_MASK) as u8) as f32 / BYTE_MAX
}

pub(crate) const fn hex(value: u32) -> Rgba {
    Rgba {
        r: channel(value, RED_SHIFT),
        g: channel(value, GREEN_SHIFT),
        b: channel(value, BLUE_SHIFT),
        a: channel(value, 0),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::channel_byte;

    #[test]
    fn hex_splits_channels() {
        const CASES: &[(u32, [u8; 4])] = &[
            (0xfcfc_faff, [0xfc, 0xfc, 0xfa, 0xff]),
            (0x0000_002e, [0, 0, 0, 0x2e]),
            (0x98a8_6a4c, [0x98, 0xa8, 0x6a, 0x4c]),
        ];
        for &(value, [r, g, b, a]) in CASES {
            let color = hex(value);
            let bytes = [color.r, color.g, color.b, color.a].map(channel_byte);
            assert_eq!(bytes, [r, g, b, a], "literal {value:#010x}");
        }
    }
}
