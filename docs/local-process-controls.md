# Stage 5: run local development services

## Start a service

1. Start Git/Me's frontend and Go companion in their usual terminals.
2. Open Projects, select your project, then Frontend, Backend or Other services.
3. Select a saved profile. Check its Service folder, Development command and Local port using Edit profile if needed.
4. Wait for Saved to SQLite.
5. Under Local development, click Start.
6. Review the exact working folder and command, then click Run command.

Go starts the command in the background. No extra terminal window opens. Output and errors appear in the profile's log panel, with process status and exit code. A running shell does not automatically mean the web server is ready; use Check endpoint or inspect its output.

For example, Commi can have two independent profiles:

| Service | Working folder | Development command | Port |
| --- | --- | --- | --- |
| Frontend | `C:\Users\USER\project\commi_project\web` | `npm.cmd run dev` | 3000 |
| Go API | `C:\Users\USER\project\commi_project\api` | `go run ./cmd/api` | 8080 |

These are examples; use your saved paths and actual entry points. Starting does not install packages or build prerequisites automatically. Install dependencies or fix compilation problems in your terminal as needed.

## What runs where?

```text
Git/Me frontend + companion: your two startup terminals
                          ↓
Start button → Go companion → background command + child processes
                          ↓
                 Dashboard shows their output
```

The working folder is equivalent to running `cd` before the command. Commands use Windows Command Prompt (`cmd.exe`), not PowerShell. `npm.cmd run dev`, `go run ./cmd/api`, PHP commands and your custom project CLI can all be configured. PowerShell-specific syntax needs an explicit PowerShell command.

This is a background runner with output capture, not an interactive terminal. There is no stdin prompt, shell session or interactive debugger in the dashboard. Use VS Code's terminal for Git, package installation, tests, interactive tools or debugging.

Git/Me's own frontend and companion stay in their startup terminals at this stage. Their folders and ports 3015/47821 are protected from these process controls.

## Stop and restart

- Stop terminates the process tree that Git/Me launched, including npm/Node children or the compiled program created by `go run`.
- Restart first asks you to review the current saved command. It stops the owned tree, checks the configured port, then launches a new run.
- Closing or refreshing the browser page does not stop the server. Reopening its profile retrieves the running process state.
- Stopping or closing the companion also terminates its managed process trees. There is no automatic server restart or Windows startup service yet.

On Windows, each launch is attached to a Job Object before its command begins. The job owns the shell and its descendants, with kill-on-close cleanup. Stop is a process-tree termination, not a graceful Ctrl+C. Use an app's normal shutdown procedure in a terminal if it requires a graceful shutdown.

The displayed Launch PID identifies the root shell. Node or Go server children can have different PIDs. Git/Me uses its own run identity and Windows job handle for controls; it does not take a PID you enter and kill it.

## Using your terminal too

You can keep using terminals for other work while Git/Me manages a server.

To run the same server manually, Stop it in Git/Me first, then run its usual command in your terminal. To switch back, stop that terminal's copy, then Start it in Git/Me. You can run different services simultaneously when their ports do not conflict.

A terminal-started server is not adopted by Git/Me. It may answer Check endpoint, but Start will reject an occupied configured port and Stop will not control that external server. Set the real Local port in the profile so this check can work. Without a configured port, Git/Me cannot pre-check that port. Port availability is checked before launch; the app still controls its actual binding.

## Status, logs and health

| Reading | Meaning |
| --- | --- |
| Not managed | No launch for this profile is tracked in the current companion session |
| Running | The owned root process is active |
| Stopped | Git/Me stopped the owned process tree |
| Exited | The command finished successfully |
| Failed | The command exited with an error; inspect its logs and exit code |
| Status unavailable | The browser cannot currently verify the process state |

Check endpoint sends a two-second GET request to the profile's Local URL plus Health path. Only loopback addresses on this PC are supported. Redirects are not followed. The HTTP status is shown separately from process ownership; HTTP 200 alone does not identify which process answered.

Logs combine stdout and stderr, refresh about every two seconds while visible, and retain the most recent 64 KiB. Follow scrolls to new output. Export logs downloads the captured text. Output is displayed as plain text; terminal control sequences are removed. Your program can still print private values, so treat exported logs accordingly.

Profiles remain in SQLite. Process ownership, recent launch records and logs stay in memory for the current companion session; they are not workspace exports or focus sessions. The manager supports eight simultaneous launches and keeps bounded recent records. Import/reset changes records, not running programs.

## Debug view and troubleshooting

Debug & tests > Local services lists Git/Me-managed launches across projects. It can stop an owned process even if an import or another browser removed its service profile. Normal profile editing/deletion is disabled while its managed process is active.

- Port in use: stop the existing server in its terminal or Git/Me, or update the port and command consistently.
- Runtime not found: install it and restart the companion so it inherits the updated PATH. The launcher includes the Go directory it found.
- Command fails: inspect Output & errors. This stage runs only the saved Development command; it does not fix or install project dependencies.
- Review expired/settings changed: close review, check the saved profile and review again. Reviews expire after two minutes and can only launch once.
- Request timed out: refresh status before retrying; the command may already have started.
- Process controls unavailable: restart the updated companion with `npm.cmd run companion:dev`.

```powershell
npm.cmd run companion:test
npm.cmd run lint
npm.cmd run build
```

Sources: [Windows Job Objects](https://learn.microsoft.com/en-us/windows/win32/procthread/job-objects), [assigning suspended processes to jobs](https://learn.microsoft.com/en-us/windows/win32/api/jobapi2/nf-jobapi2-assignprocesstojobobject), [Go process execution](https://pkg.go.dev/os/exec).
