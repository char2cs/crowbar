use std::sync::Arc;

use super::*;
use crate::protocol_request::ProtocolRequest;
use crate::protocol_handler::ProtocolResponder;

struct Silent;

impl ProtocolHandler for Silent {
    fn handle(&self, _request: ProtocolRequest, _respond: ProtocolResponder) {}
}

#[test]
fn debug_shows_the_scheme_and_hides_the_handler() {
    let config = HostConfig {
        scheme: "crowbar".into(),
        protocol: Arc::new(Silent),
        data_dir: None,
    };
    let text = format!("{config:?}");
    assert!(text.contains("crowbar"), "scheme shown: {text}");
    assert!(!text.contains("Silent"), "handler hidden: {text}");
}
