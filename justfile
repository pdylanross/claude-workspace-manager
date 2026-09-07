set quiet

# Format, lint, test, and build
all: fmt lint test build

# Install required development tools
tools:
    echo "Installing development tools..."
    go install golang.org/x/tools/cmd/goimports@latest
    go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
    go install github.com/goreleaser/goreleaser/v2@latest
    echo "All tools installed successfully"

# Build the project
build:
    echo "Building..."
    go build -v ./...

# Run the cwm CLI — usage: just run version  /  just run -- version --short
run *args:
    go run ./cmd/cwm {{args}}

# Format code using goimports
fmt:
    echo "Formatting code..."
    goimports -w .
    gofmt -s -w .

# Run linter
lint:
    echo "Running golangci-lint..."
    golangci-lint run --fix

# Run unit tests
test:
    echo "Running unit tests..."
    go test -v ./... -race

# Build a local snapshot release into dist/ without tagging
snapshot:
    echo "Building snapshot release..."
    goreleaser release --snapshot --clean

# Clean build artifacts
clean:
    echo "Cleaning..."
    go clean
    rm -rf dist bin
    rm -f coverage.out
