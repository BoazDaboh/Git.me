# Stage 4: scan an existing project

## Use it

1. Keep the Next.js frontend and Go companion running.
2. Open Projects, then choose a project.
3. Check its Local folder. Use Edit project if the path needs changing, then wait for Saved to SQLite.
4. Click Scan project in Project discovery.
5. Expand Files and suggested commands to see the evidence.
6. Click Review profile, edit the suggested fields, then Save profile.
7. Wait for Saved to SQLite. Use Open profile to view it under Frontend, Backend or Other services.

For Git/Me itself, the project folder is `C:\Users\USER\project\gitme`. Scanning it finds the Next.js app in the root and the Go module under `companion`. The detected Go development command is `go run ./cmd/gitme`, run from the companion folder.

## What happens behind the button?

```text
Dashboard sends the saved project ID
                 ↓
Go reads that project's folder path from SQLite
                 ↓
Go inspects supported metadata inside that folder
                 ↓
Dashboard shows temporary suggestions
                 ↓
You review and save a profile → SQLite
```

The scan itself does not change your source files or saved workspace. Cancelling review saves nothing. Save profile records configuration; it does not run the commands.

| Evidence | Suggestions |
| --- | --- |
| `package.json` | Node.js and declared tools, plus existing dev/build/test/start scripts |
| `package-lock.json`, `pnpm-lock.yaml`, `yarn.lock`, Bun lockfiles or `packageManager` | Package manager; npm uses `ci` when a package lock is present |
| `go.mod`, root `main.go`, `cmd/*/main.go` | Go version requirement and likely entry points |
| `composer.json`, `artisan` | Composer/PHP commands and Laravel when detected |
| `index.php` or `public/index.php` | Plain PHP development server suggestion |
| `index.html` | Static frontend marker |
| `Dockerfile` | Container configuration exists; container controls come later |
| `.git/HEAD`, `.git/config` | Local branch and sanitized origin URL |

Node detection recognizes common declared tools including Next.js, React, NestJS, Express, Vite, Vue, Nuxt, Svelte, Astro, TypeScript and Tailwind CSS. It only suggests npm-family run commands for script names that exist. It does not copy raw script bodies into service profiles.

Go entry points are inferred from filenames, not compiled. A library or another layout may need run commands entered manually. Ports are detected from a numeric development-script port flag or suggested from framework defaults, and remain editable. Detecting a runtime in files does not verify that it is installed or that the app works.

## Existing profiles and rescans

Scan again reads the files afresh. Detected profiles have a saved source key and folder so they can be matched on a later scan. Matching manual profiles may also be suggested by folder, service type and stack.

- New service profile creates an additional profile after review.
- Update: [name] opens the existing profile for review. Saved fields and custom commands, including intentionally blank commands, are preserved by default.
- Use detected local commands explicitly replaces the local command suggestions in review. Deployment commands remain separate.
- Add reviewed technologies to this project's stack merges the reviewed profile's stack into the project when you save. Turn it off to keep the project stack unchanged.

You can change the Save as target before reviewing. Rescanning does not delete profiles for removed files or folders.

## Scope and troubleshooting

Scanning stays inside the saved project folder using Go's directory-scoped filesystem access. It checks up to three subfolder levels, 160 directories and 48 service suggestions. Metadata files larger than 512 KiB are skipped. Dependency folders, generated output, hidden folders, source asset folders and linked directories are skipped. Scan notes explain limits or unreadable metadata.

`.env` files, keys and credentials are not read. Raw Git configuration is not returned; credentials and query parameters are removed from supported remote URLs. Linked Git metadata outside the chosen root is left unread. A subfolder will not inherit Git information from outside the selected root.

- Folder missing: update the project path or restore the folder, save, then scan again.
- Wait for Saved to SQLite: the scan uses the database's saved project path. Pending browser edits must save first.
- Could not reach Go: start the companion with `npm.cmd run companion:dev`.
- Scan endpoint not found: restart Go after updating it. `go run` does not watch source changes.
- No supported service found: select a more specific folder or create a manual service profile.
- Partial results: inspect Scan notes, then scan the relevant subproject as its own project if needed.

Run `npm.cmd run companion:test` for scanner and API checks. Frontend checks remain `npm.cmd run lint` and `npm.cmd run build`.

Sources: [npm package metadata](https://docs.npmjs.com/cli/v11/configuring-npm/package-json/), [Go modules](https://go.dev/ref/mod), [Go directory-scoped access](https://pkg.go.dev/os#Root), [Composer metadata](https://getcomposer.org/doc/04-schema.md), [Git layout](https://git-scm.com/docs/gitrepository-layout).

## Stack-aware checks

Open **Debug & tests > Checks** and choose a project. Git/Me reads its local metadata automatically and groups check commands by service folder. **Detect again** refreshes after files change; there is no background scan timer.

- npm, pnpm, Yarn and Bun: only declared lint, type-check, test and build scripts appear. The default "no test specified" placeholder is excluded.
- Go modules: tests, vet and a build across the module's packages.
- PHP: syntax checks for detected entry files. Composer adds validation and declared check scripts; Laravel adds its test command when appropriate.
- Saved custom test/build commands remain labelled separately. Detection never overwrites service profiles or executes commands.

Copy a command and run it from its displayed folder. If the folder or companion is unavailable, only saved commands are shown, with their source labelled. Plain static or unsupported stacks do not receive invented npm commands.
