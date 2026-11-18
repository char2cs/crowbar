//! A unit-range colour channel as a byte.

use terminal::floor_cells;

const BYTE_MAX: f32 = 255.0;
const ROUNDING: f32 = 0.5;

pub(crate) fn channel_byte(unit: f32) -> u8 {
    let scaled = unit.clamp(0.0, 1.0) * BYTE_MAX + ROUNDING;
    u8::try_from(floor_cells(scaled)).unwrap_or(u8::MAX)
}

#[cfg(test)]
mod tests {
    use super::*;

    const CASES: &[(f32, u8)] = &[(-1.0, 0), (0.0, 0), (0.5, 128), (1.0, 255), (2.0, 255)];

    #[test]
    fn channels_round_and_saturate() {
        for (unit, expected) in CASES {
            assert_eq!(channel_byte(*unit), *expected);
        }
    }
}
