// Reads an alert rule back in plain words and flags settings that cause false alarms.
// Pure: no Vue, no DOM, plain strings only (the caller renders them with {{ }}).
import { checkInterval } from '../conditionRules.js'
// Follows the engine (internal/engine/engine.go): groups are OR'd; inside a group
// the operator is the LAST condition's (engine.go:76-79), empty = AND; a condition
// with Fail Count N > 1 needs N matching results in a row, one non-match breaks the run.

const ORDINALS = ['', 'first', 'second', 'third', 'fourth', 'fifth', 'sixth', 'seventh', 'eighth', 'ninth', 'tenth']
function ordinal(n) {
  if (ORDINALS[n]) return ORDINALS[n]
  const last2 = n % 100
  const suffix = last2 >= 11 && last2 <= 13 ? 'th' : ({ 1: 'st', 2: 'nd', 3: 'rd' }[n % 10] || 'th')
  return `${n}${suffix}`
}

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
// Like the checkers' configInt (internal/checker/checker.go): a number is kept as is
// (0 included); anything else (missing, blank string) takes the checker's default.
const numOr = (v, dflt) => (typeof v === 'number' && Number.isFinite(v) ? v : dflt)
const isStatus = (c, v) => c.field === 'status' && c.comparator === 'eq' && c.value === v

// "2 to 3 minutes" / "90 to 135 seconds" for a low and high bound in seconds.
function rangeText(lo, hi) {
  if (lo % 60 === 0 && hi % 60 === 0) return `${lo / 60} to ${hi / 60} minute${hi / 60 === 1 ? '' : 's'}`
  return `${lo} to ${hi} seconds`
}
const delayRange = (failCount, intervalS) => rangeText((failCount - 1) * intervalS, failCount * intervalS)

function everyPhrase(intervals) {
  const list = [...new Set(intervals)].sort((a, b) => a - b)
  if (list.length === 1) {
    const i = list[0]
    if (i === 60) return 'one check a minute'
    if (i % 60 === 0) return `one check every ${i / 60} minutes`
    return i === 1 ? 'one check every second' : `one check every ${i} seconds`
  }
  // Different intervals in one group: list them, in minutes only if all are whole minutes.
  if (list.every((i) => i % 60 === 0)) return `checks every ${joinList(list.map((i) => i / 60), 'and')} minutes`
  return `checks every ${joinList(list, 'and')} seconds`
}

export function outageDelay(failCount, intervalS) {
  const n = Math.floor(Number(failCount))
  const i = Number(intervalS)
  if (!(n > 1) || !(i > 0)) return null
  return `${delayRange(n, i)} into an outage, at ${everyPhrase([i])}`
}

// The Fail Count note's eight sample results, with the figures it used: a whole
// Fail Count of at least 1, and the interval Bekci runs (checkInterval: 10 to
// 86400 seconds, 60 when blank), so the note's caption and delays match Bekci's.
export function failCountTimeline(failCount, intervalS) {
  const n = Math.max(1, Math.floor(Number(failCount)) || 1)
  const interval = checkInterval(intervalS)
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
    summary = `${never} A real outage alerts on the ${ordinal(n)} failed check, ${delayRange(n, interval)} in.`
  }
  return { steps, summary, failCount: n, interval }
}

function subjectOf(c) {
  const k = cfg(c)
  switch (c.check_type) {
    case 'ping': return 'ping'
    case 'http': return k.endpoint && k.endpoint !== '/' ? `the page ${k.endpoint}` : 'the web page'
    case 'tcp': return typeof k.port === 'number' && Number.isFinite(k.port) ? `port ${k.port}` : 'the port'
    case 'dns': {
      const q = typeof k.query === 'string' ? k.query.trim() : ''
      return `the DNS answer${q ? ` for ${q}` : ''}${k.nameserver ? ` at ${k.nameserver}` : ''}`
    }
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
  const value = String(c.value ?? '').trim()
  if (c.field === 'response_ms' && c.comparator === 'gt') {
    return value ? `taken longer than ${value} ms` : 'taken longer than the time you set'
  }
  if (c.field === 'metrics.packet_loss' && c.comparator === 'gt') {
    return value ? `lost more than ${value}% of its pings` : 'lost more than the share you set of its pings'
  }
  if (c.field === 'status' && c.comparator === 'eq' && (c.value === 'down' || c.value === 'up')) {
    const down = c.value === 'down'
    switch (c.check_type) {
      case 'http': return `${down ? 'not answered' : 'answered'} with ${numOr(k.expect_status, 200)}`
      case 'tcp': return (down !== !!k.expect_closed) ? 'refused connections' : 'accepted a connection'
      case 'dns':
        if (k.expect_value) {
          return down ? `given an address other than ${k.expect_value}, or failed to resolve` : `given ${k.expect_value}`
        }
        return down ? 'failed to resolve' : 'resolved'
      case 'page_hash': return down ? 'changed, or could not be read' : 'matched its baseline'
      case 'tls_cert':
        return down ? `shown fewer than ${numOr(k.warn_days, 30)} days left, or could not be checked`
          : `shown at least ${numOr(k.warn_days, 30)} days left`
      case 'snmp_v2c':
      case 'snmp_v3': return down ? 'not answered' : 'answered'
      default: return down ? 'failed' : 'answered' // ping and anything unknown
    }
  }
  const field = FIELD_LABEL[c.field] || c.field
  const cmp = COMPARATOR_WORDS[c.comparator] || c.comparator
  return `had ${field} ${cmp} ${value || 'an empty value'}`
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

// One delay line per group, from the first check to the alert. Each condition alerts
// between (N-1) and N intervals into an outage (Fail Count 1: 0 to one interval).
// AND waits for every member: largest low to largest high. OR fires on the first
// member: smallest low to smallest high.
function groupDelay(conds, op) {
  if (!conds.some((c) => failCountOf(c) > 1)) return null
  const lows = conds.map((c) => (failCountOf(c) - 1) * intervalOf(c))
  const highs = conds.map((c) => failCountOf(c) * intervalOf(c))
  const pick = op === 'OR' ? Math.min : Math.max
  return `${rangeText(pick(...lows), pick(...highs))} into an outage, at ${everyPhrase(conds.map(intervalOf))}`
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

  // Everything except Status equals up can flicker (up is a passing test that rules
  // out outages), unless a failing test of another type in its AND group must
  // agree first: that test confirms it (ruling R14). A ping on the latest check
  // AND'd with a web page on 2 in a row pages only once the web page has failed
  // twice. A partner that must be passing (Status equals up) is the normal state
  // and confirms nothing: Page Hash on the latest check AND "web page answers"
  // still pages on one changed page.
  const groupOf = new Map(groups.flatMap((g) => g.conds.map((c) => [c, g])))
  const confirmed = (c) => {
    const g = groupOf.get(c)
    return g.op === 'AND' && g.conds.some((o) => o.check_type !== c.check_type && !isStatus(o, 'up'))
  }
  const downs = list.filter((c) => !isStatus(c, 'up'))
  const alone = downs.filter((c) => !confirmed(c))
  if (downs.length > 0 && alone.every((c) => failCountOf(c) >= 2)) {
    ok('No test that can flicker alerts on a single failure.')
  }
  for (const c of alone) {
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
