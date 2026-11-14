use super::*;

#[test]
fn counts_every_written_byte() {
    let mut counter = ByteCounter(0);
    counter.write_all(b"abc").unwrap();
    counter.write_all(b"de").unwrap();
    counter.flush().unwrap();
    assert_eq!(counter.0, 5, "bytes written");
}
