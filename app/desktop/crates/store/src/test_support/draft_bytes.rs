const OBJECT_TAG: u8 = 2;
const DOUBLE_TAG: u8 = 10;
const STRING_TAG: u8 = 16;
const TERMINATOR: u32 = 0xFFFF_FFFF;
const EIGHT_BIT: u32 = 0x8000_0000;
const VERSION: [u8; 4] = [0x0f, 0, 0, 0];
const UPDATED_AT: f64 = 1_700_000_000_000.0;

/// `text` as WebKit writes a Latin-1 string: its length with the 8-bit flag, then its bytes.
pub(crate) fn latin1(text: &str) -> Vec<u8> {
    let length = u32::try_from(text.len()).unwrap_or(0) | EIGHT_BIT;
    let mut bytes = length.to_le_bytes().to_vec();
    bytes.extend(text.bytes());
    bytes
}

/// A draft laid out as WebKit 2.4x wrote one for develop: version 15, an object of `uri`,
/// `text` and `savedText` (each already encoded), and `updatedAt`, then the terminator.
pub(crate) fn draft_bytes(
    uri: &str,
    text: &[u8],
    saved: &[u8],
) -> Vec<u8> {
    let mut bytes = VERSION.to_vec();
    bytes.push(OBJECT_TAG);
    let fields = [
        ("uri", latin1(uri)),
        ("text", text.to_vec()),
        ("savedText", saved.to_vec()),
    ];
    for (name, value) in fields {
        bytes.extend(latin1(name));
        bytes.push(STRING_TAG);
        bytes.extend(value);
    }
    bytes.extend(latin1("updatedAt"));
    bytes.push(DOUBLE_TAG);
    bytes.extend(UPDATED_AT.to_le_bytes());
    bytes.extend(TERMINATOR.to_le_bytes());
    bytes
}
