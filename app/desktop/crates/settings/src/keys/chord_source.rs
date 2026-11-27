#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum ChordSource {
    Default,
    Preset,
    Custom,
}
