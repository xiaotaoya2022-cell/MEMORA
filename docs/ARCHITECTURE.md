# 架构

MEMORA 只有一个可执行文件，内部分四层：采集、判断、决策、执行。层与层之间
只通过结构体传递数据，没有全局状态，也没有跨层回调。

```
cmd/memora        命令行入口（status / watch / doctor / trace / journal / config / startup）
internal/winmem   Windows 内存接口与页链读取
internal/proc     进程枚举、工作集、私有提交、CPU 时间
internal/engine   采样循环、趋势判断、策略、动作、日志
internal/config   配置读写
internal/journal  追加式 JSONL 日志
```

## 采集

常规采样循环做四件事：

1. `GlobalMemoryStatusEx` 取物理内存总量与可用量。
2. `GetPerformanceInfo` 取提交量、提交上限、内核分页/非分页池、系统缓存、句柄与线程数。
   这个结构在不同 Windows 版本上长度不同：当前版本以 `cb` 字段开头（104 字节），
   旧版本没有该字段（96 字节）。代码先按新布局调用，失败或页大小不合理时回退旧布局。
3. `NtQuerySystemInformation(SystemMemoryListInformation)` 取各状态页链长度：
   Free / Zeroed / Modified / Standby。
4. 进程侧主路径使用 `NtQuerySystemInformation(SystemProcessInformation = 5)`，一次系统调用
   直接读出全部进程（含跨会话、高权限、WSL/系统进程）的 PID、父进程 PID、Session ID、
   WorkingSet、WorkingSetPrivateSize、PrivatePageCount、句柄数、线程数与累计 CPU 时间，
   无需逐个 `OpenProcess`；仅当该调用不可用时才降级回退到 `EnumProcesses` + `GetProcessMemoryInfo`
   并标出无权限数量。真正执行工作集裁切时仍使用受安全门控保护的 `OpenProcess` + `EmptyWorkingSet`。

按需深层诊断（`status`、`status --deep` 与 `doctor` 观察阶段，不在 `watch` 每 10 秒热循环里扫盘）：

5. `NtQuerySystemInformation(SystemPoolTagInformation = 0x16)` 与 `NtQueryObject(ObjectTypesInformation = 3)`：
   读取全部内核 Pool Tag（分页/非分页占用、分配次数、释放次数）以及 `Process` / `Token` / `Job` 等
   对象总计数与句柄数，用于识别"进程已退出但内核对象未销毁"的僵尸引用泄漏。
6. `NtQuerySystemInformation(SystemModuleInformation = 11)`（Windows 11 标准用户下 `EnumDeviceDrivers`
   会将基址置零，导致路径全部解析失败，因此采用内核模块枚举 11 号信息类）读取已加载驱动路径，
   在 `status --deep` 与 `doctor` 结束归因时按需扫描 `.sys` 二进制特征（正向及小端字节序），
   把未知高占用 Pool Tag 匹配到疑似驱动文件。
7. `pdh.dll` 英文性能计数器查询 Hyper-V / WSL2 `Vid` 分区物理锁页数，以及各 NUMA 节点的可用/总容量。

## 判断

### 为什么不拿"工作集合计"当答案

工作集包含共享页。同一个 DLL 被 30 个进程映射，就会被算 30 次。把工作集相加
得到的数字必然大于真实占用，用它来判断"内存被谁吃了"会把人带到错误的方向。
所以判断的主线是**提交量**，定位具体进程套件时也会并行展示**独占工作集（Private Working Set）**。

### 未归因提交

```
未归因提交 = 提交总量 − Σ进程私有提交 − 分页池 − 非分页池
```

私有提交不会重复计算共享页，池由内核报告，剩下的部分就是"没有单个进程认领"的
提交。主路径已通过 `SystemProcessInformation` 覆盖全部活跃进程的私有提交，因此该差额
不再混入 `OpenProcess` 拒绝访问造成的盲区，而是更真实地反映共享区段（Section）、
会话空间、驱动内部分配以及虚机/系统级提交（仅在回退路径下包含无权限进程）。

### 内核对象泄漏、Vid 锁页与 NUMA 局部挤兑

单靠提交量无法识别所有"物理内存卡死"：

- **僵尸 `EPROCESS` 检测**：当 `Process` 内核对象数远超当前活跃进程数（> 3 倍）且远超打开的进程句柄数，或 `Proc` 池分配次数远大于释放次数且净残骸超过活跃进程数 3 倍时，判定存在内核驱动 `ObReferenceObject` 引用泄漏。
- **虚机锁页**：Hyper-V / WSL2 通过 `Vid.sys` 锁定物理内存，这部分直接从硬件可用页扣除且不计入进程工作集。
- **NUMA 失衡**：在多 NUMA 节点平台上，即使全局可用内存看似充裕，单一节点被锁页或驱动压至接近 0 MB 时仍会触发严重卡顿，需单独预警。

### 趋势而不是瞬时值

内存管理最忌讳看见一个数字就动手。判断使用最近一个窗口（默认 30 分钟）内的
最小二乘斜率，单位是 MB/小时：

- 提交斜率 × 窗口 ≥ 400 MB → 判定为 `commit` 型增长
- 物理占用斜率 × 窗口 ≥ 400 MB → 判定为 `workset` 型增长
- 待机缓存 > 2 GB 且可用内存低于阈值 → 判定为 `standby` 型压力

样本少于 6 个时不做趋势判断，避免开机初期误报。

## 决策

只有当**可用内存低于阈值**（默认 12%）、**冷却期已过**、**本小时动作次数未超限**、
**本小时回收预算未用完**时，才会生成计划。四条同时成立才算"真的需要动手"。
任何一个动作被关闭，就完全不会尝试。

## 执行

动作只有两个，且都不触碰用户进程的存活状态：

- `EmptyWorkingSet`：把某个空闲进程的工作集页移到待机链表。代价是该进程下次
  被唤醒时要有一次缺页；所以只针对连续 20 分钟 CPU 占用低于 1% 的进程。
- `PurgeStandbyList`：清空待机链表里的干净缓存页。代价是文件缓存变冷，后续读盘
  变慢；默认关闭，需要管理员权限，且 30 分钟内不重复。

动作后强制等待 3 秒重新采样，用真实的可用内存差值作为"回收量"写进日志。如果
没有回收，就如实记 0，不修饰。

## 日志

`%LOCALAPPDATA%\MEMORA\journal.jsonl`，一行一条 JSON，8 MB 轮转。动作条目包含
原因、目标进程、动作前后快照、实测回收量和执行结果。这是"可审计"的全部含义：
凡是动过的地方，都能对上账。

## 自身开销

单文件静态链接，无运行时依赖。空载常驻约 7 MB 工作集、90 余个句柄，采样时
CPU 占用低于 1%，采样间隔默认 10 秒。进程枚举是唯一的重活，约 200 个进程在
毫秒级完成。
