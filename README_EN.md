<p align="right">
  <strong>English</strong> | <a href="README.md">简体中文</a>
</p>

<p align="center">
  <img src="assets/brand/cover.jpg" width="760" alt="MEMORA — Memory, Back to Its Nature.">
</p>

# MEMORA

**Memory, Back to Its Nature.**  
**Always Watching. Never Intruding.**

An open-source intelligent memory guardian engine for Windows.

MEMORA is designed to resolve systemic issues that emerge during extended Windows uptime: abnormal memory footprint inflation, resource retention, driver and kernel object leaks, virtualization locks, and declining memory allocation efficiency.

Unlike conventional "memory cleaners" or "optimizer" utilities, MEMORA never resorts to indiscriminate batch trimming, forced working set emptying, or cosmetic metric suppression that harms multitasking responsiveness. Instead, it operates on first principles and necessity: continuously observing kernel-grade memory metrics, attributing process behaviors, identifying latent leak vectors, and applying adaptive, reversible interventions so that Windows maintains sustained responsiveness and low baseline footprint across weeks of uninterrupted uptime.

---

## Project Origins

This project did not originate from a theoretical premise. It stems from real-world, reproducible phenomena on modern Windows production workstations that existing system utilities struggle to explain:

- Fresh baseline memory footprint rests around 9–10 GB;
- After running diverse workloads throughout the day—even after closing applications from the system tray and terminating processes via Task Manager—footprint never returns to baseline;
- Leaving the system idle for 10 to 24 hours causes memory usage to gradually inflate on its own, with no visible user process exhibiting proportional growth;
- Built-in utilities (Task Manager, Resource Monitor) and third-party "PC managers" show seemingly normal process lists; summing all visible process working sets accounts for only a fraction of the occupied physical memory;
- The only effective remedy has historically been a system reboot.

The core dilemma is not a shortage of physical RAM, but rather: **who holds the allocated memory, why was it never reclaimed, and why can standard tools not account for it?** MEMORA was engineered to answer these questions directly.

---

## Core Philosophy

- **Intelligent Monitoring** — Continually perceive memory state and kernel allocations rather than relying on reactive user intervention.
- **Root Cause Analysis** — Focus on the true origin of memory bloat rather than superficial working set figures.
- **Adaptive Management** — Formulate targeted, reversible management strategies dynamically based on system pressure curves.
- **Zero Interference** — Prioritize uninterrupted foreground responsiveness, active workflows, and seamless multitasking above all else.
- **Minimal Footprint** — Maintain strictly bounded runtime overhead (typically ~40 MB working set and zero negligible CPU footprint).
- **Open & Transparent** — Completely open-source, deterministic, and auditable; every intervention is logged and verifiable.
- **Long-Term Stewardship** — Adhere to long-term stewardship. Prove value through multi-day and multi-week "Quiet Ledgers" demonstrating active restraint, rather than manufacturing illusionary short-term numbers.

---

## What MEMORA Does / Does Not Do

**What MEMORA Does:**

- Resides quietly in the background, putting perception before action and restraint before intervention;
- Pinpoints the root causes of abnormal memory inflation instead of masking symptoms;
- Intervenes only when genuinely warranted, ensuring every action is explainable and reversible;
- Guarantees that upon background process termination, freed memory is properly released to the OS;
- Operates with an ultra-lightweight footprint, capable of running smoothly even on resource-constrained hardware;
- Replaces opaque, disruptive "AI memory cleaners" with a deterministic, rule-governed guardian engine.

**What MEMORA Does Not Do:**

- No "one-click cleaners", no scheduled bulk flushes, and no indiscriminate memory purges;
- Never forces working set truncation merely to generate the illusion of "freed" memory;
- Never compromises the stability, caching, or performance of running background services or foreground tasks;
- Zero intrusive popups, banners, ads, or unsolicited telemetry;
- Never promises to turn 8 GB of physical RAM into 16 GB.

---

## Our Vision

We do not intend for users to keep MEMORA open on screen all day, nor should anyone feel compelled to click memory optimization buttons.

