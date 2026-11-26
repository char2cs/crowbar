/// A rounded rectangle of a float window, in device pixels from the window's top-left corner.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub struct Card {
    /// Left edge.
    pub x: i16,
    /// Top edge.
    pub y: i16,
    /// Width.
    pub width: u16,
    /// Height.
    pub height: u16,
    /// Corner radius; it is clamped to half the shorter side.
    pub radius: u16,
}
