## 关于本项目

为 @bonzezeng 布置的课外任务。

在根目录添加 `.env` 文件并配置 `REDIS.ADDR`, `REDIS.DB`, `PORT` 后，可分别在本地启动 Redis (6379) - 后端 (8080) - 前端 (5173) 后直接访问

也可以使用 Docker Compose 一键创建镜像与启动容器，但由于部分文件未添加到 Git，不保证前端能跑起来

## 技术选型

前端：Vue 3 + Vite + Tailwind CSS

后端：Go + Gin + Redis (ZSET, AOF, RDB) + SSE

## 已支持功能

- 展示文档列表
- 展示点击量历史总排行榜
- 展示点击量近 10 分钟排行榜
- 新增、编辑、删除文档
- 任何用户产生数据变更时，其他用户页面自动更新
- 服务可用期间产生的数据支持重启后恢复

## 项目结构

```
├── .env
├── Dockerfile
├── README.md
├── cmd (1)
│   └── main.go                        入口函数
├── config (1)
│   └── config.go                      配置 Redis 地址
├── doc-rank-frontend (1008 | skipped) 前端项目
├── docker-compose.yml                 Docker Compose 配置
├── go.mod
├── go.sum
├── internal (7)
│   ├── handlers (3)
│   │   ├── click.go                   点击量增加
│   │   ├── doc.go                     文档管理
│   │   └── rank.go                    排行榜获取
│   ├── redis (2)
│   │   ├── client.go                  Redis 连接
│   │   └── doc.go                     Redis 记录点击
│   └── sse (2)
│       ├── hub.go                     客户端订阅解绑
│       └── stream.go                  接收 Hub 消息并广播
├── package-lock.json
└── redis_data (1)
    └── redis.conf                     Redis 配置
```