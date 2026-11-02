# Zentea 示例站

一个完整可上线的茶叶 B2B 双语站（en/zh），用来演示引擎的全部能力。**这就是「engine new 之后填上真实内容」的样子。**

## 跑起来（3 条命令）

```bash
cd examples/zentea
go build -o engine ./cmd/engine   # 仓库根执行一次即可
./engine dev . --addr :8080       # 打开 http://localhost:8080
```

## 看什么

| 看点 | 在哪 |
|---|---|
| 16 种内容区块 | `content/pages/home.yaml` |
| 产品 / 分类 / 场景 / 案例 / 证书 | `content/` 各目录 |
| 多语言与 hreflang | 每个实体的 `en:` / `zh:` 字段；构建报告的 hreflang 组 |
| 图片管线 | `media/`（构建时自动生成 LQIP 与 WebP 派生图） |
| 询盘表单 | 任意产品页底部；`./engine inquiries` 查看落库结果 |
| 内容库往返 | `./engine import .` → `./engine build . --db .data/content.db` → `./engine export . --out /tmp/restored` |

## 拿它当起点改造

```bash
cp -r examples/zentea mysite
cd mysite && rm -rf .data .build .backup
# 全局替换品牌与内容，然后：
engine dev . --addr :8080
```

内容结构与字段说明见仓库根 `README.md` 的「站点结构」一节。构建期校验（36 条规则）会在你漏字段、写错格式时精确报错。
