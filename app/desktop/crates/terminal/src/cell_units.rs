//! Conversions between cell counts and floats without lossy `as` casts.

const HALF_WORD_BITS: u32 = 16;
const HALF_WORD_RANGE: f32 = 65_536.0;
const SEARCH_STEPS: u32 = 32;

/// A cell or line count as a float; counts of 2^32 and above saturate.
pub fn cells_f32(count: usize) -> f32 {
    let high = u16::try_from(count >> HALF_WORD_BITS).unwrap_or(u16::MAX);
    let low = u16::try_from(count & usize::from(u16::MAX)).unwrap_or(u16::MAX);
    f32::from(high) * HALF_WORD_RANGE + f32::from(low)
}

/// The largest whole count not above `value`; negative and NaN values give 0.
pub fn floor_cells(value: f32) -> usize {
    let upper = usize::try_from(u32::MAX).unwrap_or(usize::MAX);
    let (floor, _) = (0..SEARCH_STEPS).fold((0_usize, upper), |range, _| narrow(range, value));
    floor
}

fn narrow(
    (low, high): (usize, usize),
    value: f32,
) -> (usize, usize) {
    let middle = low + (high - low).div_ceil(2);
    if cells_f32(middle) <= value {
        return (middle, high);
    }
    (low, middle - 1)
}

#[cfg(test)]
mod tests {
    use super::*;

    const COUNT_CASES: &[(usize, f32)] = &[
        (0, 0.0),
        (80, 80.0),
        (65_536, 65_536.0),
        (200_000, 200_000.0),
    ];
    const FLOOR_CASES: &[(f32, usize)] = &[
        (-3.0, 0),
        (f32::NAN, 0),
        (0.0, 0),
        (7.9, 7),
        (80.0, 80),
        (150_000.5, 150_000),
    ];

    #[test]
    fn counts_and_floors_convert() {
        for (count, expected) in COUNT_CASES {
            assert_eq!(cells_f32(*count), *expected);
        }
        for (value, expected) in FLOOR_CASES {
            assert_eq!(floor_cells(*value), *expected);
        }
    }
}
