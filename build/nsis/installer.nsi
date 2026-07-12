; DeskCalendar 安装器（单文件，用户级，免管理员权限）
; 由 build/packaging.NSISPackager 经 -D 定义注入；亦可手动 `makensis installer.nsi` 编译。
; 关键定义：APPNAME VERSION EXE SOURCE_EXE INSTALLDIR CREATE_DESKTOP
;           CREATE_STARTMENU AUTOSTART OUTFILE [ICON]

!ifndef APPNAME
  !define APPNAME "DeskCalendar"
!endif
!ifndef VERSION
  !define VERSION "1.0.0"
!endif
!ifndef EXE
  !define EXE "deskcalendar-amd64.exe"
!endif
!ifndef SOURCE_EXE
  !error "必须定义 SOURCE_EXE（源 exe 绝对/相对路径）"
!endif
!ifndef INSTALLDIR
  !define INSTALLDIR "$LOCALAPPDATA\DeskCalendar"
!endif
!ifndef CREATE_DESKTOP
  !define CREATE_DESKTOP "1"
!endif
!ifndef CREATE_STARTMENU
  !define CREATE_STARTMENU "1"
!endif
!ifndef AUTOSTART
  !define AUTOSTART "1"
!endif
!ifndef ICON
  !define ICON ""
!endif
!ifndef OUTFILE
  !define OUTFILE "DeskCalendar-Setup.exe"
!endif

!include "MUI2.nsh"

Name "${APPNAME}"
OutFile "${OUTFILE}"
InstallDir "${INSTALLDIR}"
RequestExecutionLevel user

!if "${ICON}" != ""
  !define MUI_ICON "${ICON}"
  !define MUI_UNICON "${ICON}"
!endif

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "English"

; ── 主区段（必需）──
Section "${APPNAME}" SecMain
  SectionIn RO
  SetOutPath "$INSTDIR"
  File "${SOURCE_EXE}"

  !if "${ICON}" != ""
    File "${ICON}"
  !endif

  WriteUninstaller "$INSTDIR\uninstall.exe"

  !if "${CREATE_STARTMENU}" == "1"
    CreateDirectory "$SMPROGRAMS\${APPNAME}"
    !if "${ICON}" != ""
      CreateShortCut "$SMPROGRAMS\${APPNAME}\${APPNAME}.lnk" "$INSTDIR\${EXE}" "" "$INSTDIR\app.ico"
    !else
      CreateShortCut "$SMPROGRAMS\${APPNAME}\${APPNAME}.lnk" "$INSTDIR\${EXE}"
    !endif
  !endif
SectionEnd

; ── 桌面快捷方式（可选项，默认按 CREATE_DESKTOP）──
!if "${CREATE_DESKTOP}" == "1"
Section "桌面快捷方式" SecDesktop
!else
Section /o "桌面快捷方式" SecDesktop
!endif
  SetShellVarContext current
  !if "${ICON}" != ""
    CreateShortCut "$DESKTOP\${APPNAME}.lnk" "$INSTDIR\${EXE}" "" "$INSTDIR\app.ico"
  !else
    CreateShortCut "$DESKTOP\${APPNAME}.lnk" "$INSTDIR\${EXE}"
  !endif
SectionEnd

; ── 开机自动启动（可选项，默认按 AUTOSTART 写 HKCU Run）──
!if "${AUTOSTART}" == "1"
Section "开机自动启动" SecAuto
!else
Section /o "开机自动启动" SecAuto
!endif
  ; 注册表值：带 --minimized，使系统登录拉起时仅驻托盘（见 docs/20-Platform/Startup.md）。
  ; 引号包裹保证路径含空格时也能被正确解析（与 startup.intendedValue() 契约一致）。
  ; NSIS 双引号串内嵌字面量引号用 "" 转义。
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "${APPNAME}" """$INSTDIR\${EXE}"" --minimized"
SectionEnd

; ── 卸载 ──
Section "Uninstall"
  Delete "$INSTDIR\${EXE}"
  Delete "$INSTDIR\uninstall.exe"
  Delete "$INSTDIR\app.ico"
  Delete "$SMPROGRAMS\${APPNAME}\${APPNAME}.lnk"
  RMDir "$SMPROGRAMS\${APPNAME}"
  Delete "$DESKTOP\${APPNAME}.lnk"
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "${APPNAME}"
  RMDir "$INSTDIR"
SectionEnd
