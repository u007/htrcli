# CI/CD Configuration

This document describes the CI/CD setup for the HTR NControl Chrome extension.

## Workflows

### 1. PR Checks (`.github/workflows/pr-checks.yml`)

Runs automatically on all pull requests and pushes to `main`:

- TypeScript type checking
- Biome linting and formatting checks
- Unit tests
- Build verification

### 2. Release (`.github/workflows/release.yml`)

Runs automatically when PRs are merged to `main`:

- Runs all checks (lint, test, build)
- Generates changelog from commit messages since last tag
- Creates signed `.crx` package for Chrome extension (using `CRX_PRIVATE_KEY`)
- Creates GitHub release with:
  - Auto-generated changelog
  - Signed `.crx` file for manual installation
  - `.zip` file for archival
- Uploads signed `.crx` to Chrome Web Store API
- Publishes to Chrome Web Store automatically

**Security**: The workflow uses signed CRX files with verified uploads, protecting against unauthorized package updates even if credentials are compromised.

## Required GitHub Secrets

To enable the full CI/CD pipeline, you need to configure the following secrets in your GitHub repository settings (`Settings > Secrets and variables > Actions`):

### CRX Signing

1. **`CRX_PRIVATE_KEY`**
   - Private key for signing the `.crx` extension package
   - Must be consistent across builds to maintain the same extension ID
   - Generate once and store securely in GitHub secrets
   
   **Generate the key:**
   ```bash
   openssl genrsa 2048 | openssl pkcs8 -topk8 -nocrypt
   ```
   
   Copy the entire output (including `-----BEGIN PRIVATE KEY-----` and `-----END PRIVATE KEY-----`) and save it as the `CRX_PRIVATE_KEY` secret.

### Chrome Web Store Publishing

2. **`CHROME_EXTENSION_ID`**
   - Your extension's ID from the Chrome Web Store Developer Dashboard
   - Format: 32-character string (e.g., `abcdefghijklmnopqrstuvwxyz123456`)

3. **`CHROME_CLIENT_ID`**
   - OAuth 2.0 Client ID for Chrome Web Store API
   - Get from [Google Cloud Console](https://console.cloud.google.com/)

4. **`CHROME_CLIENT_SECRET`**
   - OAuth 2.0 Client Secret
   - Get from Google Cloud Console (same place as Client ID)

5. **`CHROME_REFRESH_TOKEN`**
   - OAuth 2.0 Refresh Token for automated publishing
   - Generate using the Chrome Web Store API setup guide

### How to Get Chrome Web Store API Credentials

1. **Create a Google Cloud Project**
   - Go to [Google Cloud Console](https://console.cloud.google.com/)
   - Create a new project or select an existing one

2. **Enable Chrome Web Store API**
   - In the project, go to "APIs & Services" > "Library"
   - Search for "Chrome Web Store API"
   - Click "Enable"

3. **Create OAuth 2.0 Credentials**
   - Go to "APIs & Services" > "Credentials"
   - Click "Create Credentials" > "OAuth client ID"
   - Choose "Desktop app" as application type
   - Name it (e.g., "CI/CD Publishing")
   - Save the Client ID and Client Secret

4. **Generate Refresh Token**
   - Use this URL (replace `YOUR_CLIENT_ID`):
     ```
     https://accounts.google.com/o/oauth2/auth?response_type=code&scope=https://www.googleapis.com/auth/chromewebstore&client_id=YOUR_CLIENT_ID&redirect_uri=urn:ietf:wg:oauth:2.0:oob
     ```
   - Authorize and copy the authorization code
   - Exchange for refresh token using curl:
     ```bash
     curl "https://accounts.google.com/o/oauth2/token" \
       -d "client_id=YOUR_CLIENT_ID" \
       -d "client_secret=YOUR_CLIENT_SECRET" \
       -d "code=YOUR_AUTH_CODE" \
       -d "grant_type=authorization_code" \
       -d "redirect_uri=urn:ietf:wg:oauth:2.0:oob"
     ```
   - Copy the `refresh_token` from the response

5. **Get Extension ID**
   - Go to [Chrome Web Store Developer Dashboard](https://chrome.google.com/webstore/devconsole)
   - Find your extension
   - The ID is in the URL or extension details

6. **Enable Verified CRX Uploads (Recommended)**
   
   This adds an extra layer of security by requiring all package updates to be signed with your private key, preventing unauthorized updates even if your developer account is compromised.
   
   - Extract your public key:
     ```bash
     ./scripts/extract-public-key.sh
     ```
   - Go to [Chrome Web Store Developer Dashboard](https://chrome.google.com/webstore/devconsole)
   - Select your extension
   - Navigate to the **Package** tab
   - Find the **Verified CRX Uploads** section
   - Click **Opt In**
   - Paste your public key (including BEGIN/END lines)
   
   **Important**: After opting in, all future uploads MUST be signed CRX files. The CI/CD pipeline handles this automatically using the `CRX_PRIVATE_KEY` secret.

## Versioning

The CI/CD pipeline uses the version from `package.json` to:
- Tag releases (e.g., `v0.1.0`)
- Name release files
- Prevent duplicate releases

To create a new release:
1. Bump the version in `package.json`
2. Commit and push (or merge PR) to `main`
3. The workflow will automatically create the release

## Changelog Generation

Changelogs are auto-generated from commit messages between the last tag and current commit.

**Recommended commit message format:**
```
feat: Add new feature
fix: Fix bug in component
docs: Update README
chore: Update dependencies
```

This keeps changelogs clean and informative.

## Manual Override

If you need to skip auto-publishing to Chrome Web Store:
1. Comment out the "Upload to Chrome Web Store" step in `.github/workflows/release.yml`
2. Manually upload the `.zip` file from GitHub releases

## Testing the Workflow

1. Create a test branch
2. Make changes and commit
3. Open a PR to `main` - PR checks will run
4. Merge the PR - Release workflow will run
5. Check the "Actions" tab in GitHub to monitor progress

## Troubleshooting

- **Release skipped**: Check if the version tag already exists. Bump version in `package.json`.
- **Chrome Web Store upload fails**: Verify all four secrets are correctly set and the API is enabled.
- **Build fails**: Run `bun run build` locally to debug.
- **Changelog empty**: Ensure commits exist since last tag.
