use crate::token_error::TokenError;

pub(crate) trait TokenStore: Send + Sync {
    fn get(
        &self,
        host_id: &str,
    ) -> Result<Option<String>, TokenError>;
    fn set(
        &self,
        host_id: &str,
        token: &str,
    ) -> Result<(), TokenError>;
    fn delete(
        &self,
        host_id: &str,
    ) -> Result<(), TokenError>;
}
