# Stage 3: SQLite and saving your workspace

## What SQLite does

SQLite is a database engine that reads and writes a database file directly. Unlike a typical MySQL setup in WAMP, there is no separate database server to start, no port and no database account to create. It is free and suits a personal application that stores records on one PC.

```text
You edit a note
    ↓
Next.js updates the screen and browser cache
    ↓ after a short pause
PUT /workspace sends the workspace to Go
    ↓
Go validates it and starts a transaction
    ↓
SQLite saves the records in gitme.sqlite
    ↓
Go confirms success → “Saved to SQLite”
```

| Term | Meaning in Git/Me |
| --- | --- |
| Database | The complete `gitme.sqlite` file |
| Table | A collection such as `notes` or `projects` |
| Row | One saved note, project, task or session |
| Column | A field in a row, such as its ID or payload |
| SQL | The language used to read and change database records |
| Driver | The Go library that connects `database/sql` to SQLite |
| Transaction | One complete save. It all succeeds or all rolls back. |
| Migration | Code that creates or changes the database structure |

## Setup on this PC

The driver is already installed in the companion: `modernc.org/sqlite`, pinned in `go.mod` and verified through `go.sum`. It works without installing a C compiler. You do not need to download the SQLite command-line program to run Git/Me.

Start the frontend in one PowerShell terminal:

```powershell
cd C:\Users\USER\project\gitme
npm.cmd run dev
```

Open a second terminal:

```powershell
cd C:\Users\USER\project\gitme
npm.cmd run companion:dev
```

The companion creates the database directory, file and tables automatically on first startup. Go downloads any missing dependencies using the versions recorded in `go.mod`.

The default file on your Windows account is:

```text
C:\Users\USER\AppData\Roaming\GitMe\data\gitme.sqlite
```

It sits outside your source folder, so moving the code does not move or delete your records. Settings > Your data shows the actual path. To find it in PowerShell:

```powershell
Get-Item -LiteralPath "$env:APPDATA\GitMe\data\gitme.sqlite"
```

`GITME_DATA_DIR` can select a different data directory before starting Go. Leave it unset for normal use. Changing it selects a different database, not an automatic transfer of your records.

## How existing data moves

Open your usual Git/Me browser first. If SQLite is empty, Git/Me imports that browser's existing workspace automatically. The old browser record is retained, and a pre-migration browser backup is available in Settings.

If SQLite already contains a workspace, that saved workspace loads in other browsers. Existing browser data is backed up before replacement. Git/Me does not combine different browser workspaces automatically. Export an older browser backup and review it before using Import workspace, which replaces the whole current workspace.

Projects, service profiles, tasks, notes, drafts, document drafts, settings, completed sessions and the active focus session are preserved. Theme preference remains browser-specific.

## What is inside the database?

There are five record tables: `projects`, `tasks`, `notes`, `sessions`, `drafts`. Each row has `id`, `position` and `payload`. The payload stores the record's fields as JSON, which preserves existing nested service profiles and custom details. These are separate SQLite tables, although individual record fields are not all separate SQL columns yet.

`workspace_state` holds the database identity, save revision, document, settings and active focus session. `PRAGMA user_version` tracks the schema version. Startup migrations are repeatable and do not reset saved data.

Every save uses parameterized SQL and a transaction. The revision increases after a successful save. If another browser has saved a newer revision, Go rejects the older write instead of silently overwriting it. Export pending edits, then use Load database copy in Settings to resolve this.

For this small personal workspace, Git/Me saves a complete snapshot after roughly 600 ms without an edit. It checks for database changes every five seconds while the page is visible. Larger or shared workspaces can move to individual record endpoints later.

## See your records visually

Optional: install the free [DB Browser for SQLite](https://sqlitebrowser.org/dl/). This is a database viewer/editor; SQLite itself is the engine already used by Go.

For your first inspection, stop the companion with Ctrl+C and open a backup copy of `gitme.sqlite` in DB Browser. Choose Browse Data, then `notes`. Under Execute SQL, try:

```sql
SELECT id,
       json_extract(payload, '$.title') AS title,
       json_extract(payload, '$.body') AS body
FROM notes
ORDER BY position;
```

`SELECT` reads records, `FROM` chooses the table, and `ORDER BY` sets their order. `json_extract` reads a field from the stored JSON. You do not need SQL to use the dashboard; Go runs it for you.

## Saving, backups and recovery

- Saved to SQLite: Go confirmed the write. Refreshing or using another browser can load those records.
- Pending save: a database write has not been confirmed yet. Wait before closing.
- Browser cache only: Go is unavailable. Edits remain pending in that browser and retry when Go returns. Do not clear browser storage while edits are pending.
- Review pending edits: newer database changes exist. Export your edits before loading the database copy.

Settings > Export workspace downloads the currently displayed workspace as JSON, including pending edits. Keep copies outside the project folder. Import workspace restores a valid export after confirmation; it replaces the entire workspace and then saves through Go.

To back up the database file itself, stop the companion first, then run:

```powershell
$backupDirectory = Join-Path $env:USERPROFILE 'Documents\GitMeBackups'
New-Item -ItemType Directory -Path $backupDirectory -Force
$databaseFile = Join-Path $env:APPDATA 'GitMe\data\gitme.sqlite'
$backupFile = Join-Path $backupDirectory ("gitme-" + (Get-Date -Format 'yyyyMMdd-HHmmss') + '.sqlite')
Copy-Item -LiteralPath $databaseFile -Destination $backupFile
```

Restart the companion afterwards. A copy on the same PC protects against accidental edits; keep another copy off the PC for device failure. SQLite persistence is not cloud backup, and it does not synchronize between computers.

## Files and checks

| File | Responsibility |
| --- | --- |
| `companion/internal/workspace/store.go` | Opens SQLite, migrates tables, validates and saves records |
| `companion/cmd/gitme/main.go` | Handles `GET /workspace` and `PUT /workspace` |
| `src/lib/store.ts` | Browser cache, migration, autosave and revision conflicts |
| `src/components/WorkspaceStorage.tsx` | Save status, exports, imports and recovery controls |

```powershell
npm.cmd run companion:test
npm.cmd run lint
npm.cmd run build
```

Try creating a note, wait for Saved to SQLite, refresh, then open Git/Me in another browser. Stop Go, edit the note, observe the offline status, restart Go and wait for the saved status to return. Pending edits are saved only when that browser is open and can reconnect.

Database files and journals are ignored by Git. Commit `go.mod`, `go.sum` and the source files. Do not put API secret values in notes or service profiles; this database is ordinary local storage, not a secret vault.

Sources: [SQLite use cases](https://www.sqlite.org/whentouse.html), [Go SQLite driver](https://pkg.go.dev/modernc.org/sqlite), [SQLite backups](https://www.sqlite.org/backup.html), [DB Browser](https://sqlitebrowser.org/).
