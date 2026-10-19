# docker build -t gachi-server .
# docker run -d -p 8080:8080 -v gachi-data:/data --name gachi gachi-server

# 1) Frontend: Vite + Preact → internal/webui/dist
FROM node:22-alpine AS web
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# 2) Backend: Go, embeds the built frontend
FROM golang:1.24 AS build
# Let Go fetch a newer toolchain if a dependency requires it.
ENV GOTOOLCHAIN=auto CGO_ENABLED=0
WORKDIR /src
COPY . .
COPY --from=web /src/internal/webui/dist ./internal/webui/dist
RUN go mod tidy && go build -trimpath -ldflags "-s -w" -o /out/gachi-server ./cmd/server

# 3) Runtime: one static binary
FROM gcr.io/distroless/static-debian12
COPY --from=build /out/gachi-server /gachi-server
ENV GACHI_DB=/data/gachi.db GACHI_ADDR=:8080
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["/gachi-server", "serve"]