MEMORA is designed to be an unobtrusive guardian in your operating system—staying silent, continuously monitoring, and intervening only when true pressure thresholds are crossed. It embodies **long-term stewardship**: you do not need to check it daily, but when you open its interface after several weeks or months, a calm **Quiet Ledger** will show you exactly how many hours it guarded your system, what percentage of time it exercised active restraint, how many genuine pressure spikes it relieved, how many orphaned child chains it reined in, and which unseen driver or virtualization locks it identified.

It will not give your computer more physical memory than hardware provides.  
It enables your operating system to utilize the memory it already owns with natural efficiency.

**Your Memory. Your System. Naturally Efficient.**

---

## Authentic Telemetry & UI Showcase (160+ Hours Real-World Test)

> [!NOTE]
> The following telemetry data was captured from a production Windows 11 workstation after **162.9 hours (approximately 7 days)** of continuous operation without rebooting.
> MEMORA maintained an ultra-lean footprint of ~41 MB working set and 790 handles throughout the entire test period, adhering strictly to first-principles necessity.

### 1. Smart Sensing & Dynamic Dispatch Center

<p align="center">
  <img src="assets/screenshots/dashboard-smart-dispatch.jpg" width="100%" alt="MEMORA Smart Sensing & Dynamic Dispatch Center">
</p>

- **Porcelain Daylight Bento Architecture**: Engineered with a minimalist porcelain white aesthetic and a tight `Bahnschrift + Segoe UI Variable` typographic hierarchy. Manages 61 idle background processes natively with one-click export for 24-hour auditable diagnostic telemetry.
- **5-Dimensional Physical Memory Decomposition**: Replaces misleading single-metric working set graphs with an exact color-coded breakdown across 39.2 GB of physical RAM:
  - **Foreground & Active Exclusive**: 3.4 GB (8.6%) — Strictly shielded via foreground process family tree inheritance; never touches interactive applications;
  - **Background Idle Retention**: 1.5 GB (3.9%) — Pinpoints suspended or out-of-focus background working sets;
  - **Zombie / Kernel Leaks**: 0 MB (0.0%) — Continuously monitors kernel `EPROCESS` object counts and Pool Tag leaks;
  - **System Kernel, Shared & Mapped Cache**: 5.2 GB (13.3%) — Encompasses file system page mappings and kernel non-paged/paged pools;
  - **Immediately Available Memory (incl. Standby)**: 29.1 GB (74.1%) — Ensures abundant high-speed memory buffers remain instantly ready for heavy workloads.
- **Six Core Adaptive Scheduling Strategy Modules**:
  - **Smart Tiered Memory Dispatch**: Operating in "Tier 1: Light Cruise", dynamically relieving an accumulated **635.34 GB** with an anti-rebound calibration circuit protecting 33 sensitive processes;
  - **Background CPU / EcoQoS Efficiency Management**: Gently assigns EcoQoS and CPU throttling to 61 non-foreground idle processes, reducing background cycle waste and thermal load;
  - **Child Process Fork Storm Suppression**: Monitors high-frequency short-lived process spawn pulses, intercepting **15,297** malicious forks with an automated 7-item suppression blocklist;
  - **Standby & Modified Page Intelligent Flushing**: Manages 28.4 GB of standby cache under high memory pressure, preventing file I/O cache spikes from starving foreground productivity apps;
  - **Fullscreen Gaming / Creative Silent Escort**: Detects true exclusive fullscreen states, yielding completely **20,780 times** with zero drops in frame rate and zero background interruptions during gaming and 3D rendering;
  - **Edge Horizon**: A 4px unobtrusive edge indicator on screen borders that supports free-drag edge docking and displays live CPU/GPU temperatures and memory loops upon mouse hover.

---

### 2. Quiet Ledger & 7-Day Baseline Conservation

<p align="center">
  <img src="assets/screenshots/dashboard-quiet-ledger.jpg" width="100%" alt="MEMORA Quiet Ledger & 7-Day Baseline Conservation">
</p>

