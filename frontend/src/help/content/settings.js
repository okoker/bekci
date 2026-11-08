// Help for the Settings tabs, one help page each (design §7.11). Facts, by tab:
// General: sign-in length internal/auth/auth.go:107-128; results purge cmd/bekci/main.go:252-262 and
//   internal/store/results.go:242-263 (rollups 90 days), only 4-hour views read raw results
//   (dashboard_handlers.go:187-202); audit and alert retention main.go:287-305; backup copies
//   backup_handlers.go:240-310; public SOC router.go:152-153, 171-173, 196-198, SocView.vue, its data
//   dashboard_handlers.go dashboardCheck/dashboardTarget (:10-36) and handleCheckHistory (4h, 90d);
//   rate limit middleware.go:114-135; SNMP masking settings_handlers.go:174-186, 314-322,
//   SNMP cache internal/scheduler/scheduler.go:45-66.
// Tags: internal/store/tags.go (normalize :28-37, look-alikes :39-60, delete :114-158, rename :162-201).
// SLA: sla_handlers.go:143-162, dashboard_handlers.go:133-152, SettingsView.vue saveSettings.
// Alerting: internal/alerter/alerter.go (Dispatch :25-196, CheckRealerts :200-345, webhook
//   :359-384, tests :444-529), system_alert.go, webhook.go:82, alert_handlers.go:95-200.
// Backup: backup_handlers.go (export :25-39, full :131-238, passphrase :331-338, saved :340-370,
//   schedule :540-600), internal/store/backup.go (export, restore :386-420).
// Fail2Ban: fail2ban_handlers.go, SettingsView.vue (30-second poll).
// System Health: system_info.go, system_handler.go:75-135, SystemHealthTab.vue.

export const settingsGeneral = {
  summary: {
    name: 'General settings',
    text: 'are the system-wide limits: how long sign-ins last, how long history is kept, how many backup copies are kept, public access to the SOC view, the API rate limit, and the SNMP credentials every SNMP check uses.',
  },
  items: {
    session: {
      title: 'Session Timeout',
      text: ['Hours a sign-in lasts. Applies to sign-ins made after saving.'],
    },
    history: {
      title: 'History Retention',
      text: [
        'Days of individual check results kept (3 by default), used for the 4-hour views.',
        "Doesn't affect uptime or SLA, which come from daily summaries kept 90 days.",
      ],
    },
    auditret: {
      title: 'Audit Log Retention',
      text: ['Days of Audit Log and Alerts history kept (91 by default).'],
    },
    copies: {
      title: 'Backup Copies to Keep',
      text: ['Full backups kept on the server (5 by default). Every full backup counts, downloads included; the oldest goes when the next one is saved.'],
    },
    soc: {
      title: 'SOC View Public Access',
      text: [
        'Yes lets anyone who can reach this server see the SOC view without signing in.',
        "That shows target names, addresses and categories, their state and uptime, and the server's own health; the data behind it also gives each check's name, type, interval, last response time, last result message and 4-hour and 90-day history, each target's SLA goal, the tags and Bekci's version.",
      ],
    },
    rate: {
      title: 'API Rate Limit',
      text: ['Requests per minute for each API token (60 by default). A system that goes over is refused until its minute is up, and the Audit Log records it.'],
    },
    snmp: {
      title: 'SNMP Credentials',
      text: ['Shared by all SNMP v2c and v3 checks. Saved secrets show masked; leave them as they are to keep them. Running checks pick up a change within 15 minutes.'],
    },
  },
}

export const settingsTags = {
  summary: {
    name: 'Tags',
    text: 'are the pick-lists used to label targets: free tags, projects, locations and categories. Categories also set the SLA groups.',
  },
  items: {
    free: {
      title: 'Free Tags',
      text: ['Many per target. Stored in capitals; a name that differs only in i, ı, İ or I from an existing one is refused. Deleting one removes it from every target.'],
    },
    proj: {
      title: 'Project and Location Tags',
      text: ['One of each per target. Deleting one clears it from every target that used it.'],
    },
    cat: {
      title: 'Categories',
      text: [
        'Every target has one. Rename updates every target and keeps its SLA goal; Delete is refused while targets use it.',
        '"Other" can\'t be renamed or deleted.',
      ],
    },
  },
}

export const settingsSla = {
  summary: {
    name: 'SLA goals',
    text: 'set the uptime each category should reach over 90 days. The Dashboard and the SOC view then mark each target HEALTHY or UNHEALTHY, and the SLA page counts the hosts below their goal.',
  },
  items: {
    goal: {
      title: 'Category goal',
      text: [
        'The 90-day uptime this category should reach, in %. 0 turns SLA off for it; a category never saved uses 99.9.',
        "Measured on each target's primary check (the first check of the preferred type, by name).",
      ],
      place: 'left',
    },
    save: {
      title: 'Save',
      text: ['Saves this tab and the General tab together.'],
    },
    guide: {
      title: 'Badge guide',
      text: ['UP/DOWN is the alert rule now; HEALTHY/UNHEALTHY is 90-day uptime against this goal.'],
    },
  },
}

