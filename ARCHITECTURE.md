# Race Condition Kingdom - MUD Architecture Plan

## 🎯 Project Overview

Race Condition Kingdom is a modern MUD (Multi-User Dungeon) written in Go, built around nostalgia for telnet-based gameplay but modernized with out-of-band HTTPS authentication, extensibility, and balance-focused gameplay design.

## 🏗️ High-Level System Architecture

```mermaid
graph TB
    subgraph "Client Layer"
        TC[Telnet Clients<br/>iTerm, Terminal, etc.]
        WC[Web Telnet Client<br/>Browser-based]
    end
    
    subgraph "Load Balancer"
        LB[Kubernetes Ingress<br/>NGINX/Traefik]
    end
    
    subgraph "Application Layer"
        subgraph "Telnet Gateway Pods"
            TG1[Telnet Gateway 1]
            TG2[Telnet Gateway 2]
            TGN[Telnet Gateway N]
        end
        
        subgraph "Game Engine Pods"
            GE1[Game Engine 1]
            GE2[Game Engine 2]
            GEN[Game Engine N]
        end
        
        subgraph "Auth Service Pods"
            AS1[Auth Service 1]
            AS2[Auth Service 2]
        end
        
        subgraph "World Builder Pods"
            WB1[World Builder 1]
            WB2[World Builder 2]
        end
        
        subgraph "Admin Console"
            AC[Admin REPL/Console]
        end
    end
    
    subgraph "Message Queue"
        MQ[Redis Pub/Sub<br/>Game Events]
    end
    
    subgraph "Data Layer"
        subgraph "PostgreSQL Cluster"
            PG[(PostgreSQL<br/>Users, Transactions<br/>World Data)]
        end
        
        subgraph "Redis Cluster"
            RD[(Redis<br/>Sessions, Game State<br/>Real-time Data)]
        end
        
        subgraph "MongoDB"
            MG[(MongoDB<br/>Logs, Analytics<br/>Builder Content)]
        end
    end
    
    subgraph "External Services"
        DC[Discord Bot<br/>Logging & Alerts]
        WH[Webhooks<br/>External Integrations]
    end
    
    TC --> LB
    WC --> LB
    LB --> TG1
    LB --> TG2
    LB --> TGN
    LB --> AS1
    LB --> AS2
    
    TG1 --> MQ
    TG2 --> MQ
    TGN --> MQ
    
    MQ --> GE1
    MQ --> GE2
    MQ --> GEN
    
    GE1 --> PG
    GE1 --> RD
    GE1 --> MG
    GE2 --> PG
    GE2 --> RD
    GE2 --> MG
    
    AS1 --> RD
    AS2 --> RD
    AS1 --> PG
    AS2 --> PG
    
    WB1 --> MG
    WB2 --> MG
    WB1 --> PG
    WB2 --> PG
    
    GE1 --> DC
    GE2 --> DC
    AC --> PG
    AC --> RD
    AC --> MG
```

## 🔧 Core Service Architecture

```mermaid
graph TB
    subgraph "Telnet Gateway Service"
        TGS[Connection Handler]
        TGS --> ANS[ANSI Processor]
        TGS --> CMD[Command Parser]
        TGS --> AUTH[Auth Flow Manager]
    end
    
    subgraph "Game Engine Service"
        GES[Game Loop Manager]
        GES --> WM[World Manager]
        GES --> PM[Player Manager]
        GES --> CM[Combat Manager]
        GES --> EM[Economy Manager]
        GES --> TM[Time Manager]
        
        WM --> RM[Room Manager]
        WM --> IM[Item Manager]
        WM --> NM[NPC Manager]
        
        PM --> INV[Inventory System]
        PM --> STAT[Stats System]
        PM --> PERM[Permissions System]
    end
    
    subgraph "Scripting Engine"
        SE[Starlark Runtime]
        SE --> SB[Script Sandbox]
        SE --> API[Script API Layer]
        SE --> VAL[Script Validator]
    end
    
    subgraph "World Builder Service"
        WBS[Builder Interface]
        WBS --> RE[Room Editor]
        WBS --> IE[Item Editor]
        WBS --> NE[NPC Editor]
        WBS --> SE2[Script Editor]
        WBS --> PREV[Preview System]
    end
```

