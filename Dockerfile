# Kamishibai triage board.
#
# Produces a single small image containing the Go binary, the migrations compiled
# into it, and the built frontend. There is nothing else to deploy: the same
# binary serves the API, the static assets, the scheduled rollup job and the CLI.

# ---------------------------------------------------------------------------
# 1. Build the frontend
# ---------------------------------------------------------------------------
FROM node:26-alpine AS frontend

WORKDIR /app/frontend

# Dependencies first, so a source-only change reuses this layer.
# `npm ci` installs exactly what the lockfile pins, unlike `npm install`.
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci

COPY frontend/ ./

# Vite is configured to emit into ../backend/pb_public, which is where the
# backend expects to find static assets.
RUN mkdir -p /app/backend && npm run build

# ---------------------------------------------------------------------------
# 2. Build the backend
# ---------------------------------------------------------------------------
FROM golang:1.27-alpine AS backend

WORKDIR /src

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./

# Run the tests as part of the image build. A container that cannot pass its own
# unit tests should not reach a cluster.
RUN go vet ./... && go test ./...

# CGO stays off: PocketBase uses the pure Go SQLite port, which is what allows
# the static base image below.
# -trimpath keeps build paths out of the binary; -s -w drop the symbol table.
RUN CGO_ENABLED=0 GOOS=linux go build \
      -trimpath \
      -ldflags="-s -w" \
      -o /out/kamishibai .

# An empty data directory, staged here purely so the final image can own it.
# Explained at the COPY below.
RUN mkdir -p /out/pb_data

# ---------------------------------------------------------------------------
# 3. Runtime
# ---------------------------------------------------------------------------
# distroless static: no shell, no package manager, no libc. Nothing to exploit
# and nothing to patch. The timezone database is compiled into the binary (see
# internal/domain/tzdata.go), so period boundaries are correct here even though
# the image carries no /usr/share/zoneinfo.
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=backend /out/kamishibai /app/kamishibai
COPY --from=frontend /app/backend/pb_public /app/pb_public

# The data directory has to exist in the image, owned by the user the process runs
# as (65532, "nonroot" in this base image).
#
# Docker seeds a fresh volume from whatever is at the mount point in the image,
# ownership included. If the directory is absent, Docker creates the mount point
# root-owned instead, and a non-root process cannot create the database file
# inside it: SQLite reports "unable to open database file" and the container exits.
# Kubernetes sidesteps this with fsGroup, but the image should not depend on the
# orchestrator to be usable.
#
# Numeric IDs are used because there is no shell here to resolve a user name.
COPY --from=backend --chown=65532:65532 /out/pb_data /app/pb_data

# The SQLite database lives here and must be a mounted volume. Without one, data
# is lost when the pod restarts.
VOLUME ["/app/pb_data"]

EXPOSE 8090

# Runs as uid 65532 from the base image; never root.
USER nonroot:nonroot

ENV KAMISHIBAI_TIMEZONE=America/New_York \
    KAMISHIBAI_PUBLIC_DIR=/app/pb_public

ENTRYPOINT ["/app/kamishibai"]

# `serve` applies any pending migrations before listening, so a deploy carrying a
# schema change needs no separate migration step.
CMD ["serve", "--http=0.0.0.0:8090", "--dir=/app/pb_data"]
