# Git Commit Hash Embedding (SO-15)

## Overview

The `secondorder` binary now embeds the git commit hash at build time and exposes it at runtime.

## Implementation

### Variable

`internal/models/version.go` defines:

```go
var CommitHash = "unknown"
```

The default value `"unknown"` acts as a safe fallback when the binary is built outside a git repository.

### Makefile

The `build` target uses `-ldflags` to inject the current short commit hash:

```makefile
COMMIT_HASH=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS=-ldflags "-X github.com/msoedov/secondorder/internal/models.CommitHash=$(COMMIT_HASH)"
```

If `git` is unavailable or the directory is not a git repo, it falls back to `"unknown"`.

### GoReleaser

`.goreleaser.yml` passes the full commit hash via GoReleaser's built-in `{{.Commit}}` template variable:

```yaml
ldflags:
  - -X github.com/msoedov/secondorder/internal/models.CommitHash={{.Commit}}
```

### Startup Log

On startup the binary logs the embedded commit hash at `INFO` level:

```
secondorder running  url=http://localhost:3001  commit=a9d657f
```

## Usage

```bash
# Normal dev build (embeds short hash automatically)
make build

# Build without git (fallback = "unknown")
go build -o secondorder ./cmd/secondorder

# Access in code
import "github.com/msoedov/secondorder/internal/models"
fmt.Println(models.CommitHash)
```
