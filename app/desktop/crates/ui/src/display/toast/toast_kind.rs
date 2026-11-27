#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
#[non_exhaustive]
pub enum ToastKind {
    #[default]
    Info,
    Success,
    Warning,
    Error,
    Loading,
}
