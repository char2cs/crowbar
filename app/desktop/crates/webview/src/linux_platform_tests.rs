use super::*;
use crate::protocol_handler::{ProtocolHandler, ProtocolResponder};
use crate::protocol_request::ProtocolRequest;

struct Silent;

impl ProtocolHandler for Silent {
    fn handle(&self, _request: ProtocolRequest, _respond: ProtocolResponder) {}
}

#[test]
fn starting_without_a_display_is_a_typed_error() {
    if std::env::var_os("DISPLAY").is_some() {
        return;
    }
    let config = HostConfig {
        scheme: "crowbar".into(),
        protocol: std::sync::Arc::new(Silent),
        data_dir: None,
    };
    assert!(
        matches!(LinuxPlatform::start(config), Err(Error::PlatformThread(_))),
        "no display"
    );
}
