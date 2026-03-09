# Database Schema Reference

**Current schema version:** 17
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

Tracks current migration version. Single row.

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
| history_days           | 90      | migration001 |
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
| created_at           | DATETIME | DEFAULT CURRENT_TIMESTAMP           |
| updated_at           | DATETIME | DEFAULT CURRENT_TIMESTAMP           |

**Notes:**
- `project_id` FK was removed in migration004 (projects table dropped).
- `UNIQUE(project_id, name)` was replaced by `UNIQUE(name)` after migration004 table rebuild.
- `rule_id` is a logical FK to `rules(id)` but not enforced at DDL level (nullable, set by app code).
- `category` was originally `severity` (renamed in migration007). Values remapped in migration010. DDL default is `'critical'` but app code defaults to `'Other'` when empty.
- `idx_targets_project_id` index from migration002 is orphaned after migration004 dropped the projects table (harmless but unclean).
- `paused_at` — when NOT NULL, target is paused: scheduler skips checks, dashboard shows "PAUSED" badge.

### checks

| Column     | Type     | Constraints                                                  |
|------------|----------|--------------------------------------------------------------|
| id         | TEXT     | **PK**                                                       |
| target_id  | TEXT     | NOT NULL, FK -> targets(id) ON DELETE CASCADE                |
| type       | TEXT     | NOT NULL, CHECK(type IN ('http','tcp','ping','dns','page_hash','tls_cert')) |
| name       | TEXT     | NOT NULL                                                     |
| config     | TEXT     | NOT NULL DEFAULT '{}'                                        |
| interval_s | INTEGER  | NOT NULL DEFAULT 300                                         |
| enabled    | INTEGER  | NOT NULL DEFAULT 1                                           |
| created_at | DATETIME | DEFAULT CURRENT_TIMESTAMP                                    |
| updated_at | DATETIME | DEFAULT CURRENT_TIMESTAMP                                    |

**Indexes:**
- `idx_checks_target_id` ON checks(target_id)

### check_results

High-volume time-series data. Purged by `PurgeOldResults(days)`.

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

| #   | Function       | Changes |
|-----|----------------|---------|
| 001 | migration001   | Create `users`, `sessions`, `settings` tables. Seed default settings. |
| 002 | migration002   | Create `projects`, `targets`, `checks`, `check_results` tables + indexes. |
| 003 | migration003   | Add `preferred_check_type` column to targets. Seed `soc_public` setting. |
| 004 | migration004   | Drop `projects` table. Rebuild `targets` without `project_id` FK. `name` becomes globally unique. |
| 005 | migration005   | Create `rules`, `rule_conditions`, `rule_states`, `alert_channels`, `rule_alerts`, `alert_history` tables. |
| 006 | migration006   | Add `operator`, `severity`, `rule_id` columns to targets. Auto-link existing single-target rules. Delete multi-target orphan rules. |
| 007 | migration007   | Rename `targets.severity` to `targets.category`. Remap old severity values to 'Other'. |
| 008 | migration008   | Create `audit_logs` table + index. |
| 009 | migration009   | Re-seed `audit_retention_days` setting (INSERT OR IGNORE). |
| 010 | migration010   | Remap old granular categories to simplified set: ISP/Router->Network, FW/VPN/SIEM/PAM->Security, IT Server->Key Services. |
| 011 | migration011   | Create `target_alert_recipients`. Add `phone` to users. Add `target_id`, `recipient_id` to alert_history. Drop `rule_alerts`, `alert_channels`. Seed alerting settings. |
| 012 | migration012   | Backfill rules for targets that have checks but no rule_id. |
| 013 | migration013   | Seed 5 SLA threshold settings (`sla_network`, `sla_security`, `sla_physical_security`, `sla_key_services`, `sla_other`), all default `99.9`. |
| 014 | migration014   | Add `condition_group` (INTEGER DEFAULT 0) and `group_operator` (TEXT DEFAULT 'AND') to `rule_conditions`. Backfill `group_operator` from parent rule's `operator`. |
| 015 | migration015   | Add `paused_at` (DATETIME DEFAULT NULL) to `targets`. Create `target_pause_history` table with index on `target_id`. |
| 016 | migration016   | Seed Signal alerting settings: `signal_api_url`, `signal_number`, `signal_username`, `signal_password`. |
| 017 | migration017   | Create composite index `idx_check_results_check_id_checked_at` on check_results(check_id, checked_at DESC) for dashboard/history performance. |

**Note:** Function declarations appear out of order in the source file (e.g. migration005 before migration004, migration008 before migration007), but the `migrations` slice defines the correct sequential execution order: 001 through 016, strictly in order.

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
  |--< rule_conditions   (check_id FK, CASCADE)

rules
  |
  |--< rule_conditions   (rule_id FK, CASCADE)
  |--< rule_states        (rule_id FK, CASCADE, 1:1)
  |--< alert_history     (rule_id, logical ref, no FK)

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
         |--< CheckResult                 +--- references Check
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
| checks     | rule_conditions          | CASCADE   |
| rules      | rule_conditions          | CASCADE   |
| rules      | rule_states              | CASCADE   |

**Non-cascading FKs:** `alert_history.acknowledged_by -> users(id)` (no cascade behavior specified).

**App-level cleanup:** Deleting a target also deletes its linked rule via application code (`DeleteTarget` explicitly deletes the rule before the target).
