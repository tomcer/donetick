# Deployment Guide - tomcer Fork

Tento guide pokrývá deployment forku donetick publikovaného do GitHub Container Registry (GHCR).

## Základní Docker Compose Setup

### docker-compose.yml

```yaml
version: '3.8'

services:
  donetick:
    image: ghcr.io/tomcer/donetick:latest
    container_name: donetick-core
    restart: unless-stopped

    volumes:
      # SQLite database - persistent storage
      - ./data:/usr/src/app/data
      # Configuration files
      - ./config:/config

    ports:
      - "2021:2021"

    environment:
      - DT_ENV=selfhosted
      - TZ=Europe/Prague
      # Optional: Custom SQLite path
      # - DT_SQLITE_PATH=/usr/src/app/data/donetick.db

    networks:
      - donetick-network

    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:2021/api/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s

    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"

networks:
  donetick-network:
    driver: bridge
```

### Spuštění

```bash
# Vytvořit potřebné adresáře
mkdir -p data config

# Spustit službu
docker-compose up -d

# Sledovat logy
docker-compose logs -f

# Kontrola zdraví
curl http://localhost:2021/api/health
```

## Migrace z Oficiálního Image

### Postup Migrace

```bash
# 1. Backup současných dat
cp -r data data.backup.$(date +%Y%m%d)
docker inspect donetick-core > donetick-config-backup.json

# 2. Stop současný container
docker stop donetick-core
docker rm donetick-core

# 3. Pull fork image
docker pull ghcr.io/tomcer/donetick:latest

# 4. Aktualizovat docker-compose.yml
# Změnit:
#   image: donetick/donetick:latest
# Na:
#   image: ghcr.io/tomcer/donetick:latest

# 5. Start s novým image (data persistují přes volumes)
docker-compose up -d

# 6. Verify
docker-compose logs -f
curl http://localhost:2021/api/health
```

### Rollback (pokud něco selže)

```bash
# Rychlý rollback na oficiální image
docker-compose down
sed -i 's|ghcr.io/tomcer/donetick|donetick/donetick|g' docker-compose.yml
docker-compose pull
docker-compose up -d

# Nebo restore backup
docker-compose down
rm -rf data
mv data.backup.YYYYMMDD data
docker-compose up -d
```

## Version Pinning

Pro produkci se doporučuje pinnovat konkrétní verzi místo `latest`:

```yaml
services:
  donetick:
    image: ghcr.io/tomcer/donetick:v0.1.64  # Konkrétní verze
```

**Výhody:**
- Předvídatelné updaty
- Snadnější rollback
- Lepší kontrola nad změnami

## Reverse Proxy Setup

### Nginx

```nginx
# /etc/nginx/sites-available/donetick

upstream donetick_backend {
    server localhost:2021;
    keepalive 32;
}

server {
    listen 443 ssl http2;
    server_name donetick.example.com;

    ssl_certificate /path/to/ssl/cert.pem;
    ssl_certificate_key /path/to/ssl/key.pem;

    location / {
        proxy_pass http://donetick_backend;
        proxy_http_version 1.1;

        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket support
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";

        # Timeouts
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }

    # Health check endpoint
    location /api/health {
        proxy_pass http://donetick_backend/api/health;
        access_log off;
    }
}

# HTTP to HTTPS redirect
server {
    listen 80;
    server_name donetick.example.com;
    return 301 https://$server_name$request_uri;
}
```

### Traefik (docker-compose)

```yaml
version: '3.8'

services:
  donetick:
    image: ghcr.io/tomcer/donetick:latest
    container_name: donetick-core
    restart: unless-stopped

    volumes:
      - ./data:/usr/src/app/data
      - ./config:/config

    environment:
      - DT_ENV=selfhosted
      - TZ=Europe/Prague

    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.donetick.rule=Host(`donetick.example.com`)"
      - "traefik.http.routers.donetick.entrypoints=websecure"
      - "traefik.http.routers.donetick.tls=true"
      - "traefik.http.routers.donetick.tls.certresolver=letsencrypt"
      - "traefik.http.services.donetick.loadbalancer.server.port=2021"

    networks:
      - traefik
      - donetick

networks:
  traefik:
    external: true
  donetick:
    driver: bridge
```

