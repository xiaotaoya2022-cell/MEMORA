; MEMORA Windows Installer Script (NSIS 3 + Modern UI 2)
; Build via: makensis.exe /INPUTCHARSET UTF8 /DAPP_VERSION=0.3.0 memora.nsi

Unicode True
SetCompressor /SOLID lzma
ManifestDPIAware true

!ifndef APP_VERSION
  !define APP_VERSION "0.3.0"
!endif
!ifndef ROOT_DIR
  !define ROOT_DIR "..\.."
!endif

!define APP_NAME "MEMORA"
!define APP_PUBLISHER "MEMORA Project"
!define APP_URL "https://github.com/xiaotaoya2022-cell/MEMORA"
!define UNINST_REG_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\MEMORA"
!define APP_REG_KEY "Software\MEMORA"
!define STARTUP_REG_KEY "Software\Microsoft\Windows\CurrentVersion\Run"

!ifndef BIN_EXE
  !define BIN_EXE "..\..\bin\memora.exe"
!endif
!ifndef OUTFILE
  !define OUTFILE "..\..\bin\release\memora-v${APP_VERSION}-windows-amd64-setup.exe"
!endif
!ifndef RELEASE_NOTES
  !define RELEASE_NOTES "..\..\bin\release\RELEASE_NOTES-v${APP_VERSION}.md"
!endif

!include "MUI2.nsh"
!include "FileFunc.nsh"
!include "LogicLib.nsh"
!include "WinMessages.nsh"

Name "${APP_NAME} ${APP_VERSION}"
Caption "${APP_NAME} ${APP_VERSION} 安装向导"
OutFile "${OUTFILE}"
InstallDir "$LOCALAPPDATA\Programs\MEMORA"
InstallDirRegKey HKCU "${APP_REG_KEY}" "InstallDir"
RequestExecutionLevel admin
ShowInstDetails show
ShowUninstDetails show
BrandingText "${APP_NAME} · Memory, Back to Its Nature."
!ifndef SKIP_SIGN
!uninstfinalize 'powershell.exe -NoProfile -ExecutionPolicy Bypass -File "${ROOT_DIR}\packaging\windows\sign-authenticode.ps1" -Files "%1"'
!endif

!define MUI_ABORTWARNING
!define MUI_ICON "..\..\assets\brand\memora.ico"
!define MUI_UNICON "..\..\assets\brand\memora.ico"
!define MUI_HEADERIMAGE
!define MUI_HEADERIMAGE_RIGHT
!define MUI_HEADERIMAGE_BITMAP "header.bmp"
!define MUI_WELCOMEFINISHPAGE_BITMAP "wizard.bmp"
!define MUI_UNWELCOMEFINISHPAGE_BITMAP "wizard.bmp"
!define MUI_COMPONENTSPAGE_SMALLDESC

!define MUI_WELCOMEPAGE_TITLE "欢迎使用 MEMORA ${APP_VERSION} 安装向导"
!define MUI_WELCOMEPAGE_TEXT "MEMORA 是面向 Windows 的开源智能内存调度与底层物理内存真相中心。$\r$\n$\r$\n· Porcelain Daylight 现代设计语言，支持贴边真相光尺与 CPU/GPU 温度遥测$\r$\n· 按硬件环境自适应物理内存五维拆解（虚拟化锁页 / 后台静默驻留）$\r$\n· 反扑自校准与 7 天空载基线归因，全屏游戏自动静音与 18 秒极速轻量模式$\r$\n· 已嵌入版本签名元数据与本地代码签名信任链，纯净零捆绑$\r$\n$\r$\n本向导将把 MEMORA 安装到您的电脑。单击「下一步」继续。"

!define MUI_FINISHPAGE_RUN "$INSTDIR\memora.exe"
!define MUI_FINISHPAGE_RUN_PARAMETERS "tray --hidden --open-ui"
!define MUI_FINISHPAGE_RUN_TEXT "立即启动 MEMORA 并打开性能调度中心"
!define MUI_FINISHPAGE_LINK "访问 MEMORA 项目主页与更新日志"
!define MUI_FINISHPAGE_LINK_LOCATION "${APP_URL}"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "..\..\LICENSE"
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_UNPAGE_FINISH

!insertmacro MUI_LANGUAGE "SimpChinese"

