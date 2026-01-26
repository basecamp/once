# AGENTS.md

## Project Overview

Amar is a CLI/TUI tool for installing and managing web applications from Docker
images. It's designed to make self-hosting as easy as possible.

Amar uses a proxy server (github.com/basecamp/kamal-proxy) to route traffic to
the application containers, which allows it to provide zero-downtime restarts
and upgrades, automatic SSL, and multiple applications running on a single
server. A single instance of Amar may deploy one proxy container, along with
multiple application containers. Host-based routing is used inside the proxy to
route traffic to the correct application container.

## Code Architecture

## Design and Concepts

### Container Naming

All containers are namespaced (default namespace: "amar"):
- Proxy: `{namespace}-proxy`
- Apps: `{namespace}-app-{appName}-{shortID}`

This allows us to easily identify the app containers, but still allows us to
boot a second app container without naming collisions when we want to deploy a
new version without downtime.

### Data and State

Amar deals primarily with two classes of state:

- Application data: this is stored in Docker volumes to provide persistent
storage between versions of the app container. Amar provides one volume for
each app, which it will mount into two locations (`/storage` and
`/rails/storage`) to match typical app conventions. Amar may provide backup and
restore features on the contents of those volumes. But it does not otherwise
touch the volume contents, or have any opinions about what should be in there
-- this data is entirely for each app's own use.

- Configuration: this keeps track of the applications and settings that have
been set up using Amar itself. For example, the list of applications that are
deployed, the hostname and TLS settings for each, any custom port settings for
the proxy, and so on. Amar stores this information as JSON strings in an `amar`
label on the containers and volumes, so that everything is stored within the
Docker state. Application settings go with the app container; proxy settings go
with the proxy container; volume settings (like encryption keys) go with the
volume.

## Build Commands

```bash
make build       # Build binary to bin/ (CGO disabled)
make test        # Run unit tests (internal/ packages)
make integration # Run integration tests (requires Docker)
make lint        # Run golangci-lint
```

Run a single test:
```bash
go test -v -run TestName ./internal/...
```

## Coding Style

- Always follow idiomatic Go style
- Don't use excessive comments. Try to make the code speak for itself; only resort to adding comments where the meaning or intention may otherwise be unclear, or a subtle detail may be missed.
- Organize imports in sections: stdlib imports, 3rd-party imports, and project imports. Each section should be sorted alphabetically, and the sections should be separated from each other by 1 blank line.
- Where a struct type has both public and private methods, arrange the public methods first, then add a comment line of `// Private` and put the private methods below that comment. In the public section, put any constructor methods first, followed by simple accessor methods, followed by the rest.
- Private functions that are not methods of a type should go last in the file, and should be separated with a `// Helpers` comment.
- Prefer modern Go constructs where possible (for example, ranging over numbers is better than a for loop). If the LSP suggests there is an option to modernize something you should generally do so.
- Write tests to cover changes where possible, but don't go overboard trying to cover every single case. Prefer using helper functions (including functions defined locally inside the test) over table-driven tests, except in cases where the latter would be more readable.
- Regularly check your work with the linter and LSP to ensure it follows conventions, and run tests as needed to ensure they pass.
- Consider opportunities to refactor large methods into smaller pieces, and spot opportunities where it's worth extracting functionality into a new type. But do not go overboard with this.

## Agent behaviour

- Never make commits, or push changes to remotes. I will take care of this myself. You may, however, do any read-only Git operations that you like, such as checking for upstream changes, reading history.
