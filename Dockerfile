# ── Stage 1: Frontend (Node) ──────────────────────────────────────────────────
# Vite baut nach "../server/static" relativ zum frontend-Verzeichnis.
# Deshalb die Verzeichnisstruktur aus dem Repo nachbauen.
FROM node:20-alpine AS frontend
WORKDIR /workspace/Development/frontend
COPY Development/frontend/package*.json ./
RUN npm ci
COPY Development/frontend/ .
RUN npm run build
# Ergebnis liegt jetzt in /workspace/Development/server/static/

# ── Stage 2: Go-Server ────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder
WORKDIR /build
COPY Development/server/go.mod Development/server/go.sum ./
RUN go mod download
COPY Development/server/ .
# Eingebettete statische Dateien aus Stage 1
COPY --from=frontend /workspace/Development/server/static ./static/
RUN CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -ldflags="-s -w" -o liedanzeige .

# ── Stage 3: Runtime ──────────────────────────────────────────────────────────
FROM alpine:3.20
# /data wird als Volume gemountet:
#   config.json   – muss manuell vorbefüllt werden (server_host, port)
#   settings.json – wird beim ersten Start automatisch angelegt
#   server.log    – wird beim ersten Start automatisch angelegt
WORKDIR /data
COPY --from=builder /build/liedanzeige /usr/local/bin/liedanzeige
EXPOSE 1980
CMD ["/usr/local/bin/liedanzeige"]
