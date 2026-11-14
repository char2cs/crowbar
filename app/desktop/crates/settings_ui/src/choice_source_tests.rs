use super::*;

#[test]
fn fixed_pairs_become_choices_in_order() {
    let source = ChoiceSource::fixed(&[("left", "Left"), ("right", "Right")]);
    assert_eq!(
        source,
        ChoiceSource::Fixed(vec![
            Choice::new("left", "Left"),
            Choice::new("right", "Right")
        ])
    );
    assert_eq!(
        ChoiceSource::provided("themes"),
        ChoiceSource::Provided("themes".to_owned())
    );
}
