<p align="right">
  <a href="README_EN.md">English</a> | <strong>简体中文</strong>
</p>

<p align="center">
  <img src="assets/brand/cover.jpg" width="760" alt="MEMORA — Memory, Back to Its Nature.">
</p>

# MEMORA

**Memory, Back to Its Nature.**
**Always Watching. Never Intruding.**

面向 Windows 的开源智能内存守护引擎。

MEMORA 致力于解决 Windows 长时间运行过程中可能出现的内存资源异常增长、资源滞留、内存泄漏以及系统内存利用效率下降等问题。

不同于传统内存清理软件，MEMORA 不追求通过频繁清理、强制释放工作集或制造短暂的内存占用下降来展示优化效果。它更关注内存占用背后的真实原因：通过持续监测系统内存状态、分析进程资源使用情况、识别异常内存行为，并结合安全策略进行自适应管理，让 Windows 在长期运行后依然保持健康、稳定的内存状态。

---

## 项目起点

这不是一个凭空设想的需求，它来自一台真实机器上反复出现、却很难被现有工具解释的现象：

- 空载内存占用约 9–10 GB；
- 运行过一批软件后，即使从托盘退出、在任务管理器里彻底结束进程，占用也不会回到原来的水平；
- 静置十小时甚至一天之后，占用会自行"虚高"，此时没有任何可见进程在明显增长；
- 系统自带工具与第三方管家类软件的后台列表都看不出异常，手工把可见的内存占用相加，结果也远小于实际被占用的部分；
- 唯一有效的解法是重启。

问题不在于"内存不够用"，而在于内存被谁拿着、为什么没有归还、以及这些工具为什么看不见它。MEMORA 要回答的就是这个问题。

## Core Philosophy

- **Intelligent Monitoring** — 持续感知系统内存状态，而不是被动等待用户操作。
- **Root Cause Analysis** — 关注内存异常的真实来源，而不只是表面占用数字。
- **Adaptive Management** — 根据系统状态动态制定管理策略，避免无差别清理。
- **Zero Interference** — 将正常进程的稳定运行和多任务体验放在首位。
- **Minimal Footprint** — 尽可能降低自身 CPU、内存和磁盘资源消耗。
- **Open & Transparent** — 保持开源、可审计，让每一次管理行为都有依据。
- **Long-Term Stewardship** — 坚持长期主义，用跨天、跨周的静默账本与主动克制记录证明价值，不靠短暂数字刷存在感。

## 做什么，不做什么

**MEMORA 会做：**

- 长期驻留、静默运行，把感知放在前面，把动作放在后面；
- 定位异常增长的真实来源，而不是只处理表面数字；
- 在真正需要的时候介入，并且每一次介入都可解释、可追溯；
- 确保退出之后，占用的空间真正归还给系统；
- 以极低的资源占用运行，让配置很差的机器也能带得动；
- 用一个不需要用户操心的机制，替代管家类工具里所谓"AI 内存动态调整"的角色。

**MEMORA 不会做：**

- 不做一键清理，不做定时批量扫描与批量释放；
- 不用强制清空工作集的方式制造"内存一下子变多"的错觉；
- 不为了数字好看而牺牲正在运行的多任务进程；
- 不做常驻弹窗、推广与打扰，存在感低到你不会意识到它起了作用；
- 不承诺把 8 GB 变成 16 GB。

## Our Vision

我们不希望用户每天打开 MEMORA，也不希望用户不断点击内存清理按钮。

MEMORA 应该像系统中一个安静工作的守护者，长期驻留，持续感知，在真正需要时采取行动。它也追求**长期主义**的陪伴感：你不需要天天盯着它，但隔几周或一两个月偶尔打开一次时，能够在一张冷静的「静默账本」里，清楚看到这段时间以来它护航了多久、有多少比例的时间选择了主动克制不打扰、拦截或缓解过多少次真实压力峰、收敛过多少组主程序退出后的后台残留，以及排查出过哪些平时看不见的驱动/虚机暗账。

它不应该让你的电脑拥有超出硬件规格的内存。

