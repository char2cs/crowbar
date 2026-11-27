use std::collections::BTreeMap;

#[derive(Debug, Clone, PartialEq)]
#[non_exhaustive]
pub enum Value {
    Bool(bool),
    Text(String),
    Number(f64),
    List(Vec<String>),
    Map(BTreeMap<String, String>),
}

impl Value {
    pub fn as_bool(&self) -> Option<bool> {
        match self {
            Self::Bool(flag) => Some(*flag),
            _ => None,
        }
    }

    pub fn as_text(&self) -> Option<&str> {
        match self {
            Self::Text(text) => Some(text),
            _ => None,
        }
    }

    pub fn as_number(&self) -> Option<f64> {
        match self {
            Self::Number(number) => Some(*number),
            _ => None,
        }
    }

    pub fn as_list(&self) -> Option<&[String]> {
        match self {
            Self::List(items) => Some(items),
            _ => None,
        }
    }

    pub(crate) fn as_map(&self) -> Option<&BTreeMap<String, String>> {
        match self {
            Self::Map(pairs) => Some(pairs),
            _ => None,
        }
    }
}

impl From<&str> for Value {
    fn from(text: &str) -> Self {
        Self::Text(text.to_owned())
    }
}

impl From<f64> for Value {
    fn from(number: f64) -> Self {
        Self::Number(number)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn accessors_return_only_their_own_shape() {
        let values = [
            Value::Bool(true),
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
}
