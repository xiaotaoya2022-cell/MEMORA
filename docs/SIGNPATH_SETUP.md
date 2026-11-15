# SignPath Foundation 开源代码签名接入指南

MEMORA 已配置好面向 **SignPath Foundation**（开源项目免费、受微软原生信任的 Authenticode 代码签名）的 GitHub Actions 自动签名流水线：[release-signpath.yml](file:///f:/MEMORA/.github/workflows/release-signpath.yml)。

## 为什么选择 SignPath Foundation？
- 普通用户从浏览器下载安装 `.exe` 时，Windows Defender SmartScreen 会核验签名证书是否来自**微软公信根证书库**。
- 本机自签名证书无法通过 SmartScreen 云端信誉校验；而 SignPath Foundation 为符合条件的开源项目免费提供**受微软信任的代码签名证书**，并在 GitHub Actions 中自动完成签名，消除普通用户双击时的拦截弹窗。

## 接入步骤（只需一次）

1. **确认仓库公开且含开源协议**
   - 仓库：`https://github.com/xiaotaoya2022-cell/MEMORA`（确保为 Public，根目录已包含 `LICENSE` 与 `README.md`）。

2. **申请 SignPath Foundation 免费开源计划**
   - 前往 [SignPath Open Source](https://about.signpath.io/product/open-source) 提交申请，填写 GitHub 仓库地址与项目说明。
   - 审核通过后登录 SignPath 控制台：
     - 绑定 GitHub Connector（授权 `xiaotaoya2022-cell/MEMORA`）。
     - 创建 Project（Slug 建议设为 `memora`），配置 Artifact Configuration 匹配 `unsigned-memora-exe` 与 `unsigned-installer`（`.exe` 文件），并关联 `release-signing` 策略。

3. **在 GitHub 仓库配置 Secrets 与 Variables**
   打开 GitHub 仓库 `Settings -> Secrets and variables -> Actions`：
   - **Repository secret**：
     - `SIGNPATH_API_TOKEN`：SignPath 控制台生成的 CI/CD API Token
   - **Repository variables**：
     - `SIGNPATH_ORGANIZATION_ID`：SignPath 组织 UUID
     - `SIGNPATH_PROJECT_SLUG`：`memora`（可选，默认即 `memora`）
     - `SIGNPATH_SIGNING_POLICY_SLUG`：`release-signing`（可选，默认即 `release-signing`）

4. **触发正式签名发版**
   ```powershell
   git tag v0.3.0
   git push origin v0.3.0
   ```
   GitHub Actions 会自动：
   1. 运行 `go vet ./...` 与全量 `go test -count=1 ./...`；
   2. 构建无签名的 `bin/memora.exe` 并提交 SignPath 完成正式代码签名；
   3. 用 SignPath 已签名的 `memora.exe` 打包 NSIS 安装器，再提交 SignPath 对安装器 `.exe` 完成正式代码签名；
   4. 自动生成 `SHA256SUMS-v0.3.0.txt` 并发布到 GitHub Releases。
