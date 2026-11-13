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
	"memora/internal/proc"
	"memora/internal/winmem"
)

const version = "0.1.0"

const runKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}
	switch os.Args[1] {
	case "status":
		cmdStatus(os.Args[2:])
	case "watch":
		cmdWatch(os.Args[2:])
	case "doctor":
		cmdDoctor(os.Args[2:])
	case "trace":
		cmdTrace(os.Args[2:])
	case "journal":
		cmdJournal(os.Args[2:])
	case "config":
		cmdConfig(os.Args[2:])
	case "startup":
		cmdStartup(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Print("memora " + version + "\n")
	default:
		usage()
	}
}

func usage() {
	fmt.Print(`MEMORA — Memory, Back to Its Nature.

用法:
  memora status [--json]        看一次当前状态
  memora watch  [--quiet]       常驻监视（--hidden 不显示控制台）
  memora doctor [--minutes N]   观察一段时间并给出归因结论
  memora trace  [--minutes N] [--out FILE]
                                把采样写成 JSONL
  memora journal [--lines N]    查看行为日志
  memora config [--reset]       查看/重置配置
  memora startup enable|disable|status
  memora version

说明:
  watch 默认按配置运行；config.json 里的 mode 为 observe 时只记录不动作。
  所有动作都会写进 journal，包含原因、前后数值与实测回收量。
`)
}

// ---------- status ----------

func cmdStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "输出 JSON")
	_ = fs.Parse(args)

	snap, err := winmem.Collect()
	if err != nil {
		fail("读取内存状态失败: %v", err)
	}
	procs, _ := proc.List()

	var ws, private uint64
	for _, p := range procs {
		ws += p.WorkingSet
		private += p.Private
	}
	residual := int64(snap.Commit) - int64(private) - int64(snap.KernelPaged) - int64(snap.KernelNonpaged)

	if *asJSON {
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
			"working_set_bytes":         ws,
			"private_bytes":             private,
			"unattributed_commit_bytes": residual,
			"memora_pid":                os.Getpid(),
			"memora_working_set":        selfWorkingSet(),
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
	inaccessible := int(snap.Processes) - len(procs)
	if inaccessible < 0 {
		inaccessible = 0
	}
	fmt.Printf("进程       %d 个   工作集合计 %s   私有提交合计 %s\n",
		len(procs), human(ws), human(private))
	if inaccessible > 0 {
		fmt.Printf("           系统报告 %d 个进程，其中 %d 个无权限读取，未计入上面的合计\n",
			snap.Processes, inaccessible)
	}

	label := "正常"
	if residual > 0 {
		label = "上限估计"
	}
	fmt.Printf("\n未归因提交 %s（%s）\n", humanU(residual), label)
	fmt.Printf("  这部分提交不属于已读取进程的私有内存，也不属于内核池。\n")
	fmt.Printf("  无权限进程与跨会话分配都在其中，所以它是上限，不是确数。\n\n")

	fmt.Printf("%-6s %10s %10s  %s\n", "PID", "工作集", "私有提交", "进程")
	sort.Slice(procs, func(i, j int) bool { return procs[i].WorkingSet > procs[j].WorkingSet })
	limit := 10
	for i, p := range procs {
		if i >= limit {
			break
		}
		fmt.Printf("%-6d %10s %10s  %s\n", p.PID, human(p.WorkingSet), human(p.Private), p.Name)
	}
	fmt.Printf("\nMEMORA 自身 %s / %d 个句柄\n", human(selfWorkingSet()), selfHandles())
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

	eng := engine.New(cfg, jr)

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
		fmt.Printf("MEMORA 守护中  模式=%s  间隔=%s  日志=%s\n", effective, shown, jr.Path())
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
	firstWS := map[uint32]uint64{}
	lastWS := map[uint32]uint64{}
	names := map[uint32]string{}
	count := 0

	opts := engine.Options{
		Interval: *interval,
		Act:      false,
		OnReading: func(s engine.Sample) {
			if count == 0 {
				first = s
				for _, p := range s.Procs {
					firstWS[p.PID] = p.WorkingSet
					names[p.PID] = p.Name
				}
			}
			last = s
			lastWS = map[uint32]uint64{}
			for _, p := range s.Procs {
				lastWS[p.PID] = p.WorkingSet
				names[p.PID] = p.Name
			}
			count++
			fmt.Printf("\r观察中 %d 个样本  可用 %.1f%%", count, s.Snap.AvailablePercent())
		},
	}

	fmt.Printf("MEMORA doctor  观察 %.0f 分钟，间隔 %s\n\n", *minutes, *interval)
	_ = eng.Run(ctx, opts)
	fmt.Println()

	if count < 2 {
		fail("样本不足，观察时间太短")
	}

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

	fmt.Printf("\n增长最快的进程\n")
	type grow struct {
		pid   uint32
		name  string
		delta int64
	}
	var growers []grow
	for pid, ws := range lastWS {
		base, ok := firstWS[pid]
		if !ok {
			continue
		}
		if d := int64(ws) - int64(base); d > 32<<20 {
			growers = append(growers, grow{pid: pid, name: names[pid], delta: d})
		}
	}
	sort.Slice(growers, func(i, j int) bool { return growers[i].delta > growers[j].delta })
	if len(growers) == 0 {
		fmt.Printf("  没有进程在观察期内增长超过 32 MB。\n")
	} else {
		for i, g := range growers {
			if i >= 5 {
				break
			}
			fmt.Printf("  %-28s %+8s  (PID %d)\n", g.name, humanU(g.delta), g.pid)
		}
	}

	fmt.Printf("\n结论\n")
	fmt.Print(conclusion(dInUse, dCommit, dResid, dPrivate, dWS, dStandby, dPaged, dNon))
	if extra := notes(last); extra != "" {
		fmt.Printf("\n值得单独看一眼\n")
		fmt.Print(extra)
	}
	fmt.Printf("\n观察 %d 个样本，%.1f 分钟。以上为真实读数，不是估算。\n", count, elapsedMin)
}

