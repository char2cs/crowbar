//! Colour channel conversion for assertions.

/// Rounds a 0..=1 channel to its byte.
#[allow(
    clippy::cast_possible_truncation,
    clippy::cast_sign_loss,
    reason = "test helper over channels already in 0..=1"
)]
pub(crate) fn channel_byte(channel: f32) -> u8 {
    (channel * 255.0).round() as u8
}
