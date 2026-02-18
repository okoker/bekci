# Database Schema Reference

**Current schema version:** 11
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
| default_check_interval | 300     | migration001 |
| audit_retention_days   | 91      | migration001 (re-seeded migration009) |
| soc_public             | false   | migration003 |
| alert_method           | email   | migration011 |
| resend_api_key         |         | migration011 |
| alert_from_email       |         | migration011 |
| alert_cooldown_s       | 1800    | migration011 |
| alert_realert_s        | 3600    | migration011 |

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
| created_at           | DATETIME | DEFAULT CURRENT_TIMESTAMP           |
| updated_at           | DATETIME | DEFAULT CURRENT_TIMESTAMP           |

**Notes:**
- `project_id` FK was removed in migration004 (projects table dropped).
- `UNIQUE(project_id, name)` was replaced by `UNIQUE(name)` after migration004 table rebuild.
- `rule_id` is a logical FK to `rules(id)` but not enforced at DDL level (nullable, set by app code).
- `category` was originally `severity` (renamed in migration007). Values remapped in migration010.

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

Each condition links a rule to a check with evaluation criteria.

| Column      | Type    | Constraints                                   |
|-------------|---------|-----------------------------------------------|
| id          | TEXT    | **PK**                                        |
| rule_id     | TEXT    | NOT NULL, FK -> rules(id) ON DELETE CASCADE   |
| check_id    | TEXT    | NOT NULL, FK -> checks(id) ON DELETE CASCADE  |
| field       | TEXT    | NOT NULL DEFAULT 'status'                     |
| comparator  | TEXT    | NOT NULL DEFAULT 'eq'                         |
| value       | TEXT    | NOT NULL                                      |
| fail_count  | INTEGER | NOT NULL DEFAULT 1                            |
| fail_window | INTEGER | NOT NULL DEFAULT 0                            |
| sort_order  | INTEGER | NOT NULL DEFAULT 0                            |

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

Append-only log of sent alerts. Not purged automatically.

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
- `alert_type` values: `firing`, `recovery`

### target_alert_recipients

Junction table: which users receive alerts for which targets.

| Column    | Type | Constraints                                   |
|-----------|------|-----------------------------------------------|
| target_id | TEXT | NOT NULL, FK -> targets(id) ON DELETE CASCADE |
| user_id   | TEXT | NOT NULL, FK -> users(id) ON DELETE CASCADE   |

**PK:** (target_id, user_id) composite

### audit_logs

Append-only audit trail. Purged by `PurgeOldAuditEntries(days)`.

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

**Note:** Migration ordering in code is: 001, 002, 003, **004** (listed after 005 in source but runs 4th), 005, 006, 007, 008, 009, 010, 011. The function declarations are out of order in the source file but the `migrations` slice defines the correct execution sequence.

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
4. The rule's **operator** (AND/OR) defines how conditions combine
5. **rule_states** tracks whether the rule is `healthy` or `unhealthy`

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
| checks     | check_results            | CASCADE   |
| checks     | rule_conditions          | CASCADE   |
| rules      | rule_conditions          | CASCADE   |
| rules      | rule_states              | CASCADE   |

**Non-cascading FKs:** `alert_history.acknowledged_by -> users(id)` (no cascade behavior specified).

**App-level cleanup:** Deleting a target also deletes its linked rule via application code (`DeleteTarget` explicitly deletes the rule before the target).
