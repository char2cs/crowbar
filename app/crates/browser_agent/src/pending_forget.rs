pub(crate) trait PendingForget: Send + Sync {
    fn forget(
        &self,
        id: &str,
    );
}
