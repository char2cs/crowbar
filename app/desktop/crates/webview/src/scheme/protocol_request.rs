use crate::view::Id;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ProtocolRequest {
    pub web_view: Id,
    pub uri: String,
    pub method: String,
    pub headers: Vec<(String, String)>,
    pub body: Vec<u8>,
}
