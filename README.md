# 🏰 Race Condition Kingdom

A modern MUD (Multi-User Dungeon) written in Go, built around nostalgia for telnet-based gameplay but modernized with out-of-band HTTPS authentication, extensibility, and balance-focused gameplay design.

## 🎯 Project Overview

Race Condition Kingdom combines the classic feel of traditional MUDs with modern architecture and security practices. Players connect via telnet for that authentic retro experience, but authentication happens securely through HTTPS, and the entire system is designed to run on Kubernetes with horizontal scaling.

## ✨ Key Features

### 🔐 Authentication & Connectivity
- **Telnet-based gameplay** with ANSI color support
- **Out-of-band HTTPS authentication** for security
- **Web-based telnet client** option (coming soon)
- **Session management** with Redis-backed tokens
- **Multi-character support** per account

### 🏗️ Modern Architecture
- **Kubernetes-native** with stateless containers
- **Hybrid database approach**: PostgreSQL for critical data, Redis for sessions/cache, MongoDB for logs
- **Event-driven architecture** with Redis pub/sub
- **Horizontal scaling** support
- **Health checks** and monitoring

### 🎮 Gameplay Systems
- **Town square hub** with banks, shops, inns, and auction house
- **Territory-based PvP** with safe zones
- **Stamina/health management** with rest requirements
- **Death and recovery system** with humorous healthcare references
- **Alignment system** (Lawful/Chaotic, Good/Evil)
- **Time-of-day and seasonal cycles** (planned)

### 🧠 Extensibility
- **World Building Mode** with sandboxed environment
- **Starlark scripting** for safe, sandboxed content creation
- **Admin approval system** for user-generated content
- **Discord integration** for logging and moderation
- **Plugin architecture** for custom functionality

