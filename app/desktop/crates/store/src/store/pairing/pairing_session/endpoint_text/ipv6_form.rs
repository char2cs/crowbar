//! Whether a typed host is an IPv6 literal or a name or IPv4 address.

#[derive(Clone, Copy, PartialEq, Eq)]
pub(crate) enum Ipv6Form {
    Yes,
    No,
}
