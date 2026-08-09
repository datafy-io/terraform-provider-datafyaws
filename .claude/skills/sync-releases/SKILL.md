---
name: sync-releases
description: Manage the three-release workflow for this repo - create aligned DT-ticket feature branches off release/4.x, 5.x and 6.x, port (sync) commits from the currently checked-out ticket branch to its two sibling release branches, and push all three. Use when the user says "sync", "sync releases", "sync the branches", "port to the other branches", "push" (push all three ticket branches), "sync push" (sync then push), or wants to start ticket work across all release lines.
---

# sync-releases

One entry point for working a change across `release/4.x`, `release/5.x`, and `release/6.x`: detect or create the ticket's three feature branches, then port new commits from the **current branch** to the other two. Stateless — everything is derived from branch names; never store state in files.

Branch naming convention: `DT-<number>/release<4|5|6>`, or with an optional title `DT-<number>_<title>/release<4|5|6>` — e.g. `DT-11355_autoscaling-native-fixups/release5`. The **currently checked-out ticket branch is the source**; its two siblings are the port targets — the user may be working on any of the three (release4, release5, or release6). If it's unclear which branch is the source, ask the user to confirm. Each branch's base is `origin/release/<X>.x`, where `<X>` is the number after the `/release` segment.

## Flow

1. Run `git fetch origin release/4.x release/5.x release/6.x` and `git branch --show-current`.
2. **Current branch matches** `^DT-[0-9]+(_[A-Za-z0-9-]+)?/release[456]$`:
   - Derive the two sibling names by swapping the `release<N>` segment after the `/` (keep the `DT-<number>[_<title>]` prefix identical). Check each with `git branch --list '<sibling-name>'`.
   - Create any missing sibling automatically from its base **without touching the checkout**: `git branch --no-track 'DT-<n>[_<title>]/release<X>' origin/release/<X>.x` (`--no-track` is required — otherwise the branch tracks the release branch and `git push` targets it instead of creating a new remote branch). Tell the user which branches were created, then go to **Sync**.
3. **Current branch does not match** the convention:
   - Ask for the Jira ticket number **as a plain text message and end the turn** — never via AskUserQuestion (free-form values can't be typed into its options). Validate the reply matches `DT-<number>`; re-ask if not. In the same message, also ask for the optional feature title (kebab-case, e.g. `autoscaling-native-fixups`) so one reply can answer both — e.g. "reply like: `DT-12345 my-feature-title` or just `DT-12345`".
   - AskUserQuestion is fine only for fixed-choice prompts (like picking a release line) — never for anything the user must type.
   - Create all three branches: `git branch --no-track 'DT-<number>[_<title>]/release<X>' origin/release/<X>.x` for X in 4, 5, 6 (never let them track the release branches — see step 2).
   - Decide which of the three the user will work on: if the current branch is `release/<X>.x`, that release line is the working one; otherwise ask (fixed choice: release4 / release5 / release6). Switch the checkout to that ticket branch (`git switch`). If the working tree has uncommitted changes, ask before switching.
   - Nothing to sync yet; tell the user to work normally and say "sync" when ready.

## Sync (port current-branch commits to the two sibling branches)

**Run fully autonomously.** Once the user asks to sync, drive the whole procedure end-to-end — branch creation, worktrees, cherry-picks, conflict resolution, adaptations, compile checks, commits, cleanup — without asking for confirmation at any step. The only reasons to stop and ask: uncommitted ticket-relevant changes (tell the user to commit), an ambiguous source branch, or a conflict you genuinely cannot resolve from the adaptation notes (report what you tried). Never ask "should I proceed?".

Source of truth: commits on the current branch, `git log --oneline origin/release/<X>.x..<current-branch>` (X = the source branch's release number). Uncommitted working-tree changes are NOT ported — if `git status` shows modified files relevant to the ticket, tell the user to commit first and stop.

**Detecting what's already ported (no state file):** ported commits must keep the **same commit subject** as their source commit — cherry-picks do this automatically; hand-ports must reuse the subject verbatim. For each target, compare subjects of `git log --format=%s origin/release/<X>.x..<source-branch>` against `git log --format=%s origin/release/<Y>.x..<target-branch>` (Y = target's release number); port the missing ones oldest-first.

**Never touch the user's checkout.** Do all target-branch work in worktrees under the session scratchpad dir:

```bash
git worktree add <scratchpad>/port-<Y>x 'DT-<n>[_<title>]/release<Y>'
```

For each target: cherry-pick each missing commit (`git cherry-pick <sha>`). Between adjacent generations (5⇔6) it usually applies clean or with small conflicts; any port involving release4 usually conflicts heavily — hand-apply the same logical change instead, then commit reusing the source commit's subject. Apply the adaptation notes below **in the direction of the port** (they're written as 5.x-era ⇔ 4.x-era / 6.x-era equivalences; invert when porting out of 4.x or 6.x).

### Crossing into/out of release6

