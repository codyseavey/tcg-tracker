# Combined Frontend + Backend Dockerfile
# Builds Vue.js frontend and Go backend into a single image

# Stage 1: Build frontend
FROM node:20-alpine AS frontend-builder

WORKDIR /frontend

COPY frontend/package*.json ./
RUN npm ci

COPY frontend/ .
RUN npm run build

# Stage 2: Build backend
FROM golang:1.24-alpine AS backend-builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache gcc musl-dev

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ .
RUN CGO_ENABLED=1 GOOS=linux go build -o server ./cmd/server/main.go

# Stage 3: Production image
FROM alpine:3.19

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ca-certificates sqlite-libs

# Copy backend binary
COPY --from=backend-builder /app/server .

# Copy frontend build
COPY --from=frontend-builder /frontend/dist ./frontend/dist

# Create data directory
RUN mkdir -p /app/data

# Environment variables
ENV PORT=8080
ENV DB_PATH=/app/data/tcg.db
ENV FRONTEND_DIST_PATH=/app/frontend/dist

EXPOSE 8080

CMD ["./server"]
