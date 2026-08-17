# Builds the headless game server and bundles the exported web client with it,
# so one Fly machine serves both the page and the protocol. That is what lets
# the browser client configure nothing: its own origin is the server.
#
# Export the client BEFORE building this image:
#   .\scripts\build-web.ps1
# Without that, build/web is empty and the image still runs — it just serves no
# page, only the /ws protocol for native clients.

FROM golang:1.25-alpine AS build
WORKDIR /src

COPY server/go.mod server/go.sum ./
RUN go mod download

COPY server/ ./
RUN CGO_ENABLED=0 go build -trimpath -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/server /server
COPY build/web /web
# The map, the item table and the spell table. Without these the server still
# starts — and that is the trap: it comes up on a generated empty arena with no
# objects and no spells, which looks like a broken game rather than like a
# missing flag. The first deploy did exactly that.
COPY server/maps /maps
EXPOSE 8080
ENTRYPOINT ["/server", \
    "-web-dir", "/web", \
    "-map-file", "/maps/map1.json", \
    "-items-file", "/maps/items.json", \
    "-spells-file", "/maps/spells.json"]
