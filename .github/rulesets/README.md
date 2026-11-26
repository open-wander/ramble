# Repository rulesets

GitHub does not apply these files automatically. They are the source of truth
for the repository's rulesets; import or update them by hand.

Import: Settings > Rules > Rulesets > New ruleset > Import a ruleset, then
choose the JSON file. To change a ruleset, edit the file in a pull request and
re-import it (or edit the ruleset in the UI and export it over the file).

| File | Applies to | Rules |
|------|------------|-------|
| `main.json` | default branch | no deletion, no force push, pull request required (no approval needed, squash merge only); `DCO`, `test` and `security-scan` checks required; repository admins may bypass |

Each required check is pinned to the app that reports it (`integration_id`:
1861 is the DCO app, 15368 is GitHub Actions), so a status with the same name
from another integration does not satisfy it. A renamed CI job must be renamed
here too, or every pull request waits for a check that never reports.
