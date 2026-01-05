job "ramble" {
  datacenters = ["tpi-dc1"]
  type        = "service"

  group "ramble" {
    count = 1

    network {
      port "http" {
        to = 3000
      }
    }

    service {
      name     = "ramble"
      port     = "http"
      provider = "nomad"
      tags = [
        "traefik.enable=true",
        "traefik.http.routers.ramble.rule=Host(`ramble.openwander.org`)",
      ]

      check {
        type     = "http"
        path     = "/"
        interval = "10s"
        timeout  = "2s"
      }
    }

    task "server" {
      driver = "docker"

      config {
        image = "ghcr.io/open-wander/ramble:latest"
        ports = ["http"]
      }

      resources {
        cpu    = 200
        memory = 256
      }

      template {
        data = <<EOH
{{ with nomadVar "nomad/jobs/ramble" }}
ENV="production"
DB_HOST="{{ .db_host }}"
DB_USER="{{ .db_user }}"
DB_PASSWORD="{{ .db_password }}"
DB_NAME="{{ .db_name }}"
DB_PORT="{{ .db_port }}"
SESSION_SECRET="{{ .session_secret }}"
AUTO_SEED="{{ .auto_seed }}"
INITIAL_USER_USERNAME="{{ .initial_user_username }}"
INITIAL_USER_EMAIL="{{ .initial_user_email }}"
INITIAL_USER_PASSWORD="{{ .initial_user_password }}"
SMTP_HOST="{{ .smtp_host }}"
SMTP_PORT="{{ .smtp_port }}"
SMTP_USER="{{ .smtp_user }}"
SMTP_PASSWORD="{{ .smtp_password }}"
FROM_ADDRESS="{{ .from_address }}"
GITHUB_KEY="{{ .github_key }}"
GITHUB_SECRET="{{ .github_secret }}"
GITLAB_KEY="{{ .gitlab_key }}"
GITLAB_SECRET="{{ .gitlab_secret }}"
BASE_URL="{{ .base_url }}"
{{ end }}
EOH
        destination = "local/env.env"
        env         = true
      }
    }
  }
}
