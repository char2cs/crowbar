const CORNERS: usize = 4;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Rounded {
    pub width: u16,
    pub height: u16,
    pub radii: [u16; CORNERS],
}
