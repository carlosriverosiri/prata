@echo off
chcp 65001 >nul
title Avinstallera Prata
echo.
echo   Avinstallerar Prata (tar bort autostart + programfiler).
echo   Personliga inställningar (nyckel, ordlista) lämnas kvar.
echo   Godkänn UAC-rutan som dyker upp.
echo.
"%~dp0prata.exe" --uninstall || pause
