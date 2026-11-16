/// An opaque-or-translucent sRGB colour in 8-bit channels.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Color {
    /// Red channel.
    pub red: u8,
    /// Green channel.
    pub green: u8,
    /// Blue channel.
    pub blue: u8,
    /// Alpha channel; 255 is opaque.
    pub alpha: u8,
}

impl Color {
    /// The channels in the order the engines take them.
    #[must_use]
    pub fn rgba(self) -> (u8, u8, u8, u8) {
        (self.red, self.green, self.blue, self.alpha)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_channels_come_out_in_engine_order() {
        let color = Color {
            red: 1,
            green: 2,
            blue: 3,
            alpha: 4,
        };
        assert_eq!(color.rgba(), (1, 2, 3, 4), "red, green, blue, alpha");
    }
}
