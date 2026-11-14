//! Where a remote daemon listens: the `host:port` form every typed address is normalised to.

use std::fmt;
use std::net::{Ipv4Addr, Ipv6Addr, SocketAddr};

use serde::{Deserialize, Serialize};

use crate::error::Error;
use tokio::net::{TcpStream, lookup_host};

/// A DNS name, IPv4 address or bracketed IPv6 address, and a port. Parsing resolves nothing:
/// a name is looked up only when it is dialled.
#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(try_from = "String", into = "String")]
pub struct Endpoint {
    host: String,
    port: u16,
}

impl Endpoint {
    /// Parses `host:port`, `a.b.c.d:port` or `[v6]:port`; nothing else is taken.
    ///
    /// # Errors
    /// [`Error::MalformedEndpoint`] for anything else.
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

    /// Resolves the host, then connects to the first of its addresses that answers.
    ///
    /// # Errors
    /// [`Error::Unresolvable`] or [`Error::Unreachable`].
    pub async fn connect(&self) -> Result<TcpStream, Error> {
        let unresolvable = |reason: String| Error::Unresolvable {
            host: self.host.clone(),
            reason,
        };
        let addrs: Vec<SocketAddr> = lookup_host((self.host.as_str(), self.port))
            .await
            .map_err(|e| unresolvable(e.to_string()))?
            .collect();
        let mut refused = None;
        for addr in addrs {
            match TcpStream::connect(addr).await {
                Ok(tcp) => return Ok(tcp),
                Err(e) => refused = Some(e),
            }
        }
        Err(match refused {
            Some(source) => Error::Unreachable {
                endpoint: self.clone(),
                source,
            },
            None => unresolvable("it has no addresses".to_owned()),
        })
    }
}

impl fmt::Display for Endpoint {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
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
    host.len() <= 253 && host.split('.').all(is_label)
}

fn is_label(label: &str) -> bool {
    let bytes = label.as_bytes();
    (1..=63).contains(&bytes.len())
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
#[path = "endpoint_tests.rs"]
mod tests;
