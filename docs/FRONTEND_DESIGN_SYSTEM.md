# MEMORA Frontend Design System — Porcelain Daylight Bento

> **版本**：v0.3.0  
> **适用范围**：MEMORA WebView2 主控中心 (`cmd/memora/dashboard_html.go`)、Win32 原生贴边光尺 (`cmd/memora/edge_horizon.go`)、系统托盘轻量浮窗 (`cmd/memora/tray_menu.go`)、托盘矢量高分图标流水线 (`cmd/memora/tray.go`)。

---

## 1. 设计哲学（Design Philosophy）

MEMORA 是一个 Windows 真实物理内存调度与诊断引擎。界面设计服务于三个核心原则：

1. **零存在感与极度克制（Never Intruding）**  
   绝大多数时间内程序在系统托盘静默运行；当用户主动展开或贴边窥视时，视觉呈现必须明亮、冷静、有序，不弹焦虑广告、不用刺眼霓虹告警轰炸用户。
2. **Porcelain Daylight Bento（瓷白昼光微晶 Bento）**  
   以高纯度瓷白卡片（`#FFFFFF`）承载数据模块，铺陈在冷静浅灰蓝画布（`#F1F5F9`）之上；顶部 44px 无边框标题栏以深岩板青黑（`#0F172A`）收束视觉重心，形成「沉稳顶栏 + 明澈仪表台」的专业工程仪器质感。
3. **诚实数据与零 Emoji 约束（Honest Telemetry & Vector-First）**  
   所有百分比、工作集、驱动标签必须严格可溯源、加和守恒；全界面禁用 Unicode Emoji，所有图标均采用自定义线性 SVG（WebView）或像素级 SDF / GDI 矢量绘制（Win32 原生浮窗）。

---

## 2. 色彩与语义 Token 体系（Color Tokens）

### 2.1 基础表面与文字阶梯（Surfaces & Ink）

| Token | Hex / Value | 用途说明 |
| :--- | :--- | :--- |
| `--bg` | `#f1f5f9` | 主画布底色（Slate 100） |
| `--card` | `#ffffff` | 瓷白卡片表面（Card Surface） |
| `--card-sub` | `#f8fafc` | 二级嵌套 Bento 格、开关悬浮底、Sparkline 盒子 |
| `--border` | `#e2e8f0` | 主卡片与分割线 1px 描边（Slate 200） |
| `--border-strong` | `#cbd5e1` | 悬浮浮窗/按钮 hover 描边（Slate 300） |
| `--chrome-dark` | `#0f172a` | 顶部无边框标题栏背景 & 主要正文墨色（Slate 900） |
| `--chrome-border` | `#1e293b` | 顶栏底边分割线（Slate 800） |
| `--text` | `#0f172a` | 核心数值、主标题（Slate 900） |
| `--muted` | `#475569` | 描述文案、副标题、维度单位（Slate 600，高清晰对比度调优） |
| `--subtle` | `#64748b` | 弱化时间戳、占位符、滚动条基础色（Slate 500，高清晰对比度调优） |

### 2.2 五维物理内存拆解与状态语义色（Semantic Accents）

| 语义角色 | 主色 (Stroke / Dot) | 浅底 (Pill / Highlight BG) | 深字 (Pill Ink) | 应用场景 |
| :--- | :--- | :--- | :--- | :--- |
| **MEMORA 品牌 / 前台活跃** | `#2563eb` / `#3b82f6` | `#eff6ff` / `#dbeafe` | `#1d4ed8` | 主调度按钮、滑块、应用活跃独占工作集 |
| **虚机锁页 / 后台驻留** | `#8b5cf6` / `#7c3aed` | `#f5f3ff` / `#ede9fe` | `#6d28d9` | 动态第二槽位（WSL2 Vid 锁页 或 后台静默驻留） |
| **可用内存 / 一级巡航** | `#10b981` | `#ecfdf5` / `#d1fae5` | `#047857` | 可用内存+待机缓存、一级轻度巡航、管理员就绪 |
| **内核分享页 / 二级平衡** | `#f59e0b` | `#fffbeb` / `#fef3c7` | `#b45309` | 常规内核池与共享映射页、子进程派生速率、二级平衡 |
| **僵尸泄漏 / 高压告警** | `#ef4444` | `#fef2f2` / `#fee2e2` | `#dc2626` / `#b91c1c` | 内核对象泄漏、僵尸 EPROCESS、三级高压急救 |
| **静默托管 / 观测中性** | `#64748b` | `#f1f5f9` | `#475569` | 仅观测关闭状态、全屏就绪占位胶囊 |

---

## 3. 排版选型与字体清晰度规范（Typography & Clarity Discipline）

### 3.1 四套系统原生字体候选方案（Font Candidates）

