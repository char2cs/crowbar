#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum PageCaller {
    Unstated,
    ShellPage,
    Stranger,
}
