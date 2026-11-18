use crate::host_token::HostToken;

pub(crate) fn token() -> HostToken {
    HostToken::parse("t0k3n").unwrap()
}
