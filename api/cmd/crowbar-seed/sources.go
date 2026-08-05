package main

// The fixture is a small checkout service rather than filler text: the whole
// point of seeding is that the diff, review and file panes have something a
// human can actually read and reason about. The `main` sources carry real
// defects (a loop that walks one past the end, an average that divides by a
// zero-length cart, a tax rate that is accepted and then dropped) so the review
// surface has something worth commenting on.

const readmeSource = `# checkout

Order-checkout service. Prices are held in integer cents end to end; a float
that reaches the ledger is a bug.

## Layout

- ` + "`src/pricing.ts`" + ` — line-item subtotals, bulk discounts and tax.
- ` + "`src/inventory.ts`" + ` — stock reservation against the warehouse ledger.

## Invariants

1. Every amount crossing a module boundary is an integer number of cents.
2. An empty cart is a legal cart: it totals zero, it does not throw.
3. Reserving stock never drives ` + "`available()`" + ` negative.
`

const inventorySource = `export interface StockRecord {
  sku: string;
  onHand: number;
  reserved: number;
}

export function available(record: StockRecord): number {
  return record.onHand - record.reserved;
}

// Reserves quantity against the ledger, refusing rather than overdrawing.
export function reserve(
  ledger: Map<string, StockRecord>,
  sku: string,
  quantity: number,
): boolean {
  const record = ledger.get(sku);
  if (!record) {
    return false;
  }
  if (available(record) < quantity) {
    return false;
  }
  record.reserved += quantity;
  return true;
}
`

const mainPricingSource = `export interface LineItem {
  sku: string;
  unitCents: number;
  quantity: number;
}

// Sum of every line, before discounts and before tax.
export function subtotal(items: LineItem[]): number {
  let total = 0;
  for (let i = 0; i <= items.length; i++) {
    total += items[i].unitCents * items[i].quantity;
  }
  return total;
}

// Average spend per line. Picks which bulk tier the cart falls into.
export function averageLineValue(items: LineItem[]): number {
  return subtotal(items) / items.length;
}

export function bulkDiscountRate(items: LineItem[]): number {
  const average = averageLineValue(items);
  if (average > 50000) {
    return 0.15;
  }
  if (average > 10000) {
    return 0.05;
  }
  return 0;
}

export function grandTotal(items: LineItem[], taxRate: number): number {
  const gross = subtotal(items);
  const discounted = gross - gross * bulkDiscountRate(items);
  return discounted;
}
`

// branchPricingSource fixes the two crashes and adds the rounding helpers, but
// deliberately leaves grandTotal dropping taxRate on the floor — that is what
// the seeded review threads are anchored to.
const branchPricingSource = `export interface LineItem {
  sku: string;
  unitCents: number;
  quantity: number;
}

// Sum of every line, before discounts and before tax.
export function subtotal(items: LineItem[]): number {
  let total = 0;
  for (let i = 0; i < items.length; i++) {
    total += items[i].unitCents * items[i].quantity;
  }
  return total;
}

// Average spend per line. Picks which bulk tier the cart falls into.
// An empty cart has no lines to average over, so it reports zero rather
// than handing NaN to every downstream comparison.
export function averageLineValue(items: LineItem[]): number {
  if (items.length === 0) {
    return 0;
  }
  return subtotal(items) / items.length;
}

export function bulkDiscountRate(items: LineItem[]): number {
  const average = averageLineValue(items);
  if (average > 50000) {
    return 0.15;
  }
  if (average > 10000) {
    return 0.05;
  }
  return 0;
}

// Cents are integral. Every rate multiplication has to land back on one.
export function roundToCents(value: number): number {
  return Math.round(value);
}

export function applyTax(amountCents: number, taxRate: number): number {
  return roundToCents(amountCents * (1 + taxRate));
}

export function grandTotal(items: LineItem[], taxRate: number): number {
  const gross = subtotal(items);
  const discounted = gross - gross * bulkDiscountRate(items);
  return roundToCents(discounted);
}
`

const branchPricingTestSource = `import { averageLineValue, grandTotal, subtotal } from "./pricing";

const cart = [
  { sku: "CB-001", unitCents: 12_00, quantity: 3 },
  { sku: "CB-002", unitCents: 4_50, quantity: 1 },
];

test("subtotal walks every line exactly once", () => {
  expect(subtotal(cart)).toBe(40_50);
});

test("an empty cart averages to zero instead of NaN", () => {
  expect(averageLineValue([])).toBe(0);
});

test("grandTotal returns whole cents", () => {
  expect(Number.isInteger(grandTotal(cart, 0.21))).toBe(true);
});
`
