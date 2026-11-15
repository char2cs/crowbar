use sha2::{Digest, Sha256};

const DIGEST_LEN: usize = 32;

pub(crate) type CertificateDigest = [u8; DIGEST_LEN];

const DISPLAYED_BYTES: usize = 12;
const GROUP_CHARS: usize = 4;

pub(crate) fn certificate_digest(der: &[u8]) -> CertificateDigest {
    Sha256::digest(der).into()
}

pub(crate) fn hex(bytes: &[u8]) -> String {
    bytes
        .iter()
        .map(|b| format!("{b:02x}"))
        .collect::<Vec<_>>()
        .concat()
}

pub(crate) fn fingerprint_of(der: &[u8]) -> String {
    display_fingerprint(&certificate_digest(der))
}

pub(crate) fn display_fingerprint(digest: &CertificateDigest) -> String {
    let (shown, _) = digest.split_at(DISPLAYED_BYTES);
    let groups: Vec<String> = hex(shown)
        .as_bytes()
        .chunks(GROUP_CHARS)
        .map(|group| String::from_utf8_lossy(group).into_owned())
        .collect();
    format!("SHA256:{}", groups.join(" "))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_fingerprint_has_the_go_side_format() {
        let fingerprint = fingerprint_of(b"anything");
        let body = fingerprint.strip_prefix("SHA256:").expect("prefix");
        let groups: Vec<&str> = body.split(' ').collect();
        assert_eq!(groups.len(), 6, "six groups: {groups:?}");
        for group in groups {
            assert_eq!(group.len(), 4, "four chars: {group:?}");
            assert!(group
                .chars()
                .all(|c| c.is_ascii_hexdigit() && !c.is_uppercase()));
        }
    }

    #[test]
    fn equal_input_gives_equal_fingerprints_and_different_input_does_not() {
        type Case = (&'static [u8], &'static [u8], bool);
        const CASES: &[Case] = &[
            (b"a", b"a", true),
            (b"a", b"b", false),
            (
                b"some fake certificate bytes",
                b"some fake certificate bytes",
                true,
            ),
        ];
        for (left, right, same) in CASES {
            assert_eq!(fingerprint_of(left) == fingerprint_of(right), *same);
        }
    }

    #[test]
    fn hex_is_lowercase_and_zero_padded() {
        const CASES: &[(&[u8], &str)] = &[(&[], ""), (&[0, 15, 255], "000fff"), (&[171], "ab")];
        for (bytes, expected) in CASES {
            assert_eq!(hex(bytes), *expected);
        }
    }

    #[test]
    fn the_digest_is_the_whole_sha256() {
        assert_eq!(
            hex(&certificate_digest(b"abc")),
            "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
        );
    }
}
