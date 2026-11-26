//! WebKit's `SerializedScriptValue`, the bytes `IndexedDB` keeps a record in, read only as far as a
//! flat object of strings and numbers, which is what an editor draft is.

use std::collections::BTreeMap;

const OBJECT_TAG: u8 = 2;
const UNDEFINED_TAG: u8 = 3;
const NULL_TAG: u8 = 4;
const INT_TAG: u8 = 5;
const ZERO_TAG: u8 = 6;
const ONE_TAG: u8 = 7;
const FALSE_TAG: u8 = 8;
const TRUE_TAG: u8 = 9;
const DOUBLE_TAG: u8 = 10;
const STRING_TAG: u8 = 16;
const EMPTY_STRING_TAG: u8 = 17;
const TERMINATOR: u32 = 0xFFFF_FFFF;
const STRING_POOL: u32 = 0xFFFF_FFFE;
const EIGHT_BIT: u32 = 0x8000_0000;
const VERSION_LEN: usize = 4;
const INT_LEN: usize = 4;
const DOUBLE_LEN: usize = 8;
const SHORT_POOL: usize = 0xFF;
const MEDIUM_POOL: usize = 0xFFFF;

pub(crate) struct ScriptValue<'a> {
    bytes: &'a [u8],
    at: usize,
    pool: Vec<String>,
    strings: BTreeMap<String, String>,
}

impl ScriptValue<'_> {
    pub(crate) fn strings_of(bytes: &[u8]) -> Option<BTreeMap<String, String>> {
        let mut value = ScriptValue {
            bytes,
            at: VERSION_LEN,
            pool: Vec::new(),
            strings: BTreeMap::new(),
        };
        (value.byte()? == OBJECT_TAG).then_some(())?;
        let mut length = value.word()?;
        while length != TERMINATOR {
            let name = value.string(length)?;
            value.value_of(name)?;
            length = value.word()?;
        }
        Some(value.strings)
    }

    fn take(
        &mut self,
        len: usize,
    ) -> Option<&[u8]> {
        let end = self.at.checked_add(len)?;
        let taken = self.bytes.get(self.at..end)?;
        self.at = end;
        Some(taken)
    }

    fn byte(&mut self) -> Option<u8> {
        self.take(1)?.first().copied()
    }

    fn word(&mut self) -> Option<u32> {
        let bytes = self.take(INT_LEN)?;
        Some(u32::from_le_bytes(bytes.try_into().ok()?))
    }

    fn value_of(
        &mut self,
        name: String,
    ) -> Option<()> {
        let tag = self.byte()?;
        let skipped = match tag {
            STRING_TAG => {
                let length = self.word()?;
                let value = self.string(length)?;
                self.strings.insert(name, value);
                0
            },
            EMPTY_STRING_TAG => {
                self.strings.insert(name, String::new());
                0
            },
            INT_TAG => INT_LEN,
            DOUBLE_TAG => DOUBLE_LEN,
            UNDEFINED_TAG | NULL_TAG | ZERO_TAG | ONE_TAG | FALSE_TAG | TRUE_TAG => 0,
            _ => return None,
        };
        self.take(skipped).map(drop)
    }

    fn string(
        &mut self,
        length: u32,
    ) -> Option<String> {
        if length == STRING_POOL {
            return self.pooled();
        }
        let units = usize::try_from(length & !EIGHT_BIT).ok()?;
        let text = match length & EIGHT_BIT {
            0 => utf16(self.take(units.checked_mul(2)?)?)?,
            _ => self
                .take(units)?
                .iter()
                .map(|byte| char::from(*byte))
                .collect(),
        };
        self.pool.push(text.clone());
        Some(text)
    }

    fn pooled(&mut self) -> Option<String> {
        let index = match self.pool.len() {
            len if len <= SHORT_POOL => usize::from(self.byte()?),
            len if len <= MEDIUM_POOL => {
                usize::from(u16::from_le_bytes(self.take(2)?.try_into().ok()?))
            },
            _ => usize::try_from(self.word()?).ok()?,
        };
        self.pool.get(index).cloned()
    }
}

fn utf16(bytes: &[u8]) -> Option<String> {
    let units: Vec<u16> = bytes
        .as_chunks::<2>()
        .0
        .iter()
        .map(|pair| u16::from_le_bytes(*pair))
        .collect();
    String::from_utf16(&units).ok()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::{draft_bytes, latin1};

    fn wide(text: &str) -> Vec<u8> {
        let units: Vec<u16> = text.encode_utf16().collect();
        let mut bytes = u32::try_from(units.len()).unwrap().to_le_bytes().to_vec();
        bytes.extend(units.iter().flat_map(|unit| unit.to_le_bytes()));
        bytes
    }

    #[test]
    fn a_draft_reads_its_strings_in_latin1_utf16_or_from_the_pool() {
        let pooled = [STRING_POOL.to_le_bytes().as_slice(), &[3]].concat();
        let cases = [
            (latin1("b"), latin1("a"), "b", "a"),
            (wide("€uro"), latin1("euro"), "€uro", "euro"),
            (latin1("same"), pooled, "same", "same"),
        ];
        for (text, saved, expected_text, expected_saved) in cases {
            let strings =
                ScriptValue::strings_of(&draft_bytes("crowbar://host/%2Fa", &text, &saved))
                    .unwrap();
            assert_eq!(strings["uri"], "crowbar://host/%2Fa");
            assert_eq!(strings["text"], expected_text);
            assert_eq!(strings["savedText"], expected_saved);
        }
    }

    #[test]
    fn anything_but_a_flat_object_reads_as_nothing() {
        assert_eq!(
            ScriptValue::strings_of(&[0x0f, 0, 0, 0, 1]),
            None,
            "an array"
        );
        assert_eq!(
            ScriptValue::strings_of(&[0x0f, 0, 0, 0, OBJECT_TAG, 3]),
            None,
            "cut short"
        );
    }
}
