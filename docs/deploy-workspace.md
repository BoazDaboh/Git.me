# Deploy workspace

Open a project, choose **Deploy**, then select a service by its saved title. Frontend/Backend profiles keep their GitHub link, local commands, runtime controls and local health. **Open deployment** opens Deploy for that exact service.

## Setup

- Link or change the service's GitHub account/repository. This is the same saved link shown in Frontend/Backend, used by activity and release checks. Setup shows connection controls only; repository history stays in Frontend/Backend and is not fetched here.
- Link the service's hosting account/project. Vercel is currently supported.
- Edit the recorded provider, deployment method, build/start commands. Deployment URLs are shown in Activity; environment values are managed with your host.
- These settings are **Reference only**. They do not modify Vercel. Use **Manage settings on Vercel** for actual host settings and secret values.
- Existing service links and release history keep their account/team/project bindings.

## Release

Choose a flow; **Save default flow** remembers it for this service without running anything.

| Flow | Actions |
| --- | --- |
| Push through GitHub | Refresh Git status, inspect changes, review and confirm staging/commit, then separately review and confirm the push. The host's Git branch rules determine Preview/Production. |
| Existing GitHub commit | Select a recorded release or verify the already-pushed current commit. Choose Preview/Production, review the exact SHA/account/project and explicitly confirm the deployment request. Local edits are excluded. |
| Local upload | Choose Preview/Production, review current service files and exclusions, then confirm source upload and build. GitHub is optional. See [local uploads](local-upload.md). |

After a confirmed push or deployment response, Git/Me opens **Activity** for that service. Switching flow, service or area does not execute a release. Open reviews must still be confirmed; closing an unfinished review cancels it.

## Activity

- **Live status:** the provider's current production deployment, plus a collapsible list of recent provider builds.
- **Deployment tracking:** saved GitHub releases and exact commit/branch deployment matches. The selected record follows you between manual Release and Activity.
- **Deployment requests:** durable manual request outcomes and returned deployment IDs. Local uploads include their source fingerprint and live build state, matched by deployment ID. Acceptance does not prove the build succeeded or is serving production.

Requests and release records resume after refresh. Unconfirmed requests are never automatically resent. Local terminal changes are rechecked before Git writes, and all prior review/expiry checks remain in place.
