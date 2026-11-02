# 部署指南

产物是纯静态目录（`.build/public`），任何静态托管都能用。下面三条路都是「一条命令」，按你的基础设施选。

> 构建机不需要任何运行时——`engine build` 产出即最终形态。服务器上唯一可选的进程是 `engine serve`（只有当你要在源站收询盘时才需要）。

---

## 路线 1：VPS（nginx 托管 + 询盘走 engine serve）

适合：自己有服务器、要收询盘。推荐架构：nginx 直接服务静态文件，`engine serve` 只挂 `/api/` 收询盘。

```bash
# 构建
engine build .

# 同步到服务器（rsync 增量 + --delete 清孤儿，与 engine deploy 同一套语义）
rsync -avz --delete .build/public/ user@server:/srv/www/mysite/

# 目标是本机路径时可用引擎自带同步（sha256 比对、只写变化的文件）：
#   engine deploy . --to /srv/www/mysite
```

服务器上的询盘服务（systemd 常驻）：

```ini
# /etc/systemd/system/mysite-engine.service
[Unit]
Description=mysite inquiry service
After=network.target

[Service]
WorkingDirectory=/srv/www/mysite
ExecStart=/usr/local/bin/engine serve . --addr 127.0.0.1:8091
Restart=always
User=www-data

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable --now mysite-engine
```

nginx 站点配置要点（完整文件见 `examples/nginx.conf`）：

```nginx
server {
    listen 443 ssl;
    server_name www.mysite.com;
    root /srv/www/mysite;
    # 带内容哈希的文件永久缓存（构建管线已保证指纹不变）
    location ~* \.[0-9a-f]{8}\.[A-Za-z0-9]+$ {
        add_header Cache-Control "public, max-age=31536000, immutable";
    }
    # 询盘 API 转给 engine serve
    location /api/ {
        proxy_pass http://127.0.0.1:8091;
        proxy_set_header X-Forwarded-For $remote_addr;
    }
    location / { try_files $uri $uri/ =404; }
}
```

## 路线 2：Cloudflare Pages（纯静态，免费 CDN）

适合：不需要源站进程；询盘 API 走第三方（表单 `api_base` 指向任意可收 POST 的端点）。

```bash
# 前置：wrangler 登录一次（npx wrangler login）
engine build . && npx wrangler pages deploy .build/public --project-name mysite
```

## 路线 3：GitHub Pages（推送即上线）

仓库里放一份 workflow（`.github/workflows/deploy-pages.yml`，见 `examples/` 说明），每次 push 到 main 自动构建发布：

```yaml
name: Deploy
on:
  push:
    branches: [main]
permissions:
  contents: read
  pages: write
  id-token: write
jobs:
  build-deploy:
    runs-on: ubuntu-latest
    environment:
      name: github-pages
      url: ${{ steps.deployment.outputs.page_url }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "1.27" }
      - run: go build -o engine ./cmd/engine
      - run: ./engine new site --brand "${{ vars.BRAND || 'My Site' }}"
      # 内容在仓库里时：拷贝 content/settings/theme 到 site/ 再 build
      - run: |
          cp -r content settings.yaml engine.yaml theme site/ 2>/dev/null || true
          cd site && ../engine build
      - uses: actions/upload-pages-artifact@v3
        with: { path: site/.build/public }
      - id: deployment
        uses: actions/deploy-pages@v4
```

---

## 询盘链路的部署取舍

| 托管方式 | 询盘怎么收 |
|---|---|
| VPS + engine serve | 原生：落库 + 邮件通知，`engine inquiries` 查看 |
| Cloudflare Pages / GitHub Pages | 表单 `api_base` 指向任意 HTTP 端点（自建、Serverless、第三方表单服务均可） |

静态托管下引擎的验证仍会全量执行（构建期校验、hreflang、断链），只是运行时的询盘落库换了个去处。
