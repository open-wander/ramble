# Repository rulesets

GitHub does not apply these files automatically. They are the source of truth
for the repository's rulesets; import or update them by hand.

Import: Settings > Rules > Rulesets > New ruleset > Import a ruleset, then
choose the JSON file. To change a ruleset, edit the file in a pull request and
re-import it (or edit the ruleset in the UI and export it over the file).

| File | Applies to | Rules |
|------|------------|-------|
| `main.json` | default branch | no deletion, no force push, pull request required (no approval needed, squash merge only); `DCO`, `test` and `security-scan` checks required; repository admins may bypass |
| `tags.json` | `v*` tags | no deletion, no update (a release tag cannot be moved); repository admins may bypass. Creation is not restricted: the release workflow creates the tag with `GITHUB_TOKEN`, which cannot be added to a bypass list. The `release` environment approval gates that job, and the provider's regenerate workflow only uses a tag whose commit is on `main` |

Each required check is pinned to the app that reports it (`integration_id`:
1861 is the DCO app, 15368 is GitHub Actions), so a status with the same name
from another integration does not satisfy it. A renamed CI job must be renamed
here too, or every pull request waits for a check that never reports.

## Environments

Settings > Environments. These are not exported as files.

| Environment | Used by | Protection | Secrets |
|-------------|---------|------------|---------|
| `release` | `release.yml`: bump-version, deploy, release, notify-provider | required reviewers (admins); deployment branches: `main` only | `NOMAD_ADDR`, `NOMAD_TOKEN`, `NOMAD_DATACENTER`, `HOMEBREW_TAP_TOKEN`, `RELEASE_APP_PRIVATE_KEY` (move them out of repository secrets) |

The self-hosted runner belongs in a runner group that allows only
`open-wander/ramble/.github/workflows/release.yml@refs/heads/main`, so a pull
request cannot schedule a job on it. Under Settings > Actions > General, set
fork pull request workflows to require approval for all outside contributors.
