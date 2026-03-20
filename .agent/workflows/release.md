---
description: how to perform a push and release to Prerelease or main
---

# Release Workflow

Follow these steps to push changes and trigger a release.



## 1. Development (devwork) & Prerelease
Use this for regular development and staging changes for test releases.

// turbo
1. Push to `devwork` and merge to `Prerelease`:
   ```powershell
   git add . && git commit -m "feat/fix: descriptive message" && git push origin devwork && git checkout Prerelease && git merge devwork && git push origin Prerelease && git checkout devwork
   ```

// turbo
2. Trigger the release workflow (optional):
   ```powershell
   gh workflow run release.yml --ref Prerelease
   ```



## 2. Main Branch (Production Release)
Use this for official releases. It generates a production tag like `v2.YYYYMMDD.N`.

// turbo
1. Merge `Prerelease` into `main`:
   ```powershell
   git checkout main && git merge Prerelease
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

## 4. Post-Release
Always return to `devwork` for continued development:

// turbo
1. Switch back to `devwork`:
   ```powershell
   git checkout devwork
   ```
