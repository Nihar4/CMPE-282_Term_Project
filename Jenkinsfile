// ============================================================
// Enterprise Knowledge Portal - Jenkins CI/CD Pipeline
// Builds, tests, pushes Docker images, deploys to GKE
// ============================================================

pipeline {
  agent any

  environment {
    GCP_PROJECT_ID    = credentials('GCP_PROJECT_ID')         // Jenkins credential
    GCP_REGION        = 'us-central1'
    GKE_CLUSTER       = 'enterprise-portal-cluster'
    DOCKER_REGISTRY   = "gcr.io/${GCP_PROJECT_ID}"
    KUBECONFIG        = '/tmp/kubeconfig'
    SERVICES          = 'api-gateway auth-service data-service file-service ai-service analytics-service'
    NAMESPACE         = 'enterprise-portal'
  }

  parameters {
    booleanParam(name: 'DEPLOY_INFRA',    defaultValue: false, description: 'Run Terraform apply (GCP infra)?')
    booleanParam(name: 'RUN_MIGRATIONS',  defaultValue: false, description: 'Run database migrations?')
    booleanParam(name: 'DEPLOY_FRONTEND', defaultValue: true,  description: 'Build & deploy frontend?')
    string(name: 'TARGET_ENV', defaultValue: 'production', description: 'Target environment (production|staging)')
  }

  stages {

    // ── 1. Checkout ────────────────────────────────────────────────────────────
    stage('Checkout') {
      steps {
        checkout scm
        sh 'git log --oneline -5'
      }
    }

    // ── 2. Setup & Auth ────────────────────────────────────────────────────────
    stage('GCP Auth') {
      steps {
        withCredentials([file(credentialsId: 'GCP_SERVICE_ACCOUNT_KEY', variable: 'GCP_KEY')]) {
          sh '''
            gcloud auth activate-service-account --key-file=$GCP_KEY
            gcloud config set project $GCP_PROJECT_ID
            gcloud auth configure-docker gcr.io --quiet
          '''
        }
      }
    }

    // ── 3. Infrastructure (optional Terraform) ─────────────────────────────────
    stage('Terraform Apply') {
      when { expression { params.DEPLOY_INFRA == true } }
      steps {
        withCredentials([string(credentialsId: 'TF_DB_PASSWORD', variable: 'DB_PASSWORD')]) {
          dir('infrastructure/terraform') {
            sh '''
              terraform init
              terraform plan -var="db_password=$DB_PASSWORD" -out=tfplan
              terraform apply -auto-approve tfplan
            '''
          }
        }
      }
    }

    // ── 4. Go Tests ────────────────────────────────────────────────────────────
    stage('Test Backend') {
      parallel {
        stage('Test api-gateway')        { steps { dir('backend/api-gateway')        { sh 'go test ./...' } } }
        stage('Test auth-service')       { steps { dir('backend/auth-service')       { sh 'go test ./...' } } }
        stage('Test data-service')       { steps { dir('backend/data-service')       { sh 'go test ./...' } } }
        stage('Test file-service')       { steps { dir('backend/file-service')       { sh 'go test ./...' } } }
        stage('Test ai-service')         { steps { dir('backend/ai-service')         { sh 'go test ./...' } } }
        stage('Test analytics-service')  { steps { dir('backend/analytics-service')  { sh 'go test ./...' } } }
      }
    }

    // ── 5. Build Docker Images ─────────────────────────────────────────────────
    stage('Build Images') {
      steps {
        script {
          def commitSha = sh(script: 'git rev-parse --short HEAD', returnStdout: true).trim()
          env.IMAGE_TAG = "${commitSha}-${BUILD_NUMBER}"

          // Build backend services in parallel
          def buildTasks = [:]
          ['api-gateway', 'auth-service', 'data-service', 'file-service', 'ai-service', 'analytics-service'].each { svc ->
            buildTasks["Build ${svc}"] = {
              sh """
                docker build -t ${DOCKER_REGISTRY}/enterprise-portal-${svc}:${IMAGE_TAG} \
                             -t ${DOCKER_REGISTRY}/enterprise-portal-${svc}:latest \
                             backend/${svc}
              """
            }
          }

          if (params.DEPLOY_FRONTEND) {
            buildTasks['Build frontend'] = {
              sh """
                docker build \
                  --build-arg REACT_APP_API_URL=https://portal.yourdomain.com \
                  --build-arg REACT_APP_AUTH0_DOMAIN=dev-xbnsordr5elttyug.us.auth0.com \
                  --build-arg REACT_APP_AUTH0_CLIENT_ID=x8pFzWFtyYCBXrr6U2NkxABroQqqMvxM \
                  -t ${DOCKER_REGISTRY}/enterprise-portal-frontend:${IMAGE_TAG} \
                  -t ${DOCKER_REGISTRY}/enterprise-portal-frontend:latest \
                  frontend
              """
            }
          }

          parallel buildTasks
        }
      }
    }

    // ── 6. Security Scan ────────────────────────────────────────────────────────
    stage('Security Scan') {
      steps {
        script {
          // Trivy vulnerability scan on each image
          ['api-gateway', 'auth-service', 'data-service', 'ai-service'].each { svc ->
            sh "trivy image --exit-code 0 --severity HIGH,CRITICAL ${DOCKER_REGISTRY}/enterprise-portal-${svc}:${env.IMAGE_TAG} || true"
          }
        }
      }
    }

    // ── 7. Push Images ─────────────────────────────────────────────────────────
    stage('Push Images') {
      steps {
        script {
          def pushTasks = [:]
          def images = ['api-gateway', 'auth-service', 'data-service', 'file-service', 'ai-service', 'analytics-service']
          if (params.DEPLOY_FRONTEND) images.add('frontend')

          images.each { svc ->
            pushTasks["Push ${svc}"] = {
              sh """
                docker push ${DOCKER_REGISTRY}/enterprise-portal-${svc}:${IMAGE_TAG}
                docker push ${DOCKER_REGISTRY}/enterprise-portal-${svc}:latest
              """
            }
          }
          parallel pushTasks
        }
      }
    }

    // ── 8. Get GKE Credentials ─────────────────────────────────────────────────
    stage('Get GKE Credentials') {
      steps {
        sh """
          gcloud container clusters get-credentials ${GKE_CLUSTER} \
            --region ${GCP_REGION} \
            --project ${GCP_PROJECT_ID}
        """
      }
    }

    // ── 9. Database Migrations ─────────────────────────────────────────────────
    stage('DB Migrations') {
      when { expression { params.RUN_MIGRATIONS == true } }
      steps {
        sh """
          kubectl run migration-job-\$(date +%s) \
            --image=postgres:15-alpine \
            --restart=Never \
            --namespace=${NAMESPACE} \
            --env="PGPASSWORD=\$(kubectl get secret portal-secrets -n ${NAMESPACE} -o jsonpath='{.data.DB_PASSWORD}' | base64 -d)" \
            --command -- psql \
              -h portal-postgres \
              -U portal_user \
              -d enterprise_portal \
              -f /migrations/001_init.sql
        """
      }
    }

    // ── 10. Deploy to GKE ─────────────────────────────────────────────────────
    stage('Deploy to GKE') {
      steps {
        script {
          // Update image tags in manifests and apply
          def services = ['api-gateway', 'auth-service', 'data-service', 'file-service', 'ai-service', 'analytics-service']
          if (params.DEPLOY_FRONTEND) services.add('frontend')

          services.each { svc ->
            sh """
              kubectl set image deployment/${svc} ${svc}=${DOCKER_REGISTRY}/enterprise-portal-${svc}:${env.IMAGE_TAG} \
                -n ${NAMESPACE} --record || \
              kubectl apply -f infrastructure/k8s/ -n ${NAMESPACE}
            """
          }

          // Apply namespace, configmap, services, ingress
          sh "kubectl apply -f infrastructure/k8s/namespace.yaml"
          sh "kubectl apply -f infrastructure/k8s/configmap.yaml"
          sh "kubectl apply -f infrastructure/k8s/services.yaml"
          sh "kubectl apply -f infrastructure/k8s/ingress.yaml"
        }
      }
    }

    // ── 11. Verify Deployment ──────────────────────────────────────────────────
    stage('Verify Deployment') {
      steps {
        script {
          ['api-gateway', 'auth-service', 'data-service', 'file-service', 'ai-service', 'analytics-service'].each { svc ->
            sh "kubectl rollout status deployment/${svc} -n ${NAMESPACE} --timeout=120s"
          }
          sh "kubectl get pods -n ${NAMESPACE}"
        }
      }
    }

    // ── 12. Health Check ────────────────────────────────────────────────────────
    stage('Health Check') {
      steps {
        script {
          sleep(15)
          def pods = sh(script: "kubectl get pods -n ${NAMESPACE} --field-selector=status.phase=Running --no-headers | wc -l", returnStdout: true).trim()
          echo "Running pods: ${pods}"
          if (pods.toInteger() < 5) {
            error("Not enough pods running after deployment!")
          }
        }
      }
    }

  }  // end stages

  post {
    success {
      echo "✅ Deployment successful! Image tag: ${env.IMAGE_TAG}"
      // Add Slack/email notification here
    }
    failure {
      echo "❌ Pipeline failed. Rolling back..."
      sh """
        for svc in api-gateway auth-service data-service ai-service analytics-service; do
          kubectl rollout undo deployment/\$svc -n ${NAMESPACE} || true
        done
      """
    }
    always {
      // Clean up local Docker images to save disk
      sh "docker system prune -f --filter 'until=24h' || true"
      cleanWs()
    }
  }
}