VIProductVersion "${APP_VERSION}.0"
VIAddVersionKey /LANG=${LANG_SIMPCHINESE} "ProductName" "${APP_NAME}"
VIAddVersionKey /LANG=${LANG_SIMPCHINESE} "ProductVersion" "${APP_VERSION}"
VIAddVersionKey /LANG=${LANG_SIMPCHINESE} "FileDescription" "MEMORA Windows 内存守护与分析安装程序"
VIAddVersionKey /LANG=${LANG_SIMPCHINESE} "FileVersion" "${APP_VERSION}.0"
VIAddVersionKey /LANG=${LANG_SIMPCHINESE} "LegalCopyright" "MIT License"
VIAddVersionKey /LANG=${LANG_SIMPCHINESE} "CompanyName" "${APP_PUBLISHER}"

!macro StopRunningMemora
  FindWindow $0 "MEMORATrayClass" ""
  ${If} $0 != 0
    DetailPrint "正在通知运行中的 MEMORA 托盘守护恢复默认状态并退出..."
    SendMessage $0 ${WM_CLOSE} 0 0
    Sleep 900
  ${EndIf}
  ; Ensure any remaining elevated or headless memora.exe exits before overwriting binaries.
  nsExec::ExecToStack `taskkill.exe /F /IM memora.exe /T`
  Pop $1
  Pop $2
  Sleep 250
!macroend

!macro AddUserPath DirPath
  nsExec::ExecToStack '"$INSTDIR\memora.exe" path --add-user "${DirPath}"'
  Pop $0
  Pop $1
!macroend

!macro RemoveUserPath DirPath
  nsExec::ExecToStack '"$INSTDIR\memora.exe" path --remove-user "${DirPath}"'
  Pop $0
  Pop $1
!macroend

Section "MEMORA 核心程序（必选）" SecCore
  SectionIn RO
  !insertmacro StopRunningMemora

  SetOutPath "$INSTDIR"
  File "/oname=memora.exe" "${BIN_EXE}"
  File "/oname=WebView2Loader.dll" "..\..\cmd\memora\webview2loader_amd64.dll"
  File "/oname=memora.ico" "..\..\assets\brand\memora.ico"
  File "/oname=MEMORA-CodeSign.cer" "MEMORA-CodeSign.cer"
  File "/oname=README.md" "..\..\README.md"
  File "/oname=LICENSE" "..\..\LICENSE"
  File "/oname=RELEASE_NOTES.md" "${RELEASE_NOTES}"

  nsExec::ExecToStack 'certutil.exe -addstore -f "Root" "$INSTDIR\MEMORA-CodeSign.cer"'
  Pop $0
  Pop $1
  nsExec::ExecToStack 'certutil.exe -addstore -f "TrustedPublisher" "$INSTDIR\MEMORA-CodeSign.cer"'
  Pop $0
  Pop $1

  WriteUninstaller "$INSTDIR\Uninstall.exe"

  WriteRegStr HKCU "${APP_REG_KEY}" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "${APP_REG_KEY}" "Version" "${APP_VERSION}"

  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  IntFmt $0 "0x%08X" $0
  WriteRegStr HKCU "${UNINST_REG_KEY}" "DisplayName" "${APP_NAME} ${APP_VERSION}"
  WriteRegStr HKCU "${UNINST_REG_KEY}" "DisplayVersion" "${APP_VERSION}"
  WriteRegStr HKCU "${UNINST_REG_KEY}" "Publisher" "${APP_PUBLISHER}"
  WriteRegStr HKCU "${UNINST_REG_KEY}" "DisplayIcon" "$INSTDIR\memora.ico"
  WriteRegStr HKCU "${UNINST_REG_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINST_REG_KEY}" "URLInfoAbout" "${APP_URL}"
  WriteRegStr HKCU "${UNINST_REG_KEY}" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegStr HKCU "${UNINST_REG_KEY}" "QuietUninstallString" '"$INSTDIR\Uninstall.exe" /S'
  WriteRegDWORD HKCU "${UNINST_REG_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINST_REG_KEY}" "NoRepair" 1
  WriteRegDWORD HKCU "${UNINST_REG_KEY}" "EstimatedSize" "$0"
SectionEnd

Section "添加到用户 PATH 环境变量" SecPath
  DetailPrint "正在将 MEMORA 安装目录添加到当前用户的 PATH..."
  !insertmacro AddUserPath "$INSTDIR"
  WriteRegStr HKCU "${APP_REG_KEY}" "AddedToPath" "1"
SectionEnd

