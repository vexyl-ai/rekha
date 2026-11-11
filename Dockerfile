# Built by GoReleaser: the context holds the release binaries only.
FROM gcr.io/distroless/static-debian12:nonroot
COPY rekha /usr/bin/rekha
COPY rekha-serve /usr/bin/rekha-serve
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/usr/bin/rekha-serve"]
