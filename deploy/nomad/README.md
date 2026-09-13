# deploy/nomad

`ramble.nomad.hcl` is the production deployment of https://ramble.openwander.org:
Ramble on a Nomad cluster, behind Traefik, on a Postgres reached over the
network. The release workflow runs it with `-var image=ghcr.io/open-wander/ramble:<version>`
and the datacenter; nothing is edited on any server.

| Where | What |
|---|---|
| Nomad variable `nomad/jobs/ramble` | `db_host`, `db_password`, `session_secret`, `token_encryption_key`, `github_key`, `github_secret`, `github_requests_token`, `gitlab_key`, `gitlab_secret`, `smtp_host`, `smtp_port`, `smtp_user`, `smtp_password`, `from_address` |
| Repo secrets `NOMAD_ADDR`, `NOMAD_TOKEN`, `NOMAD_DATACENTER` | how the runner reaches the cluster (set by hand) |

`AUTO_SEED` is pinned to `false` and the `INITIAL_USER_*` variables are not
passed: the database was migrated populated, and a seeding credential has no
place in a running job.

Manual deploy, from a machine that can reach the cluster:

    nomad job run -var image=ghcr.io/open-wander/ramble:<version> -var 'datacenters=["<dc>"]' deploy/nomad/ramble.nomad.hcl

The platform (Traefik, Postgres, backups, logs) is operated separately from
this repository. Self-hosting your own registry: `SELF-HOSTING.md`.
