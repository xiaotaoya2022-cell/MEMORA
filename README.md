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

## 项目状态

概念与定位阶段：仓库目前只包含项目定义、设计理念与边界，尚未进入实现。

待定事项：

- 许可证（开源前确定）；
- 技术栈与实现路径（需在真实机器上完成内存异常的复现与归因后再决定）；
- 可观测性设计：如何证明"没有介入"也是正确行为。

## 参与

如果你在 Windows 上遇到过类似现象——进程已经结束、内存却没有归还，工具看不到、只能靠重启解决——欢迎带着可复现的观察与数据来讨论。MEMORA 的判断必须建立在证据之上，而不是建立在"清了就会更快"的经验之上。

---

## In English

MEMORA is an open-source memory guardian engine for Windows. It targets abnormal memory growth, resource retention, memory leaks and declining memory efficiency during long-running sessions — not by frequent cleaning or forcing working sets down for a momentary dip, but by watching how memory is actually used, finding where the abnormality comes from, and acting only when it is genuinely needed.

It is not meant to give a machine more memory than its hardware has. It is meant to let a machine use the memory it already owns, properly.

**Memory, Back to Its Nature. Always Watching. Never Intruding.**