## 🗄️ Database Schema Design

### PostgreSQL Schema (Critical Data)

```sql
-- Users and Authentication
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username VARCHAR(50) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    last_login TIMESTAMP,
    is_active BOOLEAN DEFAULT true,
    permissions JSONB DEFAULT '{}'
);

-- Characters
CREATE TABLE characters (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(50) UNIQUE NOT NULL,
    level INTEGER DEFAULT 1,
    experience BIGINT DEFAULT 0,
    health INTEGER DEFAULT 100,
    max_health INTEGER DEFAULT 100,
    stamina INTEGER DEFAULT 100,
    max_stamina INTEGER DEFAULT 100,
    gold BIGINT DEFAULT 0,
    alignment_lawful INTEGER DEFAULT 0, -- -100 to 100
    alignment_good INTEGER DEFAULT 0,   -- -100 to 100
    current_room_id UUID,
    last_rest TIMESTAMP DEFAULT NOW(),
    is_sleeping BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT NOW()
);

-- World Structure
CREATE TABLE rooms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    description TEXT,
    short_description VARCHAR(255),
    room_type VARCHAR(50) DEFAULT 'normal', -- safe, pvp, shop, etc.
    exits JSONB DEFAULT '{}', -- {"north": "room_id", "south": "room_id"}
    flags JSONB DEFAULT '{}', -- room properties
    script_id UUID,
    created_by UUID REFERENCES users(id),
    approved_by UUID REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    approved_at TIMESTAMP
);

-- Items and Economy
CREATE TABLE items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    description TEXT,
    item_type VARCHAR(50) NOT NULL, -- weapon, armor, consumable, etc.
    weight DECIMAL(10,2) DEFAULT 0,
    value BIGINT DEFAULT 0,
    properties JSONB DEFAULT '{}',
    alignment_required JSONB DEFAULT '{}',
    script_id UUID,
    created_by UUID REFERENCES users(id),
    approved_by UUID REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW()
);

-- Player Inventory
CREATE TABLE inventory (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    character_id UUID REFERENCES characters(id) ON DELETE CASCADE,
    item_id UUID REFERENCES items(id),
    quantity INTEGER DEFAULT 1,
    equipped BOOLEAN DEFAULT false,
    acquired_at TIMESTAMP DEFAULT NOW()
);

-- Transactions (Economy)
CREATE TABLE transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    from_character_id UUID REFERENCES characters(id),
    to_character_id UUID REFERENCES characters(id),
    item_id UUID REFERENCES items(id),
    gold_amount BIGINT DEFAULT 0,
    transaction_type VARCHAR(50) NOT NULL, -- sale, gift, auction, etc.
    created_at TIMESTAMP DEFAULT NOW()
);

-- Auction House
CREATE TABLE auctions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    seller_id UUID REFERENCES characters(id),
    item_id UUID REFERENCES items(id),
    starting_bid BIGINT NOT NULL,
    current_bid BIGINT,
    current_bidder_id UUID REFERENCES characters(id),
    ends_at TIMESTAMP NOT NULL,
    status VARCHAR(20) DEFAULT 'active', -- active, completed, cancelled
    created_at TIMESTAMP DEFAULT NOW()
);
```

### Redis Data Structures (Real-time Data)

```
# Session Management
session:{session_id} -> {
    "character_id": "uuid",
    "user_id": "uuid", 
    "connection_id": "string",
    "last_activity": timestamp,
    "current_room": "uuid",
    "auth_token": "string"
}

# Authentication Tokens
auth_token:{token} -> {
    "session_id": "string",
    "expires_at": timestamp,
    "used": boolean
}

# Game State
room:{room_id}:players -> Set of character_ids
character:{character_id}:location -> room_id
character:{character_id}:online -> boolean

# Real-time Events
game_events -> Pub/Sub channel for game events
room:{room_id}:events -> Pub/Sub channel for room-specific events

# World Builder Sessions
builder:{user_id}:session -> {
    "mode": "building",
    "sandbox_world": "world_id",
    "active_since": timestamp
}
```

## 🔐 Authentication Flow

