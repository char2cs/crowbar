/// A move of the keyboard selection along a strip.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum StripStep {
    /// The tab before the current one.
    Previous,
    /// The tab after the current one.
    Next,
    /// The first tab.
    First,
    /// The last tab.
    Last,
}
