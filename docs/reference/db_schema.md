# Database Schema Reference

**Current schema version:** 24
**Engine:** SQLite 3 with WAL journal mode
**Driver:** `github.com/mattn/go-sqlite3` (CGO required)

## SQLite Connection Config

```
?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON
```

- WAL mode for concurrent reads during writes
- 5s busy timeout to avoid `SQLITE_BUSY` under contention
- Foreign keys enforced at connection level
- DB file permissions set to `0600` after creation

---

## Tables

### schema_version

Tracks current schema version. Single row. Stamped to 24 by the baseline schema on fresh install.

| Column  | Type    | Constraints |
|---------|---------|-------------|
| version | INTEGER | NOT NULL    |

### users

| Column        | Type     | Constraints                                            |
|---------------|----------|--------------------------------------------------------|
| id            | TEXT     | **PK**                                                 |
| username      | TEXT     | UNIQUE NOT NULL                                        |
| email         | TEXT     | NOT NULL DEFAULT ''                                    |
| phone         | TEXT     | NOT NULL DEFAULT '' *(added migration011)*             |
| password_hash | TEXT     | NOT NULL                                               |
| role          | TEXT     | NOT NULL, CHECK(role IN ('admin','operator','viewer')) |
| status        | TEXT     | NOT NULL DEFAULT 'active', CHECK(status IN ('active','suspended')) |
| created_at    | DATETIME | DEFAULT CURRENT_TIMESTAMP                              |
| updated_at    | DATETIME | DEFAULT CURRENT_TIMESTAMP                              |

### sessions

| Column     | Type     | Constraints                                    |
|------------|----------|------------------------------------------------|
| id         | TEXT     | **PK**                                         |
| user_id    | TEXT     | NOT NULL, FK -> users(id) ON DELETE CASCADE    |
| expires_at | DATETIME | NOT NULL                                       |
| ip_address | TEXT     |                                                |
| created_at | DATETIME | DEFAULT CURRENT_TIMESTAMP                      |

**Indexes:**
- `idx_sessions_user_id` ON sessions(user_id)
- `idx_sessions_expires_at` ON sessions(expires_at)

### settings

Key-value config store.

| Column | Type | Constraints |
|--------|------|-------------|
| key    | TEXT | **PK**      |
| value  | TEXT | NOT NULL    |

**Seeded keys:**

| Key                    | Default | Added in    |
|------------------------|---------|-------------|
| session_timeout_hours  | 24      | migration001 |
| history_days           | 3       | migration001 (re-seeded migration022) |
| audit_retention_days   | 91      | migration001 (re-seeded migration009) |
| soc_public             | false   | migration003 |
| alert_method           | email   | migration011 |
| resend_api_key         |         | migration011 |
| alert_from_email       |         | migration011 |
| alert_cooldown_s       | 1800    | migration011 |
| alert_realert_s        | 3600    | migration011 |
| sla_network            | 99.9    | migration013 |
| sla_security           | 99.9    | migration013 |
| sla_physical_security  | 99.9    | migration013 |
| sla_key_services       | 99.9    | migration013 |
| sla_other              | 99.9    | migration013 |
| signal_api_url         |         | migration016 |
| signal_number          |         | migration016 |
| signal_username        |         | migration016 |
| signal_password        |         | migration016 |
| signal_skip_tls        | false   | migration024 |
| snmp_v2c_community     | public  | migration018 |
| snmp_v3_username       |         | migration018 |
| snmp_v3_security_level | authPriv | migration018 |
| snmp_v3_auth_protocol  | SHA     | migration018 |
| snmp_v3_auth_passphrase|         | migration018 |
| snmp_v3_privacy_protocol| AES    | migration018 |
| snmp_v3_privacy_passphrase|      | migration018 |

### targets

