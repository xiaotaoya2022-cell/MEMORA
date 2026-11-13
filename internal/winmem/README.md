# MEMORA

MEMORA 的监测层：只负责如实读取 Windows 的内存状态，不做判断、不做动作。

这里使用的接口分四类：

- 文档化的：`GlobalMemoryStatusEx`、`GetPerformanceInfo`、`pdh.dll`（Hyper-V/WSL `Vid` 与 NUMA）
- 半文档化的：`NtQuerySystemInformation`（`SystemMemoryListInformation`、`SystemPoolTagInformation`、`SystemModuleInformation`）与 `NtQueryObject(ObjectTypesInformation)`
- 驱动离线对照：`MatchPoolTagsToDrivers` 在按需深度诊断阶段扫描已加载 `.sys` 匹配未知 Pool Tag
- 需要特权的：`NtSetSystemInformation`（`MemoryPurgeStandbyList` / `MemoryFlushModifiedList` / `MemoryPurgeLowPriorityStandbyList`）

## 为什么需要 page list

`GlobalMemoryStatusEx` 只给出"可用物理内存"，而 Windows 的"可用"包含
Standby 列表里的缓存页。想知道内存到底被什么形态占着，必须拿到各状态
页链的长度：Free / Zeroed / Modified / Standby。

`SystemMemoryListInformation`（0x50）返回的 `MEMORY_LIST_INFORMATION` 在
不同 Windows 版本上尾部长度不同，所以这里只读取前 22 个 `SIZE_T`，并把
Standby 记为 8 个优先级页链之和。读不到就退化为"无 page list"，上层不得
把它当作 0 处理。

## 数值口径

- 所有内部量一律用字节（uint64）
- 只在展示层换算 MB/GB
- `KernelTotal` 由内核报告，分页/非分页池之和可能与它不等，两者都保留
