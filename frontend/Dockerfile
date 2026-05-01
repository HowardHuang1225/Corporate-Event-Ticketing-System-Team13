# Build stage
FROM node:20-alpine AS builder

WORKDIR /app

# Copy package files and install dependencies
COPY package*.json ./
RUN npm install

# Copy source code and build
COPY . .
RUN npm run build

# Run stage
FROM nginx:alpine

# Create a non-root user for nginx
RUN touch /var/run/nginx.pid && \
    chown -R nginx:nginx /var/run/nginx.pid /var/cache/nginx /var/log/nginx /etc/nginx/conf.d

# Copy built files from builder stage
COPY --from=builder /app/dist /usr/share/nginx/html
RUN chown -R nginx:nginx /usr/share/nginx/html

# Copy nginx template
COPY nginx.conf /etc/nginx/templates/default.conf.template

# Default port
ENV PORT=80

# Use non-root user (Note: non-root users cannot bind to ports below 1024 by default in some envs, 
# but Nginx Docker image has some workarounds or we can use 8080 as default)
ENV NGINX_ENTRYPOINT_QUIET_LOGS=1

# Use non-root user
USER nginx

# Expose port
EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