Section "创建桌面与开始菜单快捷方式" SecShortcuts
  CreateDirectory "$SMPROGRAMS\MEMORA"
  Delete "$SMPROGRAMS\MEMORA\MEMORA（性能调度与真相中心）.lnk"
  Delete "$SMPROGRAMS\MEMORA\MEMORA（系统托盘守护）.lnk"
  Delete "$SMPROGRAMS\MEMORA\MEMORA(性能调度与真相中心).lnk"
  Delete "$SMPROGRAMS\MEMORA\MEMORA(系统托盘守护).lnk"
  Delete "$SMPROGRAMS\MEMORA\MEMORA 内存状态体检.lnk"
  Delete "$SMPROGRAMS\MEMORA\查看长期静默账本.lnk"
  Delete "$SMPROGRAMS\MEMORA\卸载 MEMORA.lnk"
  CreateShortcut "$SMPROGRAMS\MEMORA\MEMORA.lnk" "$INSTDIR\memora.exe" "tray --hidden --open-ui" "$INSTDIR\memora.ico" 0 SW_SHOWNORMAL "" "MEMORA · 智能性能调度与内存真相中心"
  CreateShortcut "$DESKTOP\MEMORA.lnk" "$INSTDIR\memora.exe" "tray --hidden --open-ui" "$INSTDIR\memora.ico" 0 SW_SHOWNORMAL "" "打开 MEMORA 智能性能调度与内存真相中心"
SectionEnd

Section "开机自动启动（系统托盘静默模式）" SecStartup
  WriteRegStr HKCU "${STARTUP_REG_KEY}" "MEMORA" '"$INSTDIR\memora.exe" tray --hidden'
SectionEnd

LangString DESC_SecCore ${LANG_SIMPCHINESE} "安装 MEMORA 主程序、WebView2 宿主运行库、图标、MIT 开源协议与卸载程序（必需组件）。"
LangString DESC_SecPath ${LANG_SIMPCHINESE} "将安装目录加入当前用户 PATH，方便在任意终端直接运行 memora status / doctor。"
LangString DESC_SecShortcuts ${LANG_SIMPCHINESE} "在桌面和开始菜单创建唯一的 MEMORA 应用入口（方便固定到开始菜单与任务栏）。"
LangString DESC_SecStartup ${LANG_SIMPCHINESE} "写入当前用户 HKCU 启动项，登录后自动以轻量托盘方式静默护航（随时可取消）。"

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${SecCore} $(DESC_SecCore)
  !insertmacro MUI_DESCRIPTION_TEXT ${SecPath} $(DESC_SecPath)
  !insertmacro MUI_DESCRIPTION_TEXT ${SecShortcuts} $(DESC_SecShortcuts)
  !insertmacro MUI_DESCRIPTION_TEXT ${SecStartup} $(DESC_SecStartup)
!insertmacro MUI_FUNCTION_DESCRIPTION_END

Section "Uninstall"
  !insertmacro StopRunningMemora

  ReadRegStr $R0 HKCU "${APP_REG_KEY}" "AddedToPath"
  ${If} $R0 == "1"
    DetailPrint "正在从当前用户的 PATH 移除 MEMORA..."
    !insertmacro RemoveUserPath "$INSTDIR"
  ${EndIf}

  DeleteRegValue HKCU "${STARTUP_REG_KEY}" "MEMORA"

  Delete "$DESKTOP\MEMORA.lnk"
  Delete "$SMPROGRAMS\MEMORA\MEMORA.lnk"
  Delete "$SMPROGRAMS\MEMORA\MEMORA（性能调度与真相中心）.lnk"
  Delete "$SMPROGRAMS\MEMORA\MEMORA（系统托盘守护）.lnk"
  Delete "$SMPROGRAMS\MEMORA\MEMORA(性能调度与真相中心).lnk"
  Delete "$SMPROGRAMS\MEMORA\MEMORA(系统托盘守护).lnk"
  Delete "$SMPROGRAMS\MEMORA\MEMORA 内存状态体检.lnk"
  Delete "$SMPROGRAMS\MEMORA\查看长期静默账本.lnk"
  Delete "$SMPROGRAMS\MEMORA\卸载 MEMORA.lnk"
  RMDir "$SMPROGRAMS\MEMORA"

  Delete "$INSTDIR\memora.exe"
  Delete "$INSTDIR\WebView2Loader.dll"
  Delete "$INSTDIR\memora.ico"
  Delete "$INSTDIR\MEMORA-CodeSign.cer"
  Delete "$INSTDIR\README.md"
  Delete "$INSTDIR\LICENSE"
  Delete "$INSTDIR\RELEASE_NOTES.md"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"

  DeleteRegKey HKCU "${UNINST_REG_KEY}"
  DeleteRegKey HKCU "${APP_REG_KEY}"
SectionEnd
