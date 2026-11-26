//! Identity and option of one search toggle button.

use terminal::SearchOptions;
use ui::IconName;

#[derive(Debug, Clone, Copy)]
pub(crate) struct ToggleSpec {
    pub(crate) id: &'static str,
    pub(crate) label: &'static str,
    pub(crate) icon: IconName,
    pub(crate) option: SearchOptions,
}

impl ToggleSpec {
    pub(crate) const CASE_SENSITIVE: Self = Self {
        id: "search-case",
        label: "Match case",
        icon: IconName::TextAa,
        option: SearchOptions::CASE_SENSITIVE,
    };
    pub(crate) const WHOLE_WORD: Self = Self {
        id: "search-word",
        label: "Match whole word",
        icon: IconName::TextT,
        option: SearchOptions::WHOLE_WORD,
    };
    pub(crate) const REGEX: Self = Self {
        id: "search-regex",
        label: "Use regular expression",
        icon: IconName::BracketsCurly,
        option: SearchOptions::REGEX,
    };
}
