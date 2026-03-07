# Full Database Backup & Restore Reference

## Overview

Two backup types exist in Bekci:

| Type | Scope | Format | Restore |
|------|-------|--------|---------|
| **Config Backup** | 9 config tables (users, targets, checks, rules, settings, recipients) | JSON | Web UI upload |
| **Full Backup** | Entire SQLite database (all tables + historical data) + config.yaml | tar.gz (optional AES-256-GCM) | CLI only (`bekci restore-full`) |

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
| `GET /api/backup/full` | admin | Download full backup (streams to browser) |
| `POST /api/backup/full/save` | admin | Save full backup to server |
| `GET /api/backup/full/list` | admin | List saved backups |
| `GET /api/backup/full/saved/{filename}` | admin | Download a saved backup |
| `DELETE /api/backup/full/saved/{filename}` | admin | Delete a saved backup |
| `GET /api/backup/generate-passphrase` | admin | Generate 4-word passphrase |

#### GET /api/backup/full

| Param | Required | Description |
|-------|----------|-------------|
| `encrypt` | no | `true` to encrypt |
| `passphrase` | if encrypt=true | Min 8 chars |

Response: binary stream with `Content-Disposition: attachment`.

File extensions:
- `.tar.gz` — plain archive
- `.tar.gz.enc` — encrypted

#### GET /api/backup/generate-passphrase

Returns `{"passphrase": "word-word-word-word"}`. Uses crypto/rand from a curated ~960-word list (~40 bits entropy for 4 words).

#### POST /api/backup/full/save

Same query params as `GET /api/backup/full` (`encrypt`, `passphrase`). Saves to `backup_dir` on the server.

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

No auto-purge — deletion is manual via the web UI.

### Frontend UI

Located in Settings > Backup & Restore tab (admin only). The "Full Database Backup" collapsible card provides:

- **Encrypt backup** toggle — when enabled, auto-fetches a passphrase
- **Passphrase display** — monospace code block with Copy and New (regenerate) buttons
- **Warning banner** — "Save this passphrase — it cannot be recovered"
- **Destination dropdown** — "Download" (streams to browser) or "Save to server" (saves to backup_dir)
- **Action button** — label changes based on destination selection
- **Saved Backups table** — lists server-side backups with filename, date, size, truncated SHA256 (click to copy), Download and Delete buttons. Max 5-6 rows visible, scrollable overflow. Delete requires confirmation popup.

### Audit Trail

| Action | Description |
|--------|-------------|
| `export_full_backup` | Direct download to browser |
| `save_full_backup` | Saved to server-side backup directory |
| `download_saved_backup` | Downloaded a previously saved backup |
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
11. Copy DB file → set 0600 permissions
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
| Admin password | (none — must provide) |

### Safety

- Does NOT auto-start the service — user must manually run `systemctl start bekci`
- Does NOT modify JWT secret — all users must re-login after restore
- Default confirmation is NO — must explicitly type `y`
- Archive extraction sanitizes filenames (only base names, no path traversal)
- Copy size limited by tar header (prevents decompression bombs)

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
| `internal/crypto/diceware.go` | Passphrase generator (~960-word list) |
| `internal/crypto/crypto_test.go` | Encrypt/decrypt round-trip, wrong passphrase, edge cases |
| `internal/crypto/diceware_test.go` | Word count, uniqueness, defaults |
| `internal/store/full_backup.go` | SQLite online backup API with VACUUM INTO fallback |
| `internal/api/backup_handlers.go` | `handleFullBackup`, `handleGeneratePassphrase` handlers |
| `cmd/bekci/restore.go` | CLI `restore-full` subcommand with interactive wizard |

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
