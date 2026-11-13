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

MEMORA 应该像系统中一个安静工作的守护者，长期驻留，持续感知，在真正需要时采取行动。

它不应该让你的电脑拥有超出硬件规格的内存。

它应该让你的电脑更合理地使用本来就拥有的内存。

**Your Memory. Your System. Naturally Efficient.**

## 快速开始

需要 Go 1.21 或更新版本，以及 Windows 10/11。

```powershell
.\build.ps1                        # 产出 bin\memora.exe（单文件、无运行时依赖）
.\bin\memora.exe status            # 看一眼当前的内存形态（含完整进程、Vid/NUMA、内核异常摘要）
.\bin\memora.exe status --deep     # 深度诊断：扫描已加载驱动以匹配异常 Pool Tag 对应的 .sys
.\bin\memora.exe doctor --minutes 10   # 观察一段时间，算斜率并做进程创建/Pool Tag 增量归因
.\bin\memora.exe watch --observe   # 前台观察模式：只记录，不做任何动作
.\bin\memora.exe watch             # 守护模式：只有确实需要时才动作
.\bin\memora.exe startup enable    # 开机自启（写入 HKCU Run，可随时 disable）
```

`status`、`doctor` 与 `trace` 不会修改系统，可以放心跑。第一次使用建议先用 `watch --observe`
观察一两天，确认它判断的对象和你的实际感受一致，再切到守护模式。

调试时可以用 `watch --only-pid <PID>` 把动作范围收窄到单个进程，正常使用不需要它。

## 命令

| 命令 | 作用 |
|---|---|
| `status [--json] [--deep]` | 物理内存、提交、内核池、页链、`Vid`/NUMA、完整进程与套件排行、内核对象/Pool Tag；`--deep` 匹配驱动 `.sys` |
| `watch [--observe\|--quiet\|--hidden] [--interval D] [--only-pid PID]` | 常驻守护；`--observe` 只记录不动手，`--hidden` 隐藏控制台，`--interval` 覆盖周期 |
| `doctor [--minutes N] [--interval D]` | 采样一段时间，算斜率，追踪进程创建/释放、`Proc`/Pool Tag 增量并匹配驱动给出结论 |
| `trace [--minutes N] [--interval D] [--out F]` | 把每次采样写成 JSONL，便于外部比对 |
| `journal [--lines N] [--path]` | 读动作日志或打印日志路径，看它到底动过什么、回收了多少 |
| `config [--reset]` | 查看或重置配置 |
| `startup enable\|disable\|status` | 开机自启管理 |
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

当前发布为 **v0.1.0 CLI 技术预览版（Pre-release，无 GUI / 托盘界面）**。核心采集、无盲区进程枚举、
决策执行、日志、`Vid`/NUMA 监测、内核僵尸进程对象泄漏识别、`Pool Tag` 与驱动 `.sys` 匹配已落地并可运行；
单文件静态链接（零第三方 Go 依赖），常驻约 7 MB。

当前能力边界与设计取舍：

- **纯命令行形态**：需在 PowerShell / 终端中携带子命令运行（例如 `memora.exe status --deep` 或 `memora.exe doctor`）；直接双击 `.exe` 只会打印用法后退出。
- **用户态安全守护**：默认守护模式只按多道门控回收长期空闲用户进程的工作集，清空待机缓存默认关闭；绝不强杀进程，也不会对 `System` / `vmmemWSL` / 安全核心进程下手。
- **内核与虚拟化问题诚实归因**：对于内核驱动引用泄漏（僵尸 `EPROCESS`、驱动非分页池不释放）和 `Vid` 锁页，MEMORA 负责提供免提权的实机取证与驱动/宿主指认，不会尝试在用户态篡改内核对象引用或强卸驱动——这类根因需靠更新/停用问题驱动、调整 WSL 内存配置或重启系统解决。

开源协议与后续规划：

- 开源协议：[MIT License](LICENSE)；
- 尚未完成：原生轻量托盘/图形界面、安装包与代码签名（当前 Release 二进制未签名，首次运行如触发 SmartScreen 可自行运行 `.\build.ps1` 从源码编译并核对 `SHA256SUMS.txt`）。

## 参与

如果你在 Windows 上遇到过类似现象——进程已经结束、内存却没有归还，工具看不到、只能靠重启解决——欢迎带着可复现的观察与数据来讨论。MEMORA 的判断必须建立在证据之上，而不是建立在"清了就会更快"的经验之上。

---

## In English

MEMORA is an open-source memory guardian engine for Windows. It targets abnormal memory growth, resource retention, kernel leaks, virtualization lockups, and declining memory efficiency during long-running sessions — not by frequent cleaning or forcing working sets down for a momentary dip, but by reading kernel-grade telemetry without privilege blind spots, finding where the abnormality actually lives (`SystemProcessInformation`, Pool Tags + loaded `.sys` attribution, kernel object leak detection, Hyper-V/WSL2 `Vid` locked pages, NUMA node balance), and managing user-space working sets only when genuinely needed.

It is not meant to give a machine more memory than its hardware has. It is meant to let a machine explain and use the memory it already owns, properly.

**Memory, Back to Its Nature. Always Watching. Never Intruding.**