func conclusion(dInUse, dCommit, dResid, dPrivate, dWS, dStandby, dPaged, dNon float64) string {
	switch {
	case dCommit > 64 && dResid > 0.6*dCommit:
		return fmt.Sprintf("  提交增长主要落在未归因部分（%+.0f MB / 总 %+.0f MB）。\n"+
			"  这部分不在任何进程里，也不在内核池里，通常是驱动或会话级分配。\n"+
			"  MEMORA 不会强行回收它：用户态没有安全的手段做到这一点。\n", dResid, dCommit)
	case dCommit > 64 && dPrivate > 0.6*dCommit:
		return fmt.Sprintf("  提交增长主要来自进程私有内存（%+.0f MB / 总 %+.0f MB）。\n"+
			"  可从下面的进程列表定位；MEMORA 只会回收其中长期空闲的那些。\n", dPrivate, dCommit)
	case dWS > 128:
		return fmt.Sprintf("  进程工作集合计增长 %+.0f MB，属于活跃使用而非泄漏。\n"+
			"  工作集包含共享页，合计会重复计算，不能直接等同于物理占用。\n", dWS)
	case dStandby > 128:
		return fmt.Sprintf("  待机缓存增长 %+.0f MB。这是文件缓存，Windows 会按需回收，不是泄漏。\n", dStandby)
	case dPaged+dNon > 64:
		return fmt.Sprintf("  内核池增长 %+.0f MB（分页 %+.0f / 非分页 %+.0f）。\n"+
			"  持续单调增长通常指向驱动；MEMORA 记录它，但不干预。\n", dPaged+dNon, dPaged, dNon)
	case dInUse > 64:
		return fmt.Sprintf("  物理占用增长 %+.0f MB，但提交和进程都平稳。\n"+
			"  优先怀疑共享 section 或驱动锁定页。\n", dInUse)
	default:
		return "  观察期内没有出现有意义的增长。\n"
	}
}

func notes(s engine.Sample) string {
	var b strings.Builder
	commit := float64(s.Snap.Commit)
	nonpaged := float64(s.Snap.KernelNonpaged) / (1 << 30)
	residual := int64(s.Snap.Commit) - int64(s.Private) - int64(s.Snap.KernelPaged) - int64(s.Snap.KernelNonpaged)
	inaccessible := int(s.Snap.Processes) - len(s.Procs)

	if nonpaged >= 1.0 {
		fmt.Fprintf(&b, "  非分页池 %.1f GB。桌面系统常见 0.2–0.8 GB，长期高于 1 GB 时值得查驱动或安全软件。\n", nonpaged)
	}
	if commit > 0 && residual > 0 {
		share := float64(residual) / commit * 100
		if share >= 50 {
			fmt.Fprintf(&b, "  未归因提交 %.1f GB，占提交的 %.0f%%。", float64(residual)/(1<<30), share)
			if inaccessible > 0 {
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
				"private_mb":         float64(s.Private) / (1 << 20),
				"processes":          len(s.Procs),
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
		fmt.Println("用法: memora startup enable|disable|status")
		return
	}
	switch args[0] {
	case "enable":
		exe, err := os.Executable()
		if err != nil {
			fail("%v", err)
		}
		value := fmt.Sprintf(`"%s" watch --quiet --hidden`, exe)
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

func selfWorkingSet() uint64 {
	pid := uint32(os.Getpid())
	for _, p := range mustProcs() {
		if p.PID == pid {
			return p.WorkingSet
		}
	}
	return 0
}

func selfHandles() uint32 {
	pid := uint32(os.Getpid())
	for _, p := range mustProcs() {
		if p.PID == pid {
			return p.Handles
		}
	}
	return 0
}

func mustProcs() []proc.Info {
	ps, _ := proc.List()
	return ps
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
