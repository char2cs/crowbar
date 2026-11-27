#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ProtocolResponse {
    pub status: u16,
    pub headers: Vec<(String, String)>,
    pub body: Vec<u8>,
}
