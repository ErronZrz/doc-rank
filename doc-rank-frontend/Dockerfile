FROM node:20-alpine AS builder
WORKDIR /app
COPY . .
RUN npm install && npm run build

# 用 nginx 托管构建结果
FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
EXPOSE 80
