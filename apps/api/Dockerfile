FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 GOTOOLCHAIN=local go build -trimpath -ldflags="-s -w" -o /out/rachata .

FROM alpine:3.23 AS base
RUN apk add --no-cache ca-certificates && addgroup -g 10001 app && adduser -D -u 10001 -G app app
WORKDIR /app
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/app/rachata"]

FROM base AS api
COPY --from=build /out/rachata /app/rachata

FROM base AS worker
USER root
RUN apk add --no-cache ffmpeg
COPY --from=build /out/rachata /app/rachata
USER 10001:10001
