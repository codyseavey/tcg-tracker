# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

TCG Tracker is a trading card game collection management application supporting Pokemon and Magic: The Gathering cards. It features card scanning via mobile camera with OCR, price tracking with caching, and collection management.

## Architecture

```
tcg-tracker/
├── backend/          # Go REST API server
├── frontend/         # Vue.js 3 web application
└── mobile/           # Flutter mobile app with OCR (see mobile/CLAUDE.md for details)
```

## Tech Stack

- **Backend**: Go 1.24+, Gin framework, GORM, SQLite
- **Frontend**: Vue.js 3, Vite, Pinia, Vue Router, Tailwind CSS
- **Mobile**: Flutter, Google ML Kit (OCR), camera integration

## Build & Run Commands

### Backend
```bash
cd backend
go run cmd/server/main.go             # Start server
go build -o server cmd/server/main.go # Build binary
go test ./...                         # Run all tests
go test -v ./internal/services/...    # Run specific package tests
go test -v -run TestParseOCRText ./internal/services/...  # Run single test by name
golangci-lint run                     # Run linter
```

Environment variables (see `backend/.env.example`):
- `DB_PATH` - SQLite database path
- `PORT` - Server port (default: 8080)
- `FRONTEND_DIST_PATH` - Path to built frontend (enables static serving)
- `POKEMON_DATA_DIR` - Directory for pokemon-tcg-data (auto-downloaded on first run)
- `JUSTTCG_API_KEY` - JustTCG API key for condition-based pricing (optional)
- `JUSTTCG_DAILY_LIMIT` - Daily API request limit (default: 100)

### Frontend
```bash
cd frontend
npm install                 # Install dependencies
npm run dev                 # Development server (port 5173)
npm run build               # Build for production (outputs to dist/)
```

### Mobile
```bash
cd mobile
flutter pub get             # Install dependencies
flutter run                 # Run on connected device/emulator
flutter test                # Run all tests
flutter analyze             # Run linter
flutter build apk           # Build Android APK
```

## Key Backend Services

| Service | File | Purpose |
|---------|------|---------|
| `PokemonHybridService` | `internal/services/pokemon_hybrid.go` | Pokemon card search with local data |
| `ScryfallService` | `internal/services/scryfall.go` | MTG card search via Scryfall API |
| `JustTCGService` | `internal/services/justtcg.go` | Condition-based pricing from JustTCG API |
| `PriceService` | `internal/services/price_service.go` | Unified price fetching with fallback chain |
| `TCGdexService` | `internal/services/tcgdex.go` | Pokemon card pricing fallback (TCGdex API) |
| `PriceWorker` | `internal/services/price_worker.go` | Background price updates (20 cards/batch hourly) |
| `OCRParser` | `internal/services/ocr_parser.go` | Parse OCR text to extract card details |
| `ServerOCRService` | `internal/services/server_ocr.go` | Server-side OCR using Tesseract (optional) |

## API Endpoints

Base URL: `http://localhost:8080/api`

### Cards
- `GET /cards/search?q=<query>&game=<pokemon|mtg>` - Search cards
- `GET /cards/:id` - Get card by ID
- `GET /cards/:id/prices` - Get condition-specific prices for a card
- `POST /cards/identify` - Identify card from OCR text (client-side OCR)
- `POST /cards/identify-image` - Identify card from uploaded image (server-side OCR)
- `GET /cards/ocr-status` - Check if server-side OCR is available
- `POST /cards/:id/refresh-price` - Refresh single card price

### Collection
- `GET /collection` - Get user's collection
- `POST /collection` - Add card to collection
- `PUT /collection/:id` - Update collection item
- `DELETE /collection/:id` - Remove from collection
- `GET /collection/stats` - Get collection statistics
- `POST /collection/refresh-prices` - Queue background price refresh

### Prices
- `GET /prices/status` - Get API quota status

### Health
- `GET /health` - Health check

## Data Flow

1. **Card Search**: User searches → Backend queries local Pokemon data or Scryfall API → Returns cards with cached prices
2. **Card Scanning**: Mobile captures image → ML Kit OCR extracts text → Backend parses OCR text → Matches card by name/set/number
3. **Price Updates**: Background worker runs hourly → Updates 20 cards per batch via JustTCG (with TCGdex/Scryfall fallback)

## Important Implementation Details

### Price Caching
- Condition-specific prices (NM, LP, MP, HP, DMG) are stored in `card_prices` table
- Base prices (NM only) kept in `cards` table for backward compatibility
- `PriceService` provides unified price fetching with fallback chain:
  1. Check database cache (fresh within 24 hours)
  2. Try JustTCG API (condition-specific pricing for Pokemon and MTG)
  3. Fallback to TCGdex (Pokemon) or Scryfall (MTG) for NM prices
  4. Return stale cached price if all else fails
- Background worker updates 20 cards per batch hourly (both Pokemon and MTG)
- Collection stats use condition-appropriate prices (e.g., LP card uses LP price)

### OCR Card Matching
The `OCRParser` extracts from scanned card images:
- Card name (first non-empty line, cleaned)
- Card number (e.g., "025/185" → "25", "TG17/TG30" → "TG17")
- Set code (e.g., "swsh4") - via direct detection, set name mapping, or total inference
- HP value (Pokemon)
- Foil indicators (V, VMAX, VSTAR, GX, EX, holo, full art, etc.)
- Rarity (Illustration Rare, Secret Rare, etc.)

Matching priority:
1. Exact match by set code + card number
2. Fuzzy match by name with ranking

### OCR Processing Options
Two OCR processing paths are available:
1. **Client-side OCR** (default): Mobile app uses Google ML Kit for OCR, sends extracted text to `/cards/identify`
2. **Server-side OCR** (optional): Upload image to `/cards/identify-image`, server uses Tesseract for OCR
   - Requires Tesseract installed on server (`tesseract` command available)
   - Supports file upload (multipart) or base64-encoded image in JSON body
   - Check availability via `/cards/ocr-status`

### Frontend Serving
The Go backend can serve the Vue.js frontend from `FRONTEND_DIST_PATH`:
- Static assets from `/assets`
- SPA fallback routes non-API paths to `index.html`

## Database

SQLite database with GORM models in `internal/models/`:
- `Card` - Card data with prices, images, metadata
- `CardPrice` - Condition-specific prices (NM, LP, MP, HP, DMG) for each card/foil combo
- `CollectionItem` - User's collection entries with quantity, condition

## Pokemon Data

Pokemon TCG data is stored at `$POKEMON_DATA_DIR/pokemon-tcg-data-master/`:
- Auto-downloaded from GitHub on first server startup
- Contains card JSON files organized by set (e.g., `cards/en/swsh4.json`)
