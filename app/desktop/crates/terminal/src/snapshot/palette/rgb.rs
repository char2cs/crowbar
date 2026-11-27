const DIM_NUMERATOR: u32 = 2;
const DIM_DENOMINATOR: u32 = 3;
const BYTE_BITS: u32 = 8;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub struct Rgb {
    pub red: u8,
    pub green: u8,
    pub blue: u8,
}

impl Rgb {
    pub const fn from_hex(hex: u32) -> Self {
        let [_, red, green, blue] = hex.to_be_bytes();
        Self { red, green, blue }
    }

    pub fn to_hex(self) -> u32 {
        (u32::from(self.red) << (2 * BYTE_BITS))
            | (u32::from(self.green) << BYTE_BITS)
            | u32::from(self.blue)
    }

    #[must_use]
    pub fn dimmed(self) -> Self {
        Self {
            red: dim_channel(self.red),
            green: dim_channel(self.green),
            blue: dim_channel(self.blue),
        }
    }
}

fn dim_channel(channel: u8) -> u8 {
    let dimmed = u32::from(channel) * DIM_NUMERATOR / DIM_DENOMINATOR;
    u8::try_from(dimmed).unwrap_or(u8::MAX)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn hex_round_trips() {
        for hex in [0x00_0000, 0xff_ffff, 0x12_34ab, 0x80_a36f] {
            assert_eq!(Rgb::from_hex(hex).to_hex(), hex, "hex {hex:06x}");
        }
    }

    #[test]
    fn dimming_takes_two_thirds_of_each_channel() {
        let dimmed = Rgb::from_hex(0x90_6030).dimmed();
        assert_eq!(dimmed, Rgb::from_hex(0x60_4020));
    }
}
