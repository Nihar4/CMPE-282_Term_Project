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
	@echo "  make k8s-deploy  - Deploy all to GKE"

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

# GCP / Kubernetes
gcp-auth:
	gcloud auth application-default login

gke-get-creds:
	gcloud container clusters get-credentials enterprise-portal-cluster \
		--region us-central1 \
		--project $(GCP_PROJECT_ID)

infra-init:
	cd infrastructure/terraform && terraform init

infra-plan:
	cd infrastructure/terraform && terraform plan

infra-up:
	cd infrastructure/terraform && terraform apply -auto-approve

infra-down:
	cd infrastructure/terraform && terraform destroy -auto-approve

k8s-deploy:
	kubectl apply -f infrastructure/k8s/namespace.yaml
	kubectl apply -f infrastructure/k8s/configmap.yaml
	kubectl apply -f infrastructure/k8s/
	kubectl rollout status deployment -n enterprise-portal

k8s-status:
	kubectl get all -n enterprise-portal

k8s-logs:
	kubectl logs -n enterprise-portal -l app=api-gateway -f

# Docker build & push to GCR
docker-push:
	@GCP_PROJECT_ID=$$(grep GCP_PROJECT_ID .env | cut -d= -f2); \
	for svc in $(SERVICES); do \
		docker build -t gcr.io/$$GCP_PROJECT_ID/enterprise-portal-$$svc:latest backend/$$svc; \
		docker push gcr.io/$$GCP_PROJECT_ID/enterprise-portal-$$svc:latest; \
	done; \
	docker build -t gcr.io/$$GCP_PROJECT_ID/enterprise-portal-frontend:latest frontend; \
	docker push gcr.io/$$GCP_PROJECT_ID/enterprise-portal-frontend:latest