| 方案代号 | 风格定位 | 数值/显示字体 (`--font-num`) | 中文/正文接口字体 (`--font-ui`) | 特点与适用场景 |
| :--- | :--- | :--- | :--- | :--- |
| **方案 A · 精密工程仪器风（已正式选定）** | 高对比度工程仪表风 | `Bahnschrift`, `Segoe UI Variable Display` | `Segoe UI Variable Text`, `Noto Sans SC`, `Microsoft YaHei UI` | 数字笔画利落锐利、高可读等宽对齐，契合 47.4% 环表与 telemetry 气质 |
| **方案 B · 微软现代系统栈** | Win11 原生现代流体 | `Segoe UI Variable Display`, `Segoe UI` | `Segoe UI Variable Text`, `Microsoft YaHei UI` | 极高亲和力与光学字号自适应，与 Windows 11 桌面浑然一体 |
| **方案 C · 终端控制台混排** | 极客硬核开发者风 | `Cascadia Mono`, `Consolas` | `Noto Sans SC`, `Microsoft YaHei UI` | 极强数据柱与过程名（PID / PoolTag / MB）扫描效率 |
| **方案 D · 柔润瓷白工程** | 温润低对比阅读风 | `Segoe UI Variable Display`, `DengXian` | `DengXian`, `Microsoft YaHei UI` | 汉字笔锋舒展柔和，适合长时间连续盯屏诊断 |

### 3.2 正式活跃 CSS Token 栈（方案 A · 精密工程仪器风 — 已正式选定）

- `--font-num`：`"Bahnschrift", "Segoe UI Variable Display", "Segoe UI", "Microsoft YaHei UI", sans-serif`
- `--font-ui`：`"Segoe UI Variable Text", "Noto Sans SC", "Microsoft YaHei UI", "Segoe UI", sans-serif`
- `--font-mono`：`"Cascadia Mono", "Consolas", "Microsoft YaHei UI", monospace`

### 3.3 跨端清晰度硬性准则（WebView2 & Win32 GDI）

1. **禁止在浅色瓷白 WebView2 中使用 `-webkit-font-smoothing: antialiased`**  
   该属性在 Windows DirectWrite 下会强制关闭 LCD 次像素渲染导致浅底黑字“发虚偏瘦”。全局统一使用 `text-rendering: optimizeLegibility; font-optical-sizing: auto;`。
2. **消除半像素字号（Integer Pixel Snap）**  
   禁止使用 `10px / 10.5px / 11.5px / 12.5px` 等小数微字号；所有正文、标签、胶囊字号严格收敛至 `>= 11px` 整数像素阶梯，并提升 `--muted` / `--subtle` 灰度阶梯避免低对比度模糊。
3. **跨机安全字体探测（`resolveInstalledFont`）**  
   Win32 原生浮窗在创建 GDI `HFONT` 前调用 `EnumFontFamiliesExW` 探测本机已安装字体（如 `Bahnschrift` / `Segoe UI Variable Text` / `Noto Sans SC`），若未安装自动优雅回退至 Windows 必装 `Microsoft YaHei UI`（防止回退到点阵宋体 `SimSun`）；温度遥测行指定使用内置标准度数符号 `°`、中点 `·` 与等宽数字字形的 `Segoe UI Variable Text`。
4. **Win32 表面区分渲染品质**  
   - **分层透明浮窗（`edge_horizon.go`）**：使用 `ANTIALIASED_QUALITY` (4) + `>= 11px` 字号，杜绝透明 DIB 边缘 ClearType 彩边。  
   - **不透明托盘浮窗（`tray_menu.go`）**：使用 `CLEARTYPE_QUALITY` (5) 释放完整 LCD 次像素清晰度。
5. **等宽对齐数字（Tabular Numerals）与内联高亮**  
   所有动态指标（`gauge-pct`, `metric-pair .val`, `legend-val`, `spark-hdr strong`, `.num-hl`, `.val-tween`, 表格 `.num` 列）必须声明 `font-variant-numeric: tabular-nums`；叙述句中的动态数值统一由 `.num-hl` 5px 圆角徽章承托。

---

## 4. 空间结构、窗体控制与高清图标规范

- **桌面窗口网格**：`grid-template-columns: 282px 1fr; gap: 13px; padding: 12px 15px;`，一屏锁死无全局滚动条，底部工作区通过内置 Tab 内滚动保持界面规整。
- **圆角阶梯**：
  - 主容器 Bento Card：`--radius: 14px`
  - 内部嵌套子卡片（`metric-pair`, `legend-item`, `spark-box`, `alert-box`, `sub-card`）：`10px`
  - 顶栏与工作区滑块拇指：`7px ~ 8px`
  - 胶囊（Pill）、进度条、开关（Toggle）：`999px`
