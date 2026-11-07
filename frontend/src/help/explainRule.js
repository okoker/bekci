// Reads an alert rule back in plain words and flags settings that cause false alarms.
// Pure: no Vue, no DOM, plain strings only (the caller renders them with {{ }}).
// Follows the engine (internal/engine/engine.go): groups are OR'd; inside a group
// the operator is the LAST condition's (engine.go:76-79), empty = AND; a condition
// with Fail Count N > 1 needs N matching results in a row, one non-match breaks the run.

const ORDINALS = ['', 'first', 'second', 'third', 'fourth', 'fifth', 'sixth', 'seventh', 'eighth', 'ninth', 'tenth']
const ordinal = (n) => ORDINALS[n] || `${n}th`

const TYPE_LABEL = {
  http: 'HTTP', tcp: 'TCP', ping: 'Ping', dns: 'DNS', page_hash: 'Page Hash',
  tls_cert: 'TLS certificate', snmp_v2c: 'SNMP', snmp_v3: 'SNMP',
}
const FIELD_LABEL = { status: 'Status', response_ms: 'Response (ms)', 'metrics.packet_loss': 'Packet loss (%)' }
const COMPARATOR_WORDS = { eq: 'equals', neq: 'not equals', gt: 'greater than', lt: 'less than', gte: 'at least', lte: 'at most' }

const intervalOf = (c) => {
  const n = Number(c.interval_s)
  return Number.isFinite(n) && n > 0 ? n : 60
}
const failCountOf = (c) => {
  const n = Math.floor(Number(c.fail_count))
  return Number.isFinite(n) && n >= 1 ? n : 1
}
const cfg = (c) => (c.config && typeof c.config === 'object' ? c.config : {})
const isStatus = (c, v) => c.field === 'status' && c.comparator === 'eq' && c.value === v

// "2 to 3 minutes" / "90 to 135 seconds" for a run of N checks every `intervalS`.
function delayRange(failCount, intervalS) {
  const lo = (failCount - 1) * intervalS
  const hi = failCount * intervalS
  return intervalS % 60 === 0 ? `${lo / 60} to ${hi / 60} minutes` : `${lo} to ${hi} seconds`
}

function everyPhrase(intervalS) {
  if (intervalS === 60) return 'one check a minute'
  if (intervalS % 60 === 0) return `one check every ${intervalS / 60} minutes`
  return intervalS === 1 ? 'one check every second' : `one check every ${intervalS} seconds`
}

export function outageDelay(failCount, intervalS) {
  const n = Math.floor(Number(failCount))
  const i = Number(intervalS)
  if (!(n > 1) || !(i > 0)) return null
  return `${delayRange(n, i)} into an outage, at ${everyPhrase(i)}`
}

export function failCountTimeline(failCount, intervalS) {
  const n = Math.max(1, Math.floor(Number(failCount)) || 1)
  const sample = [true, false, false, true, false, false, false, false] // true = check passed
  let run = 0
  const steps = sample.map((ok) => {
    if (ok) {
      const label = run > 0 && run < n ? 'reset' : ''
      run = 0
      return { result: 'ok', label }
    }
    run++
    return { result: 'fail', label: run >= n ? 'down' : String(run) }
  })
  let summary
  if (n === 1) {
    summary = 'Every failed check alerts at once, however brief the blip.'
  } else {
    const never = n === 2 ? 'A single failure never alerts.'
      : n === 3 ? 'One or two failures in a row never alert.'
        : `Up to ${n - 1} failures in a row never alert.`
    summary = `${never} A real outage alerts on the ${ordinal(n)} failed check, ${delayRange(n, Number(intervalS) > 0 ? Number(intervalS) : 60)} in.`
  }
  return { steps, summary }
}

function subjectOf(c) {
  const k = cfg(c)
  switch (c.check_type) {
    case 'ping': return 'ping'
    case 'http': return k.endpoint && k.endpoint !== '/' ? `the page ${k.endpoint}` : 'the web page'
    case 'tcp': return `port ${k.port}`
    case 'dns': return k.nameserver ? `the DNS answer at ${k.nameserver}` : 'the DNS answer'
    case 'page_hash': return "the page's content"
    case 'tls_cert': return 'the certificate'
    case 'snmp_v2c':
    case 'snmp_v3': return 'SNMP'
    default: return c.check_type || 'this check'
  }
}

