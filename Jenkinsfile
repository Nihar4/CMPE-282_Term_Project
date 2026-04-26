// ============================================================
// Enterprise Knowledge Portal — Full CI/CD Pipeline
// GCP: Artifact Registry → GKE (us-central1)
// Trigger: GitHub webhook on push to main / PR merge
// ============================================================

pipeline {
  agent any

  environment {
    // GCP
    GCP_PROJECT_ID   = "enterprise-portal-48689"
    GCP_REGION       = "us-central1"
    GKE_CLUSTER      = "enterprise-portal-cluster"
    AR_REPO          = "us-central1-docker.pkg.dev/enterprise-portal-48689/enterprise-portal"
    KUBECONFIG       = "/tmp/kubeconfig-${BUILD_NUMBER}"
    NAMESPACE        = "enterprise-portal"

    // Image tag: git short sha + build number
    IMAGE_TAG        = ""   // set in Build stage
  }

  parameters {
    booleanParam(name: 'DEPLOY_INFRA',    defaultValue: false, description: 'Run terraform apply?')
    booleanParam(name: 'RUN_MIGRATIONS',  defaultValue: false, description: 'Run DB migrations?')
    booleanParam(name: 'DEPLOY_FRONTEND', defaultValue: true,  description: 'Build & deploy frontend?')
    booleanParam(name: 'SKIP_TESTS',      defaultValue: false, description: 'Skip Go tests (for hotfix)?')
    string(name: 'TARGET_ENV', defaultValue: 'production',     description: 'Target: production | staging')
    string(name: 'BRANCH',     defaultValue: 'main',           description: 'Branch to build')
  }

  options {
    buildDiscarder(logRotator(numToKeepStr: '20'))
    timeout(time: 45, unit: 'MINUTES')
    disableConcurrentBuilds()
    timestamps()
  }

  triggers {
    // GitHub webhook — configure in repo settings:
    // Payload URL: http://<JENKINS_IP>:8080/github-webhook/
    githubPush()
  }

  stages {

    // ── 1. Checkout ──────────────────────────────────────────────────────────
    stage('Checkout') {
      steps {
        checkout([
          $class: 'GitSCM',
          branches: [[name: "*/${params.BRANCH}"]],
          userRemoteConfigs: [[
            url: 'https://github.com/YOUR_ORG/Cloud_final_Project.git',
            credentialsId: 'github-token'
          ]]
        ])
        script {
          def sha = sh(script: 'git rev-parse --short HEAD', returnStdout: true).trim()
          env.IMAGE_TAG = "${sha}-${BUILD_NUMBER}"
          env.GIT_COMMIT_MSG = sh(script: 'git log -1 --format="%s"', returnStdout: true).trim()
          currentBuild.displayName = "#${BUILD_NUMBER} | ${sha} | ${env.TARGET_ENV}"
          currentBuild.description = env.GIT_COMMIT_MSG
        }
        sh 'git log --oneline -5'
        echo "Building image tag: ${env.IMAGE_TAG}"
      }
    }

    // ── 2. GCP Authentication ────────────────────────────────────────────────
    stage('GCP Auth') {
      steps {
        withCredentials([file(credentialsId: 'GCP_SERVICE_ACCOUNT_KEY', variable: 'GCP_KEY')]) {
          sh '''
            gcloud auth activate-service-account --key-file=$GCP_KEY
            gcloud config set project $GCP_PROJECT_ID
            gcloud auth configure-docker us-central1-docker.pkg.dev --quiet
          '''
        }
      }
    }

    // ── 3. Terraform (Infrastructure) ────────────────────────────────────────
    stage('Terraform Apply') {
      when { expression { params.DEPLOY_INFRA == true } }
      steps {
        dir('infrastructure/terraform') {
          withCredentials([
            string(credentialsId: 'TF_VAR_db_password',         variable: 'TF_VAR_db_password'),
            string(credentialsId: 'TF_VAR_jwt_secret',           variable: 'TF_VAR_jwt_secret'),
            string(credentialsId: 'TF_VAR_nvidia_api_key',       variable: 'TF_VAR_nvidia_api_key'),
            string(credentialsId: 'TF_VAR_auth0_client_secret',  variable: 'TF_VAR_auth0_client_secret'),
          ]) {
            sh '''
              terraform init -input=false
              terraform validate
              terraform plan -input=false -out=tfplan \
                -var="db_password=$TF_VAR_db_password" \
                -var="jwt_secret=$TF_VAR_jwt_secret" \
                -var="nvidia_api_key=$TF_VAR_nvidia_api_key" \
                -var="auth0_client_secret=$TF_VAR_auth0_client_secret"
              terraform apply -auto-approve tfplan
              terraform output -json > /tmp/tf_outputs.json
            '''
          }
        }
      }
    }

    // ── 4. Go Tests (parallel per service) ───────────────────────────────────
    stage('Test Backend') {
      when { expression { params.SKIP_TESTS == false } }
      parallel {
        stage('Test api-gateway')       { steps { dir('backend/api-gateway')       { sh 'go vet ./... && go test ./... -timeout 60s' } } }
        stage('Test auth-service')      { steps { dir('backend/auth-service')      { sh 'go vet ./... && go test ./... -timeout 60s' } } }
        stage('Test data-service')      { steps { dir('backend/data-service')      { sh 'go vet ./... && go test ./... -timeout 60s' } } }
        stage('Test file-service')      { steps { dir('backend/file-service')      { sh 'go vet ./... && go test ./... -timeout 60s' } } }
        stage('Test ai-service')        { steps { dir('backend/ai-service')        { sh 'go vet ./... && go test ./... -timeout 60s' } } }
        stage('Test analytics-service') { steps { dir('backend/analytics-service') { sh 'go vet ./... && go test ./... -timeout 60s' } } }
      }
    }

    // ── 5. Build Docker Images (parallel) ────────────────────────────────────
    stage('Build Images') {
      steps {
        script {
          def buildTasks = [:]
          def services = ['api-gateway', 'auth-service', 'data-service',
                          'file-service', 'ai-service', 'analytics-service']

          services.each { svc ->
            def s = svc
            buildTasks["Build ${s}"] = {
              sh """
                docker build \
                  --tag ${AR_REPO}/${s}:${IMAGE_TAG} \
                  --tag ${AR_REPO}/${s}:latest \
                  --label "git.commit=${IMAGE_TAG}" \
                  --label "build.number=${BUILD_NUMBER}" \
                  --cache-from ${AR_REPO}/${s}:latest \
                  backend/${s}
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
                  --tag ${AR_REPO}/frontend:${IMAGE_TAG} \
                  --tag ${AR_REPO}/frontend:latest \
                  --cache-from ${AR_REPO}/frontend:latest \
                  frontend
              """
            }
          }

          parallel buildTasks
        }
      }
    }

    // ── 6. Security Scan (Trivy) ──────────────────────────────────────────────
    stage('Security Scan') {
      steps {
        script {
          def scanTasks = [:]
          ['api-gateway', 'auth-service', 'data-service', 'ai-service'].each { svc ->
            def s = svc
            scanTasks["Scan ${s}"] = {
              sh """
                trivy image \
                  --exit-code 0 \
                  --ignore-unfixed \
                  --severity HIGH,CRITICAL \
                  --format json \
                  --output trivy-${s}.json \
                  ${AR_REPO}/${s}:${IMAGE_TAG} || true
              """
            }
          }
          parallel scanTasks
        }
        // Archive scan results
        archiveArtifacts artifacts: 'trivy-*.json', allowEmptyArchive: true
      }
    }

    // ── 7. Push Images to Artifact Registry (parallel) ───────────────────────
    stage('Push Images') {
      steps {
        script {
          def pushTasks = [:]
          def services = ['api-gateway', 'auth-service', 'data-service',
                          'file-service', 'ai-service', 'analytics-service']
          if (params.DEPLOY_FRONTEND) services.add('frontend')

          services.each { svc ->
            def s = svc
            pushTasks["Push ${s}"] = {
              sh """
                docker push ${AR_REPO}/${s}:${IMAGE_TAG}
                docker push ${AR_REPO}/${s}:latest
              """
            }
          }
          parallel pushTasks
        }
      }
    }

    // ── 8. Get GKE Credentials ────────────────────────────────────────────────
    stage('GKE Credentials') {
      steps {
        sh """
          gcloud container clusters get-credentials ${GKE_CLUSTER} \
            --region ${GCP_REGION} \
            --project ${GCP_PROJECT_ID}
          kubectl version --client
          kubectl get nodes -o wide
        """
      }
    }

    // ── 9. Apply K8s Base Resources ───────────────────────────────────────────
    stage('Apply K8s Base') {
      steps {
        sh """
          kubectl apply -f infrastructure/k8s/namespace.yaml
          kubectl apply -f infrastructure/k8s/configmap.yaml
          kubectl apply -f infrastructure/k8s/pdb.yaml
        """
      }
    }

    // ── 10. Database Migrations ───────────────────────────────────────────────
    stage('DB Migrations') {
      when { expression { params.RUN_MIGRATIONS == true } }
      steps {
        withCredentials([string(credentialsId: 'TF_VAR_db_password', variable: 'DB_PASS')]) {
          sh """
            cat infrastructure/k8s/migration-job.yaml | \
              sed "s|__DB_PASS__|${DB_PASS}|g" | \
              kubectl apply -f -
            kubectl wait --for=condition=complete job/db-migration \
              -n ${NAMESPACE} --timeout=300s
            kubectl delete job db-migration -n ${NAMESPACE} --ignore-not-found
          """
        }
      }
    }

    // ── 11. Deploy All Services ───────────────────────────────────────────────
    stage('Deploy Services') {
      steps {
        script {
          def services = ['api-gateway', 'auth-service', 'data-service',
                          'file-service', 'ai-service', 'analytics-service']
          if (params.DEPLOY_FRONTEND) services.add('frontend')

          services.each { svc ->
            sh """
              kubectl set image deployment/${svc} \
                ${svc}=${AR_REPO}/${svc}:${IMAGE_TAG} \
                -n ${NAMESPACE} \
              || kubectl apply -f infrastructure/k8s/ -n ${NAMESPACE}
            """
          }

          // Apply ingress and network resources
          sh """
            kubectl apply -f infrastructure/k8s/api-gateway.yaml
            kubectl apply -f infrastructure/k8s/frontend.yaml
            kubectl apply -f infrastructure/k8s/services.yaml
            kubectl apply -f infrastructure/k8s/ingress.yaml
          """
        }
      }
    }

    // ── 12. Rollout Verification ──────────────────────────────────────────────
    stage('Verify Rollout') {
      steps {
        script {
          def services = ['api-gateway', 'auth-service', 'data-service',
                          'file-service', 'ai-service', 'analytics-service']
          if (params.DEPLOY_FRONTEND) services.add('frontend')

          services.each { svc ->
            sh """
              kubectl rollout status deployment/${svc} \
                -n ${NAMESPACE} --timeout=180s
            """
          }

          sh """
            echo "=== Pod Status ==="
            kubectl get pods -n ${NAMESPACE} -o wide
            echo "=== Services ==="
            kubectl get svc -n ${NAMESPACE}
            echo "=== Ingress ==="
            kubectl get ingress -n ${NAMESPACE}
            echo "=== HPA ==="
            kubectl get hpa -n ${NAMESPACE}
          """
        }
      }
    }

    // ── 13. Smoke Test ────────────────────────────────────────────────────────
    stage('Smoke Test') {
      steps {
        script {
          // Wait for services to be ready, then hit health endpoints
          sh """
            sleep 10
            # Port-forward and test health endpoints
            kubectl port-forward svc/api-gateway 18080:8080 -n ${NAMESPACE} &
            PF_PID=\$!
            sleep 5
            curl -sf http://localhost:18080/health && echo "API Gateway: OK"
            kill \$PF_PID || true
          """
        }
      }
    }

  } // end stages

  post {
    success {
      echo "✅ Deployment successful! Tag: ${env.IMAGE_TAG} → ${params.TARGET_ENV}"
      // Add Slack/email notification here:
      // slackSend(channel: '#deployments', color: 'good', message: "✅ Portal deployed: ${env.IMAGE_TAG}")
    }

    failure {
      echo "❌ Pipeline failed! Rolling back ${params.TARGET_ENV}..."
      sh """
        for svc in api-gateway auth-service data-service file-service ai-service analytics-service; do
          kubectl rollout undo deployment/\$svc -n ${NAMESPACE} || true
        done
        echo "=== Post-rollback pod status ==="
        kubectl get pods -n ${NAMESPACE}
      """
      // slackSend(channel: '#deployments', color: 'danger', message: "❌ Portal deploy FAILED: build #${BUILD_NUMBER}")
    }

    always {
      // Clean up Docker images to save disk space
      sh """
        docker image prune -f --filter "until=2h" || true
        rm -f /tmp/kubeconfig-${BUILD_NUMBER} || true
      """
      cleanWs()
    }
  }
}
