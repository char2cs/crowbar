use x11rb::connection::Connection as _;
use x11rb::errors::ReplyError;
use x11rb::protocol::xproto::{ConnectionExt as _, GetKeyboardMappingReply, Keycode, Keysym};
use x11rb::rust_connection::RustConnection;

use crate::error::Error;
use crate::linux::keyboard::us_key::UsKey;

const UNICODE_KEYSYM: u32 = 0x0100_0000;
const LATIN_1_END: u32 = 0xff;
const SHIFT_LEVEL: usize = 1;
const FIRST_DIGIT: Keycode = 10;
const ZERO: Keycode = 19;
const MINUS: Keycode = 20;
const EQUALS: Keycode = 21;

/// The X server's keyboard mapping, asked again on each lookup so a layout switch counts at once.
///
/// GPUI names a shifted key by the character it types (`@`, not `shift-2`); a shortcut that
/// means a key's place, as develop's `KeyboardEvent.code` does, finds that place here.
#[derive(Debug)]
pub struct KeyMap {
    connection: RustConnection,
}

impl KeyMap {
    /// Connects to the X server named by `DISPLAY`.
    ///
    /// # Errors
    /// [`Error::X11Connect`] when no X server answers.
    pub fn connect() -> Result<Self, Error> {
        let (connection, _screen) = RustConnection::connect(None)?;
        Ok(Self { connection })
    }

    /// The key that types `typed`, a single character, on the active layout: the first one
    /// that types it unshifted, else the first that types it with shift.
    ///
    /// # Errors
    /// [`Error::KeyMap`] when the server does not answer.
    pub fn us_key_of(
        &self,
        typed: char,
    ) -> Result<Option<UsKey>, Error> {
        let mapping = self.mapping()?;
        let first = self.connection.setup().min_keycode;
        Ok(place_of(&mapping, first, keysym_of(typed)))
    }

    fn mapping(&self) -> Result<GetKeyboardMappingReply, Error> {
        let setup = self.connection.setup();
        let count = setup.max_keycode - setup.min_keycode + 1;
        self.connection
            .get_keyboard_mapping(setup.min_keycode, count)
            .map_err(ReplyError::from)
            .and_then(x11rb::cookie::Cookie::reply)
            .map_err(Error::KeyMap)
    }
}

fn keysym_of(typed: char) -> Keysym {
    let code = u32::from(typed);
    if code <= LATIN_1_END {
        return code;
    }
    UNICODE_KEYSYM | code
}

fn place_of(
    mapping: &GetKeyboardMappingReply,
    first: Keycode,
    wanted: Keysym,
) -> Option<UsKey> {
    let width = usize::from(mapping.keysyms_per_keycode).max(1);
    let rows = mapping.keysyms.chunks(width).zip(first..=Keycode::MAX);
    let levels = |level: usize| {
        rows.clone()
            .find(|(syms, _)| syms.get(level) == Some(&wanted))
            .map(|(syms, code)| (syms, code, level == SHIFT_LEVEL))
    };
    let (syms, code, shifted) = levels(0).or_else(|| levels(SHIFT_LEVEL))?;
    let key = us_unshifted(code).or_else(|| ascii(syms.first().copied()))?;
    Some(UsKey { key, shifted })
}

fn us_unshifted(code: Keycode) -> Option<char> {
    match code {
        FIRST_DIGIT..ZERO => char::from_digit(u32::from(code - FIRST_DIGIT + 1), 10),
        ZERO => Some('0'),
        MINUS => Some('-'),
        EQUALS => Some('='),
        _ => None,
    }
}

fn ascii(keysym: Option<Keysym>) -> Option<char> {
    let keysym = u8::try_from(keysym?).ok()?;
    keysym.is_ascii_graphic().then(|| char::from(keysym))
}

#[cfg(test)]
mod tests {
    use super::*;

    const FIRST: Keycode = 8;
    const Q: Keycode = 24;

    fn mapping(rows: &[[Keysym; 2]]) -> GetKeyboardMappingReply {
        GetKeyboardMappingReply {
            keysyms_per_keycode: 2,
            keysyms: rows.iter().flatten().copied().collect(),
            ..GetKeyboardMappingReply::default()
        }
    }

    fn us_digit_row() -> GetKeyboardMappingReply {
        let mut rows = vec![[0, 0]; usize::from(FIRST_DIGIT - FIRST)];
        rows.extend([
            [u32::from('1'), u32::from('!')],
            [u32::from('2'), u32::from('@')],
        ]);
        rows.resize(usize::from(Q - FIRST), [0, 0]);
        rows.push([u32::from('q'), u32::from('Q')]);
        mapping(&rows)
    }

    #[test]
    fn a_typed_character_names_the_us_key_under_it() {
        let place = |key, shifted| Some(UsKey { key, shifted });
        let cases = [
            ("a shifted symbol names its digit", '@', place('2', true)),
            ("a letter names its own key", 'q', place('q', false)),
            ("a character no key types", '€', None),
        ];
        for (case, typed, expected) in cases {
            let found = place_of(&us_digit_row(), FIRST, keysym_of(typed));
            assert_eq!(found, expected, "{case}");
        }
    }

    #[test]
    fn a_table_that_runs_to_the_last_keycode_is_searched_to_its_end() {
        let full = mapping(&vec![[0, 0]; usize::from(Keycode::MAX - FIRST) + 1]);
        assert_eq!(place_of(&full, FIRST, keysym_of('|')), None);
    }
}