- **162.9 Hours Uptime & 94.8% System Restraint Rate**:
  - **Continuous Uptime**: 162.9 hours of rock-solid protection without requiring a system restart;
  - **System Restraint Rate**: Maintained active restraint in **94.8%** of sampling cycles—rejecting the erratic killing behavior of typical PC managers that causes disk I/O thrashing and UI stutter;
  - **Total Dynamic Relief**: 635.34 GB across multiple tiers;
  - **High-Pressure Emergency Interventions**: 3,012 / 3,042 actions, safely defusing memory exhaustion near critical limits;
  - **Fullscreen / Game Silent Yields**: 20,781 true fullscreen yields;
  - **Orphan Chains & Fork Storms Blocked**: Reclaimed 23 orphaned child processes left behind by terminated parent applications and halted 15,297 spawn storms;
  - **Kernel Anomaly Witnessing**: 1,899 precise observations of kernel-level `Process` object surges (with historical peaks reaching 353,346 objects).
- **7-Day Hardware Baseline Conservation**:
  - Validates idle physical memory footprint across 7 continuous days of varied multitasking workloads;
  - Successfully brought long-term cumulative idle bloat down from **22.93 GB to 6.57 GB** (a net reduction of **-16.36 GB**), eliminating the cycle of forced Windows reboots.
- **Top 5 Background Bloat Culprits (Native Icons & Real Telemetry Attribution)**:
  - Tracks background applications that expand silently without user intent, applying "anti-rebound soft suppression" and "tiered tightening" without killing processes or losing state:
    - `WeChatAppEx.exe` (Mini-program / WebView container): Reined in 37 times, peak working set 220 MB, net savings **6.19 GB**;
    - `Nexus.exe`: Reined in 19 times, peak working set 123 MB, net savings **5.72 GB**;
    - `douyin.exe`: Reined in 24 times, peak working set 1,004 MB, net savings **5.50 GB**;
    - `Flydigi Space Station.exe`: Reined in 28 times, peak working set 87 MB, net savings **5.05 GB**;
    - `verge-mihomo.exe`: Reined in 9 times, peak working set 128 MB, net savings **3.89 GB**.

---

## Quick Start

Requires Go 1.21 or newer on Windows 10 or Windows 11.

```powershell
.\build.ps1                              # Builds bin\memora.exe (single executable, zero runtime dependencies)
.\build.ps1 -Installer                   # Runs automated test suites and compiles the official NSIS setup package (*-setup.exe)
.\bin\memora.exe status                  # Inspect current memory anatomy (processes, Vid/NUMA, kernel object summary)
.\bin\memora.exe status --deep           # Deep diagnostics: scan loaded drivers to match abnormal Pool Tags to .sys files
.\bin\memora.exe doctor --minutes 10     # Sample over time, calculate slopes, attribute child leaks and Pool Tag deltas
.\bin\memora.exe watch --observe         # Foreground observation mode: log only, take no interventions
.\bin\memora.exe watch                   # Guardian mode: adaptive tiered dispatch based on pressure curves
.\bin\memora.exe ui                      # Launch/wake tray daemon and open the graphical Dispatch & Truth Center
.\bin\memora.exe tray                    # Lightweight native system tray daemon (double-click opens Dispatch Center)
.\bin\memora.exe ledger                  # View Quiet Ledger (uptime, restraint rate, peak escort, and baselines)
.\bin\memora.exe startup enable --tray   # Configure tray auto-start on Windows login (HKCU Run)
```

`status`, `doctor`, `trace`, and `ledger` never alter system memory; they are strictly read-only diagnostics. On first run, we recommend observing with `watch --observe` or `tray --observe` for 24–48 hours to inspect findings before enabling active guardian mode.

During development and testing, you can restrict interventions to an isolated process via `watch --only-pid <PID>`.

---

## Command Reference

