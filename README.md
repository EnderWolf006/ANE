# ANE

![Static Badge](https://img.shields.io/badge/LinuxDO-_Community-blue?link=https%3A%2F%2Flinux.do%2F)

<p align="center">
  <img src="web/public/assets/ane-logo.png" alt="ANE" width="280">
</p>

ANE （Automated Notification-Driven Epay） 是一个自托管的收款订单匹配服务。Android 设备监听支付宝和微信的收款通知，服务端按渠道、实付金额和订单有效期匹配订单，再向业务系统发送易支付回调。

ANE 使用你自己的固定收款码，不代收资金，也不调用官方支付 API。

![ANE 管理工作台演示：交易汇总与监听设备状态](docs/images/dashboard.png)

## 可以做什么

- 接收支付宝个人、支付宝商家和微信收款助手的系统通知，支持经典易支付 MD5 协议。
- 按商户管理收款码、设备、订单和优惠池。每个应用有独立密钥，同一商户的应用共享设备及对应渠道的优惠池；支付宝和微信的额度独立。
- 在 Web 面板筛选订单、查看实收汇总和回调日志、导出 CSV，并为每个邮箱单独设置通知偏好。
- 查看设备心跳和通知上报状态，按需订阅收款、回调失败、设备离线及持续离线提醒。

## 快速开始

准备 Docker Compose 和一个专用的空 PostgreSQL 数据库。Compose 只启动 ANE，不会创建数据库。

```sh
./scripts/init-env.sh
# 编辑 .env，填写 PG_URI 和 PUBLIC_BASE_URL
docker compose up -d --build
curl --fail http://127.0.0.1:8080/healthz
```

默认只监听宿主机本地端口。从 `PUBLIC_BASE_URL` 对应的 `/admin` 登录，账号和密码见本地 `.env`。公网 HTTPS、数据库连接和备份配置见[部署指南](DEPLOYMENT.md)。

1. 在「商户配置」上传支付宝或微信收款码。
2. 在 Android 8.0 或更新版本上安装 ANE，选择用于保存配置和队列的共享目录。启用支付宝收款通知；使用微信时开启微信收款助手的系统通知。
3. 在 Web「监听设备」生成连接令牌，在 Android「配置」填写服务地址和令牌。授权通知访问，打开监听并确认 Web 页面显示设备在线。
4. 在「易支付应用」复制当前应用的网关地址、商户 ID 和密钥，填入业务系统。建议先用小额订单核对支付和回调流程。

Android 后台监听依赖手机供电、网络和系统权限。设备停机，或通知未进入系统通知栏时，ANE 无法确认该笔收款。

## 易支付接入

| 路径 | 用途 |
| --- | --- |
| `/submit.php` | 网页下单并跳转到收银台 |
| `/mapi.php` | 创建订单并返回 JSON，使用响应中的 `payurl` 打开收银台 |
| `/api.php?act=order` | 使用应用密钥查询该应用的订单 |

提交 `pid`、`type`（`alipay` 或 `wxpay`）、`out_trade_no`、`name`、`money`、`notify_url` 和签名；`return_url`、`param` 可选。同一商户的应用共用商户 ID，服务端通过签名识别应用。

签名时排除 `sign`、`sign_type` 和空字段；将解码后的参数按名称 ASCII 升序以 `key=value&key=value` 拼接，直接附加应用密钥，计算 UTF-8 MD5 的小写十六进制。可用 [下单脚本](scripts/create-order.py) 检查接入。

回调通过 GET 发送。业务系统应验签、检查订单及金额、幂等处理，再回复 HTTP 2xx 和正文 `success`。浏览器返回页不能代替服务端回调。查询 URL 可能包含密钥，避免记录查询串。

## 构建和测试

GitHub Actions 会在推送、拉取请求和手动运行时执行测试并上传产物。在仓库的 **Actions → Build → 对应运行 → Artifacts** 下载：

- `ane-server-linux`：Linux amd64 与 arm64 的 `.tar.gz` 构建包，已内嵌 Web 前端，含 SHA-256 校验文件。
- `ane-android`：可用于测试的 Debug APK、未签名 Release APK 和 SHA-256 校验文件。Debug APK 不用于覆盖生产安装；部署到长期使用的设备时，请用自己保管的同一把发布密钥签名。

本地服务端开发需要 Go 1.24+、Node.js 22.12+ 和 PostgreSQL：

```sh
npm --prefix web ci
npm --prefix web test
npm --prefix web run build
go build -o ane ./cmd/ane
go vet ./...
# 使用专用测试数据库。未设置 TEST_DATABASE_URL 时跳过 PostgreSQL 集成测试。
TEST_DATABASE_URL='postgres://ane:example-password@127.0.0.1:5432/ane_test?sslmode=disable' go test -race ./...
```

Android 本地构建需要 JDK 17+ 和 Android SDK 36。执行 `./scripts/build-apk.sh`，签名 APK 和校验文件位于 `dist/`。不要提交 `.env`、签名文件、设备共享数据或数据库备份。

代码目录：`cmd/ane` 是服务入口，`internal/server` 是 Go 服务，`web` 是 Web 前端，`android` 是收款设备客户端。第三方图标来源见[声明](THIRD_PARTY_NOTICES.md)。

## 许可证

ANE 依据 [GNU 通用公共许可证第 3 版（GPL-3.0-only）](LICENSE) 发布。第三方素材适用各自的许可，详见[第三方声明](THIRD_PARTY_NOTICES.md)。

## 免责声明

ANE 是独立项目，与支付宝、微信支付及其运营方无隶属或合作关系。ANE 不经手资金，也不能代替支付平台账单或官方支付接口。订单匹配依赖设备收到的系统通知；通知缺失、延迟或设备离线时，可能漏报或错配。请以实际到账记录为准，并在用于正式收款前验证流程。

本软件按现状提供，不作任何明示或默示保证。部署者负责遵守当地法律及支付平台规则，并妥善保护密钥、设备令牌和交易数据。担保免责声明及责任限制以 GPL v3 第 15 至 17 条和适用法律为准。
