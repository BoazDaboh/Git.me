# Git/Me

Git/Me is a local Windows workstation for organizing projects, tasks, notes, developer tools, and deployment workflows.

## For users

Download the Windows installer from this repository's **Releases** page when a release is available. Run the installer and follow its instructions.

The installer is being prepared for distribution. If there is no release yet, use the developer setup below to run Git/Me from source.

## For developers

The repository contains the application source and the files used to build the installer. To run from source, install Git, Node.js and Go, then clone the repository and open its folder in PowerShell:

```powershell
git clone <repository-url>
cd <repository-folder>
npm.cmd install
npm.cmd run dev
```

Open http://127.0.0.1:3015. In a second terminal in the repository folder, start the Go companion:

```powershell
npm.cmd run companion:dev
```

Keep both terminals running. The companion health endpoint is http://127.0.0.1:47821/health. See [the local companion guide](docs/local-companion.md).

The `installer/` folder contains the installer configuration; it is not itself a downloadable installer. Built installers can be attached to GitHub Releases when ready.

## Current features

- Services have custom titles such as Test, Production or Review. Use Rename beside a service title, or Service title in the editor. Titles are labels and do not change branches or provider environments. Deploy groups hosting connections; Vercel is the first supported provider.
- Project Overview shows each service's folder and copyable Command Prompt startup/setup/build commands. Process ownership and local HTTP health are separate readings. Overview process status and visible service cards refresh automatically about every thirty minutes after the initial check. Check health refreshes both immediately. Focus, reconnection and workspace saves do not bypass that interval; database ports are not treated as HTTP endpoints. Open service leads to the existing reviewed launch controls.

