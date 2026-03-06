# Domain Proxy

一个基于 Go 实现的 MITM（中间人）代理服务器，支持 HTTP 和 SOCKS5 协议，能够将指定域名的流量透明转发到另一个域名。

## 功能特性

- **HTTP + SOCKS5 共用端口**：同一端口自动识别协议，无需分开配置
- **HTTP 代理**：支持 HTTP CONNECT 隧道，拦截 HTTPS 流量
- **SOCKS5 代理**：完整实现 RFC 1928 协议，支持无认证模式
- **MITM 拦截**：动态签发 TLS 证书，解密 HTTPS 流量并进行域名重写
- **域名转发**：通过 YAML 配置文件定义转发规则，将源域名的请求转发到目标域名
- **证书管理**：CLI 命令一键生成 CA 根证书，运行时动态签发域名证书并缓存
- **高性能**：Go 原生并发（goroutine），单二进制部署，跨平台编译

## 使用场景

典型的使用场景：你有一个部署在 Cloudflare Workers 上的反代服务 `your-proxy.example.com`，它反代了 `api.telegram.org`。你希望应用程序代码中不修改任何域名，仍然请求 `api.telegram.org`，但实际流量被透明地转发到 `your-proxy.example.com`。

```
应用程序请求 api.telegram.org
    ↓ 经过代理
代理将请求转发到 your-proxy.example.com
    ↓
your-proxy.example.com 反代到真正的 api.telegram.org
    ↓
响应原路返回给应用程序
```

这样应用程序完全无需修改代码，只需设置代理即可。

## 目录

