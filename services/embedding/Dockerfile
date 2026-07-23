FROM golang:1.25-alpine AS build
WORKDIR /src
COPY services/embedding/go.mod services/embedding/go.sum ./
RUN go mod download
COPY services/embedding/ .
RUN CGO_ENABLED=0 go build -o /out/embedding ./cmd/embedding

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/embedding /embedding
ENTRYPOINT ["/embedding"]
