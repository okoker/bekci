// Help for Alerts (design §7.7). Facts: internal/alerter/alerter.go:39-53, 104-106, 115-127, 145-161, 188-192,
// 201-231; internal/store/alerts.go:34, 165-178; retention cmd/bekci/main.go:288-305.
export const alerts = {
  summary: {
    name: 'Alerts',
    text: 'lists every alert message Bekci tried to send, newest first: one row per person and channel for each down, recovery and reminder.',
  },
  items: {
    audit: {
      title: 'Total',
      text: [
        'For auditors: a row means Bekci tried to send; delivery failures are only in the server log.',
        'A change within the cooldown (30 minutes by default) is neither sent nor listed. People without an email (or phone, for Signal) and suspended users are skipped.',
        'Rows are kept for 91 days by default.',
      ],
    },
    type: {
      title: 'Type',
      text: ['What the message was about.'],
      rows: [
        { control: { label: 'firing', className: 'badge badge-firing' }, text: 'The target went down.' },
        { control: { label: 'recovery', className: 'badge badge-recovery' }, text: 'It came back.' },
        { control: { label: 're-alert', className: 'badge badge-realert' }, text: 'A reminder that it is still down (hourly by default).' },
      ],
    },
    rcp: {
      title: 'Recipient',
      text: ['The person, or Webhook. "(deleted user)" and "(deleted target)" appear once those are gone.'],
    },
    msg: {
      title: 'Message',
      text: ['Email rows show the subject that was sent. Signal and webhook rows start with [Signal] or [Webhook]. Long text is cut off.'],
    },
  },
}
