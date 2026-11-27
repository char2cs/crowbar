#[derive(Debug, Clone, PartialEq, Eq)]
pub struct HostSpec {
    pub id: String,
    pub name: String,
    pub endpoint: Option<String>,
}