// The participle that follows "has" / "have": what the check did.
function phraseOf(c) {
  const k = cfg(c)
  if (c.field === 'response_ms' && c.comparator === 'gt') return `taken longer than ${c.value} ms`
  if (c.field === 'metrics.packet_loss' && c.comparator === 'gt') return `lost more than ${c.value}% of its pings`
  if (c.field === 'status' && c.comparator === 'eq' && (c.value === 'down' || c.value === 'up')) {
    const down = c.value === 'down'
    switch (c.check_type) {
      case 'http': return `${down ? 'not answered' : 'answered'} with ${k.expect_status || 200}`
      case 'tcp': return (down !== !!k.expect_closed) ? 'refused connections' : 'accepted a connection'
      case 'dns':
        if (k.expect_value) return down ? `given an address other than ${k.expect_value}` : `given ${k.expect_value}`
        return down ? 'failed to resolve' : 'resolved'
      case 'page_hash': return down ? 'changed, or could not be read' : 'matched its baseline'
      case 'tls_cert':
        return down ? `shown fewer than ${k.warn_days} days left, or could not be checked`
          : `shown at least ${k.warn_days} days left`
      case 'snmp_v2c':
      case 'snmp_v3': return down ? 'not answered' : 'answered'
      default: return down ? 'failed' : 'answered' // ping and anything unknown
    }
  }
  const field = FIELD_LABEL[c.field] || c.field
  const cmp = COMPARATOR_WORDS[c.comparator] || c.comparator
  return `had ${field} ${cmp} ${c.value}`
}

// A neutral wording for a group whose conditions would otherwise each say something
// different. Not used for a TCP "must stay closed" check: "failed" would read backwards.
function sharedPhrase(c) {
  if (c.check_type === 'tcp' && cfg(c).expect_closed) return null
  if (isStatus(c, 'down')) return 'failed'
  if (isStatus(c, 'up')) return 'passed'
  return null
}

const countPhrase = (fc) => (fc > 1 ? `${fc} checks in a row` : 'on the latest check')

function joinList(items, word) {
  if (items.length <= 2) return items.join(` ${word} `)
  return `${items.slice(0, -1).join(', ')} ${word} ${items[items.length - 1]}`
}

function groupSentence(conds, op) {
  const isOr = op === 'OR'
  if (conds.length === 1) {
    const c = conds[0]
    return `${subjectOf(c)} has ${phraseOf(c)} ${countPhrase(failCountOf(c))}`
  }
  const sameCount = conds.every((c) => failCountOf(c) === failCountOf(conds[0]) && intervalOf(c) === intervalOf(conds[0]))
  let phrase = null
  if (sameCount) {
    const own = conds.map(phraseOf)
    if (own.every((p) => p === own[0])) phrase = own[0]
    else {
      const shared = conds.map(sharedPhrase)
      if (shared.every((p) => p && p === shared[0])) phrase = shared[0]
    }
  }
  const count = countPhrase(failCountOf(conds[0]))
  const subjects = conds.map(subjectOf)
  if (phrase) {
    if (isOr) return `any one of ${joinList(subjects, 'or')} has ${phrase} ${count}`
    return `${joinList(subjects, 'and')} have ${conds.length === 2 ? 'both' : 'all'} ${phrase} ${count}`
  }
  const clauses = conds.map((c) => `${subjectOf(c)} has ${phraseOf(c)} ${countPhrase(failCountOf(c))}`)
  return isOr ? `any one of these: ${joinList(clauses, 'or')}` : joinList(clauses, 'and')
}

