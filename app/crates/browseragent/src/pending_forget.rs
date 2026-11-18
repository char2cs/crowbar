pub(crate) trait PendingForget: Send + Sync {
    fn forget(
        &self,
        entry: &(String, u64),
    );
}