export const settingsAlerting = {
  summary: {
    name: 'Alerting',
    text: "sets how Bekci tells people a target went down or recovered: email, Signal and a webhook, with timing and who gets system alerts. Who gets a target's alerts is set on the target itself.",
  },
  items: {
    method: {
      title: 'Alert Method',
      text: ['Email, Signal, both, or neither (Disabled). The webhook has its own Enabled switch.'],
    },
    cool: {
      title: 'Cooldown',
      text: ['After a down or recovery message, further changes for that target are not sent for this long, recovery included. 0 means the default, 30 minutes.'],
      place: 'corner',
    },
    realert: {
      title: 'Re-alert Interval',
      text: ['A reminder while a target stays down. 0 turns reminders off.'],
    },
    sections: {
      title: 'Save (each section)',
      text: ['Saves that section only. Save one section before editing another: saving reloads the others and drops unsaved edits.'],
    },
    from: {
      title: 'From Email Address',
      text: ['Required: without it no email is sent.'],
    },
    test: {
      title: 'Send Test',
      // The user's wording (07/11/2026), so the note doesn't repeat the hint beside the buttons
      text: ["Each test sends a real message. Test Email goes to the address in the box, or to your own Profile email if the box is empty. Test Signal goes to the number in the box. Test Webhook sends a test event named 'Bekci Test' to the saved URL; a receiving system such as a SOAR may open a ticket for it, so tell its owners first. Every test is recorded in the Audit Log."],
      place: 'corner',
    },
    signal: {
      title: 'Signal fields',
      text: ['Endpoint, number, username and password are all needed: without one, target alerts, reminders and the test skip Signal.'],
    },
    hook: {
      title: 'Webhook',
      text: ['Sends JSON to the URL for every down, recovery and reminder, and for system alerts. A reply of 300 or above, or none, counts as failed, shown in the warning at the top of this section.'],
    },
    system: {
      title: 'System Alerts',
      text: ["Who hears about Bekci's own problems (an unclean restart, a failed scheduled backup), by the Alert Method and the webhook. These ignore the cooldown."],
    },
  },
}

export const settingsBackup = {
  summary: {
    name: 'Backup & Restore',
    text: "schedules automatic backups, downloads the configuration, makes full backups to download or keep on the server, and restores the configuration. A full restore is done from the server's command line.",
  },
  items: {
    auto: {
      title: 'Automated Backups',
      text: ['Starts some time within the chosen hour once the newest saved backup is 7, 10 or 30 days old, counted by date: after a Monday backup, the next is due the following Monday. Not encrypted.'],
    },
    config: {
      title: 'Download Config Backup',
      text: ['A JSON file of the configuration and users, including password hashes and every saved secret in plain text. It holds no check results, alerts or audit log. Keep it safe.'],
    },
    full: {
      title: 'Full Database Backup',
      text: ["Everything, plus the server's config file. Download also keeps a copy on the server."],
    },
    enc: {
      title: 'Encrypt backup',
      text: ["Encrypts with a four-word passphrase Bekci makes up and does not keep. Lose it and the backup can't be opened."],
    },
    saved: {
      title: 'Saved Backups',
      text: ["Copies on the server. Hash is the start of the file's SHA-256, to check a copy."],
    },
    restore: {
      title: 'Config Restore',
      text: [
        'Replaces the configuration and users, and deletes the audit log, alert history and check results. Everyone is signed out.',
        'API tokens and the saved backups stay as they are.',
      ],
    },
  },
}

export const settingsFail2ban = {
  summary: {
    name: 'Fail2Ban',
    text: "is a read-only view of the server's fail2ban jails: how many addresses are blocked now and before, and which. Nothing can be banned or unbanned here.",
  },
  items: {
    refresh: {
      title: 'Updated and Refresh',
      text: ["The table refreshes every 30 seconds; open lists don't. Refresh reads it now."],
    },
    active: {
      title: 'Active Bans',
      text: ['Addresses blocked right now. Click a number for the list.'],
    },
    total: {
      title: 'Bans (total)',
      text: ["fail2ban's own running count; click it for the history, kept as long as fail2ban keeps it."],
    },
    failed: {
      title: 'Failed (window) and (total)',
      text: ["Failed attempts not yet leading to a ban, and fail2ban's running count."],
    },
  },
}

export const settingsHealth = {
  summary: {
    name: 'System Health',
    text: 'shows the health of the server running Bekci: disk, memory, load, uptime, version, updates, last backup, logs, scheduler, database and email.',
  },
  items: {
    refresh: {
      title: 'Refresh',
      text: ["This tab doesn't refresh by itself."],
    },
    disk: {
      title: 'Disk Space',
      text: ["The disk holding Bekci's database."],
    },
    load: {
      title: 'CPU Load',
      text: ['Load averages, not a percentage; compare with the number of cores.'],
    },
    updates: {
      title: 'OS Updates',
      text: ["From the server's last package-list refresh, checked without root; the figure can be up to 15 minutes old."],
    },
    backup: {
      title: 'Last Backup',
      text: ["The newest full backup kept on the server; config downloads don't count."],
    },
    sched: {
      title: 'Scheduler',
      text: ['Running, Starting, Stale or Unavailable; Stale means checks have stopped.'],
    },
    email: {
      title: 'Email',
      text: ['Configured means the fields are filled in, not that sending works: use Send Test Email.'],
    },
  },
}
