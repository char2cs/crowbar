/// One toggle of the find widget, named as Monaco's find widget names it.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum FindOption {
    /// "Match Case" (alt+c): letters match only in the same case.
    MatchCase,
    /// "Match Whole Word" (alt+w): a match must start and end at word separators.
    WholeWord,
    /// "Use Regular Expression" (alt+r): the query is a regular expression.
    Regex,
    /// "Preserve Case" (alt+p): a replacement takes the case of the text it replaces.
    PreserveCase,
}
