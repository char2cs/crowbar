//! Where a remote daemon listens: the `host:port` the webview normalizes every
//! typed address to. Only that form is taken, so nothing dialed is a guess.

use std::fmt;
use std::net::{Ipv4Addr, Ipv6Addr, SocketAddr};

use tokio::net::{lookup_host, TcpStream};

/// A DNS name, IPv4 address or bracketed IPv6 address, and a port. Parsing
/// resolves nothing: a name is looked up only when it is dialed.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Endpoint {
    /// Without brackets, so it resolves as written.
    host: String,
    port: u16,
}

pub fn parse_endpoint(text: &str) -> Result<Endpoint, String> {
    let malformed = |why: &str| format!("{text:?} is not a host:port address: {why}");
    let allowed = |c: char| c.is_ascii_alphanumeric() || ".-:[]".contains(c);
    if !text.chars().all(allowed) {
        return Err(malformed("it holds characters no host or port does"));
    }
    let bracketed = text.strip_prefix('[');
    let (host, port) = match bracketed {
        Some(rest) => rest.split_once("]:"),
        None => text.split_once(':'),
    }
    .ok_or_else(|| malformed("it names no port"))?;
    let valid_host = match bracketed {
        Some(_) => host.parse::<Ipv6Addr>().is_ok(),
        None => is_name_or_ipv4(host),
    };
    if !valid_host {
        return Err(malformed("the host is no DNS name or IP address"));
    }
    let port = parse_port(port).ok_or_else(|| malformed("the port is not 1-65535"))?;
    Ok(Endpoint {
        host: host.to_string(),
        port,
    })
}

/// A numeric last label makes it an IPv4 address or nothing, as resolvers read it.
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

impl Endpoint {
    /// Resolves the host, then connects to the first of its addresses that
    /// answers. A name that resolves to nothing fails as `cannot resolve <host>`,
    /// told apart from a host nothing answers at, `cannot reach <endpoint>`.
    pub async fn connect(&self) -> Result<TcpStream, String> {
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
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        if self.host.contains(':') {
            return write!(f, "[{}]:{}", self.host, self.port);
        }
        write!(f, "{}:{}", self.host, self.port)
    }
}

#[cfg(test)]
mod tests {
    use super::parse_endpoint;

    #[test]
    fn a_host_and_port_parses_and_reads_back_as_written() {
        let long_label = format!("{}.com:1", "a".repeat(63));
        let longest_name = format!("{}a:3737", "a.".repeat(126));
        let valid = [
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
        for text in valid {
            let endpoint = parse_endpoint(text).unwrap_or_else(|e| panic!("{e}"));
            assert_eq!(endpoint.to_string(), text);
        }
    }

    #[test]
    fn anything_but_a_host_and_port_is_refused() {
        let long_label = format!("{}.com:3737", "a".repeat(64));
        let too_long = format!("{}ab:3737", "a.".repeat(126));
        let refused = [
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
            &long_label,
            &too_long,
        ];
        for text in refused {
            let err = parse_endpoint(text).expect_err(text);
            assert!(err.contains("is not a host:port address"), "{err}");
        }
    }
}
