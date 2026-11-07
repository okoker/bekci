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

export function minFailWindow(cond) {
  // Match the submitted defaults and the API's count/interval bounds.
  const count = Math.min(100, Math.max(1, Number(cond.fail_count) || 1))
  const interval = Math.min(86400, Math.max(10, Number(cond.interval_s) || 60))
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
