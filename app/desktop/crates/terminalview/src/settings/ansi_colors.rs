use terminal::Rgb;
use ui::Appearance;

const ANSI_COUNT: usize = 16;
const LIGHT: [u32; ANSI_COUNT] = [
    0x26_2626, 0xb8_422c, 0x4e_7a36, 0xd9_a441, 0x52_7ca8, 0x5b_754a, 0xb6_5b46, 0x48_4848,
    0x48_4848, 0xb8_422c, 0x4e_7a36, 0xd9_a441, 0x52_7ca8, 0x5b_754a, 0xb6_5b46, 0x1b_1b19,
];
const DARK: [u32; ANSI_COUNT] = [
    0x08_0809, 0xe0_795f, 0x80_a36f, 0xd5_a24a, 0x7a_a6d8, 0xb6_93ce, 0x88_b5c6, 0xc7_c8ce,
    0x75_7982, 0xea_9177, 0x93_b584, 0xe0_b65f, 0x93_bde9, 0xc8_a7db, 0x9e_c9d7, 0xf3_f4f6,
];

pub(crate) fn ansi_colors(appearance: Appearance) -> [Rgb; ANSI_COUNT] {
    let table = match appearance {
        Appearance::Light => LIGHT,
        Appearance::Dark => DARK,
    };
    table.map(Rgb::from_hex)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn red_and_bright_white_match_the_spec() {
        assert_eq!(ansi_colors(Appearance::Dark)[1].to_hex(), 0xe0_795f);
        assert_eq!(ansi_colors(Appearance::Light)[15].to_hex(), 0x1b_1b19);
    }
}
