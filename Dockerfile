FROM ubuntu:24.04

COPY simple-discord-bot /app/
CMD ["/app/simple-discord-bot", "--config", "/config/config.yaml"]
