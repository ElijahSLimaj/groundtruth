FROM golang:1.25-alpine AS build
WORKDIR /src
COPY services/ingestion/go.mod services/ingestion/go.sum ./
RUN go mod download
COPY services/ingestion/ .
RUN CGO_ENABLED=0 go build -o /out/ingestion ./cmd/ingestion

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ingestion /ingestion
ENTRYPOINT ["/ingestion"]
