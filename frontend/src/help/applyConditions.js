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

// The need a recipe left on this field of the condition (`field` is a path such as
// 'config.port'), or null.
export function needOf(cond, field) {
  return cond?.needs?.find((need) => need.field === field) ?? null
}

// The recipe's message while the field it needs is empty, or doesn't match the need's
// pattern (S4's admin path must start with "/"); '' once it is filled in, and for a
// field no recipe asked for. The form sets it as the field's custom validity, so Save
// is refused with this message until the field is filled in.
export function needError(cond, field) {
  const need = needOf(cond, field)
  if (!need) return ''
  const value = field.split('.').reduce((at, key) => at?.[key], cond)
  const text = value == null ? '' : String(value).trim()
  if (text === '') return need.message
  if (need.pattern && !new RegExp(need.pattern).test(text)) return need.message
  return ''
}