```mermaid
sequenceDiagram
    participant Client as Telnet Client
    participant TG as Telnet Gateway
    participant Auth as Auth Service
    participant Redis as Redis
    participant Web as Web Browser
    participant PG as PostgreSQL
    
    Client->>TG: Connect via telnet
    TG->>Client: Welcome! Enter username:
    Client->>TG: username
    
    TG->>Auth: Request auth token for username
    Auth->>Redis: Store temp token with session mapping
    Auth->>TG: Return auth URL with token
    
    TG->>Client: Please visit: https://mud.game/auth?token=xyz123
    TG->>Client: Waiting for authentication...
    
    Web->>Auth: GET /auth?token=xyz123
    Auth->>Redis: Validate token, get session info
    Auth->>Web: Show login form
    
    Web->>Auth: POST credentials
    Auth->>PG: Validate user credentials
    Auth->>Redis: Mark token as used, create session
    Auth->>Web: Authentication successful
    
    TG->>Redis: Poll for session validation
    Redis->>TG: Session validated
    TG->>Client: Welcome back, [character]!
    TG->>Client: [Game starts]
```

## 🎮 Game Loop Architecture

```mermaid
graph TB
    subgraph "Game Loop (60 FPS)"
        GL[Game Loop Manager]
        GL --> TU[Time Update]
        GL --> EU[Event Update]
        GL --> PU[Player Update]
        GL --> WU[World Update]
        GL --> OU[Output Update]
    end
    
    subgraph "Event System"
        ES[Event Dispatcher]
        ES --> CE[Combat Events]
        ES --> ME[Movement Events]
        ES --> EE[Economy Events]
        ES --> SE[Script Events]
    end
    
    subgraph "Command Processing"
        CP[Command Processor]
        CP --> MV[Movement Commands]
        CP --> CB[Combat Commands]
        CP --> SO[Social Commands]
        CP --> EC[Economy Commands]
        CP --> AD[Admin Commands]
    end
    
    GL --> ES
    ES --> CP
    CP --> GL
```

## 🛡️ Security & Sandboxing

```mermaid
graph TB
    subgraph "Starlark Sandbox"
        SS[Starlark Runtime]
        SS --> API[Restricted API Layer]
        SS --> RL[Resource Limits]
        SS --> TL[Time Limits]
        SS --> ML[Memory Limits]
    end
    
    subgraph "Script API"
        API --> RM[Room Methods]
        API --> IM[Item Methods]
        API --> PM[Player Methods]
        API --> EM[Event Methods]
        API --> UT[Utility Methods]
    end
    
    subgraph "Validation Layer"
        VL[Script Validator]
        VL --> SYN[Syntax Check]
        VL --> SEC[Security Scan]
        VL --> RES[Resource Check]
        VL --> APP[Admin Approval]
    end
    
    SS --> VL
```

## 📊 Monitoring & Logging

```mermaid
graph TB
    subgraph "Logging Pipeline"
        LP[Log Aggregator]
        LP --> GL[Game Logs]
        LP --> AL[Admin Logs]
        LP --> EL[Error Logs]
        LP --> PL[Performance Logs]
    end
    
    subgraph "Discord Integration"
        DI[Discord Bot]
        DI --> AC[Admin Channel]
        DI --> EC[Error Channel]
        DI --> LC[Log Channel]
        DI --> RC[Rollback Channel]
    end
    
    subgraph "Metrics"
        MT[Metrics Collector]
        MT --> PC[Player Count]
        MT --> PT[Performance Metrics]
        MT --> ET[Error Tracking]
        MT --> UT[Usage Tracking]
    end
    
    LP --> DI
    MT --> DI
```

## 🔧 Key Go Packages & Structure

```
race-condition-kingdom/
├── cmd/
│   ├── telnet-gateway/     # Telnet server entry point
│   ├── game-engine/        # Game logic service
│   ├── auth-service/       # Authentication service
│   ├── world-builder/      # World building service
│   └── admin-console/      # Admin REPL
├── internal/
│   ├── auth/              # Authentication logic
│   ├── game/              # Core game systems
│   │   ├── world/         # World management
│   │   ├── player/        # Player management
│   │   ├── combat/        # Combat system
│   │   ├── economy/       # Economy system
│   │   └── time/          # Time/season system
│   ├── telnet/            # Telnet protocol handling
│   ├── ansi/              # ANSI escape code utilities
│   ├── scripting/         # Starlark integration
│   ├── database/          # Database abstractions
│   ├── events/            # Event system
│   └── discord/           # Discord bot integration
├── pkg/
│   ├── models/            # Shared data models
│   ├── config/            # Configuration management
│   └── utils/             # Shared utilities
├── scripts/               # Deployment scripts
├── k8s/                   # Kubernetes manifests
└── docs/                  # Documentation
```

