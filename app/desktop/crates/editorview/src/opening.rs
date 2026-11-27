pub(crate) mod access;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Opening {
    pub line: Option<u32>,
    pub access: crate::Access,
}
