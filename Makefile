.PHONY: dev-start dev-app dev-frontend dev-stop dev-status dev-restart build up down clean

# === Development mode (run infrastructure in Docker, app + frontend locally) ===

dev-start:
	docker compose up -d postgres redis milvus minio minio-init
	@echo "Infrastructure started (postgres, redis, milvus, minio)"
	@echo "Now run: make dev-app (new terminal) and make dev-frontend (new terminal)"

dev-app:
	@echo "Starting API server (http://localhost:8081)..."
	DB_HOST=localhost REDIS_ADDR=localhost:6379 MILVUS_HOST=localhost MINIO_ENDPOINT=localhost:9000 go run ./cmd/api

dev-frontend:
	@echo "Starting frontend dev server (http://localhost:5173)..."
	cd web && npm run dev

dev-stop:
	docker compose down
	@echo "Infrastructure stopped"

dev-status:
	docker compose ps

dev-restart: dev-stop dev-start
	@echo "Infrastructure restarted"

# === Docker mode (everything in containers) ===

build:
	docker compose build

up:
	docker compose up -d
	@echo "All services started"
	@echo "  Frontend: http://localhost"
	@echo "  API:      http://localhost:8081"

# === Cleanup ===

clean:
	docker compose down -v
	rm -rf tmp/
	@echo "Cleaned up containers, volumes, and temp files"
