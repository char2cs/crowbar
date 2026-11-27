use std::fmt::{self, Display};
use std::io;
use std::time::Duration;

use serde::{Serialize, Serializer};
use tauri::http;
use tokio::time::error::Elapsed;

use crate::token_error::TokenError;

type BoxError = Box<dyn std::error::Error + Send + Sync>;

#[derive(Debug)]
pub(crate) enum HostError {
    HostIdCharacters(String),
    HostIdIsLocal(String),
    NotAnAddress {
        text: String,
        why: &'static str,
    },
    Resolve {
        host: String,
        source: io::Error,
    },
    NoAddress(String),
    Refused {
        endpoint: String,
        source: io::Error,
    },
    ProbeTimeout {
        endpoint: String,
        limit: Duration,
        source: Elapsed,
    },
    PairingTimeout {
        endpoint: String,
        limit: Duration,
        source: Elapsed,
    },
    Handshake {
        endpoint: String,
        source: io::Error,
    },
    NoCertificate(String),
    TlsUnavailable(rustls::Error),
    ServerName {
        name: &'static str,
        source: rustls::pki_types::InvalidDnsNameError,
    },
    BadRequest(http::Error),
    Transport(BoxError),
    PairingTransport {
        endpoint: String,
        source: BoxError,
    },
    Dial(BoxError),
    Rejected {
        marker: &'static str,
        source: BoxError,
    },
    PairingRefused {
        status: http::StatusCode,
        reason: String,
    },
    PairingAnswerLacks(String),
    RevokeRefused(http::StatusCode),
    KeychainRefused {
        endpoint: String,
        source: Box<HostError>,
        revocation: Option<Box<HostError>>,
    },
    Token(TokenError),
    Interrupted(tokio::task::JoinError),
    InvalidCredential(http::header::InvalidHeaderValue),
    UnknownHost(String),
    Unpaired,
    LookupFailed(Box<HostError>),
    NoLocalDaemon,
    MissingKey {
        command: &'static str,
        key: String,
    },
    JsonExpected(&'static str),
    NotAString {
        command: &'static str,
        key: String,
    },
    BadChannelId {
        command: &'static str,
        reason: String,
    },
    BadWsRequest(tokio_tungstenite::tungstenite::Error),
    WsUpgrade(tokio_tungstenite::tungstenite::Error),
    WsClosed(String),
    NoWsConnection(String),
}

impl Display for HostError {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::HostIdCharacters(_)
            | Self::HostIdIsLocal(_)
            | Self::NotAnAddress { .. }
            | Self::Resolve { .. }
            | Self::NoAddress(_)
            | Self::Refused { .. }
            | Self::ProbeTimeout { .. }
            | Self::PairingTimeout { .. }
            | Self::Handshake { .. }
            | Self::NoCertificate(_)
            | Self::TlsUnavailable(_)
            | Self::ServerName { .. } => self.fmt_reaching(f),
            Self::BadRequest(_)
            | Self::Transport(_)
            | Self::PairingTransport { .. }
            | Self::Dial(_)
            | Self::Rejected { .. }
            | Self::PairingRefused { .. }
            | Self::PairingAnswerLacks(_)
            | Self::RevokeRefused(_)
            | Self::KeychainRefused { .. } => self.fmt_pairing(f),
            _ => self.fmt_lookup(f),
        }
    }
}

impl HostError {
    pub(crate) fn missing_key(
        command: &'static str,
        key: &str,
    ) -> Self {
        Self::MissingKey {
            command,
            key: key.to_owned(),
        }
    }

    pub(crate) fn not_a_string(
        command: &'static str,
        key: &str,
    ) -> Self {
        Self::NotAString {
            command,
            key: key.to_owned(),
        }
    }

    pub(crate) fn bad_channel_id(
        command: &'static str,
        reason: &str,
    ) -> Self {
        Self::BadChannelId {
            command,
            reason: reason.to_owned(),
        }
    }

    pub(crate) fn pairing_refused(
        status: http::StatusCode,
        reason: &str,
    ) -> Self {
        Self::PairingRefused {
            status,
            reason: reason.to_owned(),
        }
    }

