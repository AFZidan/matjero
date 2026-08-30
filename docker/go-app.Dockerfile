FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG APP_PATH=./apps/admin-api
RUN go build -o /out/app ${APP_PATH}

FROM alpine:3.22

RUN adduser -D -H appuser
USER appuser
COPY --from=build /out/app /app
ENTRYPOINT ["/app"]
