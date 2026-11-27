#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum LinkChoice {
    DefaultBrowser,
    CrowbarBrowser,
    CopyLink,
}

impl LinkChoice {
    pub(crate) const ALL: [Self; 3] = [Self::DefaultBrowser, Self::CrowbarBrowser, Self::CopyLink];

    pub(crate) fn label(self) -> &'static str {
        match self {
            Self::DefaultBrowser => "Open in Default Browser",
            Self::CrowbarBrowser => "Open in Crowbar Browser",
            Self::CopyLink => "Copy Link",
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_menu_lists_develops_three_rows_in_order() {
        let labels = LinkChoice::ALL.map(LinkChoice::label);
        assert_eq!(
            labels,
            [
                "Open in Default Browser",
                "Open in Crowbar Browser",
                "Copy Link"
            ]
        );
    }
}