它应该让你的电脑更合理地使用本来就拥有的内存。

**Your Memory. Your System. Naturally Efficient.**

## 真实运行实况与界面展示 (160+ 小时实测)

> [!NOTE]
> 下列运行数据来自生产环境 Windows 11 主机连续运行 **162.9 小时（约 7 天）** 的真实实录（无重启）。
> MEMORA 自身常驻仅占 ~41 MB 工作集与 790 句柄，始终遵循第一性原理与必要性原则进行自适应管理。

### 1. 智能感知与动态调度中心 (Smart Sensing & Dynamic Dispatch)

<p align="center">
  <img src="assets/screenshots/dashboard-smart-dispatch.jpg" width="100%" alt="MEMORA 智能感知与动态调度面板">
</p>

- **Porcelain Daylight Bento 设计语言**：遵循冷瓷白（Porcelain White）极简工程审美与 `Bahnschrift + Segoe UI Variable` 紧凑排版，右上角原生托管 61 个后台闲置进程，支持一键导出 24 小时可审计诊断报告。
- **五维物理内存真实拆解**：告别任务管理器单一工作集的虚幻统计，以彩色标尺还原 39.2 GB 物理内存的真实分布——
  - **前台与活跃独占**：3.4 GB (8.6%)，严格实施前沿进程家族链保护，永不误碰正在交互的应用；
  - **后台静默驻留**：1.5 GB (3.9%)，锁定长期挂起、脱离前台焦点的后台工作集；
  - **僵尸 / 内核泄漏**：0 MB (0.0%)，实时监视内核 EPROCESS 对象与 Pool Tag 泄露；
  - **常规内核 · 共享与缓存**：5.2 GB (13.3%)，涵盖系统文件映射与内核非分页/分页池开销；
  - **即刻可用内存 (含待机)**：29.1 GB (74.1%)，让系统随时拥有充足的高速内存缓冲。
- **六项核心自适应调度策略矩阵**：
  - **智能内存分级调度**：运行于「一级 · 轻度巡航」，累计自适应调节释放 **635.34 GB**，并结合防反扑自校准机制自动保护 33 项敏感进程；
  - **后台进程 CPU / EcoQoS 能效调度**：温和接管 61 个非前沿闲置后台进程，降低背景 CPU 周期与能耗；
  - **子进程派生风暴压制**：实时监控高频短命派生（脉冲波形），累计压制恶意刷新与风暴派生 **15,297 次**，长期沉淀压制名单 7 项；
  - **待机缓存与修改页智能清洗**：针对 28.4 GB 待机缓存实现高压临界动态解压，防止大文件读写挤兑前台游戏与生产力；
  - **全屏游戏 / 创作自动静音护航**：智能识别真全屏独占，累计静音避让 **20,780 次**，游戏与 3D 渲染期间绝对零打扰、零掉帧；
  - **贴边真相光尺 (Edge Horizon)**：屏幕边缘仅占 4px，自由拖拽换边吸附，鼠标悬停即刻展现实时 CPU/GPU 核心温度与深释回环。

---

### 2. 长期静默账本与 7 天基线守恒 (Quiet Ledger & Baseline Conservation)

<p align="center">
  <img src="assets/screenshots/dashboard-quiet-ledger.jpg" width="100%" alt="MEMORA 长期静默账本与 7 天基线守恒">
</p>

- **162.9 小时静默守护 & 94.8% 系统克制率**：
  - **累计守护时长**：162.9 小时无重启稳定护航；
  - **系统克制率**：**94.8%** 的采样周期内选择主动克制不干预，彻底杜绝传统管家类软件高频乱杀引起的磁盘 I/O 抖动与进程卡顿；
  - **累计动态调度释放**：635.34 GB；
  - **高压急救 / 工作集收紧**：3,012 / 3,042 次，在系统内存水位告警边缘精准回血；
  - **全屏 / 游戏静音避让**：20,781 次真全屏避让；
  - **孤儿链与风暴阻断**：成功捕获并收敛 23 个父进程退出后残留的孤儿链，阻断 15,297 次派生宿主风暴；
  - **内核异常目击**：1,899 次精准记录内核级 Process 对象膨胀（峰值曾达 353,346 个对象）。
