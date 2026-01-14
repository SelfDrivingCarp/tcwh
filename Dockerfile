# Stage 0: use a golang image to build tcwh
FROM golang:1.25-alpine AS build
WORKDIR /work
ARG GOOS=linux
ARG GOARCH=amd64
ADD / .
ENV GOOS="$GOOS"
ENV GOARCH="$GOARCH"
RUN go build -o /tmp/tcwh -trimpath -tags netgo -ldflags="-w -s" cmd/tcwh/*.go

# Stage 1: build the container that actually gets run
FROM scratch
COPY --from=build /tmp/tcwh /
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /work/server/html /html

USER 9999:9999
EXPOSE 8000/tcp
ENTRYPOINT ["/tcwh", "/data/tcwh-cfg.yaml"]