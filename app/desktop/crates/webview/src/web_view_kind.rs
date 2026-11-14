use protocol::ExtensionId;

/// Who a web view belongs to, which decides how it is isolated.
///
/// Core views share one web process (related views) to keep memory low. Each extension gets
/// its own web context and process, so a hung or hostile extension pane cannot touch core
/// panes or other extensions.
#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub enum WebViewKind {
    /// A pane shipped with the app.
    Core,
    /// A pane provided by the given extension.
    Extension(ExtensionId),
}
