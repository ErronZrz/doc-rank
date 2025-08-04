FROM node:20-alpine AS builder

# 接收构建参数
ARG VITE_API_BASE_URL
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL

WORKDIR /app
COPY . .

# 写入动态 .env 文件（供 Vite 构建读取）
RUN echo "VITE_API_BASE_URL=$VITE_API_BASE_URL" > .env.production

RUN npm install && npm run build

# 用 nginx 托管构建结果
FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/nginx.conf
EXPOSE 80
