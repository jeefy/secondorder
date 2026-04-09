package models

// Version is the current version of the application.
const Version = "v0.1.0"

// CommitHash is the git commit hash embedded at build time via ldflags.
// Falls back to "unknown" if not set during build.
var CommitHash = "unknown"
