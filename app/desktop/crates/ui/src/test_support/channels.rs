//! Colour channel conversion for assertions.

/// Rounds a 0..=1 channel to its byte.
pub(crate) fn channel_byte(channel: f32) -> u8 {
    let scaled = (channel * f32::from(u8::MAX)).round();
    (0..=u8::MAX)
        .find(|byte| f32::from(*byte) >= scaled)
        .unwrap_or(u8::MAX)
}
