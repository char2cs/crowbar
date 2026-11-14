use super::*;

#[test]
fn keeps_title_and_description() {
    let heading = Heading::new("Hosts", "Add another machine.");
    assert_eq!(
        (heading.title.as_str(), heading.description.as_str()),
        ("Hosts", "Add another machine.")
    );
}
