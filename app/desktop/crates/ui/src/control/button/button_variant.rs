#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
#[non_exhaustive]
pub enum ButtonVariant {
    #[default]
    Primary,
    Destructive,
    Outline,
    Secondary,
    Ghost,
    Muted,
    Faint,
    Row,
}
