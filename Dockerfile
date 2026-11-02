# Image Go multi-stage de la stack compose (voir ADR 0005).
# Cibles : server (API BOM Studio), priceapi (API de prix simulée), migrate (goose, one-shot).
# Images de base épinglées : on les monte de version volontairement, jamais `latest`.

FROM golang:1.27.2-alpine3.23 AS build
WORKDIR /src
ENV CGO_ENABLED=0
# Dépendances d'abord : la couche reste en cache tant que go.mod / go.sum ne changent pas.
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server \
 && go build -trimpath -ldflags="-s -w" -o /out/priceapi ./cmd/priceapi \
 # goose est un `tool` de go.mod : même version que `make migrate`, aucune dépendance ajoutée.
 && go build -trimpath -ldflags="-s -w" -o /out/goose github.com/pressly/goose/v3/cmd/goose

# Base commune d'exécution : alpine (wget de busybox pour les healthchecks), utilisateur non root.
FROM alpine:3.23.2 AS runtime
RUN addgroup -S app && adduser -S -G app app
USER app

FROM runtime AS server
COPY --from=build /out/server /usr/local/bin/server
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/server"]

FROM runtime AS priceapi
COPY --from=build /out/priceapi /usr/local/bin/priceapi
ENV PORT=8090
EXPOSE 8090
ENTRYPOINT ["/usr/local/bin/priceapi"]

# One-shot : applique les migrations puis s'arrête ; un code de sortie non nul bloque le back.
FROM runtime AS migrate
COPY --from=build /out/goose /usr/local/bin/goose
COPY migrations /migrations
ENTRYPOINT ["/bin/sh", "-c", "exec goose -dir /migrations postgres \"$DATABASE_URL\" up"]
