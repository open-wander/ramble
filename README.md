# RMBL - Nomad Job & Pack Registry

A modernized registry for HashiCorp Nomad job files and Nomad Packs, built with Go, Fiber, and HTMX.

## Features

- **Modern UI:** Responsive design using Tailwind CSS.
- **Interactive Search:** Live search-as-you-type powered by HTMX.
- **Fast Navigation:** SPA-like experience with server-side rendering.
- **Job/Pack Registry:** Easily discover and share Nomad specifications.
- **Authentication:** User accounts and session-based auth.

## Tech Stack

- **Backend:** Go 1.23, Fiber v2
- **Database:** PostgreSQL (GORM)
- **Frontend:** HTMX, Hyperscript, Tailwind CSS
- **Templating:** Go `html/template`

## Getting Started

### Prerequisites

- Go 1.23+
- PostgreSQL

### Local Development

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/your-org/rmbl-server.git
    cd rmbl-server
    ```

2.  **Set up the database:**
    Ensure you have a PostgreSQL database running and set the `DATABASE_URL` environment variable:
    ```bash
    export DATABASE_URL="host=localhost user=postgres password=postgres dbname=rmbl port=5432 sslmode=disable"
    ```

3.  **Run the application:**
    ```bash
    make run
    ```
    The server will start on `http://localhost:3000`.

## Deployment

For production setup, database configuration, and OAuth provider instructions, see the [Deployment Guide](DEPLOYMENT.md).

### Using Docker

```bash
docker build -t rmbl .
docker run -p 3000:3000 -e DATABASE_URL="your-db-url" rmbl
```

## License

MIT