| Column               | Type     | Constraints                         |
|----------------------|----------|-------------------------------------|
| id                   | TEXT     | **PK**                              |
| name                 | TEXT     | UNIQUE NOT NULL                     |
| host                 | TEXT     | NOT NULL                            |
| description          | TEXT     | NOT NULL DEFAULT ''                 |
| enabled              | INTEGER  | NOT NULL DEFAULT 1                  |
| preferred_check_type | TEXT     | NOT NULL DEFAULT 'ping'             |
| operator             | TEXT     | NOT NULL DEFAULT 'AND' *(migration006)* |
| category             | TEXT     | NOT NULL DEFAULT 'critical' *(migration006, renamed from severity in migration007)* |
| rule_id              | TEXT     | DEFAULT NULL *(migration006)*       |
| paused_at            | DATETIME | DEFAULT NULL *(migration015)*       |
| notes                | TEXT     | DEFAULT NULL *(migration019)*       |
| contacts             | TEXT     | DEFAULT NULL *(migration019)*       |
| project              | TEXT     | DEFAULT NULL *(migration019)*       |
| location             | TEXT     | DEFAULT NULL *(migration019)*       |
| created_at           | DATETIME | DEFAULT CURRENT_TIMESTAMP           |
| updated_at           | DATETIME | DEFAULT CURRENT_TIMESTAMP           |

**Notes:**
- `project_id` FK was removed in migration004 (projects table dropped).
- `UNIQUE(project_id, name)` was replaced by `UNIQUE(name)` after migration004 table rebuild.
- `rule_id` is a logical FK to `rules(id)` but not enforced at DDL level (nullable, set by app code).
- `category` was originally `severity` (renamed in migration007). Values remapped in migration010. DDL default is `'critical'` but app code defaults to `'Other'` when empty.
- `idx_targets_project_id` index from migration002 was dropped along with the old table during migration004's table rebuild (CREATE new → DROP old → RENAME).
- `idx_targets_rule_id` ON targets(rule_id) *(migration021)* — accelerates rule-to-target lookups.
- `paused_at` — when NOT NULL, target is paused: scheduler skips checks, dashboard shows "PAUSED" badge.
- `notes`, `contacts` — free-text optional fields.
- `project`, `location` — optional tag values from `tag_options` table. Validated on create/update. Cascade-cleared when tag option is deleted.

### tag_options

*(migration019, updated migration023, updated migration025)*

Admin-managed list of allowed tag values. Four groups: `project`, `location`, `category` (single-value slots on `targets`), and `tag` (many-per-target via the `target_tags` join).

| Column | Type    | Constraints                                                         |
|--------|---------|---------------------------------------------------------------------|
| id     | INTEGER | **PK** AUTOINCREMENT                                               |
| grp    | TEXT    | NOT NULL, CHECK(grp IN ('project', 'location', 'category', 'tag')) |
| value  | TEXT    | NOT NULL                                                            |

**Unique:** (grp, value) composite

**Seeded categories** (migration023): Key Services, Network, Other, Physical Security, Security.

**Notes:**
- Column named `grp` (not `group`) to avoid SQL reserved word.
- For project/location: deleting a tag option cascade-clears the corresponding field on all targets (app-level, not DDL cascade).
- For category: delete is blocked if any targets use that category (returns 409 with target names). Rename cascades to `targets.category` and renames the corresponding `sla_*` settings key. "Other" cannot be renamed or deleted.
- For `tag`: values are uppercased + trimmed on create/rename (handler-side); deletion relies on the DDL-level FK cascade on `target_tags.tag_id` — no app-level sweep. Renames are free (no sweep) because targets reference tags by id via the join table.
- SLA key derivation: `"sla_" + lowercase(replace(name, " ", "_"))` — e.g. "Physical Security" → `sla_physical_security`.

### target_tags

*(migration025)*

Join table for many-to-many free-form tags per target. Each row links one target to one `tag_options` row where `grp='tag'`.

| Column    | Type    | Constraints                                                    |
|-----------|---------|----------------------------------------------------------------|
| target_id | TEXT    | NOT NULL, FK → targets(id) **ON DELETE CASCADE**              |
| tag_id    | INTEGER | NOT NULL, FK → tag_options(id) **ON DELETE CASCADE**          |

