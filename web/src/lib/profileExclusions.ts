// Case-insensitive add/remove for a single exclusion list — excluded_sources or
// excluded_companies. Unlike skills (see profileSkills.ts), neither has a "wanted"
// counterpart list to reconcile against, so a toggle only ever touches the one list.

const same = (a: string, b: string) => a.toLowerCase() === b.toLowerCase();

/** The list once `value` is added, or unchanged if already present. */
export function withExcluded(list: string[], value: string): string[] {
  return list.some((v) => same(v, value)) ? list : [...list, value];
}

/** The list once `value` is removed. A no-op if it was not present. */
export function withoutExcluded(list: string[], value: string): string[] {
  return list.filter((v) => !same(v, value));
}
