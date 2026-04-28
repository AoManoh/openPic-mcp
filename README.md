# openPic-mcp

openPic-mcp 是一个基于 MCP (Model Context Protocol) 协议的图像能力服务器，为 AI 编程助手提供图片理解、图片比较、图片生成和图片编辑能力。它支持任何 OpenAI-Compatible 的图像 API 服务。

> ⚠️ **当前版本说明**：当前主路径是 stdio 传输，可使用本地编译后的可执行文件，也可通过 `go run github.com/AoManoh/openPic-mcp/cmd/vision-mcp@master` 在线拉取源码并在本机运行。`uvx` / `npx` 包装入口和远端 Streamable HTTP 服务仍属于后续规划。

## 功能特性

- **MCP 协议兼容**：实现 MCP 协议规范，支持 stdio 传输
- **OpenAI-Compatible**：支持任何兼容 OpenAI 图像能力接口的服务
- **多种图片输入**：支持 Base64 编码、Data URI、HTTP/HTTPS URL、**本地文件路径**
- **多种图片格式**：支持 JPEG、PNG、WebP、GIF、BMP、TIFF、ICO、HEIC、AVIF、SVG 格式
- **图像比较**：支持 2-4 张图片的智能比较分析
- **图片生成与编辑**：支持通过 OpenAI-Compatible `/images/generations` 和 `/images/edits` 路由生成或编辑图片

## 快速开始

### 前置条件

- Go 1.23 或更高版本（本地构建）
- OpenAI API 密钥或兼容服务的访问凭证

### 本地运行

1. 克隆项目

```bash
git clone https://github.com/AoManoh/openPic-mcp.git
cd openPic-mcp
```

2. 配置环境变量

```bash
cp .env.example .env
# 编辑 .env 文件，填写实际配置
```

3. 构建并运行

```bash
go build -o openPic-mcp ./cmd/vision-mcp
./openPic-mcp
```

### 在线拉取运行（Go）

如果本机已安装 Go 1.23 或更高版本，可以不手动克隆仓库，直接在 MCP 客户端中使用 `go run` 拉取并运行：

```json
{
  "mcpServers": {
    "openPic-mcp": {
      "command": "go",
      "args": [
        "run",
        "github.com/AoManoh/openPic-mcp/cmd/vision-mcp@master"
      ],
      "env": {
        "OPENPIC_API_BASE_URL": "https://your-server.com/v1",
        "OPENPIC_API_KEY": "your-api-key",
        "OPENPIC_VISION_MODEL": "your-vision-model-name",
        "OPENPIC_IMAGE_MODEL": "your-image-model-name",
        "OPENPIC_TIMEOUT": "5m"
      }
    }
  }
}
```

国内网络环境下，首次运行需要下载 Go 模块和依赖，可临时在 MCP 配置的 `env` 中增加 Go 代理加速：

```json
{
  "env": {
    "GOPROXY": "https://goproxy.cn,direct",
    "GOSUMDB": "sum.golang.google.cn"
  }
}
```

`GOPROXY` 能加速公开 Go 模块下载；`GOSUMDB` 使用 Go checksum database 的国内可访问镜像。该方式仍然需要本机安装 Go，并且首次启动会在本机下载依赖和编译，速度取决于网络和机器性能。

### Docker 运行

> ⚠️ **注意**：当前版本仅支持 stdio 传输，Docker 部署**暂时无法直接用于 MCP 服务配置**。Docker 相关文件是为后续支持 HTTP 传输（Streamable HTTP）预留的，届时将支持端口暴露和远程调用。当前阶段请使用**本地运行**方式。

<details>
<summary>Docker 命令参考（开发测试用）</summary>

1. 配置环境变量

```bash
cp .env.example .env
# 编辑 .env 文件，填写实际配置
```

2. 使用 Docker Compose 启动

```bash
docker-compose up -d --build
```

3. 查看日志

```bash
docker-compose logs -f
```

4. 停止服务

```bash
docker-compose down
```

### 直接使用 Docker

```bash
docker build -t openpic-mcp:latest .

docker run -it --rm \
  -e OPENPIC_API_BASE_URL=https://api.openai.com/v1 \
  -e OPENPIC_API_KEY=your-api-key \
  -e OPENPIC_VISION_MODEL=gpt-4o \
  -e OPENPIC_IMAGE_MODEL=gpt-image-1 \
  openpic-mcp:latest
```

</details>

## 配置说明

