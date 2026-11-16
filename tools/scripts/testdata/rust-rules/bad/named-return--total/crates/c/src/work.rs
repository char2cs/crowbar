/// Works.
pub fn work(items: &[u32]) -> u32 {
    let mut total = 0;
    for item in items {
        total += item;
    }
    total
}

#[cfg(test)]
mod tests {}