| Command | Description |
|---|---|
| `memora` (no arguments) / `ui` | Launches/wakes the tray daemon and opens the standalone desktop **Performance Dispatch & Truth Center** |
| `status [--json] [--deep]` | Comprehensive inspection of physical memory, commit, kernel pools, page lists, `Vid`/NUMA, fullscreen status, process rankings, and kernel object leaks; `--deep` attributes pool tags to `.sys` drivers |
| `watch [--observe\|--quiet\|--hidden] [--interval D] [--only-pid PID]` | Background guardian daemon; `--observe` monitors without intervening, `--hidden` hides console, `--interval` overrides polling frequency |
| `tray [--observe\|--hidden] [--open-ui]` | Lightweight Win32 native system tray resident (double-click opens dashboard, right-click menu offers one-click relief, mode toggles, and status dialogs) |
| `doctor [--minutes N] [--interval D]` | Multi-minute diagnostic sampling: calculates slopes, identifies orphan child processes after parent exits, and tracks `Proc`/Pool Tag deltas |
| `trace [--minutes N] [--interval D] [--out F]` | Streams per-sample diagnostic frames into JSON Lines format for external analysis |
| `ledger [--json] [--reset]` | Displays the long-term Quiet Ledger: uptime, restraint percentage, game fullscreen yields, soft mitigation counts, and 30-day baseline floor |
| `journal [--lines N] [--path]` | Reads action journal or prints log paths to verify past interventions and reclaimed memory |
| `config [--reset]` | Views or resets engine configuration |
| `startup enable [--tray]\|disable\|status` | Manages Windows login auto-start (supports silent tray start) |
| `version` | Displays version and build metadata |

---

## How MEMORA Diagnoses

MEMORA does not rely on simple working set sums to determine memory usage. Working sets include shared pages (such as mapped system DLLs shared across dozens of processes), causing naive sums to dramatically overestimate process memory while missing kernel allocations.

MEMORA bases its primary accounting on **Commit Limits and Physical/Kernel Locks**:

$$\text{Unattributed Commit} = \text{Total Commit} - \sum \text{Private Commit} - \text{Paged Pool} - \text{Non-Paged Pool}$$

Process enumeration utilizes kernel-level `NtQuerySystemInformation(SystemProcessInformation)`, allowing standard unprivileged users to read private commit and exclusive working sets of protected and cross-session system processes (such as `dwm.exe`, `vmmemWSL`, and security daemons) without permission blind spots that plague standard `OpenProcess` calls.

Beyond commit differentials, `status` and `doctor` natively inspect three major sources of invisible memory inflation hidden from Task Manager:

1. **Virtualization Page Locking & NUMA Imbalance**: Directly samples Hyper-V and WSL2 `Vid` partition locked physical pages and individual NUMA node balances via system performance counters;
2. **Kernel Object Reference Leaks**: Compares kernel `Process`, `Token`, and `Job` object counts against active handle tables and `Proc` pool allocation deltas, pinpointing zombie `EPROCESS` structures caused by third-party drivers omitting `ObReferenceObject` dereferences;
3. **Kernel Pool Tag Driver Attribution**: Reads raw `Pool Tag` allocations (`SystemPoolTagInformation`) and, in `--deep` or `doctor` modes, maps tags against loaded `.sys` kernel driver binaries to attribute non-paged and paged pool leak culprits.

Growth trends are evaluated via ordinary least squares (OLS) linear regression over 30-minute sliding windows (measured in MB/hour). Isolated spikes are discarded; only sustained, statistically significant slopes trigger warnings or mitigations.

For detailed architecture, consult [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md). For safety constraints, see [docs/SAFETY.md](docs/SAFETY.md). For a deep dive into why Windows memory fails to return to baseline, read [docs/DIAGNOSIS.md](docs/DIAGNOSIS.md).

---

## Production Verification

Running `memora status --deep` on a production Windows 11 Pro system (40 GB RAM, 39.2 GB addressable) after multi-day uptime revealed:

