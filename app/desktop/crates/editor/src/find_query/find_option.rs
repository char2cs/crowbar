/// One toggle of a [`FindQuery`](crate::FindQuery).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum FindOption {
    /// Letters match only in the same case.
    MatchCase,
    /// A match must start and end at word separators.
    WholeWord,
}