所有配置通过环境变量设置。可以创建 `.env` 文件或直接设置环境变量。新配置优先使用 `OPENPIC_*`，并兼容旧的 `VISION_*`。

| 环境变量 | 必填 | 默认值 | 说明 |
| -------- | ---- | ------ | ---- |
| `OPENPIC_API_BASE_URL` | 是 | - | OpenAI-Compatible API 基础 URL，兼容 `VISION_API_BASE_URL` |
| `OPENPIC_API_KEY` | 是 | - | API 密钥，兼容 `VISION_API_KEY` |
| `OPENPIC_VISION_MODEL` | 是 | - | 视觉理解模型，兼容 `VISION_MODEL` |
| `OPENPIC_IMAGE_MODEL` | 使用 `generate_image` 或 `edit_image` 时必填 | - | 图片生成或编辑模型 |
| `OPENPIC_TIMEOUT` | 否 | 5m | API 请求超时时间，兼容 `VISION_TIMEOUT` |
| `OPENPIC_LOG_LEVEL` | 否 | info | 日志级别，兼容 `VISION_LOG_LEVEL` |

> **注意**：`OPENPIC_TIMEOUT` / `VISION_TIMEOUT` 必须使用 Go 的 duration 格式，例如：`30s`（30秒）、`2m`（2分钟）、`5m`（5分钟）。纯数字如 `120` 会导致解析错误。部分图片生成或编辑模型单次推理可能需要 1-4 分钟，不建议将该值设置得过低。

### 配置示例

OpenAI:

```bash
OPENPIC_API_BASE_URL=https://api.openai.com/v1
OPENPIC_API_KEY=your-openai-api-key
OPENPIC_VISION_MODEL=gpt-4o
OPENPIC_IMAGE_MODEL=gpt-image-1
```

Azure OpenAI:

```bash
OPENPIC_API_BASE_URL=https://your-resource.openai.azure.com/openai/deployments/your-deployment
OPENPIC_API_KEY=your-azure-api-key
OPENPIC_VISION_MODEL=gpt-4o
OPENPIC_IMAGE_MODEL=your-image-model-name
```

自托管服务:

```bash
OPENPIC_API_BASE_URL=https://your-server.com/v1
OPENPIC_API_KEY=your-api-key
OPENPIC_VISION_MODEL=your-vision-model-name
OPENPIC_IMAGE_MODEL=your-image-model-name
```

## MCP 配置示例

当前支持两种 stdio 使用方式：

- 使用本地编译后的可执行文件路径。
- 使用 `go run github.com/AoManoh/openPic-mcp/cmd/vision-mcp@master` 在线拉取并在本机运行。

`uvx` / `npx` 形式尚未提供包装入口，因此当前不能直接使用 `uvx --from git+... openpic-mcp`。如需完全免 Go 环境，需要后续提供预编译二进制下载器或独立包管理器入口。

### Claude Desktop

在 Claude Desktop 配置文件中添加：

macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
Windows: `%APPDATA%\Claude\claude_desktop_config.json`

```json
{
  "mcpServers": {
    "openPic-mcp": {
      "command": "/path/to/openPic-mcp",
      "args": [],
      "env": {
        "OPENPIC_API_BASE_URL": "https://api.openai.com/v1",
        "OPENPIC_API_KEY": "your-api-key",
        "OPENPIC_VISION_MODEL": "gpt-4o",
        "OPENPIC_IMAGE_MODEL": "gpt-image-1",
        "OPENPIC_TIMEOUT": "5m"
      }
    }
  }
}
```

### Cursor

在 Cursor MCP 配置中添加：

macOS: `~/.cursor/mcp.json`
Windows: `%USERPROFILE%\.cursor\mcp.json`

```json
{
  "mcpServers": {
    "openPic-mcp": {
      "command": "D:\\path\\to\\openPic-mcp.exe",
      "args": [],
      "env": {
        "OPENPIC_API_BASE_URL": "https://api.openai.com/v1",
        "OPENPIC_API_KEY": "your-api-key",
        "OPENPIC_VISION_MODEL": "gpt-4o",
        "OPENPIC_IMAGE_MODEL": "gpt-image-1",
        "OPENPIC_TIMEOUT": "5m"
      }
    }
  }
}
```

> **重要**：`args` 字段必须显式指定（即使为空数组 `[]`），否则 MCP 客户端可能无法正确启动服务。

## API/工具说明

### describe_image

分析并描述图片内容。

**参数：**

