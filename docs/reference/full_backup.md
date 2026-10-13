# Full Database Backup & Restore Reference

## Overview

Two backup types exist in Bekci:

| Type | Scope | Format | Restore |
|------|-------|--------|---------|
| **Config Backup** | 11 config tables (users, settings, rules, targets, checks, rule_conditions, rule_states, recipients, pause_history, tag_options, target_tags); format version 2 | JSON | Web UI upload |
| **Full Backup** | Entire SQLite database (all tables + historical data) + config.yaml | tar.gz (optional AES-256-GCM, off by default; scheduled backups are never encrypted) | CLI only (`bekci restore-full`) |

Both restore paths preserve stored failure counts and windows without applying the [target create/update validation](api_reference.md#post-apitargets). Restoring or starting the service does not repair impossible thresholds or shorten windows larger than 1800 seconds. The engine uses the restored values; an invalid multi-failure combination must be corrected before the target can next be saved through the API/editor.

## Full Backup

### How it works

1. **SQLite Online Backup API** (`sqlite3.SQLiteConn.Backup()`) creates a consistent snapshot of the live database to a temp file. Falls back to `VACUUM INTO` if raw connection access fails.
2. **Archive builder** creates a tar.gz containing:
   - `bekci.db` — the database snapshot
   - `config.yaml` — the server config file (if available; omitted in env-only setups like Docker)
3. **Optional encryption** wraps the archive with AES-256-GCM:
   - Key derivation: Argon2id (time=3, memory=64MB, parallelism=4, keyLen=32)
   - Random salt (16 bytes) + random nonce (12 bytes) prepended to ciphertext
   - Wire format: `salt (16B) || nonce (12B) || ciphertext+GCM_tag`

### API Endpoints

| Endpoint | Auth | Description |
|----------|------|-------------|
| `POST /api/backup/full` | admin | Download full backup (streams to browser) |
| `POST /api/backup/full/save` | admin | Save full backup to server |
| `GET /api/backup/full/list` | admin | List saved backups |
| `GET /api/backup/full/saved/{filename}` | admin | Download a saved backup |
| `DELETE /api/backup/full/saved/{filename}` | admin | Delete a saved backup |
| `GET /api/backup/generate-passphrase` | admin | Generate 4-word passphrase |

#### POST /api/backup/full

JSON body: `{"encrypt": bool, "passphrase": "string"}` (passphrase min 8 chars, required when encrypt is true). `encrypt` defaults to false: an omitted field or empty body produces a plain `.tar.gz`.

Response: binary stream with `Content-Disposition: attachment`.

File extensions:
- `.tar.gz` — plain archive
- `.tar.gz.enc` — encrypted

#### GET /api/backup/generate-passphrase

Returns `{"passphrase": "word-word-word-word"}`. Uses crypto/rand from a curated 904-word list (~40 bits entropy for 4 words).

#### POST /api/backup/full/save

Same JSON body fields as `POST /api/backup/full` (`encrypt`, `passphrase`). Saves to `backup_dir` on the server.

Response: `{"message": "backup saved", "filename": "bekci-full-20260307-015116.tar.gz", "sha256": "82d676c68f5e..."}`

#### GET /api/backup/full/list

Response: JSON array of saved backup metadata.
```json
[{"filename": "bekci-full-20260307-015116.tar.gz", "sha256": "82d6...", "size": 535424, "created_at": "2026-03-07T01:51:16Z", "encrypted": false}]
```

#### GET /api/backup/full/saved/{filename}

Filename must match `^bekci-full-\d{8}-\d{6}\.tar\.gz(\.enc)?$`. Path traversal protected.

Response: binary stream with `Content-Disposition: attachment`.

#### DELETE /api/backup/full/saved/{filename}

Same filename validation. Response: `{"message": "deleted"}`

### Server-Side Storage

**Config:** `server.backup_dir` in config.yaml. Default: `{db_dir}/backups/` (e.g. `/var/lib/bekci/backups/` in prod, `/data/backups/` in Docker).

Env override: `BEKCI_BACKUP_DIR`.

Directory auto-created on first save. Metadata tracked in `{backup_dir}/index.json` — SHA256 hash computed at save time to avoid re-hashing large files on list.

Auto-save on download + auto-prune oldest when `backup_max_copies` exceeded (default 5). The download copy is saved before the archive is sent, so it is already listed when the download arrives and is kept even if the download fails. The new archive is written first and the oldest copies are pruned only after that write succeeds, so a failed save never removes an existing backup (a partial file from the failed write is deleted). This needs room for one extra archive while saving. Manual delete also available via the web UI.

All three save paths (Save to server, the download auto-save, scheduled backups) go through one routine, `saveBackupCopy`. Saves and deletes are serialized by a single in-process lock, so concurrent operations never lose index entries. A save never overwrites an existing file: filenames have one-second resolution, so a second save within the same second fails ("backup failed", HTTP 500) instead of replacing the first. `index.json` is written to `index.json.tmp` and renamed into place, so a crash or a concurrent reader never sees a partial index. Pruned files are deleted only after the new index is saved; if the index write fails, the new archive is removed and the save reports failure, leaving the previous backups and index unchanged.

Failures are reported, never logged as success: Save to server returns 500 and audits `save_full_backup` with status `failure`; a scheduled backup audits `auto_backup` `failure` and sends a system alert ("Bekci scheduled backup failed") to the configured system alert recipients and, if enabled, the webhook (even when no recipients resolve); a download auto-save failure is logged only and the download still goes ahead.

### Frontend UI

Located in Settings > Backup & Restore tab (admin only). The "Full Database Backup" collapsible card provides:

- **Encrypt backup** toggle — when enabled, auto-fetches a passphrase
- **Passphrase display** — monospace code block with Copy and New (regenerate) buttons
- **Warning banner** — "Save this passphrase — it cannot be recovered"
- **Destination dropdown** — "Download" (streams to browser) or "Save to server" (saves to backup_dir)
- **Action button** — label changes based on destination selection
- **Saved Backups table** — lists server-side backups with filename, date, size, truncated SHA256 (click to copy), Download and Delete buttons. Max 5-6 rows visible, scrollable overflow. Delete requires confirmation popup. Reloaded after every download attempt, so the server copy of a download appears even when the download itself failed.

### Audit Trail

| Action | Description |
|--------|-------------|
| `export_full_backup` | Direct download to browser. Written after the send; `failure` if the archive could not be written to the connection |
| `save_full_backup` | Saved to server-side backup directory |
| `download_saved_backup` | Downloaded a previously saved backup. Written after the send; `failure` if the stream broke |
| `delete_saved_backup` | Deleted a saved backup from server |

---

## CLI Restore

### Usage

```
bekci restore-full <archive-path>
```

The archive path can also be provided interactively if omitted from args.

### Flow

```
1. Welcome banner + safety warning
2. Read archive file
3. If .enc extension → prompt passphrase → decrypt
4. Extract tar.gz to temp directory
5. Verify bekci.db exists in archive
6. If config.yaml bundled → display contents
7. "Use bundled config? (Y/n)"
   - Yes → use as-is
   - No  → interactive config wizard
8. Prompt destination paths (DB + config)
9. Show summary of all changes
10. "Proceed? (y/N)" — default NO
11. Remove the destination's old `-wal` / `-shm` / `-journal` side files (abort, leaving the DB untouched, if one cannot be removed) → copy DB file over the destination → set 0600 permissions
12. Write config file (bundled or wizard) → set 0600 permissions
13. Print: "Start service with: sudo systemctl start bekci"
```

### Config Wizard

When declining the bundled config, the wizard prompts for each field with the bundled value as default (press Enter to keep):

| Field | Default |
|-------|---------|
| Server port | 65000 (or from bundled config) |
| Database path | /var/lib/bekci/bekci.db |
| Log level | warn |
| Log file path | /var/log/bekci/bekci.log |
| Admin username | admin |
| Admin password | (empty — Enter accepts it) |

The wizard writes only `server.port`, `server.db_path`, `logging.level`, `logging.path` and `init_admin`; there is no `auth` section, and `server.host`, `server.backup_dir` and `server.cors_origin` fall back to env overrides or defaults. `init_admin` is only used to create the first admin when the database has no users. A restored database keeps the users from the backup, so the wizard's admin username and password create no account and reset no password — log in with the credentials stored in the backup.

### Safety

- Does NOT auto-start the service — user must manually run `systemctl start bekci`
- Does not copy or change the `.jwt_secret` file (it is not in the archive). The signing secret at next start is `BEKCI_JWT_SECRET` if set, else `auth.jwt_secret` from the written config (a bundled config may carry one; the wizard config never does), else `.jwt_secret` in the directory of the configured `db_path` (generated if missing) — so the config choice can change the secret. The sessions table is restored with the database: an unexpired session from the backup stays valid only if the effective secret is the one that signed its token and its user is still active; otherwise users log in again
- Default confirmation is NO — must explicitly type `y`
- Removes the old database's leftover WAL/journal side files before copying. An unclean stop (crash, `kill -9`, OOM, power loss) leaves committed pages in `bekci.db-wal`; SQLite would replay them onto the restored file on next start, silently mixing old pages into the snapshot or corrupting it. A normal `systemctl stop` closes the database cleanly and leaves none. The existing database file is overwritten in place, so it keeps its owner.
- Archive extraction sanitizes filenames (only base names, no path traversal)
- Each extracted entry is copied up to the size declared in its own tar header. That size comes from the archive, and there is no separate limit on entry size, total extracted size or entry count, so this does not guard against decompression bombs. The whole archive is read (and decrypted) in memory before extraction

### Typical Restore Procedure

```bash
# 1. Stop the service
sudo systemctl stop bekci

# 2. Run restore
bekci restore-full /path/to/bekci-full-20260306-235000.tar.gz.enc
# Follow interactive prompts

# 3. Start the service
sudo systemctl start bekci
```

---

## Code Map

| File | Purpose |
|------|---------|
| `internal/crypto/crypto.go` | AES-256-GCM encrypt/decrypt with Argon2id KDF |
| `internal/crypto/diceware.go` | Passphrase generator (904-word list) |
| `internal/crypto/crypto_test.go` | Encrypt/decrypt round-trip, wrong passphrase, edge cases |
| `internal/crypto/diceware_test.go` | Word count, uniqueness, defaults |
| `internal/store/full_backup.go` | SQLite online backup API with VACUUM INTO fallback |
| `internal/api/backup_handlers.go` | `handleFullBackup`, `handleGeneratePassphrase`, server-side save with retention (`handleSaveFullBackup`, `saveBackupCopy`), saved-backup list/download/delete, `RunScheduledBackup` |
| `internal/api/backup_retention_test.go` | Save keeps existing backups when the write fails; pruning keeps exactly the newest copies; same-name save refused; concurrent saves/deletes keep the index; index write failure changes nothing; scheduled backup failure audited and alerted |
| `cmd/bekci/restore.go` | CLI `restore-full` subcommand with interactive wizard; `replaceDatabase` removes stale side files before copying |
| `cmd/bekci/restore_test.go` | Stale WAL discarded on restore, removal failure leaves DB untouched, fresh destination |

## Encryption Details

| Parameter | Value |
|-----------|-------|
| Algorithm | AES-256-GCM |
| KDF | Argon2id |
| Argon2 time | 3 iterations |
| Argon2 memory | 64 MB |
| Argon2 parallelism | 4 threads |
| Key length | 32 bytes (256 bits) |
| Salt length | 16 bytes (random per encryption) |
| Nonce length | 12 bytes (random per encryption) |
| Auth tag | 16 bytes (GCM default) |
