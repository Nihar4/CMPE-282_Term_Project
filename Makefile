.PHONY: all build up down logs ps clean test seed migrate help

DOCKER_COMPOSE = docker compose
SERVICES = api-gateway auth-service data-service file-service ai-service analytics-service parser-service

help:
	@echo "Enterprise Knowledge Portal - Available Commands:"
	@echo ""
	@echo "  make up          - Start all services (docker compose)"
	@echo "  make down        - Stop all services"
	@echo "  make build       - Build all Docker images"
	@echo "  make logs        - Tail logs from all services"
	@echo "  make ps          - Show running containers"
	@echo "  make seed        - Run database seed data"
	@echo "  make migrate     - Run database migrations"
	@echo "  make clean       - Remove containers, volumes, images"
	@echo "  make test        - Run all service tests"
	@echo "  make frontend    - Start React dev server only"
	@echo "  make infra-up    - Apply Terraform (GCP infra)"
	@echo "  make cloud-run-deploy - Deploy all services to Cloud Run"

up:
	$(DOCKER_COMPOSE) up -d
	@echo "Portal running at http://localhost:3000"
	@echo "API Gateway at http://localhost:8080"

down:
	$(DOCKER_COMPOSE) down

build:
	$(DOCKER_COMPOSE) build --parallel

logs:
	$(DOCKER_COMPOSE) logs -f

ps:
	$(DOCKER_COMPOSE) ps

migrate:
	$(DOCKER_COMPOSE) exec postgres psql -U portal_user -d enterprise_portal -f /docker-entrypoint-initdb.d/001_init.sql

seed:
	$(DOCKER_COMPOSE) exec postgres psql -U portal_user -d enterprise_portal -f /seeds/mock_data.sql
	$(DOCKER_COMPOSE) exec postgres psql -U portal_user -d enterprise_portal -f /seeds/002_testdb_sample.sql
	@echo "Seed complete: portal mock data + test_db (200 employees)"

clean:
	$(DOCKER_COMPOSE) down -v --remove-orphans
	docker rmi $$(docker images -q --filter "reference=enterprise-portal*") 2>/dev/null || true

test:
	@for svc in $(SERVICES); do \
		echo "Testing $$svc..."; \
		cd backend/$$svc && go test ./... && cd ../..; \
	done

frontend:
	cd frontend && npm start

# GCP / Serverless
gcp-auth:
	gcloud auth application-default login

infra-init:
	cd infrastructure/terraform && terraform init

infra-plan:
	cd infrastructure/terraform && terraform plan

infra-up:
	cd infrastructure/terraform && terraform apply -auto-approve

infra-down:
	cd infrastructure/terraform && terraform destroy -auto-approve

cloud-run-deploy:
	gcloud builds submit --config cloudbuild-serverless.yaml \
		--substitutions="_PROJECT_ID=$${GCP_PROJECT_ID:-enterprise-portal-48689},_REGION=$${GCP_REGION:-us-central1},_KAFKA_BROKERS=$${KAFKA_BROKERS:-REPLACE_WITH_MANAGED_KAFKA_BOOTSTRAP:9092},_REACT_APP_API_URL=$${REACT_APP_API_URL:-http://localhost:8080},_OKTA_ISSUER=$${OKTA_ISSUER:-https://trial-5413467.okta.com/oauth2/default},_OKTA_CLIENT_ID=$${OKTA_CLIENT_ID:-0oa12cfmwjeBVrl0I698},_OKTA_REDIRECT_URI=$${OKTA_REDIRECT_URI:-http://localhost:3000/authorization-code/callback},_OKTA_LOGOUT_REDIRECT_URI=$${OKTA_LOGOUT_REDIRECT_URI:-http://localhost:3000},_IMAGE_TAG=$$(git rev-parse --short HEAD 2>/dev/null || echo manual)"

cloud-run-status:
	gcloud run services list --region $${GCP_REGION:-us-central1}

# Docker build & push to Artifact Registry
docker-push:
	@GCP_PROJECT_ID=$$(grep GCP_PROJECT_ID .env | cut -d= -f2); \
	for svc in $(SERVICES); do \
		docker build -t us-central1-docker.pkg.dev/$$GCP_PROJECT_ID/enterprise-portal/$$svc:latest backend/$$svc; \
		docker push us-central1-docker.pkg.dev/$$GCP_PROJECT_ID/enterprise-portal/$$svc:latest; \
	done; \
	docker build -t us-central1-docker.pkg.dev/$$GCP_PROJECT_ID/enterprise-portal/frontend:latest frontend; \
	docker push us-central1-docker.pkg.dev/$$GCP_PROJECT_ID/enterprise-portal/frontend:latest
