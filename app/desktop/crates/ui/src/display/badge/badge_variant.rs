#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
#[non_exhaustive]
pub enum BadgeVariant {
    #[default]
    Primary,
    Destructive,
    Error,
    Info,
    Outline,
    Secondary,
    Success,
    Warning,
}
