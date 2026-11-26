//! Encodes mouse events the way xterm reports them.

use crate::input::modifiers::Modifiers;
use crate::input::pointer::mouse_action::MouseAction;
use crate::input::pointer::mouse_button::MouseButton;
use crate::input::pointer::mouse_encoding::MouseEncoding;
use crate::input::pointer::mouse_event::MouseEvent;
use crate::input::pointer::mouse_tracking::MouseTracking;

const COORDINATE_OFFSET: usize = 1;
const LEGACY_OFFSET: usize = 32;
const LEGACY_MAX_COORDINATE: usize = 223;
const MOTION_BIT: u8 = 32;
const SHIFT_BIT: u8 = 4;
const ALT_BIT: u8 = 8;
const CONTROL_BIT: u8 = 16;
const RELEASE_CODE: u8 = 3;
const WHEEL_UP_CODE: u8 = 64;
const WHEEL_DOWN_CODE: u8 = 65;
const LEGACY_PREFIX: &[u8] = b"\x1b[M";

pub(crate) fn encode_mouse(
    event: &MouseEvent,
    tracking: MouseTracking,
    encoding: MouseEncoding,
) -> Option<Vec<u8>> {
    if !wanted(event, tracking) {
        return None;
    }
    let code = button_code(event, encoding);
    let column = event.point.column + COORDINATE_OFFSET;
    let row = event.point.row + COORDINATE_OFFSET;
    match encoding {
        MouseEncoding::Sgr => Some(sgr_report(code, column, row, event.action)),
        MouseEncoding::Utf8 => Some(legacy_report(code, column, row, MouseEncoding::Utf8)),
        MouseEncoding::Legacy => fits_legacy(column, row)
            .then(|| legacy_report(code, column, row, MouseEncoding::Legacy)),
    }
}

fn fits_legacy(
    column: usize,
    row: usize,
) -> bool {
    column.max(row) <= LEGACY_MAX_COORDINATE
}

fn sgr_report(
    code: u8,
    column: usize,
    row: usize,
    action: MouseAction,
) -> Vec<u8> {
    let end = match action {
        MouseAction::Release => 'm',
        _ => 'M',
    };
    format!("\x1b[<{code};{column};{row}{end}").into_bytes()
}

fn wanted(
    event: &MouseEvent,
    tracking: MouseTracking,
) -> bool {
    let wheel = matches!(event.button, MouseButton::WheelUp | MouseButton::WheelDown);
    match (tracking, event.action) {
        (MouseTracking::Off, _) => false,
        (_, MouseAction::Press) => true,
        (_, MouseAction::Release) => !wheel,
        (MouseTracking::Click, _) => false,
        (MouseTracking::Drag, action) => action == MouseAction::Drag,
        (MouseTracking::Motion, _) => true,
    }
}

fn base_code(button: MouseButton) -> u8 {
    match button {
        MouseButton::Left => 0,
        MouseButton::Middle => 1,
        MouseButton::Right => 2,
        MouseButton::WheelUp => WHEEL_UP_CODE,
        MouseButton::WheelDown => WHEEL_DOWN_CODE,
        MouseButton::None => RELEASE_CODE,
    }
}

fn button_code(
    event: &MouseEvent,
    encoding: MouseEncoding,
) -> u8 {
    let moving = matches!(event.action, MouseAction::Drag | MouseAction::Move);
    let motion = MOTION_BIT * u8::from(moving);
    let button = match (event.action, encoding) {
        (MouseAction::Release, MouseEncoding::Legacy | MouseEncoding::Utf8) => RELEASE_CODE,
        _ => base_code(event.button),
    };
    button + motion + modifier_bits(event.modifiers)
}

fn modifier_bits(modifiers: Modifiers) -> u8 {
    let held = [
        (Modifiers::SHIFT, SHIFT_BIT),
        (Modifiers::ALT, ALT_BIT),
        (Modifiers::CONTROL, CONTROL_BIT),
    ];
    held.iter()
        .filter(|(modifier, _)| modifiers.contains(*modifier))
        .map(|(_, bit)| *bit)
        .sum()
}

fn legacy_report(
    code: u8,
    column: usize,
    row: usize,
    encoding: MouseEncoding,
) -> Vec<u8> {
    let mut bytes = LEGACY_PREFIX.to_vec();
    bytes.push(code.saturating_add(u8::try_from(LEGACY_OFFSET).unwrap_or(u8::MAX)));
    for coordinate in [column, row] {
        bytes.extend(coordinate_bytes(coordinate + LEGACY_OFFSET, encoding));
    }
    bytes
}

fn coordinate_bytes(
    value: usize,
    encoding: MouseEncoding,
) -> Vec<u8> {
    let character = u32::try_from(value).ok().and_then(char::from_u32);
    match (encoding, character) {
        (MouseEncoding::Utf8, Some(character)) => character.to_string().into_bytes(),
        _ => vec![u8::try_from(value).unwrap_or(u8::MAX)],
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::input::geometry::ViewPoint;

    use MouseAction::{Drag, Move, Press, Release};
    use MouseButton::{Left, None as NoButton, Right, WheelUp};
    use MouseEncoding::{Legacy, Sgr, Utf8};
    use MouseTracking::{Click, Drag as Dragging, Motion, Off};

    const FAR: usize = 300;

    type Case = (
        MouseButton,
        MouseAction,
        Modifiers,
        usize,
        MouseTracking,
        MouseEncoding,
    );

    fn report(case: Case) -> Option<Vec<u8>> {
        let (button, action, modifiers, column, tracking, encoding) = case;
        let point = ViewPoint { row: 4, column };
        let event = MouseEvent {
            button,
            action,
            point,
            modifiers,
        };
        encode_mouse(&event, tracking, encoding)
    }

    #[test]
    fn reports_follow_encoding_and_tracking_mode() {
        let no = Modifiers::NONE;
        let cases = [
            ((Left, Press, no, 9, Click, Sgr), Some("\x1b[<0;10;5M")),
            ((Left, Release, no, 9, Click, Sgr), Some("\x1b[<0;10;5m")),
            (
                (Right, Release, Modifiers::CONTROL, 9, Click, Legacy),
                Some("\x1b[M3*%"),
            ),
            ((Left, Drag, no, 9, Dragging, Sgr), Some("\x1b[<32;10;5M")),
            ((NoButton, Move, no, 9, Motion, Sgr), Some("\x1b[<35;10;5M")),
            ((WheelUp, Press, no, 9, Click, Sgr), Some("\x1b[<64;10;5M")),
            ((Left, Drag, no, 9, Click, Sgr), None),
            ((NoButton, Move, no, 9, Dragging, Sgr), None),
            ((WheelUp, Press, no, 9, Off, Sgr), None),
            ((Left, Press, no, FAR, Click, Legacy), None),
            ((Left, Press, no, FAR, Click, Utf8), Some("\x1b[M \u{14d}%")),
        ];
        for (case, expected) in cases {
            let wanted = expected.map(|text| text.as_bytes().to_vec());
            assert_eq!(report(case), wanted, "{case:?}");
        }
    }
}
