# Stage 1: project service profiles

A project is the product you are building. A service is one part of that product that can have its own folder, repository, commands and deployment target.

For example, Commi has a Next.js frontend and a Go API. Keep them as separate service profiles even if they share one repository.

## Try it

1. Open Projects, then Commi.
2. Open Frontend and choose Review suggested profile.
3. Review the three local sections below and save the profile. Deployment configuration is managed separately under project Deploy.
4. Open Backend and repeat for the Go API.
5. Refresh the page, reopen either tab and check your saved configuration.

Suggestions come from existing project details and starter templates. They are not results of a folder scan or a cloud connection. Review the folder paths and commands before saving.

Changing a service's stack updates its suggested local commands. JavaScript/PHP projects start with separate frontend JavaScript and backend PHP suggestions. You can select a specific Command suggestions template if the stack is ambiguous.

Plain PHP suggests `php -S 127.0.0.1:8080` for local development, using the profile's port. It needs no dependency installation by itself. Add Composer to the stack to suggest `composer install`, which requires `composer.json`. Plain browser JavaScript also has no automatic install command; npm templates assume an npm-based project and matching scripts.

References: [PHP development server](https://www.php.net/manual/en/features.commandline.webserver.php), [Composer dependency installation](https://getcomposer.org/doc/01-basic-usage.md).

Commands you edit are marked Custom and retained when suggestions change. Apply suggestions explicitly replaces them. Saving keeps these choices; cancelling leaves the saved profile unchanged.

## The local profile sections

| Section | What you record |
| --- | --- |
| Identity | Service name, type, runtime, stack, repository and branch. |
| Local setup | The folder in which commands will run later, the intended port, local URL, health path and local commands. |
| Local environment | Local URL and required variable names. |

For hosting connections, deployment reference commands and Preview/Production configuration, open **Deploy > select service > Setup**. See [the Deploy guide](deploy-workspace.md).

Other services can hold workers, databases or another service type. You can add multiple services of the same type.

## Why local and deployment commands are separate

A local Go build on Windows might use:

```powershell
go build -o bin/api.exe ./cmd/api
```

A Linux host might use:

```text
go build -o api ./cmd/api
```

These are configuration examples. The profile form saves their text; it does not execute them.

## What the status means

Saved configuration means Git/Me has your profile record. Not connected means Git/Me has not connected this profile to local controls or a cloud account. It does not mean the application itself is offline.

Ports, URLs and provider names are manually entered targets. No running process, deployment or health result is inferred from them.

Record variable names such as `API_URL` or `PORT`, without values. The form rejects pasted `NAME=value` entries. Secret values are not part of this stage.

Profiles save in the existing browser workspace and are included in Settings > Export workspace. Existing projects, notes and tasks are preserved. Deleting a profile only removes its saved configuration; it does not delete folders, repositories or hosted services.

Next stage: connect Git/Me to its own local Go companion. Database storage, folder detection, process controls, Docker and cloud integrations follow in later stages.
