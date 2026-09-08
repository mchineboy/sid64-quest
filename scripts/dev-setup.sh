#!/bin/bash

# SID64 Quest - Development Setup Script

set -e

echo "SID64 Quest - Development Setup"
echo "=============================================="

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if required tools are installed
check_requirements() {
    print_status "Checking requirements..."
    
    # Check Go
    if ! command -v go &> /dev/null; then
        print_error "Go is not installed. Please install Go 1.21 or later."
        exit 1
    fi
    
    GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
    print_success "Go $GO_VERSION is installed"
    
    # Check Docker
    if ! command -v docker &> /dev/null; then
        print_error "Docker is not installed. Please install Docker."
        exit 1
    fi
    
    print_success "Docker is installed"
    
    # Check Docker Compose
    if ! command -v docker-compose &> /dev/null; then
        print_error "Docker Compose is not installed. Please install Docker Compose."
        exit 1
    fi
    
    print_success "Docker Compose is installed"
    
    # Check Make
    if ! command -v make &> /dev/null; then
        print_warning "Make is not installed. Some build commands may not work."
    else
        print_success "Make is installed"
    fi
}

# Create necessary directories
create_directories() {
    print_status "Creating project directories..."
    
    mkdir -p build
    mkdir -p logs
    mkdir -p data
    mkdir -p templates
    mkdir -p static
    mkdir -p scripts
    mkdir -p k8s
    mkdir -p docker
    mkdir -p monitoring/prometheus
    mkdir -p monitoring/grafana/dashboards
    mkdir -p monitoring/grafana/datasources
    mkdir -p tests/integration
    mkdir -p tests/load
    
    print_success "Project directories created"
}

# Setup environment files
setup_environment() {
    print_status "Setting up environment files..."
    
    # Create .env file if it doesn't exist
    if [ ! -f .env ]; then
        umask 077
        local rck_pg_password=$(openssl rand -hex 32)
        local rck_mongo_password=$(openssl rand -hex 32)
        local rck_auth_secret=$(openssl rand -hex 32)
        cat > .env << EOF
# SID64 Quest - Environment Configuration

# Server Configuration
HOST=0.0.0.0
TELNET_PORT=2323
PETSCII_PORT=6464
HTTP_PORT=8080
READ_TIMEOUT=30
WRITE_TIMEOUT=30
IDLE_TIMEOUT=900
MAX_CONNECTIONS=1000

# PostgreSQL Configuration
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_DB=race_condition_kingdom
POSTGRES_USER=mud_user
POSTGRES_PASSWORD=$rck_pg_password
POSTGRES_SSL_MODE=disable
POSTGRES_MAX_CONNS=25

# Redis Configuration
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0
REDIS_POOL_SIZE=10

# MongoDB Configuration
MONGODB_URI=
MONGODB_ROOT_PASSWORD=$rck_mongo_password
MONGODB_DB=race_condition_kingdom_logs
MONGODB_TIMEOUT=10

# Authentication Configuration
AUTH_TOKEN_EXPIRY=300
AUTH_SESSION_EXPIRY=86400
AUTH_BASE_URL=http://localhost:8080
AUTH_SECRET_KEY=$rck_auth_secret
BCRYPT_COST=12

# Discord Configuration (optional)
DISCORD_TOKEN=
DISCORD_GUILD_ID=
DISCORD_ADMIN_CHANNEL=
DISCORD_LOG_CHANNEL=
DISCORD_ERROR_CHANNEL=

# Game Configuration
GAME_TICK_RATE=60
GAME_MAX_PLAYERS=500
GAME_REST_INTERVAL=3600
GAME_STAMINA_DRAIN=1
GAME_DEATH_PENALTY=100
GAME_MAX_INVENTORY=50
GAME_STARTING_GOLD=100
GAME_STARTING_HEALTH=100
GAME_STARTING_STAMINA=100
EOF
        print_success "Created .env file"
    else
        print_warning ".env file already exists, skipping"
    fi
    
    # Create .env.example
    cp .env .env.example
    print_success "Created .env.example file"
}

