# ANE 部署说明

当前版本 2.4.0，数据库 schema 6。服务使用外部 PostgreSQL，Compose 只运行 ANE。

## 配置与启动

首次部署前准备空数据库，复制 `.env.example` 为 `.env`，填写自己的配置：

```dotenv
PG_URI=postgres://ane:replace-database-password@database.example.com:5432/ane?sslmode=verify-full
PUBLIC_BASE_URL=https://ane.example.com
ADMIN_USERNAME=admin
ADMIN_PASSWORD=replace-with-at-least-16-random-characters
BIND_ADDRESS=127.0.0.1
PORT=8080
ALLOW_PRIVATE_CALLBACKS=false
TZ=Asia/Shanghai
```

数据库密码中的特殊字符需 URL 编码；数据库用户需要建表权限。实际 TLS 参数按数据库证书配置。容器内 localhost 是容器自身，宿主机数据库可使用 `host.docker.internal`。

```sh
./scripts/init-env.sh
# 编辑 .env，确认数据库和域名配置。
docker compose build server
docker compose up -d server
docker compose ps
curl --fail http://127.0.0.1:8080/healthz
```

已有 `.env` 时初始化脚本不会覆盖。镜像构建自动安装前端依赖、构建静态资源并嵌入 Go 二进制。运行时不需要 Node 或 PHP。

## 反向代理

将 HTTPS 域名转发到服务监听地址；以下示例假设代理和服务在同一宿主机：

```nginx
client_max_body_size 8m;
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
}
```

保留路径及 query。管理面板从 `PUBLIC_BASE_URL` 指定的源访问；HTTPS 模式 Cookie 为 Secure。`/api.php` 查询串包含应用密钥，访问日志应脱敏或禁用该路径的 query 记录。

## 现有业务升级

升级前备份外部数据库、`.env`、原镜像和 Android 签名文件。备份包含凭据和交易信息，保存在受控目录，禁止提交到仓库。

2.4.0 从 schema 5 迁移到 schema 6：新增微信收款码及订单/通知渠道，历史订单归为支付宝；删除业务用户名和付款人字段、清理旧收款原文及付款邮件副本。旧商户、应用密钥、会话、设备绑定、订单和邮件队列保留。较早的 v2 schema 2–4 会自动迁移到 schema 6；升级前应在隔离数据库副本演练迁移，校验原有数据。v1 数据库不兼容，不应直接连接。

schema 5 将原商户密钥原样迁移成普通应用 `default`，已有订单归属 `default`，不修改历史签名密钥。各应用共享商户的收款设备和优惠池，应用端继续使用原易支付协议。多邮箱订阅按邮箱、事件和应用独立配置。

```sh
# 对现有业务执行前先完成备份和隔离验证。
docker compose build server
docker compose up -d server
curl --fail http://127.0.0.1:8080/healthz
```

更新后核对版本、历史应用密钥、交易与回调、订阅、在线设备及容器健康。回退使用兼容当前数据库 schema 的镜像，不要用旧备份覆盖仍有新交易写入的正式数据库。

## Android 更新

使用 `scripts/build-apk.sh` 构建。已有安装必须沿用原 `android/keystore/ane-release.jks` 和 `android/keystore.properties`；这些文件不在仓库中，需单独保管。换签名不能覆盖安装，也可能改变设备标识。

2.4.0 同时监听支付宝与微信收款，默认 15 秒状态上报，可配置 5–120 秒；收款通知立即上报。旧共享目录配置自动使用默认间隔，保留令牌和待上报队列。覆盖安装后保留原共享目录；真机后台行为受系统省电和权限限制。

本次迁移会按要求删除旧交易用户名；商户登录账号、密钥、会话、设备和原有交易金额及回调凭据保留。备份仍需单独受控保管，不提交 GitHub。升级应先在隔离网络恢复副本并验证，再短暂停止旧服务、事务迁移、启动新镜像。schema 6 禁止启动历史 schema 5 镜像；若迁移事务失败，回滚事务并恢复旧服务；提交后使用兼容 schema 6 的修复镜像。

反代应允许至少 8 MiB 请求体，支持同时保存两张最大 2 MiB 的收款码（包含 base64 开销）。微信付款需要在面板上传微信收款码，并在业务端启用 `wxpay`；现有支付宝配置继续使用。
