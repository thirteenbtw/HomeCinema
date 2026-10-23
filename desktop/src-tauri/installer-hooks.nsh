; Allow the media server in Windows Firewall on private networks so TVs and
; phones on the LAN can reach it. Public networks stay closed.
!macro NSIS_HOOK_POSTINSTALL
  nsExec::Exec 'netsh advfirewall firewall delete rule name="HomeCinema"'
  nsExec::Exec 'netsh advfirewall firewall add rule name="HomeCinema" dir=in action=allow program="$INSTDIR\homecinema-server.exe" enable=yes profile=private'
!macroend

!macro NSIS_HOOK_PREUNINSTALL
  nsExec::Exec 'netsh advfirewall firewall delete rule name="HomeCinema"'
!macroend
