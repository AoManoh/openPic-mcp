# openPic-mcp

openPic-mcp 是一个基于 MCP (Model Context Protocol) 协议的 Vision 服务器，为 AI 编程助手提供图片理解能力。它支持任何 OpenAI-Compatible 的 Vision API 服务。

> ⚠️ **当前版本说明**：v1.0 仅支持**本地部署**方式（stdio 传输），需要用户自行编译并配置本地可执行文件路径。后续版本将支持线上服务调用方式（类似 `npx @anthropic/openPic-mcp`），届时用户无需本地编译，可直接通过 MCP 配置使用。

## 功能特性

- **MCP 协议兼容**：实现 MCP 协议规范，支持 stdio 传输
- **OpenAI-Compatible**：支持任何兼容 OpenAI Vision API 格式的服务
- **多种图片输入**：支持 Base64 编码、Data URI、HTTP/HTTPS URL、**本地文件路径**
- **多种图片格式**：支持 JPEG、PNG、WebP、GIF、BMP、TIFF、ICO、HEIC、AVIF、SVG 格式
- **图像比较**：支持 2-4 张图片的智能比较分析

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
  -e VISION_API_BASE_URL=https://api.openai.com/v1 \
  -e VISION_API_KEY=sk-your-api-key \
  -e VISION_MODEL=gpt-4o \
  openpic-mcp:latest
```

</details>

## 配置说明

所有配置通过环境变量设置。可以创建 `.env` 文件或直接设置环境变量。

| 环境变量                | 必填 | 默认值 | 说明                   |
| ----------------------- | ---- | ------ | ---------------------- |
| `VISION_API_BASE_URL` | 是   | -      | Vision API 基础 URL    |
| `VISION_API_KEY`      | 是   | -      | Vision API 密钥        |
| `VISION_MODEL`        | 是   | -      | Vision 模型名称        |
| `VISION_TIMEOUT`      | 否   | 30s    | API 请求超时时间（需带单位，如 `30s`、`2m`） |
| `VISION_LOG_LEVEL`    | 否   | info   | 日志级别               |

> **注意**：`VISION_TIMEOUT` 必须使用 Go 的 duration 格式，例如：`30s`（30秒）、`2m`（2分钟）、`1m30s`（1分30秒）。纯数字如 `120` 会导致解析错误。

### 配置示例

OpenAI:

```bash
VISION_API_BASE_URL=https://api.openai.com/v1
VISION_API_KEY=sk-your-openai-api-key
VISION_MODEL=gpt-4o
```

Azure OpenAI:

```bash
VISION_API_BASE_URL=https://your-resource.openai.azure.com/openai/deployments/your-deployment
VISION_API_KEY=your-azure-api-key
VISION_MODEL=gpt-4o
```

自托管服务:

```bash
VISION_API_BASE_URL=https://your-server.com/v1
VISION_API_KEY=your-api-key
VISION_MODEL=your-model-name
```

## MCP 配置示例

> 📌 **当前版本**：需要指定本地编译后的可执行文件路径。后续版本将支持类似以下的线上调用方式：
> ```json
> {
>   "mcpServers": {
>     "openPic-mcp": {
>       "command": "npx",
>       "args": ["-y", "@anthropic/openPic-mcp"],
>       "env": { "VISION_API_KEY": "sk-your-api-key" }
>     }
>   }
> }
> ```

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
        "VISION_API_BASE_URL": "https://api.openai.com/v1",
        "VISION_API_KEY": "sk-your-api-key",
        "VISION_MODEL": "gpt-4o",
        "VISION_TIMEOUT": "120s"
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
        "VISION_API_BASE_URL": "https://api.openai.com/v1",
        "VISION_API_KEY": "sk-your-api-key",
        "VISION_MODEL": "gpt-4o",
        "VISION_TIMEOUT": "120s"
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
│   │   └── compare.go       # compare_images 工具
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
- ✅ describe_image 和 compare_images 工具
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
