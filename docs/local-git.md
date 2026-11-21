# Stage 8b: local Git

Open Projects > your project > Deploy > choose a service > Release > Push through GitHub. The Local Git controls appear under that flow.

## Normal workflow

1. Click Refresh Git status. The panel shows the actual branch, HEAD commit and changed files in that service.
2. Use Working diff to inspect unstaged changes or Staged diff to inspect the next commit's content.
3. Select files, click Review stage, inspect the review and confirm Stage selected files.
4. To remove files from staging, select them, click Review unstage and confirm. Working files are preserved.
5. Write a commit message, click Review commit, check the branch, author, full staged-file list and diff, then confirm Commit staged changes.

The new commit exists locally. Use the separate **Review push** control for Stage 8c, then confirm before uploading to GitHub. See [the push guide](git-push.md).

Checking a file's checkbox only selects it for an operation. Confirming the staging review changes Git's staging area. A commit records the entire staged snapshot, including eligible files staged from your terminal; it does not automatically include later working-file edits.

In Deploy > Release, Push through GitHub shows Local Git. Choosing a flow or saving its default does not execute Git actions.

## Terminal compatibility

Expand **Terminal commands · PowerShell** in Local Git for copyable commands using this repository, the selected files and your commit message. Each confirmation review also shows its terminal equivalent. Copy only copies text; it does not run Git.

From a repository folder, the basic actions are:

```powershell
git status
git add -- 'src/app/page.tsx'
git restore --staged -- 'src/app/page.tsx'
git commit -m 'feat: update homepage'
```

`add` stages the current changes, `restore --staged` unstages them without deleting edits, and `commit` records all staged changes locally. Before the first commit, use `git rm --cached --force -- 'file'` to unstage while preserving the working file. Copied commands include `-C` to choose the repository and `--literal-pathspecs` for exact file names. They are quoted for PowerShell.

Terminal commands act on files and staging at the time you run them; Git/Me uses the confirmed review. A terminal commit can include staged files outside this service and runs your configured hooks/signing. Refresh Git status after terminal actions.

Git/Me uses the saved service folder and finds its repository inside the saved project boundary. It does not need a GitHub token to stage or commit. The author comes from local Git configuration, independently of the cloud account connected in Settings.

You can continue using Git Bash, PowerShell or VS Code. Refresh Git status after terminal changes. Reviews bind to the repository, service, branch, HEAD, index/configuration and selected content. Relevant changes invalidate the review. Reviews expire after two minutes and can only be executed once; Cancel releases the review.

For a service inside a monorepo, the file list stays inside its folder. If another service has staged changes, Git/Me blocks a repository-wide commit from this service panel. Manage that combined commit from the repository root in your terminal. Individual staging/unstaging still preserves other services' index entries.

## First-version limits

- Stage or unstage up to 24 paths per operation. Review diffs show up to 64 KiB, with an explicit truncation notice.
- Staging accepts regular files up to 2 MiB each and 8 MiB total per review. Large repositories/indexes or more than 500 changed paths require terminal handling.
- Private environment files, keys, credential/data files and generated directories are not previewed or staged through this panel. `.env.example` and `.env.sample` are allowed.
- Git filters, including Git LFS, symlinks, submodules, linked worktrees, shared object stores and merge conflicts require terminal handling.
- Detached HEAD is read-only here. Branch switching, initial repository creation, amend, force operations and history rewriting are not provided.
- Git hooks, fsmonitor helpers, external diff/text-conversion helpers, clean filters and commit signing are not run by these controls. If your project requires hooks or signed commits, commit from your terminal instead. Git/Me does not change your global Git settings.

Commits use the reviewed index through Git's tree/commit/ref operations. A compare-and-swap ref update prevents overwriting a changed branch. Staging and unstaging use a separate temporary index and acquire Git's real index lock only when applying the reviewed result. Working files are not overwritten. A failed ref/index check leaves existing working content intact.

Review content stays in companion/browser memory. No diffs, commit messages or Git review tokens are saved in SQLite workspace records or JSON exports. Temporary Git index files are removed after each operation. Commit objects and history are normal repository data.

If a request stops or times out, refresh Git status before retrying. A commit may have completed before a later status refresh failed; the interface does not automatically retry writes. An existing index lock is treated as another Git process being busy, never deleted by Git/Me.

## Validation

Tests use temporary repositories and a temporary SQLite workspace. They cover selected staging, unstage preservation, staged-only commits, stale/replayed reviews, private-file blocking, service boundaries, hook/filter suppression and browser/mobile behavior. No tests commit, stage or push the user's real projects.

References: [Git status](https://git-scm.com/docs/git-status), [Git diff](https://git-scm.com/docs/git-diff), [Git update-index](https://git-scm.com/docs/git-update-index), [Git commit-tree](https://git-scm.com/docs/git-commit-tree), [Git update-ref](https://git-scm.com/docs/git-update-ref).
