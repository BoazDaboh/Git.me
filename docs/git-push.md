# Stage 8c: push to GitHub

**Commit saves locally. Push uploads committed branch history to GitHub.**

## Use it

1. Save the service's repository URL and wait for Saved to SQLite. Its local `origin` must point to the same HTTPS GitHub repository. If a dashboard GitHub repository is linked, that must match too.
2. Open Deploy > choose the service > Release > Push through GitHub. In Local Git, click **Review push**. Git/Me reads the actual GitHub branch using your PC's Git sign-in.
3. Check the destination, current branch, local/remote commit, outgoing commits and changed file paths. The whole repository's committed history is involved, including other services. Uncommitted files are excluded.
4. Click **Confirm push**. It sends the reviewed commit to the same named GitHub branch. A new branch can be created; an existing branch must be a fast-forward update.
5. Read the result and use **View pushed commit**. Linked GitHub activity refreshes separately. Deploy opens Activity after an accepted push. Deployment tracking follows exact-commit matches in the service's linked Vercel project.

Each review expires after two minutes and is single-use. Cancel does not push. There are no automatic merges, force pushes, tag uploads, submodule pushes or automatic retries. The source branch and upstream settings are not changed.

## PC sign-in versus dashboard connection

The token connected in Git/Me Settings still provides read-only dashboard activity. It is never supplied to Git push. Push uses the Git Credential Manager bundled with Git for Windows and credentials saved by your PC's Git sign-in. That sign-in can differ from the dashboard account; it must have write access to the reviewed repository. GitHub branch protection and organisation rules still apply.

If Git/Me cannot authenticate, open a terminal in the project and run `git ls-remote origin` to complete Git Credential Manager's normal sign-in. This reads remote references without pushing. Then review again in Git/Me. Background requests never open a sign-in prompt. SSH, custom credential helpers and other Git hosts currently require the terminal.

## Terminal context

From the repository folder, the normal basic command is:

```powershell
git push origin your-branch
```

Git/Me's review contains a copyable PowerShell command with an explicit destination and exact commit ID. It does not rely on upstream defaults, suppresses pre-push hooks, signing, extra tags and submodule pushes, and does not rewrite history. Copying does not execute it. Running a copied command bypasses Git/Me's expiring review and stale-state checks.

If remote commits are missing locally, run `git fetch origin`, then review again. If histories have diverged, inspect and reconcile them in your terminal. Git/Me never pulls or merges automatically.

## Deployment and results

A push may cause a provider such as Vercel to deploy, including to production, according to that provider's branch rules. The saved intended environment does not change those rules. This step does not start a separate deployment or claim one succeeded.

After a successful Git response, Git/Me checks the GitHub branch again. If the follow-up fails or the branch moves, the result still reports the accepted push separately. If a request fails or times out, it may already have reached GitHub: review the branch again before retrying.

## Boundaries

Reviews check up to 200 outgoing commits and 1,000 changed paths. Private/generated paths are blocked throughout outgoing history, even if deleted later. This is a path check, not a complete secret-content scan. Large or special repositories require terminal handling.

Network operations use a disposable bare Git context with read access to local objects. It ignores source/global credential commands, URL rewrites, HTTP headers and SSH settings. Only HTTPS github.com is accepted; authenticated redirects are disabled and TLS verification stays enabled. The bundled Git Credential Manager is the only credential helper. Owned Windows jobs stop Git and helper processes together on cancellation. No dashboard tokens or raw Git network output appear in workspace exports, logs or errors.

Tests use temporary source/bare repositories and simulated network routing. They do not push real projects or access real credentials.

References: [Git push](https://git-scm.com/docs/git-push), [Git credentials](https://git-scm.com/docs/gitcredentials), [Git ls-remote](https://git-scm.com/docs/git-ls-remote).
