pub(crate) trait TokenStore: Send + Sync {
    fn get(
        &self,
        host_id: &str,
    ) -> Result<Option<String>, String>;
    fn set(
        &self,
        host_id: &str,
        token: &str,
    ) -> Result<(), String>;
    fn delete(
        &self,
        host_id: &str,
    ) -> Result<(), String>;
}