- **7 天本机空载基线守恒对比**：
  - 真实验证系统在 7 天连续多任务使用周期内空载真实占用的变动；
  - 成功将长期累积的稳态虚高从 **22.93 GB 压降并稳定收敛至 6.57 GB**（净优化 **-16.36 GB**），打破 Windows 长时间开机必须重启的恶性循环。
- **后台虚高膨胀罪魁榜 Top 5（原生彩色图标与实测归因）**：
  - 通过本地账本追踪后台应用无序膨胀，采用「防反扑软抑」与「分级收紧」策略，不杀进程、不丢状态：
    - `WeChatAppEx.exe`（微信小程序/内置浏览器）：收敛 37 次，峰值工作集 220 MB，累计节约 **6.19 GB**；
    - `Nexus.exe`：收敛 19 次，峰值 123 MB，累计节约 **5.72 GB**；
    - `douyin.exe`：收敛 24 次，峰值 1,004 MB，累计节约 **5.50 GB**；
    - `Flydigi Space Station.exe`：收敛 28 次，峰值 87 MB，累计节约 **5.05 GB**；
    - `verge-mihomo.exe`：收敛 9 次，峰值 128 MB，累计节约 **3.89 GB**。

---

## 快速开始

需要 Go 1.21 或更新版本，以及 Windows 10/11。

```powershell
.\build.ps1                        # 产出 bin\memora.exe（单文件、无运行时依赖）
.\build.ps1 -Installer             # 跑全套校验测试并产出正式 Windows 安装向导安装包 (*-setup.exe)
.\bin\memora.exe status            # 看一眼当前的内存形态（含完整进程、Vid/NUMA、内核异常摘要）
.\bin\memora.exe status --deep     # 深度诊断：扫描已加载驱动以匹配异常 Pool Tag 对应的 .sys
.\bin\memora.exe doctor --minutes 10   # 观察一段时间，算斜率并做进程/无主残留/Pool Tag 增量归因
.\bin\memora.exe watch --observe   # 前台观察模式：只记录，不做任何动作
.\bin\memora.exe watch             # 守护模式：按三级智能调度曲线温和自适应介入
.\bin\memora.exe ui                # 启动/唤醒托盘守护并打开图形化性能调度与真相中心界面
.\bin\memora.exe tray              # 轻量原生系统托盘常驻（双击托盘图标打开控制中心）
.\bin\memora.exe ledger            # 查看「静默账本」（累计护航时长、克制率、峰值护航与基线）
.\bin\memora.exe startup enable --tray   # 托盘模式开机自启（写入 HKCU Run，可随时 disable）
```

`status`、`doctor`、`trace` 与 `ledger` 不会对系统内存做清扫修改，可以放心跑。第一次使用建议先用 `watch --observe` 或 `tray --observe`
观察一两天，确认它判断的对象和你的实际感受一致，再切到守护模式。

调试时可以用 `watch --only-pid <PID>` 把动作范围收窄到单个进程，正常使用不需要它。

## 命令

| 命令 | 作用 |
|---|---|
| `memora`（无参数） / `ui` | 启动/唤醒托盘常驻进程并打开独立应用窗口版 **控制与真相中心界面** |
| `status [--json] [--deep]` | 物理内存、提交、内核池、页链、`Vid`/NUMA、全屏保护态、完整进程与套件排行、内核对象/Pool Tag；`--deep` 匹配驱动 `.sys` |
| `watch [--observe\|--quiet\|--hidden] [--interval D] [--only-pid PID]` | 常驻守护；`--observe` 只记录不动手，`--hidden` 隐藏控制台，`--interval` 覆盖周期 |
| `tray [--observe\|--hidden] [--open-ui]` | 轻量 Win32 原生系统托盘常驻（双击图标打开控制中心，右键支持一键智能调度/切换模式/恢复默认） |
| `doctor [--minutes N] [--interval D]` | 采样一段时间，算斜率，追踪进程创建/释放、主程序退出后的后台无主子进程、`Proc`/Pool Tag 增量并匹配驱动给出结论 |
| `trace [--minutes N] [--interval D] [--out F]` | 把每次采样写成 JSONL，便于外部比对 |
| `ledger [--json] [--reset]` | 查看长期「静默账本」（守护时长、克制率、游戏全屏避让次数、软降级收敛与 30 天低水位基线） |
| `journal [--lines N] [--path]` | 读动作日志或打印日志路径，看它到底动过什么、回收了多少 |
| `config [--reset]` | 查看或重置配置 |
| `startup enable [--tray]\|disable\|status` | 开机自启管理（可指定托盘隐藏启动） |
| `version` | 版本号 |

