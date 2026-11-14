use protocol::RelayScope;

/// Decides which `Origin` a pane's webview may present for a token issued to `scope`. The shell
/// owns the origins of its webviews, so it supplies the policy.
pub trait OriginPolicy: Send + Sync {
    /// Whether `origin` is the exact origin of the pane `scope` was issued to.
    fn allows(&self, scope: &RelayScope, origin: &str) -> bool;
}

#[cfg(test)]
#[path = "origin_policy_tests.rs"]
mod tests;
