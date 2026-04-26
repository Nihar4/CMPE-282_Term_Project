# Build stage
FROM node:20-alpine AS builder
WORKDIR /app
COPY package.json ./
RUN npm install --legacy-peer-deps && \
    npm install ajv@^8.17.1 ajv-keywords@^5.1.0 --legacy-peer-deps --save-exact
COPY . .
ARG REACT_APP_API_URL=http://localhost:8080
ARG REACT_APP_AUTH0_DOMAIN=dev-xbnsordr5elttyug.us.auth0.com
ARG REACT_APP_AUTH0_CLIENT_ID=x8pFzWFtyYCBXrr6U2NkxABroQqqMvxM
ENV REACT_APP_API_URL=$REACT_APP_API_URL
ENV REACT_APP_AUTH0_DOMAIN=$REACT_APP_AUTH0_DOMAIN
ENV REACT_APP_AUTH0_CLIENT_ID=$REACT_APP_AUTH0_CLIENT_ID
ENV DISABLE_ESLINT_PLUGIN=true
ENV GENERATE_SOURCEMAP=false
RUN npm run build

# Serve stage
FROM nginx:alpine
COPY --from=builder /app/build /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