| 参数             | 类型   | 必填 | 说明                                                                |
| ---------------- | ------ | ---- | ------------------------------------------------------------------- |
| `image`        | string | 是   | 图片数据，支持 Base64 编码、Data URI、HTTP/HTTPS URL 或本地文件路径 |
| `prompt`       | string | 否   | 自定义分析提示词，不提供则使用默认提示词                            |
| `detail_level` | string | 否   | 描述详细程度：`brief`、`normal`（默认）、`detailed`           |

**示例请求：**

```json
{
  "name": "describe_image",
  "arguments": {
    "image": "https://example.com/image.jpg",
    "prompt": "描述这张图片中的主要内容",
    "detail_level": "detailed"
  }
}
```

**本地文件路径示例：**

```json
{
  "name": "describe_image",
  "arguments": {
    "image": "/path/to/local/image.jpg",
    "detail_level": "normal"
  }
}
```

**示例响应：**

```json
{
  "content": [
    {
      "type": "text",
      "text": "这张图片展示了..."
    }
  ]
}
```

### compare_images

比较多张图片，分析它们的相似点和差异。

**参数：**

| 参数             | 类型   | 必填 | 说明                                                       |
| ---------------- | ------ | ---- | ---------------------------------------------------------- |
| `images`       | array  | 是   | 图片数组（2-4张），每个元素支持 Base64、URL 或本地文件路径 |
| `prompt`       | string | 否   | 自定义比较提示词，不提供则使用默认提示词                   |
| `detail_level` | string | 否   | 比较详细程度：`brief`、`normal`（默认）、`detailed`  |

**示例请求：**

```json
{
  "name": "compare_images",
  "arguments": {
    "images": [
      "https://example.com/image1.jpg",
      "https://example.com/image2.jpg"
    ],
    "prompt": "比较这两张图片的主要差异",
    "detail_level": "detailed"
  }
}
```

**本地文件比较示例：**

```json
{
  "name": "compare_images",
  "arguments": {
    "images": [
      "/path/to/image1.png",
      "/path/to/image2.png",
      "/path/to/image3.png"
    ]
  }
}
```

**示例响应：**

```json
{
  "content": [
    {
      "type": "text",
      "text": "这些图片的比较结果：\n\n相似点：...\n\n差异点：..."
    }
  ]
}
```

### generate_image

根据文本提示词生成图片。该工具调用 OpenAI-Compatible `/images/generations` 路由，需要配置 `OPENPIC_IMAGE_MODEL`。

**参数：**

| 参数 | 类型 | 必填 | 说明 |
| ---- | ---- | ---- | ---- |
| `prompt` | string | 是 | 图片生成提示词 |
| `size` | string | 否 | 输出尺寸，默认 `1024x1024`；支持 `1024x1024`、`1024x1536`、`1536x1024`、`2048x2048` |
| `quality` | string | 否 | 输出质量，实际取值取决于服务支持情况 |
| `response_format` | string | 否 | 响应格式：`file_path`、`url` 或 `b64_json`，默认 `file_path`；仅显式选择 `b64_json` 时返回内联 Base64；若上游在 `url` 模式返回 Data URI，服务端会自动落盘并返回 `file_path` |
| `n` | number | 否 | 生成图片数量，当前仅支持 `1` |

**示例请求：**

```json
{
  "name": "generate_image",
  "arguments": {
    "prompt": "一只橘猫坐在窗边，电影感光影",
    "size": "1024x1024",
    "response_format": "file_path",
    "n": 1
  }
}
```

**示例响应：**

```json
{
  "content": [
    {
      "type": "text",
      "text": "{\n  \"images\": [\n    {\n      \"file_path\": \"/tmp/openpic-mcp/generate-123456.png\"\n    }\n  ],\n  \"created\": 1234567890\n}"
    }
  ]
}
```

### edit_image

根据输入图片、文本提示词和可选 mask 编辑图片。该工具调用 OpenAI-Compatible `/images/edits` 路由，需要配置 `OPENPIC_IMAGE_MODEL`。

**参数：**

| 参数 | 类型 | 必填 | 说明 |
| ---- | ---- | ---- | ---- |
| `image` | string | 是 | 待编辑图片，支持本地文件路径、HTTP/HTTPS URL、Data URI 或原始 Base64 |
| `prompt` | string | 是 | 图片编辑提示词 |
| `mask` | string | 否 | 可选 mask 图片，支持本地文件路径、HTTP/HTTPS URL、Data URI 或原始 Base64 |
| `size` | string | 否 | 输出尺寸，默认 `1024x1024`；支持 `1024x1024`、`1024x1536`、`1536x1024`、`2048x2048` |
| `quality` | string | 否 | 输出质量，实际取值取决于服务支持情况 |
| `response_format` | string | 否 | 响应格式：`file_path`、`url` 或 `b64_json`，默认 `file_path`；仅显式选择 `b64_json` 时返回内联 Base64；若上游在 `url` 模式返回 Data URI，服务端会自动落盘并返回 `file_path` |
| `n` | number | 否 | 编辑结果数量，当前仅支持 `1` |

