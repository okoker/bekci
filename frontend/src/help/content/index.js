// Every help page, by the id a view names in data-help-page (and the header's,
// HEADER_PAGE in ../anchors.js). Each content module exports its pages under
// their ids (Settings will export one per tab).
//
// pages[id] = { summary?: { name, text }, items: { key: Note } }, where
//   Note = {
//     title,                          the mark's name (tooltip, "Help: <title>") and the note's heading
//     text: string[] | (args) => string[],          one to three sentences
//     rows?: [{ control: { label, className }, text }] | (args) => rows,
//                                     controls drawn with the app's own classes, one line each
//     tip?: string | (args) => string,                the grey tip box
//     link?: 'lesson' | 'recipes',    shown once that window exists (HelpLayer.vue LINKS)
//     place?: 'right' | 'left' | 'corner',            where the mark sits: right of the control
//                                     (default), left of it, or on its top-right corner (tight spots)
//     box?: 'content' | 'label',      'content': measure the control's content (its text and children),
//                                     not its whole box: for a full-width bar whose items are centred.
//                                     'label': the control is a form field (its .form-group); the mark
//                                     sits just after the text of its label
//     timeline?: (args) => { caption, steps: [{ result: 'ok' | 'fail', label }], summary },
//                                     a row of check results drawn under the text (the Fail Count note)
//     each?: true,                    a mark on every visible instance, not just the first (the target
//                                     form's per-condition fields); `args` are then that instance's
//   }
// `args` is the parsed JSON of the control's data-help-args attribute ({} if none).
// Content is plain data, rendered with {{ }}, never as HTML.
import * as alerts from './alerts.js'
import * as auditLog from './auditLog.js'
import * as dashboard from './dashboard.js'
import * as header from './header.js'
import * as profile from './profile.js'
import * as search from './search.js'
import * as settings from './settings.js'
import * as sla from './sla.js'
import * as targets from './targets.js'
import * as targetForm from './targetForm.js'
import * as users from './users.js'

export const pages = {
  ...alerts, ...auditLog, ...dashboard, ...header, ...profile, ...search, ...settings, ...sla, ...targets, ...targetForm, ...users,
}
