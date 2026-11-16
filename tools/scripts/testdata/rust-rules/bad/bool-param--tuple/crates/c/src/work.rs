/// Works.
pub fn work(
    (words, whole): (&[String], bool),
) -> usize {
    words.len() + usize::from(whole)
}

#[cfg(test)]
mod tests {}
