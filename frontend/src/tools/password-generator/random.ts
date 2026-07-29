/** Fills the array with cryptographically strong random 32-bit values and
 *  returns it. Injected so the tests can drive a scripted sequence. */
export type RandomFill = (out: Uint32Array) => Uint32Array

// Math.random is a fast non-cryptographic generator whose output can be worked
// out from a handful of previous values, and everything this tool produces is a
// real credential that gets typed into a real firewall. It is never used here.
export const cryptoFill: RandomFill = (out) => crypto.getRandomValues(out)

/** A uniform integer in [0, limit). limit must be at least 1. */
export function randomIndex(limit: number, fill: RandomFill = cryptoFill): number {
  // Everything from ceiling upwards would make some results more likely than
  // others, so a draw that lands there is discarded and taken again.
  const ceiling = Math.floor(4294967296 / limit) * limit
  const buffer = new Uint32Array(1)
  for (;;) {
    fill(buffer)
    if (buffer[0] < ceiling) return buffer[0] % limit
  }
}

/** Fisher-Yates, in place, driven by the same unbiased source. Returns items. */
export function shuffle<T>(items: T[], fill: RandomFill = cryptoFill): T[] {
  for (let i = items.length - 1; i > 0; i--) {
    const j = randomIndex(i + 1, fill)
    const held = items[i]
    items[i] = items[j]
    items[j] = held
  }
  return items
}
