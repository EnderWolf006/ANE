# ANE 2.4 部署指南

## 准备数据库与环境

ANE 使用外部 PostgreSQL。为新安装创建专用空数据库和账号，数据库账号应有目标 schema 的建表权限。启动时自动创建表。容器中的 `localhost` 指容器自身；访问宿主机上的 PostgreSQL 时可使用 `host.docker.internal`。

运行 `./scripts/init-env.sh` 生成本地 `.env` 和随机管理员密码，然后填写：

| 变量 | 说明 |
| --- | --- |
| `PG_URI` | PostgreSQL 连接 URI；对密码中的特殊字符进行 URL 编码。远程数据库按证书配置 TLS。 |
| `PUBLIC_BASE_URL` | 用户访问的 HTTP(S) 源地址，不含路径或查询参数。公网应使用 HTTPS。 |
| `ADMIN_USERNAME`、`ADMIN_PASSWORD` | 管理员账号和密码。 |
| `BIND_ADDRESS`、`PORT` | Compose 发布到宿主机的监听地址和端口，默认 `127.0.0.1:8080`。 |
| `ALLOW_PRIVATE_CALLBACKS` | 默认关闭；只有明确需要受控内网回调时才开启。 |

```sh
docker compose up -d --build
docker compose ps
curl --fail http://127.0.0.1:8080/healthz
```

服务端产物已内嵌 Web 前端。直接运行二进制时，通过进程环境设置 `PG_URI`、`PUBLIC_BASE_URL`、`ADMIN_USERNAME` 和 `ADMIN_PASSWORD`，用 `LISTEN_ADDR` 设置监听地址；二进制不会读取 `.env`。

## HTTPS 反向代理

以下 Nginx 示例适用于代理与 ANE 运行在同一宿主机的场景：

```nginx
client_max_body_size 8m;
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
}
```

保留路径和查询参数以支持易支付协议，但访问日志不要记录 `/api.php` 的查询串。对外启用 HTTPS 后，从 `PUBLIC_BASE_URL` 指定的源访问管理面板及付款页。

## 运维

- 备份外部 PostgreSQL、`.env`、Android 发布签名和设备共享目录。备份可能包含业务数据和凭据，存入受限位置。
- 上传收款码后，核对设备通知访问权限、心跳状态和订单回调。Android 系统的强制停止和省电策略会影响监听。
- SMTP 与邮箱订阅在 Web 面板设置。启用 SMTP 后可先发送测试邮件，再按邮箱选择收款、设备与回调提醒。
