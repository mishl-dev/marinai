FROM alpine:3.19

RUN apk add --no-cache ca-certificates

RUN adduser -D -u 1000 sandbox

RUN mkdir -p /workspace && chown sandbox:sandbox /workspace

WORKDIR /workspace

COPY sandbox-service /app/sandbox-service

USER sandbox

EXPOSE 3002

CMD ["/app/sandbox-service"]