## Environment Variables

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `DT_ENV` | Environment mode | `selfhosted` | Yes |
| `DT_SQLITE_PATH` | SQLite database path | `/usr/src/app/data/donetick.db` | No |
| `TZ` | Timezone | `UTC` | No |

## Data Persistence

### Adresářová Struktura

```
data/
├── donetick.db          # SQLite database
├── donetick.db-shm      # Shared memory file
└── donetick.db-wal      # Write-ahead log

config/
└── selfhosted.yaml      # Configuration file (optional)
```

### Backup Strategy

```bash
#!/bin/bash
# backup.sh - Jednoduchý backup script

BACKUP_DIR="/backup/donetick"
DATE=$(date +%Y%m%d_%H%M%S)

# Create backup
mkdir -p "$BACKUP_DIR"
docker exec donetick-core sqlite3 /usr/src/app/data/donetick.db ".backup '/tmp/backup.db'"
docker cp donetick-core:/tmp/backup.db "$BACKUP_DIR/donetick_$DATE.db"

# Keep last 7 days of backups
find "$BACKUP_DIR" -name "donetick_*.db" -mtime +7 -delete

echo "Backup completed: $BACKUP_DIR/donetick_$DATE.db"
```

## Multi-Architecture Support

Image je buildován pro:
- `linux/amd64` - x86_64 servers (Intel/AMD)
- `linux/arm64` - ARM64 (Raspberry Pi 4/5, AWS Graviton)
- `linux/arm/v7` - ARMv7 (starší Raspberry Pi)

Docker automaticky stáhne správnou verzi pro vaši architekturu:

```bash
# Ověření architektury
docker run --rm ghcr.io/tomcer/donetick:latest uname -m
```

## Monitoring & Health Checks

### Health Check Endpoint

```bash
# Základní check
curl http://localhost:2021/api/health

# Podrobný check s timeout
curl -f -m 5 http://localhost:2021/api/health || echo "Health check failed"
```

### Docker Health Status

```bash
# Zobrazit health status
docker ps --format "table {{.Names}}\t{{.Status}}"

# Detail health check logs
docker inspect donetick-core | jq '.[0].State.Health'
```

## Troubleshooting

### Container se nespustí

```bash
# Zkontrolovat logy
docker logs donetick-core

# Zkontrolovat permissions
ls -la data/

# Zkontrolovat port binding
netstat -tulpn | grep 2021
```

### Database locked

```bash
# Ukončit všechny connections
docker-compose down

# Počkat 5 sekund
sleep 5

# Restart
docker-compose up -d
```

### High memory usage

```bash
# Přidat memory limit do docker-compose.yml
services:
  donetick:
    # ...
    deploy:
      resources:
        limits:
          memory: 512M
        reservations:
          memory: 256M
```

## Production Checklist

- [ ] Backup strategy implementována
- [ ] Health checks nakonfigurovány
- [ ] Reverse proxy setup (HTTPS)
- [ ] Firewall pravidla nastavena
- [ ] Monitoring nastaven
- [ ] Version pinning použit (ne `latest`)
- [ ] Log rotation nakonfigurován
- [ ] Data adresář má správná permissions
- [ ] Regular update schedule naplánován

## Další Zdroje

- [FORK_INFO.md](FORK_INFO.md) - Informace o forku
- [Upstream dokumentace](https://github.com/donetick/donetick)
- [Issues](https://github.com/tomcer/donetick/issues)

## Support

Pro problémy specifické pro tento fork:
- GitHub Issues: https://github.com/tomcer/donetick/issues

Pro obecné donetick problémy:
- Upstream Issues: https://github.com/donetick/donetick/issues
- Discord: https://discord.gg/6hSH6F33q7
- Reddit: https://www.reddit.com/r/donetick