- Physical RAM: 39.2 GB total; 26.2 GB in use (66.9%), 13.0 GB available;
- 403 active processes enumerated: combined working sets totaled 7.2 GB (4.6 GB exclusive private), with private commit totaling 24.3 GB;
- Hyper-V / WSL2 `Vid` partition page locks held 6.0 GB (unattributed in any process working set), leaving NUMA Node0 with only 52 MB available out of 15,843 MB;
- Kernel paged pool at 2.1 GB, non-paged pool at 2.8 GB (4.9 GB total pool memory); kernel object inspection discovered two latent leak sources:
  - **Zombie `EPROCESS` Object Accumulation**: 187,040 kernel `Process` objects remained allocated (while active running processes were only 403, and `Proc` pool tags showed 187,042 allocations versus only 2 frees), consuming ~1.4 GB alongside related structures (`MiP2`, `Toke`);
  - **Driver Non-Paged Pool Leak**: An unindexed pool tag `ConT` held 308 MB, matched via deep driver binary scanning to `mtkwl6ex.sys`;
- Standby cache: 11.8 GB; modified cache: 140 MB; free memory: 1.2 GB;
- MEMORA resident overhead: 7 MB working set, 93 open handles.

The structural divergence between process working sets (7.2 GB), private commit (24.3 GB), virtualization page locks (6.0 GB), and kernel pool leaks (4.9 GB) explains why traditional memory cleaners fail to diagnose or resolve system slowdowns.

---

## Project Status

Current release: **v0.3.0** (single binary with zero external Go runtime dependencies `CGO_ENABLED=0`, including official NSIS setup installer `*-setup.exe` and standalone **Dispatch & Truth Center** desktop interface).

Core capabilities and engineering highlights:

- **Dual-Mode Entry & Edge Horizon**: Run `memora.exe` or click the desktop shortcut to launch a borderless standalone desktop window built on WebView2 / Chromium (Porcelain Daylight Bento design with `Bahnschrift + Segoe UI Variable + Noto Sans SC` typographic stack). The interface auto-adapts physical memory breakdown according to host hardware (developer rigs show WSL2 / Vid locks; standard machines show Background Idle Retention). Minimizing to tray auto-reclaims the WebView2 rendering host after 18 seconds. A 4px Edge Horizon indicator sits on any screen edge with 120Hz spring snap physics, revealing live CPU/GPU temperatures and one-click relief upon hover.
- **Three-Tier Adaptive Scheduling & Anti-Rebound Calibration**: Does not wait until available memory drops below 12% to act. Tier 1 gently demotes idle background processes and monitors fork storms; Tier 2 delicately trims long-idle outsized working sets; Tier 3 safely frees low-priority standby cache and modified pages. Powered by an adaptive cooldown mechanism (`adaptive_cooldown`) and 7-day rolling window baseline drift attribution (Top 5 culprits), never forcefully terminating third-party applications.
- **Kernel-Grade Telemetry & Long-Term Reliability**: Eliminates handle leaks across PDH counters and `EnumWindows` callbacks during multi-week runs. Delivers direct kernel object parsing, driver `.sys` matching, and fork storm throttling for driver leaks and `Vid` locks. The system tray menu operates with zero flickering and live resident memory reporting.
- **Bilingual Internationalization (i18n)**: Instant, zero-reload switching between English (`en-US`) and Simplified Chinese (`zh-CN`) directly from the UI header and system tray, with preferences persisted across sessions.
- **Open Source & Authenticode Code Signing**: Released under the [MIT License](LICENSE). Build pipelines integrate formal `RT_VERSION` and `RT_MANIFEST` metadata with SHA-256 Authenticode digital signing (supporting external commercial OV/EV certificates, bundled self-signed certificates, and automatic installer root trust setup), accompanied by `SHA256SUMS` and `install-trust-cert.cmd` for offline verification.

---

## Contributing

If you encounter scenarios on Windows where terminated processes leave behind unreturned memory, where standard diagnostic tools fail to identify the cause, or where system responsiveness degrades over time—we welcome your reproducible observations and test logs. MEMORA's engineering decisions are rooted strictly in reproducible evidence, not in cosmetic optimization myths.

---

## Code Signing Policy

Free code signing provided by [SignPath.io](https://signpath.io/), using a certificate by [SignPath Foundation](https://signpath.org/).
