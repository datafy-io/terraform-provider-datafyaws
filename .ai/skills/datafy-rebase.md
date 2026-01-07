# Datafy Rebase

Rebase the current branch onto a hashicorp tag, resolving conflicts according to Datafy conventions.

## Argument
The tag to rebase onto (e.g. `v6.44.0`). Full usage: `datafy-rebase v6.44.0`

## Steps

### 1. Fetch tags
```
git fetch --tags hashicorp
```
Confirm the tag `$ARGUMENTS` exists locally after fetching.

### 2. Start the rebase
```
git rebase $ARGUMENTS
```

### 3. Conflict resolution rules

When conflicts occur, apply the following rules (these mirror what `make datafy-rebase` does):

**Files to always delete (accept deletion, do not keep our version):**
- `mkdocs.yml`
- `ROADMAP.md`
- `website/` (entire directory)
- `internal/generate/allowsubcats/`
- `internal/generate/checknames/`
- `internal/generate/customends/`

**`.github/workflows/`** — delete everything except:
- `datafy-pull-request.yml`
- `datafy-release.yml`

**`docs/`** — delete everything except:
- `index.md`

**`CODEOWNERS`** — always overwrite with:
```
* @datafy-io/saas
```

**Commits prefixed `datafy:`** — these are Datafy-specific commits on top of the upstream. When a `datafy:` commit conflicts:
- Prefer our (Datafy) version for files under `internal/datafy/`, `internal/service/ec2/` (Datafy-specific logic), and `internal/provider/`
- For any other conflicting file in a `datafy:` commit, use judgment: keep Datafy changes, discard upstream noise

### 4. After each conflict batch
```
git add <resolved files>
git rebase --continue
```

### 5. After the rebase completes

Run `make datafy-rebase` to enforce the final clean state:
```
make datafy-rebase
```

Do NOT create a new commit. Instead, fold every changed/deleted file back into the correct existing `Datafy:` commit using fixups:

**For each file that was modified or deleted by `make datafy-rebase` (or by conflict resolution):**

1. Find which `Datafy:` commit last touched that file:
   ```
   git log --oneline -- <file>
   ```
   Use the first (most recent) `Datafy:` commit in the result.

2. Stage the file and create a fixup commit targeting that commit's hash:
   ```
   git add <file>
   git commit --fixup=<hash>
   ```

3. Repeat for all remaining changed files, grouping by target commit where possible.

4. Once all fixup commits are created, squash them into their targets:
   ```
   git rebase -i --autosquash <base-tag>
   ```
   Where `<base-tag>` is `$ARGUMENTS` (the hashicorp tag rebased onto). This will automatically reorder and squash all `fixup!` commits into their respective `Datafy:` commits.

### 6. Verify
- Confirm the branch tip is based on `$ARGUMENTS`
- Confirm all `datafy:` commits are present above the new base
- Confirm `CODEOWNERS` contains `* @datafy-io/saas`
- Confirm only `datafy-pull-request.yml` and `datafy-release.yml` remain in `.github/workflows/`
- Confirm only `index.md` remains in `docs/`
