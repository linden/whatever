FROM golang:1.17.3-alpine3.14

RUN mkdir /app

WORKDIR /app

COPY . .

RUN go build ./src/main.go

RUN apk add --no-cache caddy
RUN apk add --no-cache bash

ENTRYPOINT ["bash", "./run.sh"]