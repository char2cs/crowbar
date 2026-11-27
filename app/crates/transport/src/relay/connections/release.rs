pub(crate) trait Release: Send + Sync {
    fn release(
        &self,
        id: u64,
    );
}