- Overview, projects, task board, debug workspace, notes/plans, tools, portfolio/posts, reports, PDF drafting and settings.
- Use the sun/moon button in the top bar to switch themes. Your preference is saved in this browser; the first visit follows the system theme.
- Notes and plans autosave. Add headings, bold, italic, lists, checklists, code and links using the toolbar or Markdown, then switch to Preview. Pin notes, link them to projects, export Markdown or delete them.
- Checklists are clickable in Write and Preview. Enter adds the next unchecked row; Enter on an empty row returns to text, and Backspace at the start of a row converts it to text. Existing Markdown checklists open as live checkboxes.
- Add/edit project records and notes, move tasks, save post drafts and export records.
- Click a project row to open its own page with overview, tasks, linked notes and editable details. Newly created projects have their own URLs and remain available after refresh in the same browser.
- Stage 1 service profiles: use Frontend, Backend and Other services inside a project to save separate folders, repositories, runtimes, commands and deployment targets. Profiles support local/preview/production URLs and variable names. Local process state and cloud deployment state are separate. See [the service profile guide](docs/service-profiles.md).
- Stage 4: Scan project reads supported metadata from the saved project folder and shows service suggestions for review. Commands are saved only through Review profile. Rescans preserve existing custom fields and commands by default. See [the scanning guide](docs/project-discovery.md).
- Local command suggestions update when the service stack changes. PHP, Composer, Laravel, browser JavaScript, npm-based apps and Go have starter templates. Custom edits are preserved, with an explicit Apply suggestions action to replace them. These suggestions do not scan folders or verify scripts.
- Project forms support a PNG/JPG/WebP logo, editable folder suggestions and stack autocomplete with custom entries. Logos are resized locally and saved with the project.
- Set the default project directory in Settings. New paths follow the project name until manually changed. Saving a directory records the path; it does not create or move folders.
- Work/Away records focus sessions. After sleep or a companion restart, active timers stop and completed time is recorded using the last checkpoint. Offline time is excluded. Closing the browser alone does not stop a session while Go runs; switch to Away when finished. See [timer recovery](docs/local-companion.md#work-timer-recovery).
- Stage 3: workspace records save through Go to SQLite under `%APPDATA%\GitMe\data\gitme.sqlite`. The first browser workspace migrates when the database is empty. Browser storage remains a cache for pending edits. Wait for Saved to SQLite before closing. New workspaces start without projects or tasks. See [the SQLite setup and learning guide](docs/sqlite-and-storage.md).
- Settings shows the database path and revision, with JSON export/import and conflict recovery. Newer database revisions cannot be silently overwritten by an older browser. Theme preferences stay in the browser.
- Create a plain-text document and use the browser print dialog to Save as PDF. Existing PDF editing is not implemented.
- A web manifest is provided for browser installation where supported. The local Next.js server must stay running; full offline caching and desktop background operation are not implemented.
- Stage 5: saved service profiles offer reviewed Start/Stop/Restart controls, captured stdout/stderr, exit status and separate local endpoint checks. Debug & tests lists managed launches, including removed profiles. Commands run in the background on Windows; terminal-started servers are not adopted. See [the local process guide](docs/local-process-controls.md).
- Stage 7a: connect multiple GitHub accounts in Settings, with separate tokens stored in Windows credentials. Link each frontend/backend service's repository once for Git activity and release workflows. Legacy project links move to matching services; unmatched links remain in service pickers. Tokens are excluded from exports. See [the GitHub connection guide](docs/github-connection.md).
- Stage 7b: connect Vercel accounts in Settings. Choose an account, team and hosted project per project/service to read current production and recent deployment states. Tokens remain in Windows credentials. See [the Vercel connection guide](docs/vercel-connection.md).
- Local companion, GitHub and Vercel connection checks resume automatically when connectivity returns or the app becomes visible. Cloud checks also refresh after companion recovery, with throttling and a follow-up for cached failures.
- Release flows: each service can save a default route. GitHub follows provider branch rules; manual deployment offers an explicit Preview/Production target. Each stage/commit/push/deploy keeps its own review. See [release workflows](docs/release-workflows.md).
- Stage 8b: Local Git shows changed files/diffs and supports individually reviewed staging, unstaging and local commits. Terminal changes are rechecked before writes. No push/deploy occurs. See [the local Git guide](docs/local-git.md).
- Stage 8c: Review push shows the matching HTTPS GitHub destination, branch and outgoing commits before a separate confirmation. It uses your PC's Git Credential Manager sign-in, leaves working files alone and never force pushes. See [the push guide](docs/git-push.md).
- Stage 8d: Deployment tracking records accepted pushes in SQLite and reads exact commit/branch matches from the service's linked Vercel project. It resumes on app refresh and keeps push and deployment outcomes separate. See [deployment tracking](docs/deployment-tracking.md).
- The project Deploy workspace groups service-scoped Setup, Release and Activity. Choose a service, configure reference settings, release through GitHub, an existing commit or local upload, and follow live status/history. Frontend/Backend keep local runtime and GitHub links. See [the Deploy guide](docs/deploy-workspace.md).
- Stage 8e: Manual deployment reviews an exact recorded GitHub commit and Preview/Production target before requesting a fresh Vercel build. Request history persists in SQLite; uncertain submissions are never retried automatically. See [manual deployment](docs/manual-deployment.md).
- Stage 8f: Local upload reviews current service files and exclusions before uploading to Vercel without GitHub. Activity tracks the returned deployment ID; history survives refresh. See [local uploads](docs/local-upload.md).
- Checks automatically detects available lint/type/test/build commands from local service metadata, including the actual package manager. It groups commands by folder and preserves saved custom checks. Detection does not execute commands.
- Debug & tests includes an API tester with GET and reviewed JSON POST requests, local/production address suggestions, session-only request data, status/timing and body/header views. Save expected HTTP statuses and run saved GET checks per project for Passed/Failed results. POST remains separately reviewed; results are session-only. See [API testing](docs/api-tester.md).
- Render integration, additional API methods, email sending and social publishing are not connected. Docker remains deferred.
- Stage 2: the top bar and Settings show the real Go companion connection. Checks validate its identity, time out after two seconds and repeat every ten seconds while visible. Connection status is separate from SQLite saves and managed process status.

## Check and build

```powershell
npm.cmd run lint
npm.cmd run build
npm.cmd run start
```

Stop the development server before using `start` on the same port. Run `npm.cmd run companion:test` to test health, SQLite persistence, transactions, validation and revision conflicts. Go must be installed to run the companion; the frontend remains usable while it is offline. Core local features need no credentials. The optional GitHub integration requires an account token. Database schema migrations run automatically at companion startup.

## Next stages

Add hosting integrations, reporting and desktop features incrementally. Docker remains deferred.



