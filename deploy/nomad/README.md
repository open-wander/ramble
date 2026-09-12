# deploy/nomad

`ramble.nomad.hcl` is the production deployment: Ramble on the Hetzner Nomad
cluster (`nomad-prod`), behind the platform Traefik, on the platform Postgres.
The release workflow runs it with `-var image=ghcr.io/open-wander/ramble:<version>`;
nothing is edited on any server.

| Where | What |
|---|---|
| Nomad variable `nomad/jobs/ramble` | `db_password`, `session_secret`, `token_encryption_key`, `github_key`, `github_secret`, `github_requests_token`, `gitlab_key`, `gitlab_secret`, `smtp_host`, `smtp_port`, `smtp_user`, `smtp_password`, `from_address` |
| 1Password `Infra / nomad-prod-ramble-env` | the same values, source of truth |
| 1Password `Infra / nomad-prod-postgres-ramble` | the database role |
| Repo secrets `NOMAD_ADDR`, `NOMAD_TOKEN` | how the runner reaches the cluster (set by hand) |

`AUTO_SEED` is pinned to `false` and the `INITIAL_USER_*` variables are not
passed: the database was migrated populated, and a seeding credential has no
place in a running job.

Manual deploy from a tailnet device:

    export NOMAD_ADDR=http://nomad-prod:4646 NOMAD_TOKEN=$(op read "op://Infra/nomad-prod-operator-token/credential")
    nomad job run -var image=ghcr.io/open-wander/ramble:0.5.11 deploy/nomad/ramble.nomad.hcl

Platform side (Traefik, Postgres, backups, logs): `TydeWhatMay/infra` `platform/`.
