# Donetick Fork - tomcer

## Účel Forku

Tento fork je určen pro:
- Custom úpravy pro deployment na tomcer infrastruktuře
- Publikaci do GitHub Container Registry (GHCR) pro snadnější integraci
- Testování a opravy bugů před možným upstream mergem
- Vývoj aplikace pro tablet pro děti na správu úkolů

## Upstream

- **Původní repository:** https://github.com/donetick/donetick
- **Fork:** https://github.com/tomcer/donetick
- **Frontend fork:** https://github.com/tomcer/donetic-frontend

## Hlavní Změny

### GitHub Actions Workflow
- Změna publikace z Docker Hub na GitHub Container Registry
- Image path: `ghcr.io/tomcer/donetick` místo `donetick/donetick`
- Reference na forknutý frontend repository `tomcer/donetic-frontend`
- Přidáno `packages: write` permission pro GHCR
- Aktualizace action verzí pro lepší performance a cache

### Budoucí Plánované Změny
- Custom úpravy pro dětskou aplikaci
- Možné lokalizační úpravy pro češtinu
- Optimalizace pro tablet Samsung GT-N8000

## Sync s Upstream

Pro synchronizaci s původním repository:

```bash
# Fetch upstream změny
git fetch upstream

# Zobrazit rozdíly
git diff upstream/main..main

# Mergovat upstream změny
git checkout main
git merge upstream/main

# Vyřešit případné konflikty a pushnout
git push origin main
```

## Image Lokace

### GitHub Container Registry (GHCR)
- **Latest:** `ghcr.io/tomcer/donetick:latest`
- **Tagged:** `ghcr.io/tomcer/donetick:v0.1.64`
- **Develop:** `ghcr.io/tomcer/donetick:develop` (plánováno)

### Použití

```bash
# Pull image
docker pull ghcr.io/tomcer/donetick:latest

# Run container
docker run -d \
  --name donetick-core \
  -p 2021:2021 \
  -v ./data:/usr/src/app/data \
  -e DT_ENV=selfhosted \
  ghcr.io/tomcer/donetick:latest
```

## Produkční Deployment

Současný produkční backend běží na:
- **URL:** https://donetick-litice.dck-www.mksol.cz/
- **Port:** 2021
- **Database:** SQLite

## Contrib back to Upstream

Pokud vytvořím vylepšení, která mohou prospět komunitě:
1. Vytvořit branch z `upstream/main`
2. Aplikovat změny
3. Otestovat
4. Submitnout Pull Request do `donetick/donetick`

## Kontakt

- **Maintainer:** @tomcer
- **Issues:** https://github.com/tomcer/donetick/issues
