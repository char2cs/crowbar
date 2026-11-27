/// Works.
pub fn work(items: &[u32]) -> Vec<u32> {
    let mut doubled = Vec::new();
    for item in items {
        doubled.push(item + item);
    }
    doubled
}

#[cfg(test)]
mod tests {}
