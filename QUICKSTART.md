# 🚀 Quick Start Guide

Get Race Condition Kingdom running in 5 minutes!

## Prerequisites

- Go 1.21+ installed
- Docker and Docker Compose installed
- Terminal access

## Step 1: Clone and Setup

```bash
git clone https://github.com/mchineboy/race-condition-kingdom.git
cd race-condition-kingdom
```

## Step 2: Start Database Services

```bash
# Start just the essential services (PostgreSQL, Redis, MongoDB)
docker-compose -f docker-compose.simple.yml up -d

# Wait for services to be ready (about 30 seconds)
docker-compose -f docker-compose.simple.yml ps
```

## Step 3: Build the Project

```bash
# Download dependencies and build
go mod tidy
make build
```

## Step 4: Start the Services

Open two terminal windows:

**Terminal 1 - Auth Service:**
```bash
make run-auth-service
```

**Terminal 2 - Telnet Gateway:**
```bash
make run-telnet-gateway
```

## Step 5: Connect and Play!

```bash
# In a third terminal
telnet localhost 2323
```

### First Login

1. Enter username: `admin`
2. Visit the authentication URL shown
3. Login with:
   - Username: `admin`
   - Password: `admin123`
4. Return to telnet and type `check`
5. Start playing!

## Basic Commands

```
help          - Show all commands
look          - Look around
say hello     - Say something
who           - See who's online
stats         - View your character
inventory     - Check your items
quit          - Exit the game
```

## Troubleshooting

### Services won't start
```bash
# Check if ports are in use
lsof -i :2323  # Telnet port
lsof -i :8080  # HTTP port
lsof -i :5432  # PostgreSQL
lsof -i :6379  # Redis
```

### Database connection issues
```bash
# Check database status
docker-compose -f docker-compose.simple.yml logs postgres
docker-compose -f docker-compose.simple.yml logs redis
```

### Reset everything
```bash
# Stop all services
docker-compose -f docker-compose.simple.yml down -v
make clean

# Start fresh
docker-compose -f docker-compose.simple.yml up -d
make build
```

## What's Next?

- Explore the town square and different rooms
- Check out the shops and banks
- Try the auction house system
- Read the full [README.md](README.md) for advanced features
- Review [ARCHITECTURE.md](ARCHITECTURE.md) for technical details

## Development URLs

- **Telnet Game**: `telnet localhost 2323`
- **Auth Service**: `http://localhost:8080`
- **PostgreSQL**: `localhost:5432` (mud_user/mud_password)
- **Redis**: `localhost:6379`
- **MongoDB**: `localhost:27017` (admin/admin_password)

Happy adventuring! 🏰