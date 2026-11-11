FROM golang:1.24.5-alpine3.22 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/api ./cmd/api && \
    CGO_ENABLED=0 go build -trimpath -o /out/migrate ./cmd/migrate && \
    CGO_ENABLED=0 go build -trimpath -o /out/seed ./cmd/seed && \
    CGO_ENABLED=0 go build -trimpath -o /out/create-admin ./cmd/create-admin
FROM alpine:3.22.1
RUN addgroup -S app && adduser -S -G app app
COPY --from=build /out/ /usr/local/bin/
USER app
EXPOSE 8080
ENTRYPOINT ["api"]
