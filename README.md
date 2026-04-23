# ImageGen MCP

轻量图像生成 MCP 服务，接百度 ERNIE-Image API，返回 base64 图片 + 画廊存档。

## 架构

与 [search-mcp](https://github.com/qiyun-kxc/search-mcp) 同构：Go + mcp-go，Streamable HTTP，systemd 管理。

- **端口**: 8088
- **路径**: `/mcp`
- **API**: 百度 AI Studio OpenAI 兼容接口
- **模型**: ERNIE-Image-Turbo (8B, DiT)

## 部署

```bash
git clone https://github.com/qiyun-kxc/imagegen-mcp.git
cd imagegen-mcp

# 配置 API Key（从 https://aistudio.baidu.com/account/accessToken 获取）
cp secrets.env.example secrets.env
vim secrets.env

# 编译
go mod tidy
go build -o imagegen-mcp .

# systemd
sudo cp imagegen-mcp.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now imagegen-mcp
```

## Nginx

```nginx
# 画廊静态文件
location /gallery/ {
    alias /home/ubuntu/imagegen-mcp/gallery/;
    autoindex off;
}

# MCP 端点
location /imagegen/ {
    proxy_pass http://127.0.0.1:8088/;
    proxy_http_version 1.1;
    proxy_set_header Connection '';
    proxy_buffering off;
    proxy_cache off;
}
```

## 环境变量

| 变量 | 必需 | 默认值 | 说明 |
|------|------|--------|------|
| `AISTUDIO_API_KEY` | 是 | - | 百度 AI Studio Access Token |
| `API_URL` | 否 | `https://aistudio.baidu.com/llm/lmapi/v3/images/generations` | API 端点 |
| `MODEL` | 否 | `ERNIE-Image-Turbo` | 模型名 |
| `GALLERY_DIR` | 否 | `/home/ubuntu/imagegen-mcp/gallery` | 图片保存目录 |
| `PORT` | 否 | `8088` | 监听端口 |

## MCP 工具

### `generate_image`

| 参数 | 必需 | 说明 |
|------|------|------|
| `prompt` | 是 | 图片描述，支持中英文 |
| `size` | 否 | 图片尺寸，默认 `1024x1024` |

返回：base64 图片（直接在对话中渲染） + 画廊链接。

## secrets.env 原则

`secrets.env` 只存 API Key，terminal MCP 绝不读取。如需修改找阿鹤。