**示例请求：**

```json
{
  "name": "edit_image",
  "arguments": {
    "image": "/path/to/input.png",
    "prompt": "给这只猫添加一顶红色帽子",
    "size": "1024x1024",
    "response_format": "file_path",
    "n": 1
  }
}
```

**示例响应：**

```json
{
  "content": [
    {
      "type": "text",
      "text": "{\n  \"images\": [\n    {\n      \"file_path\": \"/tmp/openpic-mcp/edit-123456.png\"\n    }\n  ],\n  \"created\": 1234567890\n}"
    }
  ]
}
```

### 图片生成与编辑兼容性说明

以下说明仅适用于 `generate_image` 与 `edit_image`，对 `describe_image` / `compare_images` 不生效。

#### `response_format` 语义

`response_format` 是**纯 MCP 客户端侧的交付方式**：

- `file_path`（默认）：openPic-mcp 把上游返回的 base64 字节落到本地临时文件，客户端只看到 `file_path`，避免在 MCP 消息中传输大段 base64。
- `b64_json`：openPic-mcp 在响应中保留 base64 字段，适用于客户端需要直接拿到字节的场景。
- `url`：等同于 `file_path`，仅在上游返回的是真正可访问的 URL（非 data URI）时才保留为 URL。GPT image 等模型默认只返回 base64，因此通常会被 openPic-mcp 落盘为 `file_path`。

无论入参选哪一种，openPic-mcp **都不会把 `response_format` 转发给上游 API**。社区已确认 GPT image models 在 `images/edits` 上传该字段会触发与“模型不支持”混淆的错误，因此 server 内部一律省略。

#### `size` 与 `aspect_ratio`

`size` 默认仅信任 OpenAI 官方 enum：`1024x1024`、`1024x1536`、`1536x1024`。`2048x2048` 仅在部分 OpenAI-Compatible 代理上可用，原生 OpenAI 不一定支持。

为了避免直接面对像素值，可以使用 `aspect_ratio`：

- `1:1` → `1024x1024`
- `4:3` → `1536x1024`
- `3:4` → `1024x1536`
- `16:9` → `1536x1024`（最近的横向预设）
- `9:16` → `1024x1536`（最近的纵向预设）
- `auto` → 留空 size，由上游决定

当 `size` 与 `aspect_ratio` 同时给出时，**`size` 优先**。

#### `output_format`

`output_format` 用于控制上游生成图片的编码格式（`png` / `jpeg` / `webp`），openPic-mcp 会原样转发到上游。该字段是可选的，留空则使用上游默认（通常为 `png`）。

> **声明 vs 实际**：`output_format` 是 advisory 字段，openPic-mcp **无法强制兑现**。社区已确认部分 OpenAI-Compatible 实现会静默吞掉 `output_format`：
>
> - OpenAI 官方 `gpt-image-1` 在 `/v1/images/edits` 端点对 `output_format=webp` 直接返回 400 `"Supported values are: 'png' and 'jpeg'"`。
> - 多个第三方代理（如 sub2api）会返回成功响应但实际内容仍为 PNG。
>
> 为此 openPic-mcp 会在每张返回图片上做 magic bytes 检测，并通过两条额外字段告诉调用方真实情况：
>
> - `images[i].format`：实际检测到的格式（`png` / `jpeg` / `webp` 等），文件扩展名也按这个值打。
> - `warnings[]`：当请求的 `output_format` 与检测格式不一致时附加的提示（例如 `images[0]: requested output_format="webp" but upstream returned "png"; saved as .png`）。
>
> 调用 `list_image_capabilities` 可拿到 `output_format_enforcement: "advisory"` 与 `output_format_notes` 完整披露。

#### 502 / `upstream_error` 误读指南

`OpenAI-Compatible` 图像上游可能把多种失败都包装为 `502 upstream_error`。常见情况包括：

1. 上游服务临时不可用，可稍后重试。
2. 请求参数与目标模型不兼容，例如不支持的 `size`、`response_format` 或模型路由。
3. 图像编辑端点对输入图片本身触发内容审核，但上游没有返回明确的 moderation 错误。

