FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.25-alpine AS build
WORKDIR /src
ARG GOPROXY=https://proxy.golang.org,direct
COPY go.mod go.sum ./
RUN GOPROXY="$GOPROXY" go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY --from=web /src/internal/server/web/ internal/server/web/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ane ./cmd/ane

FROM alpine:3.22
ARG ALPINE_MIRROR=https://dl-cdn.alpinelinux.org/alpine
RUN sed -i "s|https://dl-cdn.alpinelinux.org/alpine|${ALPINE_MIRROR}|g" /etc/apk/repositories
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 ane
WORKDIR /app
COPY --from=build /out/ane /app/ane
USER ane
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD wget -q -O - http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/app/ane"]
