# Builds the headless game server. No engine and no assets go in this image —
# the Godot client is distributed separately, straight to each player.

FROM golang:1.25-alpine AS build
WORKDIR /src

COPY server/go.mod server/go.sum ./
RUN go mod download

COPY server/ ./
RUN CGO_ENABLED=0 go build -trimpath -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/server /server
EXPOSE 8080
ENTRYPOINT ["/server"]
