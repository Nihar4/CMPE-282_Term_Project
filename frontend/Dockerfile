# Build stage
FROM node:20-alpine AS builder
WORKDIR /app
COPY package.json ./
RUN npm install --legacy-peer-deps && \
    npm install ajv@^8.17.1 ajv-keywords@^5.1.0 --legacy-peer-deps --save-exact
COPY . .
ARG REACT_APP_API_URL=http://localhost:8080
ARG REACT_APP_OKTA_ISSUER=https://trial-5413467.okta.com/oauth2/default
ARG REACT_APP_OKTA_CLIENT_ID=0oa12cfmwjeBVrl0I698
ARG REACT_APP_OKTA_REDIRECT_URI=http://localhost:3000/authorization-code/callback
ARG REACT_APP_OKTA_LOGOUT_REDIRECT_URI=http://localhost:3000
RUN rm -f .env.local .env.*.local
ENV REACT_APP_API_URL=$REACT_APP_API_URL
ENV REACT_APP_OKTA_ISSUER=$REACT_APP_OKTA_ISSUER
ENV REACT_APP_OKTA_CLIENT_ID=$REACT_APP_OKTA_CLIENT_ID
ENV REACT_APP_OKTA_REDIRECT_URI=$REACT_APP_OKTA_REDIRECT_URI
ENV REACT_APP_OKTA_LOGOUT_REDIRECT_URI=$REACT_APP_OKTA_LOGOUT_REDIRECT_URI
ENV DISABLE_ESLINT_PLUGIN=true
ENV GENERATE_SOURCEMAP=false
RUN npm run build && \
    grep -R "$REACT_APP_OKTA_REDIRECT_URI" build/static/js && \
    ! grep -R "localhost:3000" build/static/js

# Serve stage
FROM nginx:alpine
COPY --from=builder /app/build /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 8080
CMD ["nginx", "-g", "daemon off;"]
