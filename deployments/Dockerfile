# Dockerfile de bootstrap para cmd/api. Sin lógica de negocio: solo
# empaqueta el binario. Se irá ajustando (usuario no-root, distroless,
# healthcheck) cuando el servicio tenga rutas reales.
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/api ./cmd/api

FROM alpine:3.20
RUN adduser -D -u 10001 appuser
COPY --from=build /out/api /usr/local/bin/api
USER appuser
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/api"]
