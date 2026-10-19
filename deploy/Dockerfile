# docker build -f deploy/Dockerfile -t gachi-server .
# docker run -d -p 8080:8080 -v gachi-data:/data --name gachi gachi-server
FROM golang:1.24 AS build
# Let Go fetch a newer toolchain if a dependency requires it.
ENV GOTOOLCHAIN=auto CGO_ENABLED=0
WORKDIR /src
COPY go.mod go.sum* ./
COPY . .
RUN go mod tidy && go build -trimpath -ldflags "-s -w" -o /out/gachi-server ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/gachi-server /gachi-server
ENV GACHI_DB=/data/gachi.db GACHI_ADDR=:8080
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["/gachi-server", "serve"]
