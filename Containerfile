FROM --platform=$BUILDPLATFORM docker.io/library/golang:1.27.2 AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /service .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /service /service
EXPOSE 8080
ENTRYPOINT ["/service"]
