# cf-best-domain

## 中文说明

`cf-best-domain` 是一个 Go 编写的 Cloudflare IP 优选工具。它会拉取 Cloudflare 官方 IPv4 段，随机抽样候选 IP，并对每个 IP 做 TCP 连接测速和 HTTPS 访问测速，选出当前网络下最优的 Cloudflare 边缘 IP。确认结果后，它可以自动把你自己的 DNS-only A 记录更新为这个最优 IP。

项目的命令行显示、测速结果表格和“默认展示最快前 10 个结果”的体验参考了 [XIU2/CloudflareSpeedTest](https://github.com/XIU2/CloudflareSpeedTest)。本项目当前重点放在 IPv4 抽样测速和 Cloudflare DNS A 记录自动更新。

重要提醒：用于承载优选 IP 的记录，例如 `cf-best.example.com`，通常应该保持 **DNS-only**，不要开启 Cloudflare 小黄云。如果开启代理模式，Cloudflare 会重新接管解析，客户端拿到的不一定是你写入的优选 IP。

适用流程：

```text
扫描 Cloudflare IPv4 段
  -> 抽样候选 IP
  -> 测试 443 TCP 和 HTTPS 延迟
  -> 选出最优 IP
  -> 可选：更新 Cloudflare DNS A 记录
```

### Cloudflare 准备

需要准备这些环境变量：

```bash
export CF_API_TOKEN="你的 Cloudflare API Token"
export CF_ZONE_ID="你的 Zone ID"
export CF_RECORD="cf-best.example.com"
export TEST_HOST="www.example.com"
```

说明：

- `CF_RECORD`：要自动更新的优选域名，建议 DNS-only。
- `TEST_HOST`：用于测速的已接入 Cloudflare 且开启代理的域名。
- `CF_API_TOKEN`：建议只授予对应 Zone 的 DNS 编辑权限。

### 本地测速

先只测速，不更新 DNS：

```bash
go run . \
  -host www.example.com \
  -sample 5 \
  -max 200 \
  -c 50
```

输出 JSON：

```bash
go run . -host www.example.com -output json
```

### 更新 DNS

确认测速结果可用后再加 `-update`：

```bash
export CF_API_TOKEN="你的 Cloudflare API Token"
export CF_ZONE_ID="你的 Zone ID"
export CF_RECORD="cf-best.example.com"
export TEST_HOST="www.example.com"

go run . -update
```

默认会创建或更新一条 DNS-only A 记录：

```text
cf-best.example.com -> 当前最优 Cloudflare IPv4
代理=关闭（DNS-only）
TTL=60
```

### 定时运行

每 30 分钟扫描并更新一次：

```bash
go run . -update -interval 30m
```

也可以用 cron：

```cron
*/30 * * * * /usr/local/bin/cf-best-domain -update >> /var/log/cf-best-domain.log 2>&1
```

### 参数与环境变量

| 参数 | 环境变量 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `-host` | `TEST_HOST` | 空 | 用于 HTTPS 测速的 Cloudflare 代理域名 |
| `-path` | `TEST_PATH` | `/cdn-cgi/trace` | HTTPS 测试路径 |
| `-sample` | `SAMPLE` / `CFBD_SAMPLE` | `5` | 每个 CIDR 抽样 IP 数 |
| `-max` | `MAX` / `CFBD_MAX` | `200` | 最大候选 IP 数 |
| `-c` | `C` / `CFBD_CONCURRENCY` | `50` | 并发测速数 |
| `-timeout` | `CFBD_TIMEOUT` | `3s` | 单次探测超时，支持 `3` 或 `3s` |
| `-update` | `UPDATE` / `CFBD_UPDATE` | 关闭 | 是否更新 DNS |
| `-zone` | `CF_ZONE_ID` | 空 | Cloudflare Zone ID |
| `-record` | `CF_RECORD` | 空 | 要更新的 A 记录名称 |
| `-token` | `CF_API_TOKEN` | 空 | Cloudflare API Token |
| `-ttl` | `CF_TTL` / `CFBD_TTL` | `60` | DNS TTL，`1` 表示自动 |
| `-proxied` | `CF_PROXIED` / `CFBD_PROXIED` | 关闭 | 是否开启 Cloudflare 代理 |
| `-interval` | `CFBD_INTERVAL` | `0` | 定时运行间隔，`0` 表示只运行一次 |
| `-output` | `CFBD_OUTPUT` | `table` | 输出格式：`table` 或 `json` |
| `-top` | `TOP` / `CFBD_TOP` | `10` | 输出前 N 条结果 |
| `-cidr` | 无 | 空 | 手动指定 IPv4 CIDR，可重复 |
| `-cidrs` | `CFBD_CIDRS` | 空 | 逗号/分号/换行分隔的 IPv4 CIDR |
| `-cidr-file` | `CFBD_CIDR_FILE` | 空 | IPv4 CIDR 文件 |

不指定 `-cidr`、`-cidrs` 或 `-cidr-file` 时，程序会从 Cloudflare 官方地址拉取 IPv4 段。

### Docker

本地构建：

```bash
docker build -t cf-best-domain:dev .
```

只测速：

```bash
docker run --rm \
  -e TEST_HOST="www.example.com" \
  cf-best-domain:dev
```

测速并更新 DNS：

```bash
docker run --rm \
  -e CF_API_TOKEN="你的 Cloudflare API Token" \
  -e CF_ZONE_ID="你的 Zone ID" \
  -e CF_RECORD="cf-best.example.com" \
  -e TEST_HOST="www.example.com" \
  cf-best-domain:dev -update
```

### 使用 git tag 发布镜像

项目内置 GitHub Actions。推送 `v*` tag 后会自动构建并发布多架构镜像到 GitHub Container Registry：

```bash
git tag v1.0.0
git push origin v1.0.0
```

镜像标签示例：

```text
ghcr.io/<owner>/<repo>:v1.0.0
ghcr.io/<owner>/<repo>:1.0.0
ghcr.io/<owner>/<repo>:1.0
```
