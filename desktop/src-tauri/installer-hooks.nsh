; Rivo used to ship as "HomeCinema". Remove that install first so the two do
; not live side by side; the library in %APPDATA%\HomeCinema is left alone and
; Rivo keeps using it.
!include "FileFunc.nsh"
!macro NSIS_HOOK_PREINSTALL
  ReadRegStr $R0 HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\HomeCinema" "UninstallString"
  StrCmp $R0 "" 0 hc_found
  ReadRegStr $R0 HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\HomeCinema" "UninstallString"
  StrCmp $R0 "" hc_done
  hc_found:
  ; the value is quoted; _?= keeps the uninstaller in place so ExecWait really waits
  StrCpy $R1 $R0 1
  StrCmp $R1 '"' 0 +2
  StrCpy $R0 $R0 -1 1
  ${GetParent} $R0 $R1
  ExecWait '"$R0" /S _?=$R1'
  Delete $R0
  RMDir $R1
  nsExec::Exec 'netsh advfirewall firewall delete rule name="HomeCinema"'
  hc_done:
!macroend

; Allow the media server in Windows Firewall on private networks so TVs and
; phones on the LAN can reach it. Public networks stay closed.
!macro NSIS_HOOK_POSTINSTALL
  nsExec::Exec 'netsh advfirewall firewall delete rule name="Rivo"'
  nsExec::Exec 'netsh advfirewall firewall add rule name="Rivo" dir=in action=allow program="$INSTDIR\homecinema-server.exe" enable=yes profile=private'
!macroend

!macro NSIS_HOOK_PREUNINSTALL
  nsExec::Exec 'netsh advfirewall firewall delete rule name="Rivo"'
!macroend
