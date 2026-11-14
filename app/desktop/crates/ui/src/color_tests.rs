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
