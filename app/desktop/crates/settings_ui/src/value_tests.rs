use super::*;

#[test]
fn accessors_return_only_their_own_shape() {
    let values = [
        Value::from(true),
        Value::from("x"),
        Value::from(2.5),
        Value::List(vec!["a".to_owned()]),
        Value::Map(BTreeMap::from([("k".to_owned(), "v".to_owned())])),
    ];
    let shapes: Vec<[bool; 5]> = values
        .iter()
        .map(|value| {
            [
                value.as_bool().is_some(),
                value.as_text().is_some(),
                value.as_number().is_some(),
                value.as_list().is_some(),
                value.as_map().is_some(),
            ]
        })
        .collect();
    for (index, shape) in shapes.iter().enumerate() {
        let expected: Vec<bool> = (0..5).map(|slot| slot == index).collect();
        assert_eq!(shape.to_vec(), expected, "value {index}");
    }
}
