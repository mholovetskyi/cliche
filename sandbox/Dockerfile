# Cliché sandbox image — the per-user container for the cloud product.
#
# It runs `cliche serve` in AUTHED, network-bound mode with a real build toolchain
# (git, Node, Go, headless Chromium) so the agent can build AND visually verify
# real projects. The orchestrator/gateway injects CLICHE_SERVE_TOKEN and maps each
# user to their own container.
#
# SECURITY: the container IS the isolation boundary. NEVER run two users in one
# container — the agent executes arbitrary shell commands. `cliche serve` refuses
# to bind a non-loopback address without a token (see internal/web/auth.go), so an
# unauthenticated sandbox cannot exist.

# --- build the CLI (the React UI is committed in internal/web/static → no Node here)
FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/cliche ./cmd/cliche

# --- runtime: the agent's toolchain (Node + Go + git + headless Chromium)
FROM node:20-bookworm-slim AS runtime
RUN apt-get update \
 && apt-get install -y --no-install-recommends git ca-certificates curl chromium \
 && rm -rf /var/lib/apt/lists/*
# Go toolchain so the agent can build Go projects it writes.
COPY --from=golang:1.23-bookworm /usr/local/go /usr/local/go
ENV PATH="/usr/local/go/bin:${PATH}"

# Never run the agent as root inside the sandbox.
RUN useradd -m -u 10001 cliche
COPY --from=build /out/cliche /usr/local/bin/cliche
USER cliche
WORKDIR /home/cliche/workspace

ENV CLICHE_NO_BROWSER=1 \
    CLICHE_WORKSPACE=/home/cliche/workspace \
    CLICHE_BROWSER=/usr/bin/chromium
# CLICHE_SERVE_TOKEN is injected by the orchestrator at run time (never baked in).
EXPOSE 7878
ENTRYPOINT ["cliche", "serve", "--listen", "0.0.0.0:7878", "--dir", "/home/cliche/workspace"]
