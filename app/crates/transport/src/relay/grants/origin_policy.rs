use protocol::RelayScope;

pub trait OriginPolicy: Send + Sync {
    fn allows(
        &self,
        scope: &RelayScope,
        origin: &str,
    ) -> bool;
}