## 它是怎么判断的

MEMORA 不看"工作集"这个数字下结论。工作集包含共享页，同一个 DLL 被多个进程映射
就会被重复计算，把它相加只会得到偏大的结果；判断主线是提交量与内核/虚机物理独占：

```
未归因提交 = 提交总量 − Σ进程私有提交 − 分页池 − 非分页池
```

进程采集主路径使用内核 `NtQuerySystemInformation(SystemProcessInformation)`，在标准
用户权限下也能完整读取受保护进程、跨会话进程（如 `dwm.exe`、`vmmemWSL`、安全守护进程）
的私有提交与独占工作集，消除传统 `OpenProcess` 枚举的权限盲区；仅在极少数降级回退时
才会出现无权限进程提示。

除提交差额外，`status` 与 `doctor` 还会原生读取普通任务管理器隐藏的三类"虚高"源头：

1. **虚拟化锁页与 NUMA 挤兑**：通过性能计数器读取 Hyper-V / WSL2 `Vid` 分区锁住的物理页与各 NUMA 节点余量；
2. **内核对象引用泄漏**：比对内核 `Process` / `Token` / `Job` 对象数与活跃进程/句柄数及 `Proc` 池分配释放差，识别驱动未归还 `ObReferenceObject` 导致的僵尸 `EPROCESS` 堆积；
3. **内核池驱动归因**：读取原生 `Pool Tag`（`SystemPoolTagInformation`），在 `--deep` 与 `doctor` 中匹配已加载 `.sys` 驱动二进制，指认异常非分页/分页池增长来源。

增长用最近 30 分钟的最小二乘斜率衡量，单位 MB/小时。单点高低不算异常，持续增长才算。

设计细节见 [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)，安全边界见
[docs/SAFETY.md](docs/SAFETY.md)，Windows 内存为什么"不回落"见
[docs/DIAGNOSIS.md](docs/DIAGNOSIS.md)。

## 实测

在一台长时间运行的 40 GB 内存（39.2 GB 可用）Windows 11 Pro 机器上执行 `memora status --deep`：

- 物理内存 39.2 GB，使用 26.2 GB（66.9%），可用 13.0 GB；
- 403 个进程完整读取：工作集合计 7.2 GB（独占 4.6 GB），私有提交合计 24.3 GB；
- Hyper-V / WSL2 `Vid` 虚机锁页达 6.0 GB（不计入任何进程工作集），使 NUMA Node0 可用仅剩 52 / 15843 MB；
- 内核分页池 2.1 GB、非分页池 2.8 GB（合计 4.9 GB）；进一步读取内核对象与 Pool Tag 定位到两个隐藏元凶：
  - **僵尸 `EPROCESS` 引用泄漏**：`Process` 对象累积 187,040 个（活跃进程仅 403 个、`Proc` 分配 187,042 次仅释放 2 次），连同 `MiP2`/`Toke` 等残骸共占约 1.4 GB；
  - **驱动非分页池大户**：未知 Pool Tag `ConT` 占 308 MB，经驱动扫描命中 `mtkwl6ex.sys`；
- 待机缓存 11.8 GB，已修改 140 MB，空闲 1.2 GB；
- MEMORA 自身仅占 7 MB 工作集、93 个句柄。

