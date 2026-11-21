# Stage 2: the local Go connection

This describes the original health-only stage. Stage 3 now adds workspace persistence; see [SQLite and storage](sqlite-and-storage.md) for the current setup.

Git/Me now checks whether its own Go companion is available. This companion is separate from Commi's Go API.

```text
Git/Me in your browser: http://127.0.0.1:3015
                   |
                   | GET /health
                   v
Git/Me Go companion: http://127.0.0.1:47821
```

The companion is written in Go. It will eventually manage projects using different languages; it is not limited to Go projects.

## Run the two services

From the Git/Me folder, start the frontend in one terminal:

```powershell
cd C:\Users\USER\project\gitme
pnpm.cmd run dev
```

Open another terminal for the companion:

```powershell
cd C:\Users\USER\project\gitme
pnpm.cmd run companion:dev
```

Keep both running. The frontend and Go companion run independently. Starting the frontend does not start Go; it can connect to a companion that is already running.

The companion command locates your installed Go tool and runs `go run ./cmd/gitme` from the `companion` folder. You can also run that Go command directly from that folder. Use `pnpm.cmd` in PowerShell if its script policy blocks `pnpm.ps1`.

## Stop a companion that stayed running

First use Ctrl+C in the terminal running the companion. This does not stop copies started in other terminals or background sessions. If Git/Me still shows Connected, find the process listening on its port:

```powershell
Get-NetTCPConnection -LocalPort 47821 -State Listen
```

The `OwningProcess` column is its process ID (PID). IDs change when a process restarts. Replace `12345` below with the number you just found, then inspect it:

```powershell
Get-CimInstance Win32_Process -Filter "ProcessId = 12345" | Select-Object ProcessId, Name, CommandLine
```

After confirming it is the Git/Me companion, stop that specific instance:

```powershell
Stop-Process -Id 12345
```

This immediately terminates the process. Wait for pending workspace saves to finish first. Check the port again:

```powershell
Get-NetTCPConnection -LocalPort 47821 -State Listen -ErrorAction SilentlyContinue
```

No output means there is no listener on that port. The frontend uses port `3015`; inspect that port instead when troubleshooting a frontend that stayed running. Avoid stopping every Node or Go process, since other projects may be using them.

## Work timer recovery

After a companion restart or detected sleep, the companion records the verified work session, stops the timer and excludes the offline interval. Sleep recovery also checks Windows awake time and gaps between checkpoints when a power notification is missed.

Checkpoints are saved every five seconds beside SQLite in `timer-checkpoint.json`. A graceful shutdown writes a final checkpoint. After a forced termination or a missed sleep notification, the last few seconds of work may be omitted rather than counting an unverified interval. Keep this file with the database when backing up an active timer.

The frontend pauses its display when the companion connection fails. It reloads the recovered workspace on reconnection. Earlier work segments are kept in focus history; Away ends the current session. Closing the browser alone does not stop a timer while Go remains running.

To check recovery: start Work, wait at least ten seconds, stop the companion, wait ten seconds, then run `pnpm.cmd run companion:dev` again. The timer should show zero and stop, while the verified work appears in the activity chart without those ten offline seconds. For sleep/hibernation, leave Go running, put the PC to sleep, then wake it and allow the connection and workspace checks to finish.

## Check it yourself

Open Git/Me > Settings > Local companion. You should see Connected, its version, the last check time and the response time.

Open `http://127.0.0.1:47821/health` directly. The JSON identifies `gitme-companion` and reports `status: ok`. Stage 2 listed `health`; the current companion also lists `workspace` for SQLite persistence.

Stop the companion with Ctrl+C. Use Check connection in Settings: it should show Offline. Start it again and check: it should return to Connected. The dashboard also checks every ten seconds while visible and checks again when you return to it.

A successful connection requires the expected Git/Me identity and protocol, not just any server returning HTTP 200.

## What the files do

| File | Purpose |
| --- | --- |
| `companion/go.mod` | Defines the separate Go module. Stage 3 adds the SQLite driver. |
| `companion/cmd/gitme/main.go` | Starts the local HTTP server and registers the health endpoint. |
| `companion/cmd/gitme/main_test.go` | Checks the health response, routes and allowed request origins. |
| `src/lib/companion.ts` | Calls the health endpoint and validates its response. |
| `src/components/CompanionConnection.tsx` | Shares the connection state across the dashboard and Settings. |

The server listens only on `127.0.0.1`. Browser requests are allowed from Git/Me at `http://127.0.0.1:3015` and `http://localhost:3015`.

## Checks and common issues

```powershell
npm.cmd run companion:test
```

- Offline: check that the companion terminal is still running and has no startup error.
- Incompatible workspace: the companion answered, but saved records failed the frontend's format check. Legacy lesson dates written as ISO text now convert to numeric timestamps on load and save through the normal revision checks. If the warning remains, inspect the saved format instead of repeatedly restarting or resetting the workspace.
- Unexpected service: another or incompatible service answered on port 47821. Inspect that process before stopping anything.
- Address already in use: a process already owns port 47821. Keep one companion instance running.
- Go not found: install Go or restart your terminal after installation. On Windows, the launcher also checks the standard Go installation folders.
- Changes to Go files: stop and restart the companion. `go run` does not watch for edits.

## Scope of this stage

Connected means the companion responded to a real health request. It does not mean project services or cloud accounts are connected.

Stage 2 kept workspace records in the browser. Stage 3 saves them through Go to SQLite and keeps browser storage as a pending-edit cache. Check the separate Saved to SQLite status for database writes. [Stage 4](project-discovery.md) adds discovery, and [Stage 5](local-process-controls.md) adds reviewed local process controls.