**Primary key:** (target_id, tag_id) — prevents duplicate links.
**Index:** `idx_target_tags_tag` on (tag_id) — supports "list all hosts with tag X" queries.

**Notes:**
- DDL-level `ON DELETE CASCADE` on both FKs means target deletion and catalog-tag deletion both auto-clean this table.
- Tags are fetched via bulk join in one query (`AttachTagsBulk` in `internal/store/targets.go`) for list views, avoiding N+1.

### checks

| Column     | Type     | Constraints                                                  |
|------------|----------|--------------------------------------------------------------|
| id         | TEXT     | **PK**                                                       |
| target_id  | TEXT     | NOT NULL, FK -> targets(id) ON DELETE CASCADE                |
| type       | TEXT     | NOT NULL, CHECK(type IN ('http','tcp','ping','dns','page_hash','tls_cert','snmp_v2c','snmp_v3')) |
| name       | TEXT     | NOT NULL                                                     |
| config     | TEXT     | NOT NULL DEFAULT '{}'                                        |
| interval_s | INTEGER  | NOT NULL DEFAULT 300                                         |
| enabled    | INTEGER  | NOT NULL DEFAULT 1                                           |
| created_at | DATETIME | DEFAULT CURRENT_TIMESTAMP                                    |
| updated_at | DATETIME | DEFAULT CURRENT_TIMESTAMP                                    |

**Indexes:**
- `idx_checks_target_id` ON checks(target_id)

### check_state

*(migration020)*

Current status cache — 1 row per check, upserted on every `SaveResult`. Replaces expensive `MAX(checked_at)` subqueries.

| Column      | Type     | Constraints                                  |
|-------------|----------|----------------------------------------------|
| check_id    | TEXT     | **PK**, FK -> checks(id) ON DELETE CASCADE   |
| status      | TEXT     | NOT NULL, CHECK(status IN ('up','down'))      |
| response_ms | INTEGER  | NOT NULL DEFAULT 0                           |
| message     | TEXT     | NOT NULL DEFAULT ''                          |
| metrics     | TEXT     | NOT NULL DEFAULT '{}' *(JSON)*               |
| checked_at  | DATETIME | NOT NULL                                     |

**Write pattern:** UPSERT (INSERT ... ON CONFLICT DO UPDATE) on every `SaveResult` call, inside the same transaction as raw insert + rollup upsert.

**Read consumers:** `GetLastResult` (engine + alerter webhook payload), `GetBatchLastResultAndUptime` (dashboard + SOC status).

**Retention:** Unbounded — always reflects current state. Rows only removed when parent check is deleted (CASCADE).

### check_daily_rollups

*(migration020)*

Pre-aggregated daily uptime — 1 row per check per day, upserted on every `SaveResult`. Eliminates expensive `GROUP BY date(checked_at)` aggregation queries.

| Column          | Type    | Constraints                                   |
|-----------------|---------|-----------------------------------------------|
| check_id        | TEXT    | NOT NULL, FK -> checks(id) ON DELETE CASCADE  |
| day             | TEXT    | NOT NULL                                      |
| total_count     | INTEGER | NOT NULL DEFAULT 0                            |
| up_count        | INTEGER | NOT NULL DEFAULT 0                            |
| down_count      | INTEGER | NOT NULL DEFAULT 0                            |
| avg_response_ms | INTEGER | NOT NULL DEFAULT 0                            |
| max_response_ms | INTEGER | NOT NULL DEFAULT 0                            |

**PK:** (check_id, day) composite

**Write pattern:** UPSERT on every `SaveResult` call. Increments counts and recalculates running average inside the same transaction.

**Read consumers:** `GetDailyUptime` (90-day daily chart), `GetUptimePercent` (SLA percentage), `GetBatchLastResultAndUptime` (dashboard 90d uptime), SLA page.

**Retention:** Purged at 90 days by `PurgeOldResults`.

### check_results

Tactical time-series window. Schema unchanged from pre-A-011, but retention reduced from 90 days to 3 days (default). No longer used for dashboard status, SLA calculations, or uptime percentages — those now read from `check_state` and `check_daily_rollups`.

