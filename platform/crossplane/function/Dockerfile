FROM golang:1.25.10-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /function .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /function /function
EXPOSE 9443
USER nonroot:nonroot
ENTRYPOINT ["/function"]
