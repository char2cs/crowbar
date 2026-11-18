//! The fixed part of the xterm 256-colour table: the 6x6x6 cube and the grey ramp.

use crate::snapshot::terminal_palette::rgb::Rgb;

const CUBE_START: u8 = 16;
const GRAY_START: u8 = 232;
const CUBE_SIDE: u8 = 6;
const CUBE_BASE: u8 = 55;
const CUBE_STEP: u8 = 40;
const GRAY_BASE: u8 = 8;
const GRAY_STEP: u8 = 10;

pub(crate) fn xterm_color(index: u8) -> Rgb {
    if index >= GRAY_START {
        return gray(GRAY_BASE + GRAY_STEP * (index - GRAY_START));
    }
    let offset = index.saturating_sub(CUBE_START);
    Rgb {
        red: cube_level(offset / (CUBE_SIDE * CUBE_SIDE)),
        green: cube_level(offset / CUBE_SIDE % CUBE_SIDE),
        blue: cube_level(offset % CUBE_SIDE),
    }
}

fn gray(level: u8) -> Rgb {
    Rgb {
        red: level,
        green: level,
        blue: level,
    }
}

fn cube_level(step: u8) -> u8 {
    match step {
        0 => 0,
        _ => CUBE_BASE + CUBE_STEP * step,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn cube_and_gray_ramp_match_xterm() {
        let cases = [
            (16, 0x00_0000),
            (21, 0x00_00ff),
            (196, 0xff_0000),
            (231, 0xff_ffff),
            (232, 0x08_0808),
            (255, 0xee_eeee),
        ];
        for (index, hex) in cases {
            assert_eq!(xterm_color(index).to_hex(), hex, "index {index}");
        }
    }
}
