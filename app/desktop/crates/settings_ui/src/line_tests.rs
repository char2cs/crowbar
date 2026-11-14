use super::*;

#[test]
fn builders_change_family_and_tone() {
    let line = Line::plain("homelab:3737").mono().toned(Tone::Error);
    assert_eq!(
        (line.text.as_str(), line.mono, line.tone),
        ("homelab:3737", true, Tone::Error)
    );
    assert_eq!(Line::plain("x").tone, Tone::Neutral);
}
