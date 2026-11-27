#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Color {
    pub red: u8,
    pub green: u8,
    pub blue: u8,
    pub alpha: u8,
}

impl Color {
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
