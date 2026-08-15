# Local development

## Run

    go run .

The server listens on http://localhost:8080 (override with `PORT`). Templates, static assets, and sample data are read from disk on every request — edit a file and refresh the browser; no restart needed.

## Local data

The server reads local data from `sampledata/`, mirroring the production Sheets layout: one directory per app, one CSV file per table, first row is the schema. It goes through the same data-source interface production backends implement, so app code never knows which backend it is talking to.

## Layout

- `main.go` — server entry point and app routing
- `internal/data` — data source interface and the CSV sample-data implementation
- `internal/directory` — directory app handlers
- `web/directory` — directory app page templates and static assets
