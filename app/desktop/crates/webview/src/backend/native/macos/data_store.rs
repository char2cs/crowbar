use protocol::ExtensionId;

pub(crate) const STORE_ID_BYTES: usize = 16;
pub(crate) const BROWSER_STORE: [u8; STORE_ID_BYTES] = *b"crowbar.browser!";

pub(crate) fn store_identifier(extension: &ExtensionId) -> [u8; STORE_ID_BYTES] {
    const OFFSET_BASIS: u128 = 0x6c62_2727_e07b_b014_62b8_2175_6295_c58d;
    const PRIME: u128 = 0x0000_0000_0100_0000_0000_0000_0000_013b;
    let hash = extension.as_str().bytes().fold(OFFSET_BASIS, |hash, byte| {
        (hash ^ u128::from(byte)).wrapping_mul(PRIME)
    });
    hash.to_be_bytes()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn extensions_get_distinct_stable_store_identifiers() {
        let git = ExtensionId::new("git").unwrap();
        let draw = ExtensionId::new("draw").unwrap();
        assert_eq!(store_identifier(&git), store_identifier(&git), "stable");
        assert_ne!(store_identifier(&git), store_identifier(&draw), "distinct");
    }
}
