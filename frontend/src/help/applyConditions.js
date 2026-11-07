// Puts a recipe's conditions into the target form. Pure: no Vue, no DOM, and no import of
// recipes.js or any help content, because the target form imports this file statically.
//
// It always REPLACES the form's conditions (never merges) with deep copies, so editing the
// form afterwards cannot reach the recipe, and applying the same recipe twice gives fresh
// objects. check_id is blanked: the form saves these as new checks. `needs` (the fields the
// person must fill in) go on the copied condition objects, never inside `config`, which is
// what gets sent to the API.
//
// Returns the form's PREVIOUS values, for Undo.

// JSON copy, not structuredClone: the form's values may be Vue reactive proxies, which
// structuredClone refuses. Conditions hold only strings, numbers and booleans.
const copy = (x) => JSON.parse(JSON.stringify(x))

export function applyConditions(form, conditions, preferred, needs = []) {
  const previous = {
    conditions: copy(form.conditions || []),
    preferred_check_type: form.preferred_check_type || '',
  }
  const next = copy(conditions).map((c) => ({ ...c, check_id: '' }))
  for (const need of needs) {
    const target = next[need.condition]
    if (!target) continue
    const { condition, ...rest } = need
    ;(target.needs ||= []).push(rest)
  }
  form.conditions = next
  form.preferred_check_type = preferred
  return previous
}
