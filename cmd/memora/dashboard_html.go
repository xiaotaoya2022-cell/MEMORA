package main

const dashboardHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width,initial-scale=1" />
  <title>MEMORA · 智能性能调度与内存真相中心</title>
  <link rel="icon" href="/favicon.ico" type="image/x-icon" />
  <link rel="apple-touch-icon" href="/logo.png" />
  <link rel="manifest" href="/manifest.json" />
  <style>
    :root {
      --bg: #f1f5f9;
      --card: #ffffff;
      --border: #e2e8f0;
      --text: #0f172a;
      --muted: #475569;
      --subtle: #64748b;
      --blue: #2563eb;
      --blue-soft: #eff6ff;
      --green: #10b981;
      --amber: #f59e0b;
      --red: #ef4444;
      --violet: #7c3aed;
      --radius: 14px;
      --shadow: 0 2px 10px rgba(15, 23, 42, 0.045);
      --font-ui: "Segoe UI Variable Text", "Noto Sans SC", "Microsoft YaHei UI", "Segoe UI", system-ui, sans-serif;
      --font-num: "Bahnschrift", "Segoe UI Variable Display", "Noto Sans SC", "Microsoft YaHei UI", sans-serif;
      --font-mono: "Cascadia Mono", "Consolas", ui-monospace, monospace;
    }
    * {
      box-sizing: border-box;
      scrollbar-width: thin;
      scrollbar-color: rgba(148, 163, 184, 0.6) transparent;
    }
    *::-webkit-scrollbar { width: 6px; height: 6px; }
    *::-webkit-scrollbar-track { background: transparent; }
    *::-webkit-scrollbar-thumb {
      background: rgba(148, 163, 184, 0.55);
      border-radius: 999px;
      border: 1px solid transparent;
    }
    *::-webkit-scrollbar-thumb:hover { background: rgba(100, 116, 139, 0.85); }

    html, body {
      margin: 0;
      padding: 0;
      width: 100%;
      height: 100%;
      background: var(--bg);
      color: var(--text);
      font-family: var(--font-ui);
      font-size: 13px;
      line-height: 1.44;
      overflow: hidden;
      text-rendering: optimizeLegibility;
      font-optical-sizing: auto;
      user-select: none;
    }
    button, input, select { font-family: inherit; }

    input, textarea, code, .proc-pid, .j-reason, .j-targets, .alert-body {
      user-select: text;
    }

    .ico {
      width: 14px;
      height: 14px;
      flex-shrink: 0;
      display: inline-block;
      vertical-align: middle;
      stroke: currentColor;
      fill: none;
      stroke-width: 2;
      stroke-linecap: round;
      stroke-linejoin: round;
    }
    .ico-sm { width: 13px; height: 13px; }
    .ico-xs { width: 11px; height: 11px; }

    /* Frameless 8-Direction Window Resize Strips */
    .win-resizer {
      position: fixed;
      z-index: 9990;
      background: transparent;
    }
    .win-resizer.r-n { top: 0; left: 10px; right: 10px; height: 5px; cursor: ns-resize; }
    .win-resizer.r-s { bottom: 0; left: 10px; right: 10px; height: 5px; cursor: ns-resize; }
    .win-resizer.r-w { left: 0; top: 10px; bottom: 10px; width: 5px; cursor: ew-resize; }
    .win-resizer.r-e { right: 0; top: 10px; bottom: 10px; width: 5px; cursor: ew-resize; }
    .win-resizer.r-nw { top: 0; left: 0; width: 11px; height: 11px; cursor: nwse-resize; z-index: 9995; }
    .win-resizer.r-ne { top: 0; right: 0; width: 11px; height: 11px; cursor: nesw-resize; z-index: 9995; }
    .win-resizer.r-sw { bottom: 0; left: 0; width: 11px; height: 11px; cursor: nesw-resize; z-index: 9995; }
    .win-resizer.r-se { bottom: 0; right: 0; width: 11px; height: 11px; cursor: nwse-resize; z-index: 9995; }
    .app-frame.is-maximized .win-resizer { display: none; }

    .app-frame {
      height: 100vh;
      width: 100%;
      display: flex;
      flex-direction: column;
      border: 1px solid rgba(15, 23, 42, 0.14);
      background: var(--bg);
      overflow: hidden;
    }
    .app-frame.is-maximized { border: none; }

    /* Custom Frameless App Title Bar */
    .topbar {
      position: relative;
      z-index: 60;
      height: 44px;
      flex-shrink: 0;
      background: #0f172a;
      color: #f8fafc;
      border-bottom: 1px solid #1e293b;
      padding: 0 6px 0 14px;
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 10px;
      user-select: none;
      cursor: default;
    }
    .topbar-left {
      display: flex;
      align-items: center;
      gap: 9px;
      flex: 1;
      min-width: 0;
      overflow: hidden;
    }
    .topbar-right {
      display: flex;
      align-items: center;
      gap: 8px;
      flex-shrink: 0;
    }
    .topbar-logo {
      width: 22px;
      height: 22px;
      border-radius: 6px;
      box-shadow: 0 0 0 1px rgba(255,255,255,0.16);
      pointer-events: none;
    }
    .topbar-title {
      font-weight: 700;
      font-size: 14px;
      letter-spacing: 0.25px;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }
    .topbar-badge {
      font-size: 11px;
      padding: 2px 8px;
      border-radius: 999px;
      background: rgba(255,255,255,0.1);
      color: #cbd5e1;
      font-weight: 600;
      white-space: nowrap;
      flex-shrink: 0;
    }
    .topbar-badge.admin-ok {
      background: rgba(16, 185, 129, 0.16);
      color: #6ee7b7;
      border: 1px solid rgba(16, 185, 129, 0.28);
    }
    .topbar-badge.admin-std {
      background: rgba(245, 158, 11, 0.16);
      color: #fcd34d;
      border: 1px solid rgba(245, 158, 11, 0.28);
    }
    .topbar-btn {
      border: 1px solid rgba(255,255,255,0.16);
      background: rgba(255,255,255,0.08);
      color: #e2e8f0;
      border-radius: 8px;
      padding: 5px 10px;
      font-size: 12px;
      font-weight: 600;
      cursor: pointer;
      display: inline-flex;
      align-items: center;
      gap: 5px;
      white-space: nowrap;
      flex-shrink: 0;
      transition: background 0.16s ease, border-color 0.16s ease, color 0.16s ease, transform 0.12s ease;
    }
    .topbar-btn:hover {
      background: rgba(37, 99, 235, 0.85);
      border-color: rgba(96, 165, 250, 0.65);
      color: #fff;
    }
    .topbar-btn:active { transform: scale(0.97); }

    /* Top-Right Unified Custom-Drawn Window Control Buttons & Hover Popovers */
    .win-controls {
      display: inline-flex;
      align-items: center;
      gap: 5px;
      margin-left: 4px;
      padding-left: 9px;
      border-left: 1px solid rgba(255,255,255,0.12);
      flex-shrink: 0;
    }
    .win-ctrl-btn {
      width: 34px;
      height: 28px;
      border: 1px solid rgba(255,255,255,0.1);
      border-radius: 8px;
      background: rgba(255,255,255,0.05);
      color: #e2e8f0;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      cursor: pointer;
      transition: background 0.16s ease, border-color 0.16s ease, color 0.16s ease, box-shadow 0.16s ease, transform 0.12s ease;
    }
    .win-ctrl-btn svg {
      width: 14px;
      height: 14px;
      stroke: currentColor;
      fill: none;
      stroke-width: 1.85;
      stroke-linecap: round;
      stroke-linejoin: round;
    }
    .win-ctrl-btn:hover {
      background: rgba(59, 130, 246, 0.22);
      border-color: rgba(147, 197, 253, 0.36);
      color: #ffffff;
    }
    .win-ctrl-btn:active { transform: scale(0.92); }
    #btn-win-min:hover {
      background: rgba(14, 165, 233, 0.22);
      border-color: rgba(125, 211, 252, 0.42);
      color: #bae6fd;
    }
    #btn-win-max:hover {
      background: rgba(99, 102, 241, 0.24);
      border-color: rgba(165, 180, 252, 0.42);
      color: #e0e7ff;
    }
    .win-lite-wrap {
      position: relative;
      display: inline-flex;
      align-items: center;
    }
    .win-ctrl-lite {
      color: #a7f3d0;
      border-color: rgba(16, 185, 129, 0.22);
      background: rgba(16, 185, 129, 0.08);
    }
    .win-ctrl-lite:hover {
      background: linear-gradient(135deg, rgba(16, 185, 129, 0.28), rgba(37, 99, 235, 0.26));
      border-color: rgba(110, 231, 183, 0.44);
      color: #6ee7b7;
      box-shadow: 0 2px 10px rgba(16, 185, 129, 0.2);
    }
    .win-lite-tip {
      position: absolute;
      top: calc(100% + 8px);
      right: 0;
      width: max-content;
      max-width: 260px;
      padding: 7px 11px;
      border-radius: 10px;
      background: rgba(15, 23, 42, 0.96);
      color: #f8fafc;
      border: 1px solid rgba(148, 163, 184, 0.26);
      box-shadow: 0 10px 26px rgba(15, 23, 42, 0.3);
      pointer-events: none;
      opacity: 0;
      transform: translateY(-4px) scale(0.96);
      transform-origin: top right;
      transition: opacity 0.16s ease, transform 0.2s cubic-bezier(0.22, 1, 0.36, 1);
      z-index: 200;
      text-align: left;
    }
    .win-lite-wrap:hover .win-lite-tip,
    .win-ctrl-btn:focus-visible + .win-lite-tip {
      opacity: 1;
      transform: translateY(0) scale(1);
    }
    .win-lite-tip-title {
      display: flex;
      align-items: center;
      gap: 6px;
      font-size: 12px;
      font-weight: 700;
      color: #f1f5f9;
      margin-bottom: 2px;
    }
    .win-ctrl-lite + .win-lite-tip .win-lite-tip-title { color: #6ee7b7; }
    .win-lite-tip-dot {
      width: 6px;
      height: 6px;
      border-radius: 50%;
      background: #10b981;
      box-shadow: 0 0 8px rgba(16, 185, 129, 0.75);
      flex-shrink: 0;
    }
    .win-lite-tip-dot.dot-sky {
      background: #38bdf8;
      box-shadow: 0 0 8px rgba(56, 189, 248, 0.75);
    }
    .win-lite-tip-dot.dot-indigo {
      background: #818cf8;
      box-shadow: 0 0 8px rgba(129, 140, 248, 0.75);
    }
    .win-lite-tip-sub {
      font-size: 11px;
      color: #cbd5e1;
      line-height: 1.4;
    }

    .shell {
      flex: 1;
      min-height: 0;
      width: 100%;
      max-width: 1560px;
      margin: 0 auto;
      padding: 12px 15px;
      display: grid;
      grid-template-columns: 282px 1fr;
      gap: 13px;
    }
    .app-frame.is-maximized .shell {
      max-width: 100%;
      padding: 14px 20px;
      grid-template-columns: 300px 1fr;
      gap: 16px;
    }
    .app-frame.is-maximized .switch-row {
      padding: 12px 16px;
    }
    .app-frame.is-maximized .list-rows {
      gap: 9px;
    }

    /* Restrained Card Spotlight Effect */
    .spot-surface {
      position: relative;
      --mx: -999px;
      --my: -999px;
      --spot-r: 360px;
      --spot-alpha: 0.055;
      --spot-radius: 14px;
    }
    .spot-surface::before {
      content: "";
      position: absolute;
      inset: 0;
      border-radius: inherit;
      clip-path: inset(0 round var(--spot-radius, 14px));
      background: radial-gradient(
        var(--spot-r) circle at var(--mx) var(--my),
        rgba(37, 99, 235, var(--spot-alpha)),
        rgba(59, 130, 246, 0.02) 45%,
        transparent 70%
      );
      opacity: 0;
      transition: opacity 0.24s ease;
      pointer-events: none;
      z-index: 1;
    }
    .spot-surface:hover::before { opacity: 1; }
    .spot-surface > * { position: relative; z-index: 2; }
    .spot-sm { --spot-r: 230px; --spot-alpha: 0.06; --spot-radius: 10px; }

    .card {
      background: var(--card);
      border: 1px solid var(--border);
      border-radius: var(--radius);
      box-shadow: var(--shadow);
      padding: 14px 16px;
    }

    /* Left Hero Column */
    .hero-card {
      display: flex;
      flex-direction: column;
      align-items: center;
      text-align: center;
      min-height: 0;
      overflow-y: auto;
      padding: 15px 14px;
    }
    .brand-row {
      width: 100%;
      display: flex;
      align-items: center;
      justify-content: space-between;
      margin-bottom: 8px;
    }
    .brand {
      display: flex;
      align-items: center;
      gap: 7px;
      font-weight: 700;
      font-size: 14px;
    }
    .brand-dot {
      width: 9px; height: 9px; border-radius: 50%;
      background: var(--blue);
      box-shadow: 0 0 0 4px rgba(37, 99, 235, 0.14);
    }
    .ver-pill {
      font-size: 11px;
      color: var(--muted);
      background: #f1f5f9;
      padding: 2px 8px;
      border-radius: 999px;
      font-weight: 600;
    }

    .gauge-wrap {
      position: relative;
      width: 138px;
      height: 138px;
      margin: 2px 0 8px;
      flex-shrink: 0;
    }
    .gauge-svg { width: 138px; height: 138px; transform: rotate(-90deg); }
    .gauge-bg { fill: none; stroke: #e2e8f0; stroke-width: 12; }
    .gauge-fg {
      fill: none; stroke: var(--green); stroke-width: 12;
      stroke-linecap: round;
      transition: stroke-dashoffset 0.55s cubic-bezier(0.22, 1, 0.36, 1), stroke 0.3s ease;
    }
    .gauge-center {
      position: absolute; inset: 0;
      display: flex; flex-direction: column;
      align-items: center; justify-content: center;
    }
    .gauge-pct { font-family: var(--font-num); font-size: 28px; font-weight: 700; letter-spacing: -0.4px; font-variant-numeric: tabular-nums; }
    .gauge-sub { font-size: 11px; color: var(--muted); }

    .metric-pair {
      width: 100%;
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 8px;
      padding: 8px 10px;
      background: #f8fafc;
      border: 1px solid #eef2f7;
      border-radius: 10px;
      margin: 4px 0 9px;
    }
    .metric-pair .val { font-family: var(--font-num); font-size: 15px; font-weight: 700; font-variant-numeric: tabular-nums; }
    .metric-pair .lbl { font-size: 11px; color: var(--muted); }

    .pill-row {
      display: flex;
      flex-wrap: wrap;
      gap: 5px;
      justify-content: center;
      margin-bottom: 10px;
      width: 100%;
    }
    .pill {
      font-size: 11px;
      padding: 3px 8px;
      border-radius: 999px;
      font-weight: 600;
      display: inline-flex;
      align-items: center;
      gap: 4px;
    }
    .pill-blue { background: var(--blue-soft); color: var(--blue); }
    .pill-green { background: #ecfdf5; color: #059669; }
    .pill-amber { background: #fffbeb; color: #b45309; }
    .pill-red { background: #fef2f2; color: #dc2626; }
    .pill-slate { background: #f1f5f9; color: #475569; }
    .pill-violet { background: #f5f3ff; color: #6d28d9; }

    .btn-primary {
      position: relative;
      overflow: hidden;
      width: 100%;
      border: none;
      border-radius: 10px;
      padding: 10px 14px;
      font-size: 14px;
      font-weight: 700;
      color: #fff;
      background: linear-gradient(135deg, #2563eb, #1d4ed8);
      cursor: pointer;
      box-shadow: 0 3px 8px rgba(37, 99, 235, 0.28), 0 1px 2px rgba(15, 23, 42, 0.14), inset 0 1px 0 rgba(255, 255, 255, 0.28);
      transition: transform 0.12s ease, filter 0.15s ease, box-shadow 0.15s ease;
      margin-bottom: 4px;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      gap: 6px;
    }
    .btn-primary::after {
      content: "";
      position: absolute;
      top: -20%;
      left: -65%;
      width: 48%;
      height: 140%;
      background: linear-gradient(
        105deg,
        rgba(255, 255, 255, 0) 0%,
        rgba(255, 255, 255, 0.14) 38%,
        rgba(255, 255, 255, 0.42) 50%,
        rgba(255, 255, 255, 0.14) 62%,
        rgba(255, 255, 255, 0) 100%
      );
      transform: skewX(-18deg);
      pointer-events: none;
      animation: btnPrimarySweep 3.8s cubic-bezier(0.25, 0.8, 0.25, 1) infinite;
    }
    @keyframes btnPrimarySweep {
      0% { left: -65%; opacity: 0; }
      8% { opacity: 1; }
      38% { left: 125%; opacity: 1; }
      42%, 100% { left: 125%; opacity: 0; }
    }
    .btn-primary:hover { filter: brightness(1.06); transform: translateY(-1px); box-shadow: 0 5px 14px rgba(37, 99, 235, 0.32), 0 2px 4px rgba(15, 23, 42, 0.14), inset 0 1px 0 rgba(255, 255, 255, 0.32); }
    .btn-primary:active { transform: translateY(0.5px) scale(0.99); box-shadow: 0 1px 4px rgba(37, 99, 235, 0.22), inset 0 1px 2px rgba(15, 23, 42, 0.15); }
    .btn-primary:disabled { opacity: 0.65; cursor: wait; }
    .btn-primary:disabled::after { animation: none; opacity: 0; }
    .btn-primary-hint {
      font-size: 11px;
      color: var(--muted);
      margin-bottom: 12px;
    }

    .btn-ghost {
      width: 100%;
      border: 1px solid var(--border);
      background: #fff;
      color: var(--text);
      border-radius: 10px;
      padding: 7px 12px;
      font-size: 12px;
      font-weight: 600;
      cursor: pointer;
      transition: background 0.15s ease, border-color 0.15s ease;
      margin-bottom: 9px;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      gap: 6px;
    }
    .btn-ghost:hover { background: #f8fafc; border-color: #cbd5e1; }
    .btn-ghost.btn-confirm {
      border-color: #f59e0b;
      background: #fffbeb;
      color: #b45309;
    }
    .btn-ghost:disabled { opacity: 0.6; cursor: wait; }
    @keyframes icoSpin { to { transform: rotate(360deg); } }
    .ico-spin { animation: icoSpin 0.9s linear infinite; transform-origin: center; }

    /* Elastic Sliding Pill Toggle Switch (智能调度 / 关闭) */
    .mode-switch {
      position: relative;
      width: 100%;
      display: grid;
      grid-template-columns: 1fr 1fr;
      background: #f1f5f9;
      border: 1px solid #e2e8f0;
      padding: 4px;
      border-radius: 11px;
      margin-bottom: 8px;
      overflow: visible;
      box-shadow: inset 0 1px 2px rgba(15, 23, 42, 0.08), 0 1px 2px rgba(15, 23, 42, 0.04);
    }
    .mode-slider-thumb {
      position: absolute;
      top: 4px;
      bottom: 4px;
      left: 4px;
      width: calc(50% - 4px);
      border-radius: 8px;
      background: linear-gradient(135deg, #2563eb, #1d4ed8);
      box-shadow: 0 3px 8px rgba(37, 99, 235, 0.28), 0 1px 2px rgba(15, 23, 42, 0.12), inset 0 1px 0 rgba(255, 255, 255, 0.26);
      transform: translate3d(0, 0, 0);
      transition: transform 0.48s cubic-bezier(0.34, 1.56, 0.64, 1), background 0.28s ease, box-shadow 0.28s ease;
      z-index: 2;
      pointer-events: none;
      overflow: hidden;
    }
    .mode-slider-thumb::before {
      content: "";
      position: absolute;
      inset: -120%;
      background: conic-gradient(
        from 0deg,
        transparent 0deg,
        transparent 290deg,
        rgba(147, 197, 253, 0.55) 325deg,
        rgba(255, 255, 255, 0.78) 344deg,
        transparent 360deg
      );
      animation: modeEdgeFlow 4.8s linear infinite;
      pointer-events: none;
    }
    .mode-slider-thumb::after {
      content: "";
      position: absolute;
      inset: 1.5px;
      border-radius: 6.5px;
      background: linear-gradient(135deg, #2563eb, #1d4ed8);
      pointer-events: none;
    }
    @keyframes modeEdgeFlow {
      to { transform: rotate(360deg); }
    }
    .mode-switch.is-off .mode-slider-thumb {
      transform: translate3d(100%, 0, 0);
      background: #ffffff;
      box-shadow: 0 2px 7px rgba(15, 23, 42, 0.12), inset 0 0 0 1px #e2e8f0;
    }
    .mode-switch.is-off .mode-slider-thumb::before,
    .mode-switch.is-off .mode-slider-thumb::after {
      opacity: 0;
      animation: none;
    }
    .mode-btn {
      position: relative;
      z-index: 3;
      border: none;
      background: transparent !important;
      border-radius: 8px;
      padding: 7px 6px;
      font-size: 13px;
      font-weight: 700;
      color: var(--muted);
      cursor: pointer;
      transition: color 0.25s ease, transform 0.12s ease;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      gap: 5px;
    }
    .mode-btn.active { color: #ffffff; }
    .mode-btn.mode-off.active { color: #334155; }
    .mode-btn:active { transform: scale(0.97); }

    .sync-footer {
      margin-top: auto;
      padding-top: 6px;
      font-size: 11px;
      color: var(--muted);
    }

    /* Right Workspace Column */
    .right-col {
      display: flex;
      flex-direction: column;
      gap: 11px;
      min-width: 0;
      min-height: 0;
      height: 100%;
    }
    .overview-card {
      flex-shrink: 0;
      padding: 12px 15px;
      overflow: visible;
    }
    .card-hdr {
      display: flex;
      align-items: center;
      justify-content: space-between;
      margin-bottom: 7px;
      gap: 10px;
      flex-wrap: wrap;
    }
    .card-title { font-size: 15px; font-weight: 700; letter-spacing: -0.1px; }
    .card-sub { font-size: 12px; color: var(--muted); }

    /* Seamless Capsule 5-Color Breakdown Bar + Hover Pop-Out (Tooltip on Bar Hover ONLY) */
    .breakdown-wrap {
      position: relative;
      margin: 5px 0 8px;
      padding: 4px 0;
      z-index: 25;
    }
    .breakdown-bar {
      height: 14px;
      border-radius: 999px;
      background: #e2e8f0;
      display: flex;
      align-items: stretch;
      gap: 0;
      padding: 0;
      overflow: visible;
      cursor: pointer;
      position: relative;
    }
    .seg {
      height: 100%;
      min-width: 0;
      border-radius: 0;
      position: relative;
      transition: width 0.45s ease, transform 0.2s cubic-bezier(0.34, 1.56, 0.64, 1), border-radius 0.16s ease, box-shadow 0.2s ease, opacity 0.18s ease, filter 0.18s ease;
    }
    .seg.seg-first { border-top-left-radius: 999px; border-bottom-left-radius: 999px; }
    .seg.seg-last { border-top-right-radius: 999px; border-bottom-right-radius: 999px; }
    .seg.seg-empty { display: none; }
    .breakdown-bar.is-hovering .seg:not(.is-hovered) {
      opacity: 0.56;
    }
    .seg.is-hovered {
      transform: scaleY(1.48) translateY(-1px);
      border-radius: 6px;
      filter: brightness(1.06);
      box-shadow: 0 5px 14px rgba(15, 23, 42, 0.26);
      z-index: 6;
    }
    .seg-apps { background: #3b82f6; }
    .seg-vid { background: #8b5cf6; }
    .seg-zombie { background: #ef4444; }
    .seg-kernel { background: #f59e0b; }
    .seg-avail { background: #10b981; }

    .bd-tooltip {
      position: absolute;
      top: calc(100% + 8px);
      left: 0;
      min-width: 280px;
      max-width: 390px;
      background: rgba(15, 23, 42, 0.96);
      backdrop-filter: blur(8px);
      color: #f8fafc;
      border: 1px solid rgba(148, 163, 184, 0.25);
      border-radius: 12px;
      padding: 10px 12px;
      box-shadow: 0 14px 32px rgba(15, 23, 42, 0.28);
      pointer-events: none;
      opacity: 0;
      transform: translateY(-4px);
      transition: opacity 0.16s ease, transform 0.16s ease, left 0.14s ease;
      z-index: 50;
      font-size: 12px;
    }
    .bd-tooltip.show {
      opacity: 1;
      transform: translateY(0);
    }
    .bd-tt-head {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 10px;
      margin-bottom: 5px;
      padding-bottom: 5px;
      border-bottom: 1px solid rgba(148, 163, 184, 0.2);
    }
    .bd-tt-title { font-weight: 700; display: flex; align-items: center; gap: 6px; font-size: 13px; }
    .bd-tt-val { font-family: var(--font-num); font-weight: 700; color: #93c5fd; font-variant-numeric: tabular-nums; }
    .bd-tt-body { color: #cbd5e1; line-height: 1.5; font-size: 12px; }
    .bd-tt-body b { color: #ffffff; }

    .legend-grid {
      display: grid;
      grid-template-columns: repeat(5, minmax(0, 1fr));
      gap: 8px;
      margin-bottom: 8px;
    }
    .legend-item {
      background: #f8fafc;
      border: 1px solid #eef2f7;
      border-radius: 10px;
      padding: 7px 9px;
      transition: border-color 0.18s ease, background 0.18s ease;
    }
    .legend-item:hover {
      border-color: #cbd5e1;
      background: #ffffff;
    }
    .legend-top {
      display: flex; align-items: center; gap: 5px;
      font-size: 11px; color: var(--muted); font-weight: 600;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }
    .dot { width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0; }
    .legend-val { font-family: var(--font-num); font-size: 15px; font-weight: 700; margin-top: 2px; font-variant-numeric: tabular-nums; }
    .legend-pct { font-size: 11px; color: var(--muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }

    .spark-grid {
      display: grid;
      grid-template-columns: repeat(4, minmax(0, 1fr));
      gap: 8px;
    }
    .spark-box {
      border: 1px solid #eef2f7;
      background: #fafcff;
      border-radius: 10px;
      padding: 6px 9px;
    }
    .spark-hdr {
      display: flex; justify-content: space-between; align-items: baseline;
      font-size: 11px; color: var(--muted); margin-bottom: 2px; gap: 6px; min-width: 0;
    }
    .spark-hdr > span { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0; }
    .spark-hdr strong { font-family: var(--font-num); font-size: 14px; font-weight: 700; color: var(--text); font-variant-numeric: tabular-nums; flex-shrink: 0; white-space: nowrap; }
    .spark-svg { width: 100%; height: 30px; display: block; }

    /* Highlighted & Synchronized Scale-Up + Count-Up/Down Tween Numbers */
    .num-hl {
      display: inline-block;
      font-family: var(--font-num);
      font-weight: 700;
      font-variant-numeric: tabular-nums;
      padding: 0 5px;
      margin: 0 1px;
      border-radius: 5px;
      transform-origin: center center;
      transition: transform 0.26s cubic-bezier(0.22, 1.38, 0.36, 1), box-shadow 0.26s ease, filter 0.24s ease, background 0.24s ease;
    }
    .num-hl.is-counting {
      transform: scale(1.22) translateY(-1px);
      filter: brightness(1.07);
      box-shadow: 0 4px 14px rgba(37, 99, 235, 0.28);
      position: relative;
      z-index: 3;
    }
    .val-tween {
      display: inline-block;
      font-family: var(--font-num);
      font-variant-numeric: tabular-nums;
      transform-origin: center center;
      transition: transform 0.26s cubic-bezier(0.22, 1.38, 0.36, 1), color 0.24s ease;
    }
    .val-tween.is-counting {
      transform: scale(1.15);
      color: var(--blue);
    }
    .hl-blue { color: #1d4ed8; background: #dbeafe; }
    .hl-emerald { color: #047857; background: #d1fae5; }
    .hl-amber { color: #b45309; background: #fef3c7; }
    .hl-red { color: #b91c1c; background: #fee2e2; }
    .hl-violet { color: #6d28d9; background: #ede9fe; }
    .hl-indigo { color: #4338ca; background: #e0e7ff; }

    /* Bottom Bounded Tab Workspace */
    .workspace-card {
      flex: 1;
      min-height: 0;
      display: flex;
      flex-direction: column;
      overflow: hidden;
      padding: 12px 15px;
    }
    .tabs {
      position: relative;
      display: flex;
      gap: 4px;
      border-bottom: 1px solid var(--border);
      padding-bottom: 9px;
      margin-bottom: 10px;
      flex-shrink: 0;
      flex-wrap: wrap;
    }
    .tab-slider-thumb {
      position: absolute;
      top: 0;
      left: 0;
      height: 31px;
      width: 0;
      border-radius: 8px;
      background: var(--blue-soft);
      border: 1px solid #dbeafe;
      box-shadow: 0 1px 4px rgba(37, 99, 235, 0.1);
      pointer-events: none;
      z-index: 0;
      transition: transform 0.42s cubic-bezier(0.34, 1.45, 0.64, 1), width 0.36s cubic-bezier(0.34, 1.45, 0.64, 1), opacity 0.2s ease;
    }
    .tab-btn {
      position: relative;
      z-index: 1;
      border: 1px solid transparent;
      background: transparent !important;
      padding: 6px 12px;
      border-radius: 8px;
      font-size: 13px;
      font-weight: 600;
      color: var(--muted);
      cursor: pointer;
      display: inline-flex;
      align-items: center;
      gap: 6px;
      white-space: nowrap;
      flex-shrink: 0;
      transition: color 0.2s ease;
    }
    .tab-btn:hover { color: var(--text); }
    .tab-btn.active { color: var(--blue); }
    .tab-count {
      font-family: var(--font-num);
      font-size: 11px;
      font-weight: 700;
      padding: 1px 6px;
      border-radius: 999px;
      background: #e2e8f0;
      color: #334155;
      transition: background 0.2s ease, color 0.2s ease;
    }
    .tab-btn.active .tab-count {
      background: #bfdbfe;
      color: #1e40af;
    }
    .tab-count.alert { background: #fee2e2; color: #dc2626; }

    .tab-pane {
      display: none;
      flex: 1;
      min-height: 0;
      overflow-y: auto;
      padding-right: 4px;
    }
    .tab-pane.active {
      display: flex;
      flex-direction: column;
      gap: 9px;
    }

    /* Smooth Gliding List Highlight Cursor */
    .cursor-list {
      position: relative;
    }
    .list-hover-cursor {
      position: absolute;
      top: 0;
      left: 0;
      width: 0;
      height: 0;
      border-radius: 10px;
      background: linear-gradient(90deg, rgba(37, 99, 235, 0.085) 0%, rgba(59, 130, 246, 0.03) 100%);
      box-shadow: inset 3px 0 0 #2563eb, 0 2px 8px rgba(37, 99, 235, 0.06);
      pointer-events: none;
      opacity: 0;
      z-index: 0;
      transition: transform 0.28s cubic-bezier(0.22, 1, 0.36, 1), width 0.24s cubic-bezier(0.22, 1, 0.36, 1), height 0.26s cubic-bezier(0.22, 1, 0.36, 1), opacity 0.18s ease;
    }
    .list-rows {
      position: relative;
      z-index: 1;
      display: flex;
      flex-direction: column;
      gap: 7px;
    }

    /* Switches Inside Tab 1 */
    .switch-row {
      position: relative;
      z-index: 1;
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 9px 12px;
      border-radius: 10px;
      border: 1px solid #eef2f7;
      background: rgba(255, 255, 255, 0.72);
      gap: 12px;
      transition: border-color 0.2s ease, background 0.2s ease;
    }
    .switch-row:hover { border-color: rgba(59, 130, 246, 0.35); background: transparent; }
    .switch-title {
      font-size: 13px;
      font-weight: 700;
      display: flex;
      align-items: center;
      gap: 8px;
    }
    .live-dot { width: 8px; height: 8px; border-radius: 50%; background: var(--green); flex-shrink: 0; }
    .live-dot.off { background: #cbd5e1; }
    .switch-desc { font-size: 12px; color: var(--muted); margin-top: 2px; line-height: 1.5; }
    .toggle {
      position: relative; width: 44px; height: 24px; flex-shrink: 0;
      display: inline-block;
    }
    .toggle input { opacity: 0; width: 0; height: 0; }
    .slider {
      position: absolute; cursor: pointer; inset: 0;
      background: #cbd5e1; border-radius: 999px;
      transition: background 0.25s ease;
    }
    .slider:before {
      content: ""; position: absolute; width: 18px; height: 18px; left: 3px; top: 3px;
      background: #fff; border-radius: 50%;
      box-shadow: 0 1px 4px rgba(15, 23, 42, 0.24);
      transition: transform 0.38s cubic-bezier(0.34, 1.56, 0.64, 1);
    }
    .toggle input:checked + .slider { background: var(--blue); }
    .toggle input:checked + .slider:before { transform: translateX(20px); }

    .baseline-tip {
      font-size: 12px;
      color: #475569;
      background: #f8fafc;
      border: 1px dashed #cbd5e1;
      border-radius: 10px;
      padding: 8px 12px;
      display: flex;
      align-items: flex-start;
      gap: 7px;
      line-height: 1.5;
    }

    /* Tab 2: Truth & Storm Governance */
    .split-two {
      display: grid;
      grid-template-columns: 1.05fr 1fr;
      gap: 12px;
      flex: 1;
      min-height: 0;
    }
    .pane-scroll {
      display: flex;
      flex-direction: column;
      gap: 8px;
      min-height: 0;
      overflow-y: auto;
      padding-right: 4px;
    }

    .alert-box {
      border-radius: 12px;
      padding: 11px 13px;
      border: 1px solid var(--border);
      background: #f8fafc;
    }
    .alert-red { border-color: #fecaca; background: #fff5f5; }
    .alert-violet { border-color: #ddd6fe; background: #f8f5ff; }
    .alert-amber { border-color: #fde68a; background: #fffbeb; }
    .alert-title {
      font-weight: 700;
      font-size: 13px;
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 8px;
      margin-bottom: 4px;
    }
    .alert-title-l {
      display: inline-flex;
      align-items: center;
      gap: 6px;
    }
    .alert-body { font-size: 12px; color: #334155; line-height: 1.5; }
    .alert-actions { display: flex; gap: 7px; margin-top: 8px; flex-wrap: wrap; }

    .btn-sm {
      border: 1px solid var(--border);
      background: #fff;
      color: var(--text);
      border-radius: 8px;
      padding: 5px 10px;
      font-size: 12px;
      font-weight: 600;
      cursor: pointer;
      display: inline-flex;
      align-items: center;
      gap: 5px;
      transition: background 0.14s ease, border-color 0.14s ease, transform 0.12s ease;
    }
    .btn-sm:hover { background: #f1f5f9; }
    .btn-sm:active { transform: scale(0.97); }
    .btn-danger { border-color: #fca5a5; color: #b91c1c; background: #fef2f2; }
    .btn-danger:hover { background: #fee2e2; }
    .btn-violet { border-color: #c4b5fd; color: #6d28d9; background: #f5f3ff; }
    .btn-violet:hover { background: #ede9fe; }
    .btn-auto-on { border-color: #6ee7b7; color: #047857; background: #ecfdf5; }
    .btn-auto-on:hover { background: #d1fae5; }

    .chip-box {
      border: 1px solid #eef2f7;
      background: #f8fafc;
      border-radius: 10px;
      padding: 8px 10px;
    }
    .chip-hdr {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 8px;
      margin-bottom: 6px;
      flex-wrap: wrap;
    }
    .chips { display: flex; flex-wrap: wrap; gap: 5px; }
    .chip {
      font-size: 11px;
      font-weight: 600;
      background: #fff;
      border: 1px solid #dbeafe;
      color: #1e40af;
      padding: 2px 7px;
      border-radius: 999px;
      display: inline-flex;
      align-items: center;
      gap: 4px;
    }
    .chip-x {
      border: none;
      background: transparent;
      color: #64748b;
      cursor: pointer;
      font-size: 12px;
      line-height: 1;
      padding: 0 2px;
    }
    .chip-x:hover { color: #dc2626; }
    .mini-add-row { display: flex; gap: 5px; margin-top: 6px; }
    .mini-input {
      flex: 1;
      min-width: 0;
      border: 1px solid var(--border);
      border-radius: 8px;
      padding: 4px 8px;
      font-size: 12px;
      outline: none;
      background: #fff;
    }
    .mini-input:focus { border-color: var(--blue); }

    table { width: 100%; border-collapse: collapse; font-size: 12px; position: relative; z-index: 1; }
    th, td { text-align: left; padding: 7px 6px; border-bottom: 1px solid #f1f5f9; }
    th { font-size: 11px; color: var(--muted); font-weight: 600; background: #fafbfd; position: sticky; top: 0; z-index: 3; }
    td.num, th.num { text-align: right; font-variant-numeric: tabular-nums; }
    td.num { font-family: var(--font-num); font-weight: 600; }
    tbody tr.hover-row { transition: background 0.15s ease; }

    /* Tab 3: Custom-Drawn Process List + Journal */
    .proc-split {
      display: grid;
      grid-template-columns: 1.4fr 1fr;
      gap: 12px;
      flex: 1;
      min-height: 0;
    }
    .sub-card {
      border: 1px solid #eef2f7;
      border-radius: 12px;
      background: #fafcff;
      display: flex;
      flex-direction: column;
      min-height: 0;
      overflow: hidden;
    }
    .sub-card-hdr {
      padding: 8px 12px;
      background: #fff;
      border-bottom: 1px solid #eef2f7;
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 8px;
      flex-wrap: wrap;
      flex-shrink: 0;
    }
    .proc-toolbar {
      position: relative;
      padding: 7px 10px;
      background: #f8fafc;
      border-bottom: 1px solid #eef2f7;
      display: flex;
      align-items: center;
      gap: 6px;
      flex-wrap: wrap;
      flex-shrink: 0;
    }
    .filter-slider-thumb {
      position: absolute;
      top: 0;
      left: 0;
      border-radius: 999px;
      background: var(--blue);
      box-shadow: 0 2px 6px rgba(37, 99, 235, 0.25);
      pointer-events: none;
      z-index: 0;
      transition: transform 0.38s cubic-bezier(0.34, 1.5, 0.64, 1), width 0.32s cubic-bezier(0.34, 1.5, 0.64, 1);
    }
    .filter-pill {
      position: relative;
      z-index: 1;
      border: 1px solid transparent;
      background: transparent !important;
      color: var(--muted);
      border-radius: 999px;
      padding: 3px 10px;
      font-size: 11px;
      font-weight: 600;
      cursor: pointer;
      display: inline-flex;
      align-items: center;
      gap: 4px;
      transition: color 0.2s ease;
    }
    .filter-pill:hover { color: var(--text); }
    .filter-pill.active { color: #ffffff; }

    .proc-list {
      flex: 1;
      min-height: 0;
      overflow-y: auto;
      padding: 8px 10px;
    }
    .proc-row {
      position: relative;
      z-index: 1;
      background: rgba(255, 255, 255, 0.86);
      border: 1px solid #e8eef5;
      border-radius: 10px;
      padding: 8px 10px;
      display: flex;
      align-items: center;
      gap: 10px;
      transition: border-color 0.2s ease, background 0.2s ease;
    }
    .proc-row:hover {
      border-color: rgba(59, 130, 246, 0.35);
      background: rgba(255, 255, 255, 0.45);
    }
    .proc-row.is-storm {
      border-color: #fecaca;
      background: linear-gradient(90deg, rgba(255, 245, 245, 0.92), rgba(255, 255, 255, 0.85) 35%);
    }
    .proc-avatar {
      width: 32px;
      height: 32px;
      border-radius: 8px;
      display: flex;
      align-items: center;
      justify-content: center;
      font-weight: 800;
      font-size: 12px;
      flex-shrink: 0;
      background: #ffffff;
      border: 1px solid #e2e8f0;
      color: #1d4ed8;
      overflow: hidden;
      position: relative;
    }
    .proc-avatar img {
      width: 24px;
      height: 24px;
      object-fit: contain;
      display: block;
    }
    .proc-avatar-fallback {
      display: none;
      width: 100%;
      height: 100%;
      align-items: center;
      justify-content: center;
      background: #eff6ff;
      color: #1d4ed8;
    }
    .proc-row.is-storm .proc-avatar {
      border-color: #fca5a5;
    }
    .proc-row.is-storm .proc-avatar-fallback {
      background: #fee2e2;
      color: #dc2626;
    }
    .mini-proc-cell {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      vertical-align: middle;
    }
    .mini-proc-ico {
      width: 16px;
      height: 16px;
      object-fit: contain;
      border-radius: 3px;
      flex-shrink: 0;
      display: inline-block;
    }
    .whitelist-bar {
      display: flex;
      flex-direction: column;
      gap: 6px;
      padding: 8px 10px;
      margin-bottom: 8px;
      border-radius: 9px;
      background: #f8fafc;
      border: 1px solid #e2e8f0;
    }
    .whitelist-bar-top {
      display: flex;
      align-items: center;
      gap: 6px;
      flex-wrap: wrap;
    }
    .whitelist-chips {
      display: flex;
      flex-wrap: wrap;
      gap: 5px;
    }
    .wl-chip {
      display: inline-flex;
      align-items: center;
      gap: 4px;
      padding: 2px 7px;
      border-radius: 6px;
      background: #ecfdf5;
      border: 1px solid #a7f3d0;
      color: #047857;
      font-size: 11px;
      font-weight: 600;
    }
    .wl-chip button {
      border: none;
      background: transparent;
      color: #059669;
      cursor: pointer;
      padding: 0 1px;
      font-size: 12px;
      line-height: 1;
    }
    .wl-chip button:hover { color: #dc2626; }
    .proc-main { flex: 1; min-width: 0; }
    .proc-name-line {
      display: flex;
      align-items: center;
      gap: 6px;
      flex-wrap: wrap;
    }
    .proc-name {
      font-weight: 700;
      font-size: 13px;
      color: var(--text);
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      max-width: 210px;
    }
    .proc-pid {
      font-family: var(--font-mono);
      font-size: 11px;
      color: #475569;
      background: #f1f5f9;
      padding: 1px 5px;
      border-radius: 5px;
    }
    .proc-tags { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 4px; }
    .proc-metrics {
      width: 135px;
      flex-shrink: 0;
      text-align: right;
    }
    .proc-mem-txt {
      font-family: var(--font-num);
      font-size: 12px;
      font-weight: 700;
      font-variant-numeric: tabular-nums;
      color: var(--text);
    }
    .proc-mem-sub {
      font-family: var(--font-num);
      font-size: 11px;
      color: var(--muted);
      font-variant-numeric: tabular-nums;
    }
    .proc-bar-bg {
      margin-top: 3px;
      height: 4px;
      border-radius: 999px;
      background: #e2e8f0;
      overflow: hidden;
    }
    .proc-bar-fg {
      height: 100%;
      border-radius: 999px;
      background: linear-gradient(90deg, #3b82f6, #6366f1);
    }
    .proc-row.is-storm .proc-bar-fg { background: linear-gradient(90deg, #f87171, #ef4444); }

    .journal-list {
      flex: 1;
      min-height: 0;
      overflow-y: auto;
      padding: 8px 10px;
    }
    .j-item {
      position: relative;
      z-index: 1;
      border: 1px solid #eef2f7;
      border-radius: 10px;
      padding: 8px 10px;
      background: rgba(255, 255, 255, 0.86);
      transition: border-color 0.2s ease, background 0.2s ease;
    }
    .j-item:hover {
      border-color: rgba(59, 130, 246, 0.35);
      background: rgba(255, 255, 255, 0.45);
    }
    .j-top {
      display: flex; justify-content: space-between; gap: 8px;
      font-size: 11px; color: var(--muted); margin-bottom: 3px;
    }
    .j-reason { font-size: 12px; font-weight: 600; color: var(--text); }
    .j-targets { font-size: 11px; color: var(--muted); margin-top: 3px; }

    /* Tab 4: Quiet Ledger */
    .ledger-grid {
      display: grid;
      grid-template-columns: repeat(4, minmax(0, 1fr));
      gap: 9px;
      margin-bottom: 10px;
    }
    .ledger-stat {
      background: #f8fafc;
      border: 1px solid #eef2f7;
      border-radius: 11px;
      padding: 10px 12px;
    }
    .ledger-stat .k { font-size: 11px; color: var(--muted); }
    .ledger-stat .v { font-family: var(--font-num); font-size: 18px; font-weight: 700; margin-top: 2px; font-variant-numeric: tabular-nums; }
    .ledger-stat .s { font-size: 11px; color: var(--muted); margin-top: 2px; }
    .ledger-bottom-split {
      display: grid;
      grid-template-columns: 1.05fr 1fr;
      gap: 10px;
      flex: 1;
      min-height: 145px;
    }

    .toast {
      position: fixed;
      bottom: 18px; right: 18px;
      background: #0f172a; color: #fff;
      padding: 10px 14px;
      border-radius: 10px;
      font-size: 13px; font-weight: 600;
      box-shadow: 0 10px 25px rgba(15, 23, 42, 0.28);
      opacity: 0; transform: translateY(10px);
      pointer-events: none;
      transition: all 0.2s ease;
      max-width: 480px;
      z-index: 9998;
    }
    .toast.show { opacity: 1; transform: translateY(0); pointer-events: auto; }
    .export-toast {
      position: fixed;
      top: 52px;
      right: 16px;
      display: flex;
      align-items: center;
      gap: 10px;
      padding: 9px 12px;
      border-radius: 10px;
      background: rgba(15, 23, 42, 0.94);
      color: #f8fafc;
      border: 1px solid rgba(148, 163, 184, 0.24);
      box-shadow: 0 10px 26px rgba(15, 23, 42, 0.28);
      backdrop-filter: blur(10px);
      font-size: 12px;
      z-index: 9999;
      opacity: 0;
      transform: translateY(-6px);
      pointer-events: none;
      transition: opacity 0.18s ease, transform 0.18s ease;
      max-width: 460px;
    }
    .export-toast.show {
      opacity: 1;
      transform: translateY(0);
      pointer-events: auto;
    }
    .export-toast-btn {
      border: 1px solid rgba(148, 163, 184, 0.35);
      background: rgba(255, 255, 255, 0.08);
      color: #e2e8f0;
      border-radius: 6px;
      padding: 3px 8px;
      font-size: 11px;
      font-weight: 600;
      cursor: pointer;
      white-space: nowrap;
    }
    .export-toast-btn:hover { background: rgba(255, 255, 255, 0.16); color: #fff; }

    /* Multi-Stage Responsive Adaptation Down to Minimum Window 980x640 */
    .ledger-bottom-split th { white-space: nowrap; }
    .ledger-stat { min-width: 0; }
    .ledger-stat .k, .ledger-stat .s { line-height: 1.3; word-break: break-word; }

    @media (max-width: 1260px) {
      .tab-btn { padding: 5px 10px; font-size: 12.5px; gap: 5px; }
      .ledger-bottom-split th { font-size: 11px; }
    }
    @media (max-width: 1180px) {
      .shell { grid-template-columns: 256px 1fr; gap: 11px; padding: 10px 12px; }
      .hero-card { padding: 12px; }
      .gauge-wrap { width: 122px; height: 122px; }
      .gauge-svg { width: 122px; height: 122px; }
      .gauge-pct { font-size: 24px; }
      .proc-metrics { width: 112px; }
      .proc-name { max-width: 155px; }
      .topbar-title { font-size: 13px; }
      .tab-btn { padding: 5px 8px; font-size: 12px; gap: 4px; }
      .btn-ghost { font-size: 11.5px; padding: 7px 8px; white-space: normal; text-align: center; line-height: 1.25; }
      .topbar-badge#baseline-pill { display: none; }
    }
    @media (max-width: 1040px) {
      .shell { grid-template-columns: 238px 1fr; gap: 10px; padding: 9px 10px; }
      .legend-grid { gap: 6px; }
      .legend-item { padding: 6px 7px; }
      .legend-val { font-size: 13px; }
      .spark-grid, .ledger-grid { gap: 6px; }
      .topbar-badge#baseline-pill { display: none; }
      .tab-btn { padding: 4px 7px; font-size: 11.5px; gap: 4px; }
      .metric-pair .val { font-size: 13.5px; }
      .metric-pair .lbl { font-size: 10.5px; }
      .mode-btn { font-size: 11.5px; padding: 6px 4px; }
      .ledger-stat { padding: 8px 9px; }
      .ledger-stat .v { font-size: 15px; }
    }
    @media (max-height: 730px) {
      .overview-card { padding: 10px 13px; }
      .card-hdr { margin-bottom: 5px; }
      .breakdown-wrap { margin: 3px 0 6px; }
      .legend-grid { margin-bottom: 6px; }
      .spark-svg { height: 24px; }
      .gauge-wrap { width: 114px; height: 114px; margin: 0 0 6px; }
      .gauge-svg { width: 114px; height: 114px; }
      .gauge-pct { font-size: 22px; }
      .metric-pair { padding: 6px 8px; margin: 2px 0 6px; }
      .pill-row { margin-bottom: 7px; }
      .btn-primary { padding: 8px 10px; font-size: 13px; }
      .btn-ghost { padding: 6px 10px; }
      .switch-row { padding: 7px 10px; }
    }
    @media (max-width: 860px) {
      html, body { height: auto; overflow-y: auto; }
      .app-frame { height: auto; }
      .shell { grid-template-columns: 1fr; }
      .split-two, .proc-split, .ledger-bottom-split { grid-template-columns: 1fr; }
      .legend-grid, .spark-grid, .ledger-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    }
  </style>
</head>
<body>
  <!-- 8 Invisible Frameless Resize Strips -->
  <div class="win-resizer r-n" data-resize="resize_n"></div>
  <div class="win-resizer r-s" data-resize="resize_s"></div>
  <div class="win-resizer r-w" data-resize="resize_w"></div>
  <div class="win-resizer r-e" data-resize="resize_e"></div>
  <div class="win-resizer r-nw" data-resize="resize_nw"></div>
  <div class="win-resizer r-ne" data-resize="resize_ne"></div>
  <div class="win-resizer r-sw" data-resize="resize_sw"></div>
  <div class="win-resizer r-se" data-resize="resize_se"></div>

  <div class="app-frame" id="app-frame">
    <!-- Pure Custom Frameless Title Bar -->
    <header class="topbar" id="titlebar">
      <div class="topbar-left">
        <img class="topbar-logo" src="/logo.png" alt="MEMORA" />
        <span class="topbar-title" id="topbar-title">MEMORA · 智能性能调度与内存真相中心</span>
        <span class="topbar-badge" id="ver-pill">v0.3.0</span>
        <span class="topbar-badge admin-ok" id="admin-pill">权限检测中</span>
      </div>
      <div class="topbar-right">
        <span class="topbar-badge" id="baseline-pill">空载基线守护中</span>
        <button class="topbar-btn" id="btn-lang" onclick="toggleLanguage()" title="切换语言 / Switch Language" aria-label="Language Toggle">
          <svg class="ico ico-sm" viewBox="0 0 24 24"><circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>
          <span id="lang-label">EN</span>
        </button>
        <button class="topbar-btn" onclick="doExportReport()">
          <svg class="ico ico-sm" viewBox="0 0 24 24"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
          <span id="topbar-export-text">导出 24h 诊断报告</span>
        </button>
        <div class="win-controls">
          <div class="win-lite-wrap">
            <button class="win-ctrl-btn" id="btn-win-min" onclick="winCtl('minimize')" aria-label="最小化">
              <svg viewBox="0 0 14 14">
                <line x1="2.8" y1="7" x2="11.2" y2="7" stroke-width="1.65"/>
              </svg>
            </button>
            <div class="win-lite-tip" role="tooltip">
              <div class="win-lite-tip-title"><span class="win-lite-tip-dot dot-sky"></span><span>收起至任务栏</span></div>
              <div class="win-lite-tip-sub">最小化窗口至底部任务栏，保持主界面常驻就绪</div>
            </div>
          </div>

          <div class="win-lite-wrap">
            <button class="win-ctrl-btn" id="btn-win-max" onclick="winCtl('maximize')" aria-label="最大化或还原">
              <span id="btn-win-max-ico" style="display:inline-flex">
                <svg viewBox="0 0 14 14">
                  <rect x="2.7" y="2.7" width="8.6" height="8.6" rx="1.6" stroke-width="1.55"/>
                </svg>
              </span>
            </button>
            <div class="win-lite-tip" role="tooltip">
              <div class="win-lite-tip-title"><span class="win-lite-tip-dot dot-indigo"></span><span id="tip-max-title">全屏展开视图</span></div>
              <div class="win-lite-tip-sub" id="tip-max-sub">最大化展开工作区，获得更充裕的进程与账本视野</div>
            </div>
          </div>

          <div class="win-lite-wrap">
            <button class="win-ctrl-btn win-ctrl-lite" id="btn-win-close" onclick="winCtl('close')" aria-label="轻量模式">
              <svg viewBox="0 0 14 14">
                <path d="M8.8 2.2H4.2a2 2 0 0 0-2 2v5.6a2 2 0 0 0 2 2h4.6" stroke-opacity="0.48"/>
                <line x1="11.8" y1="2.2" x2="11.8" y2="11.8" stroke-width="2.1"/>
                <path d="M4.8 7h4.3M7.2 4.9L9.4 7 7.2 9.1"/>
              </svg>
            </button>
            <div class="win-lite-tip" role="tooltip">
              <div class="win-lite-tip-title"><span class="win-lite-tip-dot"></span><span>轻量模式</span></div>
              <div class="win-lite-tip-sub">收起主窗至托盘与贴边光尺，闲置自动释放界面内存</div>
            </div>
          </div>
        </div>
      </div>
    </header>

    <div class="shell">
      <!-- Left Hero Column -->
      <aside class="card spot-surface hero-card">
        <div class="brand-row">
          <div class="brand"><span class="brand-dot"></span><span id="hero-title-text">性能调度面板</span></div>
          <span class="ver-pill" id="mode-pill">智能调度</span>
        </div>

        <div class="gauge-wrap">
          <svg class="gauge-svg" viewBox="0 0 160 160">
            <circle class="gauge-bg" cx="80" cy="80" r="66"></circle>
            <circle class="gauge-fg" id="gauge-fg" cx="80" cy="80" r="66"
                    stroke-dasharray="414.69" stroke-dashoffset="414.69"></circle>
          </svg>
          <div class="gauge-center">
            <div class="gauge-pct val-tween" id="gauge-pct">--%</div>
            <div class="gauge-sub" id="gauge-sub-text">物理内存占用率</div>
          </div>
        </div>

        <div class="metric-pair">
          <div>
            <div class="val val-tween" id="hero-used">--</div>
            <div class="lbl" id="hero-total">已用 / 总量 --</div>
          </div>
          <div>
            <div class="val val-tween" id="hero-avail">--</div>
            <div class="lbl" id="hero-avail-lbl">可用物理内存</div>
          </div>
        </div>

        <div class="pill-row">
          <span class="pill pill-blue" id="tier-pill">计算档位中...</span>
          <span class="pill pill-slate" id="game-pill">前沿检测中</span>
          <span class="pill pill-slate" id="family-pill">前景保护 --</span>
          <span class="pill pill-slate" id="self-pill">MEMORA 自身 --</span>
        </div>

        <button class="btn-primary" id="btn-optimize" onclick="doOptimize()">
          <svg class="ico" viewBox="0 0 24 24"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>
          <span id="btn-opt-text">立即深度释压</span>
        </button>
        <div class="btn-primary-hint" id="btn-primary-hint-text">安全收紧闲置后台/服务工作集、文件缓存与低优待机页</div>
        <button class="btn-ghost" id="btn-optimize-extreme" onclick="doOptimizeExtreme()" title="手动全域清空所有空闲工作集、系统文件缓存与待机列表（极客模式，仅手动触发）">
          <span id="btn-opt-ext-ico" style="display:inline-flex"><svg class="ico ico-sm" style="color:#2563eb" viewBox="0 0 24 24"><polygon points="11 2 3 13 10 13 9 22 17 11 10 11 11 2"/><path d="M18 5l-3 5h4l-3 5"/></svg></span>
          <span id="btn-opt-ext-text">强力全域释压（含服务与缓存）</span>
        </button>

        <div class="mode-switch" id="mode-switch">
          <div class="mode-slider-thumb" id="mode-slider-thumb"></div>
          <button class="mode-btn active" id="mode-guard" onclick="setMode(true)">智能调度</button>
          <button class="mode-btn mode-off" id="mode-observe" onclick="setMode(false)">关闭</button>
        </div>

        <div class="sync-footer" id="sync-note">正在连接本地真实状态源...</div>
      </aside>

      <!-- Right Content Column -->
      <main class="right-col">
        <!-- Top Compact Overview Card -->
        <section class="card spot-surface overview-card">
          <div class="card-hdr">
            <div>
              <div class="card-title"><span id="overview-title-text">性能概览与物理内存真实拆解</span> <span class="card-sub" id="overview-sub-text" style="font-weight:400">(仅悬停用量彩条查看精细拆解)</span></div>
              <div class="card-sub" id="commit-summary">提交与页面链表加载中...</div>
            </div>
            <div class="pill-row" style="width:auto;margin:0">
              <span class="pill pill-slate" id="proc-count-pill">活跃进程 --</span>
            </div>
          </div>

          <div class="breakdown-wrap" id="bd-wrap">
            <div class="breakdown-bar" id="bd-bar">
              <div class="seg seg-apps" id="seg-apps" data-seg="apps" style="width:0%"></div>
              <div class="seg seg-vid" id="seg-vid" data-seg="vid" style="width:0%"></div>
              <div class="seg seg-zombie" id="seg-zombie" data-seg="zombie" style="width:0%"></div>
              <div class="seg seg-kernel" id="seg-kernel" data-seg="kernel" style="width:0%"></div>
              <div class="seg seg-avail seg-first seg-last" id="seg-avail" data-seg="avail" style="width:100%"></div>
            </div>
            <div class="bd-tooltip" id="bd-tooltip"></div>
          </div>

          <div class="legend-grid">
            <div class="legend-item spot-surface spot-sm" data-seg="apps">
              <div class="legend-top"><span class="dot" style="background:#3b82f6"></span><span id="bd-apps-label">应用独占工作集</span></div>
              <div class="legend-val val-tween" id="bd-apps-val">--</div>
              <div class="legend-pct" id="bd-apps-pct">--</div>
            </div>
            <div class="legend-item spot-surface spot-sm" data-seg="vid">
              <div class="legend-top"><span class="dot" style="background:#8b5cf6"></span><span id="bd-vid-label">后台静默驻留</span></div>
              <div class="legend-val val-tween" id="bd-vid-val">--</div>
              <div class="legend-pct" id="bd-vid-pct">--</div>
            </div>
            <div class="legend-item spot-surface spot-sm" data-seg="zombie">
              <div class="legend-top"><span class="dot" style="background:#ef4444"></span><span id="bd-zombie-label">僵尸 / 内核泄漏</span></div>
              <div class="legend-val val-tween" id="bd-zombie-val">--</div>
              <div class="legend-pct" id="bd-zombie-pct">--</div>
            </div>
            <div class="legend-item spot-surface spot-sm" data-seg="kernel">
              <div class="legend-top"><span class="dot" style="background:#f59e0b"></span><span id="bd-kernel-label">常规内核 · 共享与缓存</span></div>
              <div class="legend-val val-tween" id="bd-kernel-val">--</div>
              <div class="legend-pct" id="bd-kernel-pct">--</div>
            </div>
            <div class="legend-item spot-surface spot-sm" data-seg="avail">
              <div class="legend-top"><span class="dot" style="background:#10b981"></span><span id="bd-avail-label">可用内存(含待机)</span></div>
              <div class="legend-val val-tween" id="bd-avail-val">--</div>
              <div class="legend-pct" id="bd-avail-pct">--</div>
            </div>
          </div>

          <div class="spark-grid">
            <div class="spark-box spot-surface spot-sm">
              <div class="spark-hdr"><span id="sp-use-lbl">物理内存使用率</span><strong class="val-tween" id="sp-use-val">--</strong></div>
              <svg class="spark-svg" id="sp-use" viewBox="0 0 180 30" preserveAspectRatio="none"></svg>
            </div>
            <div class="spark-box spot-surface spot-sm">
              <div class="spark-hdr"><span id="sp-apps-lbl">应用独占工作集</span><strong class="val-tween" id="sp-apps-val">--</strong></div>
              <svg class="spark-svg" id="sp-apps" viewBox="0 0 180 30" preserveAspectRatio="none"></svg>
            </div>
            <div class="spark-box spot-surface spot-sm">
              <div class="spark-hdr"><span id="sp-spawn-lbl">子进程派生速率</span><strong class="val-tween" id="sp-spawn-val">--</strong></div>
              <svg class="spark-svg" id="sp-spawn" viewBox="0 0 180 30" preserveAspectRatio="none"></svg>
            </div>
            <div class="spark-box spot-surface spot-sm">
              <div class="spark-hdr"><span id="sp-avail-lbl">可立即可用内存</span><strong class="val-tween" id="sp-avail-val">--</strong></div>
              <svg class="spark-svg" id="sp-avail" viewBox="0 0 180 30" preserveAspectRatio="none"></svg>
            </div>
          </div>
        </section>

        <!-- Bottom Bounded Tab Workspace -->
        <section class="card spot-surface workspace-card">
          <div class="tabs" id="main-tabs">
            <div class="tab-slider-thumb" id="tab-slider-thumb"></div>
            <button class="tab-btn active" data-tab="smart" onclick="switchTab('smart')">
              <svg class="ico ico-sm" viewBox="0 0 24 24"><line x1="4" y1="21" x2="4" y2="14"/><line x1="4" y1="10" x2="4" y2="3"/><line x1="12" y1="21" x2="12" y2="12"/><line x1="12" y1="8" x2="12" y2="3"/><line x1="20" y1="21" x2="20" y2="16"/><line x1="20" y1="12" x2="20" y2="3"/><line x1="1" y1="14" x2="7" y2="14"/><line x1="9" y1="8" x2="15" y2="8"/><line x1="17" y1="16" x2="23" y2="16"/></svg>
              <span id="tab-title-smart">智能感知与动态调度</span>
            </button>
            <button class="tab-btn" data-tab="truth" onclick="switchTab('truth')">
              <svg class="ico ico-sm" viewBox="0 0 24 24"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
              <span id="tab-title-truth">真相归因与派生治理</span>
              <span class="tab-count" id="tab-badge-truth">健康</span>
            </button>
            <button class="tab-btn" data-tab="managed" onclick="switchTab('managed')">
              <svg class="ico ico-sm" viewBox="0 0 24 24"><rect x="4" y="4" width="16" height="16" rx="2"/><rect x="9" y="9" width="6" height="6"/><line x1="9" y1="1" x2="9" y2="4"/><line x1="15" y1="1" x2="15" y2="4"/><line x1="9" y1="20" x2="9" y2="23"/><line x1="15" y1="20" x2="15" y2="23"/><line x1="20" y1="9" x2="23" y2="9"/><line x1="20" y1="14" x2="23" y2="14"/><line x1="1" y1="9" x2="4" y2="9"/><line x1="1" y1="14" x2="4" y2="14"/></svg>
              <span id="tab-title-managed">进程视图与日志</span>
              <span class="tab-count" id="tab-badge-managed">0</span>
            </button>
            <button class="tab-btn" data-tab="ledger" onclick="switchTab('ledger')">
              <svg class="ico ico-sm" viewBox="0 0 24 24"><path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"/></svg>
              <span id="tab-title-ledger">长期静默账本</span>
            </button>
          </div>

          <!-- Tab 1: Smart Switches -->
          <div class="tab-pane active" id="tab-smart">
            <div class="card-hdr" style="margin-bottom:2px">
              <div class="card-sub" id="smart-hdr-sub" style="flex:1;min-width:200px">开关改动立即落盘生效；关闭「智能调度」或任意子开关时自动归还被约束进程的 OS 默认优先级</div>
              <label style="font-size:12px;font-weight:600;color:var(--muted);display:flex;align-items:center;gap:6px;cursor:pointer;flex-shrink:0;white-space:nowrap;">
                <input type="checkbox" id="chk-softprio" onchange="toggleOpt('soft_memory_priority', this.checked)" />
                <span id="chk-softprio-lbl">低内存优先级 (Low / VeryLow)</span>
              </label>
            </div>

            <div class="cursor-list" id="smart-switch-list">
              <div class="list-hover-cursor"></div>
              <div class="list-rows">
                <div class="switch-row">
                  <div>
                    <div class="switch-title"><span class="live-dot" id="dot-smart"></span><span id="sw-smart-lbl">智能内存分级调度</span></div>
                    <div class="switch-desc" id="desc-smart">根据可用物理内存比例动态切换 1~3 级调度策略...</div>
                  </div>
                  <label class="toggle"><input type="checkbox" id="sw-smart" onchange="toggleOpt('smart_memory_schedule', this.checked)" /><span class="slider"></span></label>
                </div>

                <div class="switch-row">
                  <div>
                    <div class="switch-title"><span class="live-dot" id="dot-eco"></span><span id="sw-eco-lbl">后台进程 CPU / EcoQoS 能效调度</span></div>
                    <div class="switch-desc" id="desc-eco">为长期闲置后台进程开启 EcoQoS 能效核心与温和内存降档...</div>
                  </div>
                  <label class="toggle"><input type="checkbox" id="sw-eco" onchange="toggleOpt('eco_qos_background', this.checked)" /><span class="slider"></span></label>
                </div>

                <div class="switch-row">
                  <div>
                    <div class="switch-title">
                      <span class="live-dot" id="dot-storm"></span>
                      <span id="sw-storm-lbl">子进程派生风暴压制</span>
                      <span class="pill pill-blue" id="pill-autolearn" style="padding:1px 7px;font-size:11px">自动压制名单中</span>
                    </div>
                    <div class="switch-desc" id="desc-storm">监控并压制高频创建短命子进程的后台宿主...</div>
                  </div>
                  <label class="toggle"><input type="checkbox" id="sw-storm" onchange="toggleOpt('suppress_spawn_storm', this.checked)" /><span class="slider"></span></label>
                </div>

                <div class="switch-row">
                  <div>
                    <div class="switch-title"><span class="live-dot" id="dot-standby"></span><span id="sw-standby-lbl">待机缓存与修改页智能清洗</span></div>
                    <div class="switch-desc" id="desc-standby">高压档按冷静期自动冲刷无用待机缓存...</div>
                  </div>
                  <label class="toggle"><input type="checkbox" id="sw-standby" onchange="toggleOpt('purge_standby', this.checked)" /><span class="slider"></span></label>
                </div>

                <div class="switch-row">
                  <div>
                    <div class="switch-title"><span class="live-dot" id="dot-full"></span><span id="sw-full-lbl">全屏游戏 / 创作自动静音护航</span></div>
                    <div class="switch-desc" id="desc-full">检测到全屏游戏或渲染独占前景时自动暂停所有后台干扰...</div>
                  </div>
                  <label class="toggle"><input type="checkbox" id="sw-full" onchange="toggleOpt('quiet_on_fullscreen', this.checked)" /><span class="slider"></span></label>
                </div>

                <div class="switch-row">
                  <div>
                    <div class="switch-title">
                      <span class="live-dot" id="dot-edge"></span>
                      <span id="sw-edge-lbl">贴边真相光尺 (Edge Horizon)</span>
                      <span class="pill pill-blue" id="pill-edge-side" style="padding:1px 7px;font-size:11px">左侧贴边</span>
                    </div>
                    <div class="switch-desc" id="desc-edge">平时仅保留 9px 瓷白极简贴边水位尺；鼠标悬停平滑展开 CPU/GPU 温度与深度释压圆环，全屏自动隐身（支持全屏自由拖拽与松手自动贴边）。</div>
                  </div>
                  <label class="toggle"><input type="checkbox" id="sw-edge" onchange="toggleOpt('edge_horizon', this.checked)" /><span class="slider"></span></label>
                </div>
              </div>
            </div>

            <div class="baseline-tip" id="baseline-explain">
              <svg class="ico ico-sm" style="margin-top:2px;color:#2563eb" viewBox="0 0 24 24"><path d="M9 18h6"/><path d="M10 22h4"/><path d="M15.09 14c.18-.98.65-1.74 1.41-2.5A4.65 4.65 0 0 0 18 8 6 6 0 0 0 6 8c0 1 .23 2.23 1.5 3.5A4.61 4.61 0 0 1 8.91 14"/></svg>
              <div id="baseline-explain-content"><b>空载基线守恒原理</b>：MEMORA 绝不触碰前台窗口链（浏览器/编辑器/语言服务/全屏应用），仅约束长期闲置的后台膨大工作集与恶意派生宿主，使日常使用贴近“空载基线 + 正常前台应用”的真实物理占用。</div>
            </div>
          </div>

          <!-- Tab 2: Truth Diagnosis & Storm Governance -->
          <div class="tab-pane" id="tab-truth">
            <div class="split-two">
              <div class="pane-scroll" id="diag-cards">
                <div class="alert-box spot-surface spot-sm">
                  <div class="alert-title">
                    <span class="alert-title-l">
                      <svg class="ico ico-sm" viewBox="0 0 24 24"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
                      正在深度采集内核态与驱动池特征...
                    </span>
                  </div>
                  <div class="alert-body">首次采样将分析 NonPaged/Paged Pool 标签与 Virtualization Vid 状态。</div>
                </div>
              </div>

              <div class="pane-scroll">
                <div class="chip-box spot-surface spot-sm">
                  <div class="chip-hdr">
                    <div>
                      <div style="font-weight:700;font-size:13px;display:flex;align-items:center;gap:6px">
                        <svg class="ico ico-sm" style="color:#2563eb" viewBox="0 0 24 24"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>
                        <span id="truth-storm-title-text">自动压制名单与高频派生拦截</span>
                      </div>
                      <div class="card-sub" id="truth-storm-sub-text">开启自动学习后，捕获到反复派生或泄漏宿主会自动记住并在重启后持续压制</div>
                    </div>
                    <div style="display:flex;gap:5px;flex-wrap:wrap">
                      <button class="btn-sm btn-auto-on" id="btn-autolearn" onclick="toggleAutoLearn()">
                        <svg class="ico ico-xs" viewBox="0 0 24 24"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>
                        <span id="btn-autolearn-txt">自动拉入压制名单：已开启</span>
                      </button>
                      <button class="btn-sm btn-danger" onclick="doSuppressStorms()">
                        <svg class="ico ico-xs" viewBox="0 0 24 24"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
                        <span id="btn-suppress-storm-text">立即压制高频派生源</span>
                      </button>
                    </div>
                  </div>
                  <div class="chips" id="storm-chips"></div>
                  <div class="mini-add-row">
                    <input class="mini-input" id="inp-storm-name" placeholder="输入后台进程名加入压制名单，例如 xxx.exe" />
                    <button class="btn-sm" id="btn-add-storm-btn" onclick="addStormNameFromInput()">+ 加入名单</button>
                  </div>
                </div>

                <div class="sub-card spot-surface spot-sm" style="flex:1;min-height:170px;">
                  <div class="sub-card-hdr" style="padding:7px 10px;">
                    <div>
                      <span id="spawner-card-title-text" style="font-weight:700;font-size:13px">高频子进程派生源（近 5 分钟）</span>
                      <span class="card-sub" id="spawner-card-sub-text" style="margin-left:6px">点击「+ 名单」可永久加入自动压制</span>
                    </div>
                    <button class="btn-sm" id="btn-burst-spawners" onclick="doBurstSpawners()">
                      <svg class="ico ico-xs" viewBox="0 0 24 24"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>
                      <span id="btn-burst-spawners-txt">脉冲抓捕短命源</span>
                    </button>
                  </div>
                  <div id="burst-spawners-bar" style="display:none;padding:6px 10px;border-bottom:1px solid #eef2f7;background:#fafcff;font-size:11.5px;line-height:1.5"></div>
                  <div class="cursor-list" id="spawner-table-wrap" style="flex:1;min-height:0;overflow-y:auto;padding:0 6px;">
                    <div class="list-hover-cursor"></div>
                    <table>
                      <thead>
                        <tr id="th-row-spawners"><th>PID</th><th>宿主进程</th><th class="num">派生次数</th><th>状态</th><th class="num">操作</th></tr>
                      </thead>
                      <tbody id="tbl-spawners">
                        <tr><td colspan="5" style="color:var(--muted)">暂未观察到高频派生子进程宿主</td></tr>
                      </tbody>
                    </table>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- Tab 3: Custom-Drawn Process List + Journal -->
          <div class="tab-pane" id="tab-managed">
            <div class="proc-split">
              <div class="sub-card spot-surface">
                <div class="sub-card-hdr">
                  <div>
                    <span id="managed-title-text" style="font-weight:700;font-size:13px">受管后台进程视图</span>
                    <span class="card-sub" id="managed-summary"> · 0 个受管</span>
                  </div>
                  <div style="display:flex;align-items:center;gap:6px">
                    <button class="btn-sm" id="btn-restore-all-btn" onclick="restoreAll()" title="立即归还当前所有受管进程的正常优先级">全部释放</button>
                    <div style="position:relative;display:flex;align-items:center">
                      <svg class="ico ico-xs" style="position:absolute;left:8px;color:#94a3b8;pointer-events:none" viewBox="0 0 24 24"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
                      <input class="mini-input" id="proc-search" style="max-width:170px;padding-left:25px" placeholder="搜索进程名或 PID..." oninput="renderManagedFiltered()" />
                    </div>
                  </div>
                </div>
                <div class="whitelist-bar">
                  <div class="whitelist-bar-top">
                    <span id="whitelist-title-text" style="font-size:12px;font-weight:700;color:#047857">进程白名单（永不收紧/降档/压制）</span>
                    <input class="mini-input" id="inp-wl-name" style="flex:1;min-width:150px" placeholder="输入进程名加入白名单，如 OmniCode.exe" onkeydown="if(event.key==='Enter')addWhitelistFromInput()" />
                    <button class="btn-sm" id="btn-add-wl-btn" onclick="addWhitelistFromInput()">+ 添加白名单</button>
                  </div>
                  <div class="whitelist-chips" id="whitelist-chips"></div>
                </div>
                <div class="proc-toolbar" id="proc-toolbar">
                  <div class="filter-slider-thumb" id="filter-slider-thumb"></div>
                  <button class="filter-pill active" data-filter="all" onclick="setProcFilter('all')"><span id="flt-lbl-all">全部</span></button>
                  <button class="filter-pill" data-filter="storm" onclick="setProcFilter('storm')">
                    <svg class="ico ico-xs" viewBox="0 0 24 24"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>
                    <span id="flt-lbl-storm">风暴压制中</span>
                  </button>
                  <button class="filter-pill" data-filter="verylow" onclick="setProcFilter('verylow')"><span id="flt-lbl-verylow">VeryLow 内存档</span></button>
                  <button class="filter-pill" data-filter="low" onclick="setProcFilter('low')"><span id="flt-lbl-low">Low 内存档</span></button>
                </div>
                <div class="proc-list cursor-list" id="proc-custom-list">
                  <div class="list-hover-cursor"></div>
                  <div class="list-rows" id="proc-custom-rows"></div>
                </div>
              </div>

              <div class="sub-card spot-surface">
                <div class="sub-card-hdr">
                  <div>
                    <div id="journal-title-text" style="font-weight:700;font-size:13px">实时调度与压制动作日志</div>
                    <div class="card-sub" id="journal-sub-text">最近 30 条真实动作（可导出完整 24h JSON）</div>
                  </div>
                  <button class="btn-sm" onclick="doExportReport()">
                    <svg class="ico ico-xs" viewBox="0 0 24 24"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
                    <span id="btn-export-json-text">导出 JSON</span>
                  </button>
                </div>
                <div class="journal-list cursor-list" id="journal-list">
                  <div class="list-hover-cursor"></div>
                  <div class="list-rows" id="journal-rows"></div>
                </div>
              </div>
            </div>
          </div>

          <!-- Tab 4: Quiet Ledger -->
          <div class="tab-pane" id="tab-ledger">
            <div class="ledger-grid">
              <div class="ledger-stat spot-surface spot-sm">
                <div class="k" id="lg-k-hours">累计静默守护</div>
                <div class="v" id="lg-hours">--</div>
                <div class="s" id="lg-s-hours">后台低调运行总时长</div>
              </div>
              <div class="ledger-stat spot-surface spot-sm">
                <div class="k" id="lg-k-restraint">系统克制率</div>
                <div class="v" id="lg-restraint">--</div>
                <div class="s" id="lg-s-restraint">绝不乱杀、按需出手的采样比例</div>
              </div>
              <div class="ledger-stat spot-surface spot-sm">
                <div class="k" id="lg-k-dyn">累计动态调度释放</div>
                <div class="v" id="lg-dyn">--</div>
                <div class="s" id="lg-s-dyn">多级工作集收敛累计效益</div>
              </div>
              <div class="ledger-stat spot-surface spot-sm">
                <div class="k" id="lg-k-peaks">高压救急 / 工作集收紧</div>
                <div class="v" id="lg-peaks">--</div>
                <div class="s" id="lg-s-peaks">在内存告警边缘救回流畅度</div>
              </div>
              <div class="ledger-stat spot-surface spot-sm">
                <div class="k" id="lg-k-full">全屏 / 游戏静音避让</div>
                <div class="v" id="lg-full">--</div>
                <div class="s" id="lg-s-full">前台独占娱乐创作零打扰</div>
              </div>
              <div class="ledger-stat spot-surface spot-sm">
                <div class="k" id="lg-k-orphan">捕获并收敛孤儿链</div>
                <div class="v" id="lg-orphan">--</div>
                <div class="s" id="lg-s-orphan">父进程已退出却残留后台的进程</div>
              </div>
              <div class="ledger-stat spot-surface spot-sm">
                <div class="k" id="lg-k-storm">压制派生风暴宿主</div>
                <div class="v" id="lg-storm">--</div>
                <div class="s" id="lg-s-storm">已阻断子进程狂刷句柄</div>
              </div>
              <div class="ledger-stat spot-surface spot-sm">
                <div class="k" id="lg-k-leak">内核级异常目击次数</div>
                <div class="v" id="lg-leak">--</div>
                <div class="s" id="lg-leak-desc">历史僵尸内核对象追踪</div>
              </div>
            </div>

            <div class="ledger-bottom-split">
              <div class="sub-card spot-surface" style="min-height:135px;">
                <div class="sub-card-hdr" style="padding:7px 11px;">
                  <div>
                    <div id="baseline-card-title-text" style="font-weight:700;font-size:13px">7天本机空载基线守恒对比</div>
                    <div class="card-sub" id="baseline-card-sub-text">验证连续运行后空载真实物理占用是否守恒</div>
                  </div>
                  <span class="pill pill-slate" id="baseline-drift-badge">统计中</span>
                </div>
                <div class="cursor-list" id="baseline-table-wrap" style="flex:1;min-height:0;overflow-y:auto;padding:0 10px;">
                  <div class="list-hover-cursor"></div>
                  <table>
                    <thead>
                      <tr id="th-row-baselines"><th>日期</th><th class="num">最低真实占用</th><th class="num">最低私有提交</th><th class="num">虚机/容器锁页</th><th class="num">内核 Process 峰值</th></tr>
                    </thead>
                    <tbody id="tbl-baselines">
                      <tr><td colspan="5" style="color:var(--muted)">暂无跨天记录</td></tr>
                    </tbody>
                  </table>
                </div>
              </div>

              <div class="sub-card spot-surface" style="min-height:135px;">
                <div class="sub-card-hdr" style="padding:7px 11px;">
                  <div>
                    <div id="culprit-card-title-text" style="font-weight:700;font-size:13px">后台虚高膨胀罪魁榜 Top 5</div>
                    <div class="card-sub" id="culprit-card-sub-text">基于本地调度账本统计虚高膨胀与实测回收归因</div>
                  </div>
                  <span class="pill pill-blue" id="culprit-badge-text">真实归因</span>
                </div>
                <div class="cursor-list" id="culprit-table-wrap" style="flex:1;min-height:0;overflow-y:auto;padding:0 10px;">
                  <div class="list-hover-cursor"></div>
                  <table>
                    <thead>
                      <tr id="th-row-culprits"><th>后台进程</th><th class="num">收敛</th><th class="num">峰值工作集</th><th class="num">累计省下</th><th>策略</th></tr>
                    </thead>
                    <tbody id="tbl-culprits">
                      <tr><td colspan="5" style="color:var(--muted)">暂无后台膨胀收敛归因记录</td></tr>
                    </tbody>
                  </table>
                </div>
              </div>
            </div>
          </div>
        </section>
      </main>
    </div>
  </div>

  <div class="toast" id="toast"></div>
  <div class="export-toast" id="export-toast"></div>

  <script>
    const TOKEN = "__MEMORA_TOKEN__";
    const CIRCUMFERENCE = 2 * Math.PI * 66;
    let latestState = null;
    let procFilter = "all";
    let windowMaximized = false;
    let currentLang = localStorage.getItem("memora_lang") || ((navigator.language && navigator.language.toLowerCase().startsWith("en")) ? "en-US" : "zh-CN");

    const I18N = {
      "zh-CN": {
        topbar_title: "MEMORA · 智能性能调度与内存真相中心",
        admin_ok: "管理员权限就绪",
        admin_std: "标准权限 (提权将触发 UAC)",
        admin_checking: "权限检测中",
        baseline_guarding: "空载基线守护中",
        baseline_managed: "空载基线守护 · 托管 {n} 个",
        baseline_reboot: "待重启释放 {size} 历史内核残留",
        baseline_closed: "调度已关闭 · 仅真实观测",
        export_report: "导出 24h 诊断报告",
        tip_min_title: "收起至任务栏",
        tip_min_sub: "最小化窗口至底部任务栏，保持主界面常驻就绪",
        tip_max_expand: "全屏展开视图",
        tip_max_restore: "还原窗口尺寸",
        tip_max_expand_sub: "最大化展开工作区，获得更充裕的进程与账本视野",
        tip_max_restore_sub: "恢复默认悬浮桌面尺寸",
        tip_lite_title: "轻量模式",
        tip_lite_sub: "收起主窗至托盘与贴边光尺，闲置自动释放界面内存",
        hero_title: "性能调度面板",
        mode_guard_on: "智能调度中",
        mode_guard_off: "已关闭",
        mem_usage_title: "物理内存占用率",
        hero_used_lbl: "已用 / 总量 {tot}",
        avail_phys_mem: "可用物理内存",
        calculating_tier: "计算档位中...",
        detecting_game: "前沿检测中",
        game_quieting: "{label}静音护航中",
        game_ready: "非全屏 · 调度就绪",
        family_protected: "前沿链受保护 {n} 个",
        self_stats: "自身 {ws} · {hd} 句柄",
        btn_opt_text: "立即深度释压",
        btn_opt_hint: "安全收紧闲置后台/服务工作集、文件缓存与低优待机页",
        btn_opt_ext_text: "强力全域释压（含服务与缓存）",
        mode_btn_guard: "智能调度",
        mode_btn_close: "关闭",
        sync_connecting: "正在连接本地真实状态源...",
        sync_note: "最近采样 {time} · 每 2.5 秒实时同步",
        overview_title: "性能概览与物理内存真实拆解",
        overview_sub: "(仅悬停用量彩条查看精细拆解)",
        commit_loading: "提交与页面链表加载中...",
        active_procs: "活跃进程 {n} 个",
        bd_apps_single: "应用独占工作集",
        bd_apps_active: "前台与活跃独占",
        bd_vid_label: "WSL2 / Vid 锁页",
        bd_idle_label: "后台静默驻留",
        bd_zombie_label: "僵尸 / 内核泄漏",
        bd_kernel_label: "常规内核 · 共享与缓存",
        bd_avail_label: "可用内存(含待机)",
        bd_pct_app_single: "% · 程序真实独占",
        bd_pct_app_active: "% · 前沿/系统真实独占",
        bd_pct_vid: "% · 虚机锁定",
        bd_pct_idle: "% · 非前沿后台驻留",
        bd_pct_zombie: "% · 内核对象/PTE",
        bd_pct_kernel: "% · Pool+共享映射",
        bd_pct_avail: "% · 系统随时分配",
        sp_use_title: "物理内存使用率",
        sp_apps_title: "应用独占工作集",
        sp_spawn_title: "子进程派生速率",
        sp_avail_title: "可立即可用内存",
        tab_smart: "智能感知与动态调度",
        tab_truth: "真相归因与派生治理",
        tab_managed: "进程视图与日志",
        tab_ledger: "长期静默账本",
        badge_truth_healthy: "健康",
        badge_truth_alerts: "{n} 项告警",
        smart_hdr_sub: "开关改动立即落盘生效；关闭「智能调度」或任意子开关时自动归还被约束进程的 OS 默认优先级",
        chk_softprio: "低内存优先级 (Low / VeryLow)",
        sw_smart_title: "智能内存分级调度",
        sw_eco_title: "后台进程 CPU / EcoQoS 能效调度",
        sw_storm_title: "子进程派生风暴压制",
        sw_standby_title: "待机缓存与修改页智能清洗",
        sw_full_title: "全屏游戏 / 创作自动静音护航",
        sw_edge_title: "贴边真相光尺 (Edge Horizon)",
        pill_autolearn_on: "自动名单学习中",
        pill_autolearn_off: "手动名单模式",
        pill_edge_hidden: "已隐藏",
        pill_edge_left: "左侧贴边",
        pill_edge_right: "右侧贴边",
        baseline_explain: "<b>空载基线守恒原理</b>：MEMORA 绝不触碰前台窗口链（浏览器/编辑器/语言服务/全屏应用），仅约束长期闲置的后台膨大工作集与恶意派生宿主，使日常使用贴近“空载基线 + 正常前台应用”的真实物理占用。",
        truth_storm_title: "自动压制名单与高频派生拦截",
        truth_storm_sub: "开启自动学习后，捕获到反复派生或泄漏宿主会自动记住并在重启后持续压制",
        btn_autolearn_on: "自动拉入压制名单：已开启",
        btn_autolearn_off: "自动拉入压制名单：已关闭",
        btn_suppress_storms: "立即压制高频派生源",
        inp_storm_ph: "输入后台进程名加入压制名单，例如 xxx.exe",
        btn_add_storm: "+ 加入名单",
        spawner_card_title: "高频子进程派生源（近 5 分钟）",
        spawner_card_sub: "点击「+ 名单」可永久加入自动压制",
        btn_burst_capture: "脉冲抓捕短命源",
        th_pid: "PID",
        th_host_proc: "宿主进程",
        th_spawn_count: "派生次数",
        th_status: "状态",
        th_action: "操作",
        spawner_empty: "暂未观察到高频派生子进程宿主",
        managed_title: "受管后台进程视图",
        managed_summary: " · {n} 个受管",
        btn_restore_all: "全部释放",
        inp_search_ph: "搜索进程名或 PID...",
        whitelist_title: "进程白名单（永不收紧/降档/压制）",
        inp_wl_ph: "输入进程名加入白名单，如 OmniCode.exe",
        btn_add_wl: "+ 添加白名单",
        filter_all: "全部",
        filter_storm: "风暴压制中",
        filter_verylow: "VeryLow 内存档",
        filter_low: "Low 内存档",
        journal_title: "实时调度与压制动作日志",
        journal_sub: "最近 30 条真实动作（可导出完整 24h JSON）",
        btn_export_json: "导出 JSON",
        lg_hours_k: "累计静默守护",
        lg_hours_s: "后台低调运行总时长",
        lg_restraint_k: "系统克制率",
        lg_restraint_s: "绝不乱杀、按需出手的采样比例",
        lg_dyn_k: "累计动态调度释放",
        lg_dyn_s: "多级工作集收敛累计效益",
        lg_peaks_k: "高压救急 / 工作集收紧",
        lg_peaks_s: "在内存告警边缘救回流畅度",
        lg_full_k: "全屏 / 游戏静音避让",
        lg_full_s: "前台独占娱乐创作零打扰",
        lg_orphan_k: "捕获并收敛孤儿链",
        lg_orphan_s: "父进程已退出却残留后台的进程",
        lg_storm_k: "压制派生风暴宿主",
        lg_storm_s: "已阻断子进程狂刷句柄",
        lg_leak_k: "内核级异常目击次数",
        lg_leak_s: "历史僵尸内核对象追踪",
        lg_leak_recent: "最近内核目击: {desc}",
        baseline_card_title: "7天本机空载基线守恒对比",
        baseline_card_sub: "验证连续运行后空载真实物理占用是否守恒",
        baseline_drift_calc: "统计中",
        th_date: "日期",
        th_min_used: "最低真实占用",
        th_min_commit: "最低私有提交",
        th_vm_locked: "虚机/容器锁页",
        th_eprocess_peak: "内核 Process 峰值",
        baseline_empty: "暂无跨天记录",
        culprit_card_title: "后台虚高膨胀罪魁榜 Top 5",
        culprit_card_sub: "基于本地调度账本统计虚高膨胀与实测回收归因",
        culprit_badge: "真实归因",
        th_bg_proc: "后台进程",
        th_trim_hits: "收敛",
        th_peak_ws: "峰值工作集",
        th_saved_tot: "累计省下",
        th_strategy: "策略",
        culprit_empty: "暂无后台膨胀收敛归因记录",
        strat_rebound: "防反扑软抑",
        strat_trim: "分级收紧",
        strat_storm: "风暴压制",
        strat_managed: "能效托管"
      },
      "en-US": {
        topbar_title: "MEMORA · Performance & Memory Center",
        admin_ok: "Admin Ready",
        admin_std: "Standard User",
        admin_checking: "Checking...",
        baseline_guarding: "Baseline Guarding",
        baseline_managed: "Baseline Guard · {n}",
        baseline_reboot: "Reboot pending ({size} residues)",
        baseline_closed: "Dispatch Off · Observing",
        export_report: "Export 24h Report",
        tip_min_title: "Minimize to Taskbar",
        tip_min_sub: "Keep window on taskbar, ready for instant access",
        tip_max_expand: "Full Workspace View",
        tip_max_restore: "Restore Window Size",
        tip_max_expand_sub: "Maximize workspace for broad process and ledger visibility",
        tip_max_restore_sub: "Restore floating desktop dimensions",
        tip_lite_title: "Lite Mode",
        tip_lite_sub: "Fold to tray & Edge Horizon, automatically frees UI RAM when idle",
        hero_title: "Performance Dispatch",
        mode_guard_on: "Smart Dispatching",
        mode_guard_off: "Disabled",
        mem_usage_title: "Physical RAM Usage",
        hero_used_lbl: "Used / Total {tot}",
        avail_phys_mem: "Available Physical RAM",
        calculating_tier: "Calculating tier...",
        detecting_game: "Detecting foreground...",
        game_quieting: "{label} Quiet Escort Active",
        game_ready: "Windowed · Dispatch Ready",
        family_protected: "Foreground Chain Protected: {n}",
        self_stats: "Self {ws} · {hd} handles",
        btn_opt_text: "Deep Pressure Relief Now",
        btn_opt_hint: "Safely trims idle background/service working sets, file cache & standby pages",
        btn_opt_ext_text: "Global Relief (Services & Cache)",
        mode_btn_guard: "Smart Dispatch",
        mode_btn_close: "Off",
        sync_connecting: "Connecting to local kernel telemetry...",
        sync_note: "Latest sample {time} · Sync every 2.5s",
        overview_title: "Telemetry Overview & Physical RAM Breakdown",
        overview_sub: "(Hover over the colored bar for detailed analysis)",
        commit_loading: "Loading commit and page lists...",
        active_procs: "Active Processes: {n}",
        bd_apps_single: "App Exclusive WS",
        bd_apps_active: "Foreground & Active",
        bd_vid_label: "WSL2 / Vid Locked",
        bd_idle_label: "Background Idle",
        bd_zombie_label: "Zombie / Kernel Leak",
        bd_kernel_label: "Kernel / Shared / Cache",
        bd_avail_label: "Available (w/ Standby)",
        bd_pct_app_single: "% · Private exclusive",
        bd_pct_app_active: "% · Foreground/System",
        bd_pct_vid: "% · Hyper-V locked",
        bd_pct_idle: "% · Idle non-foreground",
        bd_pct_zombie: "% · Kernel objects/PTE",
        bd_pct_kernel: "% · Pool + shared maps",
        bd_pct_avail: "% · Ready to allocate",
        sp_use_title: "RAM Usage",
        sp_apps_title: "App Exclusive WS",
        sp_spawn_title: "Spawn Storm Rate",
        sp_avail_title: "Instant Avail RAM",
        tab_smart: "Smart Dispatch",
        tab_truth: "Diagnostics & Storms",
        tab_managed: "Processes & Logs",
        tab_ledger: "Quiet Ledger",
        badge_truth_healthy: "Healthy",
        badge_truth_alerts: "{n} Alerts",
        smart_hdr_sub: "Changes take effect immediately; turning off Smart Dispatch restores processes to OS default priority",
        chk_softprio: "Low RAM Priority (Low / VeryLow)",
        sw_smart_title: "Tiered Dynamic Scheduling",
        sw_eco_title: "Background CPU / EcoQoS Efficiency",
        sw_storm_title: "Spawn Storm Suppression",
        sw_standby_title: "Standby Cache & Modified Page Flush",
        sw_full_title: "Fullscreen Game / Work Quiet Escort",
        sw_edge_title: "Edge Horizon (Docked Light Ruler)",
        pill_autolearn_on: "Auto-Learning Storm List",
        pill_autolearn_off: "Manual List Mode",
        pill_edge_hidden: "Hidden",
        pill_edge_left: "Left Docked",
        pill_edge_right: "Right Docked",
        baseline_explain: "<b>Idle Baseline Conservation</b>: MEMORA strictly avoids foreground chains (browsers, IDEs, language servers, fullscreen apps), managing only long-idle bloat and storm spawners to keep daily usage anchored to true baseline levels.",
        truth_storm_title: "Auto-Suppression List & Storm Interception",
        truth_storm_sub: "With auto-learning enabled, repeatedly spawning or leaking hosts are permanently suppressed across restarts",
        btn_autolearn_on: "Auto-Learn List: Enabled",
        btn_autolearn_off: "Auto-Learn List: Disabled",
        btn_suppress_storms: "Suppress High-Freq Spawners Now",
        inp_storm_ph: "Enter process name to suppress, e.g., xxx.exe",
        btn_add_storm: "+ Add to List",
        spawner_card_title: "High-Frequency Spawners (Last 5 mins)",
        spawner_card_sub: "Click '+ List' to permanently add to suppression list",
        btn_burst_capture: "Burst Capture (2.5s)",
        th_pid: "PID",
        th_host_proc: "Host Process",
        th_spawn_count: "Spawns",
        th_status: "Status",
        th_action: "Action",
        spawner_empty: "No high-frequency child spawners observed",
        managed_title: "Managed Processes",
        managed_summary: " · {n} managed",
        btn_restore_all: "Release All",
        inp_search_ph: "Search process or PID...",
        whitelist_title: "Whitelist (Protected)",
        inp_wl_ph: "Add process to whitelist, e.g. OmniCode.exe",
        btn_add_wl: "+ Whitelist",
        filter_all: "All",
        filter_storm: "Storm Suppressed",
        filter_verylow: "VeryLow Tier",
        filter_low: "Low Tier",
        journal_title: "Dispatch & Storm Journal",
        journal_sub: "Recent 30 actions (24h JSON export)",
        btn_export_json: "Export JSON",
        lg_hours_k: "Quiet Stewardship",
        lg_hours_s: "Total silent runtime in background",
        lg_restraint_k: "System Restraint Rate",
        lg_restraint_s: "Sampling ratio with zero intrusion or kill",
        lg_dyn_k: "Dynamic Scheduled Reclaim",
        lg_dyn_s: "Cumulative multi-tier working set savings",
        lg_peaks_k: "Pressure Saves / Trim Actions",
        lg_peaks_s: "Safely relieved under tight memory thresholds",
        lg_full_k: "Fullscreen Quiet Skips",
        lg_full_s: "Zero-interference during games & creation",
        lg_orphan_k: "Orphan Chains Converged",
        lg_orphan_s: "Remnants whose parent processes exited",
        lg_storm_k: "Storm Spawners Suppressed",
        lg_storm_s: "Blocked recursive handle-flooding hosts",
        lg_leak_k: "Kernel Anomalies Witnessed",
        lg_leak_s: "Historical zombie kernel object observations",
        lg_leak_recent: "Recent kernel sighting: {desc}",
        baseline_card_title: "7-Day Idle Baseline Conservation",
        baseline_card_sub: "Verifying whether idle physical footprint remains conserved over time",
        baseline_drift_calc: "Calculating",
        th_date: "Date",
        th_min_used: "Min Used",
        th_min_commit: "Min Commit",
        th_vm_locked: "VM Locked",
        th_eprocess_peak: "Proc Peak",
        baseline_empty: "No multi-day records available yet",
        culprit_card_title: "Top 5 Background Bloat Culprits",
        culprit_card_sub: "Real attribution based on local dispatch ledger and verified savings",
        culprit_badge: "Real Attribution",
        th_bg_proc: "Process",
        th_trim_hits: "Hits",
        th_peak_ws: "Peak WS",
        th_saved_tot: "Total Saved",
        th_strategy: "Strategy",
        culprit_empty: "No bloat attribution records yet",
        strat_rebound: "Soft Anti-Rebound",
        strat_trim: "Tiered Trim",
        strat_storm: "Storm Throttled",
        strat_managed: "EcoQoS Managed"
      }
    };

    function t(key, params) {
      const dict = I18N[currentLang] || I18N["zh-CN"];
      let str = dict[key] != null ? dict[key] : (I18N["zh-CN"][key] != null ? I18N["zh-CN"][key] : key);
      if (params) {
        for (const k in params) {
          str = str.replace(new RegExp("{" + k + "}", "g"), params[k]);
        }
      }
      return str;
    }

    function translateSmartLabel(lbl) {
      if (!lbl) return t("calculating_tier");
      if (currentLang === "en-US") {
        if (lbl.indexOf("四级") >= 0 || lbl.indexOf("紧急") >= 0) return "Tier 4 · Critical Rescue";
        if (lbl.indexOf("三级") >= 0 || lbl.indexOf("高压") >= 0) return "Tier 3 · High Pressure Relief";
        if (lbl.indexOf("二级") >= 0 || lbl.indexOf("平衡") >= 0) return "Tier 2 · Balanced Schedule";
        if (lbl.indexOf("一级") >= 0 || lbl.indexOf("巡航") >= 0) return "Tier 1 · Light Cruise";
      }
      return lbl;
    }

    function translateFullscreenLabel(lbl) {
      if (!lbl) return "Fullscreen";
      if (currentLang === "en-US") {
        if (lbl.indexOf("全屏") >= 0) return "Fullscreen";
        if (lbl.indexOf("游戏") >= 0) return "Game";
      }
      return lbl;
    }

    function translateStrategy(st) {
      if (!st) return "";
      if (currentLang === "en-US") {
        if (st.indexOf("防反扑") >= 0) return t("strat_rebound");
        if (st.indexOf("分级") >= 0) return t("strat_trim");
        if (st.indexOf("风暴") >= 0) return t("strat_storm");
        if (st.indexOf("托管") >= 0) return t("strat_managed");
      }
      return st;
    }

    function applyStaticI18N() {
      document.documentElement.lang = currentLang;
      const setTxt = (id, val) => { const el = document.getElementById(id); if (el) el.textContent = val; };
      const setHTML = (id, val) => { const el = document.getElementById(id); if (el) el.innerHTML = val; };

      setTxt("lang-label", currentLang === "zh-CN" ? "EN" : "中文");
      setTxt("topbar-title", t("topbar_title"));
      setTxt("topbar-export-text", t("export_report"));
      setTxt("hero-title-text", t("hero_title"));
      setTxt("gauge-sub-text", t("mem_usage_title"));
      setTxt("hero-avail-lbl", t("avail_phys_mem"));
      setTxt("btn-opt-text", t("btn_opt_text"));
      setTxt("btn-primary-hint-text", t("btn_opt_hint"));
      setTxt("btn-opt-ext-text", t("btn_opt_ext_text"));
      setTxt("mode-guard", t("mode_btn_guard"));
      setTxt("mode-observe", t("mode_btn_close"));
      setTxt("overview-title-text", t("overview_title"));
      setTxt("overview-sub-text", t("overview_sub"));
      setTxt("bd-zombie-label", t("bd_zombie_label"));
      setTxt("bd-kernel-label", t("bd_kernel_label"));
      setTxt("bd-avail-label", t("bd_avail_label"));

      setTxt("sp-use-lbl", t("sp_use_title"));
      setTxt("sp-apps-lbl", t("sp_apps_title"));
      setTxt("sp-spawn-lbl", t("sp_spawn_title"));
      setTxt("sp-avail-lbl", t("sp_avail_title"));

      setTxt("tab-title-smart", t("tab_smart"));
      setTxt("tab-title-truth", t("tab_truth"));
      setTxt("tab-title-managed", t("tab_managed"));
      setTxt("tab-title-ledger", t("tab_ledger"));

      setTxt("smart-hdr-sub", t("smart_hdr_sub"));
      setTxt("chk-softprio-lbl", t("chk_softprio"));
      setTxt("sw-smart-lbl", t("sw_smart_title"));
      setTxt("sw-eco-lbl", t("sw_eco_title"));
      setTxt("sw-storm-lbl", t("sw_storm_title"));
      setTxt("sw-standby-lbl", t("sw_standby_title"));
      setTxt("sw-full-lbl", t("sw_full_title"));
      setTxt("sw-edge-lbl", t("sw_edge_title"));
      setHTML("baseline-explain-content", t("baseline_explain"));

      setTxt("truth-storm-title-text", t("truth_storm_title"));
      setTxt("truth-storm-sub-text", t("truth_storm_sub"));
      setTxt("btn-suppress-storm-text", t("btn_suppress_storms"));
      const inpStorm = document.getElementById("inp-storm-name");
      if (inpStorm) inpStorm.placeholder = t("inp_storm_ph");
      setTxt("btn-add-storm-btn", t("btn_add_storm"));
      setTxt("spawner-card-title-text", t("spawner_card_title"));
      setTxt("spawner-card-sub-text", t("spawner_card_sub"));
      setTxt("btn-burst-spawners-txt", t("btn_burst_capture"));

      const thSpawners = document.getElementById("th-row-spawners");
      if (thSpawners) {
        thSpawners.innerHTML = "<th>" + t("th_pid") + "</th><th>" + t("th_host_proc") + "</th><th class=\"num\">" + t("th_spawn_count") + "</th><th>" + t("th_status") + "</th><th class=\"num\">" + t("th_action") + "</th>";
      }

      setTxt("managed-title-text", t("managed_title"));
      setTxt("btn-restore-all-btn", t("btn_restore_all"));
      const procSearch = document.getElementById("proc-search");
      if (procSearch) procSearch.placeholder = t("inp_search_ph");
      setTxt("whitelist-title-text", t("whitelist_title"));
      const inpWl = document.getElementById("inp-wl-name");
      if (inpWl) inpWl.placeholder = t("inp_wl_ph");
      setTxt("btn-add-wl-btn", t("btn_add_wl"));

      setTxt("flt-lbl-all", t("filter_all"));
      setTxt("flt-lbl-storm", t("filter_storm"));
      setTxt("flt-lbl-verylow", t("filter_verylow"));
      setTxt("flt-lbl-low", t("filter_low"));

      setTxt("journal-title-text", t("journal_title"));
      setTxt("journal-sub-text", t("journal_sub"));
      setTxt("btn-export-json-text", t("btn_export_json"));

      setTxt("lg-k-hours", t("lg_hours_k"));
      setTxt("lg-s-hours", t("lg_hours_s"));
      setTxt("lg-k-restraint", t("lg_restraint_k"));
      setTxt("lg-s-restraint", t("lg_restraint_s"));
      setTxt("lg-k-dyn", t("lg_dyn_k"));
      setTxt("lg-s-dyn", t("lg_dyn_s"));
      setTxt("lg-k-peaks", t("lg_peaks_k"));
      setTxt("lg-s-peaks", t("lg_peaks_s"));
      setTxt("lg-k-full", t("lg_full_k"));
      setTxt("lg-s-full", t("lg_full_s"));
      setTxt("lg-k-orphan", t("lg_orphan_k"));
      setTxt("lg-s-orphan", t("lg_orphan_s"));
      setTxt("lg-k-storm", t("lg_storm_k"));
      setTxt("lg-s-storm", t("lg_storm_s"));
      setTxt("lg-k-leak", t("lg_leak_k"));
      setTxt("lg-leak-desc", t("lg_leak_s"));

      setTxt("baseline-card-title-text", t("baseline_card_title"));
      setTxt("baseline-card-sub-text", t("baseline_card_sub"));
      const thBaselines = document.getElementById("th-row-baselines");
      if (thBaselines) {
        thBaselines.innerHTML = "<th>" + t("th_date") + "</th><th class=\"num\">" + t("th_min_used") + "</th><th class=\"num\">" + t("th_min_commit") + "</th><th class=\"num\">" + t("th_vm_locked") + "</th><th class=\"num\">" + t("th_eprocess_peak") + "</th>";
      }

      setTxt("culprit-card-title-text", t("culprit_card_title"));
      setTxt("culprit-card-sub-text", t("culprit_card_sub"));
      setTxt("culprit-badge-text", t("culprit_badge"));
      const thCulprits = document.getElementById("th-row-culprits");
      if (thCulprits) {
        thCulprits.innerHTML = "<th>" + t("th_bg_proc") + "</th><th class=\"num\">" + t("th_trim_hits") + "</th><th class=\"num\">" + t("th_peak_ws") + "</th><th class=\"num\">" + t("th_saved_tot") + "</th><th>" + t("th_strategy") + "</th>";
      }
    }

    function toggleLanguage() {
      currentLang = (currentLang === "zh-CN" ? "en-US" : "zh-CN");
      localStorage.setItem("memora_lang", currentLang);
      applyStaticI18N();
      api("/api/toggle", "POST", { key: "language", value: currentLang }).catch(() => {});
      if (latestState) {
        renderState(latestState);
      }
      requestAnimationFrame(() => {
        syncTabSlider();
        syncFilterSlider();
      });
      showToast(currentLang === "en-US" ? "Switched to English" : "已切换为简体中文", 2000);
    }

    const SVG = {
      zap: '<svg class="ico ico-xs" viewBox="0 0 24 24"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>',
      alert: '<svg class="ico ico-sm" style="color:#dc2626" viewBox="0 0 24 24"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>',
      wrench: '<svg class="ico ico-xs" viewBox="0 0 24 24"><path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/></svg>',
      search: '<svg class="ico ico-xs" viewBox="0 0 24 24"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>',
      box: '<svg class="ico ico-sm" style="color:#7c3aed" viewBox="0 0 24 24"><path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/><polyline points="3.27 6.96 12 12.01 20.73 6.96"/><line x1="12" y1="22.08" x2="12" y2="12"/></svg>',
      scale: '<svg class="ico ico-sm" style="color:#d97706" viewBox="0 0 24 24"><line x1="12" y1="3" x2="12" y2="21"/><path d="M3 7h18"/><path d="M3 12l3-5 3 5a3 3 0 0 1-6 0z"/><path d="M15 12l3-5 3 5a3 3 0 0 1-6 0z"/><path d="M8 21h8"/></svg>',
      check: '<svg class="ico ico-sm" style="color:#059669" viewBox="0 0 24 24"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>',
      winMax: '<svg viewBox="0 0 14 14"><rect x="2.7" y="2.7" width="8.6" height="8.6" rx="1.6" stroke-width="1.55"/></svg>',
      winRestore: '<svg viewBox="0 0 14 14"><path d="M4.6 3.5V2.9a1.3 1.3 0 0 1 1.3-1.3h5.2a1.3 1.3 0 0 1 1.3 1.3v5.2a1.3 1.3 0 0 1-1.3 1.3h-.6" stroke-width="1.45"/><rect x="2.1" y="4.5" width="7.4" height="7.4" rx="1.4" stroke-width="1.55"/></svg>'
    };

    function api(url, method, body) {
      const opts = {
        method: method || "GET",
        headers: { "X-Memora-Token": TOKEN }
      };
      if (body !== undefined) {
        opts.headers["Content-Type"] = "application/json";
        opts.body = JSON.stringify(body);
      }
      return fetch(url, opts).then(async r => {
        if (!r.ok) throw new Error(await r.text() || ("HTTP " + r.status));
        return r.json();
      });
    }

    /* Frameless Window Drag / Resize / Button Controls */
    function updateMaxVisual(isMax) {
      windowMaximized = !!isMax;
      const frame = document.getElementById("app-frame");
      const ico = document.getElementById("btn-win-max-ico");
      const tipTitle = document.getElementById("tip-max-title");
      const tipSub = document.getElementById("tip-max-sub");
      if (frame) frame.classList.toggle("is-maximized", windowMaximized);
      if (ico) ico.innerHTML = windowMaximized ? SVG.winRestore : SVG.winMax;
      if (tipTitle) tipTitle.textContent = windowMaximized ? t("tip_max_restore") : t("tip_max_expand");
      if (tipSub) tipSub.textContent = windowMaximized ? t("tip_max_restore_sub") : t("tip_max_expand_sub");
    }

    function winCtl(action) {
      return api("/api/window-ctl", "POST", { action: action })
        .then(res => {
          if (res && typeof res.maximized === "boolean") {
            updateMaxVisual(res.maximized);
          }
          return res;
        })
        .catch(() => {});
    }

    (function initFramelessChrome() {
      const bar = document.getElementById("titlebar");
      if (bar) {
        bar.addEventListener("mousedown", e => {
          if (e.button !== 0) return;
          if (e.target.closest("button, a, input, select, .win-controls, .topbar-btn")) return;
          e.preventDefault();
          winCtl("drag");
        });
        bar.addEventListener("dblclick", e => {
          if (e.target.closest("button, a, input, select, .win-controls, .topbar-btn")) return;
          winCtl("maximize");
        });
      }
      document.querySelectorAll(".win-resizer").forEach(el => {
        el.addEventListener("mousedown", e => {
          if (e.button !== 0) return;
          e.preventDefault();
          e.stopPropagation();
          winCtl(el.dataset.resize);
        });
      });
      window.addEventListener("resize", () => {
        syncTabSlider();
        syncFilterSlider();
        clearTimeout(window._winStateSyncTimer);
        window._winStateSyncTimer = setTimeout(() => winCtl("state"), 120);
      });
    })();

    function fmtBytes(b) {
      if (!b || b <= 0) return "0 MB";
      const gb = b / (1024 * 1024 * 1024);
      if (gb >= 1) return gb.toFixed(1) + " GB";
      return Math.round(b / (1024 * 1024)) + " MB";
    }
    function fmtMB(mb) {
      if (!mb || mb <= 0) return "0 MB";
      if (mb >= 1024) return (mb / 1024).toFixed(2) + " GB";
      return Math.round(mb) + " MB";
    }
    function esc(s) {
      return String(s == null ? "" : s).replace(/[&<>"']/g, c => ({
        "&": "&amp;", "<": "&lt;", ">": "&gt;", "\"": "&quot;", "'": "&#39;"
      }[c]));
    }

    /* Scale-Up + Count-Up / Count-Down Dynamic Numeric Tween Engine */
    const prevDisplayMap = {};
    const pendingTweens = [];
    const activeTweenRAFs = {};
    const activeSettleTimers = {};

    function parseNumTokens(str) {
      const re = /-?\d+(?:\.\d+)?/g;
      const matches = [];
      let m;
      while ((m = re.exec(str)) !== null) {
        const raw = m[0];
        const dot = raw.indexOf(".");
        matches.push({
          val: parseFloat(raw),
          decimals: dot >= 0 ? (raw.length - dot - 1) : 0,
          start: m.index,
          end: m.index + raw.length
        });
      }
      return matches;
    }

    function normalizeUnitValue(val, str, targetStr) {
      const hasMBFrom = /\bMB\b/i.test(str);
      const hasGBFrom = /\bGB\b/i.test(str);
      const hasMBTo = /\bMB\b/i.test(targetStr);
      const hasGBTo = /\bGB\b/i.test(targetStr);
      if (hasMBFrom && hasGBTo) return val / 1024;
      if (hasGBFrom && hasMBTo) return val * 1024;
      return val;
    }

    function startElementTween(el, key, fromStr, toStr) {
      if (!el) return;
      if (activeTweenRAFs[key]) cancelAnimationFrame(activeTweenRAFs[key]);
      if (activeSettleTimers[key]) clearTimeout(activeSettleTimers[key]);

      const toToks = parseNumTokens(toStr);
      const fromToks = parseNumTokens(fromStr);

      // Simultaneous start: scale-up and dynamic number interpolation begin on the exact same frame.
      el.textContent = fromStr;
      el.classList.remove("is-counting");
      void el.offsetWidth;
      el.classList.add("is-counting");

      const rollDurationMs = 1100;
      const settleTransitionMs = 260;

      if (toToks.length === 0 || fromToks.length !== toToks.length) {
        el.textContent = toStr;
        activeSettleTimers[key] = setTimeout(() => {
          delete activeSettleTimers[key];
          el.classList.remove("is-counting");
        }, Math.max(0, rollDurationMs - settleTransitionMs));
        return;
      }

      const pairs = toToks.map((tt, idx) => ({
        from: normalizeUnitValue(fromToks[idx].val, fromStr, toStr),
        to: tt.val,
        decimals: tt.decimals,
        start: tt.start,
        end: tt.end
      }));

      const anyNumericDelta = pairs.some(p => Math.abs(p.from - p.to) > 1e-6);
      if (!anyNumericDelta) {
        el.textContent = toStr;
        activeSettleTimers[key] = setTimeout(() => {
          delete activeSettleTimers[key];
          el.classList.remove("is-counting");
        }, 420);
        return;
      }

      let resetStarted = false;
      const t0 = performance.now();
      const step = now => {
        const elapsed = now - t0;
        const p = Math.min(1, elapsed / rollDurationMs);
        const ease = 1 - Math.pow(1 - p, 3);
        // Simultaneous end: trigger the 260ms scale-down transition so it finishes right as numbers reach p = 1.
        if (!resetStarted && elapsed >= rollDurationMs - settleTransitionMs) {
          resetStarted = true;
          el.classList.remove("is-counting");
        }
        let out = "";
        let cursor = 0;
        for (let i = 0; i < pairs.length; i++) {
          const pr = pairs[i];
          out += toStr.slice(cursor, pr.start);
          const cur = pr.from + (pr.to - pr.from) * ease;
          out += pr.decimals > 0 ? cur.toFixed(pr.decimals) : String(Math.round(cur));
          cursor = pr.end;
        }
        out += toStr.slice(cursor);
        el.textContent = out;
        if (p < 1) {
          activeTweenRAFs[key] = requestAnimationFrame(step);
        } else {
          delete activeTweenRAFs[key];
          el.textContent = toStr;
          el.classList.remove("is-counting");
        }
      };
      activeTweenRAFs[key] = requestAnimationFrame(step);
    }

    const prevHTMLSigMap = {};
    function setTweenHTML(elId, html) {
      const el = document.getElementById(elId);
      if (!el) return;
      if (prevHTMLSigMap[elId] === html && el.innerHTML !== "") {
        return;
      }
      const stripVals = s => s.replace(/(<span class="num-hl[^"]*" data-tween-key="[^"]+">)[^<]*(<\/span>)/g, "$1*$2");
      const newStruct = stripVals(html);
      const oldStruct = el.dataset.structSig || "";
      if (oldStruct === newStruct && el.innerHTML !== "") {
        prevHTMLSigMap[elId] = html;
        return;
      }
      prevHTMLSigMap[elId] = html;
      el.dataset.structSig = newStruct;
      el.innerHTML = html;
    }

    function hlNum(val, cls, key) {
      const nextStr = String(val);
      if (key) {
        const prevStr = prevDisplayMap[key];
        if (prevStr !== undefined && prevStr !== nextStr) {
          pendingTweens.push({ type: "key", key: key, from: prevStr, to: nextStr });
          prevDisplayMap[key] = nextStr;
          return '<span class="num-hl ' + cls + '" data-tween-key="' + esc(key) + '">' + esc(prevStr) + '</span>';
        }
        prevDisplayMap[key] = nextStr;
        return '<span class="num-hl ' + cls + '" data-tween-key="' + esc(key) + '">' + esc(nextStr) + '</span>';
      }
      return '<span class="num-hl ' + cls + '">' + esc(nextStr) + '</span>';
    }

    function setTweenText(elId, val, key) {
      const el = document.getElementById(elId);
      if (!el) return;
      const nextStr = String(val);
      const prevStr = prevDisplayMap[key];
      if (prevStr === undefined) {
        prevDisplayMap[key] = nextStr;
        el.textContent = nextStr;
        return;
      }
      if (prevStr !== nextStr) {
        prevDisplayMap[key] = nextStr;
        startElementTween(el, key, prevStr, nextStr);
      }
    }

    function flushNumericTweens() {
      while (pendingTweens.length > 0) {
        const item = pendingTweens.shift();
        const el = document.querySelector('[data-tween-key="' + item.key + '"]');
        if (el) {
          startElementTween(el, item.key, item.from, item.to);
        }
      }
    }

    let toastTimer = null;
    function showToast(msg, ms = 4200) {
      const el = document.getElementById("toast");
      el.textContent = msg;
      el.classList.add("show");
      clearTimeout(toastTimer);
      toastTimer = setTimeout(() => el.classList.remove("show"), ms);
    }

    /* Elastic Sliding Mode Pill */
    function syncModeSwitchUI(isGuard) {
      const sw = document.getElementById("mode-switch");
      const btnG = document.getElementById("mode-guard");
      const btnO = document.getElementById("mode-observe");
      if (sw) sw.classList.toggle("is-off", !isGuard);
      if (btnG) btnG.classList.toggle("active", !!isGuard);
      if (btnO) btnO.classList.toggle("active", !isGuard);
    }

    /* Elastic Sliding Workspace Tab & Filter Pills */
    function syncTabSlider() {
      const bar = document.getElementById("main-tabs");
      const thumb = document.getElementById("tab-slider-thumb");
      const active = bar && bar.querySelector(".tab-btn.active");
      if (!bar || !thumb || !active) return;
      thumb.style.transform = "translate3d(" + active.offsetLeft + "px, " + active.offsetTop + "px, 0)";
      thumb.style.width = active.offsetWidth + "px";
      thumb.style.height = active.offsetHeight + "px";
      thumb.style.opacity = "1";
    }

    function syncFilterSlider() {
      const bar = document.getElementById("proc-toolbar");
      const thumb = document.getElementById("filter-slider-thumb");
      const active = bar && bar.querySelector(".filter-pill.active");
      if (!bar || !thumb || !active) return;
      thumb.style.transform = "translate3d(" + active.offsetLeft + "px, " + active.offsetTop + "px, 0)";
      thumb.style.width = active.offsetWidth + "px";
      thumb.style.height = active.offsetHeight + "px";
    }

    function switchTab(name) {
      document.querySelectorAll(".tab-btn").forEach(btn => {
        btn.classList.toggle("active", btn.dataset.tab === name);
      });
      document.querySelectorAll(".tab-pane").forEach(pane => {
        pane.classList.toggle("active", pane.id === "tab-" + name);
      });
      requestAnimationFrame(() => {
        syncTabSlider();
        if (name === "managed") syncFilterSlider();
      });
    }

    /* Restrained Card Spotlight Effect */
    (function initCardSpotlights() {
      document.addEventListener("mousemove", e => {
        const cards = document.querySelectorAll(".spot-surface");
        for (let i = 0; i < cards.length; i++) {
          const c = cards[i];
          const r = c.getBoundingClientRect();
          if (e.clientX >= r.left - 80 && e.clientX <= r.right + 80 && e.clientY >= r.top - 80 && e.clientY <= r.bottom + 80) {
            c.style.setProperty("--mx", (e.clientX - r.left).toFixed(1) + "px");
            c.style.setProperty("--my", (e.clientY - r.top).toFixed(1) + "px");
          }
        }
      }, { passive: true });
    })();

    /* Smooth Floating List Highlight Cursor Effect */
    function bindSmoothListCursor(containerId, rowSelector) {
      const box = document.getElementById(containerId);
      if (!box) return;
      const cursor = box.querySelector(".list-hover-cursor");
      if (!cursor) return;

      let activeRow = null;
      const positionCursor = row => {
        if (!row || !box.contains(row)) {
          cursor.style.opacity = "0";
          return;
        }
        const cRect = box.getBoundingClientRect();
        const rRect = row.getBoundingClientRect();
        const top = (rRect.top - cRect.top) + box.scrollTop;
        const left = (rRect.left - cRect.left) + box.scrollLeft;
        cursor.style.transform = "translate3d(" + left.toFixed(1) + "px, " + top.toFixed(1) + "px, 0)";
        cursor.style.width = rRect.width.toFixed(1) + "px";
        cursor.style.height = rRect.height.toFixed(1) + "px";
        cursor.style.opacity = "1";
      };

      box.addEventListener("mousemove", e => {
        const row = e.target.closest(rowSelector);
        if (!row || !box.contains(row)) {
          activeRow = null;
          cursor.style.opacity = "0";
          return;
        }
        activeRow = row;
        positionCursor(row);
      }, { passive: true });

      box.addEventListener("mouseleave", () => {
        activeRow = null;
        cursor.style.opacity = "0";
      });

      box.addEventListener("scroll", () => {
        if (activeRow) positionCursor(activeRow);
      }, { passive: true });
    }

    (function initSmoothCursors() {
      bindSmoothListCursor("smart-switch-list", ".switch-row");
      bindSmoothListCursor("proc-custom-list", ".proc-row");
      bindSmoothListCursor("journal-list", ".j-item");
      bindSmoothListCursor("spawner-table-wrap", "tbody tr.hover-row");
      bindSmoothListCursor("baseline-table-wrap", "tbody tr.hover-row");
      bindSmoothListCursor("culprit-table-wrap", "tbody tr.hover-row");
      requestAnimationFrame(() => {
        syncTabSlider();
        syncFilterSlider();
      });
    })();

    function renderSparkline(svgId, vals, stroke, fill, minZero = true) {
      const svg = document.getElementById(svgId);
      if (!vals || vals.length === 0) {
        svg.innerHTML = "";
        return;
      }
      if (vals.length === 1) vals = [vals[0], vals[0]];
      const w = 180, h = 30, pad = 3;
      let lo = minZero ? 0 : Math.min(...vals);
      let hi = Math.max(...vals);
      if (hi - lo < 0.001) hi = lo + 1;
      const pts = vals.map((v, i) => {
        const x = (i / (vals.length - 1)) * w;
        const y = h - pad - ((v - lo) / (hi - lo)) * (h - pad * 2);
        return [x.toFixed(1), y.toFixed(1)];
      });
      const line = pts.map((p, i) => (i === 0 ? "M" : "L") + p[0] + "," + p[1]).join(" ");
      const area = line + " L" + w + "," + h + " L0," + h + " Z";
      svg.innerHTML = '<path d="' + area + '" fill="' + fill + '"></path><path d="' + line + '" fill="none" stroke="' + stroke + '" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"></path>';
    }

    /* Breakdown Bar Hover Pop-Out + Detailed Tooltip (Triggered ONLY by Bar Segments) */
    function buildSegDetail(segKey, s) {
      const a = (s && s.assessment) || {};
      const bd = (s && s.breakdown) || {};
      const deep = (s && s.deep) || {};
      const total = bd.total_bytes || a.total_phys_bytes || 1;
      const pct = v => (((v || 0) * 100) / total).toFixed(1) + "%";
      const isVidMode = bd.slot2_mode === "vid";
      const isEn = currentLang === "en-US";

      if (segKey === "apps") {
        const valBytes = isVidMode ? bd.app_private_ws_bytes : (bd.active_app_ws_bytes || bd.app_private_ws_bytes);
        const topG = (deep.proc_groups || []).slice(0, 3)
          .map(g => esc(g.name) + " (" + fmtBytes(g.private_ws_bytes) + ")")
          .join("、");
        return {
          color: "#3b82f6",
          title: isEn
            ? (isVidMode ? "App Private Physical Working Set" : "Foreground & Active Process Exclusive")
            : (isVidMode ? "应用独占物理工作集" : "前台与活跃进程真实独占"),
          val: fmtBytes(valBytes) + " (" + pct(valBytes) + ")",
          body: isEn
            ? ((isVidMode
                ? "Actual non-shareable private physical RAM; total working set including shared DLLs is <b>" + fmtBytes(a.ws_bytes) + "</b>."
                : "Current private physical RAM consumed by foreground focus and system core (all apps private total: <b>" + fmtBytes(bd.app_private_ws_bytes) + "</b>).") +
              (topG ? ("<br/>Top applications: " + topG) : ""))
            : ((isVidMode
                ? "各进程无法共享的真实私有物理内存；含系统共享 DLL 后的总工作集为 <b>" + fmtBytes(a.ws_bytes) + "</b>。"
                : "当前前台焦点链与系统活跃核心的真实私有物理占用（全体应用私有独占共 <b>" + fmtBytes(bd.app_private_ws_bytes) + "</b>）。") +
              (topG ? ("<br/>占比较高应用：" + topG) : ""))
        };
      }
      if (segKey === "vid") {
        if (isVidMode) {
          return {
            color: "#8b5cf6",
            title: isEn ? "Hyper-V / WSL2 VM Locked Pages (Vid)" : "Hyper-V / WSL2 虚拟机锁页 (Vid)",
            val: fmtBytes(bd.vid_bytes) + " (" + pct(bd.vid_bytes) + ")",
            body: isEn
              ? "Pages rigidly locked in physical RAM by virtualization hypervisors like <b>vmmemWSL</b>; if containers are idle, execute <code>wsl --shutdown</code> in the Root Attribution tab to reclaim."
              : "由 <b>vmmemWSL</b> 等虚拟机管理程序在物理内存中硬性锁定的页面；如未使用容器可在「真相归因」页一键执行 <code>wsl --shutdown</code> 归还。"
          };
        }
        const idleBytes = (bd.idle_app_ws_bytes || 0) + (bd.vid_bytes || 0);
        const topIdle = (bd.top_idle_apps || []).map(x => esc(x)).join("、");
        return {
          color: "#8b5cf6",
          title: isEn ? "Idle Background Resident Working Set (Non-Foreground)" : "后台静默驻留工作集（非前沿进程）",
          val: fmtBytes(idleBytes) + " (" + pct(idleBytes) + ")",
          body: isEn
            ? ("No active large WSL2 VM locks detected; slot 2 automatically monitors physical working sets held by <b>non-foreground, non-protected background apps</b> (MEMORA gently deprioritizes or converges long-idle targets)." +
              (topIdle ? ("<br/>Top background consumers: " + topIdle) : ""))
            : ("未检测到活跃的 WSL2 / 虚机大额锁页，第二槽已自适应切换为监控<b>非前台、非受保护后台应用</b>当前占用的真实物理工作集（MEMORA 将对其中久置目标温和降级或收敛）。" +
              (topIdle ? ("<br/>主要后台驻留大户：" + topIdle) : ""))
        };
      }
      if (segKey === "zombie") {
        const lk = deep.kernel_leak || {};
        return {
          color: "#ef4444",
          title: isEn ? "Kernel Zombie EPROCESS / Lingering Page Tables" : "内核僵尸 EPROCESS / 未归还页表",
          val: fmtBytes(bd.zombie_leak_bytes) + " (" + pct(bd.zombie_leak_bytes) + ")",
          body: isEn
            ? ("Kernel alive Process objects: <b>" + (lk.process_objects || 0) + "</b> vs real active processes: <b>" + (lk.active_processes || a.process_count || 0) + "</b>" +
              ", related kernel pool: <b>" + fmtBytes(lk.related_pool_bytes) + "</b>." +
              (bd.zombie_leak_bytes > 64 * 1048576
                ? "<br/>Locked in kernel by lingering driver hook reference counts. Troubleshoot conflicting drivers in the Root Attribution tab to block further growth."
                : "<br/>Kernel object counts and system load are within healthy baseline bounds."))
            : ("内核存活 Process 对象 <b>" + (lk.process_objects || 0) + "</b> 个 vs 真实活跃进程 <b>" + (lk.active_processes || a.process_count || 0) + "</b> 个" +
              "，相关内核池 <b>" + fmtBytes(lk.related_pool_bytes) + "</b>。" +
              (bd.zombie_leak_bytes > 64 * 1048576
                ? "<br/>受驱动钩子引用计数残留锁定在内核态，可在「真相归因」页排查冲突驱动并阻断增长。"
                : "<br/>当前内核对象数量与系统负载处于健康基线范围内。"))
        };
      }
      if (segKey === "kernel") {
        const sharedWS = Math.max(0, (a.ws_bytes || 0) - (a.private_ws_bytes || 0));
        const topTags = (deep.top_nonpaged_tags || []).slice(0, 2)
          .map(t => esc(t.tag) + "(" + fmtBytes(t.bytes) + (t.drivers && t.drivers.length ? " → " + esc(t.drivers[0]) : "") + ")")
          .join("、");
        return {
          color: "#f59e0b",
          title: isEn ? "Kernel Pools · System Cache · Shared Resident Pages" : "常规内核池 · 系统文件缓存 · 共享驻留页",
          val: fmtBytes(bd.kernel_driver_bytes) + " (" + pct(bd.kernel_driver_bytes) + ")",
          body: isEn
            ? ("Includes kernel <b>NonPaged</b> (" + fmtBytes(a.kernel_nonpaged_bytes) + ") + <b>Paged</b> (" + fmtBytes(a.kernel_paged_bytes) + ") pools, system file cache (" + fmtBytes(a.system_cache_bytes || 0) + "), and multi-process shared working sets (" + fmtBytes(sharedWS) + "); shared pages and cache can be safely recycled via the relief buttons above." +
              (topTags ? ("<br/>Top nonpaged pool tags: " + topTags) : ""))
            : ("包含内核 <b>NonPaged</b> (" + fmtBytes(a.kernel_nonpaged_bytes) + ") + <b>Paged</b> (" + fmtBytes(a.kernel_paged_bytes) + ") 池、系统文件缓存 (" + fmtBytes(a.system_cache_bytes || 0) + ") 与多进程共享工作集 (" + fmtBytes(sharedWS) + ")；共享页与系统缓存可通过上方释压按钮安全回收。" +
              (topTags ? ("<br/>非分页内核池前二标签：" + topTags) : ""))
        };
      }
      return {
        color: "#10b981",
        title: isEn ? "Available Physical RAM (Free + Standby Cache)" : "立即可用物理内存（空闲 + 待机缓存）",
        val: fmtBytes(bd.avail_bytes) + " (" + pct(bd.avail_bytes) + ")",
        body: isEn
          ? ("Free zeroed pages <b>" + fmtBytes(a.free_bytes) + "</b> + Standby cache <b>" + fmtBytes(a.standby_bytes) + "</b> (Windows can immediately repurpose these for new apps in milliseconds); Modified pages pending flush: <b>" + fmtBytes(a.modified_bytes) + "</b>.")
          : ("空闲清零页 <b>" + fmtBytes(a.free_bytes) + "</b> + 待机缓存 <b>" + fmtBytes(a.standby_bytes) + "</b>（Windows 随时可秒级回收让给新开启软件）；待写回已修改页 <b>" + fmtBytes(a.modified_bytes) + "</b>。")
      };
    }

    function showBreakdownHover(segKey, anchorEl) {
      const bar = document.getElementById("bd-bar");
      const wrap = document.getElementById("bd-wrap");
      const tt = document.getElementById("bd-tooltip");
      bar.classList.add("is-hovering");
      document.querySelectorAll(".seg").forEach(el => el.classList.toggle("is-hovered", el.dataset.seg === segKey));
      if (!latestState) return;
      const info = buildSegDetail(segKey, latestState);
      tt.innerHTML =
        '<div class="bd-tt-head">' +
        '  <div class="bd-tt-title"><span class="dot" style="background:' + info.color + '"></span>' + info.title + '</div>' +
        '  <div class="bd-tt-val">' + info.val + '</div>' +
        '</div>' +
        '<div class="bd-tt-body">' + info.body + '</div>';
      tt.classList.add("show");

      const wrapRect = wrap.getBoundingClientRect();
      const targetRect = (anchorEl || bar).getBoundingClientRect();
      const center = (targetRect.left + targetRect.right) / 2 - wrapRect.left;
      const ttW = tt.offsetWidth || 320;
      let left = center - ttW / 2;
      if (left < 0) left = 0;
      if (left + ttW > wrapRect.width) left = Math.max(0, wrapRect.width - ttW);
      tt.style.left = left.toFixed(0) + "px";
    }

    function hideBreakdownHover() {
      const bar = document.getElementById("bd-bar");
      const tt = document.getElementById("bd-tooltip");
      bar.classList.remove("is-hovering");
      document.querySelectorAll(".seg").forEach(el => el.classList.remove("is-hovered"));
      tt.classList.remove("show");
    }

    (function bindBreakdownEvents() {
      const bar = document.getElementById("bd-bar");
      document.querySelectorAll(".seg").forEach(seg => {
        seg.addEventListener("mouseenter", () => showBreakdownHover(seg.dataset.seg, seg));
      });
      bar.addEventListener("mouseleave", hideBreakdownHover);
    })();

    function updateBreakdownCaps() {
      const segs = Array.from(document.querySelectorAll("#bd-bar .seg"));
      const visible = [];
      for (let i = 0; i < segs.length; i++) {
        const w = parseFloat(segs[i].style.width || "0");
        const empty = !(w > 0.05);
        segs[i].classList.toggle("seg-empty", empty);
        segs[i].classList.remove("seg-first", "seg-last");
        if (!empty) visible.push(segs[i]);
      }
      if (visible.length > 0) {
        visible[0].classList.add("seg-first");
        visible[visible.length - 1].classList.add("seg-last");
      }
    }

    function renderState(s) {
      if (!s || !s.assessment) return;
      latestState = s;
      const a = s.assessment;
      const bd = s.breakdown || {};
      const cfg = s.config || {};
      const act = cfg.actions || {};
      const thr = cfg.thresholds || {};
      const deep = s.deep || {};
      const lgWrap = s.ledger || {};
      const lg = lgWrap.data || {};
      const isGuard = s.mode === "guard";

      document.getElementById("ver-pill").textContent = "v" + (s.version || "0.3.0");
      const adminPill = document.getElementById("admin-pill");
      if (s.is_admin) {
        adminPill.textContent = t("admin_ok");
        adminPill.className = "topbar-badge admin-ok";
      } else {
        adminPill.textContent = t("admin_std");
        adminPill.className = "topbar-badge admin-std";
      }
      document.getElementById("mode-pill").textContent = isGuard ? t("mode_guard_on") : t("mode_guard_off");

      const usedPct = a.total_phys_bytes > 0 ? a.in_use_pct : 0;
      setTweenText("gauge-pct", usedPct.toFixed(1) + "%", "kpi-gauge");

      const fg = document.getElementById("gauge-fg");
      const offset = CIRCUMFERENCE * (1 - Math.max(0, Math.min(100, usedPct)) / 100);
      fg.style.strokeDashoffset = offset.toFixed(2);
      fg.style.stroke = a.smart_tier >= 3 ? "#ef4444" : (a.smart_tier === 2 ? "#f59e0b" : "#10b981");

      setTweenText("hero-used", fmtBytes(a.in_use_bytes), "kpi-used");
      document.getElementById("hero-total").textContent = t("hero_used_lbl", { tot: fmtBytes(a.total_phys_bytes) });
      setTweenText("hero-avail", fmtBytes(a.avail_phys_bytes), "kpi-avail");

      const tierPill = document.getElementById("tier-pill");
      tierPill.textContent = translateSmartLabel(a.smart_label);
      tierPill.className = "pill " + (a.smart_tier >= 3 ? "pill-red" : (a.smart_tier === 2 ? "pill-amber" : "pill-green"));

      const gamePill = document.getElementById("game-pill");
      if (a.fullscreen_quiet) {
        gamePill.textContent = t("game_quieting", { label: translateFullscreenLabel(a.fullscreen_label) });
        gamePill.className = "pill pill-amber";
      } else {
        gamePill.textContent = t("game_ready");
        gamePill.className = "pill pill-slate";
      }
      document.getElementById("family-pill").textContent = t("family_protected", { n: a.family_protected || 0 });
      document.getElementById("self-pill").textContent = t("self_stats", { ws: fmtBytes(s.self_ws_bytes), hd: s.self_handles || 0 });

      syncModeSwitchUI(isGuard);

      const baseEl = document.getElementById("baseline-pill");
      if (bd.zombie_leak_bytes >= 4 * 1073741824 && deep.conflict_pending_reboot) {
        baseEl.textContent = t("baseline_reboot", { size: fmtBytes(bd.zombie_leak_bytes) });
        baseEl.style.color = "#b45309";
        baseEl.style.borderColor = "#fde68a";
        baseEl.style.background = "#fffbeb";
      } else {
        baseEl.style.color = "";
        baseEl.style.borderColor = "";
        baseEl.style.background = "";
        if (isGuard) {
          baseEl.textContent = t("baseline_managed", { n: a.soft_managed || 0 });
        } else {
          baseEl.textContent = t("baseline_closed");
        }
      }

      const upd = s.updated_at ? new Date(s.updated_at).toLocaleTimeString() : "--";
      document.getElementById("sync-note").textContent = t("sync_note", { time: upd });

      // Commit + page list header
      document.getElementById("proc-count-pill").textContent = t("active_procs", { n: a.process_count || 0 });
      const isEn = currentLang === "en-US";
      const lblCommit = isEn ? "Commit " : "提交 ";
      const lblKern = isEn ? " · Kernel Pool " : " · 内核池 ";
      const lblStandby = isEn ? " · Standby " : " · 待机缓存 ";
      const lblMod = isEn ? " · Modified " : " · 已修改 ";
      setTweenHTML("commit-summary",
        lblCommit + hlNum(fmtBytes(a.commit_bytes), "hl-blue", "sum-commit") + " / " + fmtBytes(a.total_commit_bytes) +
        lblKern + hlNum(fmtBytes(a.kernel_total_bytes), "hl-amber", "sum-kern") +
        lblStandby + hlNum(fmtBytes(a.standby_bytes), "hl-emerald", "sum-standby") +
        lblMod + fmtBytes(a.modified_bytes)
      );

      // Adaptive 5-color Breakdown Bar & Legends (adapts Slot 2 between WSL2/Vid and Idle Background WS)
      const total = bd.total_bytes || a.total_phys_bytes || 1;
      const pctOf = v => Math.max(0, Math.min(100, (v || 0) * 100 / total));
      const isVidMode = bd.slot2_mode === "vid";
      const slot1Bytes = isVidMode ? (bd.app_private_ws_bytes || 0) : (bd.active_app_ws_bytes != null ? bd.active_app_ws_bytes : (bd.app_private_ws_bytes || 0));
      const slot2Bytes = isVidMode ? (bd.vid_bytes || 0) : ((bd.idle_app_ws_bytes || 0) + (bd.vid_bytes || 0));

      document.getElementById("seg-apps").style.width = pctOf(slot1Bytes).toFixed(2) + "%";
      document.getElementById("seg-vid").style.width = pctOf(slot2Bytes).toFixed(2) + "%";
      document.getElementById("seg-zombie").style.width = pctOf(bd.zombie_leak_bytes).toFixed(2) + "%";
      document.getElementById("seg-kernel").style.width = pctOf(bd.kernel_driver_bytes).toFixed(2) + "%";
      document.getElementById("seg-avail").style.width = pctOf(bd.avail_bytes).toFixed(2) + "%";
      updateBreakdownCaps();

      const lblApps = document.getElementById("bd-apps-label");
      const lblVid = document.getElementById("bd-vid-label");
      if (lblApps) lblApps.textContent = isVidMode ? t("bd_apps_single") : t("bd_apps_active");
      if (lblVid) lblVid.textContent = isVidMode ? t("bd_vid_label") : t("bd_idle_label");

      setTweenText("bd-apps-val", fmtBytes(slot1Bytes), "bd-apps");
      document.getElementById("bd-apps-pct").textContent = pctOf(slot1Bytes).toFixed(1) + (isVidMode ? t("bd_pct_app_single") : t("bd_pct_app_active"));
      setTweenText("bd-vid-val", fmtBytes(slot2Bytes), "bd-vid");
      document.getElementById("bd-vid-pct").textContent = pctOf(slot2Bytes).toFixed(1) + (isVidMode ? t("bd_pct_vid") : t("bd_pct_idle"));
      setTweenText("bd-zombie-val", fmtBytes(bd.zombie_leak_bytes), "bd-zombie");
      document.getElementById("bd-zombie-pct").textContent = pctOf(bd.zombie_leak_bytes).toFixed(1) + t("bd_pct_zombie");
      setTweenText("bd-kernel-val", fmtBytes(bd.kernel_driver_bytes), "bd-kernel");
      document.getElementById("bd-kernel-pct").textContent = pctOf(bd.kernel_driver_bytes).toFixed(1) + t("bd_pct_kernel");
      setTweenText("bd-avail-val", fmtBytes(bd.avail_bytes), "bd-avail");
      document.getElementById("bd-avail-pct").textContent = pctOf(bd.avail_bytes).toFixed(1) + t("bd_pct_avail");

      // Sparklines
      const hist = s.history || [];
      const spawnUnit = isEn ? " /min" : " /分";
      setTweenText("sp-use-val", usedPct.toFixed(1) + "%", "sp-use");
      setTweenText("sp-apps-val", fmtBytes(bd.app_private_ws_bytes), "sp-apps");
      setTweenText("sp-spawn-val", Math.round(a.spawn_rate_per_min || 0) + spawnUnit, "sp-spawn");
      setTweenText("sp-avail-val", fmtBytes(a.avail_phys_bytes), "sp-avail");
      renderSparkline("sp-use", hist.map(p => p.in_use_pct), "#2563eb", "rgba(37,99,235,0.12)");
      renderSparkline("sp-apps", hist.map(p => p.private_ws_gb), "#7c3aed", "rgba(124,58,237,0.12)");
      renderSparkline("sp-spawn", hist.map(p => p.spawn_rate), "#f59e0b", "rgba(245,158,11,0.14)");
      renderSparkline("sp-avail", hist.map(p => p.avail_gb), "#10b981", "rgba(16,185,129,0.12)");

      // Smart scheduling switches with highlighted animated numbers
      document.getElementById("chk-softprio").checked = !!act.soft_memory_priority;
      document.getElementById("sw-smart").checked = !!act.smart_memory_schedule;
      document.getElementById("sw-eco").checked = !!act.eco_qos_background;
      document.getElementById("sw-storm").checked = !!act.suppress_spawn_storm;
      document.getElementById("sw-standby").checked = !!act.purge_standby;
      document.getElementById("sw-full").checked = !!thr.quiet_on_fullscreen;
      const edgeOn = act.edge_horizon !== false;
      document.getElementById("sw-edge").checked = edgeOn;

      document.getElementById("dot-smart").classList.toggle("off", !isGuard || !act.smart_memory_schedule);
      document.getElementById("dot-eco").classList.toggle("off", !isGuard || !act.eco_qos_background);
      document.getElementById("dot-storm").classList.toggle("off", !isGuard || !act.suppress_spawn_storm);
      document.getElementById("dot-standby").classList.toggle("off", !isGuard || !act.purge_standby);
      document.getElementById("dot-full").classList.toggle("off", !thr.quiet_on_fullscreen);
      document.getElementById("dot-edge").classList.toggle("off", !edgeOn);

      const ew = cfg.edge_widget || {};
      const sideLabel = ew.side === "right" ? t("pill_edge_right") : t("pill_edge_left");
      const pillEdge = document.getElementById("pill-edge-side");
      if (pillEdge) {
        pillEdge.textContent = edgeOn ? sideLabel : t("pill_edge_hidden");
        pillEdge.className = "pill " + (edgeOn ? "pill-blue" : "pill-slate");
      }

      const autoLearn = !!act.auto_learn_storm_list;
      const pillAuto = document.getElementById("pill-autolearn");
      pillAuto.textContent = autoLearn ? t("pill_autolearn_on") : t("pill_autolearn_off");
      pillAuto.className = "pill " + (autoLearn ? "pill-blue" : "pill-slate");

      const btnAuto = document.getElementById("btn-autolearn");
      const btnAutoTxt = document.getElementById("btn-autolearn-txt");
      if (btnAutoTxt) btnAutoTxt.textContent = autoLearn ? t("btn_autolearn_on") : t("btn_autolearn_off");
      btnAuto.className = "btn-sm " + (autoLearn ? "btn-auto-on" : "");

      const adaptCnt = (a.adaptive_cooldown_procs || []).length;
      const adaptNote = adaptCnt > 0 ? (isEn ? (" · Anti-rebound active for " + hlNum(adaptCnt + " procs", "hl-violet", "adapt-cnt")) : (" · 已防反扑自校准 " + hlNum(adaptCnt + " 项", "hl-violet", "adapt-cnt"))) : "";
      const leakBackoffNote = a.kernel_leak_backoff ? (isEn ? (" · " + hlNum("Leak Anti-Thrash Backoff", "hl-amber", "lk-bo")) : (" · " + hlNum("内核泄漏防抖退避", "hl-amber", "lk-bo"))) : "";

      setTweenHTML("desc-smart", !isGuard
        ? (isEn ? "Smart dispatch disabled (all managed procs restored to default OS priority, passive observation only)" : "智能调度已关闭（已恢复全部受管进程默认优先级，当前仅保持采样分析）")
        : (act.smart_memory_schedule
          ? (isEn ? ("Smart dispatch active (Current " + hlNum(translateSmartLabel(a.smart_label), "hl-emerald", "tier-lbl") + "), cumulative dynamic savings: " + hlNum(fmtMB(lg.dynamic_reclaimed_mb || 0), "hl-blue", "dyn-mb") + adaptNote + leakBackoffNote)
                  : ("智能调度运行中（当前 " + hlNum(a.smart_label || "一级 · 轻度巡航", "hl-emerald", "tier-lbl") + "），累计动态调节 " + hlNum(fmtMB(lg.dynamic_reclaimed_mb || 0), "hl-blue", "dyn-mb") + adaptNote + leakBackoffNote))
          : (isEn ? "Smart dispatch switch disabled" : "智能调度开关已关闭")));

      setTweenHTML("desc-eco", !isGuard
        ? (isEn ? "Dispatch disabled: foreground and background processes scheduled at native OS priority" : "当前已关闭调度，前台与后台进程均按 Windows 原生优先级调度")
        : (act.eco_qos_background
          ? (isEn ? ("Background CPU / EcoQoS active: gently managing " + hlNum((a.soft_managed || 0) + " idle procs", "hl-indigo", "soft-cnt") + " off-foreground")
                  : ("后台 CPU / EcoQoS 已启用，当前正在温和托管 " + hlNum((a.soft_managed || 0) + " 个", "hl-indigo", "soft-cnt") + " 非前沿闲置进程"))
          : (isEn ? "Background EcoQoS scheduling disabled (default OS priority restored)" : "后台 EcoQoS 调度已关闭（已恢复全部受管进程默认优先级）")));

      setTweenHTML("desc-storm", !isGuard
        ? (isEn ? "Dispatch disabled (auto-throttling on spawn storm hosts stopped)" : "当前已关闭调度（停止自动对后台派生宿主降级）")
        : (act.suppress_spawn_storm
          ? (isEn ? ("Spawn storm suppression active (Current rate " + hlNum(Math.round(a.spawn_rate_per_min || 0) + " /min", "hl-amber", "spawn-rt") + "), suppressed " + hlNum((lg.storm_suppressions || 0) + " times", "hl-red", "storm-cnt") + ", persistent list " + hlNum((cfg.suppressed_spawners || []).length + " items", "hl-blue", "list-cnt"))
                  : ("派生风暴压制已启用（当前派生速率 " + hlNum(Math.round(a.spawn_rate_per_min || 0) + " 个/分", "hl-amber", "spawn-rt") + "），已压制 " + hlNum((lg.storm_suppressions || 0) + " 次", "hl-red", "storm-cnt") + "，长期名单 " + hlNum((cfg.suppressed_spawners || []).length + " 项", "hl-blue", "list-cnt")) )
          : (isEn ? "Spawn storm suppression disabled (rapid child spawner hosts will not be throttled)" : "派生风暴压制已关闭（高频拉起短命子进程的后台宿主将不被自动降级）")));

      setTweenHTML("desc-standby", !isGuard
        ? (isEn ? "Standby cache left entirely to Windows native eviction" : "待机缓存交由 Windows 原生淘汰")
        : (act.purge_standby
          ? (isEn ? ("Standby and modified page flush active (flushes cold standby pages under high memory pressure, current standby: " + hlNum(fmtBytes(a.standby_bytes), "hl-blue", "standby-sz") + ")")
                  : ("待机页与修改页清洗已启用（高压档自动清理低优先级待机缓存，当前待机 " + hlNum(fmtBytes(a.standby_bytes), "hl-blue", "standby-sz") + "）"))
          : (isEn ? "Standby cache flush disabled (delegated entirely to Windows memory manager)" : "待机缓存清洗已关闭（全权交由 Windows 内存管理器自行淘汰）")));

      setTweenHTML("desc-full", thr.quiet_on_fullscreen
        ? (isEn ? ("Fullscreen quiet escort active, bypassed true fullscreen scenarios " + hlNum((lg.fullscreen_skips || 0) + " times", "hl-violet", "full-skips"))
                : ("全屏免打扰已启用，已累计避让真全屏独占场景 " + hlNum((lg.fullscreen_skips || 0) + " 次", "hl-violet", "full-skips")))
        : (isEn ? "Fullscreen quiet escort disabled (continues light scheduling even during games)" : "全屏免打扰已关闭（即使全屏游戏也会继续常规轻度调度）"));

      setTweenHTML("desc-edge", !edgeOn
        ? (isEn ? "Edge Horizon disabled (toggle here or right-click tray icon)" : "贴边真相光尺已关闭（可随时在此开启或右键系统托盘切换）")
        : (a.fullscreen_quiet && thr.quiet_on_fullscreen
          ? (isEn ? "Edge Horizon enabled (detected fullscreen foreground, automatically hidden)" : "贴边真相光尺已开启（检测到全屏独占前景，当前自动静音隐身）")
          : (isEn ? ("Edge Horizon active (" + sideLabel + " · height " + hlNum(Math.round((ew.y_percent || 0.22) * 100) + "%", "hl-blue", "edge-y") + "); hover to expand CPU/GPU temp & deep relief circle; drag freely to dock")
                  : ("贴边真相光尺运行中（" + sideLabel + " · 高度 " + hlNum(Math.round((ew.y_percent || 0.22) * 100) + "%", "hl-blue", "edge-y") + "）；悬停展开 CPU/GPU 温度与深释圆环，支持全屏自由拖拽与松手自动贴边"))));

      // Tab 2: Truth cards, storm chips, spawner table
      const alertCount = renderDiagCards(deep, bd, a, s.is_admin, s.leak_forecast || {});
      const badgeTruth = document.getElementById("tab-badge-truth");
      if (alertCount > 0) {
        badgeTruth.textContent = t("badge_truth_alerts", { n: alertCount });
        badgeTruth.className = "tab-count alert";
      } else {
        badgeTruth.textContent = t("badge_truth_healthy");
        badgeTruth.className = "tab-count";
      }
      renderStormChips(cfg.suppressed_spawners || []);
      renderBurstSpawners(a.burst_spawners || []);
      renderSpawners(a.top_spawners || [], cfg.suppressed_spawners || []);

      // Tab 3: Custom-drawn managed processes + journal
      const managedList = s.managed_processes || [];
      document.getElementById("tab-badge-managed").textContent = String(managedList.length);
      renderManagedFiltered();
      renderJournal(s.recent_journal || []);

      // Tab 4: Ledger
      const unitHours = isEn ? " hrs" : " 小时";
      const unitTimes = isEn ? " times" : " 次";
      const unitProcs = isEn ? " procs" : " 个";
      setTweenHTML("lg-hours", hlNum(((lg.total_guard_seconds || 0) / 3600).toFixed(1) + unitHours, "hl-blue", "lg-h"));
      setTweenHTML("lg-restraint", hlNum((lgWrap.restraint_rate != null ? lgWrap.restraint_rate : 100).toFixed(1) + "%", "hl-emerald", "lg-r"));
      setTweenHTML("lg-dyn", hlNum(fmtMB(lg.dynamic_reclaimed_mb || 0), "hl-blue", "lg-d"));
      setTweenHTML("lg-peaks", hlNum((lg.peak_saves || 0) + " / " + (lg.trim_actions || 0) + unitTimes, "hl-amber", "lg-p"));
      setTweenHTML("lg-full", hlNum((lg.fullscreen_skips || 0) + unitTimes, "hl-violet", "lg-f"));
      setTweenHTML("lg-orphan", hlNum((lg.orphans_detected || 0) + unitProcs, "hl-indigo", "lg-o"));
      setTweenHTML("lg-storm", hlNum((lg.storm_suppressions || 0) + unitTimes, "hl-red", "lg-s"));
      setTweenHTML("lg-leak", hlNum((lg.kernel_leak_observations || 0) + unitTimes, "hl-red", "lg-l"));
      if (lg.last_kernel_leak_desc) {
        document.getElementById("lg-leak-desc").textContent = t("lg_leak_recent", { desc: lg.last_kernel_leak_desc });
      }
      renderBaselines(lg.daily_baselines || [], s.baseline_comparison || {});
      renderBloatCulprits(s.bloat_culprits || []);

      flushNumericTweens();
      syncTabSlider();
    }

    function renderDiagCards(deep, bd, a, isAdmin, forecast) {
      const box = document.getElementById("diag-cards");
      const cards = [];
      let alertCount = 0;
      const isEn = currentLang === "en-US";
      const leak = deep.kernel_leak || {};
      const extras = deep.extras || {};
      const fc = forecast || {};

      if (leak.suspected || bd.zombie_leak_bytes > 512 * 1048576 || (deep.handle_leakers || []).length > 0) {
        alertCount++;
        const allTags = (deep.top_nonpaged_tags || []).concat(deep.top_paged_tags || []);
        allTags.sort((x, y) => {
          const xd = (x.drivers && x.drivers.length) ? 1 : 0;
          const yd = (y.drivers && y.drivers.length) ? 1 : 0;
          if (xd !== yd) return yd - xd;
          const xa = (x.active || 0) >= 100000 ? 1 : 0;
          const ya = (y.active || 0) >= 100000 ? 1 : 0;
          if (xa !== ya) return ya - xa;
          return (y.bytes || 0) - (x.bytes || 0);
        });
        const findings = allTags.slice(0, 6)
          .map(t => {
            const actNote = (t.active || 0) >= 10000 ? (", " + t.active + (isEn ? " objs" : "对象")) : "";
            const drvNote = (t.drivers && t.drivers.length) ? (" → " + esc(t.drivers.join(","))) : "";
            return esc(t.tag) + "(" + fmtBytes(t.bytes) + actNote + drvNote + ")";
          })
          .join(isEn ? "; " : "；");

        const hasLenovoConflict = !!(deep.has_lenovo_conflict_drv);
        const hasDualHuorong = hasLenovoConflict && allTags.some(t => {
          const drvs = (t.drivers || []).map(d => String(d).toLowerCase());
          const hasLnv = drvs.some(d => d.indexOf("lnvscenter") >= 0 || d.indexOf("lrtp") >= 0 || d.indexOf("netrtp") >= 0);
          const hasSysdiag = drvs.some(d => d.indexOf("sysdiag") >= 0);
          return (t.tag === "cckT" && hasLnv) || (hasLnv && hasSysdiag);
        });

        const mitigated = !!(deep.conflict_mitigated);
        const pendingReboot = !!(deep.conflict_pending_reboot);
        const svcChips = (deep.conflict_services || [])
          .filter(s => s && s.exists)
          .map(s => {
            const st = s.running
              ? (isEn ? "Running" : "运行中")
              : ((isEn ? "Stopped(" : "已停止(") + esc(s.start_type || "--") + ")");
            return "<code>" + esc(s.name) + ": " + st + "</code>";
          })
          .join(" ");

        const leakers = deep.handle_leakers || [];
        const leakerStr = leakers.map(g => esc(g.name) + " (" + (g.handles || 0) + (isEn ? " handles)" : " 句柄)")).join(isEn ? ", " : "、");

        // Find any resettable high-handle service leaker (e.g. ESurfingSvr >= 5000 handles)
        const resettableSvcs = [];
        leakers.forEach(g => {
          const low = String(g.name || "").toLowerCase();
          if (low === "esurfingsvr.exe" && (g.handles || 0) >= 5000) {
            resettableSvcs.push({ svc: "ESurfingSvr", label: "ESurfingSvr (" + g.handles + (isEn ? " handles)" : " 句柄)") });
          } else if (low === "lenovosmartservice.exe" && (g.handles || 0) >= 3000) {
            resettableSvcs.push({ svc: "LenovoSmartService", label: "LenovoSmartService" });
          }
        });

        const extraDiag = [];

        // Bleed rate & exhaustion ETA forecast banner (Item 4)
        if (fc.active && fc.proc_obj_rate_per_hour > 0) {
          const objRate = Math.round(fc.proc_obj_rate_per_hour);
          const mbRate = Math.round(fc.leak_mb_per_hour || 0);
          let etaTxt = "";
          if (fc.already_critical) {
            etaTxt = isEn
              ? "<b>Already past 90% saturation — reboot recommended</b>"
              : "<b>已跌破 90% 挤兑线，建议择机重启清零</b>";
          } else if (fc.eta_hours > 0) {
            etaTxt = isEn
              ? ("Est. <b>~" + fc.eta_hours.toFixed(1) + "h</b> until 90% saturation")
              : ("预计约 <b>" + fc.eta_hours.toFixed(1) + " 小时</b>后触及 90% 挤兑线");
          }
          const fcLine = isEn
            ? ("<b>Ring-0 Bleed Forecast</b>: <code>+" + objRate + " objs/hr</code> (≈ <code>+" + mbRate + " MB/hr</code>)" + (etaTxt ? (" · " + etaTxt) : ""))
            : ("<b>内核失血速率预警</b>：<code>+" + objRate + " 对象/时</code>（实测 ≈ <code>+" + mbRate + " MB/时</code>）" + (etaTxt ? (" · " + etaTxt) : ""));
          extraDiag.push('<div style="margin:4px 0;padding:5px 8px;border-radius:7px;background:rgba(220,38,38,0.08);border:1px solid rgba(220,38,38,0.22);color:#991b1b">' + fcLine + '</div>');
        }

        if (pendingReboot) {
          extraDiag.push(isEn
            ? ("<b>Conflict Driver Disabled (Start=4)</b>: Legacy kernel hooks from earlier boot still lock ~<b>" + fmtBytes(bd.zombie_leak_bytes) + "</b>. <b>Reboot once to fully reclaim this memory</b>;")
            : ("<b>冲突驱动自启已永久关闭 (Start=4)</b>：因本机已连续运行多日未重启，本次开机初期驻留的内核钩子仍锁死约 <b>" + fmtBytes(bd.zombie_leak_bytes) + "</b> 历史进程对象与 PFN 物理页；<b>择机重启一次电脑即可彻底释放这 " + fmtBytes(bd.zombie_leak_bytes) + " 内存并永久停用该驱动</b>；"));
        } else if (hasDualHuorong && mitigated) {
          extraDiag.push(isEn
            ? "<b>Conflict Driver Mitigated</b>: Lenovo conflict drivers are stopped/disabled; remaining historical <code>EPROCESS</code> objects will clear on next reboot;"
            : "<b>冲突驱动已阻断</b>：联想冲突驱动与宿主服务当前处于停止/禁用状态；下方高占用的历史内核 <code>EPROCESS</code> / 对象残留形成于阻断之前，用户态无法抹除，将在下次电脑重启后清零；");
        } else if (hasDualHuorong) {
          extraDiag.push(isEn
            ? "<b>Kernel Filter Conflict (cckT)</b>: Coexistence of <code>lnvscenter.sys</code> and <code>sysdiag.sys</code> detected, prone to Ring-0 reference leaks;"
            : "<b>内核过滤驱动冲突 (cckT / 安全钩子)</b>：检测到联想内置安全驱动 (<code>lnvscenter.sys</code>) 与独立火绒驱动 (<code>sysdiag.sys</code>) 共存，极易在 Ring 0 导致进程退出引用计数未归零；");
        }

        // Dynamic PE Import Table Process-Hook Driver Audit (Item 2)
        const hookDrvs = (deep.hook_drivers || []).filter(d => d && d.risk_level !== "low");
        if (hookDrvs.length > 0) {
          const rows = hookDrvs.slice(0, 5).map(hd => {
            const rPill = hd.risk_level === "high"
              ? '<span class="pill pill-red" style="padding:0 5px;font-size:10px">' + (isEn ? 'HIGH' : '高危') + '</span>'
              : '<span class="pill pill-amber" style="padding:0 5px;font-size:10px">' + (isEn ? 'MED' : '中危') + '</span>';
            const ver = hd.file_version ? (" v" + esc(hd.file_version)) : "";
            const comp = hd.company ? (" · " + esc(hd.company)) : "";
            const poolInfo = hd.pool_tag
              ? (" · " + (isEn ? "Pool " : "关联池 ") + "<code>" + esc(hd.pool_tag) + "</code>(" + fmtBytes(hd.pool_bytes) + (hd.pool_active >= 1000 ? (", " + hd.pool_active + (isEn ? " objs" : "对象")) : "") + ")")
              : "";
            const hooksShort = (hd.hooks || []).map(h => h.replace("PsSetCreateProcessNotifyRoutine", "CreateProcNotify").replace("PsLookupProcessByProcessId", "LookupProcByPID").replace("PsSetCreateThreadNotifyRoutine", "CreateThreadNotify")).join(", ");
            return '<div style="margin-top:3px">' + rPill + ' <code>' + esc(hd.name) + '</code>' + ver + comp + ' [' + esc(hooksShort) + ']' + poolInfo + '</div>';
          }).join("");
          const hdrTxt = isEn
            ? ("<b>Ring-0 PE Driver Audit (Active Non-Microsoft Process Hooks)</b>" + (!hasLenovoConflict ? " — Lenovo drivers absent; inspect these active kernel hooks:" : ":"))
            : ("<b>本机第三方进程钩子驱动审计 (Ring 0 PE 导入表)</b>" + (!hasLenovoConflict ? "（联想冲突驱动已不存在，请重点二分排查以下仍在运行的进程钩子驱动）：" : "："));
          extraDiag.push('<div style="margin:4px 0;padding:6px 8px;border-radius:7px;background:rgba(15,23,42,0.035);border:1px solid #e2e8f0">' + hdrTxt + rows + '</div>');
        }

        if (svcChips) {
          extraDiag.push("<b>" + (isEn ? "Live Service/Driver State" : "实时服务/驱动状态") + "</b>：" + svcChips);
        }
        if (leakerStr) {
          extraDiag.push("<b>" + (isEn ? "High-Handle Leak Host" : "高句柄泄漏宿主") + "</b>：" + leakerStr + (isEn ? " (exceeds abnormal handle baseline);" : "（单进程句柄数已超异常基线）；"));
        }
        if (findings) {
          extraDiag.push((isEn ? "Top Kernel Pool Tags: " : "本机高占用内核池标签: ") + findings);
        }

        const privTip = (isAdmin === false) ? (isEn ? " (UAC prompt if elevation needed)" : "（若需提权将自动请求 UAC）") : "";
        const actionBtns = [];
        if (hasLenovoConflict) {
          if (mitigated) {
            actionBtns.push('<button class="btn-sm" id="btn-fix-kernel-leak" onclick="doFixKernelLeak()">' + SVG.wrench + '<span>' + (isEn ? 'Re-Verify Conflict Mitigation' : '再次校验并巩固冲突防护') + '</span></button>');
          } else {
            actionBtns.push('<button class="btn-sm btn-danger" id="btn-fix-kernel-leak" onclick="doFixKernelLeak()">' + SVG.wrench + '<span>' + (isEn ? 'Block Conflict Drivers & Reset Hosts' : '一键阻断冲突驱动并重置宿主服务') + '</span></button>');
          }
        }
        resettableSvcs.forEach(item => {
          actionBtns.push('<button class="btn-sm btn-danger" onclick="doResetLeaker(\'' + esc(item.svc) + '\')">' + SVG.wrench + '<span>' + (isEn ? ('Reset Service: ' + esc(item.label)) : ('单点重置高句柄服务: ' + esc(item.label))) + '</span></button>');
        });
        if (!hasLenovoConflict && resettableSvcs.length === 0 && leakers.length > 0) {
          actionBtns.push('<button class="btn-sm btn-danger" onclick="doSuppressStorms()">' + SVG.zap + '<span>' + (isEn ? 'Throttle High-Handle Hosts' : '立即压制高句柄泄漏宿主') + '</span></button>');
        }
        actionBtns.push('<button class="btn-sm" onclick="doDeepScan()">' + SVG.search + '<span>' + (isEn ? 'Rescan Drivers (.sys)' : '重新扫描本机驱动 (.sys)') + '</span></button>');

        const boxCls = mitigated ? "alert-amber" : "alert-red";
        const pillCls = mitigated ? "pill-amber" : "pill-red";
        const titleTxt = mitigated
          ? (isEn ? "Root Cause 1: Conflict Mitigated · Historical Residues Pending Reboot" : "根因 1：冲突源头已阻断 · 历史内核残留待重启清零")
          : (isEn ? "Root Cause 1: Kernel Object / Handle Pool Leak Attribution" : "根因 1：内核对象 / 句柄泄漏归因");
        const procLine = isEn
          ? ("    Alive kernel Process objects: <b>" + (leak.process_objects || 0) + "</b> vs active processes: <b>" + (leak.active_processes || a.process_count || 0) + "</b> (Direct pool: <b>" + fmtBytes(leak.related_pool_bytes) + "</b>; Alloc/Free: <b>" + (leak.proc_tag_allocs || 0) + " / " + (leak.proc_tag_frees || 0) + "</b>).")
          : ("    内核存活 Process 对象 <b>" + (leak.process_objects || 0) + "</b> 个 vs 真实活跃进程 <b>" + (leak.active_processes || a.process_count || 0) + "</b> 个（直连内核池 <b>" + fmtBytes(leak.related_pool_bytes) + "</b>；Proc 分配/释放 <b>" + (leak.proc_tag_allocs || 0) + " / " + (leak.proc_tag_frees || 0) + "</b>）。");
        const descLine = mitigated
          ? (isEn
              ? ("    Boot protection is active; remaining <b>" + (leak.process_objects || 0) + "</b> zombie Process objects will clear after next reboot.")
              : ("    已开启开机自动静默压制；当前残留的 <b>" + (leak.process_objects || 0) + "</b> 个内核 Process 对象会在下次重启后释放。"))
          : (isEn
              ? ("    When Ring-0 driver hooks omit <code>ObDereferenceObject</code> on process exit, zombie <code>EPROCESS</code> structures and page tables cannot be freed from user mode — isolate the leaking driver above and reboot" + privTip + ".")
              : ("    当第三方驱动过滤钩子在 Ring 0 未正确释放引用计数或后台进程疯狂泄漏句柄时，已退出的 EPROCESS 与物理页表无法被用户态强行抹除，需排查上方高危进程钩子驱动并择机重启清理" + privTip + "。"));

        cards.push([
          '<div class="alert-box spot-surface spot-sm ' + boxCls + '">',
          '  <div class="alert-title">',
          '    <span class="alert-title-l">' + SVG.alert + '<span>' + titleTxt + '</span></span>',
          '    <span class="pill ' + pillCls + '">' + fmtBytes(bd.zombie_leak_bytes) + '</span>',
          '  </div>',
          '  <div class="alert-body">',
          procLine + '<br/>',
          descLine + '<br/>',
          extraDiag.join("<br/>"),
          '  </div>',
          '  <div class="alert-actions">',
          actionBtns.join(""),
          '  </div>',
          '</div>'
        ].join(""));
      }

      if ((extras.has_vid && extras.vid_bytes > 256 * 1048576) || bd.vid_bytes > 256 * 1048576) {
        alertCount++;
        const vBytes = extras.vid_bytes || bd.vid_bytes || 0;
        cards.push([
          '<div class="alert-box spot-surface spot-sm alert-violet">',
          '  <div class="alert-title">',
          '    <span class="alert-title-l">' + SVG.box + '<span>' + (isEn ? 'Virtualization / WSL2 Locked Physical Pages (Vid)' : '虚拟化 / WSL2 容器物理锁页 (Vid)') + '</span></span>',
          '    <span class="pill pill-violet">' + fmtBytes(vBytes) + '</span>',
          '  </div>',
          '  <div class="alert-body">',
          isEn
            ? ('    <code>vmmemWSL</code> currently locks <b>' + fmtBytes(vBytes) + '</b> in physical RAM. Reclaim Linux caches or shut down WSL2 below without rebooting.')
            : ('    <code>vmmemWSL</code> 当前在物理内存中硬性锁定 <b>' + fmtBytes(vBytes) + '</b>。若当前未使用 Linux 子系统或 Docker 容器，点击下方按钮可<b>免重启</b>立即归还全部虚机内存。'),
          '  </div>',
          '  <div class="alert-actions">',
          '    <button class="btn-sm" onclick="doWSLReclaim()">' + SVG.zap + '<span>' + (isEn ? 'Drop Linux Page Cache (Keep Containers)' : '温和释放 Linux 页缓存 (不停容器)') + '</span></button>',
          '    <button class="btn-sm btn-violet" onclick="doWSLShutdown()">' + SVG.zap + '<span>' + (isEn ? 'Shutdown WSL2 & Reclaim All' : '关闭 WSL2 归还全部锁页') + '</span></button>',
          '  </div>',
          '</div>'
        ].join(""));
      }

      // Universal Multi-Process / Electron / WebView2 Cluster Sprawl Check
      const multiClusters = (deep.proc_groups || []).filter(g => g.count >= 4 && g.private_ws_bytes >= 300 * 1048576);
      const multiTotal = multiClusters.reduce((acc, g) => acc + (g.private_ws_bytes || 0), 0);
      if (multiClusters.length > 0 && (multiTotal >= 1536 * 1048576 || (a.in_use_pct || 0) >= 72)) {
        const isHighMulti = (a.in_use_pct || 0) >= 72 || multiTotal >= 4096 * 1048576;
        if (isHighMulti) alertCount++;
        const topClusters = multiClusters.slice(0, 4)
          .map(g => '<span class="mini-proc-cell">' + miniProcIconHTML(0, g.name) + '<b>' + esc(g.name) + '</b></span>×' + g.count + " (" + fmtBytes(g.private_ws_bytes) + ")")
          .join(isEn ? ", " : "、");
        cards.push([
          '<div class="alert-box spot-surface spot-sm ' + (isHighMulti ? 'alert-amber' : '') + '" style="' + (isHighMulti ? '' : 'border-color:#bfdbfe;background:#f8fafc') + '">',
          '  <div class="alert-title">',
          '    <span class="alert-title-l">' + SVG.box + '<span>' + (isEn ? 'Multi-Process Cluster Footprint (Browser / Electron / WebView2)' : '多进程簇驻留分析（浏览器 / Electron / WebView2）') + '</span></span>',
          '    <span class="pill ' + (isHighMulti ? 'pill-amber' : 'pill-blue') + '">' + fmtBytes(multiTotal) + '</span>',
          '  </div>',
          '  <div class="alert-body">',
          isEn
            ? ('    Detected <b>' + multiClusters.length + '</b> multi-process clusters occupying <b>' + fmtBytes(multiTotal) + '</b> exclusive physical working set:<br/>' + topClusters + '.<br/>MEMORA shields active foreground tabs/IDEs and gently trims only long-idle background shards.')
            : ('    检测到本机共有 <b>' + multiClusters.length + '</b> 类多进程应用簇共占据 <b>' + fmtBytes(multiTotal) + '</b> 独占物理工作集：<br/>' + topClusters + '。<br/>MEMORA 会主动避让正在操作的前沿标签页/IDE，仅对后台长期闲置分片施加温和压缩。'),
          '  </div>',
          '  <div class="alert-actions">',
          '    <button class="btn-sm" onclick="doOptimize()">' + SVG.zap + '<span>' + (isEn ? 'Trim Idle Background Shards Now' : '立即收敛闲置后台分片') + '</span></button>',
          '  </div>',
          '</div>'
        ].join(""));
      }

      // Universal Virtual Memory Commit Pressure Check (Crucial for Gamers & Creators with small Pagefile)
      const commitRatio = (a.total_commit_bytes > 0) ? ((a.commit_bytes || 0) / a.total_commit_bytes) : 0;
      if (commitRatio >= 0.78) {
        alertCount++;
        const topCommit = (deep.proc_groups || []).slice(0, 3)
          .map(g => esc(g.name) + " (" + fmtBytes(g.private_bytes) + ")")
          .join(isEn ? ", " : "、");
        cards.push([
          '<div class="alert-box spot-surface spot-sm alert-amber">',
          '  <div class="alert-title">',
          '    <span class="alert-title-l">' + SVG.alert + '<span>' + (isEn ? 'Virtual Memory Commit Limit Approaching Ceiling' : '虚拟内存提交额度 (Commit Limit) 逼近上限') + '</span></span>',
          '    <span class="pill pill-amber">' + (commitRatio * 100).toFixed(0) + '%</span>',
          '  </div>',
          '  <div class="alert-body">',
          isEn
            ? ('    Committed memory is <b>' + fmtBytes(a.commit_bytes) + '</b> / <b>' + fmtBytes(a.total_commit_bytes) + '</b>. Approaching 100% can trigger OOM crashes in games or IDEs even with free physical RAM.<br/>' + (topCommit ? ('Top private commit: ' + topCommit + '.') : 'Consider closing idle heavy apps or expanding system pagefile.'))
            : ('    系统已提交虚拟内存 <b>' + fmtBytes(a.commit_bytes) + '</b> / 上限 <b>' + fmtBytes(a.total_commit_bytes) + '</b>。当 Commit 接近 100% 时，即便仍有可用物理内存，大型游戏或创作软件也可能直接触发 OOM 崩溃。<br/>' + (topCommit ? ('最高私有提交应用：' + topCommit + '（建议开启或扩大系统托管页面文件）。') : '建议关闭闲置大型应用或调大 Windows 页面文件上限。')),
          '  </div>',
          '</div>'
        ].join(""));
      }

      const nodes = extras.numa_nodes || [];
      if (nodes.length > 0) {
        const tight = nodes.find(n => n.available_mb < 512);
        const listStr = nodes.map(n => "Node" + esc(n.id) + (isEn ? ": Avail " : ": 可用 ") + n.available_mb + "/" + n.total_mb + " MB").join(" | ");
        if (tight) {
          alertCount++;
          cards.push([
            '<div class="alert-box spot-surface spot-sm alert-amber">',
            '  <div class="alert-title"><span class="alert-title-l">' + SVG.scale + '<span>' + (isEn ? 'NUMA Node Memory Imbalance Alert' : 'NUMA 节点内存失衡预警') + '</span></span><span class="pill pill-amber">Node' + esc(tight.id) + (isEn ? ' Tight' : ' 吃紧') + '</span></div>',
            '  <div class="alert-body">' + listStr + (isEn ? '. Physical pages on one NUMA node are nearly exhausted — click "Deep Pressure Relief Now" on the left.' : '。单节点物理页已见底，建议点击左侧「立即深度释压」。') + '</div>',
            '</div>'
          ].join(""));
        }
      }

      if (cards.length === 0) {
        cards.push([
          '<div class="alert-box spot-surface spot-sm" style="border-color:#a7f3d0;background:#ecfdf5">',
          '  <div class="alert-title"><span class="alert-title-l">' + SVG.check + '<span>' + (isEn ? 'Kernel Objects, VM Locks & Commit Headroom Healthy' : '内核对象、虚机锁页与提交水位均健康') + '</span></span><span class="pill pill-green">' + (isEn ? 'Healthy' : '全机健康') + '</span></div>',
          '  <div class="alert-body">' + (isEn ? 'No zombie kernel objects, abnormal driver leaks, or commit saturation detected. MEMORA is quietly protecting your idle baseline while tracking spawn storms and background bloat.' : '当前未检测到僵尸内核对象、异常驱动泄漏或虚拟内存提交过载。MEMORA 正在后台以完全自适应的方式守卫本机空载基线，并实时追踪高频子进程派生与闲置多进程膨胀。') + '</div>',
          '</div>'
        ].join(""));
      }
      box.innerHTML = cards.join("");
      return alertCount;
    }

    function renderStormChips(list) {
      const box = document.getElementById("storm-chips");
      const isEn = currentLang === "en-US";
      if (!list || list.length === 0) {
        box.innerHTML = '<span class="card-sub">' + (isEn ? 'Suppression list is empty (auto-learn adds repeat storm hosts automatically)' : '当前压制名单为空（开启自动学习后将自动捕获并加入）') + '</span>';
        return;
      }
      box.innerHTML = list.map(name =>
        '<span class="chip">' + esc(name) +
        '<button class="chip-x" title="' + (isEn ? 'Remove from list' : '移出压制名单') + '" onclick="removeStormName(\'' + esc(name).replace(/'/g, "\\'") + '\')">×</button></span>'
      ).join("");
    }

    function renderBurstSpawners(list) {
      const bar = document.getElementById("burst-spawners-bar");
      if (!bar) return;
      const isEn = currentLang === "en-US";
      if (!list || list.length === 0) {
        bar.style.display = "none";
        return;
      }
      bar.style.display = "block";
      const hdr = isEn
        ? '<b style="color:#1d4ed8">Burst Capture (Short-Lived &lt;100ms Spawns):</b> '
        : '<b style="color:#1d4ed8">短命脉冲抓捕（&lt;100ms 瞬逝子进程）:</b> ';
      const items = list.slice(0, 4).map(b => {
        const rate = Math.round(b.est_rate_per_hour || 0);
        const leakStr = (b.est_leak_mb_per_hour >= 10)
          ? (' <span class="pill pill-red" style="padding:0 5px;font-size:10px">≈+' + Math.round(b.est_leak_mb_per_hour) + ' MB/' + (isEn ? 'h' : '时') + '</span>')
          : "";
        return '<code>' + esc(b.parent_name || "?") + '(' + (b.parent_pid || 0) + ') → ' + esc(b.child_name) + '</code> ×' + b.captured_count + ' (~' + rate + '/' + (isEn ? 'h' : '时') + ')' + leakStr;
      }).join(" · ");
      bar.innerHTML = hdr + items;
    }

    function procIconURL(pid, name) {
      return "/api/proc-icon?v=2&pid=" + encodeURIComponent(pid || 0) + "&name=" + encodeURIComponent(name || "") + "&token=" + encodeURIComponent(TOKEN);
    }

    function miniProcIconHTML(pid, name) {
      if (!name || name === "已退出") return "";
      return '<img class="mini-proc-ico" loading="lazy" decoding="async" src="' + procIconURL(pid, name) + '" alt="" onerror="this.style.display=\'none\'" />';
    }

    function renderSpawners(list, stormList) {
      const tb = document.getElementById("tbl-spawners");
      const isEn = currentLang === "en-US";
      const inRoster = {};
      (stormList || []).forEach(n => { inRoster[String(n).toLowerCase()] = true; });
      if (!list || list.length === 0) {
        tb.innerHTML = '<tr><td colspan="5" style="color:var(--muted)">' + (isEn ? 'No high-frequency spawners observed in the last 5 mins' : '近 5 分钟暂无高频子进程派生宿主') + '</td></tr>';
        return;
      }
      tb.innerHTML = list.map(sp => {
        const lowName = String(sp.name || "").toLowerCase();
        const badge = sp.throttled
          ? '<span class="pill pill-red">' + (isEn ? 'Suppressed' : '已压制') + '</span>'
          : '<span class="pill pill-slate">' + (isEn ? 'Observing' : '正常观测') + '</span>';
        const opBtn = inRoster[lowName]
          ? '<span class="card-sub">' + (isEn ? 'In List' : '已在名单') + '</span>'
          : '<button class="btn-sm" style="padding:2px 7px;font-size:11px" onclick="addStormName(\'' + esc(sp.name).replace(/'/g, "\\'") + '\')">' + (isEn ? '+ List' : '+ 名单') + '</button>';
        const ico = miniProcIconHTML(sp.pid, sp.name);
        return '<tr class="hover-row"><td>' + sp.pid + '</td><td><span class="mini-proc-cell">' + ico + '<b>' + esc(sp.name || (isEn ? "Exited" : "已退出")) + '</b></span></td><td class="num">' + sp.child_count + (isEn ? '' : ' 次') + '</td><td>' + badge + '</td><td class="num">' + opBtn + '</td></tr>';
      }).join("");
    }

    function setProcFilter(f) {
      procFilter = f;
      document.querySelectorAll(".filter-pill").forEach(el => {
        el.classList.toggle("active", el.dataset.filter === f);
      });
      requestAnimationFrame(syncFilterSlider);
      renderManagedFiltered();
    }

    function renderManagedFiltered() {
      const list = (latestState && latestState.managed_processes) || [];
      const stormList = (latestState && latestState.config && latestState.config.suppressed_spawners) || [];
      const wlList = (latestState && latestState.config && latestState.config.whitelist) || [];
      const inRoster = {};
      stormList.forEach(n => { inRoster[String(n).toLowerCase()] = true; });
      const inWL = {};
      wlList.forEach(n => { inWL[String(n).toLowerCase()] = true; });
      renderWhitelistChips(wlList);

      const q = (document.getElementById("proc-search").value || "").trim().toLowerCase();
      const filtered = list.filter(p => {
        if (procFilter === "storm" && !p.storm_throttled) return false;
        if (procFilter === "verylow" && p.memory_priority !== "VeryLow") return false;
        if (procFilter === "low" && p.memory_priority !== "Low") return false;
        if (q) {
          const hitName = String(p.name || "").toLowerCase().indexOf(q) >= 0;
          const hitPid = String(p.pid || "").indexOf(q) >= 0;
          if (!hitName && !hitPid) return false;
        }
        return true;
      });

      const isEn = currentLang === "en-US";
      document.getElementById("managed-summary").textContent = isEn
        ? (" · " + list.length + " managed · showing " + filtered.length)
        : (" · 共 " + list.length + " 个受管 · 显示 " + filtered.length + " 个");
      const rowsEl = document.getElementById("proc-custom-rows");
      if (filtered.length === 0) {
        rowsEl.innerHTML = '<div style="padding:18px;text-align:center;color:var(--muted)">' + (isEn ? 'No managed background processes under current filter' : '当前筛选条件下无受管后台进程（全屏免打扰、调度关闭或尚未出现久置进程）') + '</div>';
        return;
      }

      const maxWS = Math.max(80, ...filtered.map(p => p.working_set_mb || 0));
      rowsEl.innerHTML = filtered.map(p => {
        const initial = esc((p.name || "?").slice(0, 2).toUpperCase());
        const prioPill = p.memory_priority === "VeryLow"
          ? '<span class="pill pill-blue" style="padding:1px 6px;font-size:11px">VeryLow</span>'
          : (p.memory_priority === "Low" ? '<span class="pill pill-slate" style="padding:1px 6px;font-size:11px">Low</span>' : "");
        const ecoPill = p.eco_qos ? '<span class="pill pill-green" style="padding:1px 6px;font-size:11px">EcoQoS</span>' : "";
        const cpuPill = p.below_normal_cpu ? '<span class="pill pill-amber" style="padding:1px 6px;font-size:11px">BelowNormal</span>' : "";
        const stormPill = p.storm_throttled ? '<span class="pill pill-red" style="padding:1px 6px;font-size:11px">' + SVG.zap + '<span>' + (isEn ? 'Storm Throttled' : '风暴压制中') + '</span></span>' : "";
        const adaptPill = p.adaptive_cooldown ? '<span class="pill pill-violet" style="padding:1px 6px;font-size:11px">' + (isEn ? 'Anti-Rebound' : '防反扑自校准') + '</span>' : "";
        const barPct = Math.max(4, Math.min(100, ((p.working_set_mb || 0) * 100) / maxWS)).toFixed(1);
        const lowName = String(p.name || "").toLowerCase();
        const escapedName = esc(p.name).replace(/'/g, "\\'");
        const wlBtn = inWL[lowName]
          ? '<span class="pill pill-green" style="padding:1px 6px;font-size:11px">' + (isEn ? 'Whitelisted' : '白名单中') + '</span>'
          : '<button class="btn-sm" style="padding:2px 6px;font-size:11px;color:#047857;border-color:#a7f3d0;background:#ecfdf5" onclick="addWhitelistName(\'' + escapedName + '\')">' + (isEn ? '+ Whitelist' : '+ 白名单') + '</button>';
        const addBtn = inRoster[lowName]
          ? '<span class="pill pill-violet" style="padding:1px 6px;font-size:11px">' + (isEn ? 'In Roster' : '自动名单内') + '</span>'
          : '<button class="btn-sm" style="padding:2px 6px;font-size:11px" onclick="addStormName(\'' + escapedName + '\')">' + (isEn ? '+ Suppress' : '+ 压制') + '</button>';
        const iconSrc = procIconURL(p.pid, p.name);

        return [
          '<div class="proc-row' + (p.storm_throttled ? ' is-storm' : '') + '">',
          '  <div class="proc-avatar"><img decoding="async" src="' + iconSrc + '" alt="" onerror="this.style.display=\'none\';if(this.nextElementSibling)this.nextElementSibling.style.display=\'flex\'" /><span class="proc-avatar-fallback">' + initial + '</span></div>',
          '  <div class="proc-main">',
          '    <div class="proc-name-line">',
          '      <span class="proc-name" title="' + esc(p.name) + '">' + esc(p.name) + '</span>',
          '      <span class="proc-pid">PID ' + p.pid + '</span>',
          '      <span class="card-sub">' + (isEn ? 'Managed ' : '已托管 ') + p.managed_seconds + 's</span>',
          '    </div>',
          '    <div class="proc-tags">' + stormPill + adaptPill + prioPill + ecoPill + cpuPill + '</div>',
          '  </div>',
          '  <div class="proc-metrics">',
          '    <div class="proc-mem-txt">' + fmtMB(p.working_set_mb) + (isEn ? ' WS' : ' 工作集') + '</div>',
          '    <div class="proc-mem-sub">' + (isEn ? 'Private ' : '私有提交 ') + fmtMB(p.private_mb) + '</div>',
          '    <div class="proc-bar-bg"><div class="proc-bar-fg" style="width:' + barPct + '%"></div></div>',
          '  </div>',
          '  <div style="display:flex;gap:4px;align-items:center">' + wlBtn + addBtn + '</div>',
          '</div>'
        ].join("");
      }).join("");
    }

    function renderJournal(list) {
      const isEn = currentLang === "en-US";
      const rowsEl = document.getElementById("journal-rows");
      if (!list || list.length === 0) {
        rowsEl.innerHTML = '<div style="padding:18px;text-align:center;color:var(--muted)">' + (isEn ? 'No dispatch logs yet (MEMORA is holding restraint)' : '暂无调度日志记录（MEMORA 保持克制待命中）') + '</div>';
        return;
      }
      rowsEl.innerHTML = list.map(j => {
        const t = j.time ? new Date(j.time).toLocaleTimeString() : "";
        const targets = (j.targets || []).slice(0, 4).map(x => esc(x.name) + "(" + Math.round(x.working_set_mb) + "MB)").join(isEn ? ", " : "、");
        const rec = j.reclaimed_mb ? ((isEn ? " · Freed " : " · 释放 ") + fmtMB(j.reclaimed_mb)) : "";
        return [
          '<div class="j-item">',
          '  <div class="j-top"><span>' + esc(j.action || j.kind) + rec + '</span><span>' + t + '</span></div>',
          '  <div class="j-reason">' + esc(j.reason) + '</div>',
          targets ? ('  <div class="j-targets">' + (isEn ? 'Targets: ' : '目标: ') + targets + '</div>') : '',
          '</div>'
        ].join("");
      }).join("");
    }

    function renderBaselines(list, cmp) {
      const tb = document.getElementById("tbl-baselines");
      const badge = document.getElementById("baseline-drift-badge");
      const isEn = currentLang === "en-US";
      if (badge) {
        if (!cmp || !cmp.days_tracked) {
          badge.textContent = isEn ? "Awaiting multi-day baseline" : "待累积跨天基线";
          badge.className = "pill pill-slate";
        } else if (cmp.days_tracked === 1) {
          badge.textContent = (isEn ? "Day 1 Baseline " : "首日基准 ") + fmtMB(cmp.first_min_in_use_mb || 0);
          badge.className = "pill pill-blue";
        } else {
          const sign = cmp.drift_mb >= 0 ? "+" : "-";
          const absTxt = fmtMB(Math.abs(cmp.drift_mb || 0));
          const daysLbl = isEn ? (cmp.days_tracked + "d ") : (cmp.days_tracked + "天");
          if (cmp.stable) {
            badge.textContent = daysLbl + (isEn ? "Stable: " : "稳态: ") + fmtMB(cmp.first_min_in_use_mb) + " → " + fmtMB(cmp.latest_min_in_use_mb) + " (" + sign + absTxt + ")";
            badge.className = "pill pill-green";
          } else {
            badge.textContent = daysLbl + (isEn ? "Drift: " : "偏移: ") + fmtMB(cmp.first_min_in_use_mb) + " → " + fmtMB(cmp.latest_min_in_use_mb) + " (" + sign + absTxt + ")";
            badge.className = "pill pill-amber";
          }
        }
      }
      if (!list || list.length === 0) {
        tb.innerHTML = '<tr><td colspan="5" style="color:var(--muted)">' + (isEn ? "No multi-day records available yet" : "暂无跨天记录") + '</td></tr>';
        return;
      }
      const recent7 = list.slice(-7);
      tb.innerHTML = recent7.map(b =>
        '<tr class="hover-row"><td><b>' + esc(b.date) + '</b></td><td class="num">' + fmtMB(b.min_in_use_mb) + '</td><td class="num">' + fmtMB(b.min_private_mb) + '</td><td class="num">' + fmtMB(b.vid_mb) + '</td><td class="num">' + (b.process_objects || 0) + '</td></tr>'
      ).join("");
    }

    function renderBloatCulprits(list) {
      const tb = document.getElementById("tbl-culprits");
      if (!tb) return;
      const isEn = currentLang === "en-US";
      if (!list || list.length === 0) {
        tb.innerHTML = '<tr><td colspan="5" style="color:var(--muted)">' + (isEn ? "No bloat attribution records yet (restrained and stable)" : "暂未记录后台膨胀与虚高收敛目标（系统表现克制平稳）") + '</td></tr>';
        return;
      }
      tb.innerHTML = list.map(c => {
        const strat = c.adaptive_cooldown
          ? '<span class="pill pill-violet" style="padding:1px 6px;font-size:11px">' + t("strat_rebound") + '</span>'
          : '<span class="pill pill-blue" style="padding:1px 6px;font-size:11px">' + t("strat_trim") + '</span>';
        const hitsUnit = isEn ? " hits" : " 次";
        return '<tr class="hover-row"><td><span class="mini-proc-cell">' + miniProcIconHTML(0, c.name) + '<b>' + esc(c.name) + '</b></span></td><td class="num">' + (c.hits || 0) + hitsUnit + '</td><td class="num">' + fmtMB(c.max_ws_mb || 0) + '</td><td class="num">' + fmtMB(c.reclaimed_mb || 0) + '</td><td>' + strat + '</td></tr>';
      }).join("");
    }

    async function refreshState() {
      try {
        const s = await api("/api/state", "GET");
        renderState(s);
      } catch (e) {
        document.getElementById("sync-note").textContent = "等待守护服务响应: " + e.message;
      }
    }

    async function toggleOpt(key, enabled) {
      try {
        const s = await api("/api/toggle", "POST", { key, enabled });
        renderState(s);
        showToast(enabled ? "已开启设置并实时生效" : "已关闭设置（已恢复对应受管进程默认状态）");
      } catch (e) {
        showToast("更新失败: " + e.message);
        refreshState();
      }
    }

    async function toggleAutoLearn() {
      const cur = !!(latestState && latestState.config && latestState.config.actions && latestState.config.actions.auto_learn_storm_list);
      await toggleOpt("auto_learn_storm_list", !cur);
    }

    async function addStormName(name) {
      try {
        const data = await api("/api/storm-list", "POST", { action: "add", name: name });
        renderState(data.state);
        showToast(data.ok ? ("已将 " + name + " 加入自动压制名单并实时压制") : ("未能加入（可能属于核心系统或前台保护应用）"));
      } catch (e) {
        showToast("添加名单失败: " + e.message);
      }
    }

    function addStormNameFromInput() {
      const inp = document.getElementById("inp-storm-name");
      const val = (inp.value || "").trim();
      if (!val) return;
      addStormName(val);
      inp.value = "";
    }

    async function removeStormName(name) {
      try {
        const data = await api("/api/storm-list", "POST", { action: "remove", name: name });
        renderState(data.state);
        showToast("已将 " + name + " 移出自动压制名单并释放压制");
      } catch (e) {
        showToast("移除名单失败: " + e.message);
      }
    }

    function renderWhitelistChips(list) {
      const el = document.getElementById("whitelist-chips");
      if (!el) return;
      if (!list || list.length === 0) {
        el.innerHTML = '<span class="card-sub">暂无手动白名单（点击下方进程行「+ 白名单」或上方输入进程名即可立即移出托管并永久保护）</span>';
        return;
      }
      el.innerHTML = list.map(n =>
        '<span class="wl-chip">' + esc(n) +
        '<button title="移出白名单" onclick="removeWhitelistName(\'' + esc(n).replace(/'/g, "\\'") + '\')">×</button></span>'
      ).join("");
    }

    async function addWhitelistName(name) {
      try {
        const data = await api("/api/whitelist", "POST", { action: "add", name: name });
        renderState(data.state);
        showToast(data.ok ? ("已将 " + name + " 加入白名单并立即恢复其正常优先级") : (name + " 已在白名单中"));
      } catch (e) {
        showToast("添加白名单失败: " + e.message);
      }
    }

    function addWhitelistFromInput() {
      const inp = document.getElementById("inp-wl-name");
      const val = (inp.value || "").trim();
      if (!val) return;
      addWhitelistName(val);
      inp.value = "";
    }

    async function removeWhitelistName(name) {
      try {
        const data = await api("/api/whitelist", "POST", { action: "remove", name: name });
        renderState(data.state);
        showToast("已将 " + name + " 移出白名单");
      } catch (e) {
        showToast("移出白名单失败: " + e.message);
      }
    }

    async function setMode(guard) {
      syncModeSwitchUI(guard);
      try {
        const s = await api("/api/toggle", "POST", { key: "mode", enabled: guard });
        renderState(s);
        showToast(guard ? "已开启「智能调度」并立即重新接管后台基线" : "已切换至「关闭」并归还全部受管进程默认优先级");
      } catch (e) {
        showToast("切换失败: " + e.message);
        refreshState();
      }
    }

    let extremeConfirmTimer = null;
    let extremeArmed = false;

    function formatOptimizeToast(r, isExtreme) {
      const top = (r.targets || []).slice(0, 3).map(t => t.name + "(" + Math.round(t.reclaimed_mb) + "MB)").join("、");
      const svcNote = (r.service_trimmed_count > 0) ? ("（含系统服务 " + r.service_trimmed_count + " 个）") : "";
      const extNotes = [];
      if (r.all_working_sets_emptied) extNotes.push("已全域清空 WS");
      if (r.cache_trimmed) extNotes.push("已收敛文件缓存");
      if (r.standby_purged) extNotes.push("已排空待机页");
      const prefix = isExtreme ? "强力全域释压完成：" : "深度释压完成：";
      return prefix +
        "收紧 " + (r.trimmed_count || 0) + " 个工作集" + svcNote + " " + fmtMB(r.ws_shrunk_mb) +
        "，可用物理内存净增 " + fmtMB(r.avail_gain_mb) +
        "（耗时 " + (r.duration_ms || 0) + "ms" + (extNotes.length ? " · " + extNotes.join("、") : "") + "）" +
        (top ? " · " + top : "");
    }

    async function doOptimize() {
      const btn = document.getElementById("btn-optimize");
      const txt = document.getElementById("btn-opt-text");
      btn.disabled = true;
      if (txt) txt.textContent = "正在深度释压...";
      try {
        const data = await api("/api/optimize", "POST", { extreme: false });
        renderState(data.state);
        showToast(formatOptimizeToast(data.result || {}, false), 6500);
      } catch (e) {
        showToast("深度释压失败: " + e.message);
      } finally {
        btn.disabled = false;
        if (txt) txt.textContent = "立即深度释压";
      }
    }

    const EXT_ICONS = {
      idle: '<svg class="ico ico-sm" style="color:#2563eb" viewBox="0 0 24 24"><polygon points="11 2 3 13 10 13 9 22 17 11 10 11 11 2"/><path d="M18 5l-3 5h4l-3 5"/></svg>',
      confirm: '<svg class="ico ico-sm" style="color:#d97706" viewBox="0 0 24 24"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>',
      running: '<svg class="ico ico-sm ico-spin" viewBox="0 0 24 24"><path d="M21 12a9 9 0 1 1-6.219-8.56"/></svg>'
    };

    async function doOptimizeExtreme() {
      const btn = document.getElementById("btn-optimize-extreme");
      const ico = document.getElementById("btn-opt-ext-ico");
      const txt = document.getElementById("btn-opt-ext-text");
      if (!extremeArmed) {
        extremeArmed = true;
        if (btn) btn.classList.add("btn-confirm");
        if (ico) ico.innerHTML = EXT_ICONS.confirm;
        if (txt) txt.textContent = "再次点击确认全域清空（短暂换页）";
        clearTimeout(extremeConfirmTimer);
        extremeConfirmTimer = setTimeout(() => {
          extremeArmed = false;
          if (btn) btn.classList.remove("btn-confirm");
          if (ico) ico.innerHTML = EXT_ICONS.idle;
          if (txt) txt.textContent = "强力全域释压（含服务与缓存）";
        }, 4000);
        return;
      }
      extremeArmed = false;
      clearTimeout(extremeConfirmTimer);
      if (btn) {
        btn.classList.remove("btn-confirm");
        btn.disabled = true;
      }
      if (ico) ico.innerHTML = EXT_ICONS.running;
      if (txt) txt.textContent = "正在全域深度清空...";
      try {
        const data = await api("/api/optimize", "POST", { extreme: true });
        renderState(data.state);
        showToast(formatOptimizeToast(data.result || {}, true), 7500);
      } catch (e) {
        showToast("强力全域释压失败: " + e.message);
      } finally {
        if (btn) btn.disabled = false;
        if (ico) ico.innerHTML = EXT_ICONS.idle;
        if (txt) txt.textContent = "强力全域释压（含服务与缓存）";
      }
    }

    async function doRestore() {
      try {
        const data = await api("/api/restore", "POST", {});
        renderState(data.state);
        showToast("已归还所有被 MEMORA 软调度进程的默认 OS 优先级与 EcoQoS 状态");
      } catch (e) {
        showToast("恢复失败: " + e.message);
      }
    }

    async function doDeepScan() {
      showToast("正在扫描系统已加载驱动并匹配内核池标签...", 3000);
      try {
        const s = await api("/api/deep", "POST", {});
        renderState(s);
        showToast("深度驱动池扫描完成");
      } catch (e) {
        showToast("深度扫描失败: " + e.message);
      }
    }

    async function doSuppressStorms() {
      try {
        const data = await api("/api/suppress-storms", "POST", {});
        renderState(data.state);
        const total = data.total_throttled || data.throttled_count || 0;
        const newly = data.throttled_count || 0;
        showToast("已压制 " + total + " 个后台派生/高句柄宿主（本次新增 " + newly + " 个，已排除浏览器与代码工具链）", 5500);
      } catch (e) {
        showToast("压制失败: " + e.message);
      }
    }

    async function doFixKernelLeak() {
      const btn = document.getElementById("btn-fix-kernel-leak");
      if (btn) {
        btn.disabled = true;
        const span = btn.querySelector("span");
        if (span) span.textContent = "正在停止宿主与卸载冲突驱动...";
      }
      showToast("正在停止关联宿主并卸载冲突驱动、写入重启防护（若弹出 UAC 请允许）...", 5000);
      try {
        const data = await api("/api/fix-kernel-leak", "POST", {});
        renderState(data.state);
        showToast(data.message || (data.ok ? "已完成冲突驱动卸载与持久防护" : "修复未完全完成"), 8500);
      } catch (e) {
        showToast("冲突修复请求失败: " + e.message, 6000);
      } finally {
        const b2 = document.getElementById("btn-fix-kernel-leak");
        if (b2) b2.disabled = false;
      }
    }

    async function doBurstSpawners() {
      const isEn = currentLang === "en-US";
      const btn = document.getElementById("btn-burst-spawners");
      const txt = document.getElementById("btn-burst-spawners-txt");
      if (btn) btn.disabled = true;
      if (txt) txt.textContent = isEn ? "Capturing (2.5s)..." : "高频抓捕中 (2.5s)...";
      try {
        const data = await api("/api/burst-spawners", "POST", {});
        if (data.state) renderState(data.state);
        const list = data.burst_spawners || [];
        if (list.length > 0) {
          const top = list[0];
          showToast(isEn
            ? ("Captured " + list.length + " short-lived spawn source(s): top " + (top.parent_name || "?") + " -> " + top.child_name + " (~" + Math.round(top.est_rate_per_hour || 0) + "/hr)")
            : ("脉冲抓捕完成：发现 " + list.length + " 组短命子进程源，首位 " + (top.parent_name || "?") + " → " + top.child_name + "（折算约 " + Math.round(top.est_rate_per_hour || 0) + " 次/时）"), 6000);
        } else {
          showToast(isEn ? "Burst capture finished: no short-lived child spawns detected in 2.5s" : "脉冲抓捕完成：2.5 秒窗口内未捕获到短命子进程派生", 4000);
        }
      } catch (e) {
        showToast((isEn ? "Burst capture failed: " : "脉冲抓捕失败: ") + e.message, 5000);
      } finally {
        if (btn) btn.disabled = false;
        if (txt) txt.textContent = t("btn_burst_capture");
      }
    }

    async function doResetLeaker(svc) {
      const isEn = currentLang === "en-US";
      showToast(isEn ? ("Resetting high-handle service " + svc + "...") : ("正在安全重置高句柄服务 " + svc + "（若弹出 UAC 请允许）..."), 4500);
      try {
        const data = await api("/api/reset-leaker", "POST", { service: svc });
        if (data.state) renderState(data.state);
        showToast(data.output || (isEn ? ("Service " + svc + " reset completed") : ("服务 " + svc + " 重置完成")), 6500);
      } catch (e) {
        showToast((isEn ? "Service reset failed: " : "服务重置失败: ") + e.message, 5500);
      }
    }

    async function doWSLReclaim() {
      showToast("正在温和释放 WSL2 Linux 页缓存（不关闭容器）...", 4000);
      try {
        const data = await api("/api/wsl-reclaim", "POST", {});
        renderState(data.state);
        if (data.ok) {
          showToast("WSL2 页缓存清理完成：Vid 释放 " + fmtMB(data.vid_released_mb) + "，可用内存净增 " + fmtMB(data.avail_gain_mb), 6000);
        } else {
          showToast("温和清理返回: " + (data.error || data.output || "未检测到运行中的 WSL 实例"), 5000);
        }
      } catch (e) {
        showToast("WSL 页缓存清理失败: " + e.message);
      }
    }

    async function doWSLShutdown() {
      showToast("正在执行 wsl --shutdown 释放虚拟化锁页，请稍候...", 5000);
      try {
        const data = await api("/api/wsl-shutdown", "POST", {});
        renderState(data.state);
        if (data.ok) {
          const cfgNote = data.wslconfig_updated ? "；已同步在 .wslconfig 开启自动内存回收" : "";
          showToast("WSL2 已安全关停：Vid 锁页释放 " + fmtMB(data.vid_released_mb) + "，系统可用物理内存净增 " + fmtMB(data.avail_gain_mb) + cfgNote, 6500);
        } else {
          showToast("执行 wsl --shutdown 返回: " + (data.error || data.output || "未知错误"), 5500);
        }
      } catch (e) {
        showToast("WSL 释放请求失败: " + e.message);
      }
    }

    let exportToastTimer = null;
    let lastExportPath = "";
    function showExportToast(filename, fullPath, sizeBytes) {
      const el = document.getElementById("export-toast");
      if (!el) return;
      lastExportPath = fullPath || "";
      const kb = Math.max(1, Math.round((sizeBytes || 0) / 1024));
      el.innerHTML =
        '<svg class="ico ico-sm" style="color:#34d399;flex-shrink:0" viewBox="0 0 24 24"><polyline points="20 6 9 17 4 12"/></svg>' +
        '<div style="flex:1;min-width:0">' +
        '  <div style="font-weight:700;color:#fff;white-space:nowrap;overflow:hidden;text-overflow:ellipsis">' + esc(filename) + '</div>' +
        '  <div style="font-size:11px;color:#94a3b8">已保存至下载目录 · ' + kb + ' KB</div>' +
        '</div>' +
        '<button class="export-toast-btn" onclick="openExportedFolder()">打开目录</button>' +
        '<button class="export-toast-btn" onclick="hideExportToast()">关闭</button>';
      el.classList.add("show");
      clearTimeout(exportToastTimer);
      exportToastTimer = setTimeout(hideExportToast, 7000);
    }

    function hideExportToast() {
      const el = document.getElementById("export-toast");
      if (el) el.classList.remove("show");
    }

    async function openExportedFolder() {
      if (!lastExportPath) return;
      try {
        await api("/api/export-report?mode=open&path=" + encodeURIComponent(lastExportPath), "POST", {});
        hideExportToast();
      } catch (e) {
        showToast("打开目录失败: " + e.message, 4000);
      }
    }

    async function doExportReport() {
      try {
        const data = await api("/api/export-report?mode=save", "POST", {});
        if (data && data.ok) {
          showExportToast(data.filename, data.path, data.size_bytes);
        } else {
          showToast("导出报告失败", 4500);
        }
      } catch (e) {
        showToast("导出报告失败: " + e.message, 5000);
      }
    }

    applyStaticI18N();
    refreshState();
    setInterval(() => {
      if (!document.hidden) {
        refreshState();
      }
    }, 2500);
    document.addEventListener("visibilitychange", () => {
      if (!document.hidden) {
        refreshState();
      }
    });
  </script>
</body>
</html>
`
