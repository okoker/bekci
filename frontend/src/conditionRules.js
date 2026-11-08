// Pure validation rules for the target form. No component state in here.

export const checkTypes = [
  { value: 'http', label: 'HTTP/HTTPS' },
  { value: 'tcp', label: 'TCP' },
  { value: 'ping', label: 'Ping (ICMP)' },
  { value: 'dns', label: 'DNS' },
  { value: 'page_hash', label: 'Page Hash' },
  { value: 'tls_cert', label: 'TLS Certificate' },
  { value: 'snmp_v2c', label: 'SNMP v2c' },
  { value: 'snmp_v3', label: 'SNMP v3' },
]

export function getDefaultConfig(type_) {
  switch (type_) {
    case 'http': return { scheme: 'https', port: 0, endpoint: '/', expect_status: 200, skip_tls_verify: false, timeout_s: 10 }
    case 'tcp': return { port: 80, timeout_s: 5, expect_closed: false }
    case 'ping': return { count: 3, timeout_s: 5 }
    case 'dns': return { query: '', record_type: 'A', expect_value: '', nameserver: '' }
    case 'page_hash': return { scheme: 'https', endpoint: '/', baseline_hash: '', timeout_s: 10 }
    case 'tls_cert': return { port: 443, warn_days: 30, timeout_s: 10 }
    case 'snmp_v2c': return { port: 161, timeout_s: 5 }
    case 'snmp_v3': return { port: 161, timeout_s: 5 }
    default: return {}
  }
}

export function getDefaultCondition(conditionGroup = 0, groupOperator = 'AND') {
  return {
    check_id: '',
    check_type: 'ping',
    check_name: '',
    config: getDefaultConfig('ping'),
    interval_s: 60,
    field: 'status',
    comparator: 'eq',
    value: 'down',
    fail_count: 1,
    fail_window: 0,
    condition_group: conditionGroup,
    group_operator: groupOperator,
  }
}

export const maxFailWindow = 1800

// The interval Bekci runs a check at: the form sends 60 for a blank one, and
// the API keeps it within 10 to 86400 seconds (target_handlers.go).
export function checkInterval(intervalS) {
  return Math.min(86400, Math.max(10, Number(intervalS) || 60))
}

export function minFailWindow(cond) {
  // Match the submitted defaults and the API's count/interval bounds.
  const count = Math.min(100, Math.max(1, Number(cond.fail_count) || 1))
  const interval = checkInterval(cond.interval_s)
  return count > 1 ? count * interval : 0
}

export function failWindowError(cond) {
  const minimum = minFailWindow(cond)
  if (minimum <= maxFailWindow) return ''
  const duration = minimum % 60 === 0
    ? `${minimum / 60}-minute`
    : `${Math.floor(minimum / 60)}-minute ${minimum % 60}-second`
  return `These settings require a ${duration} failure window. The maximum allowed is 30 minutes. Reduce the Fail Count or shorten the check Interval.`
}

// Number fields are compared with Go's strconv.ParseFloat in the engine; a
// plain decimal is what both sides read the same way. Anything else never fires.
export const plainNumber = /^-?\d+(\.\d+)?$/

export function valueError(cond) {
  if (cond.field === 'response_ms' && !plainNumber.test(cond.value)) {
    return 'Enter a number of milliseconds, such as 500.'
  }
  if (cond.field === 'metrics.packet_loss' &&
      (!plainNumber.test(cond.value) || Number(cond.value) < 0 || Number(cond.value) > 100)) {
    return 'Enter a percentage from 0 to 100, such as 30.'
  }
  return ''
}

// Packet loss = lost ÷ sent × 100, so with n pings it moves in steps of 100/n.
export function packetLossHint(cond) {
  const count = cond.config.count
  const n = Number.isInteger(count) && count >= 1 ? count : 3
  const parts = [`Share of this check's ${n} pings that got no reply, in steps of ${Math.round(1000 / n) / 10}%.`]
  if (cond.comparator === 'gt' && plainNumber.test(cond.value)) {
    const k = Math.floor(Number(cond.value) * n / 100) + 1
    parts.push(k > n
      ? `Greater than ${cond.value} can never be true: at most ${n} of ${n} can be lost.`
      : `Greater than ${cond.value} = ${k} or more of the ${n} lost (no reply at all counts too).`)
  }
  parts.push('More pings give finer steps; each adds about a second to the check.')
  return parts.join(' ')
}

// A target Host is a name or IP, optionally followed by a path
// (www.example.com/health). These mirror the server's SplitHost, HostError and
// buildHTTPURL (internal/checker/host.go, url.go); frontend/tests/condition-rules.test.mjs
// runs them against the same tables (internal/checker/testdata/), so the form
// and the server agree.
export function splitHost(host) {
  const i = host.indexOf('/')
  return i >= 0 ? [host.slice(0, i), host.slice(i)] : [host, '']
}

// The server's net.ParseIP, for a name holding ":" (only IPv6 can).
function isIPv6(name) {
  if (!/^[0-9A-Fa-f:.]+$/.test(name)) return false
  try {
    new URL(`http://[${name}]/`)
    return true
  } catch {
    return false
  }
}

// Why Save refuses a Host, or '' when it is fine. Trims first, as the server
// does; an empty Host is left to the field's "required".
export function hostError(host) {
  const h = String(host ?? '').trim()
  if (h === '') return ''
  if (/[\p{White_Space}\p{Cc}]/u.test(h)) return "Host can't contain spaces."
  if (h.includes('://')) return "Leave out http:// or https://: each HTTP or Page Hash check has its own Scheme."
  if (/[?#]/.test(h)) return "Put a query (? or #) in the check's Endpoint, not in Host."
  const [name] = splitHost(h)
  if (name === '') return 'Host needs a name or IP address before the path.'
  if (name.includes('@')) return "Host can't contain @ before the path."
  if (name.includes(':') && !isIPv6(name)) {
    return "Leave out the port: each check has its own Port field. Write an IPv6 address without brackets."
  }
  return ''
}

// The URL an HTTP or Page Hash check requests: the Host's name (and port), the
// Host's path as typed, then the Endpoint; an Endpoint of '' or '/' adds nothing.
export function buildHTTPURL(scheme, host, port, endpoint) {
  const [name, path] = splitHost(host)
  const bracketed = name.includes(':') ? `[${name}]` : name
  const base = `${scheme}://${port > 0 ? `${bracketed}:${port}` : bracketed}`
  if (endpoint === '' || endpoint === '/') {
    if (path !== '') return base + path
    return endpoint === '' ? base : `${base}/`
  }
  return `${base}${path.replace(/\/+$/, '')}/${endpoint.replace(/^\/+/, '')}`
}

// The address shown under an HTTP or Page Hash Endpoint, with the defaults the
// check applies (checker.go configStr/configInt: a missing or empty scheme is
// https, an empty Endpoint is "/", a port that is not a number is 0). '' while
// the Host is empty or refused, or the scheme is one the check rejects.
export function checkURL(config, host) {
  const h = String(host ?? '').trim()
  if (h === '' || hostError(h)) return ''
  const str = (v, d) => (typeof v === 'string' && v !== '' ? v : d)
  const scheme = str(config?.scheme, 'https')
  if (scheme !== 'http' && scheme !== 'https') return ''
  const port = typeof config?.port === 'number' && Number.isFinite(config.port) ? Math.trunc(config.port) : 0
  return buildHTTPURL(scheme, h, port, str(config?.endpoint, '/'))
}
