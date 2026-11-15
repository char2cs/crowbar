pub(crate) const WS_CLOSE_SENTINEL: &str = "\u{0}crowbar-ws-close";

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum Frame {
    Text(String),
    Binary(Vec<u8>),
}