| Column      | Type     | Constraints                                   |
|-------------|----------|-----------------------------------------------|
| id          | INTEGER  | **PK AUTOINCREMENT**                          |
| check_id    | TEXT     | NOT NULL, FK -> checks(id) ON DELETE CASCADE  |
| status      | TEXT     | NOT NULL, CHECK(status IN ('up','down'))       |
| response_ms | INTEGER  | NOT NULL DEFAULT 0                            |
| message     | TEXT     | NOT NULL DEFAULT ''                           |
| metrics     | TEXT     | NOT NULL DEFAULT '{}' *(JSON)*                |
| checked_at  | DATETIME | NOT NULL                                      |

**Indexes:**
- `idx_check_results_check_id` ON check_results(check_id)
- `idx_check_results_checked_at` ON check_results(checked_at)
- `idx_check_results_check_id_checked_at` ON check_results(check_id, checked_at DESC) *(migration017)*

**Read consumers:** `GetRecentResultsSlim` (4h bar chart on dashboard), `GetRecentResultsByWindow` (fail_window condition evaluation in engine), forensic debugging.

**Retention:** Default 3 days (code default in `main.go`; `history_days` setting still respected if set higher). Purged hourly by `PurgeOldResults`. At 3-day steady state with 1,500 checks at 5-min intervals: ~1.3M rows.

### rules

Hidden rules auto-managed by the target CRUD layer. One rule per target (1:1 via `targets.rule_id`).

| Column      | Type     | Constraints                                              |
|-------------|----------|----------------------------------------------------------|
| id          | TEXT     | **PK**                                                   |
| name        | TEXT     | NOT NULL                                                 |
| description | TEXT     | NOT NULL DEFAULT ''                                      |
| operator    | TEXT     | NOT NULL DEFAULT 'AND', CHECK(operator IN ('AND','OR'))  |
| severity    | TEXT     | NOT NULL DEFAULT 'critical', CHECK(severity IN ('critical','warning','info')) |
| enabled     | INTEGER  | NOT NULL DEFAULT 1                                       |
| created_at  | DATETIME | DEFAULT CURRENT_TIMESTAMP                                |
| updated_at  | DATETIME | DEFAULT CURRENT_TIMESTAMP                                |

### rule_conditions

Each condition links a rule to a check with evaluation criteria. Conditions are grouped by `condition_group`; within a group, `group_operator` (AND/OR) decides how conditions combine. Groups are always OR'd together by the engine.

| Column          | Type    | Constraints                                   |
|-----------------|---------|-----------------------------------------------|
| id              | TEXT    | **PK**                                        |
| rule_id         | TEXT    | NOT NULL, FK -> rules(id) ON DELETE CASCADE   |
| check_id        | TEXT    | NOT NULL, FK -> checks(id) ON DELETE CASCADE  |
| field           | TEXT    | NOT NULL DEFAULT 'status'                     |
| comparator      | TEXT    | NOT NULL DEFAULT 'eq'                         |
| value           | TEXT    | NOT NULL                                      |
| fail_count      | INTEGER | NOT NULL DEFAULT 1                            |
| fail_window     | INTEGER | NOT NULL DEFAULT 0                            |
| sort_order      | INTEGER | NOT NULL DEFAULT 0                            |
| condition_group | INTEGER | NOT NULL DEFAULT 0 *(migration014)*           |
| group_operator  | TEXT    | NOT NULL DEFAULT 'AND' *(migration014)*       |

**Indexes:** *(migration021)*
- `idx_rule_conditions_check_id` ON rule_conditions(check_id)
- `idx_rule_conditions_rule_id` ON rule_conditions(rule_id, condition_group, sort_order)

### rule_states

One row per rule. Tracks current health state for the rule engine.

| Column         | Type     | Constraints                                  |
|----------------|----------|----------------------------------------------|
| rule_id        | TEXT     | **PK**, FK -> rules(id) ON DELETE CASCADE    |
| current_state  | TEXT     | NOT NULL DEFAULT 'healthy'                   |
| last_change    | DATETIME | nullable                                     |
| last_evaluated | DATETIME | nullable                                     |

