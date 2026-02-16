---
name: git
description: Git operations and workflows for version control.
emoji: 🔧
metadata: { "requires": { "bins": ["git"] } }
---

# Git Skill

Common git operations and workflows.

## Quick Status

```bash
git status
git log --oneline -10
git branch -a
```

## Commit Changes

```bash
git add .
git commit -m "descriptive message"
```

## Create and Switch Branches

```bash
git checkout -b feature/new-feature
git switch main
git branch -d feature/old-feature
```

## Sync with Remote

```bash
git pull --rebase origin main
git push origin feature/branch
```

## View Diffs

```bash
git diff
git diff --staged
git diff HEAD~1
```

## Undo Changes

```bash
git checkout -- <file>        # Discard working changes
git reset HEAD <file>         # Unstage
git reset --soft HEAD~1       # Undo last commit, keep changes
git reset --hard HEAD~1       # Undo last commit, discard changes
```

## Stash Changes

```bash
git stash
git stash pop
git stash list
```

## Interactive Rebase

```bash
git rebase -i HEAD~3
```

## Merge vs Rebase

- Use `merge` for feature branches into main
- Use `rebase` for local cleanup before push
