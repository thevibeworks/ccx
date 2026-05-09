FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME}" \
    -o /ccx ./cmd/ccx

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /ccx /ccx
EXPOSE 8080
ENTRYPOINT ["/ccx"]
CMD ["web", "--host", "0.0.0.0", "--port", "8080", "--no-open"]