### 💰 Economy
- **Player-to-player trading** and gifting
- **Auction house** with bidding system
- **Weight-based inventory** with encumbrance limits
- **Inn storage** with "infinite" capacity (Hitchhiker's Guide reference)
- **Dual banking system** with different rules and benefits

## 🚀 Quick Start

### Prerequisites

- Go 1.21 or later
- Docker and Docker Compose
- Make (optional, but recommended)

### Development Setup

1. **Clone the repository**
   ```bash
   git clone https://github.com/tylerhardison/race-condition-kingdom.git
   cd race-condition-kingdom
   ```

2. **Run the development setup script**
   ```bash
   chmod +x scripts/dev-setup.sh
   ./scripts/dev-setup.sh
   ```

3. **Start development services**
   ```bash
   make dev-start
   ```

4. **Build the project**
   ```bash
   make build
   ```

5. **Start the services**
   ```bash
   # Terminal 1: Start auth service
   make run-auth-service
   
   # Terminal 2: Start telnet gateway
   make run-telnet-gateway
   ```

6. **Connect and play**
   ```bash
   telnet localhost 2323
   ```

### First Login

1. Connect via telnet: `telnet localhost 2323`
2. Enter username when prompted
3. Visit the authentication URL provided
4. Use the default admin account:
   - Username: `admin`
   - Password: `admin123`
5. Return to telnet and type `check` to continue

## 🏗️ Architecture

The system is built with a microservices architecture designed for Kubernetes deployment:

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│  Telnet Gateway │    │  Auth Service   │    │  Game Engine    │
│     (Go)        │    │     (Go)        │    │     (Go)        │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         └───────────────────────┼───────────────────────┘
                                 │
         ┌─────────────────────────────────────────────────┐
         │              Redis Pub/Sub                      │
         │            (Event System)                       │
         └─────────────────────────────────────────────────┘
                                 │
    ┌────────────────┬───────────┼───────────┬────────────────┐
    │                │           │           │                │
┌───▼───┐    ┌──────▼──────┐    │    ┌─────▼─────┐    ┌────▼────┐
│PostgreSQL│  │    Redis    │    │    │  MongoDB  │    │ Discord │
│(Critical │  │ (Sessions/  │    │    │  (Logs/   │    │   Bot   │
│  Data)   │  │  Cache)     │    │    │Analytics) │    │(Logging)│
└──────────┘  └─────────────┘    │    └───────────┘    └─────────┘
                                 │
                    ┌────────────▼────────────┐
                    │    World Builder        │
                    │   (Starlark Scripts)    │
                    └─────────────────────────┘
```

### Core Services

- **Telnet Gateway**: Handles telnet connections, ANSI formatting, and player I/O
- **Auth Service**: Manages HTTPS authentication and session linking
- **Game Engine**: Core game logic, world state, and event processing (planned)
- **World Builder**: Sandboxed content creation with Starlark scripting (planned)
- **Admin Console**: REPL interface for administration (planned)

### Data Storage

- **PostgreSQL**: Users, characters, world data, transactions
- **Redis**: Sessions, real-time game state, pub/sub events
- **MongoDB**: Logs, analytics, builder content

## 🎮 Gameplay

### Basic Commands

```
Movement & Exploration:
  look, l          - Look around the current room
  north, n         - Go north (if exit exists)
  south, s         - Go south (if exit exists)
  east, e          - Go east (if exit exists)
  west, w          - Go west (if exit exists)

Communication:
  say <message>    - Say something to everyone in the room
  tell <player>    - Send a private message to a player
  who, w           - See who's online

Character & Inventory:
  stats, st        - View your character statistics
  inventory, inv, i - View your inventory
  equipment, eq    - View your equipped items

Game Information:
  time             - Check the current game time
  weather          - Check the weather
  help, h          - Show help message
  quit, q          - Quit the game
```

### World Layout

The game starts in the **Town Square**, a safe zone that serves as the hub for all activities:

- **Banks**: Two different banks with unique rules and benefits
- **Shops**: Magic items, weapons/armor, and trinkets
- **Inn**: Rest and "infinite" storage (with Hitchhiker's Guide humor)
- **Auction House**: Player-to-player trading
- **Infirmary**: Resurrection services with healthcare humor

## 🛠️ Development

### Project Structure

```
race-condition-kingdom/
├── cmd/                    # Service entry points
│   ├── telnet-gateway/     # Telnet server
│   ├── auth-service/       # Authentication service
│   ├── game-engine/        # Game logic service (planned)
│   ├── world-builder/      # World building service (planned)
│   └── admin-console/      # Admin REPL (planned)
├── internal/               # Private application code
│   ├── auth/              # Authentication logic
│   ├── game/              # Core game systems (planned)
│   ├── telnet/            # Telnet protocol handling
│   ├── ansi/              # ANSI escape code utilities
│   ├── scripting/         # Starlark integration (planned)
│   ├── database/          # Database abstractions
│   ├── events/            # Event system
│   └── discord/           # Discord bot integration (planned)
├── pkg/                   # Public library code
│   ├── models/            # Shared data models
│   ├── config/            # Configuration management
│   └── utils/             # Shared utilities
├── scripts/               # Development and deployment scripts
├── k8s/                   # Kubernetes manifests (planned)
├── docker/                # Docker configurations (planned)
└── docs/                  # Documentation
```

### Available Make Targets

```bash
make help              # Show all available targets
make build             # Build all services
make test              # Run tests
make dev-start         # Start development services
make dev-stop          # Stop development services
make run-telnet-gateway # Run telnet gateway locally
make run-auth-service  # Run auth service locally
```

### Environment Configuration

Copy `.env.example` to `.env` and customize:

```bash
# Server Configuration
TELNET_PORT=2323
HTTP_PORT=8080

# Database Configuration
POSTGRES_HOST=localhost
POSTGRES_PASSWORD=mud_password
REDIS_HOST=localhost
MONGODB_URI=mongodb://admin:admin_password@localhost:27017

# Authentication
AUTH_SECRET_KEY=your-secret-key-here
```

## 🧪 Testing

### Unit Tests
```bash
make test
```

### Integration Tests
```bash
make integration-test
```

### Load Testing
```bash
make load-test
```

## 📊 Monitoring

The development environment includes comprehensive monitoring:

- **Grafana**: http://localhost:3000 (admin/admin123)
- **Prometheus**: http://localhost:9090
- **pgAdmin**: http://localhost:8081 (admin@raceconditionkingdom.com/admin123)
- **Redis Commander**: http://localhost:8082
- **Mongo Express**: http://localhost:8083 (admin/admin123)

## 🚢 Deployment

### Docker

```bash
make docker-build
make docker-push
```

### Kubernetes

```bash
make k8s-deploy
```

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/amazing-feature`
3. Make your changes
4. Run tests: `make test`
5. Commit your changes: `git commit -m 'Add amazing feature'`
6. Push to the branch: `git push origin feature/amazing-feature`
7. Open a Pull Request

### Code Style

- Follow Go conventions and use `gofmt`
- Write tests for new functionality
- Update documentation as needed
- Use conventional commit messages

## 📋 Roadmap

### Phase 1: Core Infrastructure ✅
- [x] Authentication system with HTTPS
- [x] Telnet gateway with ANSI support
- [x] Database abstraction layer
- [x] Event system with Redis pub/sub
- [x] Basic project structure and tooling

### Phase 2: Basic Gameplay (In Progress)
- [ ] Room navigation system
- [ ] Inventory management
- [ ] Basic combat system
- [ ] Chat and communication
- [ ] Character creation

### Phase 3: Economy & Trading
- [ ] Shop system
- [ ] Auction house
- [ ] Player-to-player trading
- [ ] Banking system
- [ ] Item crafting

### Phase 4: World Building
- [ ] Starlark scripting engine
- [ ] World builder interface
- [ ] Content approval system
- [ ] Script sandboxing

### Phase 5: Advanced Features
- [ ] PvP system
- [ ] Guild system
- [ ] Time and weather cycles
- [ ] Advanced combat mechanics
- [ ] Mobile/web client

### Phase 6: Polish & Scale
- [ ] Performance optimization
- [ ] Advanced monitoring
- [ ] Load balancing
- [ ] Content management tools

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- Inspired by classic MUDs like DikuMUD, CircleMUD, and LPC-based systems
- Built with modern Go practices and cloud-native architecture
- Special thanks to the MUD development community for decades of innovation

## 📞 Support

- **Issues**: [GitHub Issues](https://github.com/tylerhardison/race-condition-kingdom/issues)
- **Discussions**: [GitHub Discussions](https://github.com/tylerhardison/race-condition-kingdom/discussions)
- **Discord**: Coming soon!

---

*"In a world of race conditions, only the Kingdom stands eternal."* 🏰