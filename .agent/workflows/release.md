---
description: how to perform a push and release to Prerelease or main
---

# Release Workflow

Follow these steps to push changes and trigger a release.

## 1. Prerelease Branch (Development/Test Release)
Use this for testing changes. It generates a version tag like `v2.YYYYMMDD.N-Prerelease`.

// turbo
1. Stage and commit changes:
   ```powershell
   git add .
   git commit -m "feat/fix: descriptive message"
   ```

// turbo
2. Push to `Prerelease`:
   ```powershell
   git push origin Prerelease
   ```

// turbo
3. Trigger the release workflow:
   ```powershell
   gh workflow run release.yml --ref Prerelease
   ```

## 2. Main Branch (Production Release)
Use this for official releases. It generates a production tag like `v2.YYYYMMDD.N`.

// turbo
1. Merge `Prerelease` into `main`:
   ```powershell
   git checkout main
   git merge Prerelease
   ```

// turbo
2. Push to `main`:
   ```powershell
   git push origin main
   ```

// turbo
3. Trigger the release workflow on `main`:
   ```powershell
   gh workflow run release.yml --ref main
   ```

## 3. Monitoring
You can monitor the progress of the release using:
```powershell
gh run list --workflow release.yml --limit 5
```