    fn fmt_reaching(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::HostIdCharacters(id) => write!(
                f,
                "host id {id:?} may only hold lowercase letters, digits and hyphens"
            ),
            Self::HostIdIsLocal(id) => write!(f, "host id {id:?} names the local daemon"),
            Self::NotAnAddress { text, why } => {
                write!(f, "{text:?} is not a host:port address: {why}")
            },
            Self::Resolve { host, source } => write!(f, "cannot resolve {host}: {source}"),
            Self::NoAddress(host) => write!(f, "cannot resolve {host}: it has no addresses"),
            Self::Refused { endpoint, source } => write!(f, "cannot reach {endpoint}: {source}"),
            Self::ProbeTimeout {
                endpoint, limit, ..
            } => write!(f, "{endpoint} did not answer within {limit:?}"),
            Self::PairingTimeout {
                endpoint, limit, ..
            } => write!(f, "{endpoint} did not finish pairing within {limit:?}"),
            Self::Handshake { endpoint, source } => {
                write!(f, "TLS handshake with {endpoint} failed: {source}")
            },
            Self::NoCertificate(endpoint) => write!(f, "{endpoint} presented no certificate"),
            Self::TlsUnavailable(source) => write!(f, "TLS 1.3 is unavailable: {source}"),
            Self::ServerName { name, source } => write!(f, "{name}: {source}"),
            _ => Ok(()),
        }
    }

    fn fmt_pairing(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::BadRequest(source) => source.fmt(f),
            Self::Transport(source) | Self::Dial(source) => source.fmt(f),
            Self::PairingTransport { endpoint, source } => {
                write!(f, "pairing with {endpoint} failed: {source}")
            },
            Self::Rejected { marker, source } => write!(f, "{marker}: {source}"),
            Self::PairingRefused { status, reason } => {
                write!(f, "the host refused pairing ({status}): {reason}")
            },
            Self::PairingAnswerLacks(field) => {
                write!(f, "the host's pairing answer carried no {field}")
            },
            Self::RevokeRefused(status) => write!(f, "the host answered {status}"),
            Self::KeychainRefused {
                endpoint,
                source,
                revocation,
            } => {
                write!(
                    f,
                    "the keychain could not keep the token for {endpoint} ({source})"
                )?;
                match revocation {
                    None => f.write_str("; the host revoked it"),
                    Some(why) => write!(f, "; revoking it on the host failed: {why}"),
                }
            },
            _ => Ok(()),
        }
    }

    fn fmt_lookup(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        match self {
            Self::Token(source) => source.fmt(f),
            Self::Interrupted(source) => source.fmt(f),
            Self::InvalidCredential(_) => {
                f.write_str("the stored credential is not a valid header value")
            },
            Self::UnknownHost(id) => write!(f, "unknown remote host {id}"),
            Self::Unpaired => f.write_str("no stored credential for this host — pair it again"),
            Self::LookupFailed(source) => write!(f, "token lookup failed: {source}"),
            Self::NoLocalDaemon => f.write_str("no local daemon on this device"),
            Self::MissingKey { command, key } => {
                write!(f, "command {command} missing required key {key}")
            },
            Self::JsonExpected(command) => write!(f, "{command} expects JSON arguments"),
            Self::NotAString { command, key } => {
                write!(
                    f,
                    "invalid args `{key}` for command `{command}`: expected a string"
                )
            },
            Self::BadChannelId { command, reason } => write!(
                f,
                "invalid args `onMessage` for command `{command}`: {reason}"
            ),
            Self::BadWsRequest(source) => write!(f, "build ws request: {source}"),
            Self::WsUpgrade(source) => write!(f, "ws upgrade: {source}"),
            Self::WsClosed(id) => write!(f, "ws connection {id} is closed"),
            Self::NoWsConnection(id) => write!(f, "no open ws connection {id}"),
            _ => Ok(()),
        }
    }
}

impl std::error::Error for HostError {
    fn source(&self) -> Option<&(dyn std::error::Error + 'static)> {
        match self {
            Self::Resolve { source, .. }
            | Self::Refused { source, .. }
            | Self::Handshake { source, .. } => Some(source),
            Self::ProbeTimeout { source, .. } | Self::PairingTimeout { source, .. } => Some(source),
            Self::TlsUnavailable(source) => Some(source),
            Self::ServerName { source, .. } => Some(source),
            Self::BadRequest(source) => Some(source),
            Self::Transport(source)
            | Self::Dial(source)
            | Self::Rejected { source, .. }
            | Self::PairingTransport { source, .. } => Some(source.as_ref()),
            Self::Token(source) => Some(source),
            Self::Interrupted(source) => Some(source),
            Self::InvalidCredential(source) => Some(source),
            Self::BadWsRequest(source) | Self::WsUpgrade(source) => Some(source),
            Self::KeychainRefused { source, .. } | Self::LookupFailed(source) => {
                Some(source.as_ref())
            },
            _ => None,
        }
    }
}

impl Serialize for HostError {
    fn serialize<S: Serializer>(
        &self,
        serializer: S,
    ) -> Result<S::Ok, S::Error> {
        serializer.collect_str(self)
    }
}

impl From<TokenError> for HostError {
    fn from(error: TokenError) -> Self {
        Self::Token(error)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_failure_reaches_the_page_as_its_message_text() {
        let cases = [
            (HostError::NoLocalDaemon, "no local daemon on this device"),
            (
                HostError::WsClosed("c1".into()),
                "ws connection c1 is closed",
            ),
            (
                HostError::HostIdIsLocal("localhost".into()),
                r#"host id "localhost" names the local daemon"#,
            ),
        ];
        for (error, text) in cases {
            assert_eq!(error.to_string(), text);
            assert_eq!(serde_json::to_string(&error).unwrap(), format!("{text:?}"));
        }
    }
}