- [安装](#安装)
  - [前提条件](#前提条件)
  - [从源码编译](#从源码编译)
  - [交叉编译](#交叉编译)
- [快速开始](#快速开始)
  - [第一步：生成 CA 证书](#第一步生成-ca-证书)
  - [第二步：编写配置文件](#第二步编写配置文件)
  - [第三步：启动代理服务器](#第三步启动代理服务器)
  - [第四步：测试代理](#第四步测试代理)
- [安装 CA 证书到系统](#安装-ca-证书到系统)
- [在 Docker 中使用](#在-docker-中使用)
  - [方式一：代理运行在宿主机，应用在容器内](#方式一代理运行在宿主机应用在容器内)
  - [方式二：代理和应用都在容器内（Docker Compose）](#方式二代理和应用都在容器内docker-compose)
- [在应用程序中使用](#在应用程序中使用)
  - [环境变量方式（推荐）](#环境变量方式推荐)
  - [Python](#python)
  - [Node.js](#nodejs)
  - [Go](#go)
  - [Java](#java)
  - [curl](#curl)
  - [wget](#wget)
- [配置文件详解](#配置文件详解)
- [CLI 命令参考](#cli-命令参考)
- [工作原理](#工作原理)
- [技术细节](#技术细节)
- [项目结构](#项目结构)
- [常见问题](#常见问题)
- [安全警告](#安全警告)

## 安装

### 前提条件

- Go 1.21 或更高版本（使用了 `log/slog` 标准库）

检查 Go 版本：

```bash
go version
# 输出示例: go version go1.24.3 linux/amd64
```

如果没有安装 Go，请从 https://go.dev/dl/ 下载安装。

### 从源码编译

```bash
# 1. 克隆仓库
git clone <repo-url>
cd domain-proxy

# 2. 下载依赖
go mod download

# 3. 编译（当前平台）
go build -trimpath -ldflags="-s -w" -o domain-proxy .
```

编译参数说明：

| 参数 | 作用 | 体积影响 |
|------|------|---------|
| `-trimpath` | 移除二进制中的本地文件路径信息，增强安全性和可复现性 | 略减 |
| `-ldflags="-s -w"` | `-s` 移除符号表，`-w` 移除 DWARF 调试信息 | **减少约 35%**（11M → 7M） |

Windows 上编译：

```powershell
go build -trimpath -ldflags="-s -w" -o domain-proxy.exe .
```

### 交叉编译

Go 支持交叉编译，可以在任意平台上编译出其他平台的二进制文件：

```bash
# Linux amd64
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o domain-proxy-linux-amd64 .

# Linux arm64（树莓派、ARM 服务器）
GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o domain-proxy-linux-arm64 .

# macOS Apple Silicon
GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o domain-proxy-darwin-arm64 .

# macOS Intel
GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o domain-proxy-darwin-amd64 .

# Windows
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o domain-proxy.exe .
```

编译产物是一个无依赖的静态二进制文件（约 7MB），可以直接拷贝到目标机器上运行。

## 快速开始

以下以 Linux 环境为例，Windows / macOS 操作类似。

### 第一步：生成 CA 证书

首次使用前，需要生成一个 CA（证书颁发机构）根证书。这个根证书用于在运行时动态签发各个域名的 TLS 证书。

```bash
./domain-proxy gencert
```

输出：

```
CA certificate generated:
  cert: certs/ca.crt
  key:  certs/ca.key
```

默认会在当前目录下创建 `certs/` 目录，生成两个文件：

| 文件 | 说明 |
|------|------|
| `ca.crt` | CA 根证书，需要安装到客户端/系统的信任存储中 |
| `ca.key` | CA 私钥，**必须妥善保管**，拥有此文件可以签发任意域名的证书 |

你也可以指定输出目录：

```bash
./domain-proxy gencert --out /etc/domain-proxy/certs
```

### 第二步：编写配置文件

项目提供了 `config/config.example.yaml` 作为配置模板。复制并修改为你自己的配置：

```bash
cp config/config.example.yaml config/config.yaml
```

> **注意**：`config/config.yaml` 已被 `.gitignore` 忽略，不会被提交到版本控制中（因为可能包含敏感的转发规则）。
> 仓库中只保留 `config/config.example.yaml` 作为参考模板。

编辑 `config/config.yaml`：

```yaml
# 代理监听地址（HTTP + SOCKS5 共用同一端口，自动识别协议）
proxy:
  addr: "127.0.0.1:1080"

# CA 证书路径（第一步生成的证书）
tls:
  ca_cert: "./certs/ca.crt"
  ca_key: "./certs/ca.key"

# 域名转发规则
# source: 客户端请求的原始域名
# target: 实际转发到的目标域名（你的反代服务）
rules:
  - source: "api.telegram.org"
    target: "your-proxy.example.com"
```

你可以添加多条转发规则：

```yaml
rules:
  - source: "api.telegram.org"
    target: "your-proxy.example.com"
  - source: "api.openai.com"
    target: "openai-proxy.example.com"
  - source: "raw.githubusercontent.com"
    target: "ghproxy.example.com"
```

**未在规则中列出的域名不会被拦截**，代理会直接透传（标准代理行为），不会解密其 HTTPS 流量。

### 第三步：启动代理服务器

```bash
# 使用默认配置文件路径 ./config/config.yaml
./domain-proxy run

# 或指定配置文件
./domain-proxy run --config /path/to/config.yaml
```

启动成功后会输出：

```
time=... level=INFO msg="loaded rewrite rules" count=1
time=... level=INFO msg=rule source=api.telegram.org target=your-proxy.example.com
time=... level=INFO msg="proxy listening (HTTP + SOCKS5)" addr=127.0.0.1:1080
```

如需后台运行：

```bash
# Linux - 使用 nohup
nohup ./domain-proxy run &

# Linux - 使用 systemd（见下方 systemd 配置）

# Windows - 直接在另一个终端窗口运行
start domain-proxy.exe run
```

#### systemd 服务配置（可选）

创建 `/etc/systemd/system/domain-proxy.service`：

```ini
[Unit]
Description=Domain Proxy MITM Server
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/domain-proxy
ExecStart=/opt/domain-proxy/domain-proxy run --config /opt/domain-proxy/config.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable domain-proxy
sudo systemctl start domain-proxy
sudo systemctl status domain-proxy
```

### 第四步：测试代理

假设你的配置中已经添加了 `api.telegram.org` → `your-proxy.example.com` 的转发规则。

#### 快速测试（跳过证书验证）

```bash
# 通过 HTTP 代理访问
curl -x http://127.0.0.1:1080 -k \
  https://api.telegram.org/bot<YOUR_TOKEN>/getMe

# 通过 SOCKS5 代理访问（同一端口，注意使用 socks5h://）
curl -x socks5h://127.0.0.1:1080 -k \
  https://api.telegram.org/bot<YOUR_TOKEN>/getMe
```

#### 对比验证

```bash
# 通过代理访问（流量经过 MITM 转发到 your-proxy.example.com）
curl -x http://127.0.0.1:1080 -k \
  https://api.telegram.org/bot<YOUR_TOKEN>/getMe

# 直接访问反代服务（不经过代理）
curl https://your-proxy.example.com/bot<YOUR_TOKEN>/getMe

# 两个命令的 JSON 输出应该完全一致
```

#### 使用 CA 证书验证（推荐）

安装 CA 证书到系统后（见下一章节），无需 `-k` 标志：

```bash
curl -x http://127.0.0.1:1080 https://api.telegram.org/bot<YOUR_TOKEN>/getMe
```

## 安装 CA 证书到系统

将 CA 根证书安装到操作系统的信任存储后，所有使用系统证书存储的程序（curl、Python requests、Node.js 等）都会自动信任代理签发的证书，无需额外配置。

### Linux (Debian / Ubuntu)

```bash
# 复制证书到系统证书目录
sudo cp certs/ca.crt /usr/local/share/ca-certificates/domain-proxy-ca.crt

# 更新系统证书存储
sudo update-ca-certificates

# 验证
curl -x http://127.0.0.1:1080 https://api.telegram.org/...
# 不需要 -k，能正常返回则说明安装成功
```

卸载：

```bash
sudo rm /usr/local/share/ca-certificates/domain-proxy-ca.crt
sudo update-ca-certificates --fresh
```

### Linux (CentOS / RHEL / Fedora)

```bash
sudo cp certs/ca.crt /etc/pki/ca-trust/source/anchors/domain-proxy-ca.crt
sudo update-ca-trust
```

### Linux (Alpine)

```bash
apk add ca-certificates
cp certs/ca.crt /usr/local/share/ca-certificates/domain-proxy-ca.crt
update-ca-certificates
```

### macOS

```bash
sudo security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain certs/ca.crt
```

卸载：打开"钥匙串访问" → "系统" → 找到 "Domain Proxy CA" → 删除。

### Windows

方式一：图形界面

1. 双击 `ca.crt` 文件
2. 点击"安装证书"
3. 选择"本地计算机" → 下一步
4. 选择"将所有的证书都放入下列存储"
5. 点击"浏览" → 选择"受信任的根证书颁发机构"
6. 完成

方式二：命令行（需要管理员权限）

```powershell
certutil -addstore "Root" certs\ca.crt
```

卸载：

```powershell
certutil -delstore "Root" "Domain Proxy CA"
```

> **Windows 注意事项**：Windows 的 curl 使用 Schannel 后端，即使安装了 CA 到系统，
> 可能仍因证书吊销检查失败。此时可添加 `--ssl-no-revoke` 标志。
> 这个问题仅影响 Windows 上的 curl，不影响 Linux/macOS 或其他编程语言的 HTTP 库。

## 在 Docker 中使用

### 方式一：代理运行在宿主机，应用在容器内

代理在宿主机上运行，容器内的应用通过环境变量使用代理。

**1. 宿主机上启动代理**

```bash
# 监听地址改为 0.0.0.0，让容器能访问
# config.yaml 中设置:
#   proxy:
#     addr: "0.0.0.0:1080"

./domain-proxy run
```

**2. 容器的 Dockerfile 中安装 CA 证书**

```dockerfile
FROM python:3.12-slim

# 安装 CA 证书到系统信任存储
COPY certs/ca.crt /usr/local/share/ca-certificates/domain-proxy-ca.crt
RUN update-ca-certificates

# 复制应用代码
COPY . /app
WORKDIR /app
RUN pip install -r requirements.txt

CMD ["python", "app.py"]
```

**3. 运行容器，设置代理环境变量**

```bash
docker run \
  -e HTTP_PROXY=http://host.docker.internal:1080 \
  -e HTTPS_PROXY=http://host.docker.internal:1080 \
  my-app
```

> `host.docker.internal` 在 Docker Desktop（Windows/macOS）中自动可用。
> Linux 上需要添加 `--add-host=host.docker.internal:host-gateway` 参数。

### 方式二：代理和应用都在容器内（Docker Compose）

**Dockerfile（代理）**

```dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /build
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o domain-proxy .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /build/domain-proxy /usr/local/bin/domain-proxy
COPY config/config.yaml /etc/domain-proxy/config.yaml
COPY certs/ /etc/domain-proxy/certs/
CMD ["domain-proxy", "run", "--config", "/etc/domain-proxy/config.yaml"]
```

**Dockerfile（应用，以 Python 为例）**

```dockerfile
FROM python:3.12-slim

# 安装代理的 CA 证书
COPY certs/ca.crt /usr/local/share/ca-certificates/domain-proxy-ca.crt
RUN update-ca-certificates

COPY . /app
WORKDIR /app
RUN pip install -r requirements.txt
CMD ["python", "app.py"]
```

**docker-compose.yml**

```yaml
services:
  proxy:
    build:
      context: ./domain-proxy
      dockerfile: Dockerfile
    ports:
      - "1080:1080"
    volumes:
      - ./domain-proxy/config/config.yaml:/etc/domain-proxy/config.yaml
      - ./domain-proxy/certs:/etc/domain-proxy/certs

  app:
    build:
      context: ./my-app
      dockerfile: Dockerfile
    environment:
      - HTTP_PROXY=http://proxy:1080
      - HTTPS_PROXY=http://proxy:1080
    depends_on:
      - proxy
```

注意 `config.yaml` 中的监听地址需要改为 `0.0.0.0`：

```yaml
proxy:
  addr: "0.0.0.0:1080"

tls:
  ca_cert: "/etc/domain-proxy/certs/ca.crt"
  ca_key: "/etc/domain-proxy/certs/ca.key"

rules:
  - source: "api.telegram.org"
    target: "your-proxy.example.com"
```

```bash
docker compose up -d
```

## 在应用程序中使用

### 环境变量方式（推荐）

大多数 HTTP 库都支持通过环境变量自动使用代理，这是最简单的方式：

```bash
export HTTP_PROXY=http://127.0.0.1:1080
export HTTPS_PROXY=http://127.0.0.1:1080

# 之后该 shell 中运行的所有程序都会自动使用代理
python app.py
node app.js
curl https://api.telegram.org/...
```

如果不想对所有域名都走代理，可以设置 `NO_PROXY` 排除某些域名：

```bash
export NO_PROXY=localhost,127.0.0.1,.example.com
```

### Python

**方式一：环境变量（推荐，无需改代码）**

```bash
export HTTPS_PROXY=http://127.0.0.1:1080
python app.py
```

```python
import requests

# 代码不需要任何修改，requests 自动读取 HTTPS_PROXY 环境变量
response = requests.get("https://api.telegram.org/bot<TOKEN>/getMe")
print(response.json())
```

**方式二：代码中显式设置代理**

```python
import requests

proxies = {
    "http": "http://127.0.0.1:1080",
    "https": "http://127.0.0.1:1080",
}

response = requests.get(
    "https://api.telegram.org/bot<TOKEN>/getMe",
    proxies=proxies,
)
print(response.json())
```

**方式三：使用 SOCKS5 代理**

```bash
pip install requests[socks]
```

```python
import requests

proxies = {
    "http": "socks5h://127.0.0.1:1080",
    "https": "socks5h://127.0.0.1:1080",
}

response = requests.get(
    "https://api.telegram.org/bot<TOKEN>/getMe",
    proxies=proxies,
)
print(response.json())
```

### Node.js

**方式一：环境变量 + undici/node-fetch**

```bash
export HTTPS_PROXY=http://127.0.0.1:1080
node app.js
```

**方式二：使用 https-proxy-agent**

```bash
npm install https-proxy-agent
```

```javascript
const { HttpsProxyAgent } = require('https-proxy-agent');

const agent = new HttpsProxyAgent('http://127.0.0.1:1080');

const response = await fetch('https://api.telegram.org/bot<TOKEN>/getMe', {
  agent,
});
const data = await response.json();
console.log(data);
```

**方式三：axios**

```javascript
const axios = require('axios');

const response = await axios.get('https://api.telegram.org/bot<TOKEN>/getMe', {
  proxy: {
    host: '127.0.0.1',
    port: 1080,
    protocol: 'http',
  },
});
console.log(response.data);
```

### Go

```go
package main

import (
    "fmt"
    "io"
    "net/http"
    "net/url"
)

func main() {
    proxyURL, _ := url.Parse("http://127.0.0.1:1080")
    client := &http.Client{
        Transport: &http.Transport{
            Proxy: http.ProxyURL(proxyURL),
        },
    }

    resp, err := client.Get("https://api.telegram.org/bot<TOKEN>/getMe")
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()

    body, _ := io.ReadAll(resp.Body)
    fmt.Println(string(body))
}
```

或通过环境变量（`http.DefaultTransport` 自动读取）：

```bash
export HTTPS_PROXY=http://127.0.0.1:1080
go run main.go
```

### Java

```java
System.setProperty("http.proxyHost", "127.0.0.1");
System.setProperty("http.proxyPort", "1080");
System.setProperty("https.proxyHost", "127.0.0.1");
System.setProperty("https.proxyPort", "1080");

HttpClient client = HttpClient.newHttpClient();
HttpRequest request = HttpRequest.newBuilder()
    .uri(URI.create("https://api.telegram.org/bot<TOKEN>/getMe"))
    .build();
HttpResponse<String> response = client.send(request, HttpResponse.BodyHandlers.ofString());
System.out.println(response.body());
```

> **注意**：Java 使用自己的证书存储（cacerts），需要额外导入 CA 证书：
> ```bash
> keytool -importcert -alias domain-proxy-ca \
>   -file certs/ca.crt \
>   -keystore $JAVA_HOME/lib/security/cacerts \
>   -storepass changeit -noprompt
> ```

### curl

```bash
# HTTP 代理
curl -x http://127.0.0.1:1080 https://api.telegram.org/...

# SOCKS5 代理（同一端口，必须用 socks5h:// 进行远程 DNS 解析）
curl -x socks5h://127.0.0.1:1080 https://api.telegram.org/...
```

### wget

```bash
# 通过环境变量
export https_proxy=http://127.0.0.1:1080
wget https://api.telegram.org/bot<TOKEN>/getMe

# 或命令行参数
wget -e https_proxy=http://127.0.0.1:1080 https://api.telegram.org/bot<TOKEN>/getMe
```

## 配置文件详解

配置文件使用 YAML 格式，以下是完整的配置项说明：

```yaml
# 代理服务器监听地址
proxy:
  # HTTP + SOCKS5 共用地址
  # 同一端口自动识别客户端使用的协议（通过首字节探测）
  # 默认值: "127.0.0.1:1080"
  # 设为 "0.0.0.0:1080" 可以允许外部访问（Docker 场景需要）
  addr: "127.0.0.1:1080"

# TLS 证书配置
tls:
  # CA 根证书路径（由 gencert 命令生成）
  # 默认值: "./certs/ca.crt"
  ca_cert: "./certs/ca.crt"

  # CA 私钥路径
  # 默认值: "./certs/ca.key"
  ca_key: "./certs/ca.key"

# 域名转发规则列表
# 只有匹配到规则的域名才会进行 MITM 拦截和转发
# 其他域名会直接透传（标准代理行为，不解密 HTTPS）
rules:
    # source: 客户端请求中的原始域名
    # target: 实际要转发到的目标域名（你的反代服务地址）
  - source: "api.telegram.org"
    target: "your-proxy.example.com"

  # 可以添加任意多条规则
  # - source: "api.openai.com"
  #   target: "openai-proxy.example.com"
  # - source: "raw.githubusercontent.com"
  #   target: "ghproxy.example.com"
```

### 默认值

所有配置项都有默认值，如果你的证书和配置都在默认路径，只需一个最小配置：

```yaml
rules:
  - source: "api.telegram.org"
    target: "your-proxy.example.com"
```

| 配置项 | 默认值 |
|--------|--------|
| `proxy.addr` | `127.0.0.1:1080` |
| `tls.ca_cert` | `./certs/ca.crt` |
| `tls.ca_key` | `./certs/ca.key` |

## CLI 命令参考

```
domain-proxy <command> [options]
```

### gencert — 生成 CA 证书

```
domain-proxy gencert [--out DIR]
```

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--out` | 证书输出目录 | `./certs` |

示例：

```bash
# 输出到默认目录 ./certs/
domain-proxy gencert

# 输出到指定目录
domain-proxy gencert --out /etc/domain-proxy/certs
```

生成文件：
- `<out>/ca.crt` — CA 根证书（ECDSA P-256，有效期 10 年）
- `<out>/ca.key` — CA 私钥（权限 600）

### run — 启动代理服务器

```
domain-proxy run [--config FILE]
```

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--config` | 配置文件路径 | `./config/config.yaml` |

示例：

```bash
# 使用默认配置
domain-proxy run

# 指定配置文件
domain-proxy run --config /etc/domain-proxy/config.yaml
```

## 工作原理

### HTTP 代理 MITM 流程

```
客户端                      代理服务器                       目标服务器
  |                            |                                |
  |-- CONNECT domain:443 ----> |                                |
  |<-- 200 Established ------  |                                |
  |                            |                                |
  |== TLS 握手（CA签发的证书）==> |                                |
  |                            |                                |
  |-- GET /path (Host:domain)-> |  1. 解密请求                    |
  |                            |  2. 匹配转发规则                  |
  |                            |  3. 重写 Host 为 target          |
  |                            |-- GET /path (Host:target) ----> |
  |                            |<-- 响应 ---------------------- |
  |<-- 响应 ----------------- |                                |
```

### SOCKS5 代理 MITM 流程

```
客户端                      代理服务器                       目标服务器
  |                            |                                |
  |-- SOCKS5 握手 -----------> |                                |
  |<-- 握手完成 -------------- |                                |
  |-- CONNECT domain:443 ----> |                                |
  |<-- 连接成功 -------------- |                                |
  |                            |                                |
  |== TLS 握手（CA签发的证书）==> |                                |
  |   ... 后续与 HTTP 代理相同 ...                                |
```

### 匹配规则的域名 vs 不匹配的域名

| | 匹配规则 | 不匹配规则 |
|---|---------|-----------|
| 行为 | MITM 拦截 + 域名重写 | 直接隧道透传 |
| 是否签发证书 | 是 | 否 |
| 是否解密 HTTPS | 是 | 否 |
| 性能开销 | 较高（TLS 解密/加密） | 极低（纯 TCP 转发）|

## 技术细节

### 证书管理

- **CA 根证书**：使用 ECDSA P-256 算法生成，有效期 10 年
- **域名证书**：运行时按需动态签发，有效期 24 小时，SAN（Subject Alternative Name）字段包含请求的域名
- **证书缓存**：同一域名的证书会被缓存在内存中（`sync.Map`），后续请求直接复用，避免重复签发带来的密钥生成和签名运算开销
- 每个不同的域名都有独立的证书，确保客户端的域名验证通过

### SOCKS5 协议实现

- 完整实现 RFC 1928 协议
- 支持无认证模式（`NO AUTHENTICATION REQUIRED`）
- 支持 `CONNECT` 命令
- 支持 IPv4、IPv6、域名三种地址类型
- **重要**：客户端必须使用 `socks5h://`（远程 DNS 解析），否则代理只能看到 IP 地址，无法匹配域名转发规则

### HTTP 协议处理

- MITM 模式下使用 Go 标准库的 `http.ReadRequest` / `http.ReadResponse` 解析 HTTP 请求和响应，确保协议处理的可靠性
- 普通 HTTP 代理（非 TLS）也支持域名重写

## 项目结构

```
domain-proxy/
├── main.go                 # 程序入口，CLI 命令分发（gencert / run）
├── go.mod                  # Go 模块定义
├── go.sum                  # 依赖校验和
├── config/
│   ├── config.go           # YAML 配置加载与解析
│   └── config.yaml         # 默认配置文件
├── cert/
│   ├── generator.go        # CA 根证书生成（ECDSA P-256）
│   └── store.go            # CA 加载、域名证书动态签发、缓存
├── proxy/
│   ├── server.go           # 统一入口，首字节探测自动分发 HTTP / SOCKS5
│   ├── http_proxy.go       # HTTP 代理服务器（CONNECT + 普通 HTTP）
│   ├── socks5_proxy.go     # SOCKS5 协议处理（RFC 1928）
│   ├── mitm.go             # MITM 拦截核心（TLS 握手、请求转发）
│   └── rewriter.go         # 域名匹配与重写引擎
├── certs/                  # 生成的 CA 证书（运行时创建）
│   ├── ca.crt
│   └── ca.key
└── README.md
```

## 常见问题

### Q: curl 报 `SEC_E_UNTRUSTED_ROOT` 或 `CERT_TRUST_REVOCATION_STATUS_UNKNOWN` 错误

**A:** 这是 Windows 特有的问题。Windows curl 使用 Schannel 后端，会强制检查证书吊销状态（CRL/OCSP），而自签名 CA 没有 CRL。解决方案：

```bash
# 方案1：添加 --ssl-no-revoke 标志
curl -x http://127.0.0.1:1080 --ssl-no-revoke https://...

# 方案2：使用 -k 跳过证书验证（仅调试用）
curl -x http://127.0.0.1:1080 -k https://...
```

这个问题**不影响 Linux/macOS**，也不影响 Python、Node.js、Go 等编程语言的 HTTP 库（它们使用 OpenSSL，默认不检查 CRL）。

### Q: SOCKS5 代理无效，请求超时

**A:** 确保使用 `socks5h://` 而不是 `socks5://`。

- `socks5://` — 客户端本地解析 DNS，向代理发送 IP 地址 → 代理无法匹配域名转发规则
- `socks5h://` — 代理端解析 DNS，客户端发送域名 → 代理可以匹配并转发

```bash
# 错误
curl -x socks5://127.0.0.1:1080 https://api.telegram.org/...

# 正确
curl -x socks5h://127.0.0.1:1080 https://api.telegram.org/...
```

### Q: 应用程序报 SSL 证书验证错误

**A:** 确认 CA 证书已正确安装到系统信任存储。参考[安装 CA 证书到系统](#安装-ca-证书到系统)章节。

对于 Docker 环境，确认 Dockerfile 中有：

```dockerfile
COPY ca.crt /usr/local/share/ca-certificates/domain-proxy-ca.crt
RUN update-ca-certificates
```

对于 Java，需要额外导入到 Java 证书存储：

```bash
keytool -importcert -alias domain-proxy-ca \
  -file ca.crt -keystore $JAVA_HOME/lib/security/cacerts \
  -storepass changeit -noprompt
```

### Q: 对不需要转发的域名有影响吗？

**A:** 没有。未配置转发规则的域名会被直接透传（纯 TCP 隧道），代理不会解密其 HTTPS 流量，也不会签发证书。行为与普通代理完全相同。

### Q: 能把已有的 CA 证书用在这个代理上吗？

**A:** 可以。只需在 `config.yaml` 中指定你的 CA 证书和私钥路径即可：

```yaml
tls:
  ca_cert: "/path/to/your/ca.crt"
  ca_key: "/path/to/your/ca.key"
```

要求：CA 证书必须是 PEM 格式，私钥必须是 ECDSA 类型。

### Q: HTTP 和 SOCKS5 可以分开端口吗？

**A:** 当前版本 HTTP 和 SOCKS5 共用同一端口，通过首字节自动探测协议类型（SOCKS5 首字节为 `0x05`，HTTP 为 ASCII 字母）。如果需要不同端口，可以运行多个实例并指定不同的配置文件。

## 安全警告

- **CA 私钥**（`ca.key`）必须妥善保管，绝不能泄露或提交到版本控制系统中。拥有该私钥的人可以签发任何域名的有效证书
- 本工具仅用于合法用途：调试、测试、开发环境中规避网络限制等
- 不要在不信任的网络上暴露代理端口，不要在生产环境中使用
- 建议仅在本地（`127.0.0.1`）或可信内网中监听
- 转发规则中的目标域名必须是你信任的反代服务
- 建议将 `certs/` 目录添加到 `.gitignore` 中

## License

MIT
