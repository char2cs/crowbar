use super::*;
use crate::response::{HOST_IDENTITY_CHANGED, HOST_UNREACHABLE};

#[test]
fn only_remote_host_failures_carry_a_proxy_marker() {
    assert_eq!(
        Error::IdentityChanged("x".into()).marker(),
        Some(HOST_IDENTITY_CHANGED)
    );
    assert_eq!(Error::UpgradeTimeout.marker(), Some(HOST_UNREACHABLE));
    assert_eq!(Error::Forbidden("x".into()).marker(), None);
    assert_eq!(Error::TooLarge.marker(), None);
}