工作集（7.2 GB）、进程私有提交（24.3 GB）、虚机锁页（6.0 GB）与内核泄漏池（4.9 GB）之间的结构性落差，正是传统内存清理器看不见也清不动的原因。

## 项目状态

当前版本为 **v0.3.0**（单文件零外部 Go 依赖 `CGO_ENABLED=0`，含 NSIS 正式安装向导 `*setup.exe` 与独立 **控制与真相中心** 桌面图形界面）。核心采集、无盲区进程枚举、
**三级智能内存调度引擎**（动态软调度 + 高频派生风暴压制 + 一键立即调度）、游戏真全屏避让（修复壁纸类 `QUNS_BUSY` 误判）、前台进程家族链免疫、`ProcessMemoryPriority`/`EcoQoS`/`BelowNormal CPU` 可逆控制、无主残留追踪、`Vid`/NUMA 监测、内核僵尸进程对象泄漏识别、`Pool Tag` 与驱动 `.sys` 匹配以及「静默账本」均已落地运行。

当前能力与设计亮点：

- **双形态入口与贴边真相光尺**：双击桌面图标或直接运行 `memora.exe` 即拉起基于 WebView2 / Chromium 的无边框独立应用窗口（`Porcelain Daylight Bento` 设计语言 + `Bahnschrift + Segoe UI Variable + Noto Sans SC` 精密字体栈），支持按硬件自动适配物理内存五维拆解（虚拟化/开发机展示 `WSL2 / Vid 锁页`，常规主机自动切换为「后台静默驻留」剖析长时闲置进程）；点击右上角「轻量模式」隐藏后 18 秒自动回收 WebView2 渲染宿主与工作集内存。屏幕边缘附带 4px 贴边真相光尺（Edge Horizon），支持全屏四边自由拖拽换边与 120Hz 弹簧贴靠，悬停展示物理内存与 **CPU / GPU 实时核心温度**，单击圆环就地触发深度释压。
- **三级智能动态调度与反扑自校准**：不再等到可用内存跌破 12% 才介入——一级巡航温和降级闲置后台进程并持续监测派生风暴；二级平衡轻修整长时闲置大工作集；三级高压降级至 `VeryLow` 并安全清理低优先级待机缓存与修改页；支持基于前后对比的反扑自校准 (`adaptive_cooldown`) 与 7 天滑动窗口空载基线漂移归因（Top 5 元凶），永不强杀任何第三方软件。
- **诚实内核取证与底层可靠性**：修复 PDH 句柄与 `EnumWindows` 回调长期运行增长问题；对于内核驱动引用泄漏（僵尸 `EPROCESS`）与 `Vid` 锁页提供直连内核数据拆解、涉事驱动 `.sys` 匹配与派生源压制；托盘图标支持连续不闪退操作及实时自占内存汇报。
- **多语言国际化支持 (i18n)**：调度中心与系统托盘深度支持简体中文 (`zh-CN`) 与英文 (`en-US`) 一键即时无缝切换与持久化存储，界面自适应零截断。
- **开源协议与 Authenticode 代码签名**：基于 [MIT License](LICENSE) 开源；构建链路已内嵌权威 `RT_VERSION` / `RT_MANIFEST` 元数据与 SHA-256 Authenticode 数字签名体系（支持环境变量接入外部商业 OV/EV 证书，并内置项目自签名证书与安装向导自动导入 `Root` / `TrustedPublisher` 信任链），同时随版本发布 `SHA256SUMS` 与 `install-trust-cert.cmd` 方便离线一键预置信任或源码复现验证。

## 参与

如果你在 Windows 上遇到过类似现象——进程已经结束、内存却没有归还，工具看不到、只能靠重启解决——欢迎带着可复现的观察与数据来讨论。MEMORA 的判断必须建立在证据之上，而不是建立在"清了就会更快"的经验之上。

---

## English Documentation

Please refer to the complete English documentation at [README_EN.md](README_EN.md).

---

## Code Signing Policy

Free code signing provided by [SignPath.io](https://signpath.io/), using a certificate by [SignPath Foundation](https://signpath.org/).