**States:** `healthy`, `unhealthy`

### alert_history

Append-only log of sent alerts. Purged daily by `PurgeOldAlertHistory(days)` using `audit_retention_days` setting (default 91).

| Column          | Type     | Constraints                        |
|-----------------|----------|------------------------------------|
| id              | INTEGER  | **PK AUTOINCREMENT**               |
| rule_id         | TEXT     | NOT NULL                           |
| channel_id      | TEXT     | nullable *(legacy, pre-migration011)* |
| alert_type      | TEXT     | NOT NULL                           |
| message         | TEXT     | nullable                           |
| sent_at         | DATETIME | DEFAULT CURRENT_TIMESTAMP          |
| acknowledged    | INTEGER  | NOT NULL DEFAULT 0                 |
| acknowledged_by | TEXT     | FK -> users(id), nullable          |
| acknowledged_at | DATETIME | nullable                           |
| target_id       | TEXT     | NOT NULL DEFAULT '' *(migration011)* |
| recipient_id    | TEXT     | NOT NULL DEFAULT '' *(migration011)* |

**Indexes:**
- `idx_ah_rule` ON alert_history(rule_id, sent_at)

**Notes:**
- `channel_id` and `acknowledged*` columns are legacy from the old alert_channels system (pre-migration011). Current code only writes `rule_id`, `target_id`, `recipient_id`, `alert_type`, `message`, `sent_at`.
- `alert_type` values: `firing`, `recovery`, `re-alert`

### target_pause_history

*(migration015)*

| Column    | Type     | Constraints                                   |
|-----------|----------|-----------------------------------------------|
| id        | INTEGER  | **PK** AUTOINCREMENT                          |
| target_id | TEXT     | NOT NULL, FK -> targets(id) ON DELETE CASCADE |
| paused_at | DATETIME | NOT NULL                                      |
| resumed_at| DATETIME | NULL (open-ended if still paused)             |
| reason    | TEXT     | NOT NULL DEFAULT ''                           |

**Indexes:** `idx_pause_history_target(target_id)`

### target_alert_recipients

Junction table: which users receive alerts for which targets.

| Column    | Type | Constraints                                   |
|-----------|------|-----------------------------------------------|
| target_id | TEXT | NOT NULL, FK -> targets(id) ON DELETE CASCADE |
| user_id   | TEXT | NOT NULL, FK -> users(id) ON DELETE CASCADE   |

**PK:** (target_id, user_id) composite

### audit_logs

Append-only audit trail. Purged by `PurgeOldAuditEntries(days)` (runs at startup + daily).

| Column        | Type     | Constraints               |
|---------------|----------|---------------------------|
| id            | INTEGER  | **PK AUTOINCREMENT**      |
| user_id       | TEXT     | NOT NULL                  |
| username      | TEXT     | NOT NULL                  |
| action        | TEXT     | NOT NULL                  |
| resource_type | TEXT     | NOT NULL DEFAULT ''       |
| resource_id   | TEXT     | NOT NULL DEFAULT ''       |
| detail        | TEXT     | NOT NULL DEFAULT ''       |
| ip_address    | TEXT     | NOT NULL DEFAULT ''       |
| status        | TEXT     | NOT NULL DEFAULT 'success' |
| created_at    | DATETIME | DEFAULT CURRENT_TIMESTAMP |

**Indexes:**
- `idx_audit_created` ON audit_logs(created_at DESC)

---

## Dropped Tables

| Table           | Created     | Dropped      | Reason                              |
|-----------------|-------------|--------------|-------------------------------------|
| projects        | migration002 | migration004 | Targets became standalone           |
| alert_channels  | migration005 | migration011 | Replaced by target_alert_recipients |
| rule_alerts     | migration005 | migration011 | Replaced by target_alert_recipients |

---

## Migration History

