FROM node:22.21.0-alpine AS web-build
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
ARG VITE_MAP_TILE_URL
ARG VITE_MAP_ATTRIBUTION
ENV VITE_MAP_TILE_URL=$VITE_MAP_TILE_URL
ENV VITE_MAP_ATTRIBUTION=$VITE_MAP_ATTRIBUTION
RUN npm run build

FROM golang:1.24.5-alpine3.22 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 go build -trimpath -o /out/api ./cmd/api && \
    CGO_ENABLED=0 go build -trimpath -o /out/migrate ./cmd/migrate && \
    CGO_ENABLED=0 go build -trimpath -o /out/seed ./cmd/seed && \
    CGO_ENABLED=0 go build -trimpath -o /out/create-admin ./cmd/create-admin
FROM alpine:3.22.1
RUN addgroup -S app && adduser -S -G app app
COPY --from=build /out/ /usr/local/bin/
COPY --from=web-build /src/web/dist/ /app/web/
ENV WEB_DIR=/app/web
USER app
EXPOSE 8080
ENTRYPOINT ["api"]
