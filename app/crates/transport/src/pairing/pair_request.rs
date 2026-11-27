#[derive(Debug, Clone, Copy)]
pub struct PairRequest<'a> {
    pub host_id: &'a str,
    pub endpoint: &'a str,
    pub fingerprint: &'a str,
    pub code: &'a str,
    pub name: &'a str,
}
