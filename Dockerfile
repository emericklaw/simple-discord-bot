FROM ubuntu:24.04

RUN apt-get update && apt install -y ca-certificates

COPY simple-discord-bot /app/
CMD ["/app/simple-discord-bot", "--config", "/config/config.yaml"]
