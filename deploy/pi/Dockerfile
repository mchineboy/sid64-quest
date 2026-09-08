FROM alpine:3.22@sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY auth-service telnet-gateway rck-admin game-core terminal-edge core-switch /app/
USER 65532:65532
CMD ["/app/auth-service"]
