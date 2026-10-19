# =============================================================================
# Stage 1: Builder
# =============================================================================
FROM golang:1.26.0-alpine AS builder

# gcc + musl-dev werden für CGO benötigt (go-sqlite3)
RUN apk add --no-cache gcc musl-dev git

# Definiere Build-Arguments zur Weitergabe an den Build-Prozess
ARG VERSION
ARG BUILD_TIME

# Ein Arbeitsverzeichnis für die App erstellen
WORKDIR /app

# Go-Abhängigkeiten separat laden, um Layer-Caching zu nutzen
COPY go.mod go.sum ./
RUN go mod download

# Kopiere den Rest des Codes ins Arbeitsverzeichnis
COPY . .

# Swagger-Docs werden bereits vom Pre-commit-Hook generiert und sind im Repo enthalten
# Baue die ausführbare Datei
# -w -s entfernt Debug-Symbole und reduziert die Binärgröße deutlich
RUN GOOS=linux go build -o /app/bin/server -ldflags "\
  -w -s \
  -X 'github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/version.Version=${VERSION:-dev}' \
  -X 'github.com/BjoernFuertges/Socioeconomic-Data/backend/internal/version.BuildTime=${BUILD_TIME:-unknown}'" \
  ./cmd/soziodata

# =============================================================================
# Stage 2: Minimales Runtime-Image
# =============================================================================
FROM alpine:3.21

# CA-Zertifikate (für HTTPS) und Zeitzonen-Daten
RUN apk add --no-cache ca-certificates tzdata

# Nicht-Root-Benutzer für mehr Sicherheit
RUN adduser -D -g '' appuser

# Setze Umgebungsvariablen für den Port
ENV PORT=8080

WORKDIR /app

# Nur die Binary und .env.example aus dem Builder-Stage kopieren
COPY --from=builder /app/bin/server .
COPY --from=builder /app/.env.example .

USER appuser

# Dokumentiere den offenen Port
EXPOSE ${PORT}

# Starte die Anwendung
CMD ["./server"]
