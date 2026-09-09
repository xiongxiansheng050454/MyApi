FROM golang:1.26-alpine AS builder
WORKDIR /src
ENV CGO_ENABLED=0
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=$GOPROXY

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/server /app/server
COPY configs/ /app/configs/
EXPOSE 8080
ENTRYPOINT ["/app/server"]
