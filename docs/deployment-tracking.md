# Stage 8d: follow a Git deployment

**Push sends commits to GitHub. Deployment tracking reads what Vercel built for that exact commit.**

## Setup and use

1. Open project Deploy, select the service and use Setup to link its Vercel account, team and hosted project. Tracking uses that same explicit service link.
2. In Release > Push through GitHub, commit and push through Local Git. An accepted push records its repository, branch, full commit ID, time and Vercel link in SQLite.
3. Activity opens after an accepted push. Deployment tracking loads there. It shows the GitHub outcome and matching Vercel builds, their actual Preview/Production/custom target, state and links.
4. For a commit pushed earlier or from your terminal, click **Track current GitHub commit**. Git/Me verifies that the local branch HEAD equals GitHub's current branch tip, then records it as an existing verified commit. It never pushes from this button.
5. Refreshing the app restores the latest record and reads its current provider status. The history selector shows the last ten records for this service.

If a record had no Vercel service link, link the service and use Track current GitHub commit. If a link changes, older records keep their original binding; the button creates a record for the explicitly selected new target. Git/Me never switches accounts or inherits another project's link automatically.

## Reading the result

| Reading | Meaning |
| --- | --- |
| GitHub push accepted | Git reported success. A later build failure does not undo this outcome. |
| Existing GitHub commit verified | The current local commit was confirmed on GitHub without a new push. |
| Waiting | No exact commit/branch match has been observed yet. |
| Queued / Initializing / Building | Vercel reports a build in progress. |
| Ready | This deployment is ready at its own URL. |
| Serving production | A ready production deployment also matches Vercel's current production-target ID. |
| Build failed / Canceled / Blocked | The deployment has that outcome; the recorded GitHub push remains successful. |
| No matching deployment observed | After ten minutes, none has been found. Automatic deployment may be disabled, delayed or skipped; Git/Me does not claim a build failed. |
| Link changed / Account unavailable | Reconnect the same account or explicitly track with a new service link. No fallback is used. |

Tracking queries the saved Vercel project using exact SHA and branch filters, checks returned GitHub metadata, and verifies the project's Git repository when reported. It reads up to twenty matching candidates, including redeployments of the same version. A match identifies the code version; it does not prove which particular push or redeploy action caused it.

Active builds/waiting records check about every fifteen seconds while visible. Settled results check about once a minute. Focus, reconnect and successful push events request a fresh reading, with a ten-second provider cache minimum. The last checked time reflects the provider reading. Missing metadata or permissions produce an explicit unavailable/unobserved state.

## Storage and boundaries

The companion migrates SQLite to schema 2 by adding `release_records` and its index. Existing workspace notes, projects, revision and account credentials are preserved. Release records are local to this PC and separate from workspace JSON imports/exports. They contain no source code, diffs, token values or environment values. Provider readings are fetched again after a companion restart.

Tracking itself remains read-only. The separate Stage 8e Manual deployment control can request a fresh build after explicit review and confirmation. No token is passed to Git push. Local uploads remain Stage 8f. See [manual deployment](manual-deployment.md).

Checks use temporary repositories/databases and simulated provider responses, covering persistence, exact matching, build outcomes, account/target changes, secret filtering and mobile layouts. No real project is pushed or deployed by these checks.

References: [Vercel deployment queries](https://vercel.com/docs/rest-api/deployments/list-deployments), [project targets and Git link](https://vercel.com/docs/rest-api/projects/find-a-project-by-id-or-name).
