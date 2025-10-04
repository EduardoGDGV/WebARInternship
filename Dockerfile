# Build frontend
FROM node:20 AS frontend-build
WORKDIR /nakama/frontend
COPY frontend/package*.json ./
RUN npm install
COPY frontend/ ./
RUN npm run build

# Build Nakama Go plugin
FROM heroiclabs/nakama-pluginbuilder:3.30.0 AS builder
WORKDIR /nakama
COPY backend/ ./backend/
COPY go.mod go.sum ./
COPY config/ ./config/
RUN cd backend && go build --trimpath --buildmode=plugin -o ../data/modules/backend.so

# Final Nakama image
FROM heroiclabs/nakama:3.30.0
COPY --from=builder /nakama/data/modules/ /nakama/data/modules/
COPY --from=builder /nakama/config/ /nakama/data/
COPY --from=frontend-build /nakama/frontend/dist /nakama/data/frontend
