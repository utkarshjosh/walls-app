# Build:  docker build -t walls apps/api
# Run:    docker run --rm -p 8787:8787 -v walls-data:/data walls            (serves)
#         docker run --rm -v walls-data:/data walls seed                      (loads the starter set)
#
# The image has no shell and runs as a non-root user. Settings are passed as
# WALLS_* environment variables (docker run -e ... or --env-file).

FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/walls ./cmd/walls
# An empty data folder to copy with the right owner (the final image cannot mkdir).
RUN mkdir /out/data

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/walls /app/walls
# Live scenes, so "walls seed" finds them at ./seed/scenes.
COPY seed /app/seed
COPY --from=build --chown=nonroot:nonroot /out/data /data
ENV WALLS_ADDR=:8787 \
    WALLS_DATA_DIR=/data
VOLUME /data
EXPOSE 8787
USER nonroot:nonroot
ENTRYPOINT ["/app/walls"]
CMD ["serve"]
