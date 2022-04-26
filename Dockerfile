FROM golang:1.18.1-stretch as builder
WORKDIR /usr/src/app
COPY fritz-status/go.mod fritz-status/go.sum ./
RUN go mod download && go mod verify

COPY fritz-status .
RUN go build  .

FROM telegraf:1.22.1 as final

COPY --from=builder /usr/src/app/fritz-status /usr/local/lib/telegraf/fritz-status
RUN chown -R telegraf:telegraf /usr/local/lib/telegraf/fritz-status

ENTRYPOINT ["/entrypoint.sh"]
CMD ["telegraf"]