- Helper signatures: 6.x tests use `testAccCheckVolumeExists(ctx, t, ...)` and `testAccCheckVolumeDestroy(ctx, t)` (extra `*testing.T`); 4.x/5.x take no `t`. Adapt every ported call site.
- Provider-level schema: on 4.x/5.x it lives in `internal/provider/provider.go` only; on 6.x that file doesn't exist — the schema is duplicated in `internal/provider/sdkv2/provider.go` (SDKv2 style) **and** `internal/provider/framework/provider.go` (plugin-framework style), and the two must stay identical (the mux server rejects mismatched provider schemas). A provider-schema change ported to 6.x goes into both files; ported out of 6.x it collapses into the one file.
- Functions live at different positions in 6.x files (e.g. `createNative` sits earlier in `ebs_volume_datafy_test.go`), so a commit that merely *moved* code shows up as add+delete hunks — resolve by keeping one copy, never duplicating a function.
- Branch-specific features (e.g. the `restored-from-snapshot` tag handling / `addRestoredFromSnapshotTag` exists on 5.x but not 6.x). If a hunk exists only because the source commit moved such code, drop it — do not introduce the feature. Flag it in the final report.
- After resolving a conflicted cherry-pick: `git add -A && GIT_EDITOR=true git cherry-pick --continue` (keeps the source subject).

### Crossing into/out of release4

| 5.x/6.x era | release4 equivalent |
|---|---|
| `awstypes "…aws-sdk-go-v2/service/ec2/types"`, `awstypes.Volume` | v1 SDK: `"…aws-sdk-go/service/ec2"`, `ec2.Volume`, `[]*ec2.Volume` |
| `awstypes.VolumeTypeGp3`, `names.AttrType` etc. | `ec2.VolumeTypeGp3`, literal strings (`"type"`, `"arn"`) |
| `aws.ToString` / `aws.Int32` | `aws.StringValue` / `aws.Int64` (v1 aws pkg) |
| `meta.(*conns.AWSClient).EC2Client(ctx)` / `DatafyClient(ctx)` | `EC2Conn()` (v1 client) / `DatafyClient()` — no ctx |
| `regexache.MustCompile` | `regexp.MustCompile` |
| `names.EC2ServiceID` in ErrorCheck | `ec2.EndpointsID` |
| `terraform-plugin-testing` imports (`plancheck`, `statecheck`, `helper/resource`) | plugin-sdk's `helper/resource`; no plan/state checks exist |
| `ConfigPlanChecks` + `plancheck.ExpectResourceAction(..., ResourceActionNoop)` | `PlanOnly: true` on the step (fails if the plan is non-empty) |
| `resourceEBSVolume` (unexported) | `ResourceEBSVolume` (exported) |
| `slices.ContainsFunc` (stdlib) | `internal/slices` helpers (`slices.Any`, `slices.ApplyToAll`) |
| mock `datafy.Volume.Volume` takes v2 `*awstypes.Volume` directly | build v2 `*types.Volume` from v1 fields (see `createDatafyReplacedByVolume` in 4.x for the conversion pattern) |
| test config sizes: 5.x conventions (e.g. size 100) | 4.x tests use `size = 1` — keep the target branch's convention |

`ResourceDiff.GetRawConfig`/`GetRawState` and `GetOk` exist on all three branches' plugin-sdk versions — no adaptation needed for CustomizeDiff raw-value logic.

### Verify, commit, clean up (all targets)

- Compile-check inside each worktree: `go vet ./<changed-pkg>/` and `go test -run xxx_nonexistent ./<changed-pkg>/` for every package the port touched. First builds of a branch take minutes — run in background if slow. Fix errors and amend the ported commit.
- Watch for source-branch bugs surfaced by porting (e.g. an `ExpectError` regex that no longer matches a reworded error). Fix on the target AND on the source working tree, and tell the user to commit the source-branch fix.
- Never push as part of sync itself — pushing happens only via the **Push** command below. Never run acceptance tests (they need AWS credentials) — tell the user the per-branch command instead.
- Remove worktrees when done: `git worktree remove --force <dir>` (builds dirty `tools/tfsdk2fw/go.mod` — safe to discard).
- Report per branch: commits ported, adaptations made, hunks dropped (missing features), and anything needing the user's attention.

## Push ("push" / "sync push")

- **"push"**: push all three ticket branches to origin under their own names: `git push -u origin '<branch>'` for each (`-u` creates the remote branch on first push and sets the upstream; safe because ticket branches are created with `--no-track` and never track the release branches). Report the three pushed refs. If a push is rejected (remote diverged, e.g. after a local amend of an already-pushed commit), stop and report — never force-push without the user explicitly asking.
- **"sync push"**: run the full **Sync** first; if it completes cleanly (or reports nothing to port), push all three as above. If sync stops (uncommitted changes, unresolvable conflict), do not push anything.
- Run autonomously like sync — no confirmation prompts.
