// ============================================================
// Enterprise Knowledge Portal — Full CI/CD Pipeline
// Flow: GitHub → Jenkins → Cloud Build → Cloud Run
// Trigger: GitHub webhook on push to main / PR merge
// ============================================================

pipeline {
  agent any

  environment {
    // GCP
    GCP_PROJECT_ID   = "enterprise-portal-48689"
    GCP_REGION       = "us-central1"
    AR_REPO          = "us-central1-docker.pkg.dev/enterprise-portal-48689/enterprise-portal"
    KAFKA_BROKERS    = "REPLACE_WITH_MANAGED_KAFKA_BOOTSTRAP:9092"
    // Frontend calls the gateway; baked into the React build at compile time.
    REACT_APP_API_URL = "https://api-gateway-ogukkf7z3q-uc.a.run.app"
    OKTA_ISSUER      = "https://trial-5413467.okta.com/oauth2/default"
    OKTA_CLIENT_ID   = "0oa12cfmwjeBVrl0I698"
    // Okta sends the auth code back to the gateway, which proxies to auth-service /callback.
    OKTA_REDIRECT_URI = "https://api-gateway-ogukkf7z3q-uc.a.run.app/api/auth/callback"
    // After logout Okta redirects back to the frontend.
    OKTA_LOGOUT_REDIRECT_URI = "https://frontend-ogukkf7z3q-uc.a.run.app"

    // Image tag: git short sha + build number
    IMAGE_TAG        = ""   // set in Build stage
  }

  parameters {
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
            url: 'https://github.com/Nihar4/CMPE-282_Term_Project.git',
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

    // ── 3. Go Tests (parallel per service) ───────────────────────────────────
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

    // ── 4. Trigger Cloud Build serverless deployment to Cloud Run ────────────
    stage('Cloud Build → Cloud Run') {
      steps {
        sh '''
          gcloud builds submit \
            --config cloudbuild-serverless.yaml \
            --substitutions "_PROJECT_ID=${GCP_PROJECT_ID},_REGION=${GCP_REGION},_KAFKA_BROKERS=${KAFKA_BROKERS},_REACT_APP_API_URL=${REACT_APP_API_URL},_OKTA_ISSUER=${OKTA_ISSUER},_OKTA_CLIENT_ID=${OKTA_CLIENT_ID},_OKTA_REDIRECT_URI=${OKTA_REDIRECT_URI},_OKTA_LOGOUT_REDIRECT_URI=${OKTA_LOGOUT_REDIRECT_URI},_IMAGE_TAG=${IMAGE_TAG}"
        '''
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
      echo "❌ Pipeline failed! Cloud Build / Cloud Run deployment did not complete for ${params.TARGET_ENV}."
      // slackSend(channel: '#deployments', color: 'danger', message: "❌ Portal deploy FAILED: build #${BUILD_NUMBER}")
    }

    always {
      // Clean up Docker images to save disk space
      sh """
        docker image prune -f --filter "until=2h" || true
      """
      cleanWs()
    }
  }
}
