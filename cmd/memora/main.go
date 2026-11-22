// Command memora is the only entry point: a console tool that can also run
// as a silent background guardian.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"memora/internal/config"
	"memora/internal/engine"
	"memora/internal/journal"
	"memora/internal/ledger"
	"memora/internal/proc"
	"memora/internal/winmem"
)

const version = "0.3.0"

const runKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`

func main() {
	if len(os.Args) < 2 {
		cmdTray([]string{"--hidden", "--open-ui"})
		return
	}
	switch os.Args[1] {
	case "ui", "dashboard":
		cmdTray(append([]string{"--hidden", "--open-ui"}, os.Args[2:]...))
	case "status":
		cmdStatus(os.Args[2:])
	case "watch":
		cmdWatch(os.Args[2:])
	case "tray":
		cmdTray(os.Args[2:])
	case "doctor":
		cmdDoctor(os.Args[2:])
	case "trace":
		cmdTrace(os.Args[2:])
	case "journal":
		cmdJournal(os.Args[2:])
	case "ledger":
		cmdLedger(os.Args[2:])
	case "config":
		cmdConfig(os.Args[2:])
	case "startup":
		cmdStartup(os.Args[2:])
	case "path":
		cmdPath(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Print("memora " + version + "\n")
	default:
		usage()
	}
}

func usage() {
	fmt.Print(`MEMORA — Memory, Back to Its Nature.

用法:
  memora                             启动或唤醒系统托盘并打开控制与真相中心界面
  memora ui                          启动或唤醒控制与真相中心界面
  memora status [--json] [--deep]    看一次当前状态（--deep 匹配驱动内核池标签）
  memora watch  [--observe] [--quiet] [--hidden] [--interval D] [--only-pid PID]
                                     后台静默守护；--observe 只记录不动手
  memora tray   [--observe] [--hidden] [--open-ui]
                                     以轻量原生系统托盘常驻（双击打开界面，右键看菜单）
  memora doctor [--minutes N] [--interval D]
                                     观察一段时间并给出归因结论
  memora trace  [--minutes N] [--interval D] [--out FILE]
                                     把采样写成 JSONL
  memora ledger [--json] [--reset]   查看长期「静默账本」（守护时长/克制率/空载基线）
  memora journal [--lines N] [--path]
                                     查看动作审计日志
  memora config [--reset]            查看/重置配置
  memora startup enable [--tray] | disable | status
  memora version

说明:
  watch 与 tray 默认按配置运行；config.json 里的 mode 为 observe 时只记录不动作。
  所有动作都会写进 journal，包含原因、前后数值与实测回收量；全屏游戏自动静音护航。