## 🚀 Deployment Strategy

The system will be deployed on Kubernetes with:

- **Horizontal Pod Autoscaling** for game engine and telnet gateway pods
- **StatefulSets** for database clusters
- **ConfigMaps** and **Secrets** for configuration management
- **Persistent Volumes** for database storage
- **Service Mesh** (Istio) for inter-service communication
- **Monitoring** with Prometheus and Grafana

## 🎯 Implementation Phases

1. **Phase 1**: Core infrastructure (auth, telnet gateway, basic game loop)
2. **Phase 2**: World system and basic gameplay (movement, rooms, items)
3. **Phase 3**: Combat and economy systems
4. **Phase 4**: Starlark scripting and world builder
5. **Phase 5**: Advanced features (PvP, auctions, time cycles)
6. **Phase 6**: Polish and optimization

## 🎮 Core Gameplay Features

### 🔐 Authentication & Connectivity
- Players connect over telnet (ANSI-supported clients like iTerm, Apple Terminal, etc.)
- After entering a username, they are provided a one-time HTTPS link to authenticate
- The MUD pauses until the authentication is confirmed via API or queue system
- A web-based telnet client (no GUI) will also be available
- Session management and authentication should be secure, user-friendly, and session-limited

### 🧱 Architecture Requirements
- The backend is intended to run in Kubernetes with stateless containers
- All data (users, rooms, items, inventory, scripting, etc.) will be stored in a hybrid database approach
- Each container must have independent access to the shared database
- Changes to in-game data or scripts will be logged to Discord via a bot for transparency and moderation, including rollback capability

### 🎮 Core Gameplay Systems
- The world begins with a town square hub, with access to:
  - Banks (two, with different rules/benefits)
  - Shops (magic, weapons, armor, trinkets)
  - Inns (player rest, guild inventory access)
  - Auction House (player-to-player sales)
- PvP is territory-specific; town square is always a safe zone
- Players lose stamina/HP if they don't rest at intervals
- Disconnecting marks the player as "asleep" and immune from harm in PvP zones
- Time-of-day and seasonal cycles affect gameplay and encounters

### ⚔️ Death, Recovery, & Balance
- Player death does not delete characters:
  - They are teleported to the town square infirmary
  - Recovery could require in-game currency or a wait period
  - Recovery may joke about the American healthcare system (e.g., "bill required to leave")
- Magic and combat systems include balancing tradeoffs:
  - Powerful weapons exact a toll on the user (HP, stamina, etc.)
  - Items are classified by alignment (Lawful/Chaotic, Good/Evil)
  - Overpowered items or suspicious player behavior should trigger admin alerts

### 🧠 Extensibility
- Select users can enter World Building Mode, isolated from the live game
- This mode enables:
  - Creating new rooms, NPCs, items, dungeons
  - Full scripting of logic (AI, combat, effects, triggers)
  - Scripts will use Starlark for secure sandboxing
- Builders are sandboxed, and new content is locked until admin-approved
- Permissions determine whether custom items can be moved between "territories"
- Builders cannot affect the live world while building

### 💰 Economy
- Player economy supports:
  - Item gifting, sales, and auctions
  - A player auction house for open bidding
- All items have weight; players have an encumbrance limit
- Players can rent rooms at the inn for "infinite" storage — a humorous nod to Hitchhiker's Guide

## 🏆 Architecture Benefits

This architecture provides:
- ✅ Stateless, horizontally scalable services
- ✅ Secure authentication with Redis-backed tokens
- ✅ Hybrid database approach for optimal performance
- ✅ Sandboxed Starlark scripting for world building
- ✅ Real-time event system for multiplayer interactions
- ✅ Discord integration for admin oversight
- ✅ Kubernetes-native deployment
- ✅ Comprehensive monitoring and logging
- ✅ Extensible plugin system for world building
- ✅ Balance-focused gameplay design