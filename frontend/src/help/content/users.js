// Help for Users: the Humans and API Access tabs (design §7.10). Facts: internal/api/user_handlers.go
// (create :28-104, no username change :128-197, blank keeps :181-188, last admin :167-179 and
// :215-227, sign-outs :239-244 and :286-289); no delete route router.go:119-124; roles router.go;
// tokens internal/store/api_tokens.go (unique name, soft revoke :105-125, hash only :30-35, prefix
// :62, last used :149-150, no expiry), internal/api/api_tokens_handlers.go; rate limit
// middleware.go:114-135, v1_rate_limit.go.
export const users = {
  summary: {
    name: 'Users',
    text: "is where admins create accounts, set each person's role, email and phone, suspend or reactivate them, and reset passwords. Accounts are suspended, never deleted.",
  },
  items: {
    create: {
      title: 'Create User',
      text: [
        'Opens the form. The username is permanent and case-sensitive; the password is 15 to 32 characters (letters such as ş or ğ count as two).',
        'A phone number is added later with Edit.',
      ],
      place: 'left',
    },
    role: {
      title: 'Role',
      text: ['What the person can do.'],
      rows: [
        { control: { label: 'viewer', className: 'badge badge-viewer' }, text: 'Looks only.' },
        { control: { label: 'operator', className: 'badge badge-operator' }, text: 'Also manages targets, Search and the Audit Log.' },
        { control: { label: 'admin', className: 'badge badge-admin' }, text: 'Also users and settings.' },
      ],
    },
    status: {
      title: 'Status',
      text: ['Whether the person can sign in.'],
      rows: [
        { control: { label: 'active', className: 'badge badge-active' }, text: 'Can sign in.' },
        { control: { label: 'suspended', className: 'badge badge-suspended' }, text: "Can't sign in." },
      ],
    },
    edit: {
      title: 'Edit',
      text: [
        'Changes email, phone and role. A field left blank keeps its old value.',
        "Reset Password, in the same window, takes effect at once and signs that person out everywhere, you included if it's your own account.",
      ],
      place: 'left',
    },
    suspend: {
      title: 'Suspend and Activate',
      text: [
        'Suspend signs the person out at once and stops them signing in; Activate lets them sign in again. Bekci asks first.',
        "The last active admin can't be suspended or demoted.",
      ],
    },
    priv: {
      title: 'Role privileges',
      text: ['A table of what each role can do.'],
    },
  },
}

export const apiAccess = {
  summary: {
    name: 'API Access',
    text: "issues tokens that let other systems, such as Grafana or Zabbix, read Bekci's machine API.",
  },
  items: {
    once: {
      title: 'New token',
      text: ["The token is shown this once. Copy it now: Bekci keeps only a fingerprint and can't show it again."],
    },
    name: {
      title: 'Token name',
      text: ['A label for the system that will use it. Names can never be reused, even after revoking.'],
    },
    prefix: {
      title: 'Prefix',
      text: ["The token's first characters, to tell tokens apart."],
    },
    used: {
      title: 'Last used',
      text: ['Updated each time the token is used; "—" means never.'],
    },
    revoke: {
      title: 'Revoke',
      text: ['Stops the token at once; any system using it stops working. Bekci asks first. Tokens never expire on their own.'],
      tip: 'Use is limited per token by Settings › General › API Rate Limit.',
    },
  },
}