> **A-042 (18/03/2026):** All 22 migration functions were collapsed into a single `baselineSchema` constant in `store.go`. Fresh installs run the baseline SQL and stamp schema version 24. Existing installs at v22 run migration023+024. The individual migration functions no longer exist in code — git history preserves them. Databases below v22 cannot auto-upgrade (the old migration code is removed).
>
> **migration023 (19/03/2026):** Recreates `tag_options` table with CHECK constraint expanded to include `'category'`. Seeds 5 default categories (Key Services, Network, Other, Physical Security, Security). SQLite doesn't support ALTER CHECK, so uses create-new/copy/drop/rename pattern.

> **migration025 (23/04/2026):** Adds `'tag'` to the `tag_options.grp` CHECK (same create-new/copy/drop/rename pattern). Creates `target_tags` join table with PK `(target_id, tag_id)` and index on `tag_id`. Both FKs use `ON DELETE CASCADE` so the join table self-maintains when either side is removed.
>
> **migration024 (19/03/2026):** Seeds `signal_skip_tls` setting (default `'false'`). Part of C-1 fix — Signal TLS verification now configurable (was hardcoded `InsecureSkipVerify: true`).

The table below is retained as historical context for how the schema evolved:

| #   | Changes |
|-----|---------|
| 001 | Create `users`, `sessions`, `settings` tables. Seed default settings. |
| 002 | Create `projects`, `targets`, `checks`, `check_results` tables + indexes. |
| 003 | Add `preferred_check_type` column to targets. Seed `soc_public` setting. |
| 004 | Drop `projects` table. Rebuild `targets` without `project_id` FK. `name` becomes globally unique. |
| 005 | Create `rules`, `rule_conditions`, `rule_states`, `alert_channels`, `rule_alerts`, `alert_history` tables. |
| 006 | Add `operator`, `severity`, `rule_id` columns to targets. Auto-link existing single-target rules. Delete multi-target orphan rules. |
| 007 | Rename `targets.severity` to `targets.category`. Remap old severity values to 'Other'. |
| 008 | Create `audit_logs` table + index. |
| 009 | Re-seed `audit_retention_days` setting (INSERT OR IGNORE). |
| 010 | Remap old granular categories to simplified set: ISP/Router->Network, FW/VPN/SIEM/PAM->Security, IT Server->Key Services. |
| 011 | Create `target_alert_recipients`. Add `phone` to users. Add `target_id`, `recipient_id` to alert_history. Drop `rule_alerts`, `alert_channels`. Seed alerting settings. |
| 012 | Backfill rules for targets that have checks but no rule_id. |
| 013 | Seed 5 SLA threshold settings (`sla_network`, `sla_security`, `sla_physical_security`, `sla_key_services`, `sla_other`), all default `99.9`. |
| 014 | Add `condition_group` (INTEGER DEFAULT 0) and `group_operator` (TEXT DEFAULT 'AND') to `rule_conditions`. Backfill `group_operator` from parent rule's `operator`. |
| 015 | Add `paused_at` (DATETIME DEFAULT NULL) to `targets`. Create `target_pause_history` table with index on `target_id`. |
| 016 | Seed Signal alerting settings: `signal_api_url`, `signal_number`, `signal_username`, `signal_password`. |
| 017 | Create composite index `idx_check_results_check_id_checked_at` on check_results(check_id, checked_at DESC) for dashboard/history performance. |
| 018 | Rebuild `checks` table to add `snmp_v2c` and `snmp_v3` to type CHECK constraint. Seed 7 SNMP settings. |
| 019 | Add `notes`, `contacts`, `project`, `location` columns (TEXT DEFAULT NULL) to `targets`. Create `tag_options` table. |
| 020 | Create `check_state` and `check_daily_rollups` tables. Backfill from existing `check_results`. Purge raw results older than 3 days. |
| 021 | Add 3 performance indexes: `idx_rule_conditions_check_id`, `idx_rule_conditions_rule_id`, `idx_targets_rule_id`. |
| 022 | Update `history_days` seed from 90 to 3 for raw result retention (aligns with A-011 3-day default). |
| 023 | Recreate `tag_options` with `category` added to CHECK. Seed 5 default categories. |
| 024 | Seed `signal_skip_tls` setting. |
| 025 | Recreate `tag_options` with `tag` added to CHECK. Create `target_tags` join table for many-to-many free-form labels. |

