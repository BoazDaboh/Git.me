# Stage 8f: local files to Vercel

Use this when you want to deploy current files without committing or pushing to GitHub.

1. Save the service's local folder in Frontend/Backend. It must be inside the saved project folder.
2. Open **Deploy > Setup** and link the Vercel account/team/project.
3. Open **Release > Local upload**. Choose Preview or Production.
4. Click **Review local files**. Check the destination, included files and exclusions. This step uploads nothing.
5. Check the acknowledgement and confirm. The companion uploads the reviewed source and requests one build.
6. **Activity > Deployment requests** shows the source fingerprint, file count, returned deployment ID and current build state. Refresh requests checks status again; history also refreshes while visible every minute.

GitHub is optional for this flow. Git/Me does not stage, commit or push. Uncommitted files can be included. Vercel runs the build with its saved settings and environment variables; the reference commands in Git/Me do not override them.

## File rules

- Only the selected service folder is uploaded. Parent folders and sibling services are not collected.
- `.env*`, credential files, database files, Git metadata, dependencies and generated output are excluded. Links and special files are not followed; Windows hard-linked files are rejected.
- Common embedded credential patterns are excluded too. These checks cannot identify every secret, so inspect the included file list and keep sensitive data in excluded environment files.
- Service-level and nested `.gitignore` / `.vercelignore` files add exclusions together. `!` reinclusions cannot undo exclusions. Escaped or malformed patterns stop the review rather than being silently ignored. Parent ignore files outside the service are not read; repeat any required rules in the service's ignore file.
- Limits: 2,000 included files, 32 MiB total, 8 MiB per file. Oversized source stops the review. Files are sent sequentially within a bounded request; a slow upload may time out.
- The Vercel project's Root Directory must be empty or `.` so it matches the service folder. Use the GitHub flow for nested-root projects for now.

The review expires after five minutes and is single-use. Changed files, folders, deployment links or account credentials require another review. Nothing runs locally, and no provider project settings or environment values are written.

## Interrupted uploads

Uploading source and creating a deployment are separate steps. A stopped upload may leave some source files stored on Vercel without creating a deployment. An uncertain deployment response may mean Vercel accepted the build. Git/Me records these outcomes in SQLite and never retries automatically. Check Activity and Vercel before trying again.

Accepted means a deployment ID was returned. READY means its build completed; **Current production** appears only when Vercel's production target points to that exact ID. Old requests retain their original account/team/project and are never retargeted after a link change.

File contents stay out of browser reviews, workspace exports and the request journal. Only the confirmed file bytes go to Vercel. Tokens stay in Windows credential storage. No database schema change is needed beyond the existing schema 3 journal.

References: [upload files](https://vercel.com/docs/rest-api/deployments/upload-deployment-files), [create a deployment](https://vercel.com/docs/rest-api/deployments/create-a-new-deployment).