- **右上角三键风格统一与单层悬浮气泡**：  
  最小化 (`#btn-win-min`)、最大化/还原 (`#btn-win-max`)、轻量模式 (`#btn-win-close`) 全部废弃浏览器原生 `title="..."` 提示与操作系统默认图标，统一使用同源 **14×14 双层自定义线性 SVG**（微沉底层线 + 高亮前景交互）与一致的深色玻璃自绘悬停气泡（`.win-lite-tip`：`dot-sky` / `dot-indigo` / 翠绿呼吸点），彻底消除风格割裂与双层 tooltip 重叠问题。
- **高清系统托盘图标流水线（`tray.go`）**：  
  基于原始 Logo 紧致裁剪 `M` 几何主标，利用 **4×4 超采样栅格化（`renderCrispTrayPNG`）** 映射至贴合瓷白底圆角方徽章（`#FFFFFF` 表面 + `#64748B` 1px 边框 + 加深暗岩板墨迹），分别按系统实时 DPI 的任务栏小图标尺寸（`max(SM_CXSMICON, 16px*scale)`）与托盘面板头图尺寸（`24px*scale`）1:1 精确输出，避免 OS 二次压缩发虚。

---

## 5. 跨端视觉一致性：Win32 原生悬浮组件规范

Windows 原生分层/弹出窗口不得使用割裂审美，必须 **1:1 复刻 Porcelain Daylight Bento**：

### 5.1 贴边真相光尺 · Edge Horizon v2

| 形态 | 尺寸 (96 DPI 基准) | 圆角 | 视觉构成 |
| :--- | :--- | :--- | :--- |
| **态 1：贴边微光尺 (Collapsed)** | `9px × 80px`，距屏幕边缘 `2px` | `4.5px` | 瓷白主体 `#FFFFFF` + 1px 描边 `#CBD5E1`；内部 `4px` 浅灰轨道 `#F1F5F9` 承载自下而上的语义水位柱；顶部点缀 `3px` MEMORA 蓝状态灯 `#2563EB`。 |
| **态 2：展开微缩卡片 (Expanded)** | `214px × 68px`，距屏幕边缘 `6px`（外画布 `230px × 96px`） | `14px`（与主面板 `--radius` 完全一致） | 瓷白卡片 `#FFFFFF` + 1px `#CBD5E1` 描边 + 柔和高阶阴影；贴边侧保留 `3.5px` 水位柱；内含 `40px` 语义圆环（悬停底染 `#EFF6FF`，点击触发深释旋转彗星弧）；右侧三行清晰排版：① `12px Bahnschrift` `MEMORA` 标题 + `42×16` `11px` SDF 语义胶囊；② `11px Segoe UI Variable Text` 硬件温度遥测 `CPU XX°C · GPU XX°C`（按高温阈值切换灰岩/琥珀/告警红）；③ `11px Noto Sans SC` 状态与可用容量反馈。 |

### 5.2 全屏自由拖拽与 120Hz 弹性贴边架构

- **全屏自由拖拽（Full-Screen Free Dragging）**：  
  鼠标按下（`wmLButtonDown`）锚定视觉卡片在屏幕上的绝对像素坐标（左侧锚左沿、右侧锚右沿）；拖拽移动期间通过 `MonitorFromPoint` 跨显示器追踪工作区，窗口可随光标在任意显示器水平、垂直自由滑动（零单边锁定、零瞬移跳变）。
- **松手近边吸附弹簧（Drop Spring Docking）**：  
  松手时（`wmLButtonUp`）依据视觉中心相对于当前显示器中轴 `midX` 自动判定 `left` / `right` 归属，并保持当帧视觉坐标连续无跳变，启动 120Hz 临界阻尼水平弹簧（`omegaX = 22.0`）丝滑滑移回弹贴边，停稳后约 `360ms` 自动收起为 `9px` 极简光尺并持久化保存位置。
- **高刷零分配管线**：Win32 消息线程启动调用 `runtime.LockOSThread()`；按 DPI 持久化缓存单一 `memDC`、32-bit DIB 与 5 枚 `HFONT`；仅在交互动画期申请 `timeBeginPeriod(1)` + `8ms` 计时器，平时后台零唤醒、零开销。

### 5.3 连续交互式托盘轻量面板（`tray_menu.go`）

