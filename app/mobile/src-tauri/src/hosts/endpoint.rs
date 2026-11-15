use std::fmt;
use std::net::{Ipv4Addr, Ipv6Addr, SocketAddr};

use tokio::net::{lookup_host, TcpStream};

const MAX_NAME_LEN: usize = 253;
const MAX_LABEL_LEN: usize = 63;

#[derive(Clone, Debug, PartialEq, Eq)]
pub(crate) struct Endpoint {
    host: String,
    port: u16,
}

fn is_address_char(c: char) -> bool {
    c.is_ascii_alphanumeric() || ".-:[]".contains(c)
}

fn split_host_port(text: &str) -> Option<(&str, &str)> {
    match text.strip_prefix('[') {
        Some(rest) => rest.split_once("]:"),
        None => text.split_once(':'),
    }
}

fn is_valid_host(
    text: &str,
    host: &str,
) -> bool {
    if text.starts_with('[') {
        return host.parse::<Ipv6Addr>().is_ok();
    }
    is_name_or_ipv4(host)
}

pub(crate) fn parse_endpoint(text: &str) -> Result<Endpoint, String> {
    let malformed = |why: &str| format!("{text:?} is not a host:port address: {why}");
    if !text.chars().all(is_address_char) {
        return Err(malformed("it holds characters no host or port does"));
    }
    let (host, port) = split_host_port(text).ok_or_else(|| malformed("it names no port"))?;
    if !is_valid_host(text, host) {
        return Err(malformed("the host is no DNS name or IP address"));
    }
    let port = parse_port(port).ok_or_else(|| malformed("the port is not 1-65535"))?;
    Ok(Endpoint {
        host: host.to_owned(),
        port,
    })
}

fn is_name_or_ipv4(host: &str) -> bool {
    let last = host.rsplit('.').next().unwrap_or_default();
    if !last.is_empty() && last.bytes().all(|b| b.is_ascii_digit()) {
        return host.parse::<Ipv4Addr>().is_ok();
    }
    host.len() <= MAX_NAME_LEN && host.split('.').all(is_label)
}

fn is_label(label: &str) -> bool {
    let bytes = label.as_bytes();
    let edge_is_hyphen = bytes.first() == Some(&b'-') || bytes.last() == Some(&b'-');
    let in_range = (1..=MAX_LABEL_LEN).contains(&bytes.len());
    let plain = bytes
        .iter()
        .all(|b| b.is_ascii_alphanumeric() || *b == b'-');
    in_range && plain && !edge_is_hyphen
}

fn parse_port(text: &str) -> Option<u16> {
    if text.is_empty() || !text.bytes().all(|b| b.is_ascii_digit()) {
        return None;
    }
    text.parse().ok().filter(|&port| port != 0)
}

impl Endpoint {
    pub(crate) async fn connect(&self) -> Result<TcpStream, String> {
        let addrs: Vec<SocketAddr> = lookup_host((self.host.as_str(), self.port))
            .await
            .map_err(|e| format!("cannot resolve {}: {e}", self.host))?
            .collect();
        let mut refused = None;
        for addr in addrs {
            match TcpStream::connect(addr).await {
                Ok(tcp) => return Ok(tcp),
                Err(e) => refused = Some(e),
            }
        }
        Err(match refused {
            Some(e) => format!("cannot reach {self}: {e}"),
            None => format!("cannot resolve {}: it has no addresses", self.host),
        })
    }
}

impl fmt::Display for Endpoint {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        if self.host.contains(':') {
            return write!(f, "[{}]:{}", self.host, self.port);
        }
        write!(f, "{}:{}", self.host, self.port)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const VALID: &[&str] = &[
        "homelab.example.com:3737",
        "HomeLab.Example.com:3737",
        "homelab:4000",
        "my-host-2.lan:3737",
        "xn--bcher-kva.example:3737",
        "localhost:38737",
        "1.2.3.4:3737",
        "0.0.0.0:1",
        "255.255.255.255:65535",
        "[::1]:3737",
        "[fd00::2]:4000",
        "[::ffff:102:304]:3737",
    ];

    const REFUSED: &[&str] = &[
        "",
        "homelab.example.com",
        "1.2.3.4",
        "[::1]",
        "::1",
        "::1:3737",
        "fd00::2:3737",
        "https://homelab.example.com:3737",
        "tcp://homelab.example.com:3737",
        "homelab.example.com:3737/",
        "homelab.example.com:3737/v0",
        "homelab.example.com:3737?q",
        "me@homelab.example.com:3737",
        "home lab.example.com:3737",
        " homelab.example.com:3737",
        "home_lab.example.com:3737",
        "-homelab.example.com:3737",
        "homelab-.example.com:3737",
        "homelab..example.com:3737",
        "homelab.example.com.:3737",
        "bücher.example:3737",
        "homelab.example.com:",
        "homelab.example.com:0",
        "homelab.example.com:65536",
        "homelab.example.com:+80",
        "homelab.example.com:-1",
        "homelab.example.com:http",
        "homelab.example.com:3737:3737",
        "1.2.3.256:3737",
        "1.2.3:3737",
        "01.2.3.4:3737",
        "host.123:3737",
        "[homelab]:3737",
        "[::1]x:3737",
        "[::1]:3737]",
        "[fe80::1%en0]:3737",
    ];

    fn assert_reads_back(text: &str) {
        let endpoint = parse_endpoint(text).unwrap_or_else(|e| panic!("{e}"));
        assert_eq!(endpoint.to_string(), text);
    }

    fn assert_refused(text: &str) {
        let err = parse_endpoint(text).expect_err(text);
        assert!(err.contains("is not a host:port address"), "{err}");
    }

    #[test]
    fn a_host_and_port_parses_and_reads_back_as_written() {
        for text in VALID {
            assert_reads_back(text);
        }
        assert_reads_back(&format!("{}.com:1", "a".repeat(63)));
        assert_reads_back(&format!("{}a:3737", "a.".repeat(126)));
    }

    #[test]
    fn anything_but_a_host_and_port_is_refused() {
        for text in REFUSED {
            assert_refused(text);
        }
        assert_refused(&format!("{}.com:3737", "a".repeat(64)));
        assert_refused(&format!("{}ab:3737", "a.".repeat(126)));
    }
}
