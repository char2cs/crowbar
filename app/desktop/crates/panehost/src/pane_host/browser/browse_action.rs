use browser::PageUrl;

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum BrowseAction {
    Go(PageUrl),
    Back,
    Forward,
    Reload,
    Pick,
    CancelPick,
    Script(String),
}
