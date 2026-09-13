# Ramble on a Nomad cluster: fronted by Traefik (Let's Encrypt), Postgres
# reached over the network, secrets and the database host from a Nomad
# variable. The public ramble.openwander.org runs from this file.
#
# A release runs .github/workflows/release.yml: it builds the image on the
# self-hosted runner, pushes ghcr.io/open-wander/ramble:<version> and runs
# this spec with -var image=<that> and the datacenter. Never :latest.
#
# Secrets: Nomad variable nomad/jobs/ramble. AUTO_SEED and INITIAL_USER_* are
# deliberately absent: the database arrives populated, and a seeding
# credential has no business in a running job.

variable "image" {
  description = "App image. Set by the deploy job to the tag being released."
  type        = string
  default     = "ghcr.io/open-wander/ramble:0.5.11"
}

variable "datacenters" {
  type = list(string)
}

job "ramble" {
  datacenters = var.datacenters
  type        = "service"
  node_pool   = "default"

  group "ramble" {
    count = 1

    network {
      port "http" { to = 3000 }
    }

    shutdown_delay = "5s"

    service {
      name     = "ramble"
      port     = "http"
      provider = "nomad"
      tags = [
        "traefik.enable=true",
        "traefik.http.routers.ramble.rule=Host(`ramble.openwander.org`)",
        "traefik.http.routers.ramble.entrypoints=websecure",
        "traefik.http.routers.ramble.tls.certresolver=letsencrypt",
        "traefik.http.routers.ramble.middlewares=block-exploits@file",
        # The old hostname, redirected permanently - as the Compose labels did.
        "traefik.http.routers.rmbl-redirect.rule=Host(`rmbl.openwander.org`)",
        "traefik.http.routers.rmbl-redirect.entrypoints=websecure",
        "traefik.http.routers.rmbl-redirect.tls.certresolver=letsencrypt",
        "traefik.http.middlewares.rmbl-to-ramble.redirectregex.regex=^https://rmbl\\.openwander\\.org/(.*)",
        "traefik.http.middlewares.rmbl-to-ramble.redirectregex.replacement=https://ramble.openwander.org/$${1}",
        "traefik.http.middlewares.rmbl-to-ramble.redirectregex.permanent=true",
        "traefik.http.routers.rmbl-redirect.middlewares=rmbl-to-ramble",
      ]

      # TCP, not HTTP: in production Ramble answers a plain-HTTP GET with a
      # 301 to HTTPS (correct behind Traefik, a failure to Nomad's probe),
      # and it has no unauthenticated health path that skips the redirect.
      check {
        type     = "tcp"
        port     = "http"
        interval = "15s"
        timeout  = "3s"
      }
    }

    task "ramble" {
      driver = "docker"

      template {
        destination = "secrets/ramble.env"
        env         = true
        change_mode = "restart"
        data        = <<-EOT
          {{ with nomadVar "nomad/jobs/ramble" }}
          ENV=production
          SENTRY_ENVIRONMENT=production
          {{ if .sentry_dsn }}SENTRY_DSN={{ .sentry_dsn }}{{ end }}
          BASE_URL=https://ramble.openwander.org
          # The reverse proxy's source range - the only peers allowed to set X-Forwarded-For.
          TRUSTED_PROXIES=127.0.0.1/8,100.64.0.0/10
          DATABASE_URL=host={{ .db_host }} user=ramble password={{ .db_password }} dbname=rambledb port=5432 sslmode=disable
          SESSION_SECRET={{ .session_secret }}
          TOKEN_ENCRYPTION_KEY={{ .token_encryption_key }}
          AUTO_SEED=false
          GITHUB_KEY={{ .github_key }}
          GITHUB_SECRET={{ .github_secret }}
          GITHUB_REQUESTS_TOKEN={{ .github_requests_token }}
          GITLAB_KEY={{ .gitlab_key }}
          GITLAB_SECRET={{ .gitlab_secret }}
          SMTP_HOST={{ .smtp_host }}
          SMTP_PORT={{ .smtp_port }}
          SMTP_USER={{ .smtp_user }}
          SMTP_PASSWORD={{ .smtp_password }}
          FROM_ADDRESS={{ .from_address }}
          {{ end }}
        EOT
      }

      config {
        image = var.image
        ports = ["http"]
      }

      kill_timeout = "15s"

      resources {
        cpu    = 200
        memory = 256 # 14 MB in use on the mail box; the Compose limit was 512
      }
    }
  }
}
