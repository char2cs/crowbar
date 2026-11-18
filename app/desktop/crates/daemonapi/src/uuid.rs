use std::fmt;

const UUID_BYTES: usize = 16;
const VERSION_BYTE: usize = 6;
const VARIANT_BYTE: usize = 8;
const KEEP_LOW_NIBBLE: u8 = 0x0f;
const VERSION_4: u8 = 0x40;
const KEEP_LOW_SIX: u8 = 0x3f;
const VARIANT_RFC: u8 = 0x80;
const GROUP_SIZES: [usize; 5] = [4, 2, 2, 2, 6];

/// A random version-4 UUID minted by the client: chats and operations are created under an id
/// the caller chooses.
///
/// It displays in its hyphenated lowercase form.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Uuid {
    bytes: [u8; UUID_BYTES],
}

impl Uuid {
    /// A fresh random UUID.
    ///
    /// Without system randomness it logs an error and the id is not random.
    pub fn random() -> Self {
        let mut bytes = [0_u8; UUID_BYTES];
        if let Err(error) = getrandom::fill(&mut bytes) {
            tracing::error!(%error, "the system has no randomness; ids repeat per second");
        }
        stamp(&mut bytes);
        Self { bytes }
    }
}

impl fmt::Display for Uuid {
    fn fmt(
        &self,
        formatter: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        formatter.write_str(&hyphenate(&self.bytes))
    }
}

fn stamp(bytes: &mut [u8; UUID_BYTES]) {
    bytes[VERSION_BYTE] = (bytes[VERSION_BYTE] & KEEP_LOW_NIBBLE) | VERSION_4;
    bytes[VARIANT_BYTE] = (bytes[VARIANT_BYTE] & KEEP_LOW_SIX) | VARIANT_RFC;
}

fn hyphenate(bytes: &[u8; UUID_BYTES]) -> String {
    let mut rest = bytes.as_slice();
    let mut groups = Vec::with_capacity(GROUP_SIZES.len());
    for size in GROUP_SIZES {
        let (group, tail) = rest.split_at(size);
        groups.push(
            group
                .iter()
                .fold(String::new(), |text, byte| text + &format!("{byte:02x}")),
        );
        rest = tail;
    }
    groups.join("-")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_uuid_has_the_v4_shape_and_ids_differ() {
        let id = Uuid::random().to_string();
        let sizes: Vec<usize> = id.split('-').map(str::len).collect();
        assert_eq!(sizes, [8, 4, 4, 4, 12]);
        let parts: Vec<&str> = id.split('-').collect();
        assert!(parts[2].starts_with('4'));
        assert!(parts[3].starts_with(['8', '9', 'a', 'b']));
        assert_ne!(Uuid::random(), Uuid::random());
    }
}