`)
}

// ---------- status ----------

func cmdStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "输出 JSON")
	deep := fs.Bool("deep", false, "扫描已加载驱动以匹配异常内核池标签")
	_ = fs.Parse(args)

	snap, err := winmem.Collect()
	if err != nil {
		fail("读取内存状态失败: %v", err)
	}
	procs, complete, _ := proc.ListAll()

	var ws, privWS, private uint64
	for _, p := range procs {
		ws += p.WorkingSet
		privWS += p.PrivateWS
		private += p.Private
	}
	residual := int64(snap.Commit) - int64(private) - int64(snap.KernelPaged) - int64(snap.KernelNonpaged)
	selfWS, selfHandles := selfStats(procs)
	diag := engine.InspectDeep(snap, procs, *deep)
	fgPID := proc.ForegroundPID()
	family := proc.FamilyPIDs(procs, fgPID)
	qState, qBusy := proc.IsFullscreenOrBusy()
	qLabel := proc.NotificationStateLabel(qState)

	if *asJSON {
		groupLimit := 15
		if len(diag.ProcGroups) < groupLimit {
			groupLimit = len(diag.ProcGroups)
		}
		out := map[string]any{
			"time":                      time.Now().Format(time.RFC3339),
			"total_bytes":               snap.TotalPhys,
			"available_bytes":           snap.AvailPhys,
			"in_use_bytes":              snap.InUse(),
			"available_percent":         snap.AvailablePercent(),
			"commit_bytes":              snap.Commit,
			"commit_limit":              snap.TotalCommit,
			"kernel_paged":              snap.KernelPaged,
			"kernel_nonpaged":           snap.KernelNonpaged,
			"standby_bytes":             snap.Pages.Standby,
			"modified_bytes":            snap.Pages.Modified,
			"free_bytes":                snap.Pages.Free,
			"processes":                 len(procs),
			"process_complete":          complete,
			"working_set_bytes":         ws,
			"private_working_set_bytes": privWS,
			"private_bytes":             private,
			"unattributed_commit_bytes": residual,
			"vid_bytes":                 diag.Extras.VidBytes,
			"numa_nodes":                diag.Extras.NUMANodes,
			"kernel_leak":               diag.Leak,
			"fullscreen_state":          qState,
			"fullscreen_label":          qLabel,
			"fullscreen_quiet":          qBusy,
			"family_protected":          len(family),
			"top_nonpaged_tags":         diag.TopNonPagedTags,
			"top_paged_tags":            diag.TopPagedTags,
			"hook_drivers":              diag.HookDrivers,
			"leak_forecast":             engine.ComputeLeakForecast(snap, diag.Leak),
			"top_process_groups":        diag.ProcGroups[:groupLimit],
			"memora_pid":                os.Getpid(),
			"memora_working_set":        selfWS,
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Printf("MEMORA %s   %s\n\n", version, time.Now().Format("2006-01-02 15:04:05"))
	usedPct := 100 - snap.AvailablePercent()
	fmt.Printf("物理内存   %8s 总量   %8s 使用 (%.1f%%)   %8s 可用\n",
		human(snap.TotalPhys), human(snap.InUse()), usedPct, human(snap.AvailPhys))
	fmt.Printf("提交       %8s / %-8s\n", human(snap.Commit), human(snap.TotalCommit))
	fmt.Printf("内核池     分页 %s   非分页 %s   合计 %s\n",
		human(snap.KernelPaged), human(snap.KernelNonpaged), human(snap.KernelTotal))
	if snap.HasPages {
		fmt.Printf("页状态     待机 %s   已修改 %s   空闲 %s   零页 %s\n",
			human(snap.Pages.Standby), human(snap.Pages.Modified), human(snap.Pages.Free), human(snap.Pages.Zeroed))
	} else {
		fmt.Printf("页状态     当前系统不支持读取页链，待机缓存不可见\n")
	}
	if diag.Extras.HasVid && diag.Extras.VidBytes > 0 {
		fmt.Printf("虚机锁页   %s（Hyper-V / WSL2 Vid 分区物理占用，不计入进程工作集）\n",
			human(diag.Extras.VidBytes))
	}
	if diag.Extras.HasNUMA && len(diag.Extras.NUMANodes) > 1 {
		parts := make([]string, 0, len(diag.Extras.NUMANodes))
		for _, n := range diag.Extras.NUMANodes {
			parts = append(parts, fmt.Sprintf("Node%s 可用 %d/%d MB", n.ID, n.AvailableMB, n.TotalMB))
		}
		fmt.Printf("NUMA 节点  %s\n", strings.Join(parts, "   "))
		if tight, ok := engine.NUMADisparity(diag.Extras.NUMANodes); ok {
			fmt.Printf("           注意: Node%s 可用仅剩 %d MB（%d MB 总量），存在局部内存挤兑\n",
				tight.ID, tight.AvailableMB, tight.TotalMB)
		}
	}

	inaccessible := int(snap.Processes) - len(procs)
	if inaccessible < 0 {
		inaccessible = 0
	}
	if complete {
		fmt.Printf("进程       %d 个（完整读取）   工作集合计 %s（独占 %s）   私有提交合计 %s\n",
			len(procs), human(ws), human(privWS), human(private))
	} else {
		fmt.Printf("进程       %d 个   工作集合计 %s   私有提交合计 %s\n",
			len(procs), human(ws), human(private))
		if inaccessible > 0 {
			fmt.Printf("           系统报告 %d 个进程，其中 %d 个无权限读取，未计入上面的合计\n",
				snap.Processes, inaccessible)
		}
	}
	if qBusy {
		fmt.Printf("场景护航   %s（触发全屏静音免打扰，已锁定保护前景家族 %d 个进程）\n", qLabel, len(family))
	} else {
		fmt.Printf("场景护航   %s（前景家族 %d 个进程受免干涉保护）\n", qLabel, len(family))
	}

	label := "正常"
	if residual > 0 {
		label = "上限估计"
	}
	fmt.Printf("\n未归因提交 %s（%s）\n", humanU(residual), label)
	if complete {
		fmt.Printf("  已覆盖全部活跃进程私有提交与内核池，剩余部分主要为共享区段(Section)、会话空间或系统/虚机提交。\n")
	} else {
		fmt.Printf("  这部分提交不属于已读取进程的私有内存，也不属于内核池（含无权限进程与跨会话分配）。\n")
	}

	printKernelStatus(diag, *deep)

	fmt.Printf("\n进程套件排行（按私有提交聚合）\n")
	fmt.Printf("%-26s %4s %10s %10s %10s %7s\n", "应用", "进程数", "工作集", "独占工作集", "私有提交", "句柄")
	for i, g := range diag.ProcGroups {
		if i >= 10 {
			break
		}
		fmt.Printf("%-26s %4d %10s %10s %10s %7d\n",
			truncateName(g.Name, 26), g.Count, human(g.WorkingSet), human(g.PrivateWS), human(g.Private), g.Handles)
	}

	fmt.Printf("\n单进程工作集 Top 8\n")
	fmt.Printf("%-6s %10s %10s %10s  %s\n", "PID", "工作集", "独占工作集", "私有提交", "进程")
	sort.Slice(procs, func(i, j int) bool { return procs[i].WorkingSet > procs[j].WorkingSet })
	for i, p := range procs {
		if i >= 8 {
			break
		}
		fmt.Printf("%-6d %10s %10s %10s  %s\n", p.PID, human(p.WorkingSet), human(p.PrivateWS), human(p.Private), p.Name)
	}
	fmt.Printf("\nMEMORA 自身 %s / %d 个句柄\n", human(selfWS), selfHandles)
}

func printKernelStatus(diag engine.DeepDiagnosis, deep bool) {
	if !diag.HasObjectTypes && !diag.HasPoolTags {
		return
	}
	fmt.Println()
	if diag.Leak.Suspected {
		fmt.Printf("内核异常   检测到疑似僵尸进程对象泄漏（内核驱动未释放对象引用）\n")
		fmt.Printf("           Process 对象 %d 个（当前运行 %d 个，打开句柄仅 %d 个），Token %d 个",
			diag.Leak.ProcessObjects, diag.Leak.ActiveProcesses, diag.Leak.ProcessHandles, diag.Leak.TokenObjects)
		if diag.Leak.JobObjects > 0 {
			fmt.Printf("，Job %d 个（句柄 %d）", diag.Leak.JobObjects, diag.Leak.JobHandles)
		}
		fmt.Println()
		if diag.Leak.ProcTagAllocs > 0 {
			fmt.Printf("           Proc 池分配 %d 次仅释放 %d 次，相关内核残骸合计 %s\n",
				diag.Leak.ProcTagAllocs, diag.Leak.ProcTagFrees, human(diag.Leak.RelatedPoolByte))
		}
	} else if diag.HasObjectTypes {
		fmt.Printf("内核对象   Process %d（句柄 %d）  Token %d（句柄 %d）\n",
			diag.Leak.ProcessObjects, diag.Leak.ProcessHandles, diag.Leak.TokenObjects, diag.Leak.TokenHandles)
	}

	if len(diag.TopNonPagedTags) > 0 {
		fmt.Printf("非分页池大户\n")
		needDeepHint := false
		for i, t := range diag.TopNonPagedTags {
			if i >= 6 {
				break
			}
			note := formatTagNote(t)
			if !deep && t.Desc == "" && (t.Bytes >= 64<<20 || (t.Allocs > 100 && t.Frees == 0 && t.Bytes >= 16<<20)) {
				needDeepHint = true
			}
			fmt.Printf("  %-4s  %9s  活跃 %-8d  (分配 %d / 释放 %d)%s\n",
				t.Tag, human(t.Bytes), t.Active, t.Allocs, t.Frees, note)
		}
		if needDeepHint {
			fmt.Printf("  提示: 加上 --deep 可扫描已加载驱动，匹配未知 Pool Tag 对应的 .sys 文件。\n")
		}
	}
	if deep && len(diag.TopPagedTags) > 0 {
		fmt.Printf("分页池大户\n")
		for i, t := range diag.TopPagedTags {
			if i >= 5 {
				break
			}
			fmt.Printf("  %-4s  %9s  活跃 %-8d  (分配 %d / 释放 %d)%s\n",
				t.Tag, human(t.Bytes), t.Active, t.Allocs, t.Frees, formatTagNote(t))
		}
	}
	if len(diag.HookDrivers) > 0 && (deep || diag.Leak.Suspected) {
		fmt.Printf("第三方进程钩子驱动审计 (Ring 0 PE 导入表)\n")
		for _, hd := range diag.HookDrivers {
			if hd.RiskLevel == "low" && !deep {
				continue
			}
			extra := ""
			if hd.PoolTag != "" {
				extra = fmt.Sprintf("  [关联池 %s: %s, %d 对象]", hd.PoolTag, human(hd.PoolBytes), hd.PoolActive)
			}
			comp := hd.Company
			if comp == "" {
				comp = "未知厂商"
			}
			ver := ""
			if hd.FileVer != "" {
				ver = " v" + hd.FileVer
			}
			fmt.Printf("  [%-6s] %-22s  %s%s  (%s)%s\n",
				strings.ToUpper(hd.RiskLevel), hd.Name, comp, ver, strings.Join(hd.Hooks, ", "), extra)
		}
	}
}

func formatTagNote(t engine.PoolFinding) string {
	var parts []string
	if t.Desc != "" {
		parts = append(parts, t.Desc)
	}
	if len(t.Drivers) > 0 {
		parts = append(parts, "疑似驱动: "+strings.Join(t.Drivers, ", "))
	}
	if t.Allocs >= 8 && t.Frees == 0 {
		parts = append(parts, "0释放")
	}
	if len(parts) == 0 {
		return ""
	}
	return "  [" + strings.Join(parts, "; ") + "]"
}

func truncateName(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

// ---------- watch ----------

func cmdWatch(args []string) {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	quiet := fs.Bool("quiet", false, "不输出每次采样")
	hidden := fs.Bool("hidden", false, "脱离控制台窗口")
	observe := fs.Bool("observe", false, "强制只观察，不执行动作")
	interval := fs.Duration("interval", 0, "采样间隔，例如 5s")
	onlyPID := fs.Uint("only-pid", 0, "调试：只对指定 PID 生效")
	_ = fs.Parse(args)

	if *hidden {
		winmem.HideConsole()
	}
	release, ok := winmem.AcquireSingleInstance(`Local\MEMORA.Watch`)
	if !ok {
		fmt.Println("已有一个 MEMORA watch 在运行。")
		return
	}
	defer release()

	cfg, err := config.Load()
	if err != nil {
		fail("读取配置失败: %v", err)
	}
	jr, err := journal.Open(config.Dir())
	if err != nil {
		fail("打开日志失败: %v", err)
	}
	defer jr.Close()
	lg, _ := ledger.Open(config.Dir())

	eng := engine.New(cfg, jr)
	if lg != nil {
		eng.SetLedger(lg)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	opts := engine.Options{
		Verbose:   !*quiet,
		Act:       !*observe && cfg.Mode == "guard",
		Interval:  *interval,
		TargetPID: uint32(*onlyPID),
	}
	effective := cfg.Mode
	if *observe {
		effective = "observe"
	}
	shown := *interval
	if shown == 0 {
		shown = time.Duration(cfg.IntervalSeconds) * time.Second
	}
	if opts.Verbose {
		ledgerPath := ""
		if lg != nil {
			ledgerPath = "  账本=" + lg.Path()
		}
		fmt.Printf("MEMORA %s 守护中  模式=%s  间隔=%s  日志=%s%s\n", version, effective, shown, jr.Path(), ledgerPath)
	}
	if err := eng.Run(ctx, opts); err != nil {
		fail("%v", err)
	}
}

// ---------- doctor ----------

func cmdDoctor(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	minutes := fs.Float64("minutes", 5, "观察时长（分钟）")
	interval := fs.Duration("interval", 5*time.Second, "采样间隔")
	_ = fs.Parse(args)

	cfg, err := config.Load()
	if err != nil {
		fail("读取配置失败: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*minutes*float64(time.Minute)))
	defer cancel()

	eng := engine.New(cfg, nil)
	var first, last engine.Sample
	var firstDeep engine.DeepDiagnosis
	firstPriv := map[uint32]uint64{}
	lastPriv := map[uint32]uint64{}
	names := map[uint32]string{}
	seenPIDs := map[uint32]struct{}{}
	spawnByParent := map[string]int{}
	orphansSeen := map[uint32]engine.OrphanProcess{}
	selfPID := uint32(os.Getpid())
	count := 0

	opts := engine.Options{
		Interval: *interval,
		Act:      false,
		OnSample: func(a engine.Assessment) {
			for _, o := range a.Orphans {
				orphansSeen[o.PID] = o
			}
		},
		OnReading: func(s engine.Sample) {
			if count == 0 {
				first = s
				firstDeep = engine.InspectDeep(s.Snap, s.Procs, false)
				for _, p := range s.Procs {
					firstPriv[p.PID] = p.Private
					names[p.PID] = p.Name
					seenPIDs[p.PID] = struct{}{}
				}
			} else {
				for _, p := range s.Procs {
					names[p.PID] = p.Name
					if _, ok := seenPIDs[p.PID]; !ok {
						seenPIDs[p.PID] = struct{}{}
						if p.PID != selfPID && p.ParentPID != selfPID {
							parent := names[p.ParentPID]
							if parent == "" {
								parent = fmt.Sprintf("PID:%d", p.ParentPID)
							}
							spawnByParent[parent]++
						}
					}
				}
			}
			last = s
			lastPriv = map[uint32]uint64{}
			for _, p := range s.Procs {
				lastPriv[p.PID] = p.Private
			}
			count++
			fmt.Printf("\r观察中 %d 个样本  可用 %.1f%%", count, s.Snap.AvailablePercent())
		},
	}

	fmt.Printf("MEMORA doctor  观察 %.1f 分钟，间隔 %s\n\n", *minutes, *interval)
	_ = eng.Run(ctx, opts)
	fmt.Println()

	if count < 2 {
		fail("样本不足，观察时间太短")
	}

	lastDeep := engine.InspectDeep(last.Snap, last.Procs, true)

	dInUse := float64(last.Snap.InUse())/(1<<20) - float64(first.Snap.InUse())/(1<<20)
	dCommit := float64(last.Snap.Commit)/(1<<20) - float64(first.Snap.Commit)/(1<<20)
	dPaged := float64(last.Snap.KernelPaged)/(1<<20) - float64(first.Snap.KernelPaged)/(1<<20)
	dNon := float64(last.Snap.KernelNonpaged)/(1<<20) - float64(first.Snap.KernelNonpaged)/(1<<20)
	dStandby := float64(last.Snap.Pages.Standby)/(1<<20) - float64(first.Snap.Pages.Standby)/(1<<20)
	dPrivate := float64(last.Private)/(1<<20) - float64(first.Private)/(1<<20)
	dWS := float64(last.WS)/(1<<20) - float64(first.WS)/(1<<20)
	residFirst := int64(first.Snap.Commit) - int64(first.Private) - int64(first.Snap.KernelPaged) - int64(first.Snap.KernelNonpaged)
	residLast := int64(last.Snap.Commit) - int64(last.Private) - int64(last.Snap.KernelPaged) - int64(last.Snap.KernelNonpaged)
	dResid := float64(residLast-residFirst) / (1 << 20)

	elapsedMin := last.At.Sub(first.At).Minutes()
	perHour := func(delta float64) float64 {
		if elapsedMin <= 0 {
			return 0
		}
		return delta / elapsedMin * 60
	}

	fmt.Printf("%-14s %12s %12s %10s %12s\n", "项目", "起点", "终点", "变化", "折算/小时")
	row := func(name string, a, b float64) {
		fmt.Printf("%-14s %10.0f MB %10.0f MB %+9.0f MB %+10.0f MB\n", name, a, b, b-a, perHour(b-a))
	}
	row("物理占用", float64(first.Snap.InUse())/(1<<20), float64(last.Snap.InUse())/(1<<20))
	row("提交总量", float64(first.Snap.Commit)/(1<<20), float64(last.Snap.Commit)/(1<<20))
	row("内核分页池", float64(first.Snap.KernelPaged)/(1<<20), float64(last.Snap.KernelPaged)/(1<<20))
	row("内核非分页", float64(first.Snap.KernelNonpaged)/(1<<20), float64(last.Snap.KernelNonpaged)/(1<<20))
	row("待机缓存", float64(first.Snap.Pages.Standby)/(1<<20), float64(last.Snap.Pages.Standby)/(1<<20))
	row("进程私有提交", float64(first.Private)/(1<<20), float64(last.Private)/(1<<20))
	row("未归因提交", float64(residFirst)/(1<<20), float64(residLast)/(1<<20))
	if firstDeep.Extras.HasVid || lastDeep.Extras.HasVid {
		row("虚机锁页(Vid)", float64(firstDeep.Extras.VidBytes)/(1<<20), float64(lastDeep.Extras.VidBytes)/(1<<20))
	}

	printDoctorSpawns(firstDeep, lastDeep, spawnByParent, elapsedMin)
	if len(orphansSeen) > 0 {
		fmt.Printf("\n退出后仍滞留的无窗孤儿子进程\n")
		for _, o := range orphansSeen {
			fmt.Printf("  %-24s (PID %d, 原父进程 %s[%d])  私有提交 %s  工作集 %s\n",
				truncateName(o.Name, 24), o.PID, o.ParentName, o.ParentPID, human(o.Private), human(o.WorkingSet))
		}
	}
	printDoctorGrowers(firstPriv, lastPriv, names, firstDeep.ProcGroups, lastDeep.ProcGroups)
	printDoctorPools(firstDeep, lastDeep)

	fmt.Printf("\n结论\n")
	fmt.Print(conclusion(dInUse, dCommit, dResid, dPrivate, dWS, dStandby, dPaged, dNon, lastDeep))
	if extra := notes(last, lastDeep); extra != "" {
		fmt.Printf("\n值得单独看一眼\n")
		fmt.Print(extra)
	}
	fmt.Printf("\n观察 %d 个样本，%.1f 分钟。以上为真实读数，不是估算。\n", count, elapsedMin)
}

func printDoctorSpawns(first, last engine.DeepDiagnosis, spawnByParent map[string]int, elapsedMin float64) {
	if !first.HasPoolTags && !last.HasObjectTypes {
		return
	}
	dAllocs := int64(last.Leak.ProcTagAllocs) - int64(first.Leak.ProcTagAllocs)
	dFrees := int64(last.Leak.ProcTagFrees) - int64(first.Leak.ProcTagFrees)
	dObjs := int64(last.Leak.ProcessObjects) - int64(first.Leak.ProcessObjects)
	if !last.Leak.Suspected && dAllocs <= 5 && dObjs <= 5 {
		return
	}
	fmt.Printf("\n进程创建与内核对象回收\n")
	rate := 0.0
	if last.HasPoolTags {
		if elapsedMin > 0 {
			rate = float64(dAllocs) / elapsedMin
		}
		fmt.Printf("  观察期内创建进程 %d 次（约 %.1f 个/分），Proc 释放 %d 次，Process 对象净增 %+d 个。\n",
			dAllocs, rate, dFrees, dObjs)
	}
	if len(spawnByParent) > 0 {
		type pSpawn struct {
			name  string
			count int
		}
		var list []pSpawn
		for k, v := range spawnByParent {
			list = append(list, pSpawn{name: k, count: v})
		}
		sort.Slice(list, func(i, j int) bool { return list[i].count > list[j].count })
		var top []string
		for i, item := range list {
			if i >= 5 {
				break
			}
			top = append(top, fmt.Sprintf("%s (+%d)", item.name, item.count))
		}
		fmt.Printf("  采样捕获频繁拉起子进程的宿主: %s\n", strings.Join(top, "、"))
	}
	if rate >= 30 || dObjs >= 20 {
		leakKB := 0.0
		if dFrees == 0 && dAllocs >= 10 {
			leakKB = 150.0
		}
		burst := proc.CaptureBurstSpawners(context.Background(), 2500*time.Millisecond, 30*time.Millisecond, leakKB)
		if len(burst) > 0 {
			fmt.Printf("  短命子进程脉冲抓捕 (2.5s @ 30ms):\n")
			for i, b := range burst {
				if i >= 5 {
					break
				}
				leakNote := ""
				if b.EstLeakMBPerHour >= 10 {
					leakNote = fmt.Sprintf("  [按当前泄漏率折算 ≈ +%.0f MB/时]", b.EstLeakMBPerHour)
				}
				fmt.Printf("    %s(%d) -> %s  捕获 %d 次 (折算 %.0f 次/时)%s\n",
					b.ParentName, b.ParentPID, b.ChildName, b.CapturedCount, b.EstRatePerHour, leakNote)
			}
		}
	}
}

func printDoctorGrowers(firstPriv, lastPriv map[uint32]uint64, names map[uint32]string, firstGroups, lastGroups []engine.ProcGroup) {
	fmt.Printf("\n私有提交增长最快的进程\n")
	type grow struct {
		pid   uint32
		name  string
		delta int64
	}
	var growers []grow
	for pid, priv := range lastPriv {
		base, ok := firstPriv[pid]
		if !ok {
			continue
		}
		if d := int64(priv) - int64(base); d > 16<<20 {
			growers = append(growers, grow{pid: pid, name: names[pid], delta: d})
		}
	}
	sort.Slice(growers, func(i, j int) bool { return growers[i].delta > growers[j].delta })
	if len(growers) == 0 {
		fmt.Printf("  没有单进程私有提交在观察期内增长超过 16 MB。\n")
	} else {
		for i, g := range growers {
			if i >= 5 {
				break
			}
			fmt.Printf("  %-28s %+8s  (PID %d)\n", g.name, humanU(g.delta), g.pid)
		}
	}

	firstMap := make(map[string]uint64, len(firstGroups))
	for _, g := range firstGroups {
		firstMap[strings.ToLower(g.Name)] = g.Private
	}
	var groupGrowers []grow
	for _, g := range lastGroups {
		base, ok := firstMap[strings.ToLower(g.Name)]
		if !ok {
			continue
		}
		if d := int64(g.Private) - int64(base); d > 16<<20 {
			groupGrowers = append(groupGrowers, grow{name: g.Name, delta: d})
		}
	}
	sort.Slice(groupGrowers, func(i, j int) bool { return groupGrowers[i].delta > groupGrowers[j].delta })
	if len(groupGrowers) > 0 {
		fmt.Printf("按套件聚合增长（含新建子进程）\n")
		for i, g := range groupGrowers {
			if i >= 5 {
				break
			}
			fmt.Printf("  %-28s %+8s\n", g.name, humanU(g.delta))
		}
	}
}

func printDoctorPools(first, last engine.DeepDiagnosis) {
	if !last.HasPoolTags {
		return
	}
	firstMap := make(map[string]winmem.PoolTag, len(first.PoolTags))
	for _, pt := range first.PoolTags {
		firstMap[pt.Tag] = pt
	}
	type poolDelta struct {
		tag     string
		delta   int64
		total   uint64
		drivers []string
		desc    string
	}
	driverMap := map[string][]string{}
	for _, f := range last.TopNonPagedTags {
		if len(f.Drivers) > 0 {
			driverMap[f.Tag] = f.Drivers
		}
	}
	for _, f := range last.TopPagedTags {
		if len(f.Drivers) > 0 {
			driverMap[f.Tag] = f.Drivers
		}
	}

	var grown []poolDelta
	for _, pt := range last.PoolTags {
		prev := firstMap[pt.Tag]
		d := int64(pt.TotalUsed()) - int64(prev.TotalUsed())
		if d >= 4<<20 {
			grown = append(grown, poolDelta{
				tag:     pt.Tag,
				delta:   d,
				total:   pt.TotalUsed(),
				drivers: driverMap[pt.Tag],
				desc:    winmem.DescribePoolTag(pt.Tag),
			})
		}
	}
	sort.Slice(grown, func(i, j int) bool { return grown[i].delta > grown[j].delta })

	if len(grown) > 0 || len(last.TopNonPagedTags) > 0 {
		fmt.Printf("\n内核池与驱动归因\n")
	}
	for i, g := range grown {
		if i >= 5 {
			break
		}
		note := g.desc
		if len(g.drivers) > 0 {
			if note != "" {
				note += "; "
			}
			note += "疑似驱动: " + strings.Join(g.drivers, ", ")
		}
		if note != "" {
			note = "  [" + note + "]"
		}
		fmt.Printf("  增长 %-4s  %+8s  (当前 %s)%s\n", g.tag, humanU(g.delta), human(g.total), note)
	}
	for i, t := range last.TopNonPagedTags {
		if i >= 5 || t.Bytes < 32<<20 {
			break
		}
		fmt.Printf("  存量 %-4s  %9s  (分配 %d / 释放 %d)%s\n",
			t.Tag, human(t.Bytes), t.Allocs, t.Frees, formatTagNote(t))
	}
}

func conclusion(dInUse, dCommit, dResid, dPrivate, dWS, dStandby, dPaged, dNon float64, deep engine.DeepDiagnosis) string {
	switch {
	case deep.Leak.Suspected && dPaged+dNon > 8:
		return fmt.Sprintf("  内核池仍在增长（%+.0f MB），同时内核已累积 %d 个未释放的 Process 对象。\n"+
			"  这是内核驱动对象引用泄漏（进程退出后 EPROCESS 仍被持住），用户态无法安全强清，需排查安全/管家/虚拟化驱动。\n",
			dPaged+dNon, deep.Leak.ProcessObjects)
	case dCommit > 64 && dResid > 0.6*dCommit:
		return fmt.Sprintf("  提交增长主要落在未归因部分（%+.0f MB / 总 %+.0f MB）。\n"+
			"  当前已读取全部进程私有提交，剩余增量来自共享区段(Section)、会话或驱动/虚机分配。\n", dResid, dCommit)
	case dCommit > 64 && dPrivate > 0.6*dCommit:
		return fmt.Sprintf("  提交增长主要来自进程私有内存（%+.0f MB / 总 %+.0f MB）。\n"+
			"  可从上面的进程/套件列表定位；MEMORA 只会回收其中长期空闲的工作集。\n", dPrivate, dCommit)
	case dWS > 128:
		return fmt.Sprintf("  进程工作集合计增长 %+.0f MB，属于活跃使用而非泄漏。\n"+
			"  工作集包含共享页，合计会重复计算，不能直接等同于物理占用。\n", dWS)
	case dStandby > 128:
		return fmt.Sprintf("  待机缓存增长 %+.0f MB。这是文件缓存，Windows 会按需回收，不是泄漏。\n", dStandby)
	case dPaged+dNon > 64:
		return fmt.Sprintf("  内核池增长 %+.0f MB（分页 %+.0f / 非分页 %+.0f）。\n"+
			"  持续单调增长通常指向驱动；可结合上面的 Pool Tag 与驱动匹配继续排查。\n", dPaged+dNon, dPaged, dNon)
	case dInUse > 64:
		return fmt.Sprintf("  物理占用增长 %+.0f MB，但提交和进程都平稳。\n"+
			"  优先怀疑共享 section、Hyper-V/WSL 分区或驱动锁定页。\n", dInUse)
	default:
		return "  观察期内系统内存整体平稳。\n"
	}
}

func notes(s engine.Sample, deep engine.DeepDiagnosis) string {
	var b strings.Builder
	commit := float64(s.Snap.Commit)
	nonpaged := float64(s.Snap.KernelNonpaged) / (1 << 30)
	residual := int64(s.Snap.Commit) - int64(s.Private) - int64(s.Snap.KernelPaged) - int64(s.Snap.KernelNonpaged)
	inaccessible := int(s.Snap.Processes) - len(s.Procs)

	if deep.Leak.Suspected {
		fmt.Fprintf(&b, "  存量存在内核 Process 对象泄漏（%d 个对象 vs %d 个运行进程 / %d 个句柄），相关池占用 %s；这类内核泄漏无法在用户态直接释放，必须靠更新/停用问题驱动并重启才能归零。\n",
			deep.Leak.ProcessObjects, deep.Leak.ActiveProcesses, deep.Leak.ProcessHandles, human(deep.Leak.RelatedPoolByte))
	}
	if nonpaged >= 1.0 {
		fmt.Fprintf(&b, "  非分页池 %.1f GB。桌面系统常见 0.2–0.8 GB，长期高于 1 GB 时重点看上面标注的 Pool Tag 与对应驱动。\n", nonpaged)
	}
	if deep.Extras.HasVid && deep.Extras.VidBytes >= 1<<30 {
		fmt.Fprintf(&b, "  Hyper-V / WSL2 Vid 分区锁住 %s 物理内存；如长期不用容器/WSL，可在 .wslconfig 检查 memory 与 autoMemoryReclaim/vmIdleTimeout 设置。\n",
			human(deep.Extras.VidBytes))
	}
	if tight, ok := engine.NUMADisparity(deep.Extras.NUMANodes); ok {
		fmt.Fprintf(&b, "  NUMA Node%s 可用仅剩 %d MB（%d MB 总量），局部节点已接近满载。\n",
			tight.ID, tight.AvailableMB, tight.TotalMB)
	}
	if commit > 0 && residual > 0 {
		share := float64(residual) / commit * 100
		if share >= 50 {
			fmt.Fprintf(&b, "  未归因提交 %.1f GB，占提交的 %.0f%%。", float64(residual)/(1<<30), share)
			if s.Complete {
				fmt.Fprintf(&b, "已完整读取全部 %d 个进程，说明差额在共享区段、虚机/驱动或跨会话提交。\n", len(s.Procs))
			} else if inaccessible > 0 {
				fmt.Fprintf(&b, "其中有 %d 个进程无权限读取，所以这个比例偏高。\n", inaccessible)
			} else {
				fmt.Fprintf(&b, "没有任何进程无权限读取，说明大头在会话或驱动侧。\n")
			}
		}
	}
	if s.Snap.HasPages && s.Snap.Pages.Standby > 6<<30 && s.Snap.AvailablePercent() > 15 {
		fmt.Fprintf(&b, "  待机缓存 %.1f GB，但可用内存仍有 %.1f%%。这是缓存，不是压力。\n",
			float64(s.Snap.Pages.Standby)/(1<<30), s.Snap.AvailablePercent())
	}
	return b.String()
}

// ---------- trace ----------

func cmdTrace(args []string) {
	fs := flag.NewFlagSet("trace", flag.ExitOnError)
	minutes := fs.Float64("minutes", 30, "采样时长（分钟）")
	interval := fs.Duration("interval", 10*time.Second, "采样间隔")
	out := fs.String("out", "trace.jsonl", "输出文件")
	_ = fs.Parse(args)

	cfg, err := config.Load()
	if err != nil {
		fail("读取配置失败: %v", err)
	}
	f, err := os.Create(*out)
	if err != nil {
		fail("创建 %s 失败: %v", *out, err)
	}
	defer f.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*minutes*float64(time.Minute)))
	defer cancel()

	eng := engine.New(cfg, nil)
	enc := json.NewEncoder(f)
	count := 0
	opts := engine.Options{
		Interval: *interval,
		OnReading: func(s engine.Sample) {
			_ = enc.Encode(map[string]any{
				"time":               s.At.Format(time.RFC3339),
				"in_use_mb":          float64(s.Snap.InUse()) / (1 << 20),
				"available_mb":       float64(s.Snap.AvailPhys) / (1 << 20),
				"commit_mb":          float64(s.Snap.Commit) / (1 << 20),
				"kernel_paged_mb":    float64(s.Snap.KernelPaged) / (1 << 20),
				"kernel_nonpaged_mb": float64(s.Snap.KernelNonpaged) / (1 << 20),
				"standby_mb":         float64(s.Snap.Pages.Standby) / (1 << 20),
				"working_set_mb":     float64(s.WS) / (1 << 20),
				"private_ws_mb":      float64(s.PrivateWS) / (1 << 20),
				"private_mb":         float64(s.Private) / (1 << 20),
				"processes":          len(s.Procs),
				"process_complete":   s.Complete,
			})
			count++
			fmt.Printf("\r已写入 %d 个样本", count)
		},
	}
	fmt.Printf("MEMORA trace → %s（%.0f 分钟）\n", *out, *minutes)
	_ = eng.Run(ctx, opts)
	fmt.Printf("\n完成，%d 个样本写入 %s\n", count, *out)
}

// ---------- journal ----------

func cmdJournal(args []string) {
	fs := flag.NewFlagSet("journal", flag.ExitOnError)
	lines := fs.Int("lines", 20, "显示最后 N 条")
	path := fs.Bool("path", false, "只输出日志路径")
	_ = fs.Parse(args)

	jr, err := journal.Open(config.Dir())
	if err != nil {
		fail("%v", err)
	}
	if *path {
		fmt.Println(jr.Path())
		return
	}

	f, err := os.Open(jr.Path())
	if err != nil {
		fmt.Println("还没有日志。先运行 memora watch。")
		return
	}
	defer f.Close()

	var all []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		all = append(all, sc.Text())
	}
	start := len(all) - *lines
	if start < 0 {
		start = 0
	}
	for _, line := range all[start:] {
		fmt.Println(prettyEntry(line))
	}
}

func prettyEntry(line string) string {
	var e journal.Entry
	if err := json.Unmarshal([]byte(line), &e); err != nil {
		return line
	}
	switch e.Kind {
	case "action":
		targets := make([]string, 0, len(e.Targets))
		for _, t := range e.Targets {
			targets = append(targets, fmt.Sprintf("%s(%d)", t.Name, t.PID))
		}
		return fmt.Sprintf("%s  动作 %s  回收 %.0f MB  %s\n           原因: %s\n           对象: %s",
			e.Time.Format("2006-01-02 15:04:05"), e.Action, e.ReclaimedMB, e.Result,
			e.Reason, strings.Join(targets, ", "))
	case "start", "stop":
		return fmt.Sprintf("%s  %s  %s", e.Time.Format("2006-01-02 15:04:05"), e.Kind, e.Detail)
	default:
		return fmt.Sprintf("%s  %s  %s", e.Time.Format("2006-01-02 15:04:05"), e.Kind, e.Detail)
	}
}

// ---------- ledger ----------

func cmdLedger(args []string) {
	fs := flag.NewFlagSet("ledger", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "输出 JSON")
	reset := fs.Bool("reset", false, "重置累计账本")
	_ = fs.Parse(args)

	lg, err := ledger.Open(config.Dir())
	if err != nil {
		fail("读取静默账本失败: %v", err)
	}
	if *reset {
		if err := lg.Reset(); err != nil {
			fail("重置失败: %v", err)
		}
		fmt.Printf("已重置静默账本: %s\n", lg.Path())
		return
	}
	data := lg.Snapshot()
	if *asJSON {
		raw, _ := json.MarshalIndent(data, "", "  ")
		fmt.Println(string(raw))
		return
	}
	fmt.Print(formatLedgerText(data))
	fmt.Printf("\n账本文件: %s\n", lg.Path())
}

func formatLedgerText(d ledger.Data) string {
	var b strings.Builder
	hours := d.TotalGuardSeconds / 3600.0
	fmt.Fprintf(&b, "MEMORA 静默守护账本（Quiet Ledger）\n\n")
	if !d.StartedAt.IsZero() {
		fmt.Fprintf(&b, "建账时间   %s（累计启动 %d 次会话）\n", d.StartedAt.Format("2006-01-02 15:04"), d.Sessions)
	}
	fmt.Fprintf(&b, "静默护航   %.1f 小时（共 %d 次感知采样）\n", hours, d.TotalSamples)
	fmt.Fprintf(&b, "主动克制   %.1f%% 采样选择不打扰（%d / %d 次按兵不动）\n",
		d.RestraintRate(), d.RestrainedSamples, d.TotalSamples)
	fmt.Fprintf(&b, "全屏静音   因全屏应用/游戏专注主动跳过干涉 %d 次\n", d.FullscreenSkips)
	fmt.Fprintf(&b, "温和调度   后台低内存优先/EcoQoS 软调节 %d 次，回前台/唤醒恢复 %d 次\n",
		d.SoftDowngrades, d.SoftRestores)
	if d.TrimActions > 0 {
		fmt.Fprintf(&b, "高压干预   仅在吃紧阈值介入 %d 次（高压脱险 %d 次，平均单次真实回落 %.0f MB）\n",
			d.TrimActions, d.PeakSaves, d.AvgReclaimMB())
	} else {
		fmt.Fprintf(&b, "高压干预   0 次（系统始终未跌破压力阈值）\n")
	}
	fmt.Fprintf(&b, "孤儿收敛   捕获主程序退出后仍滞留的无窗子进程 %d 个\n", d.OrphansDetected)
	if d.KernelLeakObservations > 0 {
		fmt.Fprintf(&b, "内核暗伤   累计预警 %d 次（最近: %s）\n", d.KernelLeakObservations, d.LastKernelLeakDesc)
	}
	if len(d.DailyBaselines) > 0 {
		fmt.Fprintf(&b, "\n近期每日最低空载水位（越接近说明长期使用后越能回到空载）\n")
		start := len(d.DailyBaselines) - 7
		if start < 0 {
			start = 0
		}
		for _, day := range d.DailyBaselines[start:] {
			extra := ""
			if day.VidMB >= 100 {
				extra += fmt.Sprintf("  Vid %.1f GB", day.VidMB/1024)
			}
			if day.ProcessObjects > 0 {
				extra += fmt.Sprintf("  Process对象 %d", day.ProcessObjects)
			}
			fmt.Fprintf(&b, "  %s  最低物理占用 %.1f GB  最低进程私有提交 %.1f GB%s\n",
				day.Date, day.MinInUseMB/1024, day.MinPrivateMB/1024, extra)
		}
	}
	return b.String()
}

// ---------- config ----------

func cmdConfig(args []string) {
	fs := flag.NewFlagSet("config", flag.ExitOnError)
	reset := fs.Bool("reset", false, "恢复默认配置")
	_ = fs.Parse(args)

	if *reset {
		if err := config.Save(config.Default()); err != nil {
			fail("写入失败: %v", err)
		}
		fmt.Printf("已恢复默认配置: %s\n", config.Path())
		return
	}

	cfg, err := config.Load()
	if err != nil {
		fail("%v", err)
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	fmt.Printf("配置文件: %s\n\n%s\n", config.Path(), string(data))
}

// ---------- startup ----------

func cmdStartup(args []string) {
	if len(args) < 1 {
		fmt.Println("用法: memora startup enable [--tray] | disable | status")
		return
	}
	switch args[0] {
	case "enable":
		fs := flag.NewFlagSet("startup-enable", flag.ExitOnError)
		withTray := fs.Bool("tray", false, "以轻量系统托盘方式开机启动")
		_ = fs.Parse(args[1:])
		exe, err := os.Executable()
		if err != nil {
			fail("%v", err)
		}
		modeArgs := "watch --quiet --hidden"
		if *withTray {
			modeArgs = "tray --hidden"
		}
		value := fmt.Sprintf(`"%s" %s`, exe, modeArgs)
		if err := runReg("add", runKey, "/v", "MEMORA", "/t", "REG_SZ", "/d", value, "/f"); err != nil {
			fail("%v", err)
		}
		fmt.Printf("已设置开机启动: %s\n", value)
	case "disable":
		if err := runReg("delete", runKey, "/v", "MEMORA", "/f"); err != nil {
			fail("%v", err)
		}
		fmt.Println("已取消开机启动。")
	case "status":
		out, err := exec.Command("reg", "query", runKey, "/v", "MEMORA").CombinedOutput()
		if err != nil {
			fmt.Println("当前未设置开机启动。")
			return
		}
		for _, line := range strings.Split(string(out), "\n") {
			if i := strings.Index(line, "REG_SZ"); i >= 0 {
				fmt.Printf("已设置开机启动: %s\n", strings.TrimSpace(line[i+len("REG_SZ"):]))
				return
			}
		}
		fmt.Println(strings.TrimSpace(string(out)))
	default:
		fmt.Println("用法: memora startup enable|disable|status")
	}
}

func runReg(args ...string) error {
	out, err := exec.Command("reg", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("reg %s: %v %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ---------- helpers ----------

func selfStats(procs []proc.Info) (uint64, uint32) {
	if p, ok := proc.Find(procs, uint32(os.Getpid())); ok {
		return p.WorkingSet, p.Handles
	}
	return 0, 0
}

func human(v uint64) string {
	switch {
	case v >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(v)/(1<<30))
	case v >= 1<<20:
		return fmt.Sprintf("%.0f MB", float64(v)/(1<<20))
	case v >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(v)/(1<<10))
	default:
		return fmt.Sprintf("%d B", v)
	}
}

func humanU(v int64) string {
	if v < 0 {
		return "-" + human(uint64(-v))
	}
	return human(uint64(v))
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
