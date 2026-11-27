use crate::snapshot::palette::Rgb;

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub struct BackgroundSpan {
    pub column: usize,
    pub columns: usize,
    pub color: Rgb,
}
