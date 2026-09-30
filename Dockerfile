# The Tessera image (RFC-0007, step 1): the static binary on distroless,
# with no shell and no root. /data holds the journal and the plugins'
# databases. The build context holds the linux binaries as
# <os>/<arch>/tessera: made by goreleaser on a release, or by make image
# locally.
FROM alpine:3.22 AS data
RUN mkdir -p /data && chown 65532:65532 /data

FROM gcr.io/distroless/static-debian12:nonroot
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/tessera /usr/local/bin/tessera
COPY --from=data --chown=65532:65532 /data /data
VOLUME /data
WORKDIR /data
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/tessera"]
