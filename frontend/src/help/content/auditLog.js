// Help for the Audit Log (design §7.9). Facts: search internal/store/audit.go:44-56; who is
// "system" alerter.go:347-357, backup_handlers.go:604-640, cmd/bekci/main.go:128-135, 202-209;
// "api-token:" only on the rate limit middleware.go:125, audit.go:68-82; failed sign-in
// auth_handlers.go:41; colours SettingsView.vue actionClass; retention cmd/bekci/main.go:287-305;
// restore wipe internal/store/backup.go:400-418; not recorded auth_handlers.go:32-35,
// middleware.go:108-111, v1_hosts.go:103-110; update_settings settings_handlers.go:335.
export const auditLog = {
  summary: {
    name: 'Audit Log',
    text: 'is a read-only record of who did what and when: sign-ins, changes to targets, users and settings, backups, and automatic jobs. Search narrows it; click a row for the full entry.',
  },
  items: {
    audit: {
      title: 'Events',
      text: [
        'How many entries match the search, or all of them when it is empty.',
        'For auditors: entries are kept 91 days by default (Settings › General), and a Config Restore deletes the whole log.',
      ],
      tip: "Not recorded: viewing pages, sign-in attempts blocked by the built-in rate limit, invalid API tokens and normal API reads. update_settings doesn't say which setting changed.",
    },
    q: {
      title: 'Search',
      text: ["Finds text in the user, action, detail, IP address or resource type. It doesn't search times, IDs or status."],
    },
    user: {
      title: 'User',
      text: [
        'The username at the time. A failed sign-in shows whatever name was typed.',
        '"system" is Bekci itself: scheduled backups, unclean restarts, a stalled scheduler, webhook sends. "api-token:" and a name is another system using an API token, recorded when it goes over the rate limit.',
      ],
    },
    action: {
      title: 'Action',
      text: ['What happened. The colour comes from the words in the name; the Status column says whether it worked.'],
      rows: [
        { control: { label: 'login_failed', className: 'badge badge-action-delete' }, text: 'Red: names with "failed", checked first.' },
        { control: { label: 'login', className: 'badge badge-action-auth' }, text: 'Blue: then names with "login".' },
        { control: { label: 'create_user', className: 'badge badge-action-create' }, text: 'Green: names with "create".' },
        { control: { label: 'delete_tag', className: 'badge badge-action-delete' }, text: 'Red: names with "delete" or "suspend".' },
        { control: { label: 'pause_target', className: 'badge badge-action-default' }, text: 'Grey: everything else, such as update_settings.' },
      ],
    },
    res: {
      title: 'Resource',
      text: ['What it happened to, with the first 8 characters of its ID. Click the row for the full ID.'],
    },
    status: {
      title: 'Status',
      text: ['Whether the attempt worked.'],
      rows: [
        { control: { label: 'success', className: 'status-ok' }, text: 'It worked.' },
        { control: { label: 'failure', className: 'status-fail' }, text: 'It was refused or went wrong.' },
      ],
    },
    ip: {
      title: 'IP',
      text: ["Where the request came from. Empty for Bekci's own jobs."],
    },
    row: {
      title: 'Audit entries',
      text: ['Click a row for the full entry: the whole detail and the full resource ID.'],
    },
  },
}
