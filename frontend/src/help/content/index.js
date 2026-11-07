// Every help page, by the id a view names in data-help-page. Each content module
// exports its pages under their ids (Settings will export one per tab).
//
// pages[id] = { summary: { name, text }, items: { key: Note } }, where
// Note = { title, text: string[] | (args) => string[],
//          rows?: [{ control: { label, className }, text }], tip?,
//          link?: 'lesson' | 'recipes', each?: true, place?: 'right' | 'left' | 'corner' }
// ('corner' is for the PRIMARY badge). Content is plain data, rendered with {{ }}.
import * as dashboard from './dashboard.js'

export const pages = { ...dashboard }
