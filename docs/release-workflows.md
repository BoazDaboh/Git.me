# Release workflows

Open **Project > Deploy**, select the service, then choose Setup, Release or Activity. See [the Deploy workspace guide](deploy-workspace.md).

| Flow | Steps | Status |
| --- | --- | --- |
| Push through GitHub | Review changes > stage > commit > review push > confirm > Activity | Available |
| Existing GitHub commit | Choose recorded commit > choose Preview/Production > review exact source and target > confirm > Activity | Available, Stage 8e |
| Local upload | Choose target > review local source and exclusions > confirm upload/build > Activity | Available, Stage 8f |

## Setup

The selected service owns its GitHub repository, hosting connection and reference configuration. Setup offers GitHub account/repository controls using the same saved link shown in its local profile, alongside hosting account/project links and recorded deployment method/build/start commands. Deployment URLs come from Activity; environment values are managed with your host. Reference values do not update the provider's actual settings or secrets.

## Release

Saving a default flow changes only the service preference. It does not stage, commit, push or deploy. Choosing a different flow for this release does not require changing the saved default.

For GitHub, the local Git controls inspect the actual repository and current branch. Stage, unstage, commit and push each keep their own explicit review and confirmation. Your PC's Git Credential Manager authorizes pushes; the GitHub dashboard token is not reused. Vercel's Git configuration determines whether a push triggers a Preview, Production or no deployment. Git/Me does not override that through an environment selector.

For an existing commit, use a saved push record or Track current GitHub commit. Verification checks the local branch against GitHub without pushing. Select Preview or Production, then review and confirm the exact recorded SHA and saved Vercel account/team/project. Local changes are excluded. This sends one provider request and never automatically resends an uncertain submission.

## Activity

After a confirmed push or deployment response, Activity opens. The selected release is retained between the manual release flow and Activity. Current production, recent provider builds, exact-commit release tracking and durable manual request outcomes remain separate readings. Request acceptance is not build success, and Ready is not proof that a deployment serves the production domain.

Historical records keep their original account/team/project bindings. Changing a current service link never retargets old releases. Switches between services/areas/flows never execute actions; unfinished reviews are discarded when their controls unmount.

Local uploads use the selected service folder and saved Vercel link without requiring GitHub. A file review and separate confirmation precede any upload. Limits, conservative ignore rules and interrupted-upload handling are covered in [local uploads](local-upload.md).

Docker remains deferred. No automatic merge, force push, rollback, secret editing or provider settings synchronization is added by this layout.

Details: [Local Git](local-git.md), [push](git-push.md), [tracking](deployment-tracking.md), [manual deployment](manual-deployment.md).
