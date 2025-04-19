# simple-discord-bot

## Development

### Setup

#### Using Air

```bash
go install github.com/air-verse/air@latest
```

Make sure Go is in your PATH

```bash
export GOPATH=$HOME/go
export PATH=$PATH:$GOROOT/bin:$GOPATH/bin
export PATH=$PATH:$(go env GOPATH)/bin
```

The `run-dev.sh` script will setup the paths when used to launch air.