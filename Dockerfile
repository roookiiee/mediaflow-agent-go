FROM golang:1.22-bookworm AS build

WORKDIR /src
COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/mediaflow-agent ./cmd/mediaflow-agent

FROM alpine:3.20

RUN apk add --no-cache ca-certificates && adduser -D -H appuser
WORKDIR /app

COPY --from=build /out/mediaflow-agent /app/mediaflow-agent
COPY web /app/web
COPY skills /app/skills
COPY knowledge /app/knowledge

RUN mkdir -p /app/data && chown -R appuser:appuser /app
USER appuser

ENV ADDR=:8080
ENV DATA_DIR=/app/data

EXPOSE 8080
CMD ["/app/mediaflow-agent"]
