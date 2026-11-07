/** Validation of the exact subset sent to atomic acceptance. */
export function dependencyProblems(
  items: { ref?: string; title: string; depends_on?: string[] }[],
  selected: ReadonlySet<string>,
): string[] {
  const byRef = new Map(items.map((p) => [p.ref!, p]));
  const errors = new Set<string>();
  const visiting = new Set<string>(),
    visited = new Set<string>();
  function visit(ref: string) {
    const item = byRef.get(ref);
    if (!item) return;
    if (visiting.has(ref)) {
      errors.add(
        `Dependency cycle includes “${item.title || ref}”. Remove a link before accepting.`,
      );
      return;
    }
    if (visited.has(ref)) return;
    visiting.add(ref);
    for (const dep of item.depends_on ?? []) {
      if (!selected.has(dep) || !byRef.has(dep)) {
        errors.add(
          `“${item.title || ref}” depends on “${byRef.get(dep)?.title || dep}”, which is not selected. Select it or remove the dependency.`,
        );
      } else visit(dep);
    }
    visiting.delete(ref);
    visited.add(ref);
  }
  for (const ref of selected) visit(ref);
  return [...errors];
}
