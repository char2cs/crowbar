//! Who gets the mouse wheel.

use crate::modifiers::Modifiers;
use crate::mouse_tracking::MouseTracking;
use crate::screen_buffer::ScreenBuffer;

/// Where a wheel turn goes.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum WheelRoute {
    /// The terminal scrolls its own history.
    Scrollback,
    /// The program tracks the mouse and gets wheel reports.
    ReportToProgram,
    /// The alternate screen has no history; the wheel becomes arrow keys.
    ArrowKeys,
}

/// Routes by what the program declared, not by which buffer it is on: a TUI may track the mouse on the
/// primary buffer. Shift on the primary buffer takes the wheel back for the scrollback.
pub fn route_wheel(
    tracking: MouseTracking,
    buffer: ScreenBuffer,
    modifiers: Modifiers,
) -> WheelRoute {
    let primary = buffer == ScreenBuffer::Primary;
    let reclaimed = modifiers.contains(Modifiers::SHIFT) && primary;
    match (tracking, primary, reclaimed) {
        (MouseTracking::Off, true, _) | (_, _, true) => WheelRoute::Scrollback,
        (MouseTracking::Off, false, _) => WheelRoute::ArrowKeys,
        (_, _, false) => WheelRoute::ReportToProgram,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const CASES: [(MouseTracking, ScreenBuffer, Modifiers, WheelRoute); 5] = [
        (
            MouseTracking::Off,
            ScreenBuffer::Primary,
            Modifiers::NONE,
            WheelRoute::Scrollback,
        ),
        (
            MouseTracking::Off,
            ScreenBuffer::Alternate,
            Modifiers::NONE,
            WheelRoute::ArrowKeys,
        ),
        (
            MouseTracking::Click,
            ScreenBuffer::Primary,
            Modifiers::NONE,
            WheelRoute::ReportToProgram,
        ),
        (
            MouseTracking::Click,
            ScreenBuffer::Primary,
            Modifiers::SHIFT,
            WheelRoute::Scrollback,
        ),
        (
            MouseTracking::Motion,
            ScreenBuffer::Alternate,
            Modifiers::SHIFT,
            WheelRoute::ReportToProgram,
        ),
    ];

    #[test]
    fn routing_follows_the_programs_declared_tracking() {
        let cases = CASES;
        for (tracking, buffer, modifiers, expected) in cases {
            assert_eq!(
                route_wheel(tracking, buffer, modifiers),
                expected,
                "{tracking:?} {buffer:?} {modifiers:?}"
            );
        }
    }
}
