//! The pinned identity of a remote host: fingerprints, pins and verifiers.

mod config;
mod fingerprint;
mod pin;
mod pinned_verifier;
mod unpinned_probe_verifier;

pub(crate) use config::client_config_with;
pub use fingerprint::fingerprint_of;
pub(crate) use fingerprint::{CertificateDigest, DIGEST_BYTES, certificate_digest, hex};
pub(crate) use pin::Pin;
pub(crate) use pinned_verifier::pinned_client_config;
pub(crate) use unpinned_probe_verifier::UnpinnedProbeVerifier;
