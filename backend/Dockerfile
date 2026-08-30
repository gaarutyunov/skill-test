# Dockerfile for the Node.js School Management backend.
# Used by the go-service integration tests and local docker-compose stack so the
# Go microservice can consume the real API.
FROM node:22-bookworm

# Build tools for native modules (argon2, sqlite3) when no prebuilt is available.
RUN apt-get update \
    && apt-get install -y --no-install-recommends python3 make g++ \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY package.json ./
RUN npm install --omit=dev

COPY . .

ENV PORT=5007
EXPOSE 5007
CMD ["node", "src/server.js"]
