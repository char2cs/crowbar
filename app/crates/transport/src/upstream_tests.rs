use protocol::ExtensionId;

use super::*;
use crate::access_scope::EXTENSION_HEADER;

#[test]
fn the_daemon_is_told_which_extension_opened_the_stream_and_core_streams_carry_nothing() {
    let acme = PaneOrigin::Extension(ExtensionId::new("acme").unwrap());
    let extension = upgrade_request(&acme, "/v0/ext/acme/stream?x=1").unwrap();
    assert_eq!(extension.headers()[EXTENSION_HEADER], "acme");
    assert_eq!(
        extension.uri().path_and_query().unwrap().as_str(),
        "/v0/ext/acme/stream?x=1"
    );
    let core = upgrade_request(&PaneOrigin::Core, "/v0/terminals/t1/stream").unwrap();
    assert!(core.headers().get(EXTENSION_HEADER).is_none());
}

#[test]
fn an_unparsable_target_is_refused_before_anything_is_dialled() {
    assert!(upgrade_request(&PaneOrigin::Core, "/v0/a b").is_err());
}
