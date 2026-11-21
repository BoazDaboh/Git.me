# Stage 7b: connect Vercel

Git/Me reads Vercel project and deployment status through the Go companion. GitHub and Vercel connections are independent.

## Connect an account

1. Open Settings > Vercel accounts > Create Vercel token.
2. In Vercel, name the token GitMe, choose the account or team scope you need, and set an expiry.
3. Generate and copy the full token into Git/Me's Vercel access token field. Keep it out of chat, notes and environment files.
4. Click Test & connect. Git/Me checks Vercel's authenticated user endpoint and shows the verified username.

Show/Hide lets you inspect the pasted value. Invalid or failed submissions keep the masked input for correction. A successful connection clears it.

Vercel token permissions can be broader than this integration's behavior. Account setup and status checks make read requests; the separate Stage 8e control can submit an explicitly reviewed deployment request. The selected scope must allow the user, team, project and deployment reads it needs; GitHub tokens do not work here.

Add account saves another Vercel account without replacing existing ones. Replace token only accepts the same account. Adding a token for an already connected account updates its credential instead of creating a duplicate. Tokens with different scopes for the same account share one connection; changing the token can change which teams are readable.

## Link a hosted project

1. Open project Deploy, select the frontend/backend/other service and open Setup. The GitHub repository link stays in the service's local profile.
2. Click Link Vercel project.
3. Select the Vercel account and the team that owns the hosted project. Hobby projects may belong to a team too. Choose Personal scope only for projects owned directly by the user.
4. Select a project and click Link selected project. Search queries Vercel; Next page loads more projects. Load more teams extends the team selector.
5. Wait for Saved to SQLite and the deployment reading.

Each service has one authoritative deployment link, shared by status, release tracking and manual deployment. Older project-level links remain available through Use saved deployment link on an unlinked service. Git/Me verifies the original account/team/project and the exact GitHub repository, then moves that saved link to the selected service through normal workspace autosave. It does not overwrite an existing service link or change historical release targets. A Next.js frontend and a Go Functions API deployed in the same Vercel project can link to the same hosted project. Both then display that project's deployment history; this does not measure individual function execution or resource usage.

Linking does not alter your saved deployment commands, provider labels or environment URLs. Those remain manual configuration. Unlink removes only the local link, leaving the hosted project unchanged.

## What Activity shows

- The current production deployment reported by the project's production target.
- Up to six recent deployments, including production, preview and custom targets.
- Ready, building, queued, initializing, error, canceled, blocked or unknown state.
- Creation time and available Git branch/commit SHA.
- Links to the Vercel project dashboard and validated Vercel deployment URLs.
- The last successful check time and permission/connectivity errors.

A failed newer production build does not replace the status of the deployment currently serving production. No reported production target is shown explicitly. If deployment history cannot be read, the available project/production information remains visible with a warning.

These are provider API readings, not uptime probes or function runtime monitoring. Manual deploy/rebuild is available separately in Stage 8e. Build/runtime log streams, rollback, environment editing and deployment deletion are not implemented. Open Vercel for its full dashboard.

Activity checks run about once per minute while visible. The companion caches readings for up to a minute and applies a ten-second minimum interval to manual activity/account refreshes. Stale readings retain their last successful timestamp when an error occurs.

Account connections also recheck automatically on internet return, app focus/tab return and local companion recovery. Repeated events are throttled, with a follow-up after about eleven seconds for cached failures. Invalid or expired tokens still require replacement in Settings.

## Storage and account isolation

| Data | Location |
| --- | --- |
| Account tokens | Separate Windows credential targets under `GitMe/Vercel/accounts/` |
| Account IDs and usernames, no tokens | Windows credential target `GitMe/Vercel/connections` |
| Chosen account/team/project IDs and project/scope names | SQLite workspace and JSON exports |
| Account verification and deployment cache | Companion memory |
| Unsubmitted token | Temporary input state only |

No tokens are written to SQLite, browser storage, source files or exports. The companion sends them only to `https://api.vercel.com`, using GET requests for status and a separately reviewed POST /v13/deployments for Stage 8e. Authenticated redirects remain disabled. Responses are reduced to the fields shown in the interface. Environment values and arbitrary provider metadata are not exposed or persisted.

Disconnect deletes only that account's local token and clears its cache. Existing links remain but stop reading; they never fall back to another account. Reconnecting the same account restores its stable identity. Disconnect does not revoke the token in Vercel.

Exports preserve links, not credentials. On another PC, connect the same accounts to use imported links. Invalid replacements and storage failures preserve the previous credential where storage is still available.

## Checks and troubleshooting

Restart the updated Go companion if the panel says the integration is unavailable:

```powershell
cd C:\Users\USER\project\gitme
npm.cmd run companion:dev
```

An empty project list usually means the wrong team/personal scope, a search with no matches, or the need to load another page. Access denied means the token scope or account role does not allow that read. A missing/renamed project should be looked up again through the picker. Rate-limit errors require waiting before retrying.

Automated checks use simulated Vercel responses and isolated credentials/workspaces. They cover multiple accounts, failed replacements, team separation, saved links, production versus recent build state, mobile layout, safe URLs and secret-free exports.

References: [access tokens](https://vercel.com/docs/accounts/access-tokens), [authenticated user](https://vercel.com/docs/rest-api/user/get-the-user), [teams](https://vercel.com/docs/rest-api/teams/list-all-teams), [projects](https://vercel.com/docs/rest-api/projects/retrieve-a-list-of-projects), [deployments](https://vercel.com/docs/rest-api/deployments/list-deployments).
