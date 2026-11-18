pub(crate) trait ConnectionRelease: Send + Sync {
    fn release(
        &self,
        id: u64,
    );
}
