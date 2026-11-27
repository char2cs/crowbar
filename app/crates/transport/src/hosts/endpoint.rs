use std::fmt;
use std::net::{Ipv4Addr, Ipv6Addr, SocketAddr};

use serde::{Deserialize, Serialize};

use std::time::Duration;

use tokio::net::{TcpStream, lookup_host};

use crate::error::Error;
use crate::hosts::first_connected::first_connected;

const MAX_HOST_BYTES: usize = 253;
const MAX_LABEL_BYTES: usize = 63;
const CONNECT_STAGGER: Duration = Duration::from_millis(250);

#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(try_from = "String", into = "String")]
pub struct Endpoint {
    host: String,
    port: u16,
}

impl Endpoint {
    pub fn parse(text: &str) -> Result<Self, Error> {
        let malformed = |why| Error::MalformedEndpoint {
            text: text.to_owned(),
            why,
        };
        let allowed = |c: char| c.is_ascii_alphanumeric() || ".-:[]".contains(c);
        if !text.chars().all(allowed) {
            return Err(malformed("it holds characters no host or port does"));
        }
        let bracketed = text.strip_prefix('[');
        let split = match bracketed {
            Some(rest) => rest.split_once("]:"),
            None => text.split_once(':'),
        };
        let (host, port) = split.ok_or_else(|| malformed("it names no port"))?;
        let valid_host = match bracketed {
            Some(_) => host.parse::<Ipv6Addr>().is_ok(),
            None => is_name_or_ipv4(host),
        };
        if !valid_host {
            return Err(malformed("the host is no DNS name or IP address"));
        }
        let port = parse_port(port).ok_or_else(|| malformed("the port is not 1-65535"))?;
        Ok(Self {
            host: host.to_owned(),
            port,
        })
    }

    pub(crate) async fn connect(&self) -> Result<TcpStream, Error> {
        let unresolvable = |reason: String| Error::Unresolvable {
            host: self.host.clone(),
            reason,
        };
        let addrs: Vec<SocketAddr> = lookup_host((self.host.as_str(), self.port))
            .await
            .map_err(|e| unresolvable(e.to_string()))?
            .collect();
        if addrs.is_empty() {
            return Err(unresolvable("it has no addresses".to_owned()));
        }
        first_connected(addrs, CONNECT_STAGGER, TcpStream::connect)
            .await
            .map_err(|source| Error::Unreachable {
                endpoint: self.to_string(),
                source,
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

impl TryFrom<String> for Endpoint {
    type Error = Error;

    fn try_from(text: String) -> Result<Self, Self::Error> {
        Self::parse(&text)
    }
}

impl From<Endpoint> for String {
    fn from(endpoint: Endpoint) -> Self {
        endpoint.to_string()
    }
}

fn is_name_or_ipv4(host: &str) -> bool {
    let last = host.rsplit('.').next().unwrap_or_default();
    if !last.is_empty() && last.bytes().all(|b| b.is_ascii_digit()) {
        return host.parse::<Ipv4Addr>().is_ok();
    }
    host.len() <= MAX_HOST_BYTES && host.split('.').all(is_label)
}

fn is_label(label: &str) -> bool {
    let bytes = label.as_bytes();
    (1..=MAX_LABEL_BYTES).contains(&bytes.len())
        && bytes
            .iter()
            .all(|b| b.is_ascii_alphanumeric() || *b == b'-')
        && bytes.first() != Some(&b'-')
        && bytes.last() != Some(&b'-')
}

fn parse_port(text: &str) -> Option<u16> {
    if text.is_empty() || !text.bytes().all(|b| b.is_ascii_digit()) {
        return None;
    }
    text.parse().ok().filter(|&port| port != 0)
}

#[cfg(test)]
mod tests {
    use super::Endpoint;

    #[test]
    fn a_host_and_port_parses_and_reads_back_as_written() {
        let long_label = format!("{}.com:1", "a".repeat(63));
        let longest_name = format!("{}a:3737", "a.".repeat(126));
        let cases = [
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
            &long_label,
            &longest_name,
        ];
        for text in cases {
            let endpoint = Endpoint::parse(text).unwrap_or_else(|e| panic!("{e}"));
            assert_eq!(endpoint.to_string(), text);
        }
    }

    fn assert_refused(refused: &[&str]) {
        for text in refused {
            let err = Endpoint::parse(text).expect_err(text);
            assert!(
                err.to_string().contains("is not a host:port address"),
                "{err}"
            );
        }
    }

    #[test]
    fn anything_but_a_host_and_port_is_refused() {
        assert_refused(&[
            "",
            "homelab.example.com",
            "1.2.3.4",
            "[::1]",
            "::1",
            "::1:3737",
            "fd00::2:3737",
            "https://homelab.example.com:3737",
            "homelab.example.com:3737/",
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
        ]);
    }

    #[test]
    fn bad_ports_bad_addresses_and_overlong_names_are_refused() {
        let long_label = format!("{}.com:3737", "a".repeat(64));
        let too_long = format!("{}ab:3737", "a.".repeat(126));
        assert_refused(&[
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
            &long_label,
            &too_long,
        ]);
    }
}
