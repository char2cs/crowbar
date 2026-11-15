use std::sync::Arc;

use rustls::crypto::aws_lc_rs;
use rustls::{ClientConfig, Error as TlsError};

mod digest;
mod pin;
mod pinned_verifier;

pub(crate) use digest::{certificate_digest, fingerprint_of, hex, CertificateDigest};
pub(crate) use pin::Pin;

use pinned_verifier::PinnedVerifier;

pub(crate) fn pinned_client_config(pin: Pin) -> Result<Arc<ClientConfig>, TlsError> {
    let provider = Arc::new(aws_lc_rs::default_provider());
    let verifier = Arc::new(PinnedVerifier::new(pin, &provider));
    let config = ClientConfig::builder_with_provider(provider)
        .with_protocol_versions(&[&rustls::version::TLS13])?
        .dangerous()
        .with_custom_certificate_verifier(verifier)
        .with_no_client_auth();
    Ok(Arc::new(config))
}

#[cfg(test)]
mod tests {
    use super::*;
    use rustls::client::danger::ServerCertVerifier;
    use rustls::internal::msgs::codec::Codec;
    use rustls::pki_types::{CertificateDer, PrivateKeyDer, PrivatePkcs8KeyDer, ServerName};
    use rustls::sign::{CertifiedKey, SingleCertAndKey};
    use rustls::{
        CertificateError, Connection, DigitallySignedStruct, PeerIncompatible, SignatureScheme,
        SupportedProtocolVersion,
    };

    const TRANSCRIPT: &[u8] = b"handshake transcript the server proves key possession over";

    fn generate_identity() -> (CertificateDer<'static>, PrivateKeyDer<'static>) {
        let certified = rcgen::generate_simple_self_signed(["localhost".to_owned()])
            .expect("generate self-signed test cert");
        let key = PrivatePkcs8KeyDer::from(certified.key_pair.serialize_der()).into();
        (certified.cert.der().clone(), key)
    }

    fn attacker_key() -> PrivateKeyDer<'static> {
        let key_pair = rcgen::KeyPair::generate().expect("generate attacker key");
        PrivatePkcs8KeyDer::from(key_pair.serialize_der()).into()
    }

    fn verifier_for(cert: &CertificateDer<'_>) -> PinnedVerifier {
        let pin = Pin::Confirmed(fingerprint_of(cert));
        PinnedVerifier::new(pin, &aws_lc_rs::default_provider())
    }

    fn sign(
        key: &PrivateKeyDer<'_>,
        message: &[u8],
    ) -> DigitallySignedStruct {
        let scheme = SignatureScheme::ECDSA_NISTP256_SHA256;
        let signer = aws_lc_rs::sign::any_ecdsa_type(key)
            .expect("load P-256 key")
            .choose_scheme(&[scheme])
            .expect("a P-256 key signs ECDSA_NISTP256_SHA256");
        let signature = signer.sign(message).expect("sign message");
        let mut wire = scheme.to_array().to_vec();
        wire.extend_from_slice(&u16::try_from(signature.len()).unwrap().to_be_bytes());
        wire.extend_from_slice(&signature);
        DigitallySignedStruct::read_bytes(&wire).expect("decode DigitallySignedStruct")
    }

    fn transfer(
        from: &mut Connection,
        to: &mut Connection,
    ) -> Result<bool, TlsError> {
        let mut bytes = Vec::new();
        from.write_tls(&mut bytes).unwrap();
        let moved = !bytes.is_empty();
        let mut inbound = bytes.as_slice();
        while !inbound.is_empty() {
            to.read_tls(&mut inbound).unwrap();
            to.process_new_packets()?;
        }
        Ok(moved)
    }

    fn server_for(
        cert: &CertificateDer<'static>,
        key: PrivateKeyDer<'static>,
        versions: &[&'static SupportedProtocolVersion],
    ) -> Result<Connection, TlsError> {
        let provider = Arc::new(aws_lc_rs::default_provider());
        let signing_key = provider.key_provider.load_private_key(key)?;
        let resolver = SingleCertAndKey::from(CertifiedKey::new(vec![cert.clone()], signing_key));
        let config = rustls::ServerConfig::builder_with_provider(provider)
            .with_protocol_versions(versions)?
            .with_no_client_auth()
            .with_cert_resolver(Arc::new(resolver));
        Ok(rustls::ServerConnection::new(Arc::new(config))?.into())
    }

    fn client_for(cert: &CertificateDer<'static>) -> Result<Connection, TlsError> {
        let pin = Pin::Confirmed(fingerprint_of(cert));
        let name = ServerName::try_from("localhost").unwrap();
        Ok(rustls::ClientConnection::new(pinned_client_config(pin)?, name)?.into())
    }

    fn handshake(
        cert: &CertificateDer<'static>,
        key: PrivateKeyDer<'static>,
        versions: &[&'static SupportedProtocolVersion],
    ) -> Result<(), TlsError> {
        let mut server = server_for(cert, key, versions)?;
        let mut client = client_for(cert)?;
        while client.is_handshaking() || server.is_handshaking() {
            let sent = transfer(&mut client, &mut server)?;
            let replied = transfer(&mut server, &mut client)?;
            assert!(sent || replied, "handshake stalled");
        }
        Ok(())
    }

    #[test]
    fn a_signature_from_another_key_is_rejected_and_the_pinned_one_accepted() {
        let (cert, key) = generate_identity();
        let verifier = verifier_for(&cert);
        let forged = sign(&attacker_key(), TRANSCRIPT);
        let genuine = sign(&key, TRANSCRIPT);
        let bad = Err(TlsError::InvalidCertificate(CertificateError::BadSignature));

        let cases = [
            (
                verifier.verify_tls13_signature(TRANSCRIPT, &cert, &forged),
                bad.clone(),
            ),
            (
                verifier.verify_tls12_signature(TRANSCRIPT, &cert, &forged),
                bad,
            ),
            (
                verifier.verify_tls13_signature(TRANSCRIPT, &cert, &genuine),
                Ok(()),
            ),
            (
                verifier.verify_tls12_signature(TRANSCRIPT, &cert, &genuine),
                Ok(()),
            ),
        ];
        for (outcome, expected) in cases {
            assert_eq!(outcome.map(drop), expected);
        }
    }

    #[test]
    fn the_handshake_succeeds_only_for_the_pinned_certs_key_over_tls13() {
        let tls12: &[&'static SupportedProtocolVersion] = &[&rustls::version::TLS12];
        let cases = [
            (false, rustls::DEFAULT_VERSIONS, Ok(())),
            (
                true,
                rustls::DEFAULT_VERSIONS,
                Err(TlsError::InvalidCertificate(CertificateError::BadSignature)),
            ),
            (
                false,
                tls12,
                Err(TlsError::PeerIncompatible(
                    PeerIncompatible::Tls12NotOfferedOrEnabled,
                )),
            ),
        ];
        for (forged, versions, expected) in cases {
            let (cert, key) = generate_identity();
            let key = forged.then(attacker_key).unwrap_or(key);
            assert_eq!(handshake(&cert, key, versions), expected);
        }
    }
}