- **结构与数据**：`344px` 宽（`56px` 顶栏），瓷白表面 + 1px 描边 + 内嵌高清 `24px` 图标，状态栏实时显示 `智能调度中 / 仅观测中 · 已用 XX% · 可用 XX% · 自身占用 XXMB`。
- **零 Emoji Vector Dot**：菜单项前置语义点由 GDI `Ellipse` 直接绘制矢量圆点，替代 Unicode Emoji。
- **连续操作免关闭（`KeepOpen` 交互流）**：  
  点击模式切换（智能调度 / 纯观测）、贴边光尺开关或立即深度释压时，面板保持展开并即时原地刷新状态摘要；点击面板空白处/分割线亦不关闭；仅当**点击面板外部区域**（`WM_ACTIVATE` `WA_INACTIVE` / `WM_KILLFOCUS`）、按下 `ESC`、再次右键托盘、打开主控制中心或退出程序时才关闭浮窗。

---

## 6. 动效与微交互语言（Motion & Micro-Interactions）

MEMORA 的动效遵循「物理阻尼 + 有意义的视觉引导」原则，不做无目的炫技：

1. **弹性滑块开关（Elastic Sliding Pill）**  
   模式切换滑块（`智能调度` / `关闭`）及 Tab 选中底衬滑块采用统一弹簧贝塞尔曲线：`cubic-bezier(0.34, 1.56, 0.64, 1)`，时长 `480ms`（Tab 为 `380ms`），点击时产生真实的滑行与轻微过冲回弹物理感。
2. **丝滑浮动列表光标（Shared Floating List Cursor）**  
   所有列表容器（开关列表、托管进程视图、调度日志、数据表）共用 `.list-hover-cursor` 共享焦点层，利用 `mousemove` 计算行高坐标并以 `translate3d` + `220ms cubic-bezier(0.22, 1, 0.36, 1)` 丝滑滑移，消除传统表格行瞬闪切换的割裂感。
3. **克制卡片聚光灯（Restrained Card Spotlight）**  
   `.spot-surface` 通过 `--mx` / `--my` 监听光标在卡片附近（含 ±80px 预热区）的移动，叠加极低透明度（`0.055`）的径向蓝光釉面，体现精细仪器表面的温润反光感，不遮挡阅读对比度。
4. **同步起止数字呼吸递增/递减引擎（1.1s Synchronized Lifecycle）**  
   每当数字指标发生跳变时触发 `startElementTween`（消除前置等待与尾部停滞，实现缩放与数字滚动的完全同步）：
   - **同步起跑（`0ms`）**：`.is-counting` 缩放放大（`1.22x` / `.val-tween` `1.15x`）与 Cubic-Out 连续数字递增/递减在同一帧同时启动。
   - **平滑插值滚动（`0 ~ 1100ms`）**：维持直观可读的滚动节奏，并在单位跨档（`MB <-> GB`）时自动做量纲对齐避免数字倒跳。
   - **同步落位复位（`840 ~ 1100ms`）**：在滚动结束前 `260ms` 提前触发 `.is-counting` 退出过渡，使字号恢复 `1.0x` 原位与数字到达终值在同一瞬间精准汇合。

---

## 7. 自适应多群体数据表达规范（Adaptive Persona UI）

禁止将只属于特定开发者环境（例如 WSL2 / 特定 OEM 驱动）的指标写死为一成不变的主界面常量：

- **动态五维内存槽位（Dynamic Slot 2 Adaptation）**：
  - **虚拟机/容器宿主机 (`vid_bytes >= 64 MB`)**：第 2 槽显示「`WSL2 / Vid 锁页`」，准确揭露容器占用的不可回收物理锁页。
  - **通用游戏/办公/创意主机 (`vid_bytes < 64 MB`)**：第 1 槽自动收敛为「`前台与活跃独占`」，第 2 槽无缝适配为「`后台静默驻留`」（展示非前沿闲置后台进程锁住的私有工作集，并提示 Top 3 后台驻留大户），让 5 张卡片在任意一台 Windows 电脑上 100% 都有高价值信号。
- **行为驱动归因卡片**：  
  在「真相归因与派生治理」页，依据本机真实遥测条件按需呈现：内核对象/驱动池/高句柄泄漏、虚机锁页、多进程簇（浏览器/Electron/WebView2）后台膨胀、虚拟内存提交水位告警（防止无 Pagefile 余量导致的闪退）、以及多 NUMA 失衡预警。

---

## 8. 禁用模式清单（Anti-Patterns）

- ❌ **禁止静态伪数据与假清理进度条**（不得为了凑释放数字而清理无辜前后台缓存）。
- ❌ **禁止使用 Unicode Emoji** 替代状态徽章、菜单项点或按钮图标。
- ❌ **禁止在浅色 WebView2 使用 `-webkit-font-smoothing: antialiased`** 或 `< 11px` / 小数字号造成字体发虚。
- ❌ **禁止写死特定电脑品牌的进程黑名单作为默认出厂配置**（必须默认留空 + 运行时基于行为自学习）。
- ❌ **禁止在全屏游戏/视频/渲染独占前台时弹出任何浮窗或调整表面**。
