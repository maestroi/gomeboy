# frontend: build the Svelte static assets (adapter-static outputs to build/)
FROM node:20-alpine AS frontend
WORKDIR /src
COPY pkg/display/web/gomeboy-web/package.json pkg/display/web/gomeboy-web/package-lock.json ./
RUN npm ci
COPY pkg/display/web/gomeboy-web/ ./
RUN npm run build

# builder: web-only Go binary (no glfw/fyne/audio drivers). The web driver
# uses cbrotli (cgo + C brotli), so cgo and the brotli dev libs are required.
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache gcc musl-dev brotli-dev pkgconfig
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /out/gomeboy-web ./cmd/gomeboy-web

# final: dynamically linked binary + pre-built frontend, no TLS calls so no
# ca-certificates; brotli-libs provides cbrotli's runtime shared libraries
FROM alpine:3.20
RUN apk add --no-cache brotli-libs
COPY --from=builder /out/gomeboy-web /gomeboy-web
COPY --from=frontend /src/build/ /app/
ENV GOMEBOY_WEB_STATIC_DIR=/app
EXPOSE 8090
ENTRYPOINT ["/gomeboy-web"]
