#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Default)]
#[non_exhaustive]
pub enum IconWeight {
    #[default]
    Regular,
    Bold,
    Fill,
    Duotone,
}