// One delay line per group. AND waits for the slowest run, OR fires on the fastest;
// any single-check condition in an OR group fires at once, so there is no delay to show.
function groupDelay(conds, op) {
  const multi = conds.filter((c) => failCountOf(c) > 1)
  if (multi.length === 0) return null
  if (op === 'OR' && multi.length < conds.length) return null
  const span = (c) => failCountOf(c) * intervalOf(c)
  const pick = multi.reduce((a, b) => ((op === 'OR' ? span(b) < span(a) : span(b) > span(a)) ? b : a))
  return outageDelay(failCountOf(pick), intervalOf(pick))
}

export function explainRule(conditions, targetName, recipientCount) {
  const list = Array.isArray(conditions) ? conditions : []
  const name = typeof targetName === 'string' ? targetName.trim() : ''
  const opening = `${name || 'This target'} counts as down when:`

  // Group like the engine: by condition_group, conditions keep their given order.
  const byGroup = new Map()
  for (const c of list) {
    const g = Number(c.condition_group) || 0
    if (!byGroup.has(g)) byGroup.set(g, [])
    byGroup.get(g).push(c)
  }
  const ids = [...byGroup.keys()].sort((a, b) => a - b)
  const groups = ids.map((id) => {
    const conds = byGroup.get(id)
    const op = conds[conds.length - 1].group_operator === 'OR' ? 'OR' : 'AND'
    return { id, conds, op }
  })

  const closing = groups.length === 0 ? ''
    : groups.length === 1 ? 'It counts as up again on the first check where this is no longer true.'
      : 'It counts as up again on the first check where none of these is true.'

  const checks = []
  const ok = (text) => checks.push({ level: 'ok', text })
  const warn = (text) => checks.push({ level: 'warn', text })

  groups.forEach((g, i) => {
    const types = new Set(g.conds.map((c) => c.check_type))
    if (g.op === 'AND' && g.conds.length >= 2 && types.size >= 2) {
      ok(`Group ${i + 1} needs two different tests to agree before it alerts.`)
    }
  })

  const downs = list.filter((c) => isStatus(c, 'down'))
  if (downs.length > 0 && downs.every((c) => failCountOf(c) >= 2)) {
    ok('No test that can flicker alerts on a single failure.')
  }
  for (const c of downs) {
    if (failCountOf(c) === 1) {
      const label = c.check_name || TYPE_LABEL[c.check_type] || 'This check'
      warn(`${label} alerts on a single failed check. One blip will page people; 3 in a row is safer.`)
    }
  }

  groups.forEach((g, i) => {
    if (g.op === 'OR' && g.conds.length >= 2) {
      warn(`Group ${i + 1} alerts when any one of its tests fails. If they should agree first, switch it to AND.`)
    }
  })

  const unpaired = groups.some((g) => g.conds.some((c) => c.check_type === 'page_hash' && isStatus(c, 'down')) &&
    !(g.op === 'AND' && g.conds.some((c) => c.check_type === 'http' && isStatus(c, 'up'))))
  if (unpaired) {
    warn('Page Hash also fails when the site is simply down. Pair it with "web page answers" in an AND group (recipe: Defacement).')
  }

  const tcpUp = list.filter((c) => c.check_type === 'tcp' && isStatus(c, 'up'))
  if (tcpUp.some((c) => !cfg(c).expect_closed)) {
    warn("This alerts when the port opens, but the target's uptime and SLA will read close to 0%, because a closed port counts as down. Tick Port must stay closed and use Status equals down instead (recipe: A port that must stay closed).")
  }
  if (tcpUp.some((c) => cfg(c).expect_closed)) {
    warn('This alerts while the port is closed. To alert when it opens, use Status equals down.')
  }

  if (list.some((c) => c.check_type === 'dns' && !cfg(c).expect_value)) {
    warn('The DNS check only fails if the lookup fails, not if the answer changes.')
  }

  if (recipientCount === 0) {
    checks.push({ level: 'note', text: 'No one is ticked, so this target sends no email or Signal. The webhook, if it is switched on, still sends.' })
  }

  return {
    opening,
    groups: groups.map((g) => ({ sentence: groupSentence(g.conds, g.op), delay: groupDelay(g.conds, g.op) })),
    closing,
    checks,
  }
}
