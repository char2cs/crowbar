use super::*;

#[test]
fn the_message_cap_applies_to_whole_messages_and_frames() {
    let config = ws_config(1024);
    assert_eq!(config.max_message_size, Some(1024));
    assert_eq!(config.max_frame_size, Some(1024));
}
