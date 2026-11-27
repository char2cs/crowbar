use protocol::ExtensionId;

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub enum Kind {
    Core,
    Extension(ExtensionId),
    Browser,
}