# Setup monitoring configuration
setup_monitoring() {
    print_status "Setting up monitoring configuration..."
    
    # Prometheus configuration
    cat > monitoring/prometheus/prometheus.yml << EOF
global:
  scrape_interval: 15s
  evaluation_interval: 15s

rule_files:
  # - "first_rules.yml"
  # - "second_rules.yml"

scrape_configs:
  - job_name: 'prometheus'
    static_configs:
      - targets: ['localhost:9090']

  - job_name: 'race-condition-kingdom'
    static_configs:
      - targets: ['host.docker.internal:8080', 'host.docker.internal:2323']
    metrics_path: /metrics
    scrape_interval: 5s
EOF

    # Grafana datasource configuration
    cat > monitoring/grafana/datasources/prometheus.yml << EOF
apiVersion: 1

datasources:
  - name: Prometheus
    type: prometheus
    access: proxy
    url: http://prometheus:9090
    isDefault: true
EOF

    print_success "Monitoring configuration created"
}

# Setup Git hooks
setup_git_hooks() {
    print_status "Setting up Git hooks..."
    
    if [ -d .git ]; then
        # Pre-commit hook
        cat > .git/hooks/pre-commit << 'EOF'
#!/bin/bash
# Pre-commit hook for SID64 Quest

echo "Running pre-commit checks..."

# Format code
go fmt ./...

# Run tests
if ! go test ./...; then
    echo "Tests failed. Commit aborted."
    exit 1
fi

# Run linter if available
if command -v golangci-lint &> /dev/null; then
    if ! golangci-lint run; then
        echo "Linting failed. Commit aborted."
        exit 1
    fi
fi

echo "Pre-commit checks passed!"
EOF
        chmod +x .git/hooks/pre-commit
        print_success "Git pre-commit hook installed"
    else
        print_warning "Not a Git repository, skipping Git hooks"
    fi
}

# Download Go dependencies
download_dependencies() {
    print_status "Downloading Go dependencies..."
    
    go mod download
    go mod tidy
    
    print_success "Go dependencies downloaded"
}

# Start development services
start_dev_services() {
    print_status "Starting development services..."
    
    # Start Docker Compose services
    docker-compose -f docker-compose.dev.yml up -d
    
    # Wait for services to be ready
    print_status "Waiting for services to be ready..."
    sleep 10
    
    # Check service health
    if docker-compose -f docker-compose.dev.yml ps | grep -q "Up (healthy)"; then
        print_success "Development services started successfully"
    else
        print_warning "Some services may not be fully ready yet"
    fi
}

# Main setup function
main() {
    echo
    print_status "Starting development environment setup..."
    echo
    
    check_requirements
    echo
    
    create_directories
    echo
    
    setup_environment
    echo
    
    setup_monitoring
    echo
    
    setup_git_hooks
    echo
    
    download_dependencies
    echo
    
    # Ask if user wants to start services
    read -p "Do you want to start development services now? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo
        start_dev_services
    fi
    
    echo
    print_success "Development environment setup complete!"
    echo
    echo "Next steps:"
    echo "1. Review and update .env file with your configuration"
    echo "2. Start development services: make dev-start"
    echo "3. Build the project: make build"
    echo "4. Run tests: make test"
    echo "5. Start the telnet gateway: make run-telnet-gateway"
    echo "6. Start the auth service: make run-auth-service"
    echo
    echo "Development URLs:"
    echo "- Telnet: telnet localhost 2323"
    echo "- Auth Service: http://localhost:8080"
    echo "- pgAdmin: http://localhost:8081"
    echo "- Redis Commander: http://localhost:8082"
    echo "- Mongo Express: http://localhost:8083"
    echo "- Grafana: http://localhost:3000"
    echo "- Prometheus: http://localhost:9090"
    echo
}

# Run main function
main "$@"
