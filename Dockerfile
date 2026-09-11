FROM golang:1.25.5-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/mock-accounting ./cmd/mock-accounting

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && adduser -D -H -u 10001 app
COPY --from=build /out/server /usr/local/bin/server
COPY --from=build /out/migrate /usr/local/bin/migrate
COPY --from=build /out/mock-accounting /usr/local/bin/mock-accounting
USER app
EXPOSE 8080 8081
ENTRYPOINT ["server"]
