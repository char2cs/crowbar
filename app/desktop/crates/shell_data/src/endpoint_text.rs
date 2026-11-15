//! The one `host:port` an address is probed, paired, saved and compared as, however it was typed.

use std::net::Ipv6Addr;

use crate::host_port::HostPort;
use crate::ipv6_form::Ipv6Form;

const DEFAULT_PORT: u16 = 3737;
const MAX_NAME: usize = 253;
const MAX_LABEL: usize = 63;
const IPV4_LABELS: usize = 4;
const MAX_OCTET: u16 = 255;
const MAX_PORT_DIGITS: usize = 5;
const MAX_OCTET_DIGITS: usize = 3;
const SCHEMES: [&str; 3] = ["http://", "https://", "tcp://"];
const FORBIDDEN: [char; 7] = ['@', '/', '?', '#', '%', '\\', ' '];

pub(crate) const NOT_AN_ADDRESS: &str =
    "Enter a host name or IP address, optionally with a port, like homelab.example.com:3737.";
pub(crate) const NOT_A_PORT: &str = "The port must be a number from 1 to 65535.";

fn without_scheme(typed: &str) -> &str {
    let bare = typed.trim();
    let lower = bare.to_ascii_lowercase();
    let cut = SCHEMES
        .iter()
        .find(|scheme| lower.starts_with(*scheme))
        .map_or(0, |scheme| scheme.len());
    let rest = bare.get(cut..).unwrap_or(bare);
    rest.strip_suffix('/').unwrap_or(rest)
}

fn bracketed(inner: &str) -> Option<HostPort<'_>> {
    let (host, after) = inner.split_once(']')?;
    let port = after.strip_prefix(':');
    let tail_ok = after.is_empty() || port.is_some();
    tail_ok.then(|| HostPort::new(host, port, Ipv6Form::Yes))
}

fn split(text: &str) -> Option<HostPort<'_>> {
    if let Some(inner) = text.strip_prefix('[') {
        return bracketed(inner);
    }
    let mut parts = text.split(':');
    let host = parts.next().filter(|host| !host.is_empty())?;
    let port = parts.next();
    if parts.next().is_some() {
        return Some(HostPort::new(text, None, Ipv6Form::Yes));
    }
    Some(HostPort::new(host, port, Ipv6Form::No))
}

fn port_of(text: Option<&str>) -> Option<u16> {
    let Some(text) = text else {
        return Some(DEFAULT_PORT);
    };
    let digits = text.len() <= MAX_PORT_DIGITS && text.bytes().all(|byte| byte.is_ascii_digit());
    digits
        .then(|| text.parse::<u16>().ok())
        .flatten()
        .filter(|port| *port >= 1)
}

fn ipv6_host(text: &str) -> Option<String> {
    let alphabet = text
        .chars()
        .all(|ch| ch.is_ascii_hexdigit() || ch == ':' || ch == '.');
    let address: Ipv6Addr = text.parse().ok().filter(|_| alphabet)?;
    Some(format!("[{address}]"))
}

fn octet_ok(label: &str) -> bool {
    let canonical = label == "0" || !label.starts_with('0');
    canonical
        && !label.is_empty()
        && label.len() <= MAX_OCTET_DIGITS
        && label.parse::<u16>().is_ok_and(|value| value <= MAX_OCTET)
}

fn label_ok(label: &str) -> bool {
    let edge = |ch: Option<char>| ch.is_some_and(|ch| ch.is_ascii_alphanumeric());
    let inner = label
        .chars()
        .all(|ch| ch.is_ascii_alphanumeric() || ch == '-');
    label.len() <= MAX_LABEL && edge(label.chars().next()) && edge(label.chars().last()) && inner
}

fn name_or_ipv4(text: &str) -> Option<String> {
    let host = text.to_ascii_lowercase();
    if !host.is_ascii() || host.is_empty() {
        return None;
    }
    let labels: Vec<&str> = host.split('.').collect();
    let numeric_tail = labels
        .last()
        .is_some_and(|last| last.bytes().all(|byte| byte.is_ascii_digit()));
    if numeric_tail {
        let ipv4 = labels.len() == IPV4_LABELS && labels.iter().all(|label| octet_ok(label));
        return ipv4.then_some(host);
    }
    let valid = host.len() <= MAX_NAME && labels.iter().all(|label| label_ok(label));
    valid.then_some(host)
}

pub(crate) fn normalize(typed: &str) -> Result<String, &'static str> {
    let bare = without_scheme(typed);
    let forbidden = bare
        .chars()
        .any(|ch| FORBIDDEN.contains(&ch) || ch.is_whitespace());
    let parts = (!forbidden)
        .then_some(bare)
        .and_then(split)
        .ok_or(NOT_AN_ADDRESS)?;
    let port = port_of(parts.port).ok_or(NOT_A_PORT)?;
    let host = match parts.ipv6 {
        Ipv6Form::Yes => ipv6_host(parts.host),
        Ipv6Form::No => name_or_ipv4(parts.host),
    };
    host.map(|host| format!("{host}:{port}"))
        .ok_or(NOT_AN_ADDRESS)
}

#[cfg(test)]
mod tests {
    use super::*;

    type Expected = Result<&'static str, &'static str>;

    const CASES: [(&str, Expected); 16] = [
        ("homelab", Ok("homelab:3737")),
        (
            "  Homelab.Example.COM:4000 ",
            Ok("homelab.example.com:4000"),
        ),
        ("https://1.2.3.4/", Ok("1.2.3.4:3737")),
        ("tcp://box:1", Ok("box:1")),
        ("[::1]:9", Ok("[::1]:9")),
        ("[FE80:0:0:0:0:0:0:1]", Ok("[fe80::1]:3737")),
        ("fe80::1", Ok("[fe80::1]:3737")),
        ("", Err(NOT_AN_ADDRESS)),
        ("a b", Err(NOT_AN_ADDRESS)),
        ("user@box", Err(NOT_AN_ADDRESS)),
        ("box/path", Err(NOT_AN_ADDRESS)),
        ("1.2.3", Err(NOT_AN_ADDRESS)),
        ("1.2.3.256", Err(NOT_AN_ADDRESS)),
        ("box:0", Err(NOT_A_PORT)),
        ("box:70000", Err(NOT_A_PORT)),
        ("-box", Err(NOT_AN_ADDRESS)),
    ];

    #[test]
    fn an_address_is_one_canonical_host_and_port_or_the_reason_it_is_not() {
        for (typed, expected) in CASES {
            assert_eq!(
                normalize(typed).as_deref().map_err(|reason| *reason),
                expected,
                "{typed:?}"
            );
        }
    }
}