如果同一张图片在多个无害 prompt 下反复 edit 失败、而其他图片同时可以 edit 成功，可能是上游 image moderation 触发。客户端无法可靠区分该情况，建议停止重试并更换输入图片。openPic-mcp 不会自动重试 502/503/504，避免在不可恢复场景下扩大错误面。

### 图片生成与编辑耗时

图片生成和编辑请求会等待上游 OpenAI-Compatible 服务完成推理后再返回。部分模型（例如高质量图片生成模型）单次 1K 图片可能需要约 1-2 分钟，2K 图片可能需要约 2-4 分钟。建议将 `OPENPIC_TIMEOUT` 保持为默认 `5m` 或按实际服务耗时调大，避免服务端在上游仍在推理时提前超时。

### 支持的图片格式

- JPEG (.jpg, .jpeg, .jpe, .jfif)
- PNG (.png)
- WebP (.webp)
- GIF (.gif)
- BMP (.bmp, .dib)
- TIFF (.tif, .tiff)
- ICO (.ico)
- HEIC/HEIF (.heic, .heif)
- AVIF (.avif)
- SVG (.svg, .svgz)

> **注意**：实际支持情况取决于您使用的 Vision API 服务。部分格式（如 HEIC、AVIF、SVG）可能不被所有 API 支持。

### 图片输入方式

1. **Base64 编码**：直接传入 Base64 编码的图片数据
2. **Data URI**：`data:image/jpeg;base64,/9j/4AAQ...`
3. **HTTP/HTTPS URL**：`https://example.com/image.jpg`
4. **本地文件路径**：`/path/to/local/image.jpg` 或 `C:\path\to\image.png`（Windows）

> **注意**：本地文件路径支持绝对路径和相对路径。系统会自动检测文件的 MIME 类型。

## 开发指南

### 项目结构

```
openPic-mcp/
├── cmd/vision-mcp/          # 主程序入口
│   ├── main.go
│   └── main_test.go
├── internal/
│   ├── config/              # 配置管理
│   ├── errors/              # 错误定义
│   ├── image/               # 图片处理（本地文件、MIME检测）
│   ├── protocol/            # 协议层（JSON-RPC、MCP）
│   ├── provider/            # Provider 层
│   │   └── openai/          # OpenAI-Compatible Provider
│   ├── retry/               # 重试机制
│   ├── service/tool/        # 工具管理器
│   ├── tools/               # 工具实现
│   │   ├── describe.go      # describe_image 工具
│   │   ├── compare.go       # compare_images 工具
│   │   ├── generate.go      # generate_image 工具
│   │   └── edit.go          # edit_image 工具
│   └── transport/           # 传输层（stdio）
├── pkg/types/               # 公共类型定义
├── .env.example             # 环境变量示例
├── Dockerfile               # Docker 构建文件（预留）
├── docker-compose.yml       # Docker Compose 配置（预留）
├── go.mod                   # Go 模块定义
└── README.md                # 项目文档
```

### 构建

```bash
# 构建可执行文件
go build -o openPic-mcp ./cmd/vision-mcp

# 构建 Docker 镜像
docker build -t openpic-mcp:latest .
```

### 测试

```bash
# 运行所有测试
go test ./...

# 运行测试并显示详细输出
go test -v ./...

# 运行测试并生成覆盖率报告
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

### 代码格式化

```bash
# 格式化代码
go fmt ./...

# 检查代码格式
gofmt -d .
```

### 代码检查

```bash
# 运行 go vet
go vet ./...
```

## 路线图

### v1.0（当前版本）
- ✅ MCP 协议核心实现（stdio 传输）
- ✅ OpenAI-Compatible Vision API 支持
- ✅ describe_image、compare_images、generate_image 和 edit_image 工具
- ✅ 本地文件路径支持
- ✅ 多格式图片支持（10种格式）

### v1.x（规划中）
- 🔲 图片压缩功能（已设计，待实现）
- 🔲 HTTP/SSE 传输支持
- 🔲 Docker 容器化部署（依赖 HTTP 传输）
- 🔲 发布到 npm，支持 `npx` 方式调用
- 🔲 更多 Vision 工具（UI 分析、代码提取等）

### v2.0（远期规划）
- 🔲 托管服务，用户无需部署即可使用
- 🔲 多 Provider 支持（Anthropic、Google 等）

## 许可证

MIT License

Copyright (c) 2024

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
