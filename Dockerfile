FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
COPY index.html ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /landing .

FROM scratch
COPY --from=build /landing /landing
EXPOSE 8080
ENTRYPOINT ["/landing"]
