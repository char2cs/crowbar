#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum MouseEncoding {
    Legacy,
    Utf8,
    Sgr,
}
