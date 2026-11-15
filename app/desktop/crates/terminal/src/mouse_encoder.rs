//! Encodes mouse events the way xterm reports them.

use crate::modifiers::Modifiers;
use crate::mouse_action::MouseAction;
use crate::mouse_button::MouseButton;
use crate::mouse_encoding::MouseEncoding;
use crate::mouse_event::MouseEvent;
use crate::mouse_tracking::MouseTracking;

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
    let motion = match moving {
        true => MOTION_BIT,
        false => 0,
    };
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
    use crate::view_point::ViewPoint;

    fn event(
        button: MouseButton,
        action: MouseAction,
        modifiers: Modifiers,
    ) -> MouseEvent {
        MouseEvent {
            button,
            action,
            point: ViewPoint { row: 4, column: 9 },
            modifiers,
        }
    }

    type Reported = (MouseEvent, MouseTracking, MouseEncoding, Vec<u8>);
    type Ignored = (MouseEvent, MouseTracking, MouseEncoding);

    fn press(button: MouseButton) -> MouseEvent {
        event(button, MouseAction::Press, Modifiers::NONE)
    }

    fn action(action: MouseAction) -> MouseEvent {
        event(MouseButton::Left, action, Modifiers::NONE)
    }

    fn wide() -> MouseEvent {
        let mut far = press(MouseButton::Left);
        far.point.column = 300;
        far
    }

    fn reported() -> Vec<Reported> {
        let ctrl_release = event(MouseButton::Right, MouseAction::Release, Modifiers::CONTROL);
        let (click, drag) = (MouseTracking::Click, MouseTracking::Drag);
        let (sgr, legacy) = (MouseEncoding::Sgr, MouseEncoding::Legacy);
        vec![
            (
                press(MouseButton::Left),
                click,
                sgr,
                b"\x1b[<0;10;5M".to_vec(),
            ),
            (
                action(MouseAction::Release),
                click,
                sgr,
                b"\x1b[<0;10;5m".to_vec(),
            ),
            (
                ctrl_release,
                click,
                legacy,
                vec![0x1b, b'[', b'M', 51, 42, 37],
            ),
            (
                action(MouseAction::Drag),
                drag,
                sgr,
                b"\x1b[<32;10;5M".to_vec(),
            ),
        ]
    }

    fn reported_motion() -> Vec<Reported> {
        let hover = event(MouseButton::None, MouseAction::Move, Modifiers::NONE);
        let (click, motion, sgr) = (
            MouseTracking::Click,
            MouseTracking::Motion,
            MouseEncoding::Sgr,
        );
        vec![
            (hover, motion, sgr, b"\x1b[<35;10;5M".to_vec()),
            (
                press(MouseButton::WheelUp),
                click,
                sgr,
                b"\x1b[<64;10;5M".to_vec(),
            ),
        ]
    }

    fn ignored() -> Vec<Ignored> {
        let hover = event(MouseButton::None, MouseAction::Move, Modifiers::NONE);
        let (sgr, legacy) = (MouseEncoding::Sgr, MouseEncoding::Legacy);
        vec![
            (action(MouseAction::Drag), MouseTracking::Click, sgr),
            (hover, MouseTracking::Drag, sgr),
            (press(MouseButton::WheelUp), MouseTracking::Off, sgr),
            (wide(), MouseTracking::Click, legacy),
        ]
    }

    #[test]
    fn reports_follow_encoding_and_tracking_mode() {
        for (event, tracking, encoding, expected) in reported().into_iter().chain(reported_motion())
        {
            assert_eq!(
                encode_mouse(&event, tracking, encoding),
                Some(expected),
                "{event:?}"
            );
        }
        for (event, tracking, encoding) in ignored() {
            assert_eq!(encode_mouse(&event, tracking, encoding), None, "{event:?}");
        }
    }

    #[test]
    fn utf8_encoding_reaches_far_cells() {
        assert!(encode_mouse(&wide(), MouseTracking::Click, MouseEncoding::Utf8).is_some());
    }
}