---

## Entity Relationships

```
users
  |
  |--< sessions          (user_id FK, CASCADE)
  |--< target_alert_recipients  (user_id FK, CASCADE)
  |--< alert_history     (acknowledged_by FK, no CASCADE)
  |                       (recipient_id, logical ref, no FK)

targets
  |
  |--< checks            (target_id FK, CASCADE)
  |--< target_alert_recipients  (target_id FK, CASCADE)
  |--< target_pause_history     (target_id FK, CASCADE)
  |--- rules              (rule_id, logical 1:1 ref, nullable)

checks
  |
  |--< check_results     (check_id FK, CASCADE)
  |--- check_state        (check_id PK/FK, CASCADE, 1:1)
  |--< check_daily_rollups (check_id FK, CASCADE)
  |--< rule_conditions   (check_id FK, CASCADE)

rules
  |
  |--< rule_conditions   (rule_id FK, CASCADE)
  |--< rule_states        (rule_id FK, CASCADE, 1:1)
  |--< alert_history     (rule_id, logical ref, no FK)

tag_options                (app-level ref from targets.project/location/category)
  |
  |--< target_tags         (tag_id FK, ON DELETE CASCADE — many-to-many 'tag' group)
settings                  (standalone key-value)
schema_version            (standalone, single row)
audit_logs                (standalone, no FKs)
```

### Key Relationship: Target -> Rule -> Check (Unified Model)

The system uses a "unified target model" where:

1. A **target** has 0..1 **rule** (linked via `targets.rule_id`)
2. A **target** has 0..N **checks** (via `checks.target_id` FK)
3. Each **check** has a corresponding **rule_condition** (via `rule_conditions.check_id`) that belongs to the target's rule
4. Each condition's **group_operator** (AND/OR) and **condition_group** define how conditions combine — within a group by `group_operator`, across groups by OR
5. **rule_states** tracks whether the rule is `healthy` or `unhealthy`
6. The rule's **operator** column is kept for backward compat but unused by the engine

```
Target (rule_id) ----1:1----> Rule (id)
   |                            |
   |--< Check (target_id)      |--< RuleCondition (rule_id, check_id)
         |                                |
         |--- CheckState (1:1)            +--- references Check
         |--< CheckDailyRollups
         |--< CheckResult (3-day window)
```

Target CRUD (`CreateTargetWithConditions`, `UpdateTargetWithConditions`) manages rules, checks, and conditions as a single transaction. Rules are "hidden" from the user -- they only interact with targets and conditions.

### Alert Flow

```
Target --< TargetAlertRecipients >-- Users
   |
   +-- rule_id -> Rule -> RuleState (healthy/unhealthy)
                            |
                            v
                     AlertHistory (target_id, rule_id, recipient_id)
```

When a rule transitions to `unhealthy`, the system looks up `target_alert_recipients` to determine who gets notified, then logs each notification in `alert_history`.

---

## Cascade Behavior Summary

| Parent     | Child                    | On Delete |
|------------|--------------------------|-----------|
| users      | sessions                 | CASCADE   |
| users      | target_alert_recipients  | CASCADE   |
| targets    | checks                   | CASCADE   |
| targets    | target_alert_recipients  | CASCADE   |
| targets    | target_pause_history     | CASCADE   |
| checks     | check_results            | CASCADE   |
| checks     | check_state              | CASCADE   |
| checks     | check_daily_rollups      | CASCADE   |
| checks     | rule_conditions          | CASCADE   |
| rules      | rule_conditions          | CASCADE   |
| rules      | rule_states              | CASCADE   |

**Non-cascading FKs:** `alert_history.acknowledged_by -> users(id)` (no cascade behavior specified).

**App-level cleanup:** Deleting a target also deletes its linked rule via application code (`DeleteTarget` explicitly deletes the rule before the target).
