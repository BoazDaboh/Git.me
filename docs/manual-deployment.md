# Stage 8e: deploy or rebuild a GitHub commit

Normal flow: commit locally, push to GitHub, let Vercel auto-deploy, then follow Deployment tracking. Manual deployment is optional. Use it to retry a failed build, apply changed Vercel settings or deploy deliberately when automatic deployment is disabled.

## In Git/Me

1. Keep the service's GitHub repository linked in Frontend/Backend. Open project Deploy, choose that service and link its matching Vercel project in Setup.
2. Open Release > Existing GitHub commit. Use a recorded push or **Track current GitHub commit**. The latter verifies that the local branch is already on GitHub; it does not push.
3. Choose the recorded release, then choose **Preview** or **Production** in Manual deployment.
4. Click **Review deployment**. Check the account, Vercel project, repository, branch and full commit SHA.
5. Confirm. This sends one build request to Vercel. No local files or environment values are uploaded, and nothing is pushed to GitHub.
6. Git/Me opens Activity. Read the returned deployment ID in Deployment requests and follow that ID in Deployment tracking. Request history survives refresh and companion restart.

Preview is the default. Production may replace the version served on the live domain after a successful build, according to Vercel's domain settings. Selecting an older commit can therefore replace newer live code. A repeated deployment of unchanged code and settings should behave the same, but creates a fresh deployment.

## What Git/Me sends

The companion calls Vercel's `POST /v13/deployments`, scoped to the saved account/team/project, with an exact GitHub repository, branch and SHA. `forceNew=1` requests a new build even if the same commit was deployed already. Preview omits the API's target field; Production explicitly sends `production`. Vercel uses its saved build settings and environment variables. Git/Me does not modify them or upload a build from your PC.

Reviews expire after five minutes and can be used once. Account/token, project Git integration and saved service/source changes invalidate the review. Read access does not prove deployment permission: Vercel may reject a connected token's write request.

## Outcomes

- **Request accepted:** Vercel returned a deployment ID. Check its build state; acceptance is not proof that it is live.
- **Request rejected:** Vercel rejected the request, such as insufficient permissions or a deployment limit.
- **Not submitted:** A review expired or source/account settings changed before submission.
- **Confirmation unavailable / Submission unconfirmed:** a request may have reached Vercel, or Git/Me stopped before recording its result. Check Vercel before reviewing another request. Git/Me never automatically resends it.

The request journal is separate from workspace edits and release records. It stores IDs, commit/target details and curated outcomes, never credentials or provider environment values. Restart the updated Go companion for the new endpoints and SQLite schema 3 migration.

API reference: [create a deployment](https://vercel.com/docs/rest-api/deployments/create-a-new-deployment).
