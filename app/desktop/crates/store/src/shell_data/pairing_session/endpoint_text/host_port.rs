//! A typed host and port, split but not yet checked.

use crate::shell_data::pairing_session::endpoint_text::ipv6_form::Ipv6Form;

pub(crate) struct HostPort<'a> {
    pub(crate) host: &'a str,
    pub(crate) port: Option<&'a str>,
    pub(crate) ipv6: Ipv6Form,
}

impl<'a> HostPort<'a> {
    pub(crate) fn new(
        host: &'a str,
        port: Option<&'a str>,
        ipv6: Ipv6Form,
    ) -> Self {
        Self { host, port, ipv6 }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_parts_are_kept_as_given() {
        let parts = HostPort::new("lab", Some("1"), Ipv6Form::No);
        assert_eq!((parts.host, parts.port), ("lab", Some("1")));
    }
}
